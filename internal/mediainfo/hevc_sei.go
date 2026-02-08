package mediainfo

import (
	"encoding/binary"
	"fmt"
)

type hevcHDRInfo struct {
	masteringPrimaries    string
	masteringLuminanceMin float64
	masteringLuminanceMax float64
	hasMastering          bool
	maxCLL                uint64
	maxFALL               uint64
	hdr10Plus             bool
	hdr10PlusVersion      int
	hdr10PlusToneMapping  bool
}

func (info *hevcHDRInfo) complete() bool {
	return info.hasMastering && info.maxCLL > 0 && info.maxFALL > 0 && info.hdr10Plus
}

func parseHEVCSampleHDR(sample []byte, nalLengthSize int, info *hevcHDRInfo) {
	if len(sample) == 0 || info == nil {
		return
	}
	if nalLengthSize > 0 && nalLengthSize <= 4 {
		parseHEVCNALUnitsLengthPrefixed(sample, nalLengthSize, info)
		return
	}
	parseHEVCNALUnitsAnnexB(sample, info)
}

func parseHEVCNALUnitsLengthPrefixed(sample []byte, nalLengthSize int, info *hevcHDRInfo) {
	for offset := 0; offset+nalLengthSize <= len(sample); {
		nalSize := readNALSize(sample[offset:], nalLengthSize)
		if nalSize <= 0 {
			break
		}
		offset += nalLengthSize
		if offset+nalSize > len(sample) {
			break
		}
		parseHEVCNAL(sample[offset:offset+nalSize], info)
		if info.complete() {
			return
		}
		offset += nalSize
	}
}

func parseHEVCNALUnitsAnnexB(sample []byte, info *hevcHDRInfo) {
	start := findAnnexBStart(sample, 0)
	for start >= 0 {
		next := findAnnexBStart(sample, start+3)
		end := len(sample)
		if next >= 0 {
			end = next
		}
		nal := sample[start:end]
		if len(nal) > 0 {
			parseHEVCNAL(nal, info)
		}
		if info.complete() || next < 0 {
			return
		}
		start = next
	}
}

func readNALSize(sample []byte, size int) int {
	if len(sample) < size {
		return 0
	}
	switch size {
	case 1:
		return int(sample[0])
	case 2:
		return int(binary.BigEndian.Uint16(sample[:2]))
	case 3:
		return int(sample[0])<<16 | int(sample[1])<<8 | int(sample[2])
	case 4:
		return int(binary.BigEndian.Uint32(sample[:4]))
	default:
		return 0
	}
}

func findAnnexBStart(data []byte, start int) int {
	for i := start; i+3 <= len(data); i++ {
		if data[i] == 0x00 && data[i+1] == 0x00 {
			if data[i+2] == 0x01 {
				return i + 3
			}
			if i+3 < len(data) && data[i+2] == 0x00 && data[i+3] == 0x01 {
				return i + 4
			}
		}
	}
	return -1
}

func parseHEVCNAL(nal []byte, info *hevcHDRInfo) {
	if len(nal) < 2 {
		return
	}
	nalType := (nal[0] >> 1) & 0x3F
	if nalType != 39 && nalType != 40 {
		return
	}
	rbsp := nalToRBSPWithHeader(nal, 2)
	parseHEVCSEI(rbsp, info)
}

func parseHEVCSEI(rbsp []byte, info *hevcHDRInfo) {
	for i := 0; i < len(rbsp); {
		payloadType := 0
		for i < len(rbsp) && rbsp[i] == 0xFF {
			payloadType += 255
			i++
		}
		if i >= len(rbsp) {
			break
		}
		payloadType += int(rbsp[i])
		i++
		payloadSize := 0
		for i < len(rbsp) && rbsp[i] == 0xFF {
			payloadSize += 255
			i++
		}
		if i >= len(rbsp) {
			break
		}
		payloadSize += int(rbsp[i])
		i++
		if i+payloadSize > len(rbsp) {
			break
		}
		payload := rbsp[i : i+payloadSize]
		i += payloadSize
		switch payloadType {
		case 137:
			parseMasteringDisplayColourVolume(payload, info)
		case 144:
			parseContentLightLevel(payload, info)
		case 4:
			parseHEVCUserDataRegistered(payload, info)
		}
		if info.complete() {
			return
		}
	}
}

func parseMasteringDisplayColourVolume(payload []byte, info *hevcHDRInfo) {
	if len(payload) < 24 {
		return
	}
	var primaries [8]uint16
	primaries[0] = binary.BigEndian.Uint16(payload[0:2])   //nolint:gosec // payload length validated
	primaries[1] = binary.BigEndian.Uint16(payload[2:4])   //nolint:gosec // payload length validated
	primaries[2] = binary.BigEndian.Uint16(payload[4:6])   //nolint:gosec // payload length validated
	primaries[3] = binary.BigEndian.Uint16(payload[6:8])   //nolint:gosec // payload length validated
	primaries[4] = binary.BigEndian.Uint16(payload[8:10])  //nolint:gosec // payload length validated
	primaries[5] = binary.BigEndian.Uint16(payload[10:12]) //nolint:gosec // payload length validated
	primaries[6] = binary.BigEndian.Uint16(payload[12:14]) //nolint:gosec // payload length validated
	primaries[7] = binary.BigEndian.Uint16(payload[14:16]) //nolint:gosec // payload length validated
	maxLum := binary.BigEndian.Uint32(payload[16:20])      //nolint:gosec // payload length validated
	minLum := binary.BigEndian.Uint32(payload[20:24])      //nolint:gosec // payload length validated
	if info.masteringPrimaries == "" {
		info.masteringPrimaries = masteringDisplayPrimariesName(primaries)
	}
	info.masteringLuminanceMin = float64(minLum) / 10000.0
	info.masteringLuminanceMax = float64(maxLum) / 10000.0
	info.hasMastering = true
}

func parseContentLightLevel(payload []byte, info *hevcHDRInfo) {
	if len(payload) < 4 {
		return
	}
	if info.maxCLL == 0 {
		info.maxCLL = uint64(binary.BigEndian.Uint16(payload[0:2]))
	}
	if info.maxFALL == 0 {
		info.maxFALL = uint64(binary.BigEndian.Uint16(payload[2:4]))
	}
}

func parseHEVCUserDataRegistered(payload []byte, info *hevcHDRInfo) {
	if len(payload) < 6 {
		return
	}
	if payload[0] != 0xB5 {
		return
	}
	provider := binary.BigEndian.Uint16(payload[1:3])
	if provider != 0x003C {
		return
	}
	oriented := binary.BigEndian.Uint16(payload[3:5])
	if oriented != 0x0001 {
		return
	}
	if payload[5] != 0x04 {
		return
	}
	parseHDR10Plus(payload[6:], info)
}

func parseHDR10Plus(payload []byte, info *hevcHDRInfo) {
	if len(payload) < 1 {
		return
	}
	info.hdr10Plus = true
	info.hdr10PlusVersion = int(payload[0])
	br := newBitReader(payload[1:])
	numWindows := int(br.readBitsValue(2))
	for w := 1; w < numWindows; w++ {
		_ = br.readBitsValue(16)
		_ = br.readBitsValue(16)
		_ = br.readBitsValue(16)
		_ = br.readBitsValue(16)
		_ = br.readBitsValue(16)
		_ = br.readBitsValue(16)
		_ = br.readBitsValue(8)
		_ = br.readBitsValue(16)
		_ = br.readBitsValue(16)
		_ = br.readBitsValue(16)
		_ = br.readBitsValue(1)
	}
	_ = br.readBitsValue(27)
	targetedActualFlag := br.readBitsValue(1) == 1
	if targetedActualFlag {
		rows := int(br.readBitsValue(5))
		cols := int(br.readBitsValue(5))
		for i := 0; i < rows*cols; i++ {
			_ = br.readBitsValue(4)
		}
	}
	for range numWindows {
		for range 3 {
			_ = br.readBitsValue(17)
		}
		_ = br.readBitsValue(17)
		numPercentiles := int(br.readBitsValue(4))
		for range numPercentiles {
			_ = br.readBitsValue(7)
			_ = br.readBitsValue(17)
		}
		_ = br.readBitsValue(10)
	}
	masteringActualFlag := br.readBitsValue(1) == 1
	if masteringActualFlag {
		rows := int(br.readBitsValue(5))
		cols := int(br.readBitsValue(5))
		for i := 0; i < rows*cols; i++ {
			_ = br.readBitsValue(4)
		}
	}
	toneMapping := false
	for range numWindows {
		if br.readBitsValue(1) == 1 {
			toneMapping = true
			_ = br.readBitsValue(12)
			_ = br.readBitsValue(12)
			anchors := int(br.readBitsValue(4))
			for range anchors {
				_ = br.readBitsValue(10)
			}
		}
	}
	info.hdr10PlusToneMapping = toneMapping
	if br.readBitsValue(1) == 1 {
		_ = br.readBitsValue(6)
	}
}

func formatHDR10Plus(info hevcHDRInfo) string {
	profile := "HDR10+ Profile A"
	if info.hdr10PlusToneMapping {
		profile = "HDR10+ Profile B"
	}
	return fmt.Sprintf("SMPTE ST 2094 App 4, Version %d, %s compatible", info.hdr10PlusVersion, profile)
}

type masteringDisplayValue struct {
	code   uint8
	values [8]uint16
}

var masteringDisplayValues = []masteringDisplayValue{
	{1, [8]uint16{15000, 30000, 7500, 3000, 32000, 16500, 15635, 16450}},
	{9, [8]uint16{8500, 39850, 6550, 2300, 35400, 14600, 15635, 16450}},
	{11, [8]uint16{13250, 34500, 7500, 3000, 34000, 16000, 15700, 17550}},
	{12, [8]uint16{13250, 34500, 7500, 3000, 34000, 16000, 15635, 16450}},
}

func masteringDisplayPrimariesName(values [8]uint16) string {
	// Try all permutations of mapping the 3 primaries to (G,B,R) so we don't rely on
	// heuristics that can fail on real-world streams.
	perms := [][3]int{
		{0, 1, 2},
		{0, 2, 1},
		{1, 0, 2},
		{1, 2, 0},
		{2, 0, 1},
		{2, 1, 0},
	}
	for _, entry := range masteringDisplayValues {
		for _, perm := range perms {
			gIndex, bIndex, rIndex := perm[0], perm[1], perm[2]
			match := true
			for j := 0; j < 2; j++ {
				gVal, ok := primariesAt(values, gIndex*2+j)
				if !ok || !withinTolerance(gVal, entry.values[0*2+j], 25) {
					match = false
				}
				bVal, ok := primariesAt(values, bIndex*2+j)
				if !ok || !withinTolerance(bVal, entry.values[1*2+j], 25) {
					match = false
				}
				rVal, ok := primariesAt(values, rIndex*2+j)
				if !ok || !withinTolerance(rVal, entry.values[2*2+j], 25) {
					match = false
				}
				wVal, ok := primariesAt(values, 3*2+j)
				if !ok || !withinTolerance(wVal, entry.values[3*2+j], 25) {
					match = false
				}
			}
			if match {
				return masteringDisplayPrimariesLabel(entry.code)
			}
		}
	}
	return ""
}

func masteringDisplayPrimariesLabel(code uint8) string {
	switch code {
	case 1:
		return "BT.709"
	case 9:
		return "BT.2020"
	case 11, 12:
		// MediaInfo reports Display P3 for common HDR mastering display metadata values.
		return "Display P3"
	default:
		return ""
	}
}

func primariesAt(values [8]uint16, index int) (uint16, bool) {
	if index < 0 || index >= len(values) {
		return 0, false
	}
	return values[index], true //nolint:gosec // bounds checked above
}

func withinTolerance(value, target uint16, tolerance uint16) bool {
	if value < target {
		return target-value <= tolerance
	}
	return value-target <= tolerance
}
