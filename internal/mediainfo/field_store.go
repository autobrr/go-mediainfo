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
	// StructuredOverride retains an exact serializer representation without
	// replacing the parser-owned canonical value or node.
	StructuredOverride *structuredNode
	Projected          bool
	Generated          bool
	DeriveString       bool
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

// OverrideStructured records an exact structured representation for an
// existing canonical field without mutating its source value.
func (s *fieldStore) OverrideStructured(ref streamRef, name fieldName, value string, raw bool) {
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
		node := structuredNode{Kind: structuredString, Text: value}
		if raw {
			parsed, err := parseStructuredNode(value)
			if err != nil {
				parsed = structuredNode{Kind: structuredRaw, Text: value}
			}
			node = parsed
		}
		entry.StructuredOverride = &node
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
	stream := Stream{Kind: store.streams[ref].Kind, canonicalPolicy: policy}
	for _, projected := range projection.Streams {
		if projected.Kind == stream.Kind {
			stream.Fields = append([]Field(nil), projected.Fields...)
			break
		}
	}
	stream.canonicalSeed = make([]fieldEntry, 0, len(store.streams[ref].Fields))
	for _, entry := range store.streams[ref].Fields {
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Generated && (key == "@type" || key == "@typeorder" || key == "StreamOrder") {
			continue
		}
		stream.canonicalSeed = append(stream.canonicalSeed, entry)
	}
	stream = publishCanonicalStreamCompatibilitySnapshot(stream, store)
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
