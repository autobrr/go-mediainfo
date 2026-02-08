package mediainfo

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type jsonKV struct {
	Key string
	Val string
	Raw bool
}

func buildJSONMedia(report Report) jsonMediaOut {
	tracks := make([]jsonTrackOut, 0, len(report.Streams)+1)
	tracks = append(tracks, jsonTrackOut{Fields: buildJSONGeneralFields(report)})
	containerFormat := findField(report.General.Fields, "Format")
	sorted := orderTracks(report.Streams)
	forEachStreamWithKindIndex(sorted, func(stream Stream, index, total, order int) {
		typeOrder := 0
		if total > 1 {
			typeOrder = index
		}
		tracks = append(tracks, jsonTrackOut{Fields: buildJSONStreamFields(stream, order, typeOrder, containerFormat)})
	})
	return jsonMediaOut{Ref: report.Ref, Tracks: tracks}
}

func buildJSONGeneralFields(report Report) []jsonKV {
	fields := []jsonKV{{Key: "@type", Val: string(StreamGeneral)}}
	counts := countStreams(report.Streams)
	for _, key := range []struct {
		Name  string
		Count int
	}{
		{Name: "VideoCount", Count: counts[StreamVideo]},
		{Name: "AudioCount", Count: counts[StreamAudio]},
		{Name: "TextCount", Count: counts[StreamText]},
		{Name: "ImageCount", Count: counts[StreamImage]},
		{Name: "MenuCount", Count: counts[StreamMenu]},
	} {
		if key.Count > 0 {
			fields = append(fields, jsonKV{Key: key.Name, Val: strconv.Itoa(key.Count)})
		}
	}
	if report.Ref != "" {
		ext := strings.TrimPrefix(filepath.Ext(report.Ref), ".")
		if ext != "" {
			fields = append(fields, jsonKV{Key: "FileExtension", Val: ext})
		}
		if size := fileSizeBytes(report.Ref); size > 0 {
			fields = append(fields, jsonKV{Key: "FileSize", Val: strconv.FormatInt(size, 10)})
		}
		if createdUTC, createdLocal, modifiedUTC, modifiedLocal, ok := fileTimes(report.Ref); ok {
			if createdUTC != "" {
				fields = append(fields, jsonKV{Key: "File_Created_Date", Val: createdUTC})
			}
			if createdLocal != "" {
				fields = append(fields, jsonKV{Key: "File_Created_Date_Local", Val: createdLocal})
			}
			fields = append(fields, jsonKV{Key: "File_Modified_Date", Val: modifiedUTC})
			fields = append(fields, jsonKV{Key: "File_Modified_Date_Local", Val: modifiedLocal})
		}
	}
	fields = append(fields, mapStreamFieldsToJSON(StreamGeneral, report.General.Fields)...)
	fields = applyJSONExtras(fields, report.General.JSON, report.General.JSONRaw)
	return sortJSONFields(StreamGeneral, fields)
}

func buildJSONStreamFields(stream Stream, order int, typeOrder int, containerFormat string) []jsonKV {
	fields := []jsonKV{{Key: "@type", Val: string(stream.Kind)}}
	if typeOrder > 0 {
		fields = append(fields, jsonKV{Key: "@typeorder", Val: strconv.Itoa(typeOrder)})
	}
	if !stream.JSONSkipStreamOrder {
		fields = append(fields, jsonKV{Key: "StreamOrder", Val: strconv.Itoa(order)})
	}
	fields = append(fields, mapStreamFieldsToJSON(stream.Kind, stream.Fields)...)
	fields = applyJSONExtras(fields, stream.JSON, stream.JSONRaw)
	fields = normalizeContainerComputedJSON(stream.Kind, fields, containerFormat)
	if !stream.JSONSkipComputed {
		fields = append(fields, buildJSONComputedFields(stream.Kind, fields, containerFormat)...)
	}
	return sortJSONFields(stream.Kind, fields)
}

func normalizeContainerComputedJSON(kind StreamKind, fields []jsonKV, containerFormat string) []jsonKV {
	if kind == StreamVideo && strings.EqualFold(containerFormat, "Matroska") {
		width, _ := strconv.ParseFloat(jsonFieldValue(fields, "Width"), 64)
		height, _ := strconv.ParseFloat(jsonFieldValue(fields, "Height"), 64)
		if width > 0 && height > 0 {
			// Official mediainfo JSON reports the numeric ratio even when the text field snaps to a common value.
			fields = setJSONField(fields, "DisplayAspectRatio", formatJSONFloat(width/height))
		}
	}
	return fields
}

func mapStreamFieldsToJSON(kind StreamKind, fields []Field) []jsonKV {
	out := make([]jsonKV, 0, len(fields))
	var extras []jsonKV
	for _, field := range fields {
		switch field.Name {
		case "CompleteName_Last":
			out = append(out, jsonKV{Key: "CompleteName_Last", Val: field.Value})
		case "Format":
			format, extra := splitAACFormat(field.Value)
			out = append(out, jsonKV{Key: "Format", Val: format})
			if extra != "" {
				out = append(out, jsonKV{Key: "Format_AdditionalFeatures", Val: extra})
			}
		case "Format profile":
			profile, level := splitProfileLevel(field.Value)
			out = append(out, jsonKV{Key: "Format_Profile", Val: profile})
			if level != "" {
				out = append(out, jsonKV{Key: "Format_Level", Val: level})
			}
		case "Format tier":
			out = append(out, jsonKV{Key: "Format_Tier", Val: field.Value})
		case "ID":
			out = append(out, jsonKV{Key: "ID", Val: field.Value})
		case "Menu ID":
			out = append(out, jsonKV{Key: "MenuID", Val: field.Value})
		case "Unique ID":
			value := strings.TrimSpace(field.Value)
			if idx := strings.IndexAny(value, " ("); idx >= 0 {
				value = strings.TrimSpace(value[:idx])
			}
			if value != "" {
				out = append(out, jsonKV{Key: "UniqueID", Val: value})
			}
		case "Format version":
			if version := extractVersionNumber(field.Value); version != "" {
				out = append(out, jsonKV{Key: "Format_Version", Val: version})
			}
		case "Format settings, CABAC":
			out = append(out, jsonKV{Key: "Format_Settings_CABAC", Val: field.Value})
		case "Format settings, BVOP":
			out = append(out, jsonKV{Key: "Format_Settings_BVOP", Val: field.Value})
		case "Format settings, QPel":
			out = append(out, jsonKV{Key: "Format_Settings_QPel", Val: field.Value})
		case "Format settings, GMC":
			value := field.Value
			if strings.HasPrefix(value, "No") {
				value = "0"
			} else if parsed := extractLeadingNumber(value); parsed != "" {
				value = parsed
			}
			out = append(out, jsonKV{Key: "Format_Settings_GMC", Val: value})
		case "Format settings, Matrix":
			out = append(out, jsonKV{Key: "Format_Settings_Matrix", Val: field.Value})
		case "Format settings, GOP":
			out = append(out, jsonKV{Key: "Format_Settings_GOP", Val: field.Value})
		case "Format settings, Reference frames":
			out = append(out, jsonKV{Key: "Format_Settings_RefFrames", Val: extractLeadingNumber(field.Value)})
		case "Format settings":
			if kind == StreamGeneral {
				out = append(out, jsonKV{Key: "Format_Settings", Val: field.Value})
			}
		case "Codec ID":
			id, compat := splitCodecID(field.Value)
			out = append(out, jsonKV{Key: "CodecID", Val: id})
			if compat != "" && kind == StreamGeneral {
				out = append(out, jsonKV{Key: "CodecID_Compatible", Val: compat})
			}
		case "Commercial name":
			out = append(out, jsonKV{Key: "Format_Commercial_IfAny", Val: field.Value})
		case "Muxing mode":
			out = append(out, jsonKV{Key: "MuxingMode", Val: field.Value})
		case "Muxing mode, more info":
			out = append(out, jsonKV{Key: "MuxingMode_MoreInfo", Val: field.Value})
		case "Codec configuration box":
			extras = append(extras, jsonKV{Key: "CodecConfigurationBox", Val: field.Value})
		case "Duration":
			if seconds, ok := parseDurationSeconds(field.Value); ok {
				out = append(out, jsonKV{Key: "Duration", Val: formatJSONSeconds(seconds)})
			}
		case "Source duration":
			if seconds, ok := parseDurationSeconds(field.Value); ok {
				out = append(out, jsonKV{Key: "Source_Duration", Val: formatJSONSeconds(seconds)})
			}
		case "Source_Duration_LastFrame":
			if seconds, ok := parseDurationSeconds(field.Value); ok {
				out = append(out, jsonKV{Key: "Source_Duration_LastFrame", Val: formatJSONSeconds(seconds)})
			}
		case "Bit rate mode":
			out = append(out, jsonKV{Key: "BitRate_Mode", Val: mapBitrateMode(field.Value)})
		case "Overall bit rate mode":
			out = append(out, jsonKV{Key: "OverallBitRate_Mode", Val: mapBitrateMode(field.Value)})
		case "Bit rate":
			if bps, ok := parseBitrateBps(field.Value); ok {
				out = append(out, jsonKV{Key: "BitRate", Val: strconv.FormatInt(bps, 10)})
			}
		case "Nominal bit rate":
			if bps, ok := parseBitrateBps(field.Value); ok {
				out = append(out, jsonKV{Key: "BitRate_Nominal", Val: strconv.FormatInt(bps, 10)})
			}
		case "Maximum bit rate":
			if bps, ok := parseBitrateBps(field.Value); ok {
				out = append(out, jsonKV{Key: "BitRate_Maximum", Val: strconv.FormatInt(bps, 10)})
			}
		case "Overall bit rate":
			if bps, ok := parseBitrateBps(field.Value); ok {
				out = append(out, jsonKV{Key: "OverallBitRate", Val: strconv.FormatInt(bps, 10)})
			}
		case "Frame rate":
			if value, ok := parseFloatValue(field.Value); ok {
				out = append(out, jsonKV{Key: "FrameRate", Val: formatJSONFloat(value)})
			}
			if num, den, ok := parseFrameRateRatio(field.Value); ok {
				out = append(out, jsonKV{Key: "FrameRate_Num", Val: strconv.Itoa(num)})
				out = append(out, jsonKV{Key: "FrameRate_Den", Val: strconv.Itoa(den)})
			}
		case "Frame rate mode":
			out = append(out, jsonKV{Key: "FrameRate_Mode", Val: mapFrameRateMode(field.Value)})
		case "Width":
			out = append(out, jsonKV{Key: "Width", Val: extractLeadingNumber(field.Value)})
		case "Height":
			out = append(out, jsonKV{Key: "Height", Val: extractLeadingNumber(field.Value)})
		case "Display aspect ratio":
			if value, ok := parseRatioFloat(field.Value); ok {
				out = append(out, jsonKV{Key: "DisplayAspectRatio", Val: formatJSONFloat(value)})
			}
		case "Chroma subsampling":
			out = append(out, jsonKV{Key: "ChromaSubsampling", Val: field.Value})
		case "Chroma subsampling position":
			out = append(out, jsonKV{Key: "ChromaSubsampling_Position", Val: field.Value})
		case "Color space":
			out = append(out, jsonKV{Key: "ColorSpace", Val: field.Value})
		case "Bit depth":
			out = append(out, jsonKV{Key: "BitDepth", Val: extractLeadingNumber(field.Value)})
		case "Scan type":
			out = append(out, jsonKV{Key: "ScanType", Val: field.Value})
		case "Scan order":
			out = append(out, jsonKV{Key: "ScanOrder", Val: field.Value})
		case "Stream size":
			if bytes, ok := parseSizeBytes(field.Value); ok {
				out = append(out, jsonKV{Key: "StreamSize", Val: strconv.FormatInt(bytes, 10)})
			}
		case "Source stream size":
			if bytes, ok := parseSizeBytes(field.Value); ok {
				out = append(out, jsonKV{Key: "Source_StreamSize", Val: strconv.FormatInt(bytes, 10)})
			}
		case "Writing application":
			out = append(out, jsonKV{Key: "Encoded_Application", Val: field.Value})
		case "Encoded date":
			out = append(out, jsonKV{Key: "Encoded_Date", Val: field.Value})
		case "Tagged date":
			out = append(out, jsonKV{Key: "Tagged_Date", Val: field.Value})
		case "Format settings, Endianness":
			out = append(out, jsonKV{Key: "Format_Settings_Endianness", Val: field.Value})
		case "Writing library":
			encoded := field.Value
			if strings.HasPrefix(encoded, "x264 ") && !strings.HasPrefix(encoded, "x264 - ") {
				encoded = "x264 - " + strings.TrimPrefix(encoded, "x264 ")
			}
			out = append(out, jsonKV{Key: "Encoded_Library", Val: encoded})
			if name, version := splitEncodedLibrary(encoded); name != "" {
				out = append(out, jsonKV{Key: "Encoded_Library_Name", Val: name})
				if version != "" {
					out = append(out, jsonKV{Key: "Encoded_Library_Version", Val: version})
				}
			}
		case "Encoding settings":
			out = append(out, jsonKV{Key: "Encoded_Library_Settings", Val: field.Value})
		case "Language":
			out = append(out, jsonKV{Key: "Language", Val: field.Value})
		case "Title":
			out = append(out, jsonKV{Key: "Title", Val: field.Value})
		case "Movie name":
			out = append(out, jsonKV{Key: "Title", Val: field.Value})
			out = append(out, jsonKV{Key: "Movie", Val: field.Value})
		case "Channel(s)":
			out = append(out, jsonKV{Key: "Channels", Val: extractLeadingNumber(field.Value)})
		case "Channel layout":
			out = append(out, jsonKV{Key: "ChannelLayout", Val: field.Value})
		case "Sampling rate":
			if hz, ok := parseSampleRate(field.Value); ok {
				out = append(out, jsonKV{Key: "SamplingRate", Val: strconv.FormatInt(hz, 10)})
			}
		case "Compression mode":
			out = append(out, jsonKV{Key: "Compression_Mode", Val: field.Value})
		case "Time code of first frame":
			out = append(out, jsonKV{Key: "TimeCode_FirstFrame", Val: field.Value})
		case "Time code source":
			out = append(out, jsonKV{Key: "TimeCode_Source", Val: field.Value})
		case "GOP, Open/Closed":
			out = append(out, jsonKV{Key: "Gop_OpenClosed", Val: field.Value})
		case "GOP, Open/Closed of first frame":
			out = append(out, jsonKV{Key: "Gop_OpenClosed_FirstFrame", Val: field.Value})
		case "Standard":
			out = append(out, jsonKV{Key: "Standard", Val: field.Value})
		case "Default":
			out = append(out, jsonKV{Key: "Default", Val: field.Value})
		case "Forced":
			out = append(out, jsonKV{Key: "Forced", Val: field.Value})
		case "Alternate group":
			out = append(out, jsonKV{Key: "AlternateGroup", Val: field.Value})
		case "ErrorDetectionType":
			extras = append(extras, jsonKV{Key: "ErrorDetectionType", Val: field.Value})
		case "Service kind":
			out = append(out, jsonKV{Key: "ServiceKind", Val: field.Value})
		case "Service name":
			out = append(out, jsonKV{Key: "ServiceName", Val: field.Value})
		case "Service provider":
			out = append(out, jsonKV{Key: "ServiceProvider", Val: field.Value})
		case "Service type":
			out = append(out, jsonKV{Key: "ServiceType", Val: field.Value})
		}
	}
	if len(extras) > 0 {
		out = append(out, jsonKV{Key: "extra", Val: renderJSONObject(extras, false), Raw: true})
	}
	return out
}

func buildJSONComputedFields(kind StreamKind, fields []jsonKV, containerFormat string) []jsonKV {
	out := []jsonKV{}
	format := jsonFieldValue(fields, "Format")
	duration, _ := strconv.ParseFloat(jsonFieldValue(fields, "Duration"), 64)
	frameRate, _ := strconv.ParseFloat(jsonFieldValue(fields, "FrameRate"), 64)
	sampleRate, _ := strconv.ParseFloat(jsonFieldValue(fields, "SamplingRate"), 64)
	channels := jsonFieldValue(fields, "Channels")
	codecID := strings.ToLower(jsonFieldValue(fields, "CodecID"))

	if kind == StreamVideo && duration > 0 && frameRate > 0 {
		if jsonFieldValue(fields, "FrameCount") == "" {
			frameCount := int(math.Round(duration * frameRate))
			out = append(out, jsonKV{Key: "FrameCount", Val: strconv.Itoa(frameCount)})
		}
		// MediaInfo doesn't emit FrameRate_Num/Den for Matroska; keep this for MPEG-4 only.
		if containerFormat == "MPEG-4" && jsonFieldValue(fields, "FrameRate_Num") == "" && jsonFieldValue(fields, "FrameRate_Den") == "" {
			num, den := rationalizeFrameRate(frameRate)
			if num > 0 && den > 0 {
				out = append(out, jsonKV{Key: "FrameRate_Num", Val: strconv.Itoa(num)})
				out = append(out, jsonKV{Key: "FrameRate_Den", Val: strconv.Itoa(den)})
			}
		}
	}

	if kind == StreamAudio {
		if channels != "" && jsonFieldValue(fields, "ChannelPositions") == "" {
			if pos := channelPositionsFromCount(channels); pos != "" {
				out = append(out, jsonKV{Key: "ChannelPositions", Val: pos})
			}
		}
		if strings.EqualFold(format, "AAC") {
			if jsonFieldValue(fields, "SamplesPerFrame") == "" {
				out = append(out, jsonKV{Key: "SamplesPerFrame", Val: "1024"})
			}
			if duration > 0 && sampleRate > 0 && jsonFieldValue(fields, "SamplingCount") == "" {
				samples := int(math.Round(duration * sampleRate))
				out = append(out, jsonKV{Key: "SamplingCount", Val: strconv.Itoa(samples)})
			}
			if duration > 0 && sampleRate > 0 && jsonFieldValue(fields, "FrameCount") == "" && strings.HasPrefix(codecID, "mp4a") {
				frameCount := int(math.Round(duration * sampleRate / 1024.0))
				out = append(out, jsonKV{Key: "FrameCount", Val: strconv.Itoa(frameCount)})
			}
			if src := jsonFieldValue(fields, "Source_Duration"); src != "" && jsonFieldValue(fields, "Source_FrameCount") == "" {
				if srcDur, err := strconv.ParseFloat(src, 64); err == nil && sampleRate > 0 {
					srcFrames := int(math.Round(srcDur * sampleRate / 1024.0))
					out = append(out, jsonKV{Key: "Source_FrameCount", Val: strconv.Itoa(srcFrames)})
				}
			}
		}
	}

	if kind == StreamVideo {
		width, _ := strconv.ParseFloat(jsonFieldValue(fields, "Width"), 64)
		height, _ := strconv.ParseFloat(jsonFieldValue(fields, "Height"), 64)
		if width > 0 && height > 0 {
			if jsonFieldValue(fields, "Sampled_Width") == "" {
				out = append(out, jsonKV{Key: "Sampled_Width", Val: trimFloat(width)})
			}
			if jsonFieldValue(fields, "Sampled_Height") == "" {
				out = append(out, jsonKV{Key: "Sampled_Height", Val: trimFloat(height)})
			}
			if jsonFieldValue(fields, "PixelAspectRatio") == "" {
				displayAspect, err := strconv.ParseFloat(jsonFieldValue(fields, "DisplayAspectRatio"), 64)
				if err == nil && displayAspect > 0 {
					pixelAspect := displayAspect / (width / height)
					out = append(out, jsonKV{Key: "PixelAspectRatio", Val: formatJSONFloat(pixelAspect)})
				}
			}
		}
	}

	return out
}

func jsonFieldValue(fields []jsonKV, key string) string {
	for _, field := range fields {
		if field.Key == key {
			return field.Val
		}
	}
	return ""
}

func setJSONField(fields []jsonKV, key, value string) []jsonKV {
	for i := range fields {
		if fields[i].Key == key {
			fields[i].Val = value
			return fields
		}
	}
	return append(fields, jsonKV{Key: key, Val: value})
}

func applyJSONExtras(fields []jsonKV, extras map[string]string, rawExtras map[string]string) []jsonKV {
	if len(extras) == 0 && len(rawExtras) == 0 {
		return fields
	}
	index := map[string]int{}
	for i, field := range fields {
		index[field.Key] = i
	}
	keys := make([]string, 0, len(extras))
	for key := range extras {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := extras[key]
		if i, ok := index[key]; ok {
			fields[i].Val = value
			continue
		}
		fields = append(fields, jsonKV{Key: key, Val: value})
	}
	if len(rawExtras) > 0 {
		rawKeys := make([]string, 0, len(rawExtras))
		for key := range rawExtras {
			rawKeys = append(rawKeys, key)
		}
		sort.Strings(rawKeys)
		for _, key := range rawKeys {
			value := rawExtras[key]
			if i, ok := index[key]; ok {
				fields[i].Val = value
				fields[i].Raw = true
				continue
			}
			fields = append(fields, jsonKV{Key: key, Val: value, Raw: true})
		}
	}
	return fields
}

func channelPositionsFromCount(value string) string {
	switch value {
	case "1":
		return "Front: C"
	case "2":
		return "Front: L R"
	case "6":
		return "Front: L C R, Side: L R, LFE"
	default:
		return ""
	}
}

func rationalizeFrameRate(rate float64) (int, int) {
	if rate <= 0 {
		return 0, 0
	}
	common := []struct {
		num int
		den int
		val float64
	}{
		{num: 24, den: 1, val: 24.0},
		{num: 25, den: 1, val: 25.0},
		{num: 30, den: 1, val: 30.0},
		{num: 50, den: 1, val: 50.0},
		{num: 60, den: 1, val: 60.0},
		{num: 24000, den: 1001, val: 24000.0 / 1001.0},
		{num: 30000, den: 1001, val: 30000.0 / 1001.0},
		{num: 60000, den: 1001, val: 60000.0 / 1001.0},
	}
	for _, entry := range common {
		if math.Abs(rate-entry.val) < 0.0005 {
			return entry.num, entry.den
		}
	}
	return int(math.Round(rate)), 1
}

func parseFrameRateRatio(value string) (int, int, bool) {
	open := strings.IndexByte(value, '(')
	if open < 0 {
		return 0, 0, false
	}
	close := strings.IndexByte(value[open:], ')')
	if close < 0 {
		return 0, 0, false
	}
	close = open + close
	inside := strings.TrimSpace(value[open+1 : close])
	parts := strings.SplitN(inside, "/", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	num, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	den, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		return 0, 0, false
	}
	return num, den, true
}

func countStreams(streams []Stream) map[StreamKind]int {
	counts := map[StreamKind]int{}
	for _, stream := range streams {
		counts[stream.Kind]++
	}
	return counts
}

func splitCodecID(value string) (string, string) {
	parts := strings.SplitN(value, "(", 2)
	id := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return id, ""
	}
	compat := strings.TrimSuffix(strings.TrimSpace(parts[1]), ")")
	return id, compat
}

func splitProfileLevel(value string) (string, string) {
	parts := strings.SplitN(value, "@", 2)
	profile := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return profile, ""
	}
	level := strings.TrimPrefix(strings.TrimSpace(parts[1]), "L")
	return profile, level
}

func splitAACFormat(value string) (string, string) {
	if after, ok := strings.CutPrefix(value, "AAC "); ok {
		return "AAC", strings.TrimSpace(after)
	}
	return value, ""
}

func extractVersionNumber(value string) string {
	tokens := strings.Fields(value)
	for i := len(tokens) - 1; i >= 0; i-- {
		if num := extractLeadingNumber(tokens[i]); num != "" {
			return num
		}
	}
	return extractLeadingNumber(value)
}

func splitEncodedLibrary(value string) (string, string) {
	if strings.HasPrefix(value, "x264") {
		trimmed := strings.TrimPrefix(value, "x264 - ")
		trimmed = strings.TrimPrefix(trimmed, "x264 ")
		return "x264", strings.TrimSpace(trimmed)
	}
	return "", ""
}

func parseDurationSeconds(value string) (float64, bool) {
	if value == "" {
		return 0, false
	}
	sign := 1.0
	if strings.HasPrefix(value, "-") {
		sign = -1
		value = strings.TrimPrefix(value, "-")
	}
	fields := strings.Fields(value)
	if len(fields) == 1 {
		if ms, err := strconv.ParseFloat(fields[0], 64); err == nil {
			return sign * ms / 1000, true
		}
	}
	var totalMs float64
	for i := 0; i+1 < len(fields); i += 2 {
		num, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			continue
		}
		switch fields[i+1] {
		case "ms":
			totalMs += num
		case "s":
			totalMs += num * 1000
		case "min":
			totalMs += num * 60 * 1000
		case "h":
			totalMs += num * 60 * 60 * 1000
		}
	}
	if totalMs == 0 {
		return 0, false
	}
	return sign * totalMs / 1000, true
}

func formatJSONSeconds(value float64) string {
	return fmt.Sprintf("%.3f", value)
}

func formatJSONSeconds6(value float64) string {
	return fmt.Sprintf("%.6f", value)
}

func parseBitrateBps(value string) (int64, bool) {
	tokens := strings.Fields(value)
	if len(tokens) < 2 {
		return 0, false
	}
	unit := tokens[len(tokens)-1]
	number := strings.Join(tokens[:len(tokens)-1], "")
	rate, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, false
	}
	switch unit {
	case "kb/s":
		return int64(rate * 1000), true
	case "Mb/s":
		return int64(rate * 1000 * 1000), true
	default:
		return 0, false
	}
}

func parseSizeBytes(value string) (int64, bool) {
	tokens := strings.Fields(value)
	if len(tokens) < 2 {
		return 0, false
	}
	last := len(tokens)
	for i, token := range tokens {
		if strings.HasPrefix(token, "(") {
			last = i
			break
		}
	}
	if last < 2 {
		return 0, false
	}
	unit := tokens[last-1]
	number := strings.Join(tokens[:last-1], "")
	size, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, false
	}
	switch unit {
	case "B":
		return int64(size), true
	case "KiB":
		return int64(size * 1024), true
	case "MiB":
		return int64(size * 1024 * 1024), true
	case "GiB":
		return int64(size * 1024 * 1024 * 1024), true
	default:
		return 0, false
	}
}

func parseFloatValue(value string) (float64, bool) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0, false
	}
	number := strings.ReplaceAll(fields[0], " ", "")
	parsed, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func parseRatioFloat(value string) (float64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, false
	}
	num, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, false
	}
	den, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || den == 0 {
		return 0, false
	}
	return num / den, true
}

func parseSampleRate(value string) (int64, bool) {
	fields := strings.Fields(value)
	if len(fields) < 2 {
		return 0, false
	}
	number := strings.ReplaceAll(fields[0], " ", "")
	rate, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, false
	}
	switch fields[1] {
	case "kHz":
		return int64(rate * 1000), true
	case "Hz":
		return int64(rate), true
	default:
		return 0, false
	}
}

func formatJSONFloat(value float64) string {
	return fmt.Sprintf("%.3f", value)
}

func trimFloat(value float64) string {
	if value == math.Trunc(value) {
		return fmt.Sprintf("%.0f", value)
	}
	return formatJSONFloat(value)
}

func extractLeadingNumber(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var buf strings.Builder
	for i, r := range value {
		if (r >= '0' && r <= '9') || r == ' ' || r == '.' {
			buf.WriteRune(r)
			continue
		}
		if i == 0 && r == '-' {
			buf.WriteRune(r)
			continue
		}
		break
	}
	if buf.Len() == 0 {
		return ""
	}
	return strings.ReplaceAll(buf.String(), " ", "")
}

func mapBitrateMode(value string) string {
	switch strings.ToLower(value) {
	case "variable":
		return "VBR"
	case "constant":
		return "CBR"
	default:
		return value
	}
}

func mapFrameRateMode(value string) string {
	switch strings.ToLower(value) {
	case "constant":
		return "CFR"
	case "variable":
		return "VFR"
	default:
		return value
	}
}

func fileSizeBytes(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
