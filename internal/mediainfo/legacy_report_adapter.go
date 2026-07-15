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
	MatroskaGoJSON         map[string]string
	MatroskaGoJSONRaw      map[string]string
	DynamicJSON            []dynamicJSONField
}

// attachCanonicalStore snapshots report and attaches its canonical representation to General.
func attachCanonicalStore(report *Report) {
	if report == nil {
		return
	}
	store := reportToFieldStore(*report, true)
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

// appendCanonicalSeed copies parser-owned canonical entries into ref with new sequence values.
func appendCanonicalSeed(store *fieldStore, ref streamRef, fields []fieldEntry) {
	stream := store.stream(ref)
	for _, field := range fields {
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
		MatroskaGoJSON:         stream.mkvGoJSON,
		MatroskaGoJSONRaw:      stream.mkvGoJSONRaw,
		DynamicJSON:            stream.dynamicJSON,
	}
	if clone {
		state.Fields = append([]Field(nil), state.Fields...)
		state.JSON = maps.Clone(state.JSON)
		state.JSONRaw = maps.Clone(state.JSONRaw)
		state.MatroskaGoJSON = maps.Clone(state.MatroskaGoJSON)
		state.MatroskaGoJSONRaw = maps.Clone(state.MatroskaGoJSONRaw)
		state.DynamicJSON = append([]dynamicJSONField(nil), state.DynamicJSON...)
	}
	return state
}

// cloneLegacyStream copies mutable legacy state while detaching canonical-store internals.
func cloneLegacyStream(stream Stream) Stream {
	stream.reportStore = nil
	stream.reportSnapshot = nil
	stream.canonicalSeed = nil
	stream.Fields = append([]Field(nil), stream.Fields...)
	stream.JSON = maps.Clone(stream.JSON)
	stream.JSONRaw = maps.Clone(stream.JSONRaw)
	stream.mkvGoJSON = maps.Clone(stream.mkvGoJSON)
	stream.mkvGoJSONRaw = maps.Clone(stream.mkvGoJSONRaw)
	stream.dynamicJSON = append([]dynamicJSONField(nil), stream.dynamicJSON...)
	stream.mkvHeaderStripBytes = append([]byte(nil), stream.mkvHeaderStripBytes...)
	return stream
}
