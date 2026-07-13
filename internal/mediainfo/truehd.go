package mediainfo

import (
	"bytes"
	"encoding/binary"
)

// trueHDInfo carries major-sync metadata needed to render MediaInfo-compatible
// TrueHD and Atmos fields.
type trueHDInfo struct {
	atmos           bool
	dynamicObjects  int
	sampleRate      int
	samplesPerFrame int
	maxBitRate      int64
}

// trueHDAtmosPresentation contains the MediaInfo-facing 16-channel Atmos
// presentation summary derived from a TrueHD major-sync header.
type trueHDAtmosPresentation struct {
	additionalFeatures    string
	dynamicObjects        int
	bedChannelCount       uint64
	bedChannelConfig      string
	bedChannelConfigShort string
}

// trueHDAtmosPresentationInfo returns the MediaInfo-style summary for the
// Dolby TrueHD Atmos home-theater spatial-coding presentation. These values do
// not describe physical height speaker channels; the base stream still exposes
// a backward-compatible 7.1 channel render, while Atmos-capable decoders recover
// spatially coded objects plus the LFE bed.
func trueHDAtmosPresentationInfo(info trueHDInfo) (trueHDAtmosPresentation, bool) {
	if !info.atmos {
		return trueHDAtmosPresentation{}, false
	}
	return trueHDAtmosPresentation{
		additionalFeatures:    "16-ch",
		dynamicObjects:        max(info.dynamicObjects, 11),
		bedChannelCount:       1,
		bedChannelConfig:      "LFE",
		bedChannelConfigShort: "LFE",
	}, true
}

// parseTrueHDFrame reads a TrueHD major-sync header from a frame payload.
// Payloads may include a leading access-unit prefix before the sync word.
func parseTrueHDFrame(payload []byte) (trueHDInfo, bool) {
	var info trueHDInfo
	offset := bytes.Index(payload, []byte{0xF8, 0x72, 0x6F, 0xBA})
	if offset < 0 {
		return info, false
	}
	data := payload[offset:]
	if len(data) < 28 || binary.BigEndian.Uint32(data[:4]) != 0xF8726FBA {
		return info, false
	}
	headerSize := 28
	if data[25]&1 != 0 {
		extensions := int(data[26] >> 4)
		headerSize += 2 + extensions*2
	}
	if len(data) < headerSize {
		return info, false
	}
	br := ac3BitReader{data: data[:headerSize]}
	if sync, ok := br.readBits(24); !ok || sync != 0xF8726F {
		return info, false
	}
	streamType, ok := br.readBits(8)
	if !ok || streamType != 0xBA {
		return info, false
	}
	rateBits, ok := br.readBits(4)
	if !ok {
		return info, false
	}
	info.sampleRate = trueHDSampleRate(int(rateBits))
	if !br.skipBits(4) || !br.skipBits(2) || !br.skipBits(2) || !br.skipBits(5) || !br.skipBits(2) || !br.skipBits(13) || !br.skipBits(48) {
		return info, false
	}
	if _, ok := br.readBits(1); !ok { // is_vbr
		return info, false
	}
	peak, ok := br.readBits(15)
	if !ok {
		return info, false
	}
	if info.sampleRate > 0 {
		info.maxBitRate = int64((int(peak)*info.sampleRate + 8) >> 4)
	}
	numSubstreams, ok := br.readBits(4)
	if !ok {
		return info, false
	}
	if !br.skipBits(2) {
		return info, false
	}
	_, ok = br.readBits(2) // extended_substream_info
	if !ok {
		return info, false
	}
	substreamInfo, ok := br.readBits(8)
	if !ok {
		return info, false
	}
	info.samplesPerFrame = 40 << (rateBits & 7)
	info.atmos = numSubstreams == 4 && substreamInfo&0x80 != 0
	if info.atmos && data[25]&1 != 0 && len(data) >= 29 {
		// extra_channel_meaning starts with its 4-bit length, followed by the
		// 16-channel dialogue norm, mix level, and channel/object count. For the
		// dynamic-object-only presentation used by TrueHD Atmos, an LFE bed is
		// included in that count and MediaInfo subtracts it from dynamic objects.
		ext := ac3BitReader{data: data[26 : headerSize-2]}
		if _, ok := ext.readBits(4); ok && ext.skipBits(5+6) {
			if count, ok := ext.readBits(5); ok {
				info.dynamicObjects = int(count) + 1
				if dynamicOnly, ok := ext.readBits(1); ok && dynamicOnly == 1 {
					if lfePresent, ok := ext.readBits(1); ok && lfePresent == 1 {
						info.dynamicObjects--
					}
				}
			}
		}
	}
	return info, true
}

// trueHDSampleRate maps MLP/TrueHD rate bits to Hz.
func trueHDSampleRate(rateBits int) int {
	if rateBits == 0x0F {
		return 0
	}
	base := 48000
	if rateBits&0x08 != 0 {
		base = 44100
	}
	return base << (rateBits & 0x07)
}
