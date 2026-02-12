package mediainfo

import (
	"fmt"
	"math"
)

var mpeg2DefaultIntraMatrix = [64]byte{
	8, 16, 19, 22, 26, 27, 29, 34,
	16, 16, 22, 24, 27, 29, 34, 37,
	19, 22, 26, 27, 29, 34, 34, 38,
	22, 22, 26, 27, 29, 34, 37, 40,
	22, 26, 27, 29, 32, 35, 40, 48,
	26, 27, 29, 32, 35, 40, 48, 58,
	26, 27, 29, 34, 38, 46, 56, 69,
	27, 29, 35, 38, 46, 56, 69, 83,
}

// Some encoders signal an intra matrix that is effectively "flat" (only DC differs).
// MediaInfo still reports this as Default.
var mpeg2DefaultIntraMatrixFlat = [64]byte{
	8, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
}

var mpeg2DefaultNonIntraMatrix = [64]byte{
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
}

func isDefaultMPEG2Matrix(m [64]byte, def [64]byte) bool {
	for i := 0; i < 64; i++ {
		if m[i] != def[i] {
			return false
		}
	}
	return true
}

func isDefaultMPEG2IntraMatrix(m [64]byte) bool {
	return isDefaultMPEG2Matrix(m, mpeg2DefaultIntraMatrix) || isDefaultMPEG2Matrix(m, mpeg2DefaultIntraMatrixFlat)
}

func formatMPEG2MatrixHex(m [64]byte) string {
	const hex = "0123456789ABCDEF"
	out := make([]byte, 0, 128)
	for i := 0; i < 64; i++ {
		b := m[i]
		out = append(out, hex[b>>4], hex[b&0x0f])
	}
	return string(out)
}

type mpeg2VideoInfo struct {
	Width                    uint64
	Height                   uint64
	AspectRatio              string
	FrameRate                float64
	FrameRateNumer           uint32
	FrameRateDenom           uint32
	Profile                  string
	Version                  string
	BitRate                  int64
	BitRateMode              string
	MaxBitRateKbps           int64
	BVOP                     *bool
	Matrix                   string
	GOPLength                int
	GOPVariable              bool
	GOPM                     int
	GOPN                     int
	GOPLengthFirst           int
	GOPOpenClosed            string
	GOPFirstClosed           string
	GOPDropFrame             *bool
	GOPClosed                *bool
	GOPBrokenLink            *bool
	TimeCode                 string
	TimeCodeSource           string
	ColourDescriptionPresent bool
	ColourPrimaries          string
	TransferCharacteristics  string
	MatrixCoefficients       string
	ColorSpace               string
	ChromaSubsampling        string
	BitDepth                 string
	ScanType                 string
	ScanOrder                string
	PictureStructure         string
	MatrixData               string
	BufferSize               int64
	IntraDCPrecision         int
	IntraDCPrecisionFirst    int
	IntraDCPrecisionLast     int
}

type mpeg2VideoParser struct {
	carry             []byte
	info              mpeg2VideoInfo
	rescanFromZero    bool
	expectPictureExt  bool
	intraFrozen       bool
	intraFreezeAfterI int
	firstIntraDCOk    bool
	firstIntraDC      int
	lastIntraDCOk     bool
	lastIntraDC       int
	currentGOPCount   int
	sawGOP            bool
	gopLength         int
	gopLengthCounts   map[int]int
	gopM              int
	gopN              int
	gopMVariable      bool
	gopNVariable      bool
	gopMCounts        map[int]int
	gopNCounts        map[int]int
	framesSinceI      int
	framesSinceAnchor int
	lastISeen         bool
	lastAnchorSeen    bool
	iFrameCount       int
	pFrameCount       int
	maxBitRateKbps    int64
	maxBitRateSet     bool
	maxBitRateMixed   bool
	firstGOPClosed    *bool
	anyOpenGOP        bool
	gotSeqExt         bool
	sawSequence       bool
	progressiveSeq    bool
	pictureCount      int
	progressiveFrames int
	repeatFirstField  int
	topFieldFirst     int
	intraDCCounts     map[int]int
	pictureStructures map[int]int
}

func (p *mpeg2VideoParser) recordGOPMCount() {
	if p.lastAnchorSeen && p.framesSinceAnchor > 0 {
		if p.gopMCounts == nil {
			p.gopMCounts = map[int]int{}
		}
		p.gopMCounts[p.framesSinceAnchor]++
	}
	p.framesSinceAnchor = 0
	p.lastAnchorSeen = true
}

func (p *mpeg2VideoParser) consume(data []byte) {
	const maxCarry = 4096
	buf := append(append([]byte{}, p.carry...), data...)
	// Only scan the new region plus enough overlap for start code prefixes.
	start := 0
	if !p.rescanFromZero && len(p.carry) > 3 {
		start = len(p.carry) - 3
	}
	p.rescanFromZero = false
	pending := -1
	for i := start; i+4 <= len(buf); i++ {
		if buf[i] != 0x00 || buf[i+1] != 0x00 || buf[i+2] != 0x01 {
			continue
		}
		code := buf[i+3]
		// picture coding extension is only valid immediately after picture_header().
		// Clear the expectation when we hit anything that implies picture data.
		if code >= 0x01 && code <= 0xAF {
			p.expectPictureExt = false
		}
		// Bound parsing to this start code's payload (up to the next start code). Without this,
		// short extension payloads can read into the next start code and corrupt bit parsing.
		end := nextStartCode(buf, i+4)
		if end < 0 {
			pending = i
			break
		}
		payload := buf[i+4 : end]
		switch code {
		case 0xB3:
			p.parseSequenceHeader(payload)
		case 0xB5:
			p.parseExtension(payload)
		case 0xB8:
			p.parseGOPHeader(payload)
		case 0x00:
			p.expectPictureExt = true
			p.parsePictureHeader(payload)
		}
	}
	if pending >= 0 {
		p.carry = append(p.carry[:0], buf[pending:]...)
		p.rescanFromZero = true
	} else if len(buf) > maxCarry {
		p.carry = append(p.carry[:0], buf[len(buf)-maxCarry:]...)
	} else {
		p.carry = append(p.carry[:0], buf...)
	}
}

func (p *mpeg2VideoParser) parseSequenceHeader(data []byte) {
	if len(data) < 8 {
		return
	}
	p.sawSequence = true
	br := newBitReader(data)
	width := br.readBitsValue(12)
	height := br.readBitsValue(12)
	aspect := br.readBitsValue(4)
	frameRateCode := br.readBitsValue(4)
	bitRateValue := br.readBitsValue(18)
	_ = br.readBitsValue(1)
	bufferSize := br.readBitsValue(10)
	_ = br.readBitsValue(1)
	loadIntra := br.readBitsValue(1)
	customMatrix := false
	matrixKnown := false
	intraMatrixData := ""
	nonIntraMatrixData := ""
	if loadIntra == 1 {
		var m [64]byte
		ok := true
		for i := 0; i < 64; i++ {
			v := br.readBitsValue(8)
			if v == ^uint64(0) {
				// Sequence header may span packets; don't treat missing matrix bytes as "Custom".
				return
			}
			m[i] = byte(v)
		}
		if ok {
			matrixKnown = true
			if !isDefaultMPEG2IntraMatrix(m) {
				customMatrix = true
				intraMatrixData = formatMPEG2MatrixHex(m)
			}
		}
	}
	loadNonIntra := br.readBitsValue(1)
	if loadNonIntra == 1 {
		var m [64]byte
		ok := true
		for i := 0; i < 64; i++ {
			v := br.readBitsValue(8)
			if v == ^uint64(0) {
				// Sequence header may span packets; don't treat missing matrix bytes as "Custom".
				return
			}
			m[i] = byte(v)
		}
		if ok {
			matrixKnown = true
			if !isDefaultMPEG2Matrix(m, mpeg2DefaultNonIntraMatrix) {
				customMatrix = true
				nonIntraMatrixData = formatMPEG2MatrixHex(m)
			}
		}
	}
	if p.info.MatrixData == "" {
		switch {
		case intraMatrixData != "" && nonIntraMatrixData != "":
			p.info.MatrixData = intraMatrixData + " / " + nonIntraMatrixData
		case intraMatrixData != "":
			p.info.MatrixData = intraMatrixData
		case nonIntraMatrixData != "":
			p.info.MatrixData = nonIntraMatrixData
		}
	}

	if width > 0 && height > 0 {
		p.info.Width = width
		p.info.Height = height
	}
	if p.info.AspectRatio == "" {
		p.info.AspectRatio = mapMPEG2AspectRatio(aspect)
	}
	if p.info.FrameRate == 0 {
		if rate, num, den := mapMPEG2FrameRate(frameRateCode); rate > 0 {
			p.info.FrameRate = rate
			p.info.FrameRateNumer = num
			p.info.FrameRateDenom = den
		}
	}
	if p.info.Matrix == "" {
		if loadIntra == 1 || loadNonIntra == 1 {
			if customMatrix {
				p.info.Matrix = "Custom"
			} else if matrixKnown {
				p.info.Matrix = "Default"
			} else {
				p.info.Matrix = "Custom"
			}
		} else {
			p.info.Matrix = "Default"
		}
	}
	if bufferSize > 0 && p.info.BufferSize == 0 {
		p.info.BufferSize = int64(bufferSize) * 2048
	}
	if p.info.ColorSpace == "" {
		p.info.ColorSpace = "YUV"
	}
	if p.info.BitDepth == "" {
		p.info.BitDepth = "8 bits"
	}
	if bitRateValue != 0x3FFFF {
		if p.info.BitRate == 0 {
			p.info.BitRate = int64(bitRateValue) * 400
		}
		maxKbps := int64(bitRateValue*400) / 1000
		if !p.maxBitRateSet {
			p.maxBitRateKbps = maxKbps
			p.maxBitRateSet = true
		} else if maxKbps != p.maxBitRateKbps {
			p.maxBitRateMixed = true
		}
		if p.info.BitRateMode == "" {
			p.info.BitRateMode = "Variable"
		}
	} else if p.info.BitRateMode == "" {
		p.info.BitRateMode = "Variable"
	}
}

func (p *mpeg2VideoParser) parseExtension(data []byte) {
	if len(data) < 2 {
		return
	}
	br := newBitReader(data)
	extID := br.readBitsValue(4)
	switch extID {
	case 1:
		profileLevel := br.readBitsValue(8)
		progressive := br.readBitsValue(1)
		if progressive == ^uint64(0) {
			return
		}
		chromaFormat := br.readBitsValue(2)
		_ = br.readBitsValue(2)
		_ = br.readBitsValue(2)
		_ = br.readBitsValue(12)
		_ = br.readBitsValue(1)
		_ = br.readBitsValue(8)
		_ = br.readBitsValue(1)
		_ = br.readBitsValue(2)
		_ = br.readBitsValue(5)
		p.info.Profile = mapMPEG2Profile(profileLevel)
		p.info.Version = "Version 2"
		p.progressiveSeq = progressive == 1
		p.info.ChromaSubsampling = mapMPEG2Chroma(chromaFormat)
		p.gotSeqExt = true
	case 2:
		// Sequence display extension.
		// ISO/IEC 13818-2: video_format (3), colour_description (1),
		// colour_primaries (8), transfer_characteristics (8), matrix_coefficients (8)
		// then display sizes. We only need the color description values for parity.
		_ = br.readBitsValue(3) // video_format
		colourDesc := br.readBitsValue(1)
		if colourDesc == ^uint64(0) {
			return
		}
		if colourDesc == 1 {
			primaries := br.readBitsValue(8)
			transfer := br.readBitsValue(8)
			matrix := br.readBitsValue(8)
			if primaries == ^uint64(0) || transfer == ^uint64(0) || matrix == ^uint64(0) {
				return
			}
			p.info.ColourDescriptionPresent = true
			p.info.ColourPrimaries = mapMPEG2ColourPrimaries(byte(primaries))
			p.info.TransferCharacteristics = mapMPEG2TransferCharacteristics(byte(transfer))
			p.info.MatrixCoefficients = mapMPEG2MatrixCoefficients(byte(matrix))
		}
	case 8:
		if !p.expectPictureExt {
			return
		}
		// Only one picture coding extension applies to a given picture header.
		p.expectPictureExt = false
		fcode := br.readBitsValue(16)
		if fcode == ^uint64(0) {
			return
		}
		intra := br.readBitsValue(2)
		pictureStructure := br.readBitsValue(2)
		topFieldFirst := br.readBitsValue(1)
		_ = br.readBitsValue(1) // frame_pred_frame_dct
		_ = br.readBitsValue(1) // concealment_motion_vectors
		_ = br.readBitsValue(1) // q_scale_type
		_ = br.readBitsValue(1) // intra_vlc_format
		_ = br.readBitsValue(1) // alternate_scan
		repeatFirstField := br.readBitsValue(1)
		_ = br.readBitsValue(1) // chroma_420_type
		progressiveFrame := br.readBitsValue(1)
		compositeDisplay := br.readBitsValue(1)
		if compositeDisplay == 1 {
			_ = br.readBitsValue(1) // v_axis
			_ = br.readBitsValue(2) // field_sequence
			_ = br.readBitsValue(1) // sub_carrier
			_ = br.readBitsValue(5) // burst_amplitude
			_ = br.readBitsValue(5) // sub_carrier_phase
		}
		if intra != ^uint64(0) {
			if !p.firstIntraDCOk {
				p.firstIntraDCOk = true
				p.firstIntraDC = int(intra)
			}
			if !p.intraFrozen {
				p.lastIntraDCOk = true
				p.lastIntraDC = int(intra)
				if p.intraDCCounts == nil {
					p.intraDCCounts = map[int]int{}
				}
				p.intraDCCounts[int(intra)]++
			}
		}
		if pictureStructure != ^uint64(0) {
			if p.pictureStructures == nil {
				p.pictureStructures = map[int]int{}
			}
			p.pictureStructures[int(pictureStructure)]++
			p.pictureCount++
			if progressiveFrame == 1 {
				p.progressiveFrames++
			}
			if repeatFirstField == 1 {
				p.repeatFirstField++
			}
			if topFieldFirst == 1 {
				p.topFieldFirst++
			}
		}
	default:
		return
	}
}

func (p *mpeg2VideoParser) parseGOPHeader(data []byte) {
	if len(data) < 4 {
		return
	}
	br := newBitReader(data)
	dropFrame := br.readBitsValue(1)
	hours := br.readBitsValue(5)
	minutes := br.readBitsValue(6)
	_ = br.readBitsValue(1)
	seconds := br.readBitsValue(6)
	pictures := br.readBitsValue(6)
	closed := br.readBitsValue(1)
	broken := br.readBitsValue(1)

	if p.info.TimeCode == "" {
		sep := ":"
		if dropFrame == 1 {
			sep = ";"
		}
		p.info.TimeCode = fmt.Sprintf("%02d:%02d:%02d%s%02d", hours, minutes, seconds, sep, pictures)
		p.info.TimeCodeSource = "Group of pictures header"
	}
	if p.info.TimeCodeSource == "" {
		p.info.TimeCodeSource = "Group of pictures header"
	}
	closedBool := closed == 1
	if p.firstGOPClosed == nil {
		p.firstGOPClosed = &closedBool
		if closedBool {
			p.info.GOPFirstClosed = "Closed"
		}
	}
	if p.info.GOPDropFrame == nil {
		val := dropFrame == 1
		p.info.GOPDropFrame = &val
	}
	if p.info.GOPClosed == nil {
		val := closed == 1
		p.info.GOPClosed = &val
	}
	if p.info.GOPBrokenLink == nil {
		val := broken == 1
		p.info.GOPBrokenLink = &val
	}
	if !closedBool {
		p.anyOpenGOP = true
	}

	if p.sawGOP && p.currentGOPCount > 0 {
		if p.gopLengthCounts == nil {
			p.gopLengthCounts = map[int]int{}
		}
		p.gopLengthCounts[p.currentGOPCount]++
		if p.gopLength == 0 {
			p.gopLength = p.currentGOPCount
		}
		if p.gopLengthCounts == nil {
			p.gopLengthCounts = map[int]int{}
		}
		p.gopLengthCounts[p.currentGOPCount]++
	}
	p.currentGOPCount = 0
	p.sawGOP = true
}

func (p *mpeg2VideoParser) parsePictureHeader(data []byte) {
	if len(data) < 2 {
		return
	}
	br := newBitReader(data)
	_ = br.readBitsValue(10)
	codingType := br.readBitsValue(3)
	if codingType == 3 {
		val := true
		p.info.BVOP = &val
	}
	if p.sawGOP {
		p.currentGOPCount++
	}

	if p.lastISeen {
		p.framesSinceI++
	}
	if p.lastAnchorSeen {
		p.framesSinceAnchor++
	}

	switch codingType {
	case 1: // I
		// MediaInfoLib can stop deep parsing after it has "enough frames" (around 8 I-frames)
		// at default ParseSpeed, then jump around the file. Freeze intra_dc_precision updates
		// after the threshold so we keep the last value from the initial scan window.
		if p.intraFreezeAfterI > 0 && p.iFrameCount >= p.intraFreezeAfterI {
			p.intraFrozen = true
		}
		p.iFrameCount++
		if p.lastISeen && p.framesSinceI > 0 {
			if p.gopNCounts == nil {
				p.gopNCounts = map[int]int{}
			}
			p.gopNCounts[p.framesSinceI]++
		}
		p.framesSinceI = 0
		p.lastISeen = true
		p.recordGOPMCount()
	case 2: // P
		p.pFrameCount++
		p.recordGOPMCount()
	}
}

func (p *mpeg2VideoParser) finalize() mpeg2VideoInfo {
	// Record the final GOP length if the stream ends without a following GOP header.
	if p.sawGOP && p.currentGOPCount > 0 {
		if p.gopLength == 0 {
			p.gopLength = p.currentGOPCount
		}
		if p.gopLengthCounts == nil {
			p.gopLengthCounts = map[int]int{}
		}
		p.gopLengthCounts[p.currentGOPCount]++
	}
	if len(p.gopLengthCounts) > 0 {
		mode, variable := modeValue(p.gopLengthCounts)
		total := 0
		modeCount := 0
		for key, count := range p.gopLengthCounts {
			total += count
			if key == mode {
				modeCount = count
			}
		}
		// MediaInfo tends to report the dominant GOP length instead of "Variable" if most GOPs match.
		if !variable || (total > 0 && float64(modeCount)/float64(total) >= 0.50) {
			p.info.GOPLength = mode
		} else {
			p.info.GOPVariable = true
		}
	} else if p.gopLength > 0 {
		p.info.GOPLength = p.gopLength
	}
	if p.gopN > 0 && !p.gopNVariable {
		p.info.GOPN = p.gopN
	}
	if p.gopM > 0 && !p.gopMVariable {
		p.info.GOPM = p.gopM
	}
	if p.info.GOPM == 0 && len(p.gopMCounts) > 0 {
		p.info.GOPM, p.gopMVariable = modeValue(p.gopMCounts)
	}
	if p.info.GOPN == 0 && len(p.gopNCounts) > 0 {
		p.info.GOPN, p.gopNVariable = modeValue(p.gopNCounts)
	}
	if p.gopMVariable || p.gopNVariable {
		p.info.GOPM = 0
		p.info.GOPN = 0
	}
	if p.maxBitRateSet {
		if p.maxBitRateMixed {
			if p.info.Width == 720 && (p.info.Height == 480 || p.info.Height == 576) {
				p.info.MaxBitRateKbps = p.maxBitRateKbps
			} else {
				p.info.MaxBitRateKbps = 0
			}
		} else {
			p.info.MaxBitRateKbps = p.maxBitRateKbps
		}
	}
	if p.info.GOPLengthFirst == 0 && p.gopLength > 0 {
		p.info.GOPLengthFirst = p.gopLength
	}
	if p.info.BVOP == nil {
		val := false
		p.info.BVOP = &val
	}
	if p.info.GOPOpenClosed == "" {
		if p.anyOpenGOP {
			p.info.GOPOpenClosed = "Open"
		} else if p.firstGOPClosed != nil {
			p.info.GOPOpenClosed = "Closed"
		}
	}
	if p.info.Matrix == "" {
		p.info.Matrix = "Default"
	}
	if p.info.PictureStructure == "" && len(p.pictureStructures) > 0 {
		mode, _ := modeValue(p.pictureStructures)
		p.info.PictureStructure = mapMPEG2PictureStructure(mode)
	}
	if len(p.intraDCCounts) > 0 {
		mode, _ := modeValue(p.intraDCCounts)
		p.info.IntraDCPrecision = 8 + mode
	}
	if p.info.ColorSpace == "" {
		p.info.ColorSpace = "YUV"
	}
	if p.info.ChromaSubsampling == "" {
		p.info.ChromaSubsampling = "4:2:0"
	}
	if p.info.BitDepth == "" {
		p.info.BitDepth = "8 bits"
	}
	if p.progressiveSeq || p.progressiveFrames > 0 {
		p.info.ScanType = "Progressive"
	} else if p.info.ScanType == "" {
		p.info.ScanType = "Interlaced"
	}
	if p.info.ScanOrder == "" && p.topFieldFirst > 0 && p.pictureCount > 0 {
		// Official mediainfo still surfaces TFF in some Progressive streams (e.g. DVD menu VOBs).
		p.info.ScanOrder = "TFF"
	}
	if p.progressiveSeq && p.repeatFirstField > 0 && p.info.FrameRate > 0 {
		if (p.info.FrameRateNumer == 30000 && p.info.FrameRateDenom == 1001) || math.Abs(p.info.FrameRate-29.97) < 0.02 {
			p.info.FrameRate = 24000.0 / 1001.0
			p.info.FrameRateNumer = 24000
			p.info.FrameRateDenom = 1001
			p.info.ScanOrder = "2:3 Pulldown"
			p.info.ScanType = "Progressive"
		}
	}
	return p.info
}

func (p *mpeg2VideoParser) finalizeTS() mpeg2VideoInfo {
	info := p.finalize()
	// TS parity: keep the dominant GOP length when variation is sparse; use "Variable"
	// only when no clear dominant mode emerges.
	if len(p.gopLengthCounts) > 0 {
		mode, variable := modeValue(p.gopLengthCounts)
		if variable {
			total := 0
			modeCount := 0
			for key, count := range p.gopLengthCounts {
				total += count
				if key == mode {
					modeCount = count
				}
			}
			if total > 0 && float64(modeCount)/float64(total) >= 0.95 {
				info.GOPVariable = false
				info.GOPLength = mode
			} else {
				info.GOPVariable = true
				info.GOPLength = 0
			}
		} else {
			info.GOPVariable = false
			info.GOPLength = mode
		}
	}
	// TS parity: MediaInfoLib keeps the last parsed intra_dc_precision (not a mode).
	if p.lastIntraDCOk {
		info.IntraDCPrecision = 8 + p.lastIntraDC
		info.IntraDCPrecisionLast = info.IntraDCPrecision
	}
	if p.firstIntraDCOk {
		info.IntraDCPrecisionFirst = 8 + p.firstIntraDC
	}
	// TS parity: on interlaced streams, MediaInfo still reports dominant M/N GOP values
	// and frame picture structure even when GOP lengths vary.
	if info.ScanType == "Interlaced" {
		if info.GOPM == 0 && len(p.gopMCounts) > 0 {
			mode, _ := modeValue(p.gopMCounts)
			info.GOPM = mode
		}
		if info.GOPN == 0 && len(p.gopNCounts) > 0 {
			mode, _ := modeValue(p.gopNCounts)
			info.GOPN = mode
		}
		if info.PictureStructure == "" && info.ScanOrder != "" {
			info.PictureStructure = "Frame"
		}
	}
	return info
}

func mapMPEG2AspectRatio(code uint64) string {
	switch code {
	case 1:
		// Square pixels: display aspect ratio is derived from stored width/height.
		return ""
	case 2:
		return "4:3"
	case 3:
		return "16:9"
	case 4:
		return "2.21:1"
	default:
		return ""
	}
}

func mapMPEG2Standard(frameRate float64) string {
	switch {
	case frameRate > 0 && math.Abs(frameRate-29.97) < 0.01:
		return "NTSC"
	case frameRate > 0 && math.Abs(frameRate-30.0) < 0.01:
		return "NTSC"
	case frameRate > 0 && math.Abs(frameRate-25.0) < 0.01:
		return "PAL"
	default:
		return ""
	}
}

func mapMPEG2ColourPrimaries(code byte) string {
	switch code {
	case 1:
		return "BT.709"
	case 4:
		return "BT.470 M"
	case 5:
		return "BT.470 BG"
	case 6:
		// SMPTE 170M
		return "BT.601 NTSC"
	case 7:
		return "SMPTE 240M"
	case 9:
		return "BT.2020"
	default:
		return ""
	}
}

func mapMPEG2TransferCharacteristics(code byte) string {
	switch code {
	case 1:
		return "BT.709"
	case 4:
		return "Gamma 2.2"
	case 5:
		return "Gamma 2.8"
	case 6:
		// SMPTE 170M
		return "BT.601"
	case 7:
		return "SMPTE 240M"
	case 13:
		return "sRGB"
	case 14:
		return "BT.2020 (10-bit)"
	case 15:
		return "BT.2020 (12-bit)"
	default:
		return ""
	}
}

func mapMPEG2MatrixCoefficients(code byte) string {
	switch code {
	case 1:
		return "BT.709"
	case 5:
		return "BT.470 BG"
	case 6:
		// SMPTE 170M
		return "BT.601"
	case 7:
		return "SMPTE 240M"
	case 9:
		return "BT.2020 non-constant"
	case 10:
		return "BT.2020 constant"
	default:
		return ""
	}
}

func mapMPEG2FrameRate(code uint64) (float64, uint32, uint32) {
	switch code {
	case 1:
		return 24000.0 / 1001.0, 24000, 1001
	case 2:
		return 24.0, 24, 1
	case 3:
		return 25.0, 25, 1
	case 4:
		return 30000.0 / 1001.0, 30000, 1001
	case 5:
		return 30.0, 30, 1
	case 6:
		return 50.0, 50, 1
	case 7:
		return 60000.0 / 1001.0, 60000, 1001
	case 8:
		return 60.0, 60, 1
	default:
		return 0, 0, 0
	}
}

func mapMPEG2Chroma(code uint64) string {
	switch code {
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

func mapMPEG2PictureStructure(code int) string {
	switch code {
	case 1, 2:
		return "Field"
	case 3:
		return "Frame"
	default:
		return ""
	}
}

func mapMPEG2Profile(profileLevel uint64) string {
	profile := (profileLevel >> 4) & 0x0F
	level := profileLevel & 0x0F
	profileStr := ""
	levelStr := ""
	switch profile {
	case 0x1:
		profileStr = "High"
	case 0x2:
		profileStr = "Spatial"
	case 0x3:
		profileStr = "SNR"
	case 0x4:
		profileStr = "Main"
	case 0x5:
		profileStr = "Simple"
	}
	switch level {
	case 0x4:
		levelStr = "High"
	case 0x6:
		levelStr = "High 1440"
	case 0x8:
		levelStr = "Main"
	case 0xA:
		levelStr = "Low"
	}
	if profileStr == "" || levelStr == "" {
		return ""
	}
	return fmt.Sprintf("%s@%s", profileStr, levelStr)
}

func modeValue(counts map[int]int) (int, bool) {
	if len(counts) == 0 {
		return 0, false
	}
	value := 0
	maxCount := -1
	for key, count := range counts {
		if count > maxCount {
			maxCount = count
			value = key
		}
	}
	return value, len(counts) > 1
}
