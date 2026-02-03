package mediainfo

import (
	"fmt"
)

type h264SPSInfo struct {
	ChromaFormat string
	BitDepth     int
	RefFrames    int
	Progressive  bool
	HasScanType  bool
}

func parseAVCConfig(payload []byte) (string, []Field) {
	if len(payload) < 7 {
		return "", nil
	}
	profileID := payload[1]
	levelID := payload[3]
	profile := mapAVCProfile(profileID)
	level := formatAVCLevel(levelID)
	var fields []Field
	if profile != "" {
		if level != "" {
			fields = append(fields, Field{Name: "Format profile", Value: fmt.Sprintf("%s@%s", profile, level)})
		} else {
			fields = append(fields, Field{Name: "Format profile", Value: profile})
		}
	}

	spsCount := int(payload[5] & 0x1F)
	offset := 6
	var spsInfo h264SPSInfo
	var ppsCABAC *bool

	if spsCount > 0 && offset+2 <= len(payload) {
		spsLen := int(payload[offset])<<8 | int(payload[offset+1])
		offset += 2
		if offset+spsLen <= len(payload) && spsLen > 0 {
			sps := payload[offset : offset+spsLen]
			spsInfo = parseH264SPS(sps)
		}
		offset += spsLen
	}

	if offset < len(payload) {
		ppsCount := int(payload[offset])
		offset++
		if ppsCount > 0 && offset+2 <= len(payload) {
			ppsLen := int(payload[offset])<<8 | int(payload[offset+1])
			offset += 2
			if offset+ppsLen <= len(payload) && ppsLen > 0 {
				pps := payload[offset : offset+ppsLen]
				if cabac, ok := parseH264PPSCabac(pps); ok {
					ppsCABAC = &cabac
				}
			}
		}
	}

	if spsInfo.ChromaFormat != "" {
		fields = append(fields, Field{Name: "Chroma subsampling", Value: spsInfo.ChromaFormat})
	}
	if spsInfo.BitDepth > 0 {
		fields = append(fields, Field{Name: "Bit depth", Value: formatBitDepth(uint8(spsInfo.BitDepth))})
	}
	if spsInfo.HasScanType {
		if spsInfo.Progressive {
			fields = append(fields, Field{Name: "Scan type", Value: "Progressive"})
		} else {
			fields = append(fields, Field{Name: "Scan type", Value: "Interlaced"})
		}
	}
	if spsInfo.RefFrames > 0 {
		fields = append(fields, Field{Name: "Format settings, Reference frames", Value: fmt.Sprintf("%d frames", spsInfo.RefFrames)})
	}
	if ppsCABAC != nil {
		if *ppsCABAC {
			fields = append(fields, Field{Name: "Format settings, CABAC", Value: "Yes"})
		} else {
			fields = append(fields, Field{Name: "Format settings, CABAC", Value: "No"})
		}
		if spsInfo.RefFrames > 0 {
			fields = append(fields, Field{Name: "Format settings", Value: fmt.Sprintf("CABAC / %d Ref Frames", spsInfo.RefFrames)})
		} else {
			fields = append(fields, Field{Name: "Format settings", Value: "CABAC"})
		}
	}

	return profile, fields
}

func parseH264SPS(nal []byte) h264SPSInfo {
	rbsp := nalToRBSP(nal)
	br := newBitReader(rbsp)
	profileID := br.readBitsValue(8)
	_ = br.readBitsValue(8) // constraint flags + reserved
	_ = br.readBitsValue(8) // level_idc
	_ = br.readUE()

	chromaFormat := 1
	bitDepth := 8

	if isHighProfile(profileID) {
		chromaFormat = br.readUE()
		if chromaFormat == 3 {
			br.readBitsValue(1)
		}
		bitDepthLuma := br.readUE() + 8
		_ = br.readUE()
		_ = br.readBitsValue(1)
		bitDepth = bitDepthLuma
		if br.readBitsValue(1) == 1 {
			for i := 0; i < 8; i++ {
				if br.readBitsValue(1) == 1 {
					skipScalingList(br, 16)
				}
			}
		}
	}

	_ = br.readUE()
	pocType := br.readUE()
	if pocType == 0 {
		_ = br.readUE()
	} else if pocType == 1 {
		_ = br.readBitsValue(1)
		_ = br.readSE()
		_ = br.readSE()
		numRef := br.readUE()
		for i := 0; i < numRef; i++ {
			_ = br.readSE()
		}
	}

	refFrames := br.readUE()
	_ = br.readBitsValue(1)
	_ = br.readUE()
	_ = br.readUE()
	frameMbsOnly := br.readBitsValue(1)
	progressive := frameMbsOnly == 1
	if frameMbsOnly == 0 {
		_ = br.readBitsValue(1)
	}
	_ = br.readBitsValue(1)
	cropFlag := br.readBitsValue(1)
	if cropFlag == 1 {
		_ = br.readUE()
		_ = br.readUE()
		_ = br.readUE()
		_ = br.readUE()
	}

	info := h264SPSInfo{
		BitDepth:    bitDepth,
		RefFrames:   refFrames,
		Progressive: progressive,
		HasScanType: true,
	}
	info.ChromaFormat = chromaFormatString(chromaFormat)
	return info
}

func parseH264PPSCabac(nal []byte) (bool, bool) {
	rbsp := nalToRBSP(nal)
	br := newBitReader(rbsp)
	_ = br.readUE()
	_ = br.readUE()
	flag := br.readBitsValue(1)
	return flag == 1, true
}

func isHighProfile(profileID uint64) bool {
	switch profileID {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134:
		return true
	default:
		return false
	}
}

func chromaFormatString(id int) string {
	switch id {
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

type bitReader struct {
	data []byte
	pos  int
	bit  uint8
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data}
}

func (br *bitReader) readBits(n uint8) bool {
	return br.readBitsValue(n) != ^uint64(0)
}

func (br *bitReader) readBitsValue(n uint8) uint64 {
	var value uint64
	for i := uint8(0); i < n; i++ {
		if br.pos >= len(br.data) {
			return ^uint64(0)
		}
		bit := (br.data[br.pos] >> (7 - br.bit)) & 1
		value = (value << 1) | uint64(bit)
		br.bit++
		if br.bit == 8 {
			br.bit = 0
			br.pos++
		}
	}
	return value
}

func (br *bitReader) readUE() int {
	zeros := 0
	for {
		bit := br.readBitsValue(1)
		if bit == ^uint64(0) {
			return 0
		}
		if bit == 1 {
			break
		}
		zeros++
	}
	if zeros == 0 {
		return 0
	}
	value := br.readBitsValue(uint8(zeros))
	if value == ^uint64(0) {
		return 0
	}
	return int((1 << zeros) - 1 + int(value))
}

func (br *bitReader) readSE() int {
	val := br.readUE()
	if val%2 == 0 {
		return -(val / 2)
	}
	return (val + 1) / 2
}

func skipScalingList(br *bitReader, size int) {
	last := 8
	next := 8
	for i := 0; i < size; i++ {
		if next != 0 {
			next = (last + br.readSE() + 256) % 256
		}
		if next != 0 {
			last = next
		}
	}
}

func nalToRBSP(nal []byte) []byte {
	if len(nal) <= 1 {
		return nil
	}
	nal = nal[1:]
	rbsp := make([]byte, 0, len(nal))
	zeroCount := 0
	for _, b := range nal {
		if zeroCount == 2 && b == 0x03 {
			zeroCount = 0
			continue
		}
		rbsp = append(rbsp, b)
		if b == 0x00 {
			zeroCount++
		} else {
			zeroCount = 0
		}
	}
	return rbsp
}
