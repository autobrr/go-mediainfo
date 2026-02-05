package mediainfo

import (
	"encoding/binary"
	"strings"
)

type eac3Dec3Info struct {
	hasJOC        bool
	hasJOCComplex bool
	jocComplexity int
}

type eac3JOCMeta struct {
	hasJOC        bool
	hasJOCComplex bool
	jocComplexity int
	jocObjects    int
	hasJOCDyn     bool
	jocDynObjects int
	hasJOCBed     bool
	jocBedCount   uint64
	jocBedLayout  string
}

func parseEAC3Dec3(data []byte) (eac3Dec3Info, bool) {
	var info eac3Dec3Info
	if len(data) < 2 {
		return info, false
	}
	if len(data) >= 8 {
		size := int(binary.BigEndian.Uint32(data[0:4]))
		if size >= 8 && size <= len(data) && string(data[4:8]) == "dec3" {
			return parseEAC3Dec3Payload(data[8:size])
		}
	}
	return parseEAC3Dec3Payload(data)
}

func parseEAC3Dec3Payload(data []byte) (eac3Dec3Info, bool) {
	var info eac3Dec3Info
	br := ac3BitReader{data: data}
	if _, ok := br.readBits(13); !ok {
		return info, false
	}
	numIndSub, ok := br.readBits(3)
	if !ok {
		return info, false
	}
	for i := 0; i <= int(numIndSub); i++ {
		if _, ok := br.readBits(2); !ok { // fscod
			return info, false
		}
		if _, ok := br.readBits(5); !ok { // bsid
			return info, false
		}
		if _, ok := br.readBits(1); !ok { // reserved
			return info, false
		}
		if _, ok := br.readBits(1); !ok { // asvc
			return info, false
		}
		if _, ok := br.readBits(3); !ok { // bsmod
			return info, false
		}
		if _, ok := br.readBits(3); !ok { // acmod
			return info, false
		}
		if _, ok := br.readBits(1); !ok { // lfeon
			return info, false
		}
		if _, ok := br.readBits(3); !ok { // reserved
			return info, false
		}
		numDepSub, ok := br.readBits(4)
		if !ok {
			return info, false
		}
		if numDepSub > 0 {
			if _, ok := br.readBits(9); !ok { // chan_loc
				return info, false
			}
		} else if _, ok := br.readBits(1); !ok { // reserved
			return info, false
		}
	}
	if br.remaining() >= 8 {
		if !br.skipBits(7) {
			return info, false
		}
		flag, ok := br.readBits(1)
		if !ok {
			return info, false
		}
		if flag == 1 {
			value, ok := br.readBits(8)
			if !ok {
				return info, false
			}
			info.hasJOC = true
			info.hasJOCComplex = true
			info.jocComplexity = int(value)
		}
	}
	if info.hasJOC || info.hasJOCComplex {
		return info, true
	}
	return info, false
}

func parseEAC3EMDF(payload []byte) (eac3JOCMeta, bool) {
	var meta eac3JOCMeta
	totalBits := len(payload) * 8
	for pos := 0; pos+16 <= totalBits; pos++ {
		value, ok := readBitsAt(payload, pos, 16)
		if !ok || value != 0x5838 {
			continue
		}
		if parseEAC3EMDFAt(payload, pos, &meta) {
			return meta, meta.hasJOC || meta.hasJOCDyn || meta.hasJOCBed || meta.hasJOCComplex || meta.jocObjects > 0
		}
	}
	return meta, false
}

func ac3HasJOCInfo(info ac3Info) bool {
	return info.hasJOC || info.hasJOCComplex || info.jocObjects > 0 || info.hasJOCDyn || info.hasJOCBed
}

func parseEAC3EMDFAt(payload []byte, bitPos int, meta *eac3JOCMeta) bool {
	br := ac3BitReader{data: payload, bitPos: bitPos}
	if _, ok := br.readBits(16); !ok { // syncword
		return false
	}
	containerLength, ok := br.readBits(16)
	if !ok {
		return false
	}
	containerEnd := br.bitPos + int(containerLength)*8
	if containerEnd > len(payload)*8 || containerEnd < br.bitPos {
		return false
	}
	version, ok := br.readBits(2)
	if !ok {
		return false
	}
	if version == 3 {
		ext, ok := br.readVariableBits(2)
		if !ok {
			return false
		}
		version += ext
	}
	if version != 0 {
		return false
	}
	keyID, ok := br.readBits(3)
	if !ok {
		return false
	}
	if keyID == 7 {
		if _, ok := br.readVariableBits(3); !ok {
			return false
		}
	}
	for br.bitPos < containerEnd {
		payloadID, ok := br.readBits(5)
		if !ok {
			return false
		}
		if payloadID == 0x1F {
			ext, ok := br.readVariableBits(5)
			if !ok {
				return false
			}
			payloadID += ext
		}
		if payloadID == 0 {
			break
		}
		if !parseEMDFPayloadConfig(&br) {
			return false
		}
		payloadSize, ok := br.readVariableBits(8)
		if !ok {
			return false
		}
		payloadEnd := br.bitPos + int(payloadSize)*8
		if payloadEnd > containerEnd {
			return false
		}
		payloadReader := ac3BitReader{data: payload, bitPos: br.bitPos, limit: payloadEnd}
		switch payloadID {
		case 11:
			if parseEAC3ObjectAudioMetadata(&payloadReader, meta) {
				meta.hasJOC = true
			}
		case 14:
			if parseEAC3JOCHeader(&payloadReader, meta) {
				meta.hasJOC = true
			}
		}
		br.bitPos = payloadEnd
	}
	return true
}

func parseEMDFPayloadConfig(br *ac3BitReader) bool {
	smploffste, ok := br.readBits(1)
	if !ok {
		return false
	}
	if smploffste == 1 {
		if !br.skipBits(11) || !br.skipBits(1) {
			return false
		}
	}
	duratione, ok := br.readBits(1)
	if !ok {
		return false
	}
	if duratione == 1 {
		if _, ok := br.readVariableBits(11); !ok {
			return false
		}
	}
	groupide, ok := br.readBits(1)
	if !ok {
		return false
	}
	if groupide == 1 {
		if _, ok := br.readVariableBits(2); !ok {
			return false
		}
	}
	codecdatae, ok := br.readBits(1)
	if !ok || codecdatae == 1 {
		return false
	}
	discardUnknown, ok := br.readBits(1)
	if !ok {
		return false
	}
	if discardUnknown == 0 {
		payloadFrameAligned := uint32(0)
		if smploffste == 0 {
			payloadFrameAligned, ok = br.readBits(1)
			if !ok {
				return false
			}
			if payloadFrameAligned == 1 {
				if !br.skipBits(2) {
					return false
				}
			}
		}
		if smploffste == 1 || payloadFrameAligned == 1 {
			if !br.skipBits(5) || !br.skipBits(2) {
				return false
			}
		}
	}
	return true
}

func parseEAC3ObjectAudioMetadata(br *ac3BitReader, meta *eac3JOCMeta) bool {
	version, ok := br.readBits(2)
	if !ok {
		return false
	}
	if version == 0x3 {
		if _, ok := br.readBits(3); !ok {
			return false
		}
	}
	objectCountBits, ok := br.readBits(5)
	if !ok {
		return false
	}
	numObjects := int(objectCountBits) + 1
	if objectCountBits == 0x1F {
		ext, ok := br.readBits(7)
		if !ok {
			return false
		}
		numObjects += int(ext)
	}
	hasDyn, dynObjects, hasBed, bedMask := parseEAC3ProgramAssignment(br, numObjects)
	if hasDyn {
		meta.jocDynObjects = dynObjects
		meta.hasJOCDyn = true
	}
	if hasBed {
		layout, count := ac3NonstdBedChannelAssignmentMaskLayout(bedMask)
		if layout != "" {
			meta.jocBedLayout = layout
			meta.jocBedCount = count
			meta.hasJOCBed = true
		}
	}
	return true
}

func parseEAC3ProgramAssignment(br *ac3BitReader, numDynamicObjects int) (bool, int, bool, uint32) {
	bDynOnly, ok := br.readBits(1)
	if !ok {
		return false, 0, false, 0
	}
	if bDynOnly == 1 {
		bLFE, ok := br.readBits(1)
		if !ok {
			return false, 0, false, 0
		}
		if bLFE == 1 && numDynamicObjects > 0 {
			numDynamicObjects--
		}
		if bLFE == 1 {
			return true, numDynamicObjects, true, 1 << 3
		}
		return true, numDynamicObjects, false, 0
	}
	contentMask, ok := br.readBits(4)
	if !ok {
		return false, 0, false, 0
	}
	var bedMask uint32
	hasBed := false
	if contentMask&0x1 != 0 {
		if _, ok := br.readBits(1); !ok { // b_bed_object_chan_distribute
			return false, 0, false, 0
		}
		multiBed, ok := br.readBits(1)
		if !ok {
			return false, 0, false, 0
		}
		numBeds := 1
		if multiBed == 1 {
			value, ok := br.readBits(3)
			if !ok {
				return false, 0, false, 0
			}
			numBeds = int(value) + 2
		}
		for range numBeds {
			bLFEOnly, ok := br.readBits(1)
			if !ok {
				return false, 0, false, 0
			}
			if bLFEOnly == 1 {
				continue
			}
			bStandard, ok := br.readBits(1)
			if !ok {
				return false, 0, false, 0
			}
			if bStandard == 1 {
				mask, ok := br.readBits(10)
				if !ok {
					return false, 0, false, 0
				}
				bedMask = ac3BedChannelAssignmentMaskToNonstd(uint16(mask))
				hasBed = true
			} else {
				mask, ok := br.readBits(17)
				if !ok {
					return false, 0, false, 0
				}
				bedMask = mask
				hasBed = true
			}
		}
	}
	if contentMask&0x2 != 0 {
		if !br.skipBits(3) {
			return false, 0, false, 0
		}
	}
	hasDyn := true
	if contentMask&0x4 != 0 {
		value, ok := br.readBits(5)
		if !ok {
			return false, 0, false, 0
		}
		if value == 0x1F {
			ext, ok := br.readBits(7)
			if !ok {
				return false, 0, false, 0
			}
			value += ext
		}
		numDynamicObjects = int(value) + 1
	} else {
		numDynamicObjects = 0
	}
	if contentMask&0x8 != 0 {
		sizeBits, ok := br.readBits(4)
		if !ok {
			return false, 0, false, 0
		}
		if sizeBits > 0 {
			if !br.skipBits(int(sizeBits)) {
				return false, 0, false, 0
			}
		}
		padding := 8 - (int(sizeBits) % 8)
		if padding > 0 {
			if !br.skipBits(padding) {
				return false, 0, false, 0
			}
		}
	}
	return hasDyn, numDynamicObjects, hasBed, bedMask
}

func parseEAC3JOCHeader(br *ac3BitReader, meta *eac3JOCMeta) bool {
	if !br.skipBits(3) { // joc_dmx_config_idx
		return false
	}
	value, ok := br.readBits(6)
	if !ok {
		return false
	}
	meta.jocObjects = int(value) + 1
	if !br.skipBits(3) { // joc_ext_config_idx
		return false
	}
	return true
}

func readBitsAt(data []byte, bitPos int, n int) (uint32, bool) {
	totalBits := len(data) * 8
	if n <= 0 || bitPos+n > totalBits {
		return 0, false
	}
	var value uint32
	for i := range n {
		byteVal := data[(bitPos+i)>>3]
		bit := (byteVal >> (7 - ((bitPos + i) & 7))) & 0x01
		value = (value << 1) | uint32(bit)
	}
	return value, true
}

var ac3NonstdBedChannelLayoutList = []string{
	"L",
	"R",
	"C",
	"LFE",
	"Ls",
	"Rs",
	"Lrs",
	"Rrs",
	"Lvh",
	"Rvh",
	"Lts",
	"Rts",
	"Lrh",
	"Rrh",
	"Lw",
	"Rw",
	"LFE2",
}

var ac3NonstdBedChannelLayoutReorder = []int{
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	6,
	6,
	-2,
	-2,
	-2,
	-2,
	-2,
	-2,
	0,
}

func ac3NonstdBedChannelAssignmentMaskLayout(mask uint32) (string, uint64) {
	var parts []string
	for i := range ac3NonstdBedChannelLayoutList {
		i2 := i + ac3NonstdBedChannelLayoutReorder[i]
		if i2 < 0 || i2 >= len(ac3NonstdBedChannelLayoutList) {
			continue
		}
		if mask&(1<<i2) != 0 {
			parts = append(parts, ac3NonstdBedChannelLayoutList[i2])
		}
	}
	if len(parts) == 0 {
		return "", 0
	}
	return strings.Join(parts, " "), uint64(len(parts))
}

var ac3BedChannelAssignmentMaskMapping = []int{
	2,
	1,
	1,
	2,
	2,
	2,
	2,
	2,
	2,
	1,
}

func ac3BedChannelAssignmentMaskToNonstd(mask uint16) uint32 {
	var out uint32
	j := 0
	for i := range ac3BedChannelAssignmentMaskMapping {
		if mask&(1<<i) != 0 {
			out |= 1 << j
			j++
			if ac3BedChannelAssignmentMaskMapping[i] > 1 {
				out |= 1 << j
				j++
			}
		} else {
			j += ac3BedChannelAssignmentMaskMapping[i]
		}
	}
	return out
}
