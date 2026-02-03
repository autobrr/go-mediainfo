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
	BitRateMode       string
	MaxBitRateKbps    int64
	BVOP              *bool
	Matrix            string
	GOPLength         int
	GOPOpenClosed     string
	GOPFirstClosed    string
	TimeCode          string
	TimeCodeSource    string
	ColorSpace        string
	ChromaSubsampling string
	BitDepth          string
	ScanType          string
	MatrixData        string
	BufferSize        int64
	IntraDCPrecision  int
}

type mpeg2VideoParser struct {
	carry           []byte
	info            mpeg2VideoInfo
	currentGOPCount int
	sawGOP          bool
	firstGOPClosed  *bool
	anyOpenGOP      bool
	gotSeqExt       bool
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
		for i := 0; i < 64; i++ {
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
		for i := 0; i < 64; i++ {
			_ = br.readBitsValue(8)
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
		p.info.MaxBitRateKbps = int64(bitRateValue*400) / 1000
		if p.info.BitRateMode == "" {
			p.info.BitRateMode = "Constant"
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
		if progressive == 1 {
			p.info.ScanType = "Progressive"
		} else {
			p.info.ScanType = "Interlaced"
		}
		p.info.ChromaSubsampling = mapMPEG2Chroma(chromaFormat)
		p.gotSeqExt = true
	case 8:
		_ = br.readBitsValue(16) // f_code
		intra := br.readBitsValue(2)
		if p.info.IntraDCPrecision == 0 {
			p.info.IntraDCPrecision = int(8 + intra)
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
	_ = br.readBitsValue(1) // drop_frame
	hours := br.readBitsValue(5)
	minutes := br.readBitsValue(6)
	_ = br.readBitsValue(1)
	seconds := br.readBitsValue(6)
	pictures := br.readBitsValue(6)
	closed := br.readBitsValue(1)
	_ = br.readBitsValue(1)

	if p.info.TimeCode == "" {
		p.info.TimeCode = fmt.Sprintf("%02d:%02d:%02d:%02d", hours, minutes, seconds, pictures)
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
	if !closedBool {
		p.anyOpenGOP = true
	}

	if p.sawGOP && p.currentGOPCount > 0 && p.info.GOPLength == 0 {
		p.info.GOPLength = p.currentGOPCount
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
}

func (p *mpeg2VideoParser) finalize() mpeg2VideoInfo {
	if p.info.GOPLength == 0 && p.currentGOPCount > 0 {
		p.info.GOPLength = p.currentGOPCount
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
	if p.info.ScanType == "" {
		p.info.ScanType = "Progressive"
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
