package mediainfo

import (
	"bytes"
	"strings"
)

type ac3Info struct {
	bitRateKbps int64
	sampleRate  float64
	channels    uint64
	layout      string
	bsid        int
	bsmod       int
	serviceKind string
	frameRate   float64
	spf         int
}

type ac3BitReader struct {
	data   []byte
	bitPos int
}

func (br *ac3BitReader) readBits(n int) (uint32, bool) {
	if n <= 0 || br.bitPos+n > len(br.data)*8 {
		return 0, false
	}
	var value uint32
	for i := 0; i < n; i++ {
		byteVal := br.data[br.bitPos>>3]
		bit := (byteVal >> (7 - (br.bitPos & 7))) & 0x01
		value = (value << 1) | uint32(bit)
		br.bitPos++
	}
	return value, true
}

func parseAC3Header(payload []byte) (ac3Info, bool) {
	var info ac3Info
	idx := bytes.Index(payload, []byte{0x0B, 0x77})
	if idx < 0 || idx+7 > len(payload) {
		return info, false
	}
	br := ac3BitReader{data: payload[idx:]}
	if sync, ok := br.readBits(16); !ok || sync != 0x0B77 {
		return info, false
	}
	if _, ok := br.readBits(16); !ok { // crc1
		return info, false
	}
	fscod, ok := br.readBits(2)
	if !ok {
		return info, false
	}
	frmsizecod, ok := br.readBits(6)
	if !ok {
		return info, false
	}
	bsid, ok := br.readBits(5)
	if !ok {
		return info, false
	}
	bsmod, ok := br.readBits(3)
	if !ok {
		return info, false
	}
	acmod, ok := br.readBits(3)
	if !ok {
		return info, false
	}
	if acmod == 0 {
		if _, ok = br.readBits(2); !ok {
			return info, false
		}
		if _, ok = br.readBits(2); !ok {
			return info, false
		}
	} else {
		if acmod&1 != 0 {
			if _, ok = br.readBits(2); !ok {
				return info, false
			}
		}
		if acmod&4 != 0 {
			if _, ok = br.readBits(2); !ok {
				return info, false
			}
		}
	}
	if acmod == 2 {
		if _, ok = br.readBits(2); !ok {
			return info, false
		}
	}
	lfeonVal, ok := br.readBits(1)
	if !ok {
		return info, false
	}

	sampleRate := ac3SampleRate(int(fscod))
	bitRate := ac3BitrateKbps(int(frmsizecod))
	channels, layout := ac3ChannelLayout(int(acmod), lfeonVal == 1)
	frameRate := 0.0
	spf := 1536
	if sampleRate > 0 {
		frameRate = sampleRate / float64(spf)
	}

	info = ac3Info{
		bitRateKbps: bitRate,
		sampleRate:  sampleRate,
		channels:    channels,
		layout:      layout,
		bsid:        int(bsid),
		bsmod:       int(bsmod),
		serviceKind: ac3ServiceKind(int(bsmod)),
		frameRate:   frameRate,
		spf:         spf,
	}
	return info, true
}

func ac3SampleRate(code int) float64 {
	switch code {
	case 0:
		return 48000
	case 1:
		return 44100
	case 2:
		return 32000
	default:
		return 0
	}
}

func ac3BitrateKbps(code int) int64 {
	bitRates := []int64{32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 448, 512, 576, 640}
	if code < 0 || code > 37 {
		return 0
	}
	idx := code >> 1
	if idx < 0 || idx >= len(bitRates) {
		return 0
	}
	return bitRates[idx]
}

func ac3ChannelLayout(acmod int, lfeon bool) (uint64, string) {
	var layout []string
	switch acmod {
	case 0:
		layout = []string{"L", "R"}
	case 1:
		layout = []string{"C"}
	case 2:
		layout = []string{"L", "R"}
	case 3:
		layout = []string{"L", "R", "C"}
	case 4:
		layout = []string{"L", "R", "S"}
	case 5:
		layout = []string{"L", "R", "C", "S"}
	case 6:
		layout = []string{"L", "R", "Ls", "Rs"}
	case 7:
		layout = []string{"L", "R", "C", "Ls", "Rs"}
	default:
		return 0, ""
	}
	if lfeon {
		withLFE := make([]string, 0, len(layout)+1)
		inserted := false
		for _, ch := range layout {
			withLFE = append(withLFE, ch)
			if ch == "C" {
				withLFE = append(withLFE, "LFE")
				inserted = true
			}
		}
		if !inserted {
			withLFE = append(withLFE, "LFE")
		}
		layout = withLFE
	}
	return uint64(len(layout)), strings.Join(layout, " ")
}

func ac3ServiceKind(bsmod int) string {
	switch bsmod {
	case 0:
		return "Complete Main"
	case 1:
		return "Music and Effects"
	case 2:
		return "Visually Impaired"
	case 3:
		return "Hearing Impaired"
	case 4:
		return "Dialogue"
	case 5:
		return "Commentary"
	case 6:
		return "Emergency"
	case 7:
		return "Voice Over"
	default:
		return ""
	}
}
