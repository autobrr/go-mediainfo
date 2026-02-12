package mediainfo

import "fmt"

func buildHEVCFieldsFromSPS(sps h264SPSInfo) []Field {
	fields := []Field{}

	profile := hevcProfileName(sps.ProfileID)
	level := hevcLevelName(sps.LevelID)
	if profile != "" {
		if level != "" {
			fields = append(fields, Field{Name: "Format profile", Value: fmt.Sprintf("%s@L%s", profile, level)})
		} else {
			fields = append(fields, Field{Name: "Format profile", Value: profile})
		}
	}
	if sps.HEVCTier == "High" {
		fields = append(fields, Field{Name: "Format tier", Value: sps.HEVCTier})
	}

	if sps.ChromaFormat != "" {
		fields = append(fields, Field{Name: "Color space", Value: "YUV"})
		fields = append(fields, Field{Name: "Chroma subsampling", Value: sps.ChromaFormat})
		if sps.ChromaFormat == "4:2:0" {
			fields = append(fields, Field{Name: "Chroma subsampling position", Value: "Type 2"})
		}
	}
	if sps.BitDepth > 0 {
		fields = append(fields, Field{Name: "Bit depth", Value: formatBitDepth(uint8(sps.BitDepth))})
	}

	return fields
}

func parseHEVCAnnexBMeta(sample []byte) ([]Field, h264SPSInfo, hevcHDRInfo, bool) {
	var hdr hevcHDRInfo
	parseHEVCSampleHDR(sample, 0, &hdr)

	start, startLen := findAnnexBStartCode(sample, 0)
	for start >= 0 && startLen > 0 {
		next, nextLen := findAnnexBStartCode(sample, start+startLen)
		end := len(sample)
		if next >= 0 {
			end = next
		}
		nal := sample[start+startLen : end]
		if len(nal) > 2 {
			nalType := (nal[0] >> 1) & 0x3F
			if nalType == 33 {
				sps := parseHEVCSPS(nal)
				if sps.Width > 0 && sps.Height > 0 {
					return buildHEVCFieldsFromSPS(sps), sps, hdr, true
				}
			}
		}
		start = next
		startLen = nextLen
	}
	return nil, h264SPSInfo{}, hdr, false
}
