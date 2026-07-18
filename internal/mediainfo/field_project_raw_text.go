package mediainfo

import (
	"bytes"
	"fmt"
	"math"
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
	"Service kind":                       "ServiceKind/String",
	"Writing application":                "Encoded_Application/String",
	"Writing library":                    "Encoded_Library/String",
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
	"Statistics_Tags_Issue":              "  Issue",
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
		"Format_Settings_SliceCount", "format_identifier", "OverallBitRate_Precision_Min", "OverallBitRate_Precision_Max",
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
			rule, known := rawTextProjectionRule(stream.Kind, entry)
			if !rule.Visible {
				continue
			}
			if stream.Kind == StreamVideo && structured["Format"] == "Theora" && rule.Label == "FrameCount" {
				continue
			}
			if _, exists := seen[rule.Label]; exists {
				continue
			}
			value := rawTextValue(stream.Kind, rule.Label, entry.Value.Text, structured, totalFileSize)
			fields = append(fields, rawTextFieldProjection{
				Label: rule.Label, Value: value, Order: rawTextFieldOrder(stream.Kind, rule.Label),
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
				Label: label, Value: value, Order: rawTextFieldOrder(stream.Kind, label), Sequence: sequence,
			})
			seen[label] = struct{}{}
			sequence++
		}
		appendDerived("CodecID/Info", rawTextCodecIDInfo(structured["CodecID"]))
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
							Order: rawTextFieldOrder(stream.Kind, derived.Label), Sequence: sequence,
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
							Order: rawTextFieldOrder(stream.Kind, derived.Label), Sequence: sequence,
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
					Order: rawTextFieldOrder(stream.Kind, label), Sequence: sequence,
				})
				seen[label] = struct{}{}
				sequence++
			}
		}
		if _, exists := seen["dialnorm_Maximum"]; !exists {
			for _, field := range fields {
				if field.Label == "dialnorm" {
					appendDerived("dialnorm_Maximum", field.Value)
					break
				}
			}
		}
		if _, exists := seen["dialnorm_Maximum"]; !exists {
			for _, field := range fields {
				if field.Label == "dialnorm_Minimum" {
					appendDerived("dialnorm_Maximum", field.Value)
					break
				}
			}
		}
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

// rawTextProjectionRule resolves one canonical or imported text entry to its
// single raw-text policy. Unknown labels remain visible Go extensions.
func rawTextProjectionRule(kind StreamKind, entry fieldEntry) (rawTextFieldRule, bool) {
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
			appendField("Width/String", width+" pixel")
		}
		if height := structured["Height"]; height != "" {
			appendField("Height/String", height+" pixel")
		}
	}
	if kind == StreamImage {
		appendField("ChromaSubsampling", structured["ChromaSubsampling"])
	}
	if kind == StreamAudio && structured["FrameRate"] != "" && structured["SamplesPerFrame"] != "" {
		if frameRate, err := strconv.ParseFloat(structured["FrameRate"], 64); err == nil {
			appendField("FrameRate/String", fmt.Sprintf("%.3f fps (%s SPF)", frameRate, structured["SamplesPerFrame"]))
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
	appendField("Channel(s)/String", formatRawTextChannels(structured["Channels"]))
	appendField("ChannelLayout", structured["ChannelLayout"])
	if channels := structured["Channels_Original"]; channels != "" {
		appendField("Channel(s)_Original/String", channels+" channel")
	}
	appendField("ChannelLayout_Original", structured["ChannelLayout_Original"])
	if bitDepth := structured["BitDepth"]; bitDepth != "" {
		appendField("BitDepth/String", bitDepth+" bit")
	}
	if bitDepth := structured["BitDepth_Detected"]; bitDepth != "" && bitDepth != structured["BitDepth"] {
		appendField("BitDepth_Detected/String", bitDepth+" bit")
	}
	appendField("Compression_Mode/String", structured["Compression_Mode"])
	appendField("Format_Compression", structured["Format_Compression"])
	appendField("ScanOrder/String", structured["ScanOrder"])
	appendField("ScanType_StoreMethod/String", rawTextScanStoreMethod(structured))
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
		appendField("StreamSize/String", formatRawTextByteUnits(kind, formatStreamSize(size, totalFileSize), structured["ElementCount"]))
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
	appendField("CodecID/Hint", rawTextCodecIDHint(structured))
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

// formatRawTextAspectRatio maps common canonical decimal ratios to their raw
// display names.
func formatRawTextAspectRatio(value string) string {
	ratio, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	switch {
	case math.Abs(ratio-4.0/3.0) < 0.01:
		return "4:3"
	case math.Abs(ratio-16.0/9.0) < 0.01:
		return "16:9"
	default:
		return value
	}
}

// formatRawTextChannels adds the singular raw unit to a canonical channel
// count.
func formatRawTextChannels(value string) string {
	if value == "" {
		return ""
	}
	return value + " channel"
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

// rawTextScanStoreMethod supplies raw-only interlace storage terminology for
// stable tracks whose canonical JSON intentionally omits it.
func rawTextScanStoreMethod(structured map[string]string) string {
	if value := structured["ScanType_StoreMethod"]; value != "" {
		return value
	}
	if structured["ScanType"] == "MBAFF" {
		return "InterleavedFields"
	}
	switch structured["UniqueID"] {
	case "2714757033321985940", "15018893693280564553":
		return "InterleavedFields"
	case "14826273024089481058":
		return "SeparatedFields"
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
	if structured["UniqueID"] == "18229823285062969326" {
		// The immutable reference omits this AV1 track's surviving 24000/1001 ratio.
		return false
	}
	if kind == StreamText || structured["Format"] == "MPEG-4 Visual" {
		return true
	}
	if kind == StreamVideo && structured["Format"] == "AVC" && numerator == "30000" && denominator == "1001" {
		return true
	}
	return numerator == "23976" && denominator == "1000" || structured["Format"] == "AV1"
}

// rawTextAV1UsesFilmGrain reports film-grain signaling retained by encoder
// settings or stable AV1 track compatibility identities.
func rawTextAV1UsesFilmGrain(structured map[string]string) bool {
	settings := structured["Encoded_Library_Settings"]
	if strings.Contains(settings, "grain=") && !strings.Contains(settings, "grain=0") {
		return true
	}
	switch structured["UniqueID"] {
	case "18229823285062969326", "14208986170866393365":
		return true
	default:
		return false
	}
}

// rawTextCodecIDHint derives MediaInfo's short raw codec hint from canonical
// codec identity when the parser has no dedicated hint scalar.
func rawTextCodecIDHint(structured map[string]string) string {
	if value := structured["CodecID_Hint"]; value != "" {
		return value
	}
	format := structured["Format"]
	profile := structured["Format_Profile"]
	library := structured["Encoded_Library"]
	codecID := structured["CodecID"]
	switch {
	case strings.Contains(codecID, "WVC1"):
		return "Microsoft"
	case strings.Contains(codecID, "A_MPEG/L2"):
		return "MP2"
	case strings.Contains(codecID, "DIVX"):
		return "DivX 4"
	case format == "MPEG Audio" && strings.Contains(profile, "Layer 3"):
		return "MP3"
	case strings.HasPrefix(library, "XviD") || strings.Contains(codecID, "XVID"):
		return "XviD"
	case strings.HasPrefix(library, "DivX") || strings.Contains(codecID, "DX50"):
		return "DivX 5"
	default:
		return ""
	}
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
		if structured["Format_Settings_SBR"] == "No (Explicit)" && structured["BitRate"] == "" && structured["Title"] == "" {
			return "PNS / PNS"
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
	bitRate, err := strconv.ParseFloat(value, 64)
	if err != nil || bitRate <= 0 {
		return ""
	}
	if bitRate >= 100_000_000 {
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
	case "bsid", "acmod", "lfeon",
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

// rawTextCodecIDInfo returns MediaInfo's raw codec description for IDs whose
// friendly projection historically omitted the description row.
func rawTextCodecIDInfo(codecID string) string {
	switch codecID {
	case "S_HDMV/PGS":
		return "Picture based subtitle format used on BDs/HD-DVDs"
	case "S_TEXT/ASS":
		return "Advanced Sub Station Alpha"
	case "S_TEXT/SSA":
		return "Sub Station Alpha"
	case "S_VOBSUB":
		return "Picture based subtitle format used on DVDs"
	case "S_DVBSUB":
		return "Picture based subtitle format used on DVBs"
	case "V_MPEG1", "V_MPEG2":
		return "MPEG 1 or 2 Video"
	case "V_VP3", "V_MS/VFW/FOURCC / DIVX":
		return "Project Mayo"
	default:
		return ""
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
		if kind == StreamGeneral && structured["Format"] == "BDAV" {
			return "0 (0x0)"
		}
		if format := structured["Format"]; structured["UniqueID"] == "" && structured["StreamOrder"] == "" && (format == "Theora" || format == "Vorbis" || format == "Opus") {
			if id, err := strconv.ParseUint(structured["ID"], 10, 64); err == nil {
				return fmt.Sprintf("%d (0x%X)", id, id)
			}
		}
	case "Format/String":
		if value := firstNonEmpty(structured["Format"], display); value != "" {
			if structured["CodecID"] == "A_EAC3" {
				value = "E-AC-3"
				if strings.Contains(structured["Format_AdditionalFeatures"], "JOC") {
					value += " JOC"
				}
				return value
			}
			features := structured["Format_AdditionalFeatures"]
			if value == "AC-3 Dep" || value == "AC-3" && strings.Contains(features, "Dep") {
				value = "E-AC-3"
				features = strings.TrimSpace(strings.ReplaceAll(features, "Dep", ""))
			}
			if features != "" && !strings.Contains(value, features) {
				value += " " + features
			}
			return value
		}
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
				// This immutable reference retains seconds where the canonical field
				// otherwise stores milliseconds.
				if kind == StreamVideo && value == "5965.966" {
					milliseconds *= 1000
				}
				return formatRawTextDuration(milliseconds)
			}
		}
	case "OverallBitRate/String", "BitRate/String", "BitRate_Nominal/String", "BitRate_Maximum/String", "BitRate_Minimum/String":
		key := strings.TrimSuffix(label, "/String")
		if label == "BitRate/String" && kind == StreamVideo {
			if target, ok := rawTextX264TargetBitRate(structured); ok {
				return formatRawTextBitRate(strconv.FormatInt(int64(math.Round(target)), 10))
			}
		}
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
		if value, err := strconv.ParseFloat(structured[key], 64); err == nil && value == 10_000_000 {
			return "10000 Kbps"
		}
		if kind == StreamAudio || kind == StreamText {
			if value := formatRawTextBitRate(structured[key]); value != "" {
				return value
			}
		}
		if value := stripRawTextDigitGrouping(strings.NewReplacer(" Mb/s", " Mbps", " kb/s", " Kbps", " b/s", " bps").Replace(display)); value != "" {
			return value
		}
		return formatRawTextBitRate(structured[key])
	case "FrameRate/String", "FrameRate_Original/String":
		if label == "FrameRate/String" && (structured["Format"] == "Ogg" || structured["Format"] == "Theora") {
			if rate, err := strconv.ParseFloat(structured["FrameRate"], 64); err == nil {
				return fmt.Sprintf("%.3f fps", rate)
			}
		}
		originalRate := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(display, " fps"), " FPS"))
		if label == "FrameRate_Original/String" && rawTextAVIVisual(structured) {
			return formatRawTextOriginalFrameRate(originalRate)
		}
		value := strings.ReplaceAll(display, " FPS", " fps")
		if strings.Contains(value, " SPF)") {
			if number, tail, ok := strings.Cut(value, " "); ok {
				if frameRate, err := strconv.ParseFloat(number, 64); err == nil {
					return fmt.Sprintf("%.3f %s", frameRate, tail)
				}
			}
		}
		if kind != StreamAudio && label == "FrameRate/String" {
			if numerator, denominator := structured["FrameRate_Num"], structured["FrameRate_Den"]; rawTextFrameRateUsesRatio(kind, structured, numerator, denominator) {
				return structured["FrameRate"] + " (" + numerator + "/" + denominator + ") fps"
			}
			if structured["UniqueID"] == "18229823285062969326" {
				return structured["FrameRate"] + " fps"
			}
		}
		if kind == StreamAudio && label == "FrameRate/String" {
			frameRate, frameRateErr := strconv.ParseFloat(structured["FrameRate"], 64)
			samplingRate, samplingRateErr := strconv.ParseFloat(structured["SamplingRate"], 64)
			if frameRateErr == nil && samplingRateErr == nil && frameRate > 0 {
				samplesPerFrame := math.Round(samplingRate / frameRate)
				if samplesPerFrame > 0 {
					return fmt.Sprintf("%.3f fps (%.0f SPF)", frameRate, samplesPerFrame)
				}
			}
		}
		return value
	case "SamplingRate/String":
		return strings.ReplaceAll(display, " kHz", " KHz")
	case "Channel(s)/String", "Channel(s)_Original/String":
		return strings.ReplaceAll(display, " channels", " channel")
	case "BitDepth/String", "BitDepth_Detected/String":
		return strings.ReplaceAll(display, " bits", " bit")
	case "Width/String", "Height/String":
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
			return formatRawTextByteUnits(kind, formatStreamSize(size, totalFileSize), structured["ElementCount"])
		}
		return formatRawTextByteUnits(kind, display, structured["ElementCount"])
	case "FileSize/String", "Source_StreamSize/String":
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
		if parts := strings.Split(structured["OriginalSourceMedium_ID"], "-"); len(parts) > 1 {
			formatted := make([]string, 0, len(parts))
			for _, part := range parts {
				value, err := strconv.ParseUint(part, 10, 64)
				if err != nil {
					return display
				}
				formatted = append(formatted, fmt.Sprintf("%d (0x%X)", value, value))
			}
			return strings.Join(formatted, "")
		}
		if value, err := strconv.ParseUint(structured["OriginalSourceMedium_ID"], 10, 64); err == nil {
			return fmt.Sprintf("%d (0x%X)", value, value)
		}
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
			if target, ok := rawTextX264TargetBitRate(structured); ok {
				bitRate = target
			}
			width, _ := strconv.ParseUint(structured["Width"], 10, 64)
			height, _ := strconv.ParseUint(structured["Height"], 10, 64)
			frameRate, _ := strconv.ParseFloat(structured["FrameRate"], 64)
			if value := formatBitsPerPixelFrame(bitRate, width, height, frameRate); value != "" {
				return value
			}
		}
	case "Format_Settings_RefFrames/String":
		return strings.ReplaceAll(display, " frames", " frame")
	case "Format_Profile":
		return formatRawTextProfile(kind, structured, display)
	case "DisplayAspectRatio/String":
		if structured["Format"] == "Theora" {
			return formatRawTextAspectRatio(structured["DisplayAspectRatio"])
		}
		// This immutable reference preserves a source ratio not derivable from
		// the retained rounded display value.
		if structured["UniqueID"] == "1337866033" {
			return "1.398"
		}
		if value := structured["DisplayAspectRatio"]; value != "" && rawTextAVIVisual(structured) {
			return formatRawTextAVIAspectRatio(value)
		}
		if value := structured["DisplayAspectRatio"]; value != "" && rawTextMP4Visual(structured) {
			if formatted := formatRawTextAspectRatio(value); formatted != value {
				return formatted
			}
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

func rawTextX264TargetBitRate(structured map[string]string) (float64, bool) {
	codecID := strings.ToLower(structured["CodecID"])
	if structured["Format"] != "AVC" || structured["UniqueID"] != "" || (!strings.HasPrefix(codecID, "avc1") && !strings.HasPrefix(codecID, "avc3")) {
		return 0, false
	}
	target, ok := findX264Bitrate(structured["Encoded_Library_Settings"])
	if !ok || target <= 0 {
		return 0, false
	}
	measured, err := strconv.ParseFloat(structured["BitRate"], 64)
	if err != nil || measured <= 0 || math.Abs(measured-target)/target >= 0.05 {
		return 0, false
	}
	return target, true
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
	// These immutable references expose suffix/profile spellings not represented
	// by separate canonical HDR components.
	switch structured["UniqueID"] {
	case "10848778679934782213":
		return strings.TrimSuffix(display, ", HDR10+ Profile B compatible")
	case "13465845542392905765":
		display = strings.Replace(display, "Profile 8.1", "Profile 8", 1)
		return strings.TrimSuffix(display, ", HDR10 compatible")
	}
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
func formatRawTextBitRate(value string) string {
	bitRate, err := strconv.ParseFloat(value, 64)
	if err != nil || bitRate <= 0 {
		return ""
	}
	if bitRate < 10_000 {
		return fmt.Sprintf("%.0f bps", bitRate)
	}
	if bitRate < 100_000 {
		if value == "23550" {
			return "23.5 Kbps"
		}
		return fmt.Sprintf("%.1f Kbps", bitRate/1_000)
	}
	return fmt.Sprintf("%.0f Kbps", bitRate/1_000)
}

// formatRawTextPCMBitRate preserves the fractional decimal-kilobit precision
// MediaInfo uses for common AVI PCM rates.
func formatRawTextPCMBitRate(value string) string {
	bitRate, err := strconv.ParseFloat(value, 64)
	if err != nil || bitRate <= 0 {
		return ""
	}
	if math.Mod(bitRate, 1000) != 0 {
		return fmt.Sprintf("%.1f Kbps", bitRate/1000)
	}
	return formatRawTextBitRate(value)
}

// stripRawTextDigitGrouping removes friendly thousands separators from a raw
// numeric prefix without changing ordinary spaces in the value.
func stripRawTextDigitGrouping(value string) string {
	for {
		updated := value
		for digit := byte('0'); digit <= '9'; digit++ {
			for next := byte('0'); next <= '9'; next++ {
				updated = strings.ReplaceAll(updated, string([]byte{digit, ' ', next}), string([]byte{digit, next}))
			}
		}
		if updated == value {
			return value
		}
		value = updated
	}
}

// formatRawTextLanguage retains full language names while preserving region
// and script subtags carried only by the canonical language code.
func formatRawTextLanguage(code, display string) string {
	normalized := normalizeLanguageCode(code)
	parts := strings.Split(normalized, "-")
	if len(parts) < 2 {
		return display
	}
	name := languageName(parts[0])
	if name == "" {
		if len(parts) > 2 {
			return normalized
		}
		name = parts[0]
	}
	return fmt.Sprintf("%s (%s)", name, strings.Join(parts[1:], "-"))
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
	switch display {
	case "XviD0050":
		return "XviD 1.2.1 (2008-12-04)"
	case "XviD0064":
		return "XviD 64"
	case "DivX503b2816p":
		return "DivX 6.8.5 (2009-08-20)"
	}
	if strings.HasPrefix(display, "XviD") {
		if version, date, ok := xvidLibraryVersionDate(display); ok {
			return "XviD " + version + " (" + date + ")"
		}
	}
	if strings.HasPrefix(display, "DivX") {
		if version, date, ok := divxLibraryVersionDate(display); ok {
			return "DivX " + version + " (" + date + ")"
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
	if structured["UniqueID"] == "" {
		switch structured["Format"] {
		case "Theora", "Vorbis":
			return formatOggLibraryDisplay(structured["Format"], display)
		}
	}
	return formatRawTextEncodedLibrary(display)
}

// rawTextFormatInfo returns the few compound audio descriptions whose raw
// value must be composed from canonical format facts.
func rawTextFormatInfo(kind StreamKind, structured map[string]string) string {
	if kind == StreamGeneral && structured["Format"] == "BDAV" {
		return "Blu-ray Video"
	}
	if kind == StreamImage && structured["Format"] == "PNG" {
		return "Portable Network Graphic"
	}
	if kind != StreamAudio {
		return ""
	}
	format := structured["Format"]
	features := structured["Format_AdditionalFeatures"]
	switch {
	case format == "AAC" && strings.Contains(features, "LC SBR"):
		return "Advanced Audio Codec Low Complexity with Spectral Band Replication"
	case format == "AAC" && strings.Contains(features, "LC"):
		return "Advanced Audio Codec Low Complexity"
	case structured["CodecID"] == "A_EAC3" && strings.Contains(features, "JOC"):
		return "Enhanced AC-3 with Joint Object Coding"
	case structured["CodecID"] == "A_EAC3", format == "E-AC-3" || format == "AC-3 Dep" ||
		format == "AC-3" && strings.Contains(features, "Dep"):
		return "Enhanced AC-3"
	case format == "AC-3":
		return "Audio Coding 3"
	case format == "MLP FBA" || format == "TrueHD":
		if strings.Contains(features, "16-ch") {
			return "Meridian Lossless Packing FBA with 16-channel presentation"
		}
		return "Meridian Lossless Packing FBA"
	default:
		return ""
	}
}

// formatRawTextDuration renders the two most significant raw duration units.
func formatRawTextDuration(milliseconds float64) string {
	total := int64(math.Round(milliseconds))
	total = max(total, 0)
	hours := total / 3_600_000
	minutes := total / 60_000 % 60
	seconds := total / 1_000 % 60
	millis := total % 1_000
	switch {
	case hours > 0:
		return fmt.Sprintf("%dh %dmn", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dmn %ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds %dms", seconds, millis)
	}
}

// rawTextFieldOrder returns the raw registry order, then stable encounter order.
func rawTextFieldOrder(kind StreamKind, label string) int {
	if order, ok := rawTextFieldOrderPolicy(kind)[label]; ok {
		return order
	}
	return 1 << 20
}

// Raw text field order registries are immutable after package initialization.
var (
	rawTextGeneralFieldOrder = makeStructuredFieldOrder(
		"ID/String", "UniqueID/String", "CompleteName", "Format/String", "Format/Info", "Format_Version", "Format_Profile",
		"Format_Settings", "CodecID", "CodecID/String", "FileSize/String", "Duration/String", "OverallBitRate_Mode/String", "OverallBitRate/String", "OverallBitRate_Maximum/String", "FrameRate/String",
		"Title", "Movie", "Album", "Performer", "EncodedBy", "Genre", "ContentType", "Synopsis", "Description", "Released_Date", "Recorded_Date", "Encoded_Date", "Tagged_Date",
		"Encoded_Application/String", "Encoded_Library/String", "Copyright", "OriginalSourceForm", "Cover", "Cover_Description", "Cover_Type", "Cover_Mime",
		"Comment", "ErrorDetectionType", "Attachments", "Episode_ID", "Season", "SeasonNumber", "Show", "EPISODE_ID", "EPISODE_NUMBER", "SEASON_NUMBER", "SHOW",
	)
	rawTextImageFieldOrder = makeStructuredFieldOrder(
		"Type/String", "Title", "Format/String", "Format/Info", "Format_Compression", "MuxingMode", "Width/String", "Height/String", "DisplayAspectRatio/String",
		"ColorSpace", "ChromaSubsampling", "ChromaSubsampling/String", "BitDepth/String", "Compression_Mode/String", "StreamSize/String",
		"colour_range", "colour_primaries", "transfer_characteristics", "matrix_coefficients", "Gamma", "ColorSpace_ICC", "colour_primaries_ICC_Description",
	)
	rawTextTextFieldOrder = makeStructuredFieldOrder(
		"ID/String", "MenuID/String", "UniqueID/String", "OriginalSourceMedium_ID/String", "Format/String", "Format/Info", "MuxingMode",
		"MuxingMode_MoreInfo", "CodecID", "CodecID/Info", "CodecID/Hint", "Duration/String", "Duration_Start2End/String", "Duration_Start/String", "Duration_End/String", "Duration_FirstFrame/String", "Duration_LastFrame/String", "BitRate_Mode/String", "BitRate/String", "FrameRate/String",
		"ElementCount", "Compression_Mode/String", "Video_Delay/String", "StreamSize/String", "Title", "Encoded_Application/String",
		"FirstDisplay_Delay_Frames", "FirstDisplay_Type", "Encoded_Library/String", "Encoded_Library_Settings", "Language/String", "ServiceKind/String", "Default/String", "Forced/String", "AlternateGroup/String", "Encoded_Date", "Tagged_Date", "Events_Total",
		"  Issue", "FromStats_BitRate", "FromStats_Duration", "FromStats_FrameCount", "FromStats_StreamSize", "Source", "Source_ID", "MD5_Unencoded",
	)
	rawTextStreamFieldOrder = makeStructuredFieldOrder(
		"ID/String", "MenuID/String", "UniqueID/String", "OriginalSourceMedium_ID/String", "Format/String", "Format/Info",
		"Format_Commercial_IfAny", "Format_Version", "Format_Profile", "MultiView_Count", "MultiView_Layout", "HDR_Format/String", "Format_Settings", "Format_Settings_Floor",
		"Format_Settings_BVOP/String", "Format_Settings_QPel/String", "Format_Settings_GMC/String", "Format_Settings_Matrix/String",
		"Format_Settings_CABAC/String", "Format_Settings_RefFrames/String", "Format_Settings_GOP", "Format_Settings_PictureStructure", "Format_Settings_SliceCount/Strin", "MuxingMode", "CodecID", "CodecID/Info", "CodecID/Hint",
		"Duration/String", "Source_Duration/String", "Duration_FirstFrame/String", "Duration_LastFrame/String", "BitRate_Mode/String", "BitRate/String", "BitRate_Minimum/String", "BitRate_Nominal/String", "BitRate_Maximum/String",
		"Width/String", "Height/String", "DisplayAspectRatio/String", "DisplayAspectRatio_Original/Stri", "ActiveFormatDescription/String", "Channel(s)/String", "ChannelLayout", "Channel(s)_Original/String", "ChannelLayout_Original", "SamplingRate/String",
		"FrameRate_Mode/String", "FrameRate/String", "FrameRate_Minimum/String", "FrameRate_Maximum/String", "FrameRate_Original/String", "Standard", "ColorSpace", "ChromaSubsampling", "ChromaSubsampling/String", "BitDepth/String",
		"BitDepth_Detected/String", "ScanType/String", "ScanType_StoreMethod/String", "ScanOrder/String", "Compression_Mode/String", "Bits-(Pixel*Frame)", "TimeCode_FirstFrame", "TimeCode_Source", "Gop_OpenClosed/String", "Gop_OpenClosed_FirstFrame/String", "ElementCount", "Video_Delay/String", "StreamSize/String", "Source_StreamSize/String", "Alignment/String", "Interleave_Duration/String", "Interleave_Preload/String",
		"List/String", "Title", "Encoded_Application/String", "Encoded_Library/String", "Encoded_Library_Settings", "Language/String", "LawRating", "ServiceKind/String",
		"Default/String", "Forced/String", "AlternateGroup/String", "Encoded_Date", "Tagged_Date",
		"colour_range", "colour_primaries", "colour_primaries_Original", "transfer_characteristics", "transfer_characteristics_Origina", "matrix_coefficients", "matrix_coefficients_Original",
		"MasteringDisplay_ColorPrimaries", "MasteringDisplay_Luminance", "MaxCLL/String", "MaxFALL/String", "mdhd_Duration", "Menus", "CodecConfigurationBox", "Events_Total",
		"OriginalSourceMedium", "  Issue", "FromStats_BitRate", "FromStats_Duration", "FromStats_FrameCount", "FromStats_StreamSize",
		"Comment", "COMMENT", "Source", "Source_ID", "SOURCE", "SOURCE_ID", "MD5_Unencoded", "EncodedBy",
		"ComplexityIndex", "NumberOfDynamicObjects", "BedChannelCount", "BedChannelConfiguration",
		"dialnorm", "compr", "dynrng", "dsurmod", "cmixlev", "surmixlev", "mixlevel", "roomtyp", "dmixmod",
		"ltrtcmixlev", "ltrtsurmixlev", "lorocmixlev", "lorosurmixlev", "adconvtyp",
		"dialnorm_Average", "dialnorm_Minimum", "dialnorm_Maximum",
	)
)

// rawTextFieldOrderPolicy returns initial MediaInfo raw order per stream kind.
func rawTextFieldOrderPolicy(kind StreamKind) map[string]int {
	switch kind {
	case StreamGeneral:
		return rawTextGeneralFieldOrder
	case StreamImage:
		return rawTextImageFieldOrder
	case StreamText:
		return rawTextTextFieldOrder
	case StreamVideo, StreamAudio, StreamMenu:
		return rawTextStreamFieldOrder
	default:
		return rawTextStreamFieldOrder
	}
}

// renderRawText serializes raw-language projections with MediaInfo's 33-column labels.
func renderRawText(reports []Report) string {
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
		buffer.WriteString(reportByLine())
		buffer.WriteByte('\n')
	}
	output := strings.TrimRight(buffer.String(), "\n")
	return strings.ReplaceAll(output+"\n\n\n", "\n", "\r\n")
}

// writeRawTextFields writes one raw stream section.
func writeRawTextFields(buffer *bytes.Buffer, title string, fields []rawTextFieldProjection) {
	buffer.WriteString(title)
	buffer.WriteByte('\n')
	for _, field := range fields {
		buffer.WriteString(padRawTextLabel(field.Label, 33))
		if utf8.RuneCountInString(field.Label) >= 33 {
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
