package mediainfo

import "slices"

// canonicalStreamBuilder collects one parser stream in canonical form while
// retaining the public compatibility snapshot required by Stream callers.
type canonicalStreamBuilder struct {
	store *fieldStore
	ref   streamRef
}

// newCanonicalStreamBuilder prepares one canonical stream of kind.
func newCanonicalStreamBuilder(kind StreamKind) *canonicalStreamBuilder {
	store := &fieldStore{}
	return &canonicalStreamBuilder{store: store, ref: store.Prepare(kind)}
}

// Fill records raw as the structured value and display as its text projection.
// Label is the legacy text label and may differ from the structured field name.
func (b *canonicalStreamBuilder) Fill(name fieldName, raw, label, display string) {
	if b == nil || b.store == nil || raw == "" {
		return
	}
	stream := b.store.stream(b.ref)
	if stream == nil {
		return
	}
	foundStructured := false
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		if entry.Name != name || !entry.Options.ShowStructured {
			continue
		}
		entry.Value.Text = raw
		entry.Options.ShowText = false
		entry.Options.ShowStructured = true
		entry.Options.ShowXML = true
		entry.TextLabel = label
		entry.StructuredKey = string(name)
		entry.Projected = false
		entry.DeriveString = false
		foundStructured = true
		break
	}
	if !foundStructured {
		b.store.Fill(b.ref, name, raw, fillReplace)
		for index := range stream.Fields {
			entry := &stream.Fields[index]
			if entry.Name != name || !entry.Options.ShowStructured {
				continue
			}
			entry.Options.ShowText = false
			entry.Options.ShowXML = true
			entry.TextLabel = label
			entry.StructuredKey = string(name)
			entry.DeriveString = false
			break
		}
	}
	if display != "" {
		for index := range stream.Fields {
			entry := &stream.Fields[index]
			if !entry.Options.ShowText || entry.TextLabel != label {
				continue
			}
			entry.Name = name + "/String"
			entry.Value.Text = display
			entry.Dynamic = false
			entry.Projected = false
			return
		}
		b.store.appendEntry(stream, fieldEntry{
			Name:      name + "/String",
			Value:     fieldValue{Text: display},
			Options:   fieldOptions{ShowText: true, ValueType: fieldValueString},
			TextLabel: label,
		})
	}
}

// ImportCanonicalSeed copies a nested parser's canonical entries into this
// stream while discarding nested report-order metadata.
func (b *canonicalStreamBuilder) ImportCanonicalSeed(seed []fieldEntry) {
	if b == nil || b.store == nil || len(seed) == 0 {
		return
	}
	appendCanonicalSeed(b.store, b.ref, seed)
}

// DirectStructured records a parser-owned structured scalar and converts a
// matching staged entry from projected compatibility data to canonical data.
func (b *canonicalStreamBuilder) DirectStructured(name fieldName, value string) {
	if b == nil || b.store == nil || value == "" {
		return
	}
	stream := b.store.stream(b.ref)
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if !entry.Options.ShowStructured || key != string(name) {
			continue
		}
		entry.Name = name
		entry.Value.Text = value
		entry.Node = nil
		entry.Options.ShowXML = true
		entry.StructuredKey = string(name)
		entry.Projected = false
		return
	}
	spec, known := structuredFieldSpec(stream.Kind, string(name))
	b.store.appendEntry(stream, fieldEntry{
		Name:          name,
		Value:         fieldValue{Text: value},
		Dynamic:       !known,
		Options:       fieldOptions{ShowStructured: true, ShowXML: true, ValueType: spec.Options.ValueType},
		StructuredKey: string(name),
	})
}

// HasStructured reports whether the builder already contains a structured key.
func (b *canonicalStreamBuilder) HasStructured(name fieldName) bool {
	if b == nil || b.store == nil {
		return false
	}
	stream := b.store.stream(b.ref)
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			return true
		}
	}
	return false
}

// ReplaceText changes an existing display field without deriving a structured
// value from the formatted text. It appends the field when absent.
func (b *canonicalStreamBuilder) ReplaceText(label, value string) {
	if b == nil || b.store == nil || value == "" {
		return
	}
	stream := b.store.stream(b.ref)
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		if entry.Options.ShowText && entry.TextLabel == label {
			entry.Value.Text = value
			return
		}
	}
	b.Text(label, value)
}

// Text records a display-only field that has no structured representation.
func (b *canonicalStreamBuilder) Text(label, value string) {
	if b == nil || b.store == nil || value == "" {
		return
	}
	b.store.appendEntry(b.store.stream(b.ref), fieldEntry{
		Name:      fieldName("Text/" + label),
		Value:     fieldValue{Text: value},
		Dynamic:   true,
		Options:   fieldOptions{ShowText: true, ValueType: fieldValueString},
		TextLabel: label,
	})
}

// Structured records a structured-only scalar field.
func (b *canonicalStreamBuilder) Structured(name fieldName, value string) {
	if b == nil || b.store == nil || value == "" {
		return
	}
	fillGeneratedStructured(b.store, b.ref, name, value)
}

// StructuredJSONOnly records a parser-owned scalar that intentionally remains
// absent from XML compatibility output.
func (b *canonicalStreamBuilder) StructuredJSONOnly(name fieldName, value string) {
	if b == nil || b.store == nil || value == "" {
		return
	}
	stream := b.store.stream(b.ref)
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			entry.Name = name
			entry.Value.Text = value
			entry.Node = nil
			entry.Options.ShowXML = false
			entry.StructuredKey = string(name)
			entry.Projected = false
			return
		}
	}
	b.Structured(name, value)
	stream = b.store.stream(b.ref)
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			entry.Options.ShowXML = false
			return
		}
	}
}

// OverrideStructured applies an exact serializer override when it differs from
// the canonical field's ordinary structured projection.
func (b *canonicalStreamBuilder) OverrideStructured(name fieldName, value string) {
	if b == nil || b.store == nil || value == "" {
		return
	}
	stream := b.store.stream(b.ref)
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if key != string(name) || !entry.Options.ShowStructured {
			continue
		}
		if entry.Node == nil && projectCanonicalStructuredValue(stream.Kind, *entry) == value {
			return
		}
		node := structuredNode{Kind: structuredString, Text: value}
		entry.StructuredOverride = &node
		return
	}
	fillGeneratedStructured(b.store, b.ref, name, value)
}

// SetStructuredDecimals selects fixed structured precision for a direct
// measured decimal while retaining its canonical base-unit value.
func (b *canonicalStreamBuilder) SetStructuredDecimals(name fieldName, decimals uint8) {
	if b == nil || b.store == nil || decimals == 0 {
		return
	}
	stream := b.store.stream(b.ref)
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			entry.StructuredDecimals = decimals
			return
		}
	}
}

// StructuredNode records an ordered structured-only object or array field.
func (b *canonicalStreamBuilder) StructuredNode(name fieldName, node structuredNode) {
	if b == nil || b.store == nil {
		return
	}
	b.store.appendEntry(b.store.stream(b.ref), fieldEntry{
		Name:          name,
		Value:         fieldValue{Text: structuredNodeText(node)},
		Dynamic:       true,
		Options:       fieldOptions{ShowStructured: true, ShowXML: true, ValueType: fieldValueNode},
		StructuredKey: string(name),
		Node:          &node,
	})
}

// OverrideStructuredNode replaces every prior value for a structured key with
// one ordered object or array value.
func (b *canonicalStreamBuilder) OverrideStructuredNode(name fieldName, node structuredNode) {
	if b == nil || b.store == nil {
		return
	}
	stream := b.store.stream(b.ref)
	writeIndex := 0
	replaced := false
	for index := range stream.Fields {
		entry := stream.Fields[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			if replaced {
				continue
			}
			entry.Name = name
			entry.Value.Text = structuredNodeText(node)
			entry.Options.ShowStructured = true
			entry.Options.ShowXML = true
			entry.Options.ValueType = fieldValueNode
			entry.StructuredKey = string(name)
			entry.Node = &node
			entry.Projected = false
			replaced = true
		}
		stream.Fields[writeIndex] = entry
		writeIndex++
	}
	stream.Fields = stream.Fields[:writeIndex]
	if !replaced {
		b.StructuredNode(name, node)
	}
}

// Snapshot finalizes the stream and returns its compatibility representation.
func (b *canonicalStreamBuilder) Snapshot(policy canonicalStreamPolicy) Stream {
	if b == nil {
		return Stream{}
	}
	return canonicalStreamSnapshot(b.store, b.ref, policy)
}

// overrideCanonicalSeedStructured mirrors a post-parse compatibility override
// into a direct stream seed without changing its text projection.
func overrideCanonicalSeedStructured(stream *Stream, name fieldName, value string) {
	if stream == nil || len(stream.canonicalSeed) == 0 || value == "" {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if key != string(name) || !entry.Options.ShowStructured {
			continue
		}
		entry.Value.Text = value
		entry.Node = nil
		entry.Projected = true
		return
	}
	spec, known := structuredFieldSpec(stream.Kind, string(name))
	sequence := uint32(len(stream.canonicalSeed))
	stream.canonicalSeed = append(stream.canonicalSeed, fieldEntry{
		Name:          spec.Name,
		Value:         fieldValue{Text: value},
		Dynamic:       !known,
		Options:       fieldOptions{ShowStructured: true, ShowXML: true, ValueType: spec.Options.ValueType},
		Sequence:      sequence,
		StructuredKey: string(name),
		Projected:     true,
	})
}

// canonicalSeedValue returns the first direct canonical scalar for key.
func canonicalSeedValue(stream Stream, key fieldName) (string, bool) {
	for _, entry := range stream.canonicalSeed {
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && structuredKey == string(key) && entry.Node == nil {
			return entry.Value.Text, true
		}
	}
	return "", false
}

// canonicalSeedHasStructured reports whether key already has a scalar or node.
func canonicalSeedHasStructured(stream Stream, key fieldName) bool {
	for _, entry := range stream.canonicalSeed {
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && structuredKey == string(key) {
			return true
		}
	}
	return false
}

// canonicalSeedTextValue returns the first direct canonical display value for
// a legacy text label.
func canonicalSeedTextValue(stream Stream, label string) (string, bool) {
	for _, entry := range stream.canonicalSeed {
		if entry.Options.ShowText && entry.TextLabel == label {
			return entry.Value.Text, true
		}
	}
	return "", false
}

// projectedCanonicalSeedValue returns a direct scalar after applying the same
// measure and normalization transform used by structured renderers.
func projectedCanonicalSeedValue(stream Stream, key fieldName) (string, bool) {
	for _, entry := range stream.canonicalSeed {
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && structuredKey == string(key) && entry.Node == nil {
			return projectCanonicalStructuredValue(stream.Kind, entry), true
		}
	}
	return "", false
}

// canonicalSeedJSONOnlyValue returns a direct scalar only when its structured
// projection is intentionally hidden from XML compatibility output.
func canonicalSeedJSONOnlyValue(stream Stream, key fieldName) (string, bool) {
	for _, entry := range stream.canonicalSeed {
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && !entry.Options.ShowXML && structuredKey == string(key) {
			return projectCanonicalStructuredValue(stream.Kind, entry), true
		}
	}
	return "", false
}

// canonicalSeedStructuredNode returns the direct compound node for key.
func canonicalSeedStructuredNode(stream *Stream, key fieldName) *structuredNode {
	if stream == nil {
		return nil
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && structuredKey == string(key) {
			return entry.Node
		}
	}
	return nil
}

// appendCanonicalSeedObjectMembers extends one direct structured object while
// preserving the source member order and duplicate-key behavior.
func appendCanonicalSeedObjectMembers(stream *Stream, key fieldName, members []structuredMember) {
	if stream == nil || len(members) == 0 {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if !entry.Options.ShowStructured || structuredKey != string(key) || entry.Node == nil || entry.Node.Kind != structuredObject {
			continue
		}
		entry.Node.Object = append(entry.Node.Object, members...)
		entry.Value.Text = structuredNodeText(*entry.Node)
		entry.Projected = false
		return
	}
	sequence := uint32(len(stream.canonicalSeed))
	node := structuredNode{Kind: structuredObject, Object: append([]structuredMember(nil), members...)}
	stream.canonicalSeed = append(stream.canonicalSeed, fieldEntry{
		Name:          key,
		Value:         fieldValue{Text: structuredNodeText(node)},
		Dynamic:       true,
		Options:       fieldOptions{ShowStructured: true, ShowXML: true, ValueType: fieldValueNode},
		Sequence:      sequence,
		StructuredKey: string(key),
		Node:          &node,
	})
}

// replaceCanonicalSeedObjectValues mirrors compatibility corrections into
// existing direct object members without disturbing their source order.
func replaceCanonicalSeedObjectValues(stream *Stream, key fieldName, values map[string]string) {
	if stream == nil || len(values) == 0 || len(stream.canonicalSeed) == 0 {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if !entry.Options.ShowStructured || structuredKey != string(key) || entry.Node == nil || entry.Node.Kind != structuredObject {
			continue
		}
		for memberIndex := range entry.Node.Object {
			member := &entry.Node.Object[memberIndex]
			if value, ok := values[member.Key]; ok {
				member.Value = structuredNode{Kind: structuredString, Text: value}
			}
		}
		entry.Value.Text = structuredNodeText(*entry.Node)
		entry.Projected = false
		return
	}
}

// prependCanonicalSeedObjectMembers places source members before an existing
// direct structured object while preserving member and duplicate-key order.
func prependCanonicalSeedObjectMembers(stream *Stream, key fieldName, members []structuredMember) {
	if stream == nil || len(members) == 0 || len(stream.canonicalSeed) == 0 {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if !entry.Options.ShowStructured || structuredKey != string(key) || entry.Node == nil || entry.Node.Kind != structuredObject {
			continue
		}
		object := make([]structuredMember, 0, len(members)+len(entry.Node.Object))
		object = append(object, members...)
		object = append(object, entry.Node.Object...)
		entry.Node.Object = object
		entry.Value.Text = structuredNodeText(*entry.Node)
		entry.Projected = false
		return
	}
	appendCanonicalSeedObjectMembers(stream, key, members)
}

// insertCanonicalSeedObjectMembersBefore inserts source members immediately
// before the first matching object member, or appends them when it is absent.
func insertCanonicalSeedObjectMembersBefore(stream *Stream, key fieldName, before string, members []structuredMember) {
	if stream == nil || len(members) == 0 || len(stream.canonicalSeed) == 0 {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if !entry.Options.ShowStructured || structuredKey != string(key) || entry.Node == nil || entry.Node.Kind != structuredObject {
			continue
		}
		position := len(entry.Node.Object)
		for memberIndex, member := range entry.Node.Object {
			if member.Key == before {
				position = memberIndex
				break
			}
		}
		object := make([]structuredMember, 0, len(entry.Node.Object)+len(members))
		object = append(object, entry.Node.Object[:position]...)
		object = append(object, members...)
		object = append(object, entry.Node.Object[position:]...)
		entry.Node.Object = object
		entry.Value.Text = structuredNodeText(*entry.Node)
		entry.Projected = false
		return
	}
	appendCanonicalSeedObjectMembers(stream, key, members)
}

// replaceCanonicalSeedFill mirrors a post-parse media fact into both projections.
func replaceCanonicalSeedFill(stream *Stream, name fieldName, raw, label, display string) {
	if stream == nil || raw == "" {
		return
	}
	foundStructured := false
	foundText := display == ""
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			entry.Name = name
			entry.Value.Text = raw
			entry.Node = nil
			entry.StructuredOverride = nil
			entry.Projected = false
			if display != "" {
				entry.Options.ShowXML = true
			}
			foundStructured = true
		}
		if display != "" && entry.Options.ShowText && entry.TextLabel == label {
			entry.Value.Text = display
			entry.Projected = false
			foundText = true
		}
	}
	sequence := uint32(len(stream.canonicalSeed))
	if !foundStructured {
		spec, known := structuredFieldSpec(stream.Kind, string(name))
		stream.canonicalSeed = append(stream.canonicalSeed, fieldEntry{
			Name:          spec.Name,
			Value:         fieldValue{Text: raw},
			Dynamic:       !known,
			Options:       fieldOptions{ShowStructured: true, ShowXML: true, ValueType: spec.Options.ValueType},
			Sequence:      sequence,
			StructuredKey: string(name),
		})
		sequence++
	}
	if !foundText {
		textName, _, known := textFieldName(stream.Kind, label)
		stream.canonicalSeed = append(stream.canonicalSeed, fieldEntry{
			Name:      textName,
			Value:     fieldValue{Text: display},
			Dynamic:   !known,
			Options:   fieldOptions{ShowText: true, ValueType: fieldValueString},
			Sequence:  sequence,
			TextLabel: label,
		})
	}
}

// replaceCanonicalSeedProjection updates a parser-owned value while retaining
// the serializer precision represented by projected.
func replaceCanonicalSeedProjection(stream *Stream, name fieldName, raw, projected, label, display string) {
	replaceCanonicalSeedFill(stream, name, raw, label, display)
	if decimals := decimalFractionDigits(projected); decimals > 0 {
		setCanonicalSeedStructuredDecimals(stream, name, uint8(decimals))
	}
}

// replaceCanonicalSeedText mirrors a source display fact without deriving a
// structured value from its formatted representation.
func replaceCanonicalSeedText(stream *Stream, label, value string) {
	if stream == nil || len(stream.canonicalSeed) == 0 || value == "" {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		if entry.Options.ShowText && entry.TextLabel == label {
			entry.Value.Text = value
			entry.Projected = false
			return
		}
	}
	name, spec, known := textFieldName(stream.Kind, label)
	stream.canonicalSeed = append(stream.canonicalSeed, fieldEntry{
		Name:      name,
		Value:     fieldValue{Text: value},
		Dynamic:   !known,
		Options:   fieldOptions{ShowText: true, ValueType: spec.Options.ValueType},
		Sequence:  uint32(len(stream.canonicalSeed)),
		TextLabel: label,
	})
}

// setCanonicalSeedStructuredDecimals retains fixed serializer precision for a
// direct measured decimal after a post-parse source update.
func setCanonicalSeedStructuredDecimals(stream *Stream, name fieldName, decimals uint8) {
	if stream == nil || decimals == 0 {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			entry.StructuredDecimals = decimals
			return
		}
	}
}

// setCanonicalSeedXMLVisibility changes XML visibility for one direct
// structured field without altering its JSON projection or canonical value.
func setCanonicalSeedXMLVisibility(stream *Stream, name fieldName, visible bool) {
	if stream == nil || len(stream.canonicalSeed) == 0 {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			entry.Options.ShowXML = visible
		}
	}
}

// replaceCanonicalSeedJSONOnly records a direct structured scalar that remains
// intentionally absent from XML and text compatibility projections.
func replaceCanonicalSeedJSONOnly(stream *Stream, name fieldName, value string) {
	if stream == nil || value == "" {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			entry.Name = name
			entry.Value.Text = value
			entry.Node = nil
			entry.Options.ShowXML = false
			entry.Projected = false
			return
		}
	}
	spec, known := structuredFieldSpec(stream.Kind, string(name))
	stream.canonicalSeed = append(stream.canonicalSeed, fieldEntry{
		Name:          spec.Name,
		Value:         fieldValue{Text: value},
		Dynamic:       !known,
		Options:       fieldOptions{ShowStructured: true, ShowXML: false, ValueType: spec.Options.ValueType},
		Sequence:      uint32(len(stream.canonicalSeed)),
		StructuredKey: string(name),
	})
}

// insertCanonicalSeedTextBefore places a direct display-only fact before the
// earliest matching display label while preserving all structured-field order.
func insertCanonicalSeedTextBefore(stream *Stream, label, value string, beforeLabels ...string) {
	if stream == nil || len(stream.canonicalSeed) == 0 || label == "" || value == "" {
		return
	}
	name, spec, known := textFieldName(stream.Kind, label)
	entry := fieldEntry{
		Name: name, Value: fieldValue{Text: value}, Dynamic: !known,
		Options: spec.Options, TextLabel: label,
	}
	for index := range stream.canonicalSeed {
		candidate := stream.canonicalSeed[index]
		if !candidate.Options.ShowText || candidate.TextLabel != label {
			continue
		}
		entry = candidate
		entry.Value.Text = value
		entry.Projected = false
		stream.canonicalSeed = append(stream.canonicalSeed[:index], stream.canonicalSeed[index+1:]...)
		break
	}
	position := len(stream.canonicalSeed)
	for index, candidate := range stream.canonicalSeed {
		if !candidate.Options.ShowText {
			continue
		}
		if slices.Contains(beforeLabels, candidate.TextLabel) {
			position = index
		}
		if position != len(stream.canonicalSeed) {
			break
		}
	}
	stream.canonicalSeed = append(stream.canonicalSeed, fieldEntry{})
	copy(stream.canonicalSeed[position+1:], stream.canonicalSeed[position:])
	entry.Sequence = uint32(position)
	stream.canonicalSeed[position] = entry
	for index := position + 1; index < len(stream.canonicalSeed); index++ {
		stream.canonicalSeed[index].Sequence = uint32(index)
	}
}

// clearCanonicalSeedField removes a media fact from both canonical projections.
func clearCanonicalSeedField(stream *Stream, name fieldName, label string) {
	if stream == nil || len(stream.canonicalSeed) == 0 {
		return
	}
	writeIndex := 0
	for _, entry := range stream.canonicalSeed {
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			continue
		}
		if label != "" && entry.Options.ShowText && entry.TextLabel == label {
			continue
		}
		stream.canonicalSeed[writeIndex] = entry
		writeIndex++
	}
	stream.canonicalSeed = stream.canonicalSeed[:writeIndex]
}

// clearCanonicalSeedText removes one compatibility display field while
// retaining any structured value derived from the same source fact.
func clearCanonicalSeedText(stream *Stream, label string) {
	if stream == nil || len(stream.canonicalSeed) == 0 || label == "" {
		return
	}
	writeIndex := 0
	for _, entry := range stream.canonicalSeed {
		if entry.Options.ShowText && entry.TextLabel == label {
			continue
		}
		stream.canonicalSeed[writeIndex] = entry
		writeIndex++
	}
	stream.canonicalSeed = stream.canonicalSeed[:writeIndex]
}
