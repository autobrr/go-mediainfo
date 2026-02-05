package mediainfo

import (
	"fmt"
)

type h264SPSInfo struct {
	ChromaFormat            string
	BitDepth                int
	RefFrames               int
	Progressive             bool
	HasScanType             bool
	VideoFormat             int
	HasVideoFmt             bool
	ColorRange              string
	HasColorRange           bool
	ColorPrimaries          string
	TransferCharacteristics string
	MatrixCoefficients      string
	HasColorDescription     bool
	ProfileID               byte
	LevelID                 byte
	Width                   uint64
	Height                  uint64
	CodedWidth              uint64
	CodedHeight             uint64
	FrameRate               float64
	FixedFrameRate          bool
	HasFixedFrameRate       bool
	BitRate                 int64
	HasBitRate              bool
	BitRateCBR              bool
	HasBitRateCBR           bool
	BufferSize              int64
	HasBufferSize           bool
}

func parseAVCConfig(payload []byte) (string, []Field, h264SPSInfo) {
	if len(payload) < 7 {
		return "", nil, h264SPSInfo{}
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
	if spsInfo.HasVideoFmt {
		if standard := mapH264VideoFormat(spsInfo.VideoFormat); standard != "" {
			fields = append(fields, Field{Name: "Standard", Value: standard})
		}
	}
	if spsInfo.HasColorRange {
		fields = append(fields, Field{Name: "Color range", Value: spsInfo.ColorRange})
	}
	if spsInfo.HasColorDescription {
		if spsInfo.ColorPrimaries != "" {
			fields = append(fields, Field{Name: "Color primaries", Value: spsInfo.ColorPrimaries})
		}
		if spsInfo.TransferCharacteristics != "" {
			fields = append(fields, Field{Name: "Transfer characteristics", Value: spsInfo.TransferCharacteristics})
		}
		if spsInfo.MatrixCoefficients != "" {
			fields = append(fields, Field{Name: "Matrix coefficients", Value: spsInfo.MatrixCoefficients})
		}
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
			if *ppsCABAC {
				fields = append(fields, Field{Name: "Format settings", Value: fmt.Sprintf("CABAC / %d Ref Frames", spsInfo.RefFrames)})
			} else {
				fields = append(fields, Field{Name: "Format settings", Value: fmt.Sprintf("%d Ref Frames", spsInfo.RefFrames)})
			}
		} else {
			fields = append(fields, Field{Name: "Format settings", Value: "CABAC"})
		}
	}

	return profile, fields, spsInfo
}

func parseH264SPS(nal []byte) h264SPSInfo {
	rbsp := nalToRBSP(nal)
	br := newBitReader(rbsp)
	profileID := br.readBitsValue(8)
	_ = br.readBitsValue(8) // constraint flags + reserved
	levelID := br.readBitsValue(8)
	_ = br.readUE()

	chromaFormat := 1
	bitDepth := 8
	separateColourPlane := 0
	videoFormat := 0
	hasVideoFormat := false
	colorRange := ""
	hasColorRange := false
	colorPrimaries := ""
	transferCharacteristics := ""
	matrixCoefficients := ""
	hasColorDescription := false
	bitRate := int64(0)
	hasBitRate := false
	bitRateCBR := false
	hasBitRateCBR := false
	bufferSize := int64(0)
	hasBufferSize := false

	if isHighProfile(profileID) {
		chromaFormat = br.readUE()
		if chromaFormat == 3 {
			separateColourPlane = int(br.readBitsValue(1))
		}
		bitDepthLuma := br.readUE() + 8
		_ = br.readUE()
		_ = br.readBitsValue(1)
		bitDepth = bitDepthLuma
		if br.readBitsValue(1) == 1 {
			for range 8 {
				if br.readBitsValue(1) == 1 {
					skipScalingList(br, 16)
				}
			}
		}
	}

	_ = br.readUE()
	pocType := br.readUE()
	switch pocType {
	case 0:
		_ = br.readUE()
	case 1:
		_ = br.readBitsValue(1)
		_ = br.readSE()
		_ = br.readSE()
		numRef := br.readUE()
		for range numRef {
			_ = br.readSE()
		}
	}

	refFrames := br.readUE()
	_ = br.readBitsValue(1)
	picWidthMbsMinus1 := br.readUE()
	picHeightMapUnitsMinus1 := br.readUE()
	frameMbsOnly := br.readBitsValue(1)
	progressive := frameMbsOnly == 1
	frameMbsOnlyInt := 0
	if frameMbsOnly != 0 {
		frameMbsOnlyInt = 1
	}
	if frameMbsOnly == 0 {
		_ = br.readBitsValue(1)
	}
	_ = br.readBitsValue(1)
	cropFlag := br.readBitsValue(1)
	var cropLeft, cropRight, cropTop, cropBottom int
	if cropFlag == 1 {
		cropLeft = br.readUE()
		cropRight = br.readUE()
		cropTop = br.readUE()
		cropBottom = br.readUE()
	}

	codedWidth := (picWidthMbsMinus1 + 1) * 16
	codedHeight := (picHeightMapUnitsMinus1 + 1) * 16
	if frameMbsOnly == 0 {
		codedHeight *= 2
	}
	width := codedWidth
	height := codedHeight
	if cropFlag == 1 {
		subWidthC := 1
		subHeightC := 1
		if chromaFormat == 1 || chromaFormat == 2 {
			subWidthC = 2
		}
		if chromaFormat == 1 {
			subHeightC = 2
		}
		if chromaFormat == 0 {
			subWidthC = 1
			subHeightC = 2 - frameMbsOnlyInt
		}
		if chromaFormat == 3 && separateColourPlane == 0 {
			subWidthC = 1
			subHeightC = 1
		}
		cropUnitX := subWidthC
		cropUnitY := subHeightC
		if frameMbsOnlyInt == 0 {
			cropUnitY *= 2
		}
		if width > (cropLeft+cropRight)*cropUnitX {
			width -= (cropLeft + cropRight) * cropUnitX
		}
		if height > (cropTop+cropBottom)*cropUnitY {
			height -= (cropTop + cropBottom) * cropUnitY
		}
	}

	frameRate := 0.0
	fixedFrameRate := false
	hasFixedFrameRate := false
	if br.readBitsValue(1) == 1 {
		if br.readBitsValue(1) == 1 {
			aspectRatioIDC := br.readBitsValue(8)
			if aspectRatioIDC == 255 {
				_ = br.readBitsValue(16)
				_ = br.readBitsValue(16)
			}
		}
		if br.readBitsValue(1) == 1 {
			_ = br.readBitsValue(1)
		}
		if br.readBitsValue(1) == 1 {
			videoFormat = int(br.readBitsValue(3))
			fullRange := br.readBitsValue(1) == 1
			hasVideoFormat = true
			if fullRange {
				colorRange = "Full"
			} else {
				colorRange = "Limited"
			}
			hasColorRange = true
			if br.readBitsValue(1) == 1 {
				primaries := br.readBitsValue(8)
				transfer := br.readBitsValue(8)
				matrix := br.readBitsValue(8)
				colorPrimaries = matroskaColorName(primaries)
				transferCharacteristics = matroskaColorName(transfer)
				matrixCoefficients = matroskaColorName(matrix)
				hasColorDescription = true
			}
		}
		if br.readBitsValue(1) == 1 {
			_ = br.readUE()
			_ = br.readUE()
		}
		if br.readBitsValue(1) == 1 {
			numUnitsInTick := br.readBitsValue(32)
			timeScale := br.readBitsValue(32)
			fixedFrameRate = br.readBitsValue(1) == 1
			hasFixedFrameRate = true
			if numUnitsInTick > 0 {
				frameRate = float64(timeScale) / (2.0 * float64(numUnitsInTick))
			}
		}
		nalHRDPresent := br.readBitsValue(1) == 1
		if nalHRDPresent {
			if hrdBitRate, hrdBuffer, hrdCBR, ok := parseH264HRD(br); ok {
				if hrdBitRate > 0 {
					bitRate = hrdBitRate
					hasBitRate = true
					bitRateCBR = hrdCBR
					hasBitRateCBR = true
				}
				if hrdBuffer > 0 {
					bufferSize = hrdBuffer
					hasBufferSize = true
				}
			}
		}
		vclHRDPresent := br.readBitsValue(1) == 1
		if vclHRDPresent {
			if hrdBitRate, hrdBuffer, hrdCBR, ok := parseH264HRD(br); ok {
				if hrdBitRate > 0 && !hasBitRate {
					bitRate = hrdBitRate
					hasBitRate = true
					bitRateCBR = hrdCBR
					hasBitRateCBR = true
				}
				if hrdBuffer > 0 && !hasBufferSize {
					bufferSize = hrdBuffer
					hasBufferSize = true
				}
			}
		}
		if nalHRDPresent || vclHRDPresent {
			_ = br.readBitsValue(1)
		}
	}

	info := h264SPSInfo{
		BitDepth:                bitDepth,
		RefFrames:               refFrames,
		Progressive:             progressive,
		HasScanType:             true,
		VideoFormat:             videoFormat,
		HasVideoFmt:             hasVideoFormat,
		ColorRange:              colorRange,
		HasColorRange:           hasColorRange,
		ColorPrimaries:          colorPrimaries,
		TransferCharacteristics: transferCharacteristics,
		MatrixCoefficients:      matrixCoefficients,
		HasColorDescription:     hasColorDescription,
		ProfileID:               byte(profileID),
		LevelID:                 byte(levelID),
		Width:                   uint64(width),
		Height:                  uint64(height),
		CodedWidth:              uint64(codedWidth),
		CodedHeight:             uint64(codedHeight),
		FrameRate:               frameRate,
		FixedFrameRate:          fixedFrameRate,
		HasFixedFrameRate:       hasFixedFrameRate,
		BitRate:                 bitRate,
		HasBitRate:              hasBitRate,
		BitRateCBR:              bitRateCBR,
		HasBitRateCBR:           hasBitRateCBR,
		BufferSize:              bufferSize,
		HasBufferSize:           hasBufferSize,
	}
	info.ChromaFormat = chromaFormatString(chromaFormat)
	return info
}

func parseH264HRD(br *bitReader) (int64, int64, bool, bool) {
	cpbCntMinus1, ok := br.readUEWithOk()
	if !ok {
		return 0, 0, false, false
	}
	bitRateScale := br.readBitsValue(4)
	cpbSizeScale := br.readBitsValue(4)
	if bitRateScale == ^uint64(0) || cpbSizeScale == ^uint64(0) {
		return 0, 0, false, false
	}
	var bitRateValue int
	var cpbSizeValue int
	var cbrFlag bool
	for i := 0; i <= cpbCntMinus1; i++ {
		brValue, ok := br.readUEWithOk()
		if !ok {
			return 0, 0, false, false
		}
		value, ok := br.readUEWithOk()
		if !ok {
			return 0, 0, false, false
		}
		if i == 0 {
			bitRateValue = brValue
			cpbSizeValue = value
		}
		flag := br.readBitsValue(1)
		if flag == ^uint64(0) {
			return 0, 0, false, false
		}
		if i == 0 {
			cbrFlag = flag == 1
		}
	}
	for range 4 {
		if br.readBitsValue(5) == ^uint64(0) {
			return 0, 0, false, false
		}
	}
	bitRate := int64(bitRateValue+1) << (6 + bitRateScale)
	bufferSize := int64(cpbSizeValue+1) << (4 + cpbSizeScale)
	if bitRate < 0 || bufferSize < 0 {
		return 0, 0, false, false
	}
	return bitRate, bufferSize, cbrFlag, true
}

func parseH264PPSCabac(nal []byte) (bool, bool) {
	rbsp := nalToRBSP(nal)
	br := newBitReader(rbsp)
	_ = br.readUE()
	_ = br.readUE()
	flag := br.readBitsValue(1)
	return flag == 1, true
}

func parseH264AnnexB(payload []byte) ([]Field, uint64, uint64, float64) {
	var spsInfo h264SPSInfo
	var hasSPS bool
	var ppsCABAC *bool
	var hasSlice bool
	start := 0
	for start+4 <= len(payload) {
		sc, scLen := findAnnexBStartCode(payload, start)
		if sc == -1 {
			break
		}
		nalStart := sc + scLen
		next, _ := findAnnexBStartCode(payload, nalStart)
		nalEnd := next
		if nalEnd == -1 {
			nalEnd = len(payload)
		}
		if nalStart < nalEnd {
			nal := payload[nalStart:nalEnd]
			if len(nal) > 0 {
				if nal[0]&0x80 != 0 {
					if next == -1 {
						break
					}
					start = next
					continue
				}
				nalType := nal[0] & 0x1F
				if nalType == 7 {
					spsInfo = parseH264SPS(nal)
					hasSPS = true
				}
				if nalType == 8 {
					if cabac, ok := parseH264PPSCabac(nal); ok {
						ppsCABAC = &cabac
					}
				}
				if nalType == 1 || nalType == 5 {
					hasSlice = true
				}
			}
		}
		if next == -1 {
			break
		}
		start = next
	}

	if !hasSPS || ppsCABAC == nil || !hasSlice {
		return nil, 0, 0, 0
	}

	profile := mapAVCProfile(spsInfo.ProfileID)
	if profile == "" || spsInfo.Width == 0 || spsInfo.Height == 0 {
		return nil, 0, 0, 0
	}
	if !isValidAVCLevel(spsInfo.LevelID) {
		return nil, 0, 0, 0
	}
	if (profile == "Baseline" || profile == "Extended") && *ppsCABAC {
		return nil, 0, 0, 0
	}

	fields := []Field{}
	if level := formatAVCLevel(spsInfo.LevelID); level != "" {
		fields = append(fields, Field{Name: "Format profile", Value: fmt.Sprintf("%s@%s", profile, level)})
	} else {
		fields = append(fields, Field{Name: "Format profile", Value: profile})
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
	if spsInfo.HasVideoFmt {
		if standard := mapH264VideoFormat(spsInfo.VideoFormat); standard != "" {
			fields = append(fields, Field{Name: "Standard", Value: standard})
		}
	}
	if spsInfo.HasColorRange {
		fields = append(fields, Field{Name: "Color range", Value: spsInfo.ColorRange})
	}
	if spsInfo.RefFrames > 0 {
		fields = append(fields, Field{Name: "Format settings, Reference frames", Value: fmt.Sprintf("%d frames", spsInfo.RefFrames)})
	}
	if *ppsCABAC {
		fields = append(fields, Field{Name: "Format settings, CABAC", Value: "Yes"})
	} else {
		fields = append(fields, Field{Name: "Format settings, CABAC", Value: "No"})
	}
	if spsInfo.RefFrames > 0 {
		if *ppsCABAC {
			fields = append(fields, Field{Name: "Format settings", Value: fmt.Sprintf("CABAC / %d Ref Frames", spsInfo.RefFrames)})
		} else {
			fields = append(fields, Field{Name: "Format settings", Value: fmt.Sprintf("%d Ref Frames", spsInfo.RefFrames)})
		}
	} else if *ppsCABAC {
		fields = append(fields, Field{Name: "Format settings", Value: "CABAC"})
	}
	width := uint64(0)
	height := uint64(0)
	frameRate := 0.0
	if hasSPS {
		width = spsInfo.Width
		height = spsInfo.Height
		frameRate = spsInfo.FrameRate
	}
	return fields, width, height, frameRate
}

func h264SliceCountAnnexB(payload []byte) int {
	counts := map[int]int{}
	current := 0
	start := 0
	for start+4 <= len(payload) {
		sc, scLen := findAnnexBStartCode(payload, start)
		if sc == -1 {
			break
		}
		nalStart := sc + scLen
		next, _ := findAnnexBStartCode(payload, nalStart)
		nalEnd := next
		if nalEnd == -1 {
			nalEnd = len(payload)
		}
		if nalStart < nalEnd {
			nal := payload[nalStart:nalEnd]
			if len(nal) > 0 {
				nalType := nal[0] & 0x1F
				switch nalType {
				case 9:
					if current > 0 {
						counts[current]++
						current = 0
					}
				case 1, 5:
					firstMB, ok := h264FirstMBInSlice(nal)
					if ok && firstMB == 0 && current > 0 {
						counts[current]++
						current = 0
					}
					current++
				}
			}
		}
		if next == -1 {
			break
		}
		start = next
	}
	if current > 0 {
		counts[current]++
	}
	bestCount := 0
	bestFreq := 0
	for count, freq := range counts {
		if freq > bestFreq || (freq == bestFreq && count > bestCount) {
			bestCount = count
			bestFreq = freq
		}
	}
	return bestCount
}

func h264FirstMBInSlice(nal []byte) (int, bool) {
	rbsp := nalToRBSP(nal)
	if len(rbsp) == 0 {
		return 0, false
	}
	br := newBitReader(rbsp)
	firstMB, ok := br.readUEWithOk()
	if !ok {
		return 0, false
	}
	return firstMB, true
}

func findAnnexBStartCode(data []byte, start int) (int, int) {
	for i := start; i+3 < len(data); i++ {
		if data[i] == 0x00 && data[i+1] == 0x00 {
			if data[i+2] == 0x01 {
				return i, 3
			}
			if i+3 < len(data) && data[i+2] == 0x00 && data[i+3] == 0x01 {
				return i, 4
			}
		}
	}
	return -1, 0
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

func mapH264VideoFormat(id int) string {
	switch id {
	case 1:
		return "PAL"
	case 2:
		return "NTSC"
	case 3:
		return "SECAM"
	case 4:
		return "MAC"
	default:
		return ""
	}
}

func isValidAVCLevel(levelID byte) bool {
	switch levelID {
	case 9, 10, 11, 12, 13, 20, 21, 22, 30, 31, 32, 40, 41, 42, 50, 51, 52, 60, 61, 62:
		return true
	default:
		return false
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

func (br *bitReader) readBitsValue(n uint8) uint64 {
	var value uint64
	for range n {
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
	value, ok := br.readUEWithOk()
	if !ok {
		return 0
	}
	return value
}

func (br *bitReader) readSE() int {
	val := br.readUE()
	if val%2 == 0 {
		return -(val / 2)
	}
	return (val + 1) / 2
}

func (br *bitReader) readUEWithOk() (int, bool) {
	zeros := 0
	for {
		bit := br.readBitsValue(1)
		if bit == ^uint64(0) {
			return 0, false
		}
		if bit == 1 {
			break
		}
		zeros++
	}
	if zeros == 0 {
		return 0, true
	}
	value := br.readBitsValue(uint8(zeros))
	if value == ^uint64(0) {
		return 0, false
	}
	return (1 << zeros) - 1 + int(value), true
}

func skipScalingList(br *bitReader, size int) {
	last := 8
	next := 8
	for range size {
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
