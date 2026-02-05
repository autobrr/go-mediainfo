package mediainfo

import (
	"fmt"
	"math"
	"strings"
)

type mpeg2VideoInfo struct {
	Width             uint64
	Height            uint64
	AspectRatio       string
	FrameRate         float64
	FrameRateNumer    uint32
	FrameRateDenom    uint32
	Profile           string
	Version           string
	BitRate           int64
	BitRateMode       string
	MaxBitRateKbps    int64
	BVOP              *bool
	Matrix            string
	GOPLength         int
	GOPVariable       bool
	GOPM              int
	GOPN              int
	GOPLengthFirst    int
	GOPOpenClosed     string
	GOPFirstClosed    string
	GOPDropFrame      *bool
	GOPClosed         *bool
	GOPBrokenLink     *bool
	TimeCode          string
	TimeCodeSource    string
	ColorSpace        string
	ChromaSubsampling string
	BitDepth          string
	ScanType          string
	ScanOrder         string
	MatrixData        string
	BufferSize        int64
	IntraDCPrecision  int
}

type mpeg2VideoParser struct {
	carry             []byte
	info              mpeg2VideoInfo
	currentGOPCount   int
	sawGOP            bool
	gopLength         int
	gopVariable       bool
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
}

func (p *mpeg2VideoParser) consume(data []byte) {
	buf := append(append([]byte{}, p.carry...), data...)
	for i := 0; i+4 <= len(buf); i++ {
		if buf[i] != 0x00 || buf[i+1] != 0x00 || buf[i+2] != 0x01 {
			continue
		}
		code := buf[i+3]
		switch code {
		case 0xB3:
			p.parseSequenceHeader(buf[i+4:])
		case 0xB5:
			p.parseExtension(buf[i+4:])
		case 0xB8:
			p.parseGOPHeader(buf[i+4:])
		case 0x00:
			p.parsePictureHeader(buf[i+4:])
		}
	}
	if len(buf) >= 3 {
		p.carry = append(p.carry[:0], buf[len(buf)-3:]...)
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
	if loadIntra == 1 {
		matrix := make([]byte, 0, 64)
		for range 64 {
			value := br.readBitsValue(8)
			if len(matrix) < 64 {
				matrix = append(matrix, byte(value))
			}
		}
		if p.info.MatrixData == "" && len(matrix) == 64 {
			var builder strings.Builder
			builder.Grow(128)
			for _, b := range matrix {
				fmt.Fprintf(&builder, "%02X", b)
			}
			p.info.MatrixData = builder.String()
		}
	}
	loadNonIntra := br.readBitsValue(1)
	if loadNonIntra == 1 {
		matrix := make([]byte, 0, 64)
		for range 64 {
			value := br.readBitsValue(8)
			if len(matrix) < 64 {
				matrix = append(matrix, byte(value))
			}
		}
		if p.info.MatrixData == "" && len(matrix) == 64 {
			var builder strings.Builder
			builder.Grow(128)
			for _, b := range matrix {
				fmt.Fprintf(&builder, "%02X", b)
			}
			p.info.MatrixData = builder.String()
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
			p.info.Matrix = "Custom"
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
	case 8:
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
		if intra != ^uint64(0) && p.info.IntraDCPrecision == 0 {
			p.info.IntraDCPrecision = int(8 + intra)
		}
		if pictureStructure != ^uint64(0) {
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
		} else {
			p.info.GOPFirstClosed = "Open"
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
		if p.gopLength == 0 {
			p.gopLength = p.currentGOPCount
		} else if p.currentGOPCount != p.gopLength {
			p.gopVariable = true
		}
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
		p.iFrameCount++
		if p.lastISeen && p.framesSinceI > 0 {
			if p.gopNCounts == nil {
				p.gopNCounts = map[int]int{}
			}
			p.gopNCounts[p.framesSinceI]++
		}
		p.framesSinceI = 0
		p.lastISeen = true
		if p.lastAnchorSeen && p.framesSinceAnchor > 0 {
			if p.gopMCounts == nil {
				p.gopMCounts = map[int]int{}
			}
			p.gopMCounts[p.framesSinceAnchor]++
		}
		p.framesSinceAnchor = 0
		p.lastAnchorSeen = true
	case 2: // P
		p.pFrameCount++
		if p.lastAnchorSeen && p.framesSinceAnchor > 0 {
			if p.gopMCounts == nil {
				p.gopMCounts = map[int]int{}
			}
			p.gopMCounts[p.framesSinceAnchor]++
		}
		p.framesSinceAnchor = 0
		p.lastAnchorSeen = true
	}
}

func (p *mpeg2VideoParser) finalize() mpeg2VideoInfo {
	switch {
	case p.gopVariable:
		p.info.GOPVariable = true
	case p.gopLength > 0:
		p.info.GOPLength = p.gopLength
	case p.currentGOPCount > 0:
		p.info.GOPLength = p.currentGOPCount
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
		p.info.GOPVariable = true
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

func mapMPEG2AspectRatio(code uint64) string {
	switch code {
	case 1:
		return "1:1"
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
