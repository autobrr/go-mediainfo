package mediainfo

import "strings"

type mpeg4VisualInfo struct {
	Profile           string
	BVOP              *bool
	QPel              *bool
	GMC               string
	Matrix            string
	MatrixData        string
	ColorSpace        string
	ChromaSubsampling string
	BitDepth          string
	ScanType          string
	WritingLibrary    string
}

func parseMPEG4Visual(data []byte) mpeg4VisualInfo {
	info := mpeg4VisualInfo{}
	startCodes := findMPEG4StartCodes(data)
	for i, sc := range startCodes {
		if sc.code == 0xB0 && sc.pos+4 < len(data) {
			// Prefer the first Visual Object Sequence Start profile-level indication.
			if info.Profile != "" {
				continue
			}
			if profile := mapMPEG4Profile(data[sc.pos+4]); profile != "" {
				info.Profile = profile
			}
		}
		if sc.code == 0xB2 {
			end := len(data)
			if i+1 < len(startCodes) {
				end = startCodes[i+1].pos
			}
			if sc.pos+4 < end {
				value := string(data[sc.pos+4 : end])
				value = strings.Trim(value, "\x00\r\n\t ")
				if value != "" {
					info.WritingLibrary = value
				}
			}
		}
		if sc.code >= 0x20 && sc.code <= 0x2F {
			if sc.pos+4 < len(data) {
				vol := parseMPEG4VOL(data[sc.pos+4:])
				if vol.ChromaSubsampling != "" {
					info.ChromaSubsampling = vol.ChromaSubsampling
					info.ColorSpace = "YUV"
				}
				if vol.BitDepth != "" {
					info.BitDepth = vol.BitDepth
				}
				if vol.ScanType != "" {
					info.ScanType = vol.ScanType
				}
				if vol.Matrix != "" {
					info.Matrix = vol.Matrix
				}
				if vol.MatrixData != "" {
					info.MatrixData = vol.MatrixData
				}
				info.QPel = vol.QPel
				info.GMC = vol.GMC
			}
		}
		if sc.code == 0xB6 && sc.pos+4 < len(data) {
			vopType := (data[sc.pos+4] >> 6) & 0x03
			if vopType == 2 {
				val := true
				info.BVOP = &val
			}
		}
	}
	if info.BVOP == nil {
		val := false
		info.BVOP = &val
	}
	if info.QPel == nil {
		val := false
		info.QPel = &val
	}
	if info.GMC == "" {
		info.GMC = "No warppoints"
	}
	if info.Matrix == "" {
		info.Matrix = "Default (H.263)"
	}
	if info.ChromaSubsampling == "" {
		info.ChromaSubsampling = "4:2:0"
		info.ColorSpace = "YUV"
	}
	if info.BitDepth == "" {
		info.BitDepth = "8 bits"
	}
	if info.ScanType == "" {
		info.ScanType = "Progressive"
	}
	return info
}

type mpeg4StartCode struct {
	pos  int
	code byte
}

func findMPEG4StartCodes(data []byte) []mpeg4StartCode {
	codes := []mpeg4StartCode{}
	for i := 0; i+3 < len(data); i++ {
		if data[i] == 0x00 && data[i+1] == 0x00 && data[i+2] == 0x01 {
			codes = append(codes, mpeg4StartCode{pos: i, code: data[i+3]})
		}
	}
	return codes
}

type mpeg4VOLInfo struct {
	ChromaSubsampling string
	BitDepth          string
	ScanType          string
	QPel              *bool
	GMC               string
	Matrix            string
	MatrixData        string
}

func parseMPEG4VOL(data []byte) mpeg4VOLInfo {
	// ISO/IEC 14496-2 Visual Object Layer (VOL) parsing.
	br := newMPEG4BitReader(data)
	_ = br.readBitsValue(1) // random_accessible_vol
	_ = br.readBitsValue(8) // video_object_type_indication
	volVerID := uint64(1)
	if br.readBitsValue(1) == 1 { // is_object_layer_identifier
		volVerID = br.readBitsValue(4) // video_object_layer_verid
		_ = br.readBitsValue(3)        // video_object_layer_priority
	}
	aspectRatioInfo := br.readBitsValue(4)
	if aspectRatioInfo == 15 {
		_ = br.readBitsValue(8) // par_width
		_ = br.readBitsValue(8) // par_height
	}
	chromaFormat := uint64(1)
	if br.readBitsValue(1) == 1 { // vol_control_parameters
		chromaFormat = br.readBitsValue(2)
		_ = br.readBitsValue(1)
		if br.readBitsValue(1) == 1 {
			_ = br.readBitsValue(15)
			_ = br.readBitsValue(1)
			_ = br.readBitsValue(15)
			_ = br.readBitsValue(1)
			_ = br.readBitsValue(15)
			_ = br.readBitsValue(1)
			_ = br.readBitsValue(3)
			_ = br.readBitsValue(11)
			_ = br.readBitsValue(1)
			_ = br.readBitsValue(15)
			_ = br.readBitsValue(1)
		}
	}
	shape := br.readBitsValue(2) // video_object_layer_shape
	if shape == 3 {              // grayscale
		_ = br.readBitsValue(4)
	}
	_ = br.readBitsValue(1) // marker
	vopTimeIncrementResolution := br.readBitsValue(16)
	_ = br.readBitsValue(1) // marker
	if br.readBitsValue(1) == 1 {
		bits := bitLength(vopTimeIncrementResolution - 1)
		_ = br.readBitsValue(uint8(bits))
	}
	_ = br.readBitsValue(1)  // marker
	_ = br.readBitsValue(13) // width
	_ = br.readBitsValue(1)
	_ = br.readBitsValue(13) // height
	_ = br.readBitsValue(1)
	interlaced := br.readBitsValue(1) == 1
	_ = br.readBitsValue(1) // obmc_disable
	// sprite_enable is 1-bit for verid==1, else 2-bit.
	spriteEnable := br.readBitsValue(func() uint8 {
		if volVerID == 1 {
			return 1
		}
		return 2
	}())

	// Skip a subset of sprite-related fields (we only need the GMC "warppoints" count heuristically).
	if spriteEnable != 0 {
		_ = br.readBitsValue(13) // sprite_width
		_ = br.readBitsValue(1)
		_ = br.readBitsValue(13) // sprite_height
		_ = br.readBitsValue(1)
		_ = br.readBitsValue(13) // sprite_left
		_ = br.readBitsValue(1)
		_ = br.readBitsValue(13) // sprite_top
		_ = br.readBitsValue(1)
	}

	// Skip not_8_bit and quant precision fields when present.
	not8bit := br.readBitsValue(1) == 1
	if not8bit {
		_ = br.readBitsValue(4) // quant_precision
		_ = br.readBitsValue(4) // bits_per_pixel
	}
	quantType := br.readBitsValue(1) // quant_type

	var intraMatrix string
	var interMatrix string
	loadIntra := false
	loadInter := false
	if quantType == 1 {
		loadIntra = br.readBitsValue(1) == 1
		if loadIntra {
			intraMatrix = readMPEG4QuantMatrix(br)
		}
		loadInter = br.readBitsValue(1) == 1
		if loadInter {
			interMatrix = readMPEG4QuantMatrix(br)
		}
	}
	quarterSample := uint64(0)
	if volVerID != 1 {
		quarterSample = br.readBitsValue(1)
	}

	info := mpeg4VOLInfo{}
	info.ChromaSubsampling = mapMPEG4Chroma(chromaFormat)
	info.BitDepth = "8 bits"
	if interlaced {
		info.ScanType = "Interlaced"
	} else {
		info.ScanType = "Progressive"
	}
	if spriteEnable == 0 {
		info.GMC = "No warppoints"
	} else {
		info.GMC = "1 warppoint"
	}
	if loadIntra || loadInter {
		info.Matrix = "Custom"
		if intraMatrix != "" && interMatrix != "" {
			info.MatrixData = intraMatrix + " / " + interMatrix
		} else if intraMatrix != "" {
			info.MatrixData = intraMatrix
		}
	} else if quantType == 0 {
		info.Matrix = "Default (H.263)"
	} else {
		info.Matrix = "Custom"
	}
	qpel := quarterSample == 1
	info.QPel = &qpel
	return info
}

type mpeg4BitReader struct {
	data []byte
	pos  int
	bit  uint8
}

func newMPEG4BitReader(data []byte) *mpeg4BitReader {
	return &mpeg4BitReader{data: data}
}

func (br *mpeg4BitReader) readBitsValue(n uint8) uint64 {
	var value uint64
	for range n {
		if br.pos >= len(br.data) {
			return 0
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

func bitLength(value uint64) int {
	bits := 0
	for value > 0 {
		bits++
		value >>= 1
	}
	if bits == 0 {
		return 1
	}
	return bits
}

func skipMPEG4QuantMatrix(br *mpeg4BitReader) bool {
	last := 8
	for range 64 {
		if last == 0 {
			return true
		}
		value := int(br.readBitsValue(8))
		last = value
	}
	return true
}

func readMPEG4QuantMatrix(br *mpeg4BitReader) string {
	// Matrices are coded in zig-zag order; values are 8-bit each with early termination:
	// when a value is 0, the remaining entries are set to the last non-zero value.
	//
	// MediaInfo exposes the coded sequence as an uppercase hex string.
	last := 8
	out := make([]byte, 0, 64*2)
	for i := 0; i < 64; i++ {
		value := int(br.readBitsValue(8))
		if value == 0 {
			value = last
			for ; i < 64; i++ {
				out = appendHexByte(out, byte(value))
			}
			return string(out)
		}
		last = value
		out = appendHexByte(out, byte(value))
	}
	return string(out)
}

func appendHexByte(dst []byte, b byte) []byte {
	const hexdigits = "0123456789ABCDEF"
	dst = append(dst, hexdigits[b>>4])
	dst = append(dst, hexdigits[b&0x0F])
	return dst
}

func mapMPEG4Chroma(value uint64) string {
	switch value {
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

func mapMPEG4Profile(value byte) string {
	switch value {
	case 0x01:
		return "Simple@L1"
	case 0x02:
		return "Simple@L2"
	case 0x03:
		return "Simple@L3"
	case 0x04:
		return "Simple@L4a"
	case 0x05:
		return "Simple@L5"
	case 0x06:
		return "Simple@L6"
	case 0x08:
		return "Simple@L0"
	case 0x09:
		return "Simple@L0b"
	case 0xF1:
		return "Advanced Simple@L0"
	case 0xF2:
		return "Advanced Simple@L1"
	case 0xF3:
		return "Advanced Simple@L2"
	case 0xF4:
		return "Advanced Simple@L3"
	case 0xF5:
		return "Advanced Simple@L5"
	case 0xF7:
		return "Advanced Simple@L3b"
	default:
		return ""
	}
}
