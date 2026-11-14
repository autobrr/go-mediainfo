package mediainfo

import (
	"maps"
	"reflect"
	"sort"
	"strconv"
)

// legacyReportState captures the public report state used to detect caller mutation.
type legacyReportState struct {
	Ref     string
	General legacyStreamState
	Streams []legacyStreamState
}

// legacyStreamState preserves all legacy rendering inputs for one stream.
type legacyStreamState struct {
	Kind                   StreamKind
	Fields                 []Field
	JSON                   map[string]string
	JSONRaw                map[string]string
	JSONSkipStreamOrder    bool
	JSONSkipComputed       bool
	JSONSkipFrameRateRatio bool
	JSONPreserveDisplayAR  bool
}

// attachCanonicalStore snapshots report and attaches its canonical representation to General.
func attachCanonicalStore(report *Report) {
	if report == nil {
		return
	}
	store := reportToFieldStore(*report, true)
	publishCanonicalCompatibilitySnapshot(report, store)
	publishCanonicalTextFields(report, store)
	publishCanonicalProjectionPolicy(&report.General)
	for index := range report.Streams {
		publishCanonicalProjectionPolicy(&report.Streams[index])
	}
	snapshot := captureLegacyReportState(*report, true)
	store.legacySnapshot = &snapshot
	report.General.reportStore = store
	report.General.reportSnapshot = &snapshot
}

// publishCanonicalCompatibilitySnapshot materializes exported JSON maps only
// at the public Report seam. Parsers and finalizers retain canonical entries;
// caller mutation remains supported by the legacy adapter fallback.
func publishCanonicalCompatibilitySnapshot(report *Report, store *fieldStore) {
	if report == nil || store == nil {
		return
	}
	projection := projectStructuredStore(store, structuredProjectionJSON)
	streamIndexes := make(map[StreamKind]int)
	for _, projected := range projection.Streams {
		var target *Stream
		if projected.Kind == StreamGeneral {
			target = &report.General
		} else {
			ordinal := streamIndexes[projected.Kind]
			for index := range report.Streams {
				if report.Streams[index].Kind != projected.Kind {
					continue
				}
				if ordinal == 0 {
					target = &report.Streams[index]
					break
				}
				ordinal--
			}
			streamIndexes[projected.Kind]++
		}
		if target == nil {
			continue
		}
		target.JSON = make(map[string]string)
		target.JSONRaw = make(map[string]string)
		for _, field := range projected.Fields {
			switch field.Value.Kind {
			case structuredString:
				target.JSON[field.Key] = field.Value.Text
			case structuredNumber, structuredBool, structuredNull, structuredObject, structuredArray, structuredRaw:
				target.JSONRaw[field.Key] = renderStructuredNode(field.Value)
			}
		}
	}
}

// publishCanonicalTextFields replaces each stream's public Fields with the
// finalized text projection, so they keep matching the rendered text after
// report-wide steps such as title selection and source-tag rows.
func publishCanonicalTextFields(report *Report, store *fieldStore) {
	projection := projectTextStore(store, "")
	ordinals := make(map[StreamKind]int)
	for _, projected := range projection.Streams {
		var target *Stream
		if projected.Kind == StreamGeneral {
			target = &report.General
		} else {
			ordinal := ordinals[projected.Kind]
			ordinals[projected.Kind]++
			for index := range report.Streams {
				if report.Streams[index].Kind != projected.Kind {
					continue
				}
				if ordinal == 0 {
					target = &report.Streams[index]
					break
				}
				ordinal--
			}
		}
		if target != nil {
			target.Fields = append([]Field(nil), projected.Fields...)
		}
	}
}

// publishCanonicalStreamCompatibilitySnapshot materializes deprecated public
// fields and maps from a canonical store without feeding them back into it.
func publishCanonicalStreamCompatibilitySnapshot(stream Stream, store *fieldStore) Stream {
	report := Report{}
	if stream.Kind == StreamGeneral {
		report.General = stream
	} else {
		report.Streams = []Stream{stream}
	}
	publishCanonicalCompatibilitySnapshot(&report, store)
	if stream.Kind == StreamGeneral {
		publishCanonicalProjectionPolicy(&report.General)
		return report.General
	}
	publishCanonicalProjectionPolicy(&report.Streams[0])
	return report.Streams[0]
}

// refreshCanonicalCompatibilitySnapshot regenerates one stream's deprecated
// public snapshot without changing canonical media facts.
func refreshCanonicalCompatibilitySnapshot(stream *Stream) {
	if stream == nil {
		return
	}
	store := &fieldStore{}
	ref := store.Prepare(stream.Kind)
	appendCanonicalSeed(store, ref, stream.canonicalSeed)
	store.streams[ref].CodecIdentity = stream.codecIdentity
	snapshot := canonicalStreamSnapshot(store, ref, stream.canonicalPolicy)
	stream.Fields = snapshot.Fields
	stream.JSON = snapshot.JSON
	stream.JSONRaw = snapshot.JSONRaw
	stream.JSONSkipStreamOrder = snapshot.JSONSkipStreamOrder
	stream.JSONSkipComputed = snapshot.JSONSkipComputed
	stream.canonicalPolicy = snapshot.canonicalPolicy
	stream.canonicalSeed = snapshot.canonicalSeed
}

// canonicalStoreForReport reuses an unchanged analysis store or rebuilds one from legacy input.
// Rebuilds retain a parser-selected reference version when legacy input does not select one.
func canonicalStoreForReport(report Report) *fieldStore {
	if report.General.reportStore != nil && report.General.reportSnapshot != nil {
		current := captureLegacyReportState(report, false)
		if reflect.DeepEqual(*report.General.reportSnapshot, current) {
			return report.General.reportStore
		}
		version := report.General.reportStore.tableVersion
		report = publicReportMutationDelta(report, *report.General.reportSnapshot)
		store := reportToFieldStore(report, false)
		if store.tableVersion == "" {
			store.tableVersion = version
		}
		return store
	}
	return reportToFieldStore(report, false)
}

// publicReportMutationDelta removes adapter-published map values that callers
// left unchanged. Legacy rebuilds then apply only explicit map mutations over
// the caller's current Fields projection.
func publicReportMutationDelta(report Report, snapshot legacyReportState) Report {
	report.General = cloneLegacyStream(report.General)
	stripUnchangedSnapshotMaps(&report.General, snapshot.General)
	report.Streams = append([]Stream(nil), report.Streams...)
	for index := range report.Streams {
		report.Streams[index] = cloneLegacyStream(report.Streams[index])
		if index < len(snapshot.Streams) {
			stripUnchangedSnapshotMaps(&report.Streams[index], snapshot.Streams[index])
		}
	}
	return report
}

// parserPresentationFactRows maps facts whose friendly display cannot recover
// the original canonical value to the row that displays them.
var parserPresentationFactRows = map[string]string{
	"Format":                    "Format",
	"Format_AdditionalFeatures": "Format",
	"Format_Profile":            "Format profile",
	"Language":                  "Language",
}

// stripUnchangedSnapshotMaps keeps explicit compatibility-map changes and
// invalidates parser lookup identity when the caller changes its codec ID.
func stripUnchangedSnapshotMaps(stream *Stream, snapshot legacyStreamState) {
	if firstNonEmpty(findField(stream.Fields, "Codec ID"), findField(stream.Fields, "CodecID")) !=
		firstNonEmpty(findField(snapshot.Fields, "Codec ID"), findField(snapshot.Fields, "CodecID")) {
		stream.codecIdentity = codecTableIdentity{}
	}
	for key := range snapshot.JSON {
		if _, exists := stream.JSON[key]; !exists {
			if stream.legacyJSONDeleted == nil {
				stream.legacyJSONDeleted = make(map[string]struct{})
			}
			stream.legacyJSONDeleted[key] = struct{}{}
		}
	}
	for key := range snapshot.JSONRaw {
		if _, exists := stream.JSONRaw[key]; !exists {
			if stream.legacyJSONRawDeleted == nil {
				stream.legacyJSONRawDeleted = make(map[string]struct{})
			}
			stream.legacyJSONRawDeleted[key] = struct{}{}
		}
	}
	// Friendly format and language rows cannot recover their canonical facts
	// (including language suffix case), so keep a fact unless its row was edited.
	for key, value := range stream.JSON {
		if snapshot.JSON[key] == value {
			if row, ok := parserPresentationFactRows[key]; ok && findField(stream.Fields, row) == findField(snapshot.Fields, row) {
				continue
			}
			delete(stream.JSON, key)
		} else if key == "CodecID" {
			stream.codecIdentity = codecTableIdentity{}
		}
	}
	for key, value := range stream.JSONRaw {
		if snapshot.JSONRaw[key] == value {
			delete(stream.JSONRaw, key)
		} else if key == "CodecID" {
			stream.codecIdentity = codecTableIdentity{}
		}
	}
}

// legacyReportToFieldStore converts caller-supplied public report state into a field store.
func legacyReportToFieldStore(report Report) *fieldStore {
	return reportToFieldStore(report, false)
}

// reportToFieldStore builds a projection store, preferring direct parser seeds when requested.
func reportToFieldStore(report Report, useCanonicalSeeds bool) *fieldStore {
	needsLegacyInput := !useCanonicalSeeds || len(report.General.canonicalSeed) == 0
	for _, stream := range report.Streams {
		needsLegacyInput = needsLegacyInput || len(stream.canonicalSeed) == 0
	}
	var legacyMedia jsonMediaOut
	if needsLegacyInput {
		structured := report
		structured.General = legacyStructuredTagNames(report.General)
		structured.Streams = append([]Stream(nil), report.Streams...)
		for index := range structured.Streams {
			structured.Streams[index] = legacyStructuredTagNames(structured.Streams[index])
		}
		legacyMedia = buildJSONMedia(structured)
	}
	store := &fieldStore{ref: report.Ref}
	generalRef := store.Prepare(StreamGeneral)
	store.streams[generalRef].TextSequence = 0
	store.streams[generalRef].TextKind = report.General.Kind
	store.streams[generalRef].StructuredSequence = 0
	if useCanonicalSeeds && len(report.General.canonicalSeed) > 0 {
		store.streams[generalRef].DirectCanonical = true
		appendCanonicalSeed(store, generalRef, report.General.canonicalSeed)
	} else {
		appendLegacyTextFields(store, generalRef, report.General.Fields)
		restoreMatroskaTagIdentities(store.stream(generalRef), report.General)
		if len(legacyMedia.Tracks) > 0 {
			fields := filterDeletedLegacyJSONFields(legacyMedia.Tracks[0].Fields, report.General)
			appendLegacyStructuredFields(store, generalRef, fields, structuredProjectionJSON)
		}
	}

	refs := make([]streamRef, len(report.Streams))
	for index, stream := range report.Streams {
		ref := store.Prepare(stream.Kind)
		refs[index] = ref
		stored := store.stream(ref)
		stored.TextKind = stream.Kind
		stored.CodecIdentity = stream.codecIdentity
		if stream.canonicalPolicy.DVDOrder {
			store.tableVersion = reportTables26
		}
		stored.TextSequence = index + 1
		if useCanonicalSeeds && len(stream.canonicalSeed) > 0 {
			stored.DirectCanonical = true
			stored.SkipStreamOrder = stream.canonicalPolicy.SkipStreamOrder
			stored.SkipComputed = stream.canonicalPolicy.SkipComputed
			stored.HideTypeOrderXML = stream.canonicalPolicy.HideTypeOrderXML

			appendCanonicalSeed(store, ref, stream.canonicalSeed)
		} else {
			appendLegacyTextFields(store, ref, stream.Fields)
			restoreMatroskaTagIdentities(stored, stream)
		}
	}
	sortedIndexes := make([]int, len(report.Streams))
	for index := range report.Streams {
		sortedIndexes[index] = index
	}
	sort.SliceStable(sortedIndexes, func(left, right int) bool {
		return streamKindOrder[report.Streams[sortedIndexes[left]].Kind] < streamKindOrder[report.Streams[sortedIndexes[right]].Kind]
	})
	for order, originalIndex := range sortedIndexes {
		stored := store.stream(refs[originalIndex])
		stored.StructuredSequence = order + 1
		trackIndex := order + 1
		if trackIndex >= len(legacyMedia.Tracks) {
			continue
		}
		fields := legacyMedia.Tracks[trackIndex].Fields
		if typeOrder := jsonKVValue(fields, "@typeorder"); typeOrder != "" {
			stored.TypeOrder, _ = strconv.Atoi(typeOrder)
		}
		if stored.DirectCanonical {
			continue
		}
		fields = filterDeletedLegacyJSONFields(fields, report.Streams[originalIndex])
		appendLegacyStructuredFields(store, refs[originalIndex], fields, structuredProjectionJSON)
	}
	if needsLegacyInput {
		general := cloneLegacyStream(report.General)
		store.legacyGeneral = &general
		store.legacyStreams = make([]Stream, len(report.Streams))
		for index, stream := range report.Streams {
			store.legacyStreams[index] = cloneLegacyStream(stream)
		}
	}
	if useCanonicalSeeds {
		// Interpret titles only at the complete parsed-report boundary, never
		// while adapting a caller-built or caller-edited report for rendering.
		finalizeGeneralTitle(store)
		finalizeFieldStore(store)
	} else {
		for ref := range store.streams {
			clearAudioSampleFrequencyFrameRate(store, streamRef(ref))
		}
	}
	return store
}

// ensureLegacyXMLProjection lazily adds XML-only compatibility fields once per store.
func ensureLegacyXMLProjection(store *fieldStore) {
	if store == nil {
		return
	}
	store.xmlOnce.Do(func() {
		store.projectionMu.Lock()
		defer store.projectionMu.Unlock()
		if store.legacyGeneral == nil {
			return
		}
		report := Report{Ref: store.ref, General: legacyStructuredTagNames(cloneLegacyStream(*store.legacyGeneral))}
		report.Streams = make([]Stream, len(store.legacyStreams))
		for index, stream := range store.legacyStreams {
			report.Streams[index] = legacyStructuredTagNames(cloneLegacyStream(stream))
		}
		if len(store.streams) > 0 && !store.streams[0].DirectCanonical {
			appendLegacyStructuredFields(store, 0, buildJSONGeneralFields(report), structuredProjectionXML)
			mergeStoredStructuredObjectProjection(&store.streams[0], "extra", structuredProjectionXML)
			mergeStoredCanonicalScalarProjections(&store.streams[0], structuredProjectionXML)
		}
		indexes := make([]int, len(report.Streams))
		for index := range report.Streams {
			indexes[index] = index
		}
		sort.SliceStable(indexes, func(left, right int) bool {
			return streamKindOrder[report.Streams[indexes[left]].Kind] < streamKindOrder[report.Streams[indexes[right]].Kind]
		})
		containerFormat := firstNonEmpty(report.General.JSON["Format"], findField(report.General.Fields, "Format"))
		for order, originalIndex := range indexes {
			ref := streamRef(originalIndex + 1)
			stored := store.stream(ref)
			if stored == nil || stored.DirectCanonical {
				continue
			}
			fields := buildJSONStreamFields(report.Streams[originalIndex], order, 0, containerFormat, reportVersion(store))
			fields = filterDeletedLegacyJSONFields(fields, report.Streams[originalIndex])
			appendLegacyStructuredFields(store, ref, fields, structuredProjectionXML)
			clearAudioSampleFrequencyFrameRate(store, ref)
		}
	})
}

// mergeStoredCanonicalScalarProjections keeps the direct canonical copy of
// each scalar and hides any compatibility-adapter duplicate.
func mergeStoredCanonicalScalarProjections(stream *storedStream, target structuredProjectionTarget) {
	if stream == nil {
		return
	}
	keys := map[string]struct{}{}
	for _, entry := range stream.Fields {
		if !entry.Projected && entry.Options.ShowStructured && entry.Node == nil {
			keys[firstNonEmpty(entry.StructuredKey, string(entry.Name))] = struct{}{}
		}
	}
	for key := range keys {
		mergeStoredStructuredScalarProjection(stream, key, target)
	}
}

// mergeStoredStructuredScalarProjection removes repeated projected copies of
// key while retaining the direct canonical scalar for target.
func mergeStoredStructuredScalarProjection(stream *storedStream, key string, target structuredProjectionTarget) {
	if stream == nil || key == "" {
		return
	}
	canonicalIndex := -1
	indexes := make([]int, 0, 2)
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		visible := target == structuredProjectionJSON && entry.Options.ShowStructured || target == structuredProjectionXML && entry.Options.ShowXML
		if !visible || structuredKey != key || entry.Node != nil {
			continue
		}
		indexes = append(indexes, index)
		if !entry.Projected {
			canonicalIndex = index
		}
	}
	if canonicalIndex < 0 || len(indexes) < 2 {
		return
	}
	for _, index := range indexes {
		if index == canonicalIndex {
			continue
		}
		if target == structuredProjectionJSON {
			stream.Fields[index].Options.ShowStructured = false
		} else {
			stream.Fields[index].Options.ShowXML = false
		}
	}
}

// mergeStoredStructuredObjectProjection folds repeated object fields into the
// projected legacy object while retaining canonical visibility for other targets.
func mergeStoredStructuredObjectProjection(stream *storedStream, key string, target structuredProjectionTarget) {
	if stream == nil || key == "" {
		return
	}
	indexes := make([]int, 0, 2)
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		visible := target == structuredProjectionJSON && entry.Options.ShowStructured || target == structuredProjectionXML && entry.Options.ShowXML
		if visible && structuredKey == key && entry.Node != nil && entry.Node.Kind == structuredObject {
			indexes = append(indexes, index)
		}
	}
	if len(indexes) < 2 {
		return
	}
	baseIndex := indexes[0]
	for _, index := range indexes {
		if stream.Fields[index].Projected {
			baseIndex = index
			break
		}
	}
	base := &stream.Fields[baseIndex]
	positions := make(map[string]int, len(base.Node.Object))
	for index, member := range base.Node.Object {
		if _, exists := positions[member.Key]; !exists {
			positions[member.Key] = index
		}
	}
	for _, index := range indexes {
		if index == baseIndex {
			continue
		}
		entry := &stream.Fields[index]
		for _, member := range entry.Node.Object {
			if position, exists := positions[member.Key]; exists {
				current := &base.Node.Object[position].Value
				if current.Kind == structuredString && member.Value.Kind == structuredString && current.Text != member.Value.Text {
					current.Text += " / " + member.Value.Text
				}
				continue
			}
			positions[member.Key] = len(base.Node.Object)
			base.Node.Object = append(base.Node.Object, member)
		}
		if target == structuredProjectionJSON {
			entry.Options.ShowStructured = false
		} else {
			entry.Options.ShowXML = false
		}
	}
	base.Value.Text = structuredNodeText(*base.Node)
}

// appendCanonicalSeed copies parser-owned canonical entries into ref with new sequence values.
func appendCanonicalSeed(store *fieldStore, ref streamRef, fields []fieldEntry) {
	stream := store.stream(ref)
	for _, field := range fields {
		key := firstNonEmpty(field.StructuredKey, string(field.Name))
		if field.Options.ShowStructured && (key == "@type" || key == "@typeorder") {
			continue
		}
		if field.Options.ShowStructured && key == "StreamOrder" && stream.SkipStreamOrder {
			continue
		}
		field.Sequence = 0
		store.appendEntry(stream, field)
	}
}

// fieldStoreToLegacyReport projects store into the public Report compatibility shape.
func fieldStoreToLegacyReport(store *fieldStore) Report {
	if store == nil {
		return Report{}
	}
	projection := projectTextStore(store, store.ref)
	report := Report{Ref: store.ref}
	for _, stream := range projection.Streams {
		if stream.Kind == StreamGeneral && report.General.Kind == "" {
			report.General = Stream{Kind: StreamGeneral, Fields: stream.Fields}
			continue
		}
		report.Streams = append(report.Streams, Stream{Kind: stream.Kind, Fields: stream.Fields})
	}
	if store.legacyGeneral != nil {
		compat := cloneLegacyStream(*store.legacyGeneral)
		report.General = compat
	}
	if len(store.legacyStreams) == len(report.Streams) {
		for index := range report.Streams {
			report.Streams[index] = cloneLegacyStream(store.legacyStreams[index])
		}
	}
	snapshot := captureLegacyReportState(report, true)
	store.legacySnapshot = &snapshot
	report.General.reportStore = store
	report.General.reportSnapshot = &snapshot
	return report
}

// appendLegacyTextFields imports legacy display fields as text-only canonical entries.
func appendLegacyTextFields(store *fieldStore, ref streamRef, fields []Field) {
	stream := store.stream(ref)
	for _, field := range fields {
		name, _, known := textFieldName(stream.Kind, field.Name)
		store.appendEntry(stream, fieldEntry{
			Name:      name,
			Value:     fieldValue{Text: field.Value},
			Dynamic:   !known,
			Options:   fieldOptions{ShowText: true, ValueType: fieldValueString},
			TextLabel: field.Name,
			Projected: true,
		})
	}
}

// appendLegacyStructuredFields imports one legacy structured projection into ref.
func appendLegacyStructuredFields(store *fieldStore, ref streamRef, fields []jsonKV, target structuredProjectionTarget) {
	stream := store.stream(ref)
	for _, field := range fields {
		spec, known := structuredFieldSpec(stream.Kind, field.Key)
		entry := fieldEntry{
			Name:          spec.Name,
			Value:         fieldValue{Text: field.Val},
			Dynamic:       !known,
			Options:       fieldOptions{ValueType: spec.Options.ValueType},
			Path:          []string{field.Key},
			StructuredKey: field.Key,
			Projected:     true,
		}
		if spec.Measure == fieldMeasureMilliseconds && !field.Raw {
			if milliseconds, ok := decimalSecondsToMilliseconds(field.Val); ok {
				entry.Value.Text = milliseconds
				entry.Projected = false
				entry.StructuredOverride = &structuredNode{Kind: structuredString, Text: field.Val}
			}
		}
		if target == structuredProjectionJSON {
			entry.Options.ShowStructured = true
		} else {
			entry.Options.ShowXML = true
		}
		if field.Raw {
			node, err := parseStructuredNode(field.Val)
			if err != nil {
				node = structuredNode{Kind: structuredRaw, Text: field.Val}
			}
			entry.Node = &node
			entry.Options.ValueType = fieldValueNode
		}
		store.appendEntry(stream, entry)
	}
}

// filterDeletedLegacyJSONFields removes compatibility-map keys that callers
// explicitly deleted without suppressing their independent text projection.
func filterDeletedLegacyJSONFields(fields []jsonKV, stream Stream) []jsonKV {
	if len(stream.legacyJSONDeleted) == 0 && len(stream.legacyJSONRawDeleted) == 0 {
		return fields
	}
	filtered := make([]jsonKV, 0, len(fields))
	for _, field := range fields {
		deleted := stream.legacyJSONDeleted
		if field.Raw {
			deleted = stream.legacyJSONRawDeleted
		}
		if _, exists := deleted[field.Key]; exists {
			continue
		}
		filtered = append(filtered, field)
	}
	return filtered
}

// jsonKVValue returns the first field value with key.
func jsonKVValue(fields []jsonKV, key string) string {
	for _, field := range fields {
		if field.Key == key {
			return field.Val
		}
	}
	return ""
}

// captureLegacyReportState records report's legacy render inputs, optionally deep-cloning them.
func captureLegacyReportState(report Report, clone bool) legacyReportState {
	state := legacyReportState{Ref: report.Ref, General: captureLegacyStreamState(report.General, clone)}
	state.Streams = make([]legacyStreamState, len(report.Streams))
	for index, stream := range report.Streams {
		state.Streams[index] = captureLegacyStreamState(stream, clone)
	}
	return state
}

// finalizeMatroskaDeferredFacts merges TrackEntry fallback facts after
// statistics, probes, delays, and tags have finalized canonical values.
func finalizeMatroskaDeferredFacts(info *MatroskaInfo) {
	if info == nil {
		return
	}
	for index := range info.Tracks {
		info.Tracks[index].matroskaDeferredFacts.ApplyToStream(&info.Tracks[index])
		refreshCanonicalCompatibilitySnapshot(&info.Tracks[index])
	}
}

// ApplyToStream merges deferred Matroska facts into the canonical seed.
func (facts *matroskaDeferredFacts) ApplyToStream(stream *Stream) {
	if facts == nil || stream == nil {
		return
	}
	streamOrder := ""
	for _, fact := range facts.values {
		if fact.name == "StreamOrder" {
			streamOrder = fact.value
			continue
		}
		if _, exists := projectedCanonicalSeedValue(*stream, fact.name); !exists {
			replaceCanonicalSeedFill(stream, fact.name, fact.value, "", "")
		}
	}
	if streamOrder != "" {
		replaceCanonicalSeedFill(stream, "StreamOrder", streamOrder, "", "")
	}
	stream.matroskaDeferredFacts = nil
}

// captureLegacyStreamState records one stream's legacy render inputs, optionally deep-cloning them.
func captureLegacyStreamState(stream Stream, clone bool) legacyStreamState {
	state := legacyStreamState{
		Kind:                   stream.Kind,
		Fields:                 stream.Fields,
		JSON:                   stream.JSON,
		JSONRaw:                stream.JSONRaw,
		JSONSkipStreamOrder:    stream.JSONSkipStreamOrder,
		JSONSkipComputed:       stream.JSONSkipComputed,
		JSONSkipFrameRateRatio: stream.JSONSkipFrameRateRatio,
		JSONPreserveDisplayAR:  stream.JSONPreserveDisplayAR,
	}
	if clone {
		state.Fields = append([]Field(nil), state.Fields...)
		state.JSON = maps.Clone(state.JSON)
		state.JSONRaw = maps.Clone(state.JSONRaw)
	}
	return state
}

// matroskaTagIdentities retains original keys only for unchanged display rows.
// Caller changes to a row's label or value do not authorize guessing its old identity.
func matroskaTagIdentities(stream Stream) map[legacyTagRow]legacyTagIdentity {
	tags := maps.Clone(stream.canonicalPolicy.LegacyTagIdentities)
	occurrences := make(map[Field]int)
	for _, entry := range stream.canonicalSeed {
		if !entry.Options.ShowText {
			continue
		}
		identity := reportFieldIdentity(reportTables23, stream.Kind, string(entry.Name), entry.TextLabel)
		if entry.MatroskaTag && entry.StructuredKey != "" {
			identity = entry.StructuredKey
		}
		label := reportFieldLabel(reportTables23, identity, entry.TextLabel)
		field := Field{Name: label, Value: entry.Value.Text}
		row := legacyTagRow{Label: label, Value: entry.Value.Text, Occurrence: occurrences[field]}
		occurrences[field]++
		if entry.MatroskaTag {
			if tags == nil {
				tags = make(map[legacyTagRow]legacyTagIdentity)
			}
			tags[row] = legacyTagIdentity{Key: entry.StructuredKey, Label: entry.TextLabel}
		}
	}
	return tags
}

// legacyStructuredTagNames supplies producer-owned extra keys to the legacy
// structured adapter, so translated display labels cannot become parser facts.
func legacyStructuredTagNames(stream Stream) Stream {
	tags := matroskaTagIdentities(stream)
	if len(tags) == 0 {
		return stream
	}
	stream.Fields = append([]Field(nil), stream.Fields...)
	occurrences := make(map[Field]int)
	for index, field := range stream.Fields {
		row := legacyTagRow{Label: field.Name, Value: field.Value, Occurrence: occurrences[field]}
		occurrences[field]++
		if tag, exists := tags[row]; exists && tag.Key != "" {
			stream.Fields[index].Name = tag.Key
		}
	}
	return stream
}

// restoreMatroskaTagIdentities restores metadata only on unchanged original rows.
func restoreMatroskaTagIdentities(stored *storedStream, stream Stream) {
	tags := matroskaTagIdentities(stream)
	occurrences := make(map[Field]int)
	for index := range stored.Fields {
		entry := &stored.Fields[index]
		if !entry.Options.ShowText {
			continue
		}
		field := Field{Name: entry.TextLabel, Value: entry.Value.Text}
		row := legacyTagRow{Label: field.Name, Value: field.Value, Occurrence: occurrences[field]}
		occurrences[field]++
		if tag, exists := tags[row]; exists {
			entry.MatroskaTag = true
			entry.StructuredKey = tag.Key
			entry.TextLabel = tag.Label
			if tag.Key != "" {
				entry.Name = fieldName("MatroskaTag/" + tag.Key)
				entry.Dynamic = true
			}
		}
	}
}

// cloneLegacyStream copies mutable legacy state while detaching canonical-store internals.
func cloneLegacyStream(stream Stream) Stream {
	stream.reportStore = nil
	stream.reportSnapshot = nil
	tags := matroskaTagIdentities(stream)
	stream.canonicalSeed = nil
	stream.canonicalPolicy = canonicalStreamPolicy{LegacyTagIdentities: tags}
	stream.Fields = append([]Field(nil), stream.Fields...)
	stream.JSON = maps.Clone(stream.JSON)
	stream.JSONRaw = maps.Clone(stream.JSONRaw)
	stream.legacyJSONDeleted = maps.Clone(stream.legacyJSONDeleted)
	stream.legacyJSONRawDeleted = maps.Clone(stream.legacyJSONRawDeleted)
	stream.mkvHeaderStripBytes = append([]byte(nil), stream.mkvHeaderStripBytes...)
	return stream
}

// publishCanonicalProjectionPolicy copies format-neutral parser policy into
// the exported legacy flags after the canonical report store has been built.
func publishCanonicalProjectionPolicy(stream *Stream) {
	if stream == nil || len(stream.canonicalSeed) == 0 {
		return
	}
	stream.JSONSkipStreamOrder = stream.canonicalPolicy.SkipStreamOrder
	stream.JSONSkipComputed = stream.canonicalPolicy.SkipComputed
}
