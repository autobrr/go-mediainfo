package mediainfo

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// friendlyReportValue formats established facts without changing their selection
// or deriving codec timing from a rounded display string.
func friendlyReportValue(kind StreamKind, identity, display string, values map[string]string, totalSize int64, version reportTableVersion) string {
	value := values[identity]
	switch identity {
	case "ID":
		if rawTextOggStream(values) {
			if id, err := strconv.ParseUint(value, 10, 32); err == nil {
				return formatID(id)
			}
		}
	case "FrameRate", "FrameRate_Minimum", "FrameRate_Maximum":
		rate, err := strconv.ParseFloat(value, 64)
		if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return ""
		}
		if kind == StreamAudio && identity == "FrameRate" {
			if sampling, _ := strconv.ParseFloat(values["SamplingRate"], 64); sampling == rate {
				return ""
			}
			result := formatFrameRate(rate)
			if spf := validSamplesPerFrame(values["SamplesPerFrame"]); spf != "" {
				result += " (" + spf + " SPF)"
			}
			return result
		}
		if identity != "FrameRate" || values["Format"] == "Theora" {
			return formatFrameRate(rate)
		}
		return display
	case "BitRate_Maximum":
		if value == "" {
			return display
		}
		if rate, err := strconv.ParseFloat(value, 64); err == nil && rate > 0 {
			return formatBitrate(rate)
		}
		return ""
	case "FileSize", "StreamSize", "Source_StreamSize":
		if size, err := strconv.ParseInt(value, 10, 64); err == nil && size >= 0 {
			if kind == StreamText && identity == "StreamSize" && size == 0 {
				return "0.00 Byte (0%)"
			}
			if identity != "FileSize" && totalSize > 0 {
				return formatStreamSize(size, totalSize)
			}
			return formatBytes(size)
		}
	case "ScanOrder", "ScanOrder_Original", "ScanOrder_Stored":
		parts := strings.Split(firstNonEmpty(value, display), " / ")
		for i, part := range parts {
			if translated := reportTables[version].Values["Interlaced_"+part]; translated != "" {
				parts[i] = translated
			}
		}
		return strings.Join(parts, " / ")
	case "ScanType_StoreMethod":
		method := firstNonEmpty(value, display)
		key := "StoreMethod_" + method
		if fields := values["ScanType_StoreMethod_FieldsPerBlock"]; fields != "" {
			key += "_" + fields
		}
		if translated := reportTables[version].Values[key]; translated != "" {
			return translated
		}
		return method
	case "OriginalSourceMedium_ID":
		return formatOriginalSourceID(value, display)
	case "MuxingMode":
		if translated := reportTables[version].Values[value]; translated != "" {
			return translated
		}
	case "Format":
		name, _ := reportFormatPresentation(version, kind, values)
		if name != "" {
			return name
		}
	case "Format_Settings_RefFrames":
		if count, err := strconv.Atoi(value); err == nil {
			return formatReferenceFrames(count)
		}
	}
	return display
}

func validSamplesPerFrame(value string) string {
	if value == "" {
		return ""
	}
	for _, part := range strings.Split(value, " / ") {
		v, err := strconv.ParseFloat(part, 64)
		if err != nil || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return ""
		}
	}
	return value
}

type codecTableIdentity struct{ Namespace, ID string }

// reportCodecDescription looks up exact keys in their container namespace.
func reportCodecDescription(version reportTableVersion, container string, kind StreamKind, id string) codecDescription {
	namespace := ""
	switch container {
	case "Matroska", "WebM":
		namespace = "Matroska"
	case "AVI", "Wave", "Wave64":
		namespace = "Riff"
	case "MPEG-4", "QuickTime":
		namespace = "Mpeg4"
	}
	return reportTables[version].Codecs[namespace+"\t"+string(kind)+"\t"+id]
}

func storedCodecDescription(version reportTableVersion, container string, stream *storedStream, values map[string]string) codecDescription {
	identity := stream.CodecIdentity
	var result codecDescription
	if identity.Namespace != "" {
		result = reportTables[version].Codecs[identity.Namespace+"\t"+string(stream.Kind)+"\t"+identity.ID]
	} else {
		result = reportCodecDescription(version, container, stream.Kind, values["CodecID"])
	}
	result.Info = firstNonEmpty(values["CodecID/Info"], values["CodecID_Info"], result.Info)
	result.Hint = firstNonEmpty(values["CodecID/Hint"], values["CodecID_Hint"], result.Hint)
	return result
}

// reportFormatPresentation applies the reference table and the supported
// HighestFormat summaries; commercial naming remains a separate field.
func reportFormatPresentation(version reportTableVersion, kind StreamKind, values map[string]string) (name, info string) {
	name = values["Format"]
	if name == "TrueHD" {
		name = "MLP FBA"
	}
	info = reportTables[version].Formats[name]
	if name == "DTS LBR" {
		info = reportTables[version].Formats["DTS"]
	}
	if kind != StreamAudio && kind != StreamGeneral {
		return name, info
	}
	features := strings.Fields(values["Format_AdditionalFeatures"])
	profile := values["Format_Profile"]
	if len(features) == 0 {
		switch name {
		case "AAC":
			switch profile {
			case "HE-AACv2":
				features = []string{"LC", "SBR", "PS"}
			case "HE-AAC+eSBR":
				features = []string{"LC", "SBR", "eSBR"}
			case "HE-AAC":
				features = []string{"LC", "SBR"}
			case "LC", "LTP", "Main", "SSR":
				features = []string{profile}
			}
		case "DTS":
			profiles := strings.Split(profile, " / ")
			for i := len(profiles) - 1; i >= 0; i-- {
				switch profiles[i] {
				case "Core", "":
				case "Express":
					name = "DTS LBR"
				case "ES Matrix":
					features = append(features, "ES")
				case "ES Discrete", "ES Discrete without ES Matrix":
					features = append(features, "XCh")
				case "MA":
					features = append(features, "XLL")
				case "X", "IMAX", "XBR", "XCh", "XXCh", "X96", "x96":
					features = append(features, profiles[i])
				}
			}
		case "AC-3", "E-AC-3", "AC-3 Dep", "MLP FBA":
			for _, p := range strings.Split(profile, " / ") {
				switch p {
				case "E-AC-3+Dep":
					features = append(features, "Dep")
				case "JOC", "16-ch", "AC-3":
					if p != name {
						features = append(features, p)
					}
				}
			}
		}
	}
	base := name
	switch name {
	case "AAC":
		if slices.Contains(features, "LC") {
			info = "Advanced Audio Codec Low Complexity"
			if slices.Contains(features, "eSBR") {
				info += " with Enhanced Spectral Band Replication"
			} else if slices.Contains(features, "SBR") {
				info += " with Spectral Band Replication"
			}
			if slices.Contains(features, "PS") {
				info += " and Parametric Stereo"
			}
		}
	case "AC-3", "AC-3 Dep", "E-AC-3", "E-AC-3 JOC":
		if name == "AC-3 Dep" || slices.Contains(features, "Dep") {
			name = "E-AC-3"
			info = "Enhanced AC-3"
		}
		if name == "E-AC-3 JOC" || slices.Contains(features, "JOC") {
			info = "Enhanced AC-3 with Joint Object Coding"
		}
	case "MLP FBA":
		info = "Meridian Lossless Packing FBA"
		if slices.Contains(features, "16-ch") {
			info += " with 16-channel presentation"
		}
	}
	allowed := []string(nil)
	switch base {
	case "AAC", "ER AAC":
		allowed = []string{"LC", "LTP", "Main", "SSR", "Scalable", "SBR", "PS", "eSBR"}
	case "DTS", "DTS LBR":
		allowed = []string{"ES", "XCh", "XXCh", "XXCH", "XLL", "XBR", "X96", "x96", "X", "IMAX", "non-core"}
	case "AC-3", "AC-3 Dep", "E-AC-3":
		allowed = []string{"JOC", "MLP FBA", "16-ch"}
	case "MLP FBA":
		allowed = []string{"16-ch", "AC-3"}
	}
	for _, feature := range features {
		if slices.Contains(allowed, feature) {
			name += " " + feature
		}
	}
	return name, info
}

// friendlyExistingFieldVisible retains explicit compatibility display options.
// Video delay rows and MP4 source-delay extras cannot bypass their default policy.
func friendlyExistingFieldVisible(kind StreamKind, identity string) bool {
	if identity == "Source_Delay" || identity == "Source_Delay_Source" {
		return false
	}
	return kind != StreamVideo || identity != "Delay" && identity != "Delay_Original"
}
