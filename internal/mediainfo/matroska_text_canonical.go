package mediainfo

import (
	"strconv"
	"strings"
)

// matroskaTextCanonicalSeed builds one subtitle stream directly from
// TrackEntry facts without recovering structured values from display fields.
func matroskaTextCanonicalSeed(
	format, codecID string,
	trackNumber, trackUID uint64,
	contentCompAlgo uint64,
	hasContentCompression bool,
	trackName, languageCode, displayLanguage string,
	defaultValue, forcedValue bool,
	serviceKinds []string,
) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamText)
	builder.Fill("Format", format, "Format", format)
	if trackNumber > 0 {
		value := strconv.FormatUint(trackNumber, 10)
		builder.Fill("ID", value, "ID", value)
	}
	if contentCompAlgo == 3 {
		builder.Fill("MuxingMode", "Header stripping", "Muxing mode", "Header stripping")
	} else if hasContentCompression && contentCompAlgo == 0 {
		builder.Fill("MuxingMode", "zlib", "Muxing mode", "zlib")
	}
	if codecID != "" {
		builder.Fill("CodecID", codecID, "Codec ID", codecID)
	}
	switch codecID {
	case "S_TEXT/UTF8":
		builder.Text("Codec ID/Info", "UTF-8 Plain Text")
	case "S_TEXT/ASS":
		builder.Text("Codec ID/Info", "Advanced Sub Station Alpha")
	case "S_TEXT/SSA":
		builder.Text("Codec ID/Info", "Sub Station Alpha")
	}
	if trackUID > 0 {
		builder.Structured("UniqueID", strconv.FormatUint(trackUID, 10))
	}
	if codecID == "S_TEXT/ASS" || codecID == "S_TEXT/SSA" {
		builder.Fill("Compression_Mode", "Lossless", "Compression mode", "Lossless")
	}
	if trackName != "" {
		builder.Fill("Title", trackName, "Title", trackName)
	}
	if languageCode != "" {
		builder.Fill("Language", languageCode, "Language", formatLanguage(displayLanguage))
	}
	if len(serviceKinds) > 0 {
		builder.Structured("ServiceKind", strings.Join(serviceKinds, " / "))
	}
	defaultText := "No"
	if defaultValue {
		defaultText = "Yes"
	}
	builder.Fill("Default", defaultText, "Default", defaultText)
	forcedText := "No"
	if forcedValue {
		forcedText = "Yes"
	}
	builder.Fill("Forced", forcedText, "Forced", forcedText)
	return builder.Snapshot(canonicalStreamPolicy{}).canonicalSeed
}

// mergeMatroskaDynamicCanonicalExtras appends normalized tag members to a
// source-built extra object using the public compatibility collision policy.
func mergeMatroskaDynamicCanonicalExtras(stream *Stream, members []structuredMember) {
	if stream == nil || len(members) == 0 {
		return
	}
	node := canonicalSeedStructuredNode(stream, "extra")
	if node == nil {
		appendCanonicalSeedObjectMembers(stream, "extra", members)
		return
	}
	if node.Kind != structuredObject {
		return
	}
	positions := make(map[string]int, len(node.Object))
	for index, member := range node.Object {
		if _, exists := positions[member.Key]; !exists {
			positions[member.Key] = index
		}
	}
	for _, member := range members {
		if member.Key == "" || member.Value.Kind != structuredString || member.Value.Text == "" {
			continue
		}
		if position, exists := positions[member.Key]; exists {
			current := &node.Object[position].Value
			if current.Kind == structuredString && current.Text != member.Value.Text {
				current.Text += " / " + member.Value.Text
			}
			continue
		}
		positions[member.Key] = len(node.Object)
		node.Object = append(node.Object, member)
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		if entry.Node == node {
			entry.Value.Text = structuredNodeText(*node)
			entry.Projected = false
			return
		}
	}
}
