package mediainfo

import (
	"fmt"
)

// h264SPSInfo contains H.264 SPS/VUI properties used to render AVC metadata and
// interpret timing SEI messages.
type h264SPSInfo struct {
	ChromaFormat            string
	BitDepth                int
	RefFrames               int
	Progressive             bool
	MBAFF                   bool
	HasScanType             bool
	FrameMbsOnly            bool
	Log2MaxFrameNumMinus4   int
	SeparateColourPlane     bool
	SARWidth                uint32
	SARHeight               uint32
	HasSAR                  bool
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
	// HEVC-only: chroma_sample_loc_type_top_field from the SPS VUI, present only when
	// chroma_loc_info_present_flag is set. MediaInfo reports it as "Type <ChromaSampleLoc>".
	HasChromaLoc    bool
	ChromaSampleLoc int
	// HEVC-only: tier name ("Main" or "High") when available.
	HEVCTier              string
	Width                 uint64
	Height                uint64
	CodedWidth            uint64
	CodedHeight           uint64
	FrameRate             float64
	FixedFrameRate        bool
	HasFixedFrameRate     bool
	BitRate               int64
	HasBitRate            bool
	BitRateCBR            bool
	HasBitRateCBR         bool
	BufferSize            int64
	HasBufferSize         bool
	BufferSizeNAL         int64
	HasBufferSizeNAL      bool
	BufferSizeVCL         int64
	HasBufferSizeVCL      bool
	CpbDpbDelaysPresent   bool
	CpbRemovalDelayLength int
	DpbOutputDelayLength  int
	PicStructPresent      bool
	TimeOffsetLength      int
}

// h264HRDInfo contains the first CPB entry and delay widths from an SPS HRD
// parameters structure.
type h264HRDInfo struct {
	bitRate               int64
	bufferSize            int64
	cbr                   bool
	cpbRemovalDelayLength int
	dpbOutputDelayLength  int
	timeOffsetLength      int
}

func parseAVCConfig(payload []byte) (string, []Field, h264SPSInfo) {
	if len(payload) < 7 {
		return "", nil, h264SPSInfo{}
	}
	profileID := payload[1]
	levelID := payload[3]
	profile := mapAVCProfile(profileID)
	level := formatAVCLevel(levelID)

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

	fields := buildH264Fields(profile, level, spsInfo, ppsCABAC, h264FieldOptions{
		includeColorDescription: true,
	})

	return profile, fields, spsInfo
}

type h264FieldOptions struct {
	includeColorDescription bool
	scanTypeFirst           bool
}

func buildH264Fields(profile string, level string, spsInfo h264SPSInfo, ppsCABAC *bool, opts h264FieldOptions) []Field {
	fields := []Field{}
	if profile != "" {
		if level != "" {
			fields = append(fields, Field{Name: "Format profile", Value: fmt.Sprintf("%s@%s", profile, level)})
		} else {
			fields = append(fields, Field{Name: "Format profile", Value: profile})
		}
	}
	if spsInfo.ChromaFormat != "" {
		// MediaInfo reports AVC/H.264 as YUV when chroma information is present.
		fields = append(fields, Field{Name: "Color space", Value: "YUV"})
		fields = append(fields, Field{Name: "Chroma subsampling", Value: spsInfo.ChromaFormat})
	}
	if spsInfo.BitDepth > 0 {
		fields = append(fields, Field{Name: "Bit depth", Value: formatBitDepth(uint8(spsInfo.BitDepth))})
	}
	if opts.scanTypeFirst && spsInfo.HasScanType {
		fields = appendH264ScanType(fields, spsInfo)
		if spsInfo.MBAFF {
			fields = append(fields, Field{Name: "Scan order", Value: "TFF"})
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
	if opts.includeColorDescription && spsInfo.HasColorDescription {
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
	if !opts.scanTypeFirst && spsInfo.HasScanType {
		fields = appendH264ScanType(fields, spsInfo)
		if spsInfo.MBAFF {
			fields = append(fields, Field{Name: "Scan order", Value: "TFF"})
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
	return fields
}

func appendH264ScanType(fields []Field, spsInfo h264SPSInfo) []Field {
	if spsInfo.Progressive {
		return append(fields, Field{Name: "Scan type", Value: "Progressive"})
	}
	if spsInfo.MBAFF {
		return append(fields, Field{Name: "Scan type", Value: "MBAFF"})
	}
	return append(fields, Field{Name: "Scan type", Value: "Interlaced"})
}

// parseH264SPS extracts supported sequence and VUI properties from an SPS NAL.
// Unavailable or malformed properties remain at their zero values.
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
	separateColourPlaneFlag := false
	videoFormat := 0
	hasVideoFormat := false
	sarWidth := uint32(1)
	sarHeight := uint32(1)
	hasSAR := false
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
	bufferSizeNAL := int64(0)
	hasBufferSizeNAL := false
	bufferSizeVCL := int64(0)
	hasBufferSizeVCL := false
	cpbDpbDelaysPresent := false
	cpbRemovalDelayLength := 0
	dpbOutputDelayLength := 0
	picStructPresent := false
	timeOffsetLength := 0

	if isHighProfile(profileID) {
		chromaFormat = br.readUE()
		if chromaFormat == 3 {
			separateColourPlane = int(br.readBitsValue(1))
			separateColourPlaneFlag = separateColourPlane == 1
		}
		bitDepthLuma := br.readUE() + 8
		_ = br.readUE()
		_ = br.readBitsValue(1)
		bitDepth = bitDepthLuma
		if br.readBitsValue(1) == 1 {
			scalingLists := 8
			// H.264: 4:4:4 has 12 scaling lists (6x 4x4, 6x 8x8).
			if chromaFormat == 3 && separateColourPlane == 0 {
				scalingLists = 12
			}
			for i := 0; i < scalingLists; i++ {
				if br.readBitsValue(1) == 1 {
					size := 16
					if i >= 6 {
						size = 64
					}
					skipScalingList(br, size)
				}
			}
		}
	}

	log2MaxFrameNumMinus4 := br.readUE()
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
	mbaff := false
	frameMbsOnlyInt := 0
	if frameMbsOnly != 0 {
		frameMbsOnlyInt = 1
	}
	if frameMbsOnly == 0 {
		mbaff = br.readBitsValue(1) == 1
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
				w := br.readBitsValue(16)
				h := br.readBitsValue(16)
				if w != ^uint64(0) && h != ^uint64(0) && w > 0 && h > 0 {
					sarWidth = uint32(w)
					sarHeight = uint32(h)
					hasSAR = true
				}
			} else {
				if w, h, ok := h264SARFromIDC(aspectRatioIDC); ok {
					sarWidth = w
					sarHeight = h
					hasSAR = true
				}
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
				colorPrimaries = matroskaColorPrimariesName(primaries)
				transferCharacteristics = matroskaTransferName(transfer)
				matrixCoefficients = matroskaMatrixName(matrix)
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
		var timingHRD h264HRDInfo
		if nalHRDPresent {
			if hrd, ok := parseH264HRD(br); ok {
				timingHRD = hrd
				if hrd.bitRate > 0 {
					bitRate = hrd.bitRate
					hasBitRate = true
					bitRateCBR = hrd.cbr
					hasBitRateCBR = true
				}
				if hrd.bufferSize > 0 {
					bufferSizeNAL = hrd.bufferSize
					hasBufferSizeNAL = true
					bufferSize = hrd.bufferSize
					hasBufferSize = true
				}
			}
		}
		vclHRDPresent := br.readBitsValue(1) == 1
		if vclHRDPresent {
			if hrd, ok := parseH264HRD(br); ok {
				if !nalHRDPresent {
					timingHRD = hrd
				}
				if hrd.bitRate > 0 && !hasBitRate {
					bitRate = hrd.bitRate
					hasBitRate = true
					bitRateCBR = hrd.cbr
					hasBitRateCBR = true
				}
				if hrd.bufferSize > 0 && !hasBufferSize {
					bufferSizeVCL = hrd.bufferSize
					hasBufferSizeVCL = true
					bufferSize = hrd.bufferSize
					hasBufferSize = true
				} else if hrd.bufferSize > 0 {
					bufferSizeVCL = hrd.bufferSize
					hasBufferSizeVCL = true
				}
			}
		}
		if nalHRDPresent || vclHRDPresent {
			_ = br.readBitsValue(1)
			cpbDpbDelaysPresent = true
			cpbRemovalDelayLength = timingHRD.cpbRemovalDelayLength
			dpbOutputDelayLength = timingHRD.dpbOutputDelayLength
			timeOffsetLength = timingHRD.timeOffsetLength
		}
		picStructPresent = br.readBitsValue(1) == 1
	}

	info := h264SPSInfo{
		BitDepth:                bitDepth,
		RefFrames:               refFrames,
		Progressive:             progressive,
		MBAFF:                   mbaff,
		HasScanType:             true,
		FrameMbsOnly:            frameMbsOnly == 1,
		Log2MaxFrameNumMinus4:   log2MaxFrameNumMinus4,
		SeparateColourPlane:     separateColourPlaneFlag,
		SARWidth:                sarWidth,
		SARHeight:               sarHeight,
		HasSAR:                  hasSAR,
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
		BufferSizeNAL:           bufferSizeNAL,
		HasBufferSizeNAL:        hasBufferSizeNAL,
		BufferSizeVCL:           bufferSizeVCL,
		HasBufferSizeVCL:        hasBufferSizeVCL,
		CpbDpbDelaysPresent:     cpbDpbDelaysPresent,
		CpbRemovalDelayLength:   cpbRemovalDelayLength,
		DpbOutputDelayLength:    dpbOutputDelayLength,
		PicStructPresent:        picStructPresent,
		TimeOffsetLength:        timeOffsetLength,
	}
	info.ChromaFormat = chromaFormatString(chromaFormat)
	return info
}

func h264SARFromIDC(idc uint64) (uint32, uint32, bool) {
	switch idc {
	case 1:
		return 1, 1, true
	case 2:
		return 12, 11, true
	case 3:
		return 10, 11, true
	case 4:
		return 16, 11, true
	case 5:
		return 40, 33, true
	case 6:
		return 24, 11, true
	case 7:
		return 20, 11, true
	case 8:
		return 32, 11, true
	case 9:
		return 80, 33, true
	case 10:
		return 18, 11, true
	case 11:
		return 15, 11, true
	case 12:
		return 64, 33, true
	case 13:
		return 160, 99, true
	case 14:
		return 4, 3, true
	case 15:
		return 3, 2, true
	case 16:
		return 2, 1, true
	default:
		return 0, 0, false
	}
}

// parseH264HRD consumes one HRD parameters structure and returns its first CPB
// entry plus the delay widths needed to decode pic_timing SEI messages.
func parseH264HRD(br *bitReader) (h264HRDInfo, bool) {
	cpbCntMinus1, ok := br.readUEWithOk()
	if !ok {
		return h264HRDInfo{}, false
	}
	bitRateScale := br.readBitsValue(4)
	cpbSizeScale := br.readBitsValue(4)
	if bitRateScale == ^uint64(0) || cpbSizeScale == ^uint64(0) {
		return h264HRDInfo{}, false
	}
	var bitRateValue int
	var cpbSizeValue int
	var cbrFlag bool
	for i := 0; i <= cpbCntMinus1; i++ {
		brValue, ok := br.readUEWithOk()
		if !ok {
			return h264HRDInfo{}, false
		}
		value, ok := br.readUEWithOk()
		if !ok {
			return h264HRDInfo{}, false
		}
		if i == 0 {
			bitRateValue = brValue
			cpbSizeValue = value
		}
		flag := br.readBitsValue(1)
		if flag == ^uint64(0) {
			return h264HRDInfo{}, false
		}
		if i == 0 {
			cbrFlag = flag == 1
		}
	}
	initialCPBRemovalDelayLengthValue := br.readBitsValue(5)
	cpbRemovalDelayLengthValue := br.readBitsValue(5)
	dpbOutputDelayLengthValue := br.readBitsValue(5)
	if initialCPBRemovalDelayLengthValue == ^uint64(0) || cpbRemovalDelayLengthValue == ^uint64(0) || dpbOutputDelayLengthValue == ^uint64(0) {
		return h264HRDInfo{}, false
	}
	timeOffsetLengthValue := br.readBitsValue(5)
	if timeOffsetLengthValue == ^uint64(0) {
		return h264HRDInfo{}, false
	}
	bitRate := int64(bitRateValue+1) << (6 + bitRateScale)
	bufferSize := int64(cpbSizeValue+1) << (4 + cpbSizeScale)
	if bitRate < 0 || bufferSize < 0 {
		return h264HRDInfo{}, false
	}
	return h264HRDInfo{
		bitRate: bitRate, bufferSize: bufferSize, cbr: cbrFlag,
		cpbRemovalDelayLength: int(cpbRemovalDelayLengthValue) + 1,
		dpbOutputDelayLength:  int(dpbOutputDelayLengthValue) + 1,
		timeOffsetLength:      int(timeOffsetLengthValue),
	}, true
}

func parseH264PPSCabac(nal []byte) (bool, bool) {
	rbsp := nalToRBSP(nal)
	br := newBitReader(rbsp)
	_ = br.readUE()
	_ = br.readUE()
	flag := br.readBitsValue(1)
	return flag == 1, true
}

func parseH264AnnexB(payload []byte) ([]Field, h264SPSInfo, bool) {
	var spsInfo h264SPSInfo
	var hasSPS bool
	var ppsCABAC *bool
	var hasSlice bool
	scanAnnexBNALs(payload, func(nal []byte) bool {
		if len(nal) == 0 {
			return true
		}
		if nal[0]&0x80 != 0 {
			return true
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
		return true
	})

	if !hasSPS || ppsCABAC == nil || !hasSlice {
		return nil, h264SPSInfo{}, false
	}

	profile := mapAVCProfile(spsInfo.ProfileID)
	if profile == "" || spsInfo.Width == 0 || spsInfo.Height == 0 {
		return nil, h264SPSInfo{}, false
	}
	if !isValidAVCLevel(spsInfo.LevelID) {
		return nil, h264SPSInfo{}, false
	}
	if (profile == "Baseline" || profile == "Extended") && *ppsCABAC {
		return nil, h264SPSInfo{}, false
	}

	level := formatAVCLevel(spsInfo.LevelID)
	fields := buildH264Fields(profile, level, spsInfo, ppsCABAC, h264FieldOptions{
		scanTypeFirst: true,
	})
	return fields, spsInfo, true
}

// h264SliceCountAnnexB returns the dominant number of slices per picture in an
// Annex B sample, using access-unit delimiters and first_mb_in_slice boundaries.
func h264SliceCountAnnexB(payload []byte) int {
	counts := map[int]int{}
	current := 0
	scanAnnexBNALs(payload, func(nal []byte) bool {
		if len(nal) == 0 {
			return true
		}
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
		return true
	})
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

// h264TimeCodeFromAnnexB returns the first complete clock timestamp found in a
// pic_timing SEI message, or an empty string when none is usable.
func h264TimeCodeFromAnnexB(payload []byte, sps h264SPSInfo) string {
	var timeCode string
	scanAnnexBNALs(payload, func(nal []byte) bool {
		if len(nal) == 0 || nal[0]&0x1F != 6 {
			return true
		}
		rbsp := nalToRBSP(nal)
		for pos := 0; pos < len(rbsp); {
			payloadType := 0
			for pos < len(rbsp) && rbsp[pos] == 0xFF {
				payloadType += 255
				pos++
			}
			if pos >= len(rbsp) {
				break
			}
			payloadType += int(rbsp[pos])
			pos++
			payloadSize := 0
			for pos < len(rbsp) && rbsp[pos] == 0xFF {
				payloadSize += 255
				pos++
			}
			if pos >= len(rbsp) {
				break
			}
			payloadSize += int(rbsp[pos])
			pos++
			if payloadSize < 0 || payloadSize > len(rbsp)-pos {
				break
			}
			if payloadType == 1 {
				if parsed, ok := parseH264PicTimingTimeCode(rbsp[pos:pos+payloadSize], sps); ok {
					timeCode = parsed
					return false
				}
			}
			pos += payloadSize
		}
		return true
	})
	return timeCode
}

// parseH264PicTimingTimeCode decodes a pic_timing payload using SPS delay
// widths. It accepts only continuous timestamps with an explicit hour value.
func parseH264PicTimingTimeCode(payload []byte, sps h264SPSInfo) (string, bool) {
	if !sps.PicStructPresent || len(payload) == 0 {
		return "", false
	}
	br := newBitReader(payload)
	read := func(bits int) (uint64, bool) {
		value := br.readBitsValue(uint8(bits))
		return value, value != ^uint64(0)
	}
	if sps.CpbDpbDelaysPresent {
		if _, ok := read(sps.CpbRemovalDelayLength); !ok {
			return "", false
		}
		if _, ok := read(sps.DpbOutputDelayLength); !ok {
			return "", false
		}
	}
	picStruct, ok := read(4)
	if !ok || picStruct > 8 {
		return "", false
	}
	numClockTS := [...]int{1, 1, 1, 2, 2, 3, 3, 2, 3}
	for range numClockTS[picStruct] {
		clockTimestampFlag, ok := read(1)
		if !ok {
			return "", false
		}
		if clockTimestampFlag == 0 {
			continue
		}
		if _, ok := read(2); !ok { // ct_type
			return "", false
		}
		if _, ok := read(1); !ok { // nuit_field_based_flag
			return "", false
		}
		if _, ok := read(5); !ok { // counting_type
			return "", false
		}
		fullTimestamp, ok := read(1)
		if !ok {
			return "", false
		}
		discontinuity, ok := read(1)
		if !ok {
			return "", false
		}
		cntDropped, ok := read(1)
		if !ok {
			return "", false
		}
		frames, ok := read(8)
		if !ok {
			return "", false
		}
		var seconds, minutes, hours uint64
		hasHours := false
		if fullTimestamp == 1 {
			if seconds, ok = read(6); !ok {
				return "", false
			}
			if minutes, ok = read(6); !ok {
				return "", false
			}
			if hours, ok = read(5); !ok {
				return "", false
			}
			hasHours = true
		} else {
			secondsFlag, valid := read(1)
			if !valid {
				return "", false
			}
			if secondsFlag == 1 {
				if seconds, ok = read(6); !ok {
					return "", false
				}
				minutesFlag, valid := read(1)
				if !valid {
					return "", false
				}
				if minutesFlag == 1 {
					if minutes, ok = read(6); !ok {
						return "", false
					}
					hoursFlag, valid := read(1)
					if !valid {
						return "", false
					}
					if hoursFlag == 1 {
						if hours, ok = read(5); !ok {
							return "", false
						}
						hasHours = true
					}
				}
			}
		}
		if sps.TimeOffsetLength > 0 {
			if _, ok := read(sps.TimeOffsetLength); !ok {
				return "", false
			}
		}
		if discontinuity == 1 {
			return "", false
		}
		// MediaInfo only promotes pic_timing to a first-frame time code when the
		// complete clock is present. Some Blu-ray streams carry minutes/seconds
		// while omitting hours; treating the absent component as zero creates a
		// timestamp that MediaInfo intentionally suppresses.
		if !hasHours {
			return "", false
		}
		separator := ":"
		if cntDropped == 1 {
			separator = ";"
		}
		return fmt.Sprintf("%02d:%02d:%02d%s%02d", hours, minutes, seconds, separator, frames), true
	}
	return "", false
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

func scanAnnexBNALs(payload []byte, fn func(nal []byte) bool) {
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
			if !fn(payload[nalStart:nalEnd]) {
				return
			}
		}
		if next == -1 {
			break
		}
		start = next
	}
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
	case 0:
		return "Component"
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

func (br *bitReader) readSEWithOk() (int, bool) {
	val, ok := br.readUEWithOk()
	if !ok {
		return 0, false
	}
	if val%2 == 0 {
		return -(val / 2), true
	}
	return (val + 1) / 2, true
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
	return nalToRBSPWithHeader(nal, 1)
}

func nalToRBSPWithHeader(nal []byte, headerLen int) []byte {
	if headerLen < 0 || len(nal) <= headerLen {
		return nil
	}
	nal = nal[headerLen:]
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
