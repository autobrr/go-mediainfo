package mediainfo

import (
	"bytes"
	"fmt"
	"math"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// rawTextFieldProjection is one raw-language label/value row plus ordering metadata.
type rawTextFieldProjection struct {
	Label     string
	Value     string
	Order     int
	Sequence  uint32
	Extension bool
}

// rawTextStreamProjection is one raw-language stream section.
type rawTextStreamProjection struct {
	Kind   StreamKind
	Fields []rawTextFieldProjection
}

// rawTextReportProjection contains raw-language streams in display order.
type rawTextReportProjection struct {
	Ref     string
	Streams []rawTextStreamProjection
}

// rawTextDerivedField is one raw-text row projected from a structured-only
// canonical scalar.
type rawTextDerivedField struct {
	Label string
	Value string
}

// rawTextLabelAliases maps compatibility/display labels to MediaInfo raw names.
// Canonical entries consume the field-name keyed registry built from these
// aliases; dynamic legacy input uses the aliases directly.
var rawTextLabelAliases = map[string]string{
	"ID":                                 "ID/String",
	"Menu ID":                            "MenuID/String",
	"Unique ID":                          "UniqueID/String",
	"Complete name":                      "CompleteName",
	"Format":                             "Format/String",
	"Format version":                     "Format_Version",
	"Format profile":                     "Format_Profile",
	"Format settings":                    "Format_Settings",
	"Commercial name":                    "Format_Commercial_IfAny",
	"Muxing mode":                        "MuxingMode",
	"Muxing mode, more info":             "MuxingMode_MoreInfo",
	"HDR format":                         "HDR_Format/String",
	"Format settings, CABAC":             "Format_Settings_CABAC/String",
	"Format settings, Reference frames":  "Format_Settings_RefFrames/String",
	"Format settings, BVOP":              "Format_Settings_BVOP/String",
	"Format settings, QPel":              "Format_Settings_QPel/String",
	"Format settings, GMC":               "Format_Settings_GMC/String",
	"Format settings, Matrix":            "Format_Settings_Matrix/String",
	"Format settings, GOP":               "Format_Settings_GOP",
	"Format settings, Picture structure": "Format_Settings_PictureStructure",
	"Format compression":                 "Format_Compression",
	"Codec ID":                           "CodecID",
	"Codec configuration box":            "CodecConfigurationBox",
	"Codec ID/Info":                      "CodecID/Info",
	"Duration":                           "Duration/String",
	"Source duration":                    "Source_Duration/String",
	"Overall bit rate mode":              "OverallBitRate_Mode/String",
	"Overall bit rate":                   "OverallBitRate/String",
	"Bit rate mode":                      "BitRate_Mode/String",
	"Bit rate":                           "BitRate/String",
	"Nominal bit rate":                   "BitRate_Nominal/String",
	"Maximum bit rate":                   "BitRate_Maximum/String",
	"File size":                          "FileSize/String",
	"Stream size":                        "StreamSize/String",
	"Source stream size":                 "Source_StreamSize/String",
	"Width":                              "Width/String",
	"Height":                             "Height/String",
	"Display aspect ratio":               "DisplayAspectRatio/String",
	"Original display aspect ratio":      "DisplayAspectRatio_Original/Stri",
	"Frame rate mode":                    "FrameRate_Mode/String",
	"Frame rate":                         "FrameRate/String",
	"Color space":                        "ColorSpace",
	"Chroma subsampling":                 "ChromaSubsampling/String",
	"Bit depth":                          "BitDepth/String",
	"Bit depth, detected":                "BitDepth_Detected/String",
	"Scan type":                          "ScanType/String",
	"Scan order":                         "ScanOrder/String",
	"Scan type, store method":            "ScanType_StoreMethod/String",
	"Active format description":          "ActiveFormatDescription/String",
	"Compression mode":                   "Compression_Mode/String",
	"Bits/(Pixel*Frame)":                 "Bits-(Pixel*Frame)",
	"Time code of first frame":           "TimeCode_FirstFrame",
	"Time code source":                   "TimeCode_Source",
	"GOP, Open/Closed":                   "Gop_OpenClosed/String",
	"GOP, Open/Closed of first frame":    "Gop_OpenClosed_FirstFrame/String",
	"Channel(s)":                         "Channel(s)/String",
	"Channel layout":                     "ChannelLayout",
	"Sampling rate":                      "SamplingRate/String",
	"Count of elements":                  "ElementCount",
	"Language":                           "Language/String",
	"Language, more info":                "Language_More",
	"Service kind":                       "ServiceKind/String",
	"Writing application":                "Encoded_Application/String",
	"Writing library":                    "Encoded_Library/String",
	flacUnencodedMD5Label:                "MD5_Unencoded",
	"Encoding settings":                  "Encoded_Library_Settings",
	"Encoded date":                       "Encoded_Date",
	"Tagged date":                        "Tagged_Date",
	"Default":                            "Default/String",
	"Forced":                             "Forced/String",
	"Alternate group":                    "AlternateGroup/String",
	"Color range":                        "colour_range",
	"Color primaries":                    "colour_primaries",
	"Transfer characteristics":           "transfer_characteristics",
	"Matrix coefficients":                "matrix_coefficients",
	"Mastering display color primaries":  "MasteringDisplay_ColorPrimaries",
	"Mastering display luminance":        "MasteringDisplay_Luminance",
	"Maximum Content Light Level":        "MaxCLL/String",
	"Maximum Frame-Average Light Level":  "MaxFALL/String",
	"Original source medium":             "OriginalSourceMedium",
	"Original source medium ID":          "OriginalSourceMedium_ID/String",
	"Complexity index":                   "ComplexityIndex",
	"Number of dynamic objects":          "NumberOfDynamicObjects",
	"Bed channel count":                  "BedChannelCount",
	"Bed channel configuration":          "BedChannelConfiguration",
	"Dialog Normalization":               "dialnorm",
	"Law rating":                         "LawRating",
	"Recorded location":                  "Recorded/Location",
	"Recorded_Location":                  "Recorded/Location",
	"Terms of use":                       "TermsOfUse",
	"List":                               "List/String",
	"Duration of the visible content":    "Duration_Start2End/String",
	"Start time":                         "Duration_Start/String",
	"End time":                           "Duration_End/String",
	"Count of frames before first event": "FirstDisplay_Delay_Frames",
	"Type of the first event":            "FirstDisplay_Type",
	"ACTOR_CHARACTER":                    "ACTOR/CHARACTER",
	"Encoded_Application_Url":            "Encoded_Application/Url",
	"Writing_Library":                    "Writing Library",
	"Statistics_Tags_Issue":              "Statistics Tags Issue",
	"Statistics Tags Issue":              "Statistics Tags Issue",
	"Delay relative to video":            "Video_Delay/String",
	"Original delay":                     "Delay_Original/String",
	"ID in the original source medium":   "OriginalSourceMedium_ID/String",
	"Format settings, picture structure": "Format_Settings_PictureStructure",
	"Service name":                       "ServiceName",
	"Service provider":                   "ServiceProvider",
	"Service type":                       "ServiceType",
}

// rawTextFieldRule is the authoritative raw-text policy for one canonical
// field. Container policy may replace its label, but cannot create a second
// projection of the same canonical value.
type rawTextFieldRule struct {
	Label   string
	Visible bool
}

// rawTextCanonicalFieldRules is keyed by canonical field name, including
// generated /String siblings. It prevents structured-only components from
// leaking into raw text and gives every known field one raw label.
var rawTextCanonicalFieldRules = func() map[fieldName]rawTextFieldRule {
	rules := make(map[fieldName]rawTextFieldRule, len(canonicalFieldDefinitions)*2)
	for name, spec := range canonicalFieldDefinitions {
		label := firstNonEmpty(rawTextLabelAliases[spec.TextLabel], spec.TextLabel, string(name))
		rules[name] = rawTextFieldRule{Label: label, Visible: spec.Options.ShowText}
		if spec.StringSibling != "" {
			rules[spec.StringSibling] = rawTextFieldRule{Label: label, Visible: true}
		}
	}
	for _, name := range []fieldName{
		"@type", "@typeorder", "StreamOrder", "FirstPacketOrder",
		"FrameRate_Num", "FrameRate_Den", "Encoded_Application_Name", "Encoded_Application_Version",
		"Encoded_Library_Name", "Encoded_Library_Version", "Encoded_Library_Date",
		"Format_Settings_SliceCount", "Format settings, Slice count",
		"format_identifier", "OverallBitRate_Precision_Min", "OverallBitRate_Precision_Max",
	} {
		rules[name] = rawTextFieldRule{Visible: false}
	}
	return rules
}()

// rawTextServiceKindNames expands canonical service abbreviations in source order.
var rawTextServiceKindNames = map[string]string{
	"CM": "Complete Main", "ME": "Music and Effects", "VI": "Visually Impaired", "HI": "Hearing Impaired",
	"D": "Dialogue", "C": "Commentary", "E": "Emergency", "VO": "Voice Over", "O": "Original",
}

// projectRawTextReport builds raw-language rows from one canonical report store.
func projectRawTextReport(report Report) rawTextReportProjection {
	store := canonicalStoreForReport(report)
	if store == nil {
		return rawTextReportProjection{Ref: report.Ref}
	}
	store.projectionMu.RLock()
	defer store.projectionMu.RUnlock()

	projected := rawTextReportProjection{Ref: store.ref, Streams: make([]rawTextStreamProjection, 0, len(store.streams))}
	dvd, dvdAggregate := rawTextDVDContext(store)
	generalFormat := rawTextGeneralFormat(store)
	version := reportVersion(store)
	totalFileSize := int64(0)
	streamIndexes := make([]int, len(store.streams))
	for index := range store.streams {
		streamIndexes[index] = index
	}
	sort.SliceStable(streamIndexes, func(left, right int) bool {
		return store.streams[streamIndexes[left]].TextSequence < store.streams[streamIndexes[right]].TextSequence
	})
	for _, streamIndex := range streamIndexes {
		stream := &store.streams[streamIndex]
		structured := rawTextStructuredValues(stream)
		if stream.Kind == StreamGeneral {
			totalFileSize, _ = strconv.ParseInt(structured["FileSize"], 10, 64)
		}
		fields := make([]rawTextFieldProjection, 0, len(stream.Fields))
		seen := make(map[string]struct{}, len(stream.Fields))
		for _, entry := range stream.Fields {
			if !entry.Options.ShowText {
				continue
			}
			identity := reportFieldIdentity(stream.Kind, string(entry.Name), entry.TextLabel)
			if identity == "Statistics_Tags_Issue" && stream.Kind != StreamMenu {
				if extra, exists := rawTextExtraNode(stream); exists && slices.ContainsFunc(extra.Object, func(member structuredMember) bool {
					return member.Key == identity
				}) {
					continue
				}
			}
			if !reportTextVisible(version, stream.Kind, identity) {
				continue
			}
			rule, known := rawTextProjectionRule(stream.Kind, entry)
			if !rule.Visible {
				continue
			}
			if !rawTextContainerFieldVisible(generalFormat, entry, rule) {
				continue
			}
			if stream.Kind == StreamVideo && structured["Format"] == "Theora" && rule.Label == "FrameCount" {
				continue
			}
			if _, exists := seen[rule.Label]; exists {
				continue
			}
			value := rawTextValue(stream.Kind, rule.Label, entry.Value.Text, structured, totalFileSize)
			if known && value == "" {
				continue
			}
			fields = append(fields, rawTextFieldProjection{
				Label: rule.Label, Value: value,
				Sequence: entry.Sequence, Extension: !known,
			})
			seen[rule.Label] = struct{}{}
		}
		sequence := uint32(len(stream.Fields))
		appendDerived := func(label, value string) {
			if value == "" {
				return
			}
			if _, exists := seen[label]; exists {
				return
			}
			fields = append(fields, rawTextFieldProjection{
				Label: label, Value: value, Sequence: sequence,
			})
			seen[label] = struct{}{}
			sequence++
		}
		codec := storedCodecDescription(version, generalFormat, stream, structured)
		appendDerived("CodecID/Info", codec.Info)
		appendDerived("CodecID/Hint", codec.Hint)
		appendDerived("ServiceKind/String", formatRawTextServiceKind(structured["ServiceKind"]))
		for _, derived := range rawTextStructuredDerivations(stream.Kind, structured, totalFileSize) {
			appendDerived(derived.Label, derived.Value)
		}
		if extra, ok := rawTextExtraNode(stream); ok && stream.Kind != StreamMenu {
			for _, member := range extra.Object {
				if member.Key == "ConformanceErrors" {
					for _, derived := range rawTextConformanceFields(member.Value) {
						if _, exists := seen[derived.Label]; exists {
							continue
						}
						fields = append(fields, rawTextFieldProjection{
							Label: derived.Label, Value: derived.Value,
							Sequence: sequence,
						})
						seen[derived.Label] = struct{}{}
						sequence++
					}
					continue
				}
				if member.Key == "ConformanceInfos" {
					for _, derived := range rawTextConformanceInfoFields(member.Value) {
						if _, exists := seen[derived.Label]; exists {
							continue
						}
						fields = append(fields, rawTextFieldProjection{
							Label: derived.Label, Value: derived.Value,
							Sequence: sequence,
						})
						seen[derived.Label] = struct{}{}
						sequence++
					}
					continue
				}
				value := structuredNodeText(member.Value)
				if !rawTextExtraVisible(member.Key, value) {
					continue
				}
				label := rawTextLabel(member.Key)
				if _, exists := seen[label]; exists {
					continue
				}
				fields = append(fields, rawTextFieldProjection{
					Label: label, Value: rawTextExtraValue(member.Key, value),
					Sequence: sequence,
				})
				seen[label] = struct{}{}
				sequence++
			}
		}
		if dvd {
			fields = normalizeDVDRawTextFields(stream, fields, structured, dvdAggregate)
		}
		filtered := fields[:0]
		for _, field := range fields {
			identity := reportFieldIdentity(stream.Kind, field.Label, field.Label)
			if !reportTextVisible(version, stream.Kind, identity) {
				continue
			}
			// Profile components stay in the existing summary and structured
			// facts rather than appearing again as standalone raw rows.
			if stream.Kind == StreamVideo && (identity == "Format_Tier" || identity == "Format_Level") {
				continue
			}
			if field.Label == "Format/Info" {
				_, field.Value = reportFormatPresentation(version, stream.Kind, structured)
				if field.Value == "" {
					continue
				}
			}
			field.Order = reportFieldRank(version, stream.Kind, canonicalTableName(field.Label))
			filtered = append(filtered, field)
		}
		fields = filtered
		sort.SliceStable(fields, func(left, right int) bool {
			if fields[left].Order != fields[right].Order {
				return fields[left].Order < fields[right].Order
			}
			return fields[left].Sequence < fields[right].Sequence
		})
		projected.Streams = append(projected.Streams, rawTextStreamProjection{Kind: stream.TextKind, Fields: fields})
	}
	return projected
}

// rawTextGeneralFormat returns the canonical container format used by
// representation-specific visibility rules.
func rawTextGeneralFormat(store *fieldStore) string {
	if store == nil {
		return ""
	}
	for index := range store.streams {
		stream := &store.streams[index]
		if stream.Kind == StreamGeneral {
			return rawTextStructuredValues(stream)["Format"]
		}
	}
	return ""
}

// rawTextContainerFieldVisible keeps aggregate AC-3 statistics in the raw
// projections where MediaInfo exposes them (DVD) or where the reviewed BDAV
// extension contract retains them. Other containers still expose the direct
// dialnorm/compr/dynrng values, but not the aggregate statistic rows.
func rawTextContainerFieldVisible(generalFormat string, entry fieldEntry, rule rawTextFieldRule) bool {
	for _, name := range []string{string(entry.Name), entry.StructuredKey, rule.Label} {
		switch name {
		case "Format settings, Slice count":
			return false
		case "compr_Average", "compr_Minimum", "compr_Maximum", "compr_Count",
			"dynrng_Average", "dynrng_Minimum", "dynrng_Maximum", "dynrng_Count":
			return generalFormat == "BDAV" || generalFormat == "DVD Video"
		}
	}
	return true
}

// rawTextDVDContext identifies DVD projections from canonical container facts.
// Aggregate IFO reports are distinguished by payload-stream provenance, not by
// input path or sample identity.
func rawTextDVDContext(store *fieldStore) (dvd, aggregate bool) {
	if store == nil {
		return false, false
	}
	generalFormat := ""
	for index := range store.streams {
		stream := &store.streams[index]
		structured := rawTextStructuredValues(stream)
		if stream.Kind == StreamGeneral {
			generalFormat = structured["Format"]
			if generalFormat == "DVD Video" {
				dvd = true
			}
		}
		if stream.Kind == StreamMenu && structured["Format"] == "DVD-Video" {
			dvd = true
		}
	}
	if generalFormat != "DVD Video" {
		return dvd, false
	}
	for index := range store.streams {
		stream := &store.streams[index]
		if stream.Kind == StreamGeneral || stream.Kind == StreamMenu {
			continue
		}
		extra, ok := rawTextExtraNode(stream)
		if !ok {
			continue
		}
		for _, member := range extra.Object {
			if member.Key == "Source" && structuredNodeText(member.Value) != "" {
				return true, true
			}
		}
	}
	return dvd, false
}

// normalizeDVDRawTextFields projects DVD raw text from the same canonical
// values that already back JSON. Legacy friendly fields can contain the
// pre-aggregation IFO values, so canonical values and the ordered extra object
// are authoritative here. Raw registry labels, including /String suffixes, are
// preserved exactly.
func normalizeDVDRawTextFields(stream *storedStream, fields []rawTextFieldProjection, structured map[string]string, aggregate bool) []rawTextFieldProjection {
	if stream == nil {
		return fields
	}

	extra, hasExtra := rawTextExtraNode(stream)
	extraAliases := make(map[string]struct{})
	if hasExtra && stream.Kind != StreamGeneral {
		for _, member := range extra.Object {
			for _, label := range []string{member.Key, rawTextLabel(member.Key), rawTextDVDExtraLabel(member.Key)} {
				if label != "" {
					extraAliases[label] = struct{}{}
				}
			}
			for _, label := range rawTextDVDLegacyExtraLabels(member.Key) {
				extraAliases[label] = struct{}{}
			}
		}
	}
	if stream.Kind == StreamMenu {
		for _, key := range []string{"List_Audio", "List_Subtitles_4_3", "List_Subtitles_Wide", "List_Subtitles_Letterbox", "List_Subtitles_PanScan"} {
			for _, label := range append([]string{key, rawTextLabel(key), rawTextDVDExtraLabel(key)}, rawTextDVDLegacyExtraLabels(key)...) {
				extraAliases[label] = struct{}{}
			}
		}
	}

	normalized := make([]rawTextFieldProjection, 0, len(fields)+len(extra.Object))
	seen := make(map[string]struct{}, len(fields)+len(extra.Object))
	for _, field := range fields {
		if _, fromExtra := extraAliases[field.Label]; fromExtra {
			continue
		}
		if rawTextDVDFieldHidden(stream.Kind, field.Label) {
			continue
		}
		if field.Label == "Language, more info" {
			field.Label = "Language_More"
		}
		switch field.Label {
		case "Conformance warnings":
			field.Label = "ConformanceWarnings"
		case " General compliance":
			field.Label = " GeneralCompliance"
		case "Caption service name":
			field.Label = "CaptionServiceName"
		}
		switch field.Label {
		case "FileSize/String":
			if size, err := strconv.ParseInt(structured["FileSize"], 10, 64); err == nil {
				field.Value = formatRawTextByteUnits(stream.Kind, formatByteQuantity(size, false), structured["ElementCount"])
			}
		case "OverallBitRate/String":
			if value := formatDVDRawTextBitRate(structured["OverallBitRate"]); value != "" {
				field.Value = value
			}
		case "BitRate/String", "BitRate_Nominal/String", "BitRate_Maximum/String":
			key := strings.TrimSuffix(field.Label, "/String")
			if value := formatDVDRawTextBitRate(structured[key]); value != "" {
				field.Value = value
			}
		case "Bits-(Pixel*Frame)":
			if value := structured["Bits-(Pixel*Frame)"]; value != "" {
				field.Value = value
			}

		case "ScanOrder/String":
			if value := structured["ScanOrder"]; value != "" {
				field.Value = value
			}
		case "Duration/String":
			field.Value = strings.TrimPrefix(field.Value, "0s ")
		}
		if _, exists := seen[field.Label]; exists {
			continue
		}
		normalized = append(normalized, field)
		seen[field.Label] = struct{}{}
	}

	sequence := uint32(len(stream.Fields))
	if stream.Kind == StreamAudio || stream.Kind == StreamText {
		if value := formatRawTextDelay(structured["Video_Delay"]); value != "" {
			label := "Video_Delay/String"
			if _, exists := seen[label]; !exists {
				normalized = append(normalized, rawTextFieldProjection{
					Label: label, Value: value, Sequence: sequence,
				})
				seen[label] = struct{}{}
				sequence++
			}
		}
	}

	if hasExtra && stream.Kind != StreamGeneral {
		for _, member := range extra.Object {
			value := structuredNodeText(member.Value)
			if !rawTextDVDExtraVisible(stream.Kind, member.Key, value, aggregate) {
				continue
			}
			label := rawTextDVDExtraLabel(member.Key)
			if label == "" {
				continue
			}
			if !aggregate || stream.Kind == StreamText || stream.Kind == StreamMenu {
				value = rawTextExtraValue(member.Key, value)
			}
			normalized = append(normalized, rawTextFieldProjection{
				Label: label, Value: value, Sequence: sequence,
			})
			sequence++
		}
	}
	return normalized
}

// rawTextDVDFieldHidden reports whether a legacy field label is suppressed from
// the DVD raw-text projection for the given stream kind.
func rawTextDVDFieldHidden(kind StreamKind, label string) bool {
	switch label {
	case "Delay relative to video", "Original delay", "Format settings, Mode":
		return true
	}
	switch kind {
	case StreamVideo:
		return label == "Video_Delay/String" || label == "Delay"
	case StreamAudio:
		return label == "Video_Delay/String" || label == "Delay"
	case StreamMenu:
		return label == "Delay" || label == "Frame count" || label == "FrameCount" || label == "FrameRate/String"
	case StreamGeneral, StreamText, StreamImage:
		return false
	}
	return false
}

// rawTextDVDExtraVisible applies DVD-specific visibility rules before falling
// back to the shared raw-text extra policy.
func rawTextDVDExtraVisible(kind StreamKind, key, value string, aggregate bool) bool {
	if kind == StreamMenu {
		return value != ""
	}
	if aggregate && kind == StreamText && key == "CaptionServiceName" {
		return value != ""
	}
	if aggregate && kind == StreamAudio {
		switch key {
		case "IsTruncated", "format_identifier", "OverallBitRate_Precision_Min", "OverallBitRate_Precision_Max",
			"CaptionServiceName", "CaptionServiceContent_IsPresent", "CaptionServiceDescriptor_IsPresent":
			return false
		default:
			return value != ""
		}
	}
	if aggregate && kind == StreamVideo && (key == "intra_dc_precision" || key == "Source") {
		return value != ""
	}
	return rawTextExtraVisible(key, value)
}

// rawTextDVDExtraLabel converts a canonical DVD extra key to its raw-text label.
func rawTextDVDExtraLabel(key string) string {
	if trimmed, ok := strings.CutSuffix(key, "_String"); ok {
		return trimmed
	}
	switch key {
	case "List_Audio":
		return "List (Audio)"
	case "List_Subtitles_4_3":
		return "List (Subtitles 4/3)"
	case "List_Subtitles_Wide":
		return "  Wide)"
	case "List_Subtitles_Letterbox":
		return "  Letterbox)"
	case "List_Subtitles_PanScan":
		return "  Pan&Scan)"
	}
	if len(key) == len("_00_00_00_000") && key[0] == '_' {
		parts := strings.Split(key[1:], "_")
		if len(parts) == 4 {
			return parts[0] + ":" + parts[1] + ":" + parts[2] + "." + parts[3]
		}
	}
	return rawTextLabel(key)
}

// rawTextDVDLegacyExtraLabels returns legacy labels that may duplicate a
// canonical DVD extra during raw-text projection.
func rawTextDVDLegacyExtraLabels(key string) []string {
	switch key {
	case "List_Audio":
		return []string{"List (Audio)"}
	case "List_Subtitles_4_3":
		return []string{"List (Subtitles 4/3)"}
	case "List_Subtitles_Wide":
		return []string{"List (Subtitles Wide)"}
	case "List_Subtitles_Letterbox":
		return []string{"List (Subtitles Letterbox)"}
	case "List_Subtitles_PanScan":
		return []string{"List (Subtitles Pan&Scan)"}
	case "CaptionServiceName":
		return []string{"Caption service name"}
	default:
		return nil
	}
}

// formatDVDRawTextBitRate formats DVD raw bitrates, including values at or
// above ten gigabits per second that require a Gbps representation.
func formatDVDRawTextBitRate(value string) string {
	bitRate, err := strconv.ParseFloat(value, 64)
	if err != nil || bitRate <= 0 {
		return ""
	}
	if bitRate >= 10_000_000_000 {
		return fmt.Sprintf("%.1f Gbps", bitRate/1_000_000_000)
	}
	return formatRawTextDerivedBitRate(value)
}

// rawTextProjectionRule resolves one canonical or imported text entry to its
// single raw-text policy. Unknown labels remain visible Go extensions.
func rawTextProjectionRule(kind StreamKind, entry fieldEntry) (rawTextFieldRule, bool) {
	identity := reportFieldIdentity(kind, string(entry.Name), entry.TextLabel)
	if table := reportKindPolicy(reportTables23, kind); table != nil {
		for _, name := range []string{identity + "/String", identity} {
			if row, ok := table.Fields[name]; ok {
				label := name
				if label == "CodecID/String" && kind != StreamGeneral {
					label = "CodecID"
				}
				if strings.HasPrefix(label, "Channels") {
					label = "Channel(s)" + strings.TrimPrefix(label, "Channels")
				}
				label = rawReportFieldName(label)
				if alias := rawTextLabelAliases[label]; alias != "" {
					label = alias
				}
				return rawTextFieldRule{Label: label, Visible: strings.HasPrefix(row.Options, "Y")}, true
			}
		}
	}

	if rule, ok := rawTextCanonicalFieldRules[entry.Name]; ok {
		if kind == StreamGeneral && entry.TextLabel == "Codec ID" {
			rule.Label = "CodecID/String"
		}
		return rule, true
	}
	if key := fieldName(firstNonEmpty(entry.StructuredKey, string(entry.Name))); key != "" {
		if rule, ok := rawTextCanonicalFieldRules[key]; ok {
			return rule, true
		}
	}
	friendly := firstNonEmpty(entry.TextLabel, string(entry.Name))
	return rawTextFieldRule{Label: rawTextLabel(friendly), Visible: true}, false
}

// rawTextStructuredDerivations exposes structured-only canonical facts that
// MediaInfo includes in raw text but the friendly renderer intentionally omits.
func rawTextStructuredDerivations(kind StreamKind, structured map[string]string, totalFileSize int64) []rawTextDerivedField {
	fields := make([]rawTextDerivedField, 0, 24)
	appendField := func(label, value string) {
		if value != "" {
			fields = append(fields, rawTextDerivedField{Label: label, Value: value})
		}
	}
	appendField("MuxingMode", structured["MuxingMode"])
	appendField("Format_Version", formatRawTextFormatVersion(structured["Format_Version"]))
	appendField("Format/Info", rawTextFormatInfo(kind, structured))
	appendField("Format_Commercial_IfAny", structured["Format_Commercial_IfAny"])
	appendField("Format_Profile", formatRawTextProfile(kind, structured, structured["Format_Profile"]))
	appendField("Format_Settings", formatRawTextSettings(structured))
	appendField("Format_Settings_Floor", structured["Format_Settings_Floor"])
	if count := structured["Format_Settings_SliceCount"]; count != "" && count != "1" {
		appendField("Format_Settings_SliceCount/Strin", count+" slice per frame")
	}
	appendField("BitRate_Mode/String", formatRawTextMode(structured["BitRate_Mode"], "CBR", "VBR"))
	bitRateText := formatRawTextDerivedBitRate(structured["BitRate"])
	if kind == StreamAudio && structured["Format"] == "PCM" {
		bitRateText = formatRawTextPCMBitRate(structured["BitRate"])
	}
	appendField("BitRate/String", bitRateText)
	appendField("BitRate_Nominal/String", formatRawTextDerivedBitRate(structured["BitRate_Nominal"]))
	appendField("BitRate_Maximum/String", formatRawTextDerivedBitRate(structured["BitRate_Maximum"]))
	appendField("BitRate_Minimum/String", formatRawTextDerivedBitRate(structured["BitRate_Minimum"]))
	appendField("OverallBitRate_Maximum/String", formatRawTextDerivedBitRate(structured["OverallBitRate_Maximum"]))
	if duration, err := strconv.ParseFloat(structured["Duration"], 64); err == nil && kind != StreamGeneral {
		appendField("Duration/String", formatRawTextDuration(duration))
	}
	if duration, err := strconv.ParseFloat(structured["Source_Duration"], 64); err == nil && kind != StreamGeneral {
		appendField("Source_Duration/String", formatRawTextDuration(duration))
	}
	if kind == StreamVideo || kind == StreamImage {
		if width := structured["Width"]; width != "" {
			appendField("Width/String", formatRawTextCount(width, "pixel"))
		}
		if height := structured["Height"]; height != "" {
			appendField("Height/String", formatRawTextCount(height, "pixel"))
		}
	}
	if kind == StreamImage {
		appendField("ChromaSubsampling", structured["ChromaSubsampling"])
	}
	for _, key := range []string{"FrameRate", "FrameRate_Minimum", "FrameRate_Maximum"} {
		if structured[key] != "" {
			appendField(key+"/String", rawTextValue(kind, key+"/String", "", structured, totalFileSize))
		}
	}
	if frameRate := structured["FrameRate_Original"]; frameRate != "" {
		if rawTextAVIVisual(structured) {
			appendField("FrameRate_Original/String", formatRawTextOriginalFrameRate(frameRate))
		} else {
			appendField("FrameRate_Original/String", frameRate+" fps")
		}
	}
	if ratio := structured["DisplayAspectRatio"]; ratio != "" {
		appendField("DisplayAspectRatio/String", formatRawTextAspectRatio(ratio))
	}
	if ratio := structured["DisplayAspectRatio_Original"]; ratio != "" {
		appendField("DisplayAspectRatio_Original/Stri", formatRawTextAspectRatio(ratio))
	}
	appendField("ColorSpace", structured["ColorSpace"])
	appendField("Channel(s)/String", formatRawTextCount(structured["Channels"], "channel"))
	appendField("ChannelLayout", structured["ChannelLayout"])
	if channels := structured["Channels_Original"]; channels != "" {
		appendField("Channel(s)_Original/String", formatRawTextCount(channels, "channel"))
	}
	appendField("ChannelLayout_Original", structured["ChannelLayout_Original"])
	if bitDepth := structured["BitDepth"]; bitDepth != "" {
		appendField("BitDepth/String", formatRawTextCount(bitDepth, "bit"))
	}
	if bitDepth := structured["BitDepth_Detected"]; bitDepth != "" && bitDepth != structured["BitDepth"] {
		appendField("BitDepth_Detected/String", formatRawTextCount(bitDepth, "bit"))
	}
	appendField("Compression_Mode/String", structured["Compression_Mode"])
	appendField("Format_Compression", structured["Format_Compression"])
	for _, key := range []string{"ScanOrder", "ScanOrder_Original", "ScanOrder_Stored"} {
		appendField(key+"/String", structured[key])
	}
	appendField("ScanType_StoreMethod/String", structured["ScanType_StoreMethod"])
	appendField("ActiveFormatDescription/String", formatRawTextActiveFormatDescription(structured["ActiveFormatDescription"]))
	appendField("Standard", structured["Standard"])
	appendField("MultiView_Count", structured["MultiView_Count"])
	appendField("MultiView_Layout", structured["MultiView_Layout"])
	bitsPerPixelFrame := structured["Bits-(Pixel*Frame)"]
	if bitsPerPixelFrame == "" && kind == StreamVideo {
		bitRate, _ := strconv.ParseFloat(firstNonEmpty(structured["BitRate"], structured["BitRate_Nominal"]), 64)
		width, _ := strconv.ParseUint(structured["Width"], 10, 64)
		height, _ := strconv.ParseUint(structured["Height"], 10, 64)
		frameRate, _ := strconv.ParseFloat(structured["FrameRate"], 64)
		bitsPerPixelFrame = formatBitsPerPixelFrame(bitRate, width, height, frameRate)
	}
	appendField("Bits-(Pixel*Frame)", bitsPerPixelFrame)
	if delay := formatRawTextDelay(structured["Video_Delay"]); delay != "" {
		appendField("Video_Delay/String", delay)
	}
	if size, err := strconv.ParseInt(structured["StreamSize"], 10, 64); err == nil && kind != StreamGeneral {
		appendField("StreamSize/String", formatRawTextByteUnits(kind, formatStreamSizeQuantity(size, totalFileSize, false), structured["ElementCount"]))
	}
	appendField("Encoded_Library_Settings", structured["Encoded_Library_Settings"])
	appendField("Encoded_Library/String", formatRawTextEncodedLibraryForStream(structured["Encoded_Library"], structured))
	appendField("matrix_coefficients", structured["matrix_coefficients"])
	appendField("colour_range", structured["colour_range"])
	appendField("colour_primaries", structured["colour_primaries"])
	appendField("transfer_characteristics", structured["transfer_characteristics"])
	appendField("transfer_characteristics_Origina", structured["transfer_characteristics_Original"])
	appendField("MasteringDisplay_ColorPrimaries", structured["MasteringDisplay_ColorPrimaries"])
	appendField("MasteringDisplay_Luminance", structured["MasteringDisplay_Luminance"])
	if value := structured["MaxCLL"]; value != "" {
		appendField("MaxCLL/String", value+" cd/m2")
	}
	if value := structured["MaxFALL"]; value != "" {
		appendField("MaxFALL/String", value+" cd/m2")
	}
	appendField("TimeCode_FirstFrame", structured["TimeCode_FirstFrame"])
	appendField("TimeCode_Source", structured["TimeCode_Source"])
	appendField("Format_Settings_GOP", structured["Format_Settings_GOP"])
	appendField("Gop_OpenClosed/String", structured["Gop_OpenClosed"])
	appendField("Gop_OpenClosed_FirstFrame/String", structured["Gop_OpenClosed_FirstFrame"])
	if alignment := structured["Alignment"]; alignment != "" {
		appendField("Alignment/String", "Alignment_"+alignment)
	}
	if duration := structured["Interleave_Duration"]; duration != "" {
		if seconds, err := strconv.ParseFloat(duration, 64); err == nil {
			value := fmt.Sprintf("%.0f ms", seconds*1000)
			if frames := structured["Interleave_VideoFrames"]; frames != "" {
				value += " (" + frames + " video frames)"
			}
			appendField("Interleave_Duration/String", value)
		}
	}
	if preload := structured["Interleave_Preload"]; preload != "" {
		if seconds, err := strconv.ParseFloat(preload, 64); err == nil {
			appendField("Interleave_Preload/String", fmt.Sprintf("%.0f ms", seconds*1000))
		}
	}

	appendField("Type/String", structured["Type"])
	if language := structured["Language"]; language != "" {
		appendField("Language/String", formatRawTextLanguage(language, formatLanguage(language)))
	}
	for _, key := range []string{"Title", "Cover", "Cover_Description", "Cover_Type", "Cover_Mime", "Description", "Released_Date", "Recorded_Date", "EncodedBy", "Genre", "Performer", "ContentType", "Copyright", "OriginalSourceForm", "Synopsis", "Comment"} {
		appendField(key, structured[key])
	}
	appendField("OverallBitRate_Mode/String", formatRawTextMode(structured["OverallBitRate_Mode"], "CBR", "VBR"))
	if kind == StreamVideo && structured["Format"] == "AV1" {
		appendField("Format/Info", "AOMedia Video 1")
		if rawTextAV1UsesFilmGrain(structured) {
			appendField("Format_Settings", "Film Grain Synthesis")
		}
	}
	if kind == StreamVideo && structured["HDR_Format"] != "" {
		appendField("HDR_Format/String", formatRawTextDerivedHDR(structured))
	}
	return fields
}

// formatRawTextCount adds a raw unit to positive canonical counts, including lists.
func formatRawTextCount(value, unit string) string {
	if value == "" {
		return ""
	}
	for _, part := range strings.Split(value, " / ") {
		if count, err := strconv.ParseUint(part, 10, 64); err != nil || count == 0 {
			return ""
		}
	}
	return value + " " + unit
}

// formatRawTextSamplingRate formats canonical hertz without reading display text.
func formatRawTextSamplingRate(value string) string {
	parts := strings.Split(value, " / ")
	for index, part := range parts {
		rate, err := strconv.ParseFloat(part, 64)
		if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return ""
		}
		parts[index] = formatSampleRateQuantity(rate, true)
	}
	return strings.Join(parts, " / ")
}

// formatRawTextActiveFormatDescription expands the coded AFD value used by
// the raw registry.
func formatRawTextActiveFormatDescription(value string) string {
	switch value {
	case "10":
		return "Letterbox 16:9 image"
	default:
		return ""
	}
}

// formatRawTextProfile composes video profile, level, and tier tokens using
// raw registry spelling.
func formatRawTextProfile(kind StreamKind, structured map[string]string, value string) string {
	if kind != StreamVideo || value == "" {
		return value
	}
	if level := structured["Format_Level"]; level != "" && !strings.Contains(value, "@"+level) && !strings.Contains(value, "@L"+level) {
		value += "@L" + strings.TrimPrefix(level, "L")
	}
	if tier := structured["Format_Tier"]; tier != "" && !strings.Contains(value, "@"+tier) {
		value += "@" + tier
	}
	return value
}

// formatRawTextFormatVersion adds the registry prefix used by raw text while
// preserving versions that already include it.
func formatRawTextFormatVersion(value string) string {
	if value != "" && !strings.HasPrefix(value, "Version ") {
		return "Version " + value
	}
	return value
}

// rawTextFrameRateUsesRatio reports when MediaInfo's raw registry exposes the
// canonical fraction instead of only the rounded display rate.
func rawTextFrameRateUsesRatio(kind StreamKind, structured map[string]string, numerator, denominator string) bool {
	if numerator == "" || denominator == "" || denominator == "1" {
		return false
	}
	if kind == StreamText || structured["Format"] == "MPEG-4 Visual" {
		return true
	}
	if kind == StreamVideo && structured["Format"] == "AVC" && numerator == "30000" && denominator == "1001" {
		return true
	}
	return numerator == "23976" && denominator == "1000"
}

// rawTextAV1UsesFilmGrain reports film-grain signaling retained by parsed
// encoder settings.
func rawTextAV1UsesFilmGrain(structured map[string]string) bool {
	settings := structured["Encoded_Library_Settings"]
	return strings.Contains(settings, "grain=") && !strings.Contains(settings, "grain=0")
}

// formatRawTextSettings composes the aggregate raw settings row from the
// format-specific canonical setting scalars.
func formatRawTextSettings(structured map[string]string) string {
	if value := structured["Format_Settings"]; value != "" {
		return value
	}
	if structured["Format"] == "MPEG Audio" {
		parts := make([]string, 0, 2)
		for _, key := range []string{"Format_Settings_Mode", "Format_Settings_ModeExtension"} {
			if value := structured[key]; value != "" {
				parts = append(parts, value)
			}
		}
		return strings.Join(parts, " / ")
	}
	if structured["Format"] == "AAC" {
		if value := structured["Format_Settings_SBR"]; strings.HasPrefix(value, "Yes (") && strings.HasSuffix(value, ")") {
			return strings.TrimSuffix(strings.TrimPrefix(value, "Yes ("), ")")
		}
	}
	if structured["Format_Settings_Endianness"] != "" && structured["Format_Settings_Sign"] != "" {
		return structured["Format_Settings_Endianness"] + " / " + structured["Format_Settings_Sign"]
	}
	if value := structured["Format_Settings_Mode"]; value != "" && (structured["Format"] == "AC-3" || structured["Format"] == "AC-3 Dep" || structured["Format"] == "E-AC-3") {
		return value
	}
	if matrix, bvop := structured["Format_Settings_Matrix"], structured["Format_Settings_BVOP"]; strings.HasPrefix(matrix, "Custom") && rawTextAVIVisual(structured) {
		parts := make([]string, 0, 2)
		if bvop != "" && bvop != "No" && bvop != "0" {
			if bvop == "Yes" {
				parts = append(parts, "BVOP")
			} else {
				parts = append(parts, "BVOP"+bvop)
			}
		}
		parts = append(parts, "Custom Matrix")
		return strings.Join(parts, " / ")
	}
	if matrix, bvop := structured["Format_Settings_Matrix"], structured["Format_Settings_BVOP"]; strings.HasPrefix(matrix, "Custom") && bvop != "" {
		return "CustomMatrix / BVOP"
	}
	if value := structured["Format_Settings_BVOP"]; value != "" {
		if value == "Yes" {
			return "BVOP"
		}
		return "BVOP" + value
	}
	return ""
}

// formatRawTextDerivedHDR renders static HDR metadata when no friendly HDR
// summary exists.
func formatRawTextDerivedHDR(structured map[string]string) string {
	format := structured["HDR_Format"]
	compatibility := structured["HDR_Format_Compatibility"]
	if format == "SMPTE ST 2086" && strings.Contains(compatibility, "HDR10") {
		return "SMPTE ST 2086, HDR10 compatible"
	}
	return ""
}

// formatRawTextMode maps canonical long mode names to raw abbreviations.
func formatRawTextMode(value, constant, variable string) string {
	switch value {
	case "Constant", "CBR":
		return constant
	case "Variable", "VBR":
		return variable
	default:
		return value
	}
}

// formatRawTextDerivedBitRate renders a structured-only raw bitrate using the
// raw registry's unit convention.
func formatRawTextDerivedBitRate(value string) string {
	parsed, err := strconv.ParseFloat(value, 32)
	bitRate := float32(parsed)
	if err != nil || bitRate <= 0 {
		return ""
	}
	if bitRate > 100_000_000 {
		return fmt.Sprintf("%.0f Mbps", bitRate/1_000_000)
	}
	if bitRate > 10_000_000 {
		return fmt.Sprintf("%.1f Mbps", bitRate/1_000_000)
	}
	return formatRawTextBitRate(value)
}

// formatRawTextDelay renders a non-zero canonical delay stored in seconds.
func formatRawTextDelay(value string) string {
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds == 0 {
		return ""
	}
	milliseconds := int64(math.Round(seconds * 1000))
	if milliseconds > -1000 && milliseconds < 1000 {
		return fmt.Sprintf("%dms", milliseconds)
	}
	sign := ""
	if milliseconds < 0 {
		sign = "-"
		milliseconds = -milliseconds
	}
	return fmt.Sprintf("%s%ds %dms", sign, milliseconds/1000, milliseconds%1000)
}

// rawTextExtraVisible excludes diagnostic/statistical members that MediaInfo
// keeps out of raw text while allowing ordinary tags and codec detail rows.
func rawTextExtraVisible(key, value string) bool {
	switch key {
	case "Source_Delay", "Source_Delay_Source", "bsid", "acmod", "lfeon",
		"dialnorm_Count", "compr_Average", "compr_Minimum", "compr_Maximum", "compr_Count",
		"dynrng_Average", "dynrng_Minimum", "dynrng_Maximum", "dynrng_Count",
		"intra_dc_precision", "IsTruncated", "format_identifier",
		"OverallBitRate_Precision_Min", "OverallBitRate_Precision_Max",
		"CaptionServiceName", "CaptionServiceContent_IsPresent", "CaptionServiceDescriptor_IsPresent":
		return false
	case "dsurmod":
		return value != "0"
	default:
		return true
	}
}

// formatRawTextServiceKind expands canonical short codes while preserving
// already-expanded service names and their source order.
func formatRawTextServiceKind(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, " / ")
	for index, part := range parts {
		if name := rawTextServiceKindNames[part]; name != "" {
			parts[index] = name
		}
	}
	return strings.Join(parts, " / ")
}

// rawTextExtraValue applies MediaInfo's display units and enum names to an
// otherwise structured-only extra value.
func rawTextExtraValue(key, value string) string {
	switch key {
	case "BedChannelCount":
		if value != "" && !strings.HasSuffix(value, " channel") {
			return value + " channel"
		}
	case "dialnorm", "dialnorm_Average", "dialnorm_Minimum", "dialnorm_Maximum",
		"compr", "dynrng", "cmixlev", "surmixlev", "ltrtcmixlev", "ltrtsurmixlev",
		"lorocmixlev", "lorosurmixlev", "mixlevel":
		if value != "" && !strings.HasSuffix(value, " dB") {
			return value + " dB"
		}
	case "dsurmod":
		switch value {
		case "1":
			return "Not Dolby Surround encoded"
		case "2":
			return "Dolby Surround encoded"
		}
	}
	return value
}

// formatRawTextByteUnits applies raw text's singular byte spelling and the
// decimal precision used for tiny subtitle payloads.
func formatRawTextByteUnits(kind StreamKind, value, elementCount string) string {
	value = strings.ReplaceAll(value, " Bytes", " Byte")
	if kind != StreamText {
		return value
	}
	count, err := strconv.ParseUint(elementCount, 10, 64)
	if err != nil || count > 2 {
		return value
	}
	number, tail, ok := strings.Cut(value, " Byte")
	if !ok || strings.Contains(number, ".") {
		return value
	}
	bytes, err := strconv.ParseUint(number, 10, 64)
	if err != nil || bytes >= 1000 {
		return value
	}
	return number + ".0 Byte" + tail
}

// rawTextConformanceFields flattens AVI's nested conformance tree into the
// indented rows emitted by MediaInfo's raw text registry.
func rawTextConformanceFields(node structuredNode) []rawTextDerivedField {
	if node.Kind != structuredArray {
		return nil
	}
	for _, item := range node.Array {
		if item.Kind != structuredObject {
			continue
		}
		for _, group := range item.Object {
			if group.Key != "B_" || group.Value.Kind != structuredArray {
				continue
			}
			for _, groupItem := range group.Value.Array {
				if groupItem.Kind != structuredObject {
					continue
				}
				for _, detail := range groupItem.Object {
					if detail.Key != "GeneralCompliance" || detail.Value.Kind != structuredString {
						continue
					}
					count := len(strings.Split(detail.Value.Text, " / "))
					if count == 0 {
						continue
					}
					flags := make([]string, count)
					for index := range flags {
						flags[index] = "Yes"
					}
					return []rawTextDerivedField{
						{Label: "ConformanceErrors", Value: strconv.Itoa(count)},
						{Label: " B_\u00C2\u00A6", Value: strings.Join(flags, " / ")},
						{Label: "  GeneralCompliance", Value: detail.Value.Text},
					}
				}
			}
		}
	}
	return nil
}

// rawTextConformanceInfoFields flattens informational parser diagnostics into
// MediaInfo's count, parser-group, and indented detail rows.
func rawTextConformanceInfoFields(node structuredNode) []rawTextDerivedField {
	if node.Kind != structuredArray || len(node.Array) == 0 {
		return nil
	}
	fields := []rawTextDerivedField{{Label: "ConformanceInfos", Value: strconv.Itoa(len(node.Array))}}
	for _, item := range node.Array {
		if item.Kind != structuredObject {
			continue
		}
		for _, group := range item.Object {
			if group.Value.Kind != structuredArray {
				continue
			}
			groupLabel := group.Key
			if groupLabel == "MPEGAudio" {
				groupLabel = "MPEG-Audio"
			}
			fields = append(fields, rawTextDerivedField{Label: " " + groupLabel, Value: "Yes"})
			for _, groupItem := range group.Value.Array {
				if groupItem.Kind != structuredObject {
					continue
				}
				for _, detail := range groupItem.Object {
					if detail.Value.Kind == structuredString {
						fields = append(fields, rawTextDerivedField{Label: "  " + detail.Key, Value: detail.Value.Text})
					}
				}
			}
		}
	}
	return fields
}

// rawTextStructuredValues returns one preferred scalar per canonical key.
func rawTextStructuredValues(stream *storedStream) map[string]string {
	values := make(map[string]string, len(stream.Fields))
	for _, entry := range stream.Fields {
		if !entry.Options.ShowStructured || entry.Node != nil || entry.Projected {
			continue
		}
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if _, exists := values[key]; !exists {
			values[key] = entry.Value.Text
		}
	}
	for _, entry := range stream.Fields {
		if !entry.Options.ShowStructured || entry.Node != nil || !entry.Projected {
			continue
		}
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if _, exists := values[key]; !exists {
			values[key] = entry.Value.Text
		}
	}
	return values
}

// rawTextExtraNode returns the preferred ordered extra object for a stream.
func rawTextExtraNode(stream *storedStream) (structuredNode, bool) {
	var fallback *structuredNode
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if !entry.Options.ShowStructured || key != "extra" || entry.Node == nil || entry.Node.Kind != structuredObject {
			continue
		}
		if !entry.Projected {
			return *entry.Node, true
		}
		fallback = entry.Node
	}
	if fallback == nil {
		return structuredNode{}, false
	}
	return *fallback, true
}

// rawTextLabel resolves a friendly compatibility label to MediaInfo's raw name.
func rawTextLabel(friendly string) string {
	if label := rawTextLabelAliases[friendly]; label != "" {
		return label
	}
	return friendly
}

// rawTextValue applies raw-language presentation without changing canonical facts.
func rawTextValue(kind StreamKind, label, display string, structured map[string]string, totalFileSize int64) string {
	switch label {
	case "ID/String":
		if kind == StreamGeneral && structured["Format"] == "BDAV" || rawTextOggStream(structured) {
			if id, err := strconv.ParseUint(structured["ID"], 10, 64); err == nil {
				return fmt.Sprintf("%d (0x%X)", id, id)
			}
		}
	case "MuxingMode":
		if value := structured["MuxingMode"]; value != "" {
			return value
		}
	case "Format/String":
		name, _ := reportFormatPresentation(reportTables23, kind, structured)
		return name
	case "Format/Info":
		if value := rawTextFormatInfo(kind, structured); value != "" {
			return value
		}
	case "HDR_Format/String":
		return formatRawTextHDR(display, structured)
	case "Duration/String", "Source_Duration/String", "Duration_Start2End/String", "Duration_Start/String", "Duration_End/String":
		key := strings.TrimSuffix(label, "/String")
		if value := structured[key]; value != "" {
			if milliseconds, err := strconv.ParseFloat(value, 64); err == nil {
				return formatRawTextDuration(milliseconds)
			}
		}
	case "Video_Delay/String":
		return formatRawTextDelay(structured["Video_Delay"])
	case "Source_Duration_LastFrame/String":
		if value, present := structured["Source_Duration_LastFrame"]; present {
			// This field is stored in milliseconds; relative-delay facts use seconds.
			milliseconds, err := strconv.ParseFloat(value, 64)
			if err != nil || math.IsNaN(milliseconds) || math.IsInf(milliseconds, 0) {
				return ""
			}
			if milliseconds == 0 {
				return fmt.Sprintf("%.0fms", milliseconds)
			}
			return formatRawTextDelay(strconv.FormatFloat(milliseconds/1000, 'f', -1, 64))
		}
	case "OverallBitRate/String", "BitRate/String", "BitRate_Nominal/String", "BitRate_Maximum/String", "BitRate_Minimum/String":
		key := strings.TrimSuffix(label, "/String")
		if label != "OverallBitRate/String" {
			if kind == StreamAudio && structured["Format"] == "PCM" {
				if value := formatRawTextPCMBitRate(structured[key]); value != "" {
					return value
				}
			}
			if value := formatRawTextDerivedBitRate(structured[key]); value != "" {
				return value
			}
		}
		if kind == StreamAudio || kind == StreamText {
			if value := formatRawTextBitRate(structured[key]); value != "" {
				return value
			}
		}
		return formatRawTextDerivedBitRate(structured[key])
	case "FrameRate/String", "FrameRate_Original/String", "FrameRate_Minimum/String", "FrameRate_Maximum/String":
		key := strings.TrimSuffix(label, "/String")
		rate, err := strconv.ParseFloat(structured[key], 64)
		if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return ""
		}
		if key == "FrameRate_Original" && rawTextAVIVisual(structured) {
			return formatRawTextOriginalFrameRate(structured[key])
		}
		result := fmt.Sprintf("%.3f", rate)
		if kind == StreamAudio && key == "FrameRate" {
			if sampling, _ := strconv.ParseFloat(structured["SamplingRate"], 64); sampling == rate {
				return ""
			}
			result += " fps"
			if spf := validSamplesPerFrame(structured["SamplesPerFrame"]); spf != "" {
				result += " (" + spf + " SPF)"
			}
			return result
		}
		if key == "FrameRate" {
			num, den := structured["FrameRate_Num"], structured["FrameRate_Den"]
			fraction := " (" + num + "/" + den + ")"
			// Retain a producer's display fraction only for the same canonical pair.
			if rawTextFrameRateUsesRatio(kind, structured, num, den) || num != "" && den != "" && den != "1" && strings.Contains(display, fraction) {
				result += fraction
			}
		}
		return result + " fps"
	case "SamplingRate/String", "SamplingRate_Original/String":
		if value, present := structured[strings.TrimSuffix(label, "/String")]; present {
			return formatRawTextSamplingRate(value)
		}
		return strings.ReplaceAll(display, " kHz", " KHz")
	case "Channel(s)/String", "Channel(s)_Original/String":
		key := strings.ReplaceAll(strings.TrimSuffix(label, "/String"), "Channel(s)", "Channels")
		if value, present := structured[key]; present {
			return formatRawTextCount(value, "channel")
		}
		return strings.ReplaceAll(display, " channels", " channel")
	case "BitDepth/String", "BitDepth_Detected/String":
		if value, present := structured[strings.TrimSuffix(label, "/String")]; present {
			return formatRawTextCount(value, "bit")
		}
		return strings.ReplaceAll(display, " bits", " bit")
	case "Width/String", "Height/String":
		if value, present := structured[strings.TrimSuffix(label, "/String")]; present {
			return formatRawTextCount(value, "pixel")
		}
		value := strings.TrimSuffix(strings.TrimSuffix(display, " pixels"), " pixel")
		return strings.ReplaceAll(value, " ", "") + " pixel"
	case "FrameRate_Mode/String":
		return formatRawTextMode(firstNonEmpty(structured["FrameRate_Mode"], display), "CFR", "VFR")
	case "BitRate_Mode/String", "OverallBitRate_Mode/String":
		key := strings.TrimSuffix(label, "/String")
		return formatRawTextMode(firstNonEmpty(structured[key], display), "CBR", "VBR")
	case "Encoded_Library/String":
		if value := structured["Encoded_Library"]; value != "" {
			display = value
		}
		return formatRawTextEncodedLibraryForStream(display, structured)
	case "Encoded_Application/String":
		if name, version := structured["Encoded_Application_Name"], structured["Encoded_Application_Version"]; name != "" && version != "" {
			return name + " " + version
		}
	case "Language/String":
		if structured["Language"] == "lvs" {
			return "lvs"
		}
		return formatRawTextLanguage(structured["Language"], display)
	case "ServiceKind/String":
		if value := formatRawTextServiceKind(structured["ServiceKind"]); value != "" {
			return value
		}
	case "StreamSize/String":
		if size, err := strconv.ParseInt(structured["StreamSize"], 10, 64); err == nil && size >= 0 {
			if kind == StreamText && size == 0 {
				return "0.00 Byte (0%)"
			}
			if size > 0 && totalFileSize <= 0 {
				return formatRawTextByteUnits(kind, formatByteQuantity(size, false), structured["ElementCount"])
			}
			return formatRawTextByteUnits(kind, formatStreamSizeQuantity(size, totalFileSize, false), structured["ElementCount"])
		}
		return formatRawTextByteUnits(kind, display, structured["ElementCount"])
	case "FileSize/String", "Source_StreamSize/String":
		if size, err := strconv.ParseInt(structured[strings.TrimSuffix(label, "/String")], 10, 64); err == nil {
			value := formatByteQuantity(size, false)
			if label == "Source_StreamSize/String" && size > 0 && totalFileSize > 0 {
				value = formatStreamSizeQuantity(size, totalFileSize, false)
			}
			return formatRawTextByteUnits(kind, value, structured["ElementCount"])
		}
		return formatRawTextByteUnits(kind, display, structured["ElementCount"])
	case "BedChannelCount":
		if value := structured["BedChannelCount"]; value != "" {
			return value + " channel"
		}
	case "ChannelLayout":
		if value := structured["ChannelLayout"]; value != "" {
			if kind == StreamAudio && structured["Format"] == "Opus" && strings.HasPrefix(value, "C L R ") {
				return "L R C " + strings.TrimPrefix(value, "C L R ")
			}
			return value
		}
	case "CodecID":
		if value := structured["CodecID"]; value != "" {
			return value
		}
	case "UniqueID/String":
		return trimRawTextHexPadding(display)
	case "OriginalSourceMedium_ID/String":
		return formatOriginalSourceID(structured["OriginalSourceMedium_ID"], display)
	case "Format_Settings_BVOP/String":
		if value := structured["Format_Settings_BVOP"]; value != "" {
			return value
		}
	case "Format_Settings_GMC/String":
		if value := structured["Format_Settings_GMC"]; value != "" {
			return value + " warppoint"
		}
	case "Format_Settings_Matrix/String":
		if value := structured["Format_Settings_Matrix"]; value != "" {
			return value
		}
	case "Title":
		if kind == StreamAudio && display == "Audio" {
			return "Audio Stream"
		}
	case "Format_Version":
		return formatRawTextFormatVersion(display)
	case "Format_Settings":
		if value := formatRawTextSettings(structured); value != "" {
			return value
		}
	case "Bits-(Pixel*Frame)":
		if kind == StreamVideo {
			bitRate, _ := strconv.ParseFloat(firstNonEmpty(structured["BitRate"], structured["BitRate_Nominal"]), 64)
			width, _ := strconv.ParseUint(structured["Width"], 10, 64)
			height, _ := strconv.ParseUint(structured["Height"], 10, 64)
			frameRate, _ := strconv.ParseFloat(structured["FrameRate"], 64)
			if value := formatBitsPerPixelFrame(bitRate, width, height, frameRate); value != "" {
				return value
			}
		}
	case "Format_Settings_RefFrames/String":
		if value, present := structured["Format_Settings_RefFrames"]; present {
			return formatRawTextCount(value, "frame")
		}
		return strings.ReplaceAll(display, " frames", " frame")
	case "Format_Profile":
		return formatRawTextProfile(kind, structured, display)
	case "DisplayAspectRatio/String":
		if structured["Format"] == "Theora" {
			return formatRawTextAspectRatio(structured["DisplayAspectRatio"])
		}
		if value := structured["DisplayAspectRatio"]; value != "" && rawTextAVIVisual(structured) {
			return formatRawTextAVIAspectRatio(value)
		}
		if value := structured["DisplayAspectRatio"]; value != "" && rawTextMP4Visual(structured) {
			if formatted := formatRawTextAspectRatio(value); formatted != value {
				return formatted
			}
		}
	case "ScanOrder/String", "ScanOrder_Original/String", "ScanOrder_Stored/String", "ScanType_StoreMethod/String":
		if value := structured[strings.TrimSuffix(label, "/String")]; value != "" {
			return value
		}
	case "ScanType/String":
		if value := structured["ScanType"]; value != "" {
			return value
		}
	case "Standard":
		if value := structured["Standard"]; value != "" {
			return value
		}
	case "colour_primaries":
		if structured["colour_primaries"] == "SMPTE 170M" || display == "SMPTE 170M" {
			return "BT.601 NTSC"
		}
	case "matrix_coefficients":
		if structured["matrix_coefficients"] == "SMPTE 170M" || display == "SMPTE 170M" {
			return "BT.601"
		}
	case "transfer_characteristics", "transfer_characteristics_Origina":
		return strings.ReplaceAll(display, "BT.2020 10-bit", "BT.2020 (10-bit)")
	case "ChromaSubsampling/String":
		if position := structured["ChromaSubsampling_Position"]; position != "" && !strings.Contains(display, position) {
			return display + " (" + position + ")"
		}
	}
	return display
}

// formatRawTextOriginalFrameRate preserves MediaInfo's decimal-ratio form for
// the AVI original-rate value produced from a 23.976 fps stream header.
func formatRawTextOriginalFrameRate(value string) string {
	if value == "23.976" {
		return "23.976 (23976/1000) fps"
	}
	return value + " fps"
}

// rawTextAVIVisual reports whether canonical codec facts identify an AVI
// MPEG-4 Visual stream rather than a Matroska or MP4 codec identifier.
func rawTextAVIVisual(structured map[string]string) bool {
	if structured["Format"] != "MPEG-4 Visual" {
		return false
	}
	switch strings.ToUpper(structured["CodecID"]) {
	case "DIVX", "DX50", "FMP4", "MP4V", "XVID":
		return true
	default:
		return false
	}
}

// rawTextMP4Visual reports whether canonical codec facts identify an MP4
// visual sample entry whose raw aspect ratio uses named common ratios.
func rawTextMP4Visual(structured map[string]string) bool {
	switch strings.ToLower(structured["CodecID"]) {
	case "avc1", "avc3", "hvc1", "hev1", "mp4v", "vp09":
		return true
	default:
		return false
	}
}

// formatRawTextAVIAspectRatio maps AVI header ratios that MediaInfo presents
// as named display ratios while preserving the shared formatter elsewhere.
func formatRawTextAVIAspectRatio(value string) string {
	ratio, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	switch {
	case math.Abs(ratio-5.0/4.0) < 0.01:
		return "5:4"
	case math.Abs(ratio-16.0/9.0) < 0.05:
		return "16:9"
	default:
		return formatRawTextAspectRatio(value)
	}
}

// trimRawTextHexPadding removes serializer padding from a parenthesized raw
// hexadecimal ID while retaining the decimal display.
func trimRawTextHexPadding(display string) string {
	start := strings.Index(display, "(0x")
	if start < 0 || !strings.HasSuffix(display, ")") {
		return display
	}
	hex := strings.TrimLeft(display[start+3:len(display)-1], "0")
	if hex == "" {
		hex = "0"
	}
	return display[:start] + "(0x" + hex + ")"
}

// formatRawTextHDR composes the static mastering component that MediaInfo
// appends to a Dolby Vision raw display.
func formatRawTextHDR(display string, structured map[string]string) string {
	format := structured["HDR_Format"]
	if !strings.Contains(display, "Dolby Vision") {
		return display
	}
	if format == "Dolby Vision / Dolby Vision" && !strings.Contains(display, " / ") {
		first := display
		if profile := rawTextDolbyVisionCommercialProfile(structured["HDR_Format_Profile"]); profile != "" {
			first = strings.Replace(first, profile+", ", "", 1)
		}
		return first + " / " + display
	}
	if strings.Contains(format, "SMPTE ST 2086") {
		const mastering = "SMPTE ST 2086, Version HDR10, HDR10 compatible"
		if !strings.Contains(display, "SMPTE ST 2086") {
			return display + " / " + mastering
		}
	}
	if strings.Contains(format, "SMPTE ST 2094 App 4") {
		profile := "HDR10+ Profile A"
		if strings.Contains(structured["HDR_Format_Compatibility"], "HDR10+ Profile B") {
			profile = "HDR10+ Profile B"
		}
		dynamic := fmt.Sprintf("SMPTE ST 2094 App 4, Version %s, %s compatible", profile, profile)
		if prefix, _, ok := strings.Cut(display, " / SMPTE ST 2094 App 4"); ok {
			return prefix + " / " + dynamic
		}
		return display + " / " + dynamic
	}
	return display
}

// rawTextDolbyVisionCommercialProfile maps a Dolby Vision codec profile to
// the commercial profile token used in raw HDR summaries.
func rawTextDolbyVisionCommercialProfile(value string) string {
	value = strings.Split(value, " / ")[0]
	value = strings.TrimPrefix(value, "dvhe.")
	value = strings.TrimPrefix(value, "dvh1.")
	if value == "" {
		return ""
	}
	profile := strings.Split(value, ".")[0]
	profile = strings.TrimLeft(profile, "0")
	if profile == "" {
		return ""
	}
	return "Profile " + profile
}

// formatRawTextBitRate renders ungrouped raw rates using MediaInfo's bps/Kbps
// boundary and precision.
// formatRawTextLanguage emits raw registry language codes and formats a single
// region or script subtag as MediaInfo's parenthesized suffix.
func formatRawTextLanguage(code, display string) string {
	if strings.Contains(code, " / ") {
		parts := strings.Split(code, " / ")
		for index, part := range parts {
			if part != "" {
				parts[index] = formatRawTextLanguage(part, part)
			}
		}
		return strings.Join(parts, " / ")
	}
	normalized := normalizeLanguageCode(code)
	parts := strings.Split(normalized, "-")
	if len(parts) < 2 {
		if normalized != "" {
			return normalized
		}
		return display
	}
	if len(parts) == 2 {
		return fmt.Sprintf("%s (%s)", parts[0], parts[1])
	}
	return normalized
}

// formatRawTextEncodedLibrary normalizes canonical encoder identifiers and
// version strings to MediaInfo's raw library/date spelling.
func formatRawTextEncodedLibrary(display string) string {
	if suffix, ok := strings.CutPrefix(display, "x264 - "); ok {
		return "x264 " + suffix
	}
	if suffix, ok := strings.CutPrefix(display, "x265 - "); ok {
		return "x265 " + suffix
	}
	if strings.HasPrefix(display, "XviD") {
		if version, date, ok := xvidLibraryVersionDate(display); ok {
			value := "XviD " + version
			if date != "" {
				value += " (" + date + ")"
			}
			return value
		}
	}
	if strings.HasPrefix(display, "DivX") {
		if version, date, ok := divxLibraryVersionDate(display); ok {
			value := "DivX " + version
			if date != "" {
				value += " (" + date + ")"
			}
			return value
		}
	}
	if strings.HasPrefix(display, "libmakemkv v") {
		return strings.Replace(display, "libmakemkv v", "libmakemkv ", 1)
	}
	if strings.HasPrefix(display, "encoded by TMPGEnc ") {
		value := strings.TrimPrefix(display, "encoded by ")
		if version, ok := strings.CutPrefix(value, "TMPGEnc (ver. "); ok {
			return "TMPGEnc " + strings.TrimSuffix(version, ")")
		}
		return value
	}
	if rest, ok := strings.CutPrefix(display, "Xiph.Org libVorbis I "); ok {
		date := rest
		if prefix, _, found := strings.Cut(rest, " "); found {
			date = prefix
		}
		if len(date) == 8 {
			formattedDate := fmt.Sprintf("%s-%s-%s", date[:4], date[4:6], date[6:])
			switch rest {
			case "20020717":
				return "libVorbis 1.0 (" + formattedDate + ")"
			case date:
				return "libVorbis " + date + " (" + formattedDate + ")"
			default:
				suffix := strings.TrimSpace(strings.TrimPrefix(rest, date))
				return "libVorbis " + suffix + " (" + rest + ")"
			}
		}
	}
	if rest, ok := strings.CutPrefix(display, "AO; aoTuV "); ok {
		if open, closeIndex := strings.IndexByte(rest, '['), strings.IndexByte(rest, ']'); open >= 0 && closeIndex > open {
			version := strings.TrimSpace(rest[:open])
			date := rest[open+1 : closeIndex]
			if len(date) == 8 {
				formattedDate := fmt.Sprintf("%s-%s-%s", date[:4], date[4:6], date[6:])
				if version == "" {
					return "aoTuV " + date + " (" + formattedDate + ")"
				}
				return "aoTuV " + version + "  (" + formattedDate + ")"
			}
		}
	}
	value := strings.TrimPrefix(display, "reference ")
	parts := strings.Fields(value)
	if len(parts) == 3 && strings.HasPrefix(parts[0], "libFLAC") && len(parts[2]) == 8 {
		if _, err := strconv.Atoi(parts[2]); err == nil {
			return fmt.Sprintf("%s %s (%s-%s-%s)", parts[0], parts[1], parts[2][:4], parts[2][4:6], parts[2][6:])
		}
	}
	return display
}

func formatRawTextEncodedLibraryForStream(display string, structured map[string]string) string {
	if rawTextOggStream(structured) {
		switch structured["Format"] {
		case "Theora", "Vorbis":
			return formatOggLibraryDisplay(structured["Format"], display)
		}
	}
	return formatRawTextEncodedLibrary(display)
}

// rawTextOggStream identifies Ogg codec streams from container projection
// facts. Matroska uses explicit codec IDs and stream ordering for the same
// codecs, so identity metadata is neither needed nor consulted.
func rawTextOggStream(structured map[string]string) bool {
	if structured["CodecID"] != "" || structured["StreamOrder"] != "" {
		return false
	}
	switch structured["Format"] {
	case "Theora", "Vorbis", "Opus":
		return true
	default:
		return false
	}
}

// rawTextFormatInfo returns the few compound audio descriptions whose raw
// value must be composed from canonical format facts.
func rawTextFormatInfo(kind StreamKind, structured map[string]string) string {
	_, info := reportFormatPresentation(reportTables23, kind, structured)
	return info
}

// renderRawText serializes raw-language projections with MediaInfo's 33-column labels.
func renderRawText(reports []Report, informVersion bool) string {
	const labelWidth = 33

	var buffer bytes.Buffer
	for reportIndex, report := range reports {
		if reportIndex > 0 {
			buffer.WriteByte('\n')
		}
		projected := projectRawTextReport(report)
		if len(projected.Streams) == 0 {
			continue
		}
		writeRawTextFields(&buffer, string(projected.Streams[0].Kind), projected.Streams[0].Fields)
		streams := projected.Streams[1:]
		counts := make(map[StreamKind]int)
		for _, stream := range streams {
			counts[stream.Kind]++
		}
		kindIndexes := make(map[StreamKind]int)
		for _, stream := range streams {
			kindIndexes[stream.Kind]++
			buffer.WriteByte('\n')
			writeRawTextFields(&buffer, streamTitle(stream.Kind, kindIndexes[stream.Kind], counts[stream.Kind]), stream.Fields)
		}
		if informVersion {
			buffer.WriteString(reportByLine(labelWidth))
			buffer.WriteByte('\n')
		}
	}
	output := strings.TrimRight(buffer.String(), "\n")
	eol := "\n"
	if runtime.GOOS == "windows" {
		eol = "\r\n"
	}
	return strings.ReplaceAll(output+"\n\n\n", "\n", eol)
}

// writeRawTextFields writes one raw stream section.
func writeRawTextFields(buffer *bytes.Buffer, title string, fields []rawTextFieldProjection) {
	buffer.WriteString(title)
	buffer.WriteByte('\n')
	for _, field := range fields {
		label := escapeOutputControls(field.Label)
		buffer.WriteString(padRawTextLabel(label, 33))
		if utf8.RuneCountInString(label) >= 33 {
			buffer.WriteByte(' ')
		}
		buffer.WriteString(": ")
		buffer.WriteString(escapeOutputControls(field.Value))
		buffer.WriteByte('\n')
	}
}

// padRawTextLabel aligns raw labels by Unicode code point. The writer adds a
// separator space for labels that fill or exceed the registry column.
func padRawTextLabel(value string, width int) string {
	length := utf8.RuneCountInString(value)
	if length >= width {
		return value
	}
	return value + strings.Repeat(" ", width-length)
}
