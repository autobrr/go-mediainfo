package mediainfo

import "fmt"

type hevcConfigInfo struct {
	profileName   string
	levelName     string
	tierName      string
	chromaFormat  string
	bitDepth      uint8
	nalLengthSize int
}

func parseHEVCConfig(payload []byte) (string, []Field, hevcConfigInfo, h264SPSInfo) {
	if len(payload) < 23 {
		return "", nil, hevcConfigInfo{}, h264SPSInfo{}
	}
	profileIDC := payload[1] & 0x1F
	tierFlag := (payload[1] >> 5) & 0x01
	levelIDC := payload[12]
	chromaFormatIDC := payload[16] & 0x03
	bitDepthLuma := (payload[17] & 0x07) + 8
	lengthSizeMinusOne := payload[21] & 0x03
	tierName := hevcTierName(tierFlag)

	info := hevcConfigInfo{
		profileName:   hevcProfileName(profileIDC),
		levelName:     hevcLevelName(levelIDC),
		tierName:      tierName,
		chromaFormat:  hevcChromaFormatName(chromaFormatIDC),
		bitDepth:      bitDepthLuma,
		nalLengthSize: int(lengthSizeMinusOne) + 1,
	}

	fields := []Field{}
	if info.profileName != "" {
		profile := info.profileName
		if info.levelName != "" {
			profile = fmt.Sprintf("%s@L%s", profile, info.levelName)
		}
		fields = append(fields, Field{Name: "Format profile", Value: profile})
	}
	if tierName == "High" {
		fields = append(fields, Field{Name: "Format tier", Value: tierName})
	}
	if info.chromaFormat != "" {
		fields = append(fields, Field{Name: "Chroma subsampling", Value: info.chromaFormat})
		if chromaFormatIDC == 1 {
			// Match official MediaInfo JSON output for HEVC 4:2:0.
			fields = append(fields, Field{Name: "Chroma subsampling position", Value: "Type 2"})
		}
	}
	if info.bitDepth > 0 {
		fields = append(fields, Field{Name: "Bit depth", Value: formatBitDepth(info.bitDepth)})
	}

	spsInfo := h264SPSInfo{}
	if sps := findHEVCSPSInConfig(payload); len(sps) > 0 {
		spsInfo = parseHEVCSPS(sps)
	}
	return info.profileName, fields, info, spsInfo
}

func hevcProfileName(idc byte) string {
	switch idc {
	case 1:
		return "Main"
	case 2:
		return "Main 10"
	case 3:
		return "Main Still"
	case 4:
		return "Range Extensions"
	case 5:
		return "High Throughput"
	default:
		return ""
	}
}

func hevcLevelName(idc byte) string {
	if idc == 0 {
		return ""
	}
	level := float64(idc) / 30.0
	if level == float64(int(level)) {
		return fmt.Sprintf("%.0f", level)
	}
	return fmt.Sprintf("%.1f", level)
}

func hevcTierName(flag byte) string {
	if flag == 1 {
		return "High"
	}
	return "Main"
}

func hevcChromaFormatName(idc byte) string {
	switch idc {
	case 0:
		return "4:0:0"
	case 1:
		return "4:2:0"
	case 2:
		return "4:2:2"
	case 3:
		return "4:4:4"
	default:
		return ""
	}
}
