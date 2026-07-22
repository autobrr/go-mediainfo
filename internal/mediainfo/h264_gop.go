package mediainfo

import "sort"

func appendH264PictureTypes(pics []byte, pending []byte, chunk []byte, limit int, sps *h264SPSInfo, seenAUD *bool, needSlice *bool) ([]byte, []byte) {
	if limit <= 0 || len(pics) >= limit {
		return pics, pending
	}
	if len(chunk) == 0 {
		return pics, pending
	}

	pending = append(pending, chunk...)

	// Drop leading junk and keep a tiny tail if we don't even have a start code yet.
	sc, scLen := findAnnexBStartCode(pending, 0)
	if sc == -1 {
		if len(pending) > 3 {
			pending = pending[len(pending)-3:]
		}
		return pics, pending
	}
	if sc > 0 {
		pending = pending[sc:]
		sc = 0
	}

	for len(pics) < limit {
		nalStart := sc + scLen
		if nalStart >= len(pending) {
			break
		}
		next, nextLen := findAnnexBStartCode(pending, nalStart)
		if next == -1 {
			break // keep incomplete last NAL for the next PES chunk
		}
		if nalStart < next {
			nal := pending[nalStart:next]
			if len(nal) > 0 && nal[0]&0x80 == 0 {
				nalType := nal[0] & 0x1F
				if nalType == 9 && seenAUD != nil && needSlice != nil {
					*seenAUD = true
					*needSlice = true
					sc = next
					scLen = nextLen
					continue
				}
				if nalType == 1 || nalType == 5 {
					if seenAUD != nil && needSlice != nil && *seenAUD && !*needSlice {
						sc = next
						scLen = nextLen
						continue
					}
					if firstMB, ok := h264FirstMBInSlice(nal); ok && firstMB == 0 && h264CountSliceForGOP(nal, nalType, sps) {
						pics = append(pics, h264SlicePictureType(nal, nalType))
						if seenAUD != nil && needSlice != nil && *seenAUD {
							*needSlice = false
						}
					}
				}
			}
		}
		sc = next
		scLen = nextLen
	}

	// Keep remainder from the last start code (the start of the incomplete NAL).
	if sc > 0 {
		pending = pending[sc:]
	}
	return pics, pending
}

func h264CountSliceForGOP(nal []byte, nalType byte, sps *h264SPSInfo) bool {
	if sps == nil || sps.FrameMbsOnly {
		return true
	}
	fieldPic, bottomField, ok := h264SliceFieldFlags(nal, *sps)
	if !ok || !fieldPic {
		return true
	}
	// Count only top fields to avoid double-counting field-coded pictures.
	return !bottomField
}

func h264FirstFieldOrder(payload []byte, sps h264SPSInfo) (string, bool) {
	if sps.FrameMbsOnly {
		return "", false
	}
	order := ""
	scanAnnexBNALs(payload, func(nal []byte) bool {
		if len(nal) == 0 {
			return true
		}
		nalType := nal[0] & 0x1f
		if nalType != 1 && nalType != 5 {
			return true
		}
		fieldPic, bottomField, ok := h264SliceFieldFlags(nal, sps)
		if !ok || !fieldPic {
			return true
		}
		if bottomField {
			order = "BFF"
		} else {
			order = "TFF"
		}
		return false
	})
	return order, order != ""
}

func h264SliceFieldFlags(nal []byte, sps h264SPSInfo) (fieldPic, bottomField, ok bool) {
	if len(nal) == 0 || sps.FrameMbsOnly {
		return false, false, false
	}

	rbsp := nalToRBSP(nal)
	if len(rbsp) == 0 {
		return false, false, false
	}
	br := newBitReader(rbsp)
	if _, ok := br.readUEWithOk(); !ok { // first_mb_in_slice
		return false, false, false
	}
	if _, ok := br.readUEWithOk(); !ok { // slice_type
		return false, false, false
	}
	if _, ok := br.readUEWithOk(); !ok { // pic_parameter_set_id
		return false, false, false
	}
	if sps.SeparateColourPlane {
		if br.readBitsValue(2) == ^uint64(0) {
			return false, false, false
		}
	}

	bits := sps.Log2MaxFrameNumMinus4 + 4
	if bits <= 0 || bits > 32 {
		return false, false, false
	}
	if br.readBitsValue(uint8(bits)) == ^uint64(0) { // frame_num
		return false, false, false
	}

	fieldPicFlag := br.readBitsValue(1)
	if fieldPicFlag == ^uint64(0) {
		return false, false, false
	}
	if fieldPicFlag == 0 {
		return false, false, true
	}

	bottomFieldFlag := br.readBitsValue(1)
	if bottomFieldFlag == ^uint64(0) {
		return false, false, false
	}
	return true, bottomFieldFlag == 1, true
}

func h264SlicePictureType(nal []byte, nalType byte) byte {
	if nalType == 5 {
		// Distinguish IDR from non-IDR I slices. MediaInfo's GOP N matches IDR spacing on some streams.
		return 'K'
	}

	rbsp := nalToRBSP(nal)
	if len(rbsp) == 0 {
		return 'P'
	}
	br := newBitReader(rbsp)
	_, _ = br.readUEWithOk() // first_mb_in_slice
	sliceType, ok := br.readUEWithOk()
	if !ok {
		return 'P'
	}

	switch sliceType % 5 {
	case 0:
		return 'P'
	case 1:
		return 'B'
	case 2:
		return 'I'
	default:
		return 'P'
	}
}

func inferH264GOPFromPics(pics []byte) (m int, n int, ok bool) {
	if len(pics) < 16 {
		return 0, 0, false
	}

	// N: prefer the most common IDR (K) spacing; fall back to I-slice spacing.
	n = inferH264GOPNModeConfident(pics, 'K', 64)
	if n <= 0 {
		n = inferH264GOPNModeConfident(pics, 'I', 64)
	}

	// M: most common spacing between anchor (I/P) pictures.
	lastAnchor := -1
	counts := map[int]int{}
	for i := 0; i < len(pics); i++ {
		if pics[i] != 'I' && pics[i] != 'P' && pics[i] != 'K' {
			continue
		}
		if lastAnchor >= 0 {
			d := i - lastAnchor
			if d > 0 && d <= 32 {
				counts[d]++
			}
		}
		lastAnchor = i
	}
	bestD := 0
	bestC := 0
	for d, c := range counts {
		if c > bestC || (c == bestC && d < bestD) {
			bestD = d
			bestC = c
		}
	}
	if bestD > 0 {
		m = bestD
	}

	if m <= 0 || n <= 0 {
		return 0, 0, false
	}
	return m, n, true
}

func inferH264GOPN(pics []byte, want byte) int {
	first := -1
	second := -1
	for i := 0; i < len(pics); i++ {
		if pics[i] == want {
			if first == -1 {
				first = i
			} else {
				second = i
				break
			}
		}
	}
	if first >= 0 && second > first {
		return second - first
	}
	return 0
}

func inferH264GOPNMode(pics []byte, want byte, maxGap int) int {
	if maxGap <= 0 {
		return 0
	}
	last := -1
	counts := map[int]int{}
	for i := 0; i < len(pics); i++ {
		if pics[i] != want {
			continue
		}
		if last >= 0 {
			d := i - last
			if d > 0 && d <= maxGap {
				counts[d]++
			}
		}
		last = i
	}
	bestD := 0
	bestC := 0
	for d, c := range counts {
		if c > bestC || (c == bestC && d > bestD) {
			bestD = d
			bestC = c
		}
	}
	if bestD > 0 {
		return bestD
	}
	// Very short samples: fall back to the first two occurrences.
	return inferH264GOPN(pics, want)
}

func inferH264GOPNModeConfident(pics []byte, want byte, maxGap int) int {
	bestD, bestC, total := inferH264GOPNModeWithSupport(pics, want, maxGap)
	// MediaInfo only emits Format_Settings_GOP when GOP structure looks stable. Require the
	// modal N to dominate the observed keyframe spacing.
	if bestD <= 0 || bestC < 3 || bestC*2 < total {
		return 0
	}
	return bestD
}

func inferH264GOPNModeWithSupport(pics []byte, want byte, maxGap int) (bestD int, bestC int, total int) {
	if maxGap <= 0 {
		return 0, 0, 0
	}
	last := -1
	counts := map[int]int{}
	for i := 0; i < len(pics); i++ {
		if pics[i] != want {
			continue
		}
		if last >= 0 {
			d := i - last
			if d > 0 && d <= maxGap {
				counts[d]++
			}
		}
		last = i
	}
	for _, c := range counts {
		total += c
	}
	for d, c := range counts {
		if c > bestC || (c == bestC && d > bestD) {
			bestD = d
			bestC = c
		}
	}
	if bestD > 0 {
		return bestD, bestC, total
	}
	// Very short samples: fall back to the first two occurrences.
	d := inferH264GOPN(pics, want)
	if d <= 0 {
		return 0, 0, total
	}
	return d, 1, total
}

// inferH264GOP estimates MediaInfo's Format_Settings_GOP (M/N) from early slice headers.
// It is intentionally lightweight: scan a bounded number of pictures and measure spacing between
// IDR (I) pictures and anchor (I/P) pictures.
func inferH264GOP(pes []byte, seededSPS ...h264SPSInfo) (m int, n int, ok bool) {
	const maxPictures = 512
	if pics := h264DisplayOrderPictureTypes(pes, maxPictures, seededSPS...); len(pics) > 0 {
		return detectH264GOPEarliest(pics)
	}
	// A supplied SPS means the stream was recognized, but display-order
	// reconstruction may be unsupported (for example POC type 2). Do not turn
	// decode order into a fabricated GOP; the fallback is only for headerless
	// probes where no timing model exists.
	if len(seededSPS) > 0 {
		return 0, 0, false
	}
	pics, _ := appendH264PictureTypes(make([]byte, 0, maxPictures), nil, pes, maxPictures, nil, nil, nil)
	return inferH264GOPFromPics(pics)
}

// detectH264GOPEarliest mirrors File_Avc freezing stream fields at its first
// successful GOP_Detect result. Later pictures do not revise a filled parser.
func detectH264GOPEarliest(pics []byte) (m int, n int, ok bool) {
	for end := range pics {
		if pics[end] != 'I' && pics[end] != 'P' {
			continue
		}
		if m, n, ok := detectH264GOP(pics[:end+1]); ok {
			return m, n, true
		}
	}
	return 0, 0, false
}

type h264GOPPicture struct {
	typ    byte
	poc    int
	decode int
	idr    bool
	ref    bool
}

// h264DisplayOrderPictureTypes reconstructs the picture order used by
// File_Avc::GOP_Detect. AVC NAL units arrive in decode order; sorting each IDR
// interval by pic_order_cnt_lsb yields the I/P/B sequence MediaInfo evaluates.
func h264DisplayOrderPictureTypes(payload []byte, limit int, seededSPS ...h264SPSInfo) []byte {
	if limit <= 0 {
		return nil
	}
	var sps h264SPSInfo
	hasSPS := false
	if len(seededSPS) > 0 && seededSPS[0].Log2MaxFrameNumMinus4 >= 0 {
		sps = seededSPS[0]
		hasSPS = true
	}
	groups := make([][]h264GOPPicture, 0, 8)
	current := make([]h264GOPPicture, 0, 64)
	decode := 0
	prevPOCMSB := 0
	prevPOCLSB := 0
	scanAnnexBNALs(payload, func(nal []byte) bool {
		if len(nal) == 0 || nal[0]&0x80 != 0 {
			return true
		}
		nalType := nal[0] & 0x1f
		if nalType == 7 {
			sps = parseH264SPS(nal)
			hasSPS = true
			return true
		}
		if !hasSPS || nalType != 1 && nalType != 5 {
			return true
		}
		picture, parsed := parseH264GOPPicture(nal, nalType, sps, decode)
		if !parsed {
			return true
		}
		decode++
		if picture.idr && len(current) > 0 {
			groups = append(groups, current)
			current = make([]h264GOPPicture, 0, 64)
		}
		maxPOCLSB := 1 << uint(sps.Log2MaxPicOrderCntMinus4+4)
		pocMSB := prevPOCMSB
		if picture.idr {
			pocMSB = 0
			prevPOCMSB = 0
			prevPOCLSB = 0
		} else if maxPOCLSB > 0 {
			if picture.poc < prevPOCLSB && prevPOCLSB-picture.poc >= maxPOCLSB/2 {
				pocMSB = prevPOCMSB + maxPOCLSB
			} else if picture.poc > prevPOCLSB && picture.poc-prevPOCLSB > maxPOCLSB/2 {
				pocMSB = prevPOCMSB - maxPOCLSB
			}
		}
		if picture.ref {
			prevPOCMSB = pocMSB
			prevPOCLSB = picture.poc
		}
		picture.poc += pocMSB
		current = append(current, picture)
		return decode < limit
	})
	if len(current) > 0 {
		groups = append(groups, current)
	}
	if len(groups) == 0 {
		return nil
	}
	pics := make([]byte, 0, decode)
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].poc == group[j].poc {
				return group[i].decode < group[j].decode
			}
			return group[i].poc < group[j].poc
		})
		for _, picture := range group {
			pics = append(pics, picture.typ)
		}
	}
	return pics
}

func parseH264GOPPicture(nal []byte, nalType byte, sps h264SPSInfo, decode int) (h264GOPPicture, bool) {
	rbsp := nalToRBSP(nal)
	br := newBitReader(rbsp)
	firstMB, ok := br.readUEWithOk()
	if !ok || firstMB != 0 {
		return h264GOPPicture{}, false
	}
	sliceType, ok := br.readUEWithOk()
	if !ok {
		return h264GOPPicture{}, false
	}
	if _, ok := br.readUEWithOk(); !ok { // pic_parameter_set_id
		return h264GOPPicture{}, false
	}
	if sps.SeparateColourPlane && br.readBitsValue(2) == ^uint64(0) {
		return h264GOPPicture{}, false
	}
	frameNumBits := sps.Log2MaxFrameNumMinus4 + 4
	if frameNumBits < 4 || frameNumBits > 32 || br.readBitsValue(uint8(frameNumBits)) == ^uint64(0) {
		return h264GOPPicture{}, false
	}
	if !sps.FrameMbsOnly {
		fieldPic := br.readBitsValue(1)
		if fieldPic == ^uint64(0) {
			return h264GOPPicture{}, false
		}
		if fieldPic == 1 {
			bottom := br.readBitsValue(1)
			if bottom == ^uint64(0) || bottom == 1 {
				return h264GOPPicture{}, false
			}
		}
	}
	if nalType == 5 {
		if _, ok := br.readUEWithOk(); !ok { // idr_pic_id
			return h264GOPPicture{}, false
		}
	}
	if sps.PicOrderCntType != 0 {
		return h264GOPPicture{}, false
	}
	pocBits := sps.Log2MaxPicOrderCntMinus4 + 4
	if pocBits < 4 || pocBits > 32 {
		return h264GOPPicture{}, false
	}
	poc := br.readBitsValue(uint8(pocBits))
	if poc == ^uint64(0) {
		return h264GOPPicture{}, false
	}
	typ := byte('P')
	switch sliceType % 5 {
	case 1:
		typ = 'B'
	case 2:
		typ = 'I'
	case 3, 4:
		typ = 'S'
	}
	if nalType == 5 {
		typ = 'I'
	}
	return h264GOPPicture{typ: typ, poc: int(poc), decode: decode, idr: nalType == 5, ref: nal[0]&0x60 != 0}, true
}

type h264GOPPattern struct {
	m     int
	n     int
	valid bool
}

// detectH264GOP is a direct translation of MediaInfoLib File_Avc::GOP_Detect:
// four matching I-to-I intervals are required, with evenly spaced P anchors.
func detectH264GOP(pics []byte) (m int, n int, ok bool) {
	patterns := make([]h264GOPPattern, 0, 8)
	for first := indexByteFrom(pics, 'I', 0); first >= 0; {
		second := indexByteFrom(pics, 'I', first+1)
		if second < 0 {
			break
		}
		positions := make([]int, 0, 16)
		for pos := indexByteFrom(pics, 'P', first+1); pos >= 0 && pos < second; pos = indexByteFrom(pics, 'P', pos+1) {
			positions = append(positions, pos)
		}
		if len(positions) > 1 && positions[0] > first+1 && positions[len(positions)-1] == second-1 {
			positions = positions[:len(positions)-1]
		}
		pattern := h264GOPPattern{n: second - first, valid: true}
		if len(positions) > 0 {
			pattern.m = positions[0] - first
			for i := 1; i < len(positions); i++ {
				if positions[i]-positions[i-1] != pattern.m {
					pattern.valid = false
					break
				}
			}
		}
		patterns = append(patterns, pattern)
		first = second
	}
	if len(patterns) >= 4 && !sameH264GOPPattern(patterns[0], patterns[1]) {
		patterns = patterns[1:]
	}
	if len(patterns) >= 4 && !sameH264GOPPattern(patterns[len(patterns)-1], patterns[len(patterns)-2]) {
		patterns = patterns[:len(patterns)-1]
	}
	if len(patterns) < 4 || !patterns[0].valid {
		return 0, 0, false
	}
	for i := 1; i < len(patterns); i++ {
		if !sameH264GOPPattern(patterns[i], patterns[0]) {
			return 0, 0, false
		}
	}
	return patterns[0].m, patterns[0].n, true
}

func indexByteFrom(data []byte, want byte, start int) int {
	for i := max(start, 0); i < len(data); i++ {
		if data[i] == want {
			return i
		}
	}
	return -1
}

func sameH264GOPPattern(left, right h264GOPPattern) bool {
	return left.valid == right.valid && left.m == right.m && left.n == right.n
}
