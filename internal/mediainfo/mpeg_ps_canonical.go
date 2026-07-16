package mediainfo

import (
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// mpegPSStructuredFact retains one canonical PS fact and its exact legacy
// structured value until the public compatibility snapshot is projected.
type mpegPSStructuredFact struct {
	name      fieldName
	canonical string
	legacy    string
}

// mpegPSStructuredFacts stages parser facts without exposing JSON-shaped state
// to the MPEG-PS parser.
type mpegPSStructuredFacts struct {
	values []mpegPSStructuredFact
}

// Set replaces one structured fact while preserving its canonical base-unit
// value and exact legacy projection value.
func (f *mpegPSStructuredFacts) Set(name fieldName, legacy string) {
	if f == nil || name == "" {
		return
	}
	fact := mpegPSStructuredFact{
		name:      name,
		canonical: canonicalMPEGPSStructuredValue(name, legacy),
		legacy:    legacy,
	}
	for index := range f.values {
		if f.values[index].name == name {
			f.values[index] = fact
			return
		}
	}
	f.values = append(f.values, fact)
}

// Legacy returns the exact compatibility value staged for name.
func (f *mpegPSStructuredFacts) Legacy(name fieldName) string {
	if f == nil {
		return ""
	}
	for index := range slices.Backward(f.values) {
		if f.values[index].name == name {
			return f.values[index].legacy
		}
	}
	return ""
}

// fillCanonicalMPEGPSH264 records AVC configuration facts retained by the PS demuxer.
func fillCanonicalMPEGPSH264(builder *canonicalStreamBuilder, stream *psStream) {
	if builder == nil || stream == nil || !stream.videoIsH264 {
		return
	}
	profile := stream.videoAVC.profile
	if profile == "High" && stream.videoSPS.ConstraintFlags&0x08 != 0 {
		profile = "Progressive High"
	}
	if profile != "" {
		builder.Fill("Format_Profile", profile, "Format profile", findField(stream.videoFields, "Format profile"))
	}
	if level := strings.TrimPrefix(stream.videoAVC.level, "L"); level != "" {
		builder.DirectStructured("Format_Level", level)
	}
	if stream.videoAVC.cabac != nil {
		value := formatYesNo(*stream.videoAVC.cabac)
		builder.Fill("Format_Settings_CABAC", value, "Format settings, CABAC", value)
	}
	if stream.videoSPS.RefFrames > 0 {
		display := findField(stream.videoFields, "Format settings, Reference frames")
		builder.Fill("Format_Settings_RefFrames", strconv.Itoa(stream.videoSPS.RefFrames), "Format settings, Reference frames", display)
	}
	if stream.videoSPS.ChromaFormat != "" {
		builder.Fill("ColorSpace", "YUV", "Color space", "YUV")
		builder.Fill("ChromaSubsampling", stream.videoSPS.ChromaFormat, "Chroma subsampling", stream.videoSPS.ChromaFormat)
	}
	if stream.videoSPS.HasChromaLoc {
		value := "Type " + strconv.Itoa(stream.videoSPS.ChromaSampleLoc)
		builder.Fill("ChromaSubsampling_Position", value, "Chroma subsampling position", value)
	}
	if stream.videoSPS.BitDepth > 0 {
		builder.Fill("BitDepth", strconv.Itoa(stream.videoSPS.BitDepth), "Bit depth", formatBitDepth(uint8(stream.videoSPS.BitDepth)))
	}
	if stream.videoSPS.HasScanType {
		builder.Fill("ScanType", h264ScanType(stream.videoSPS), "Scan type", findField(stream.videoFields, "Scan type"))
	}
	if stream.videoSPS.MBAFF {
		builder.Fill("ScanOrder", "TFF", "Scan order", "TFF")
	}
	if stream.videoSPS.HasVideoFmt {
		if standard := mapH264VideoFormat(stream.videoSPS.VideoFormat); standard != "" {
			builder.Fill("Standard", standard, "Standard", standard)
		}
	}
}

// h264ScanType returns the canonical scan type represented by SPS flags.
func h264ScanType(info h264SPSInfo) string {
	switch {
	case info.Progressive:
		return "Progressive"
	case info.MBAFF:
		return "MBAFF"
	default:
		return "Interlaced"
	}
}

// Apply imports staged PS facts into the canonical stream and records their
// exact public compatibility values.
func (f *mpegPSStructuredFacts) Apply(builder *canonicalStreamBuilder, fields []Field) {
	if f == nil || builder == nil {
		return
	}
	values := append([]mpegPSStructuredFact(nil), f.values...)
	sort.SliceStable(values, func(left, right int) bool {
		leftOrder := structuredFieldOrder(builder.store.stream(builder.ref).Kind, string(values[left].name))
		rightOrder := structuredFieldOrder(builder.store.stream(builder.ref).Kind, string(values[right].name))
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return values[left].name < values[right].name
	})
	for _, fact := range values {
		if !builder.HasStructured(fact.name) {
			label := mpegPSStructuredTextLabel(fact.name)
			display := findField(fields, label)
			if label != "" && display != "" {
				builder.Fill(fact.name, fact.canonical, label, display)
			} else {
				builder.DirectStructured(fact.name, fact.canonical)
			}
		}
		builder.MarkLegacyJSON(fact.name, fact.legacy)
	}
}

// buildMPEGPSCanonicalSnapshot finalizes one parser stream and projects its
// legacy public Fields, JSON, and JSONRaw snapshots at the package boundary.
func buildMPEGPSCanonicalSnapshot(builder *canonicalStreamBuilder, fields []Field, facts *mpegPSStructuredFacts, extra *structuredNode, policy canonicalStreamPolicy) Stream {
	facts.Apply(builder, fields)
	appendMissingMPEGPSCanonicalText(builder, fields)
	if extra != nil {
		builder.OverrideStructuredNode("extra", *extra)
		builder.MarkLegacyJSONRaw("extra", structuredNodeText(*extra))
	}
	return builder.Snapshot(policy)
}

// canonicalMPEGPSStructuredValue converts serializer seconds to canonical milliseconds.
func canonicalMPEGPSStructuredValue(name fieldName, value string) string {
	switch name {
	case "Duration", "Source_Duration", "Source_Duration_LastFrame":
		if milliseconds, ok := decimalSecondsToMilliseconds(value); ok {
			return milliseconds
		}
	}
	return value
}

// decimalSecondsToMilliseconds shifts a validated decimal without float conversion.
func decimalSecondsToMilliseconds(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	sign := ""
	if value[0] == '-' || value[0] == '+' {
		if value[0] == '-' {
			sign = "-"
		}
		value = value[1:]
	}
	whole, fraction, found := strings.Cut(value, ".")
	if !found {
		fraction = ""
	}
	if whole == "" && fraction == "" {
		return "", false
	}
	if whole == "" {
		whole = "0"
	}
	if !decimalDigits(whole) || !decimalDigits(fraction) {
		return "", false
	}
	for len(fraction) < 3 {
		fraction += "0"
	}
	millisecondsWhole := strings.TrimLeft(whole+fraction[:3], "0")
	if millisecondsWhole == "" {
		millisecondsWhole = "0"
	}
	millisecondsFraction := strings.TrimRight(fraction[3:], "0")
	if millisecondsWhole == "0" && millisecondsFraction == "" {
		sign = ""
	}
	if millisecondsFraction != "" {
		return sign + millisecondsWhole + "." + millisecondsFraction, true
	}
	return sign + millisecondsWhole, true
}

// decimalDigits reports whether value contains only decimal digits; empty is valid.
func decimalDigits(value string) bool {
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// mpegPSStructuredTextLabel maps structured PS keys to their legacy display labels.
func mpegPSStructuredTextLabel(name fieldName) string {
	switch name {
	case "ID":
		return "ID"
	case "Duration":
		return "Duration"
	case "BitRate":
		return "Bit rate"
	case "BitRate_Nominal":
		return "Nominal bit rate"
	case "BitRate_Maximum":
		return "Maximum bit rate"
	case "BitRate_Mode":
		return "Bit rate mode"
	case "StreamSize":
		return "Stream size"
	case "FrameRate":
		return "Frame rate"
	case "Width":
		return "Width"
	case "Height":
		return "Height"
	case "DisplayAspectRatio":
		return "Display aspect ratio"
	case "Channels":
		return "Channel(s)"
	case "ChannelLayout":
		return "Channel layout"
	case "SamplingRate":
		return "Sampling rate"
	case "Compression_Mode":
		return "Compression mode"
	case "ServiceKind":
		return "Service kind"
	default:
		return ""
	}
}

// appendMissingMPEGPSCanonicalText retains display-only and repeated PS fields.
func appendMissingMPEGPSCanonicalText(builder *canonicalStreamBuilder, fields []Field) {
	if builder == nil || builder.store == nil {
		return
	}
	stream := builder.store.stream(builder.ref)
	for _, field := range fields {
		found := false
		for _, entry := range stream.Fields {
			if entry.Options.ShowText && entry.TextLabel == field.Name && entry.Value.Text == field.Value {
				found = true
				break
			}
		}
		if !found {
			builder.Text(field.Name, field.Value)
		}
	}
}

// structuredObjectFromKVs converts ordered scalar facts without JSON round-tripping.
func structuredObjectFromKVs(fields []jsonKV) structuredNode {
	members := make([]structuredMember, 0, len(fields))
	for _, field := range fields {
		value := structuredNode{Kind: structuredString, Text: field.Val}
		if field.Raw {
			if parsed, err := parseStructuredNode(field.Val); err == nil {
				value = parsed
			} else {
				value = structuredNode{Kind: structuredRaw, Text: field.Val}
			}
		}
		members = append(members, structuredMember{Key: field.Key, Value: value})
	}
	return structuredNode{Kind: structuredObject, Object: members}
}

// roundedMPEGPSBitRate reproduces legacy display quantization from a raw rate.
func roundedMPEGPSBitRate(bitsPerSecond float64, precise bool) string {
	if bitsPerSecond <= 0 {
		return ""
	}
	scale := 1000.0
	if bitsPerSecond >= 10_000_000 {
		scale = 100_000
	} else if precise && bitsPerSecond < 100_000 {
		scale = 100
	}
	return strconv.FormatInt(int64(math.Round(bitsPerSecond/scale)*scale), 10)
}
