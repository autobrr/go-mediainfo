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
	publishCanonicalProjectionPolicy(&report.General)
	for index := range report.Streams {
		publishCanonicalProjectionPolicy(&report.Streams[index])
	}
	snapshot := captureLegacyReportState(*report, true)
	store.legacySnapshot = &snapshot
	report.General.reportStore = store
	report.General.reportSnapshot = &snapshot
}

// canonicalStoreForReport reuses an unchanged analysis store or rebuilds one from legacy input.
func canonicalStoreForReport(report Report) *fieldStore {
	if report.General.reportStore != nil && report.General.reportSnapshot != nil {
		current := captureLegacyReportState(report, false)
		if reflect.DeepEqual(*report.General.reportSnapshot, current) {
			return report.General.reportStore
		}
	}
	return reportToFieldStore(report, false)
}

// legacyReportToFieldStore converts caller-supplied public report state into a field store.
func legacyReportToFieldStore(report Report) *fieldStore {
	return reportToFieldStore(report, false)
}

// reportToFieldStore builds a projection store, preferring direct parser seeds when requested.
func reportToFieldStore(report Report, useCanonicalSeeds bool) *fieldStore {
	legacyMedia := buildJSONMedia(report)
	containerFormat := firstNonEmpty(report.General.JSON["Format"], findField(report.General.Fields, "Format"))
	store := &fieldStore{ref: report.Ref}
	generalRef := store.Prepare(StreamGeneral)
	store.streams[generalRef].TextSequence = 0
	store.streams[generalRef].TextKind = report.General.Kind
	store.streams[generalRef].StructuredSequence = 0
	store.streams[generalRef].StructuredAlreadyOrdered = true
	store.streams[generalRef].TextAlreadyOrdered = true
	appendLegacyTextFields(store, generalRef, report.General.Fields)
	if len(legacyMedia.Tracks) > 0 {
		appendLegacyStructuredFields(store, generalRef, legacyMedia.Tracks[0].Fields, structuredProjectionJSON)
	}
	if useCanonicalSeeds && len(report.General.canonicalSeed) > 0 {
		store.streams[generalRef].StructuredAlreadyOrdered = false
		store.streams[generalRef].StructuredOrder = structuredFieldOrderForContainer(StreamGeneral, containerFormat)
		appendCanonicalSeed(store, generalRef, report.General.canonicalSeed)
		mergeStoredStructuredObjectProjection(&store.streams[generalRef], "extra", structuredProjectionJSON)
		mergeStoredMarkedScalarProjections(&store.streams[generalRef], structuredProjectionJSON)
	}

	refs := make([]streamRef, len(report.Streams))
	for index, stream := range report.Streams {
		ref := store.Prepare(stream.Kind)
		refs[index] = ref
		stored := store.stream(ref)
		stored.TextKind = stream.Kind
		stored.TextSequence = index + 1
		stored.StructuredAlreadyOrdered = !useCanonicalSeeds || len(stream.canonicalSeed) == 0
		stored.TextAlreadyOrdered = !useCanonicalSeeds || len(stream.canonicalSeed) == 0
		if useCanonicalSeeds && len(stream.canonicalSeed) > 0 {
			stored.DirectCanonical = true
			stored.SkipStreamOrder = stream.canonicalPolicy.SkipStreamOrder || stream.JSONSkipStreamOrder
			stored.SkipComputed = stream.canonicalPolicy.SkipComputed || stream.JSONSkipComputed
			stored.HideTypeOrderXML = stream.canonicalPolicy.HideTypeOrderXML
			stored.StructuredOrder = structuredFieldOrderForContainer(stream.Kind, containerFormat)
			appendCanonicalSeed(store, ref, stream.canonicalSeed)
		} else {
			appendLegacyTextFields(store, ref, stream.Fields)
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
		if useCanonicalSeeds && len(report.Streams[originalIndex].canonicalSeed) > 0 {
			appendCanonicalGeneratedFields(store, refs[originalIndex], fields)
			if stored.HideTypeOrderXML {
				setStoredStructuredXMLVisibility(stored, "@typeorder", false)
			}
			continue
		}
		appendLegacyStructuredFields(store, refs[originalIndex], fields, structuredProjectionJSON)
	}
	general := cloneLegacyStream(report.General)
	store.legacyGeneral = &general
	store.legacyStreams = make([]Stream, len(report.Streams))
	for index, stream := range report.Streams {
		store.legacyStreams[index] = cloneLegacyStream(stream)
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
		report := Report{Ref: store.ref, General: cloneLegacyStream(*store.legacyGeneral)}
		report.Streams = make([]Stream, len(store.legacyStreams))
		for index, stream := range store.legacyStreams {
			report.Streams[index] = cloneLegacyStream(stream)
		}
		if len(store.streams) > 0 {
			appendLegacyStructuredFields(store, 0, buildJSONGeneralFields(report), structuredProjectionXML)
			mergeStoredStructuredObjectProjection(&store.streams[0], "extra", structuredProjectionXML)
			mergeStoredMarkedScalarProjections(&store.streams[0], structuredProjectionXML)
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
			fields := buildJSONStreamFields(report.Streams[originalIndex], order, 0, containerFormat)
			appendLegacyStructuredFields(store, ref, fields, structuredProjectionXML)
		}
	})
}

// mergeStoredMarkedScalarProjections keeps the canonical copy of each
// explicitly migrated compatibility scalar and hides its projected duplicate.
func mergeStoredMarkedScalarProjections(stream *storedStream, target structuredProjectionTarget) {
	if stream == nil {
		return
	}
	keys := map[string]struct{}{}
	for _, entry := range stream.Fields {
		if entry.LegacyJSON && entry.Node == nil {
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
		if field.Options.ShowStructured && (key == "@type" || key == "@typeorder" || key == "StreamOrder") {
			continue
		}
		field.Sequence = 0
		store.appendEntry(stream, field)
	}
}

// appendCanonicalGeneratedFields imports report-level type and stream-order metadata.
func appendCanonicalGeneratedFields(store *fieldStore, ref streamRef, fields []jsonKV) {
	stream := store.stream(ref)
	for _, field := range fields {
		switch field.Key {
		case "StreamOrder":
			if stream != nil && stream.SkipStreamOrder {
				continue
			}
			appendLegacyStructuredFields(store, ref, []jsonKV{field}, structuredProjectionJSON)
			appendLegacyStructuredFields(store, ref, []jsonKV{field}, structuredProjectionXML)
		case "@type", "@typeorder":
			appendLegacyStructuredFields(store, ref, []jsonKV{field}, structuredProjectionJSON)
			appendLegacyStructuredFields(store, ref, []jsonKV{field}, structuredProjectionXML)
		}
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
		compat.Fields = report.General.Fields
		report.General = compat
	}
	if len(store.legacyStreams) == len(report.Streams) {
		for index := range report.Streams {
			fields := report.Streams[index].Fields
			report.Streams[index] = cloneLegacyStream(store.legacyStreams[index])
			report.Streams[index].Fields = fields
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

// finalizeMatroskaLegacySnapshots publishes TrackEntry compatibility maps only
// after statistics, probes, delays, and tags have finalized canonical values.
func finalizeMatroskaLegacySnapshots(info *MatroskaInfo) {
	if info == nil {
		return
	}
	for index := range info.Tracks {
		info.Tracks[index].matroskaLegacySnapshot.ApplyToStream(&info.Tracks[index])
	}
}

// ApplyToStream attaches compatibility markers only to fields already owned by
// the canonical seed, then materializes the legacy maps without changing text.
func (facts *matroskaLegacySnapshotFacts) ApplyToStream(stream *Stream) {
	if facts == nil || stream == nil {
		return
	}
	streamOrder := ""
	for _, fact := range facts.values {
		if fact.name == "StreamOrder" {
			streamOrder = fact.value
			continue
		}
		if _, exists := canonicalSeedLegacyJSONValue(*stream, fact.name); !exists {
			setCanonicalSeedLegacyValue(stream, fact.name, fact.value, false)
		}
	}
	for _, name := range facts.rawNodes {
		setCanonicalSeedLegacyObject(stream, name)
	}
	refreshCanonicalLegacyMaps(stream)
	if stream.JSON == nil {
		stream.JSON = map[string]string{}
	}
	if stream.JSONRaw == nil {
		stream.JSONRaw = map[string]string{}
	}
	if streamOrder != "" {
		stream.JSON["StreamOrder"] = streamOrder
	}
	stream.matroskaLegacySnapshot = nil
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

// cloneLegacyStream copies mutable legacy state while detaching canonical-store internals.
func cloneLegacyStream(stream Stream) Stream {
	stream.reportStore = nil
	stream.reportSnapshot = nil
	stream.canonicalSeed = nil
	stream.canonicalPolicy = canonicalStreamPolicy{}
	stream.compatibilityDeletes = nil
	stream.Fields = append([]Field(nil), stream.Fields...)
	stream.JSON = maps.Clone(stream.JSON)
	stream.JSONRaw = maps.Clone(stream.JSONRaw)
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
