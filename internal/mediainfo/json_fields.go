package mediainfo

import (
	"fmt"
	"os"
	"path/filepath"
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
	sorted := orderTracks(report.Streams)
	for i, stream := range sorted {
		tracks = append(tracks, jsonTrackOut{Fields: buildJSONStreamFields(stream, i)})
	}
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
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(report.Ref)), ".")
		if ext != "" {
			fields = append(fields, jsonKV{Key: "FileExtension", Val: ext})
		}
		if size := fileSizeBytes(report.Ref); size > 0 {
			fields = append(fields, jsonKV{Key: "FileSize", Val: strconv.FormatInt(size, 10)})
		}
		if createdUTC, createdLocal, ok := fileTimes(report.Ref); ok {
			fields = append(fields, jsonKV{Key: "File_Created_Date", Val: createdUTC})
			fields = append(fields, jsonKV{Key: "File_Created_Date_Local", Val: createdLocal})
			fields = append(fields, jsonKV{Key: "File_Modified_Date", Val: createdUTC})
			fields = append(fields, jsonKV{Key: "File_Modified_Date_Local", Val: createdLocal})
		}
	}
	fields = append(fields, mapStreamFieldsToJSON(StreamGeneral, report.General.Fields)...)
	return fields
}

func buildJSONStreamFields(stream Stream, order int) []jsonKV {
	fields := []jsonKV{{Key: "@type", Val: string(stream.Kind)}}
	fields = append(fields, jsonKV{Key: "StreamOrder", Val: strconv.Itoa(order)})
	fields = append(fields, mapStreamFieldsToJSON(stream.Kind, stream.Fields)...)
	return fields
}

func mapStreamFieldsToJSON(kind StreamKind, fields []Field) []jsonKV {
	out := make([]jsonKV, 0, len(fields))
	var extras []jsonKV
	for _, field := range fields {
		switch field.Name {
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
		case "Format settings, CABAC":
			out = append(out, jsonKV{Key: "Format_Settings_CABAC", Val: field.Value})
		case "Format settings, Reference frames":
			out = append(out, jsonKV{Key: "Format_Settings_RefFrames", Val: extractLeadingNumber(field.Value)})
		case "Codec ID":
			id, compat := splitCodecID(field.Value)
			out = append(out, jsonKV{Key: "CodecID", Val: id})
			if compat != "" && kind == StreamGeneral {
				out = append(out, jsonKV{Key: "CodecID_Compatible", Val: compat})
			}
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
		case "Bit depth":
			out = append(out, jsonKV{Key: "BitDepth", Val: extractLeadingNumber(field.Value)})
		case "Scan type":
			out = append(out, jsonKV{Key: "ScanType", Val: field.Value})
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
		case "Default":
			out = append(out, jsonKV{Key: "Default", Val: field.Value})
		case "Alternate group":
			out = append(out, jsonKV{Key: "AlternateGroup", Val: field.Value})
		}
	}
	if len(extras) > 0 {
		out = append(out, jsonKV{Key: "extra", Val: renderJSONObject(extras), Raw: true})
	}
	return out
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
	if strings.HasPrefix(value, "AAC ") {
		return "AAC", strings.TrimSpace(strings.TrimPrefix(value, "AAC "))
	}
	return value, ""
}

func splitEncodedLibrary(value string) (string, string) {
	if strings.HasPrefix(value, "x264") {
		trimmed := strings.TrimPrefix(value, "x264 - ")
		trimmed = strings.TrimPrefix(trimmed, "x264 ")
		parts := strings.SplitN(trimmed, " ", 2)
		if len(parts) == 2 {
			return "x264", strings.TrimSpace(parts[1])
		}
		return "x264", ""
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

func parseBitrateBps(value string) (int64, bool) {
	fields := strings.Fields(value)
	if len(fields) < 2 {
		return 0, false
	}
	number := strings.ReplaceAll(fields[0], " ", "")
	rate, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, false
	}
	unit := fields[1]
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
	fields := strings.Fields(value)
	if len(fields) < 2 {
		return 0, false
	}
	number := strings.ReplaceAll(fields[0], " ", "")
	size, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, false
	}
	switch fields[1] {
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

func extractLeadingNumber(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return strings.ReplaceAll(fields[0], " ", "")
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

func fileTimes(path string) (string, string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", "", false
	}
	mod := info.ModTime()
	utc := mod.UTC().Format("2006-01-02 15:04:05 MST")
	local := mod.Local().Format("2006-01-02 15:04:05")
	return utc, local, true
}
