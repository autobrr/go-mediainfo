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
}

// canonicalStreamPolicy retains legacy rendering exceptions for canonical snapshots.
type canonicalStreamPolicy struct {
	SkipStreamOrder    bool
	SkipComputed       bool
	SkipFrameRateRatio bool
	PreserveDisplayAR  bool
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
	finalizeFieldStore(store)
	projection := projectTextStore(store, "")
	stream := Stream{
		Kind:                   store.streams[ref].Kind,
		JSONSkipStreamOrder:    policy.SkipStreamOrder,
		JSONSkipComputed:       policy.SkipComputed,
		JSONSkipFrameRateRatio: policy.SkipFrameRateRatio,
		JSONPreserveDisplayAR:  policy.PreserveDisplayAR,
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

// storedStream owns one stream's canonical entries and projection ordering metadata.
type storedStream struct {
	Kind                     StreamKind
	TextKind                 StreamKind
	Fields                   []fieldEntry
	TextSequence             int
	StructuredSequence       int
	TypeOrder                int
	StructuredAlreadyOrdered bool
	TextAlreadyOrdered       bool
	SkipStreamOrder          bool
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
