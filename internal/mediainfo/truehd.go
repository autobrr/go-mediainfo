package mediainfo

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// trueHDInfo carries major-sync metadata needed to render MediaInfo-compatible
// TrueHD and Atmos fields.
type trueHDInfo struct {
	atmos             bool
	dynamicObjects    int
	hasDynamicObjects bool
	atmosBedChannels  uint64
	atmosBedLayout    string
	sampleRate        int
	samplesPerFrame   int
	maxBitRate        int64
	channelMap        uint16
}

// trueHDChannelCountPerBit maps each assignment-map bit to its channel count.
var trueHDChannelCountPerBit = [...]int{2, 1, 1, 2, 2, 2, 2, 1, 1, 2, 2, 1, 1}

// trueHDChannelLayoutPerBit maps each assignment-map bit to its layout token.
var trueHDChannelLayoutPerBit = [...]string{
	"L R", "C", "LFE", "Ls Rs", "Tfl Tfr", "Lsc Rsc", "Lb Rb", "Cb", "Tc", "Lsd Rsd", "Lw Rw", "Tfc", "LFE2",
}

// trueHDChannels returns the presentation channel count encoded by a TrueHD
// 8-channel assignment map.
func trueHDChannels(channelMap uint16) uint64 {
	count := 0
	for bit, contribution := range trueHDChannelCountPerBit {
		if channelMap&(1<<bit) != 0 {
			count += contribution
		}
	}
	return uint64(count)
}

// trueHDChannelPositions renders MediaInfo's grouped presentation positions.
func trueHDChannelPositions(channelMap uint16) string {
	parts := make([]string, 0, 8)
	switch channelMap & 0x0003 {
	case 0x0003:
		parts = append(parts, "Front: L C R")
	case 0x0001:
		parts = append(parts, "Front: C")
	case 0x0002:
		parts = append(parts, "Front: L, R")
	}
	if channelMap&0x0008 != 0 {
		parts = append(parts, "Side: L R")
	}
	if channelMap&0x0080 != 0 {
		parts = append(parts, "Back: C")
	}
	if channelMap&0x0010 != 0 {
		parts = append(parts, "vh: L R")
	}
	if channelMap&0x0800 != 0 {
		parts = append(parts, "vh: C")
	}
	if channelMap&0x0020 != 0 {
		parts = append(parts, "c: L R")
	}
	if channelMap&0x0040 != 0 {
		parts = append(parts, "Back: L R")
	}
	if channelMap&0x0100 != 0 {
		parts = append(parts, "s: T")
	}
	if channelMap&0x0200 != 0 {
		parts = append(parts, "sd: L R")
	}
	if channelMap&0x0400 != 0 {
		parts = append(parts, "w: L R")
	}
	if channelMap&0x0004 != 0 {
		parts = append(parts, "LFE")
	}
	if channelMap&0x1000 != 0 {
		parts = append(parts, "LFE2")
	}
	return strings.Join(parts, ", ")
}

// trueHDChannelLayout renders MediaInfo's ordered presentation layout.
func trueHDChannelLayout(channelMap uint16) string {
	parts := make([]string, 0, len(trueHDChannelLayoutPerBit))
	for bit, name := range trueHDChannelLayoutPerBit {
		if channelMap&(1<<bit) != 0 {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, " ")
}

// trueHDAtmosPresentation contains the MediaInfo-facing 16-channel Atmos
// presentation summary derived from a TrueHD major-sync header.
type trueHDAtmosPresentation struct {
	additionalFeatures    string
	dynamicObjects        int
	hasDynamicObjects     bool
	bedChannelCount       uint64
	bedChannelConfig      string
	bedChannelConfigShort string
}

// trueHDAtmosPresentationInfo returns only the Atmos presentation metadata that
// was decoded from the major-sync extension. The Atmos signal itself is enough
// for the 16-channel commercial/profile fields, but it does not imply a fixed
// object count or LFE bed.
func trueHDAtmosPresentationInfo(info trueHDInfo) (trueHDAtmosPresentation, bool) {
	if !info.atmos {
		return trueHDAtmosPresentation{}, false
	}
	return trueHDAtmosPresentation{
		additionalFeatures:    "16-ch",
		dynamicObjects:        info.dynamicObjects,
		hasDynamicObjects:     info.hasDynamicObjects,
		bedChannelCount:       info.atmosBedChannels,
		bedChannelConfig:      info.atmosBedLayout,
		bedChannelConfigShort: info.atmosBedLayout,
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
	if !br.skipBits(4) || !br.skipBits(2) || !br.skipBits(2) || !br.skipBits(5) || !br.skipBits(2) {
		return info, false
	}
	channelMap, ok := br.readBits(13)
	if !ok || !br.skipBits(48) {
		return info, false
	}
	info.channelMap = uint16(channelMap)
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
		// 16-channel dialogue norm, mix level, and channel/object count.
		ext := ac3BitReader{data: data[26 : headerSize-2]}
		if _, ok := ext.readBits(4); ok && ext.skipBits(5+6) {
			if count, ok := ext.readBits(5); ok {
				info.dynamicObjects = int(count) + 1
				info.hasDynamicObjects = true
				if !parseTrueHDProgramAssignment(&ext, &info) {
					info.dynamicObjects = 0
					info.hasDynamicObjects = false
					info.atmosBedChannels = 0
					info.atmosBedLayout = ""
				}
			}
		}
	}
	return info, true
}

func parseTrueHDProgramAssignment(br *ac3BitReader, info *trueHDInfo) bool {
	if br == nil || info == nil {
		return false
	}
	dynamicOnly, ok := br.readBits(1)
	if !ok {
		return false
	}
	if dynamicOnly == 1 {
		lfePresent, ok := br.readBits(1)
		if !ok {
			return false
		}
		if lfePresent == 1 {
			if info.dynamicObjects > 0 {
				info.dynamicObjects--
			}
			info.atmosBedChannels = 1
			info.atmosBedLayout = "LFE"
		}
		return true
	}

	contentMask, ok := br.readBits(4)
	if !ok {
		return false
	}
	if contentMask&0x1 != 0 {
		if !br.skipBits(1) { // b_bed_object_chan_distribute
			return false
		}
		multipleBeds, ok := br.readBits(1)
		if !ok {
			return false
		}
		bedCount := 1
		if multipleBeds == 1 {
			count, ok := br.readBits(3)
			if !ok {
				return false
			}
			bedCount = int(count) + 2
		}
		for range bedCount {
			lfeOnly, ok := br.readBits(1)
			if !ok {
				return false
			}
			mask := uint32(1 << 3)
			if lfeOnly == 0 {
				standard, ok := br.readBits(1)
				if !ok {
					return false
				}
				if standard == 1 {
					value, ok := br.readBits(10)
					if !ok {
						return false
					}
					mask = trueHDStandardBedMaskToNonstandard(uint16(value))
				} else {
					value, ok := br.readBits(17)
					if !ok {
						return false
					}
					mask = uint32(value)
				}
			}
			info.atmosBedLayout, info.atmosBedChannels = trueHDBedLayout(mask)
		}
	}
	if contentMask&0x2 != 0 && !br.skipBits(3) {
		return false
	}
	if contentMask&0x4 != 0 {
		count, ok := br.readBits(5)
		if !ok {
			return false
		}
		if count == 0x1F {
			extended, ok := br.readBits(7)
			if !ok {
				return false
			}
			count += extended
		}
		info.dynamicObjects = int(count) + 1
	} else {
		info.dynamicObjects = 0
	}
	info.hasDynamicObjects = true
	if contentMask&0x8 != 0 {
		size, ok := br.readBits(4)
		if !ok || !br.skipBits(int(size)) {
			return false
		}
		padding := (8 - int(size)%8) % 8
		if !br.skipBits(padding) {
			return false
		}
	}
	return true
}

func trueHDStandardBedMaskToNonstandard(mask uint16) uint32 {
	contributions := [...]int{2, 1, 1, 2, 2, 2, 2, 2, 2, 1}
	var result uint32
	bit := 0
	for index, count := range contributions {
		if mask&(1<<index) != 0 {
			for offset := range count {
				result |= 1 << (bit + offset)
			}
		}
		bit += count
	}
	return result
}

func trueHDBedLayout(mask uint32) (string, uint64) {
	tokens := [...]string{"L", "R", "C", "LFE", "Ls", "Rs", "Lrs", "Rrs", "Lvh", "Rvh", "Lts", "Rts", "Lrh", "Rrh", "Lw", "Rw", "LFE2"}
	order := [...]int{0, 1, 2, 3, 4, 5, 6, 7, 14, 15, 8, 9, 10, 11, 12, 13, 16}
	layout := make([]string, 0, len(tokens))
	for _, bit := range order {
		if mask&(1<<bit) != 0 {
			layout = append(layout, tokens[bit])
		}
	}
	return strings.Join(layout, " "), uint64(len(layout))
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
