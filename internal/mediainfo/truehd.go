package mediainfo

import (
	"bytes"
	"encoding/binary"
)

// trueHDInfo carries major-sync metadata needed to render MediaInfo-compatible
// TrueHD and Atmos fields.
type trueHDInfo struct {
	atmos           bool
	sampleRate      int
	samplesPerFrame int
	maxBitRate      int64
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
