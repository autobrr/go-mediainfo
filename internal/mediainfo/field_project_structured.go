package mediainfo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// structuredNodeKind identifies a JSON/XML node representation.
type structuredNodeKind uint8

// Structured node kinds preserve scalar type, object-member order, and array order.
const (
	structuredString structuredNodeKind = iota
	structuredNumber
	structuredBool
	structuredNull
	structuredObject
	structuredArray
	structuredRaw
)

// structuredNode is an ordered structured value shared by JSON and XML projections.
type structuredNode struct {
	Kind   structuredNodeKind
	Text   string
	Object []structuredMember
	Array  []structuredNode
}

// structuredMember is one ordered object member.
type structuredMember struct {
	Key   string
	Value structuredNode
}

// structuredField pairs a canonical output key with its projected node.
type structuredField struct {
	Key   string
	Value structuredNode
}

// structuredStream is one projected stream and its type-local order.
type structuredStream struct {
	Kind      StreamKind
	TypeOrder int
	Fields    []structuredField
}

// structuredReport contains the ordered streams projected from a report store.
type structuredReport struct {
	Ref     string
	Streams []structuredStream
}

// structuredProjectionTarget selects the visibility policy used by a structured renderer.
type structuredProjectionTarget uint8

// Structured projection targets distinguish JSON from XML compatibility visibility.
const (
	structuredProjectionJSON structuredProjectionTarget = iota
	structuredProjectionXML
)

// projectStructuredReport returns the JSON-visible structured projection for report.
func projectStructuredReport(report Report) structuredReport {
	return projectStructuredReportFor(report, structuredProjectionJSON)
}

// projectStructuredReportFor projects report with target's visibility and compatibility policy.
func projectStructuredReportFor(report Report, target structuredProjectionTarget) structuredReport {
	store := canonicalStoreForReport(report)
	if store == nil {
		return structuredReport{Ref: report.Ref}
	}
	if target == structuredProjectionXML {
		ensureLegacyXMLProjection(store)
	}
	store.projectionMu.RLock()
	defer store.projectionMu.RUnlock()
	projected := structuredReport{Ref: store.ref, Streams: make([]structuredStream, 0, len(store.streams))}
	streamIndexes := make([]int, len(store.streams))
	for index := range store.streams {
		streamIndexes[index] = index
	}
	sort.SliceStable(streamIndexes, func(left, right int) bool {
		return store.streams[streamIndexes[left]].StructuredSequence < store.streams[streamIndexes[right]].StructuredSequence
	})
	for _, streamIndex := range streamIndexes {
		stream := &store.streams[streamIndex]
		fields := make([]fieldEntry, 0, len(stream.Fields))
		for _, entry := range stream.Fields {
			if target == structuredProjectionJSON && entry.Options.ShowStructured || target == structuredProjectionXML && entry.Options.ShowXML {
				fields = append(fields, entry)
			}
		}
		if !stream.StructuredAlreadyOrdered {
			sort.SliceStable(fields, func(left, right int) bool {
				leftSpec, _ := structuredFieldSpec(stream.Kind, firstNonEmpty(fields[left].StructuredKey, string(fields[left].Name)))
				rightSpec, _ := structuredFieldSpec(stream.Kind, firstNonEmpty(fields[right].StructuredKey, string(fields[right].Name)))
				if leftSpec.Order != rightSpec.Order {
					return leftSpec.Order < rightSpec.Order
				}
				return fields[left].Sequence < fields[right].Sequence
			})
		}
		projectedStream := structuredStream{Kind: stream.Kind, TypeOrder: stream.TypeOrder, Fields: make([]structuredField, 0, len(fields))}
		for _, entry := range fields {
			key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
			value := structuredNode{Kind: structuredString, Text: entry.Value.Text}
			if entry.Node != nil {
				value = *entry.Node
			} else if !entry.Projected {
				value.Text = projectCanonicalStructuredValue(stream.Kind, entry)
			}
			projectedStream.Fields = append(projectedStream.Fields, structuredField{Key: key, Value: value})
		}
		projected.Streams = append(projected.Streams, projectedStream)
	}
	return projected
}

// projectCanonicalStructuredValue converts canonical base-unit values to structured schema text.
func projectCanonicalStructuredValue(kind StreamKind, entry fieldEntry) string {
	spec, known := lookupFieldSpec(kind, entry.Name)
	if !known {
		return entry.Value.Text
	}
	if spec.Measure == fieldMeasureMilliseconds {
		milliseconds, err := strconv.ParseFloat(entry.Value.Text, 64)
		if err == nil {
			return formatJSONSeconds(milliseconds / 1000)
		}
	}
	switch entry.Name {
	case "BitRate_Mode", "OverallBitRate_Mode":
		return mapBitrateMode(entry.Value.Text)
	case "FrameRate_Mode":
		return mapFrameRateMode(entry.Value.Text)
	}
	return entry.Value.Text
}

// parseStructuredNode decodes exactly one JSON value while preserving object member order.
func parseStructuredNode(data string) (structuredNode, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(data))
	decoder.UseNumber()
	node, err := readStructuredNode(decoder)
	if err != nil {
		return structuredNode{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return structuredNode{}, errors.New("multiple JSON values")
		}
		return structuredNode{}, err
	}
	return node, nil
}

// readStructuredNode recursively reads the next ordered node from decoder.
func readStructuredNode(decoder *json.Decoder) (structuredNode, error) {
	token, err := decoder.Token()
	if err != nil {
		return structuredNode{}, err
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			var members []structuredMember
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return structuredNode{}, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return structuredNode{}, fmt.Errorf("object key has type %T", keyToken)
				}
				value, err := readStructuredNode(decoder)
				if err != nil {
					return structuredNode{}, err
				}
				members = append(members, structuredMember{Key: key, Value: value})
			}
			if _, err := decoder.Token(); err != nil {
				return structuredNode{}, err
			}
			return structuredNode{Kind: structuredObject, Object: members}, nil
		case '[':
			var values []structuredNode
			for decoder.More() {
				value, err := readStructuredNode(decoder)
				if err != nil {
					return structuredNode{}, err
				}
				values = append(values, value)
			}
			if _, err := decoder.Token(); err != nil {
				return structuredNode{}, err
			}
			return structuredNode{Kind: structuredArray, Array: values}, nil
		default:
			return structuredNode{}, fmt.Errorf("unexpected delimiter %q", delimiter)
		}
	}
	switch value := token.(type) {
	case string:
		return structuredNode{Kind: structuredString, Text: value}, nil
	case json.Number:
		return structuredNode{Kind: structuredNumber, Text: value.String()}, nil
	case bool:
		return structuredNode{Kind: structuredBool, Text: strconv.FormatBool(value)}, nil
	case nil:
		return structuredNode{Kind: structuredNull}, nil
	default:
		return structuredNode{}, fmt.Errorf("unexpected scalar type %T", token)
	}
}

// renderStructuredNode serializes node as compact JSON.
func renderStructuredNode(node structuredNode) string {
	var buffer bytes.Buffer
	writeStructuredNode(&buffer, node)
	return buffer.String()
}

// writeStructuredNode appends node's compact JSON representation to buffer.
func writeStructuredNode(buffer *bytes.Buffer, node structuredNode) {
	switch node.Kind {
	case structuredString:
		buffer.WriteString(renderJSONString(node.Text))
	case structuredNumber, structuredBool, structuredRaw:
		buffer.WriteString(node.Text)
	case structuredNull:
		buffer.WriteString("null")
	case structuredObject:
		buffer.WriteByte('{')
		for index, member := range node.Object {
			if index > 0 {
				buffer.WriteByte(',')
			}
			buffer.WriteString(renderJSONString(member.Key))
			buffer.WriteByte(':')
			writeStructuredNode(buffer, member.Value)
		}
		buffer.WriteByte('}')
	case structuredArray:
		buffer.WriteByte('[')
		for index, item := range node.Array {
			if index > 0 {
				buffer.WriteByte(',')
			}
			writeStructuredNode(buffer, item)
		}
		buffer.WriteByte(']')
	}
}

// structuredNodeText returns a scalar's text or compact JSON for compound nodes.
func structuredNodeText(node structuredNode) string {
	switch node.Kind {
	case structuredNull:
		return ""
	case structuredString, structuredNumber, structuredBool, structuredRaw:
		return node.Text
	case structuredObject, structuredArray:
		return renderStructuredNode(node)
	}
	return ""
}

// structuredFieldsToJSON adapts structured fields to the legacy JSON renderer shape.
func structuredFieldsToJSON(fields []structuredField) []jsonKV {
	result := make([]jsonKV, 0, len(fields))
	for _, field := range fields {
		if field.Value.Kind == structuredString {
			result = append(result, jsonKV{Key: field.Key, Val: field.Value.Text})
			continue
		}
		result = append(result, jsonKV{Key: field.Key, Val: renderStructuredNode(field.Value), Raw: true})
	}
	return result
}
