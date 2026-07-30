package mediainfo

import (
	"encoding/binary"
	"fmt"
)

// hevcConfigInfo contains HEVCDecoderConfigurationRecord facts needed by
// direct canonical stream builders and length-prefixed NAL scanning.
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

	spsInfo := h264SPSInfo{}
	if sps := findHEVCSPSInConfig(payload); len(sps) > 0 {
		spsInfo = parseHEVCSPS(sps)
	}

	fields := []Field{}
	if info.profileName != "" {
		profile := info.profileName
		if info.levelName != "" {
			profile = fmt.Sprintf("%s@L%s", profile, info.levelName)
		}
		fields = append(fields, Field{Name: "Format profile", Value: profile})
	}
	// MediaInfo always reports the HEVC tier (Main or High).
	if tierName != "" {
		fields = append(fields, Field{Name: "Format tier", Value: tierName})
	}
	if info.chromaFormat != "" {
		fields = append(fields, Field{Name: "Chroma subsampling", Value: info.chromaFormat})
		// MediaInfo emits ChromaSubsampling_Position only when the SPS VUI signals
		// chroma_loc_info; the value mirrors chroma_sample_loc_type_top_field.
		if spsInfo.HasChromaLoc {
			fields = append(fields, Field{Name: "Chroma subsampling position", Value: fmt.Sprintf("Type %d", spsInfo.ChromaSampleLoc)})
		}
	}
	if info.bitDepth > 0 {
		fields = append(fields, Field{Name: "Bit depth", Value: formatBitDepth(info.bitDepth)})
	}

	return info.profileName, fields, info, spsInfo
}

// parseHEVCConfigSEI walks the NAL arrays of an HEVC DecoderConfigurationRecord
// (hvcC) and feeds any prefix/suffix SEI NAL units to the SEI parser. Some muxers
// (e.g. ffmpeg with global headers) place the x265 encoder user_data_unregistered
// SEI in the configuration record rather than in frame data, so the cluster/bitstream
// scan never sees it.
func parseHEVCConfigSEI(payload []byte, info *hevcHDRInfo) {
	if info == nil || len(payload) < 23 {
		return
	}
	numArrays := int(payload[22])
	offset := 23
	for a := 0; a < numArrays; a++ {
		if offset+3 > len(payload) {
			return
		}
		nalUnitType := payload[offset] & 0x3F
		offset++
		numNalus := int(binary.BigEndian.Uint16(payload[offset : offset+2]))
		offset += 2
		for n := 0; n < numNalus; n++ {
			if offset+2 > len(payload) {
				return
			}
			nalLen := int(binary.BigEndian.Uint16(payload[offset : offset+2]))
			offset += 2
			if nalLen <= 0 || offset+nalLen > len(payload) {
				return
			}
			nal := payload[offset : offset+nalLen]
			offset += nalLen
			if nalUnitType == 39 || nalUnitType == 40 {
				parseHEVCNAL(nal, info)
			}
		}
	}
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
