package mediainfo

import (
	"slices"
	"sync"
)

// fillMode selects how a fill interacts with an existing canonical field.
type fillMode uint8

// Fill modes preserve legacy replacement, first-value, and tag-append behavior.
const (
	fillReplace fillMode = iota
	fillFirstNonEmpty
	fillAppend
)

// streamRef identifies a prepared stream within one field store.
type streamRef int

// fieldEntry is one ordered canonical value plus its projection metadata.
type fieldEntry struct {
	Name          fieldName
	Value         fieldValue
	Dynamic       bool
	Options       fieldOptions
	Path          []string
	Sequence      uint32
	TextLabel     string
	StructuredKey string
	Node          *structuredNode
	Projected     bool
	DeriveString  bool
	LegacyJSON    bool
	LegacyJSONRaw bool
	LegacyValue   string
	// StructuredDecimals preserves evidenced fixed precision for measured decimal fields.
	StructuredDecimals uint8
}

// canonicalStreamPolicy retains legacy rendering exceptions for canonical snapshots.
type canonicalStreamPolicy struct {
	SkipStreamOrder bool
	SkipComputed    bool
	// HideTypeOrderXML retains generated JSON type order while omitting the
	// redundant XML child used only by direct canonical streams.
	HideTypeOrderXML bool
}

// MarkLegacyJSON records the legacy JSON snapshot value for an existing structured field.
func (s *fieldStore) MarkLegacyJSON(ref streamRef, name fieldName, value string, raw bool) {
	stream := s.stream(ref)
	if stream == nil {
		return
	}
	for index := range slices.Backward(stream.Fields) {
		entry := &stream.Fields[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if key != string(name) || !entry.Options.ShowStructured {
			continue
		}
		entry.LegacyJSON = !raw
		entry.LegacyJSONRaw = raw
		entry.LegacyValue = value
		return
	}
}

// canonicalStreamSnapshot projects one direct-fill stream into the public legacy Stream shape.
func canonicalStreamSnapshot(store *fieldStore, ref streamRef, policy canonicalStreamPolicy) Stream {
	if store == nil || store.stream(ref) == nil {
		return Stream{}
	}
	store.streams[ref].SkipStreamOrder = policy.SkipStreamOrder
	store.streams[ref].SkipComputed = policy.SkipComputed
	store.streams[ref].HideTypeOrderXML = policy.HideTypeOrderXML
	finalizeFieldStore(store)
	projection := projectTextStore(store, "")
	stream := Stream{
		Kind:                store.streams[ref].Kind,
		JSONSkipStreamOrder: policy.SkipStreamOrder,
		JSONSkipComputed:    policy.SkipComputed,
		canonicalPolicy:     policy,
	}
	for _, projected := range projection.Streams {
		if projected.Kind == stream.Kind {
			stream.Fields = append([]Field(nil), projected.Fields...)
			break
		}
	}
	for _, entry := range store.streams[ref].Fields {
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.LegacyJSON {
			if stream.JSON == nil {
				stream.JSON = make(map[string]string)
			}
			stream.JSON[key] = entry.LegacyValue
		}
		if entry.LegacyJSONRaw {
			if stream.JSONRaw == nil {
				stream.JSONRaw = make(map[string]string)
			}
			stream.JSONRaw[key] = entry.LegacyValue
		}
	}
	stream.canonicalSeed = append([]fieldEntry(nil), store.streams[ref].Fields...)
	return stream
}

// refreshCanonicalLegacySnapshot regenerates one direct stream's public legacy
// fields and override maps from its canonical seed without changing media facts.
func refreshCanonicalLegacySnapshot(stream *Stream) {
	if stream == nil || len(stream.canonicalSeed) == 0 {
		return
	}
	legacyJSON := stream.JSON
	legacyJSONRaw := stream.JSONRaw
	compatibilityDeletes := append([]fieldName(nil), stream.compatibilityDeletes...)
	markCanonicalSeedLegacySnapshot(stream)
	store := &fieldStore{}
	ref := store.Prepare(stream.Kind)
	appendCanonicalSeed(store, ref, stream.canonicalSeed)
	snapshot := canonicalStreamSnapshot(store, ref, canonicalStreamPolicy{
		SkipStreamOrder:  stream.canonicalPolicy.SkipStreamOrder || stream.JSONSkipStreamOrder,
		SkipComputed:     stream.canonicalPolicy.SkipComputed || stream.JSONSkipComputed,
		HideTypeOrderXML: stream.canonicalPolicy.HideTypeOrderXML,
	})
	stream.Fields = snapshot.Fields
	stream.JSON = snapshot.JSON
	stream.JSONRaw = snapshot.JSONRaw
	for _, key := range compatibilityDeletes {
		delete(stream.JSON, string(key))
	}
	if legacyJSON != nil && stream.JSON == nil {
		stream.JSON = map[string]string{}
	}
	if streamOrder := legacyJSON["StreamOrder"]; streamOrder != "" {
		stream.JSON["StreamOrder"] = streamOrder
	}
	if legacyJSONRaw != nil && stream.JSONRaw == nil {
		stream.JSONRaw = map[string]string{}
	}
	stream.JSONSkipStreamOrder = snapshot.JSONSkipStreamOrder
	stream.JSONSkipComputed = snapshot.JSONSkipComputed
	stream.canonicalPolicy = snapshot.canonicalPolicy
	stream.compatibilityDeletes = compatibilityDeletes
	stream.canonicalSeed = snapshot.canonicalSeed
}

// markCanonicalSeedLegacySnapshot records which direct entries belong in the
// exported compatibility override maps and their exact historical values.
func markCanonicalSeedLegacySnapshot(stream *Stream) {
	for key, value := range stream.JSON {
		markCanonicalSeedLegacyValue(stream, key, value, false)
	}
	for key, value := range stream.JSONRaw {
		markCanonicalSeedLegacyValue(stream, key, value, true)
	}
}

// markCanonicalSeedLegacyValue attaches one compatibility-map value to its
// canonical structured entry when that entry exists.
func markCanonicalSeedLegacyValue(stream *Stream, key, value string, raw bool) {
	if key == "" || value == "" {
		return
	}
	for index := range slices.Backward(stream.canonicalSeed) {
		entry := &stream.canonicalSeed[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if !entry.Options.ShowStructured || structuredKey != key {
			continue
		}
		if entry.LegacyJSON || entry.LegacyJSONRaw {
			return
		}
		entry.LegacyJSON = !raw
		entry.LegacyJSONRaw = raw
		entry.LegacyValue = value
		return
	}
}

// setCanonicalSeedLegacyValue replaces one exported compatibility-map value
// after parser logic has moved its authoritative value into the canonical seed.
func setCanonicalSeedLegacyValue(stream *Stream, key fieldName, value string, raw bool) {
	if stream == nil || key == "" || value == "" {
		return
	}
	for index := range slices.Backward(stream.canonicalSeed) {
		entry := &stream.canonicalSeed[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if !entry.Options.ShowStructured || structuredKey != string(key) {
			continue
		}
		entry.LegacyJSON = !raw
		entry.LegacyJSONRaw = raw
		entry.LegacyValue = value
		return
	}
}

// clearCanonicalSeedLegacyValue removes one exported scalar compatibility
// marker while retaining its authoritative canonical fact.
func clearCanonicalSeedLegacyValue(stream *Stream, key fieldName) {
	if stream == nil || key == "" {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if !entry.Options.ShowStructured || structuredKey != string(key) {
			continue
		}
		entry.LegacyJSON = false
		entry.LegacyJSONRaw = false
		entry.LegacyValue = ""
	}
}

// canonicalSeedLegacyJSONValue returns an explicitly migrated scalar retained
// for the exported JSON compatibility snapshot.
func canonicalSeedLegacyJSONValue(stream Stream, key fieldName) (string, bool) {
	for index := range slices.Backward(stream.canonicalSeed) {
		entry := stream.canonicalSeed[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.LegacyJSON && structuredKey == string(key) {
			return entry.LegacyValue, true
		}
	}
	return "", false
}

// markCanonicalCompatibilityDeletion records a scalar to remove when the
// parser's partial public compatibility snapshot is next refreshed.
func markCanonicalCompatibilityDeletion(stream *Stream, key fieldName) {
	if stream == nil || key == "" {
		return
	}
	if slices.Contains(stream.compatibilityDeletes, key) {
		return
	}
	stream.compatibilityDeletes = append(stream.compatibilityDeletes, key)
}

// refreshCanonicalLegacyMaps projects explicitly migrated compatibility-map
// entries without replacing a partially migrated stream's legacy text fields.
func refreshCanonicalLegacyMaps(stream *Stream) {
	if stream == nil {
		return
	}
	for _, entry := range stream.canonicalSeed {
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.LegacyJSON {
			if stream.JSON == nil {
				stream.JSON = map[string]string{}
			}
			stream.JSON[key] = entry.LegacyValue
		}
		if entry.LegacyJSONRaw {
			if stream.JSONRaw == nil {
				stream.JSONRaw = map[string]string{}
			}
			stream.JSONRaw[key] = entry.LegacyValue
		}
	}
	for _, key := range stream.compatibilityDeletes {
		delete(stream.JSON, string(key))
	}
}

// storedStream owns one stream's canonical entries and projection ordering metadata.
type storedStream struct {
	Kind                     StreamKind
	TextKind                 StreamKind
	Fields                   []fieldEntry
	TextSequence             int
	StructuredSequence       int
	TypeOrder                int
	StructuredAlreadyOrdered bool
	StructuredOrder          map[string]int
	TextAlreadyOrdered       bool
	SkipStreamOrder          bool
	SkipComputed             bool
	HideTypeOrderXML         bool
	DirectCanonical          bool
}

// fieldStore is an ordered, per-report canonical field store.
//
// Its projection lock permits concurrent rendering after setup. XML compatibility data is
// initialized at most once before XML projection acquires the read lock.
type fieldStore struct {
	ref            string
	streams        []storedStream
	nextSequence   uint32
	legacyGeneral  *Stream
	legacyStreams  []Stream
	legacySnapshot *legacyReportState
	finalized      bool
	xmlOnce        sync.Once
	projectionMu   sync.RWMutex
}

// Prepare appends a stream of kind and returns its stable store reference.
func (s *fieldStore) Prepare(kind StreamKind) streamRef {
	ref := streamRef(len(s.streams))
	s.streams = append(s.streams, storedStream{
		Kind:               kind,
		TextKind:           kind,
		TextSequence:       len(s.streams),
		StructuredSequence: len(s.streams),
	})
	return ref
}

// Fill applies value to name using mode and creates a known or dynamic entry when absent.
func (s *fieldStore) Fill(ref streamRef, name fieldName, value string, mode fillMode) {
	stream := s.stream(ref)
	if stream == nil {
		return
	}
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		if entry.Name != name {
			continue
		}
		switch mode {
		case fillReplace:
			entry.Value.Text = value
		case fillFirstNonEmpty:
			if entry.Value.Text == "" && value != "" {
				entry.Value.Text = value
			}
		case fillAppend:
			if entry.Value.Text == "" {
				entry.Value.Text = value
			} else if value != "" {
				entry.Value.Text += " / " + value
			}
		}
		return
	}
	spec, known := lookupFieldSpec(stream.Kind, name)
	if !known {
		spec = fieldSpec{
			Name:          name,
			Options:       fieldOptions{ShowText: true, ShowStructured: true, ShowXML: true, ValueType: fieldValueString},
			StructuredKey: string(name),
			TextLabel:     string(name),
		}
	}
	s.appendEntry(stream, fieldEntry{
		Name:          name,
		Value:         fieldValue{Text: value},
		Dynamic:       !known,
		Options:       spec.Options,
		TextLabel:     spec.TextLabel,
		StructuredKey: spec.StructuredKey,
		DeriveString:  spec.StringSibling != "",
	})
}

// Clear removes every entry named name from ref.
func (s *fieldStore) Clear(ref streamRef, name fieldName) {
	stream := s.stream(ref)
	if stream == nil {
		return
	}
	stream.Fields = slices.DeleteFunc(stream.Fields, func(entry fieldEntry) bool {
		return entry.Name == name
	})
}

// Get returns the first value for name in ref.
func (s *fieldStore) Get(ref streamRef, name fieldName) (string, bool) {
	stream := s.stream(ref)
	if stream == nil {
		return "", false
	}
	for _, entry := range stream.Fields {
		if entry.Name == name {
			return entry.Value.Text, true
		}
	}
	return "", false
}

// stream returns ref's stored stream, or nil for an invalid reference.
func (s *fieldStore) stream(ref streamRef) *storedStream {
	if ref < 0 || int(ref) >= len(s.streams) {
		return nil
	}
	return &s.streams[ref]
}

// appendEntry assigns encounter sequence before appending entry.
func (s *fieldStore) appendEntry(stream *storedStream, entry fieldEntry) {
	entry.Sequence = s.nextSequence
	s.nextSequence++
	stream.Fields = append(stream.Fields, entry)
}
