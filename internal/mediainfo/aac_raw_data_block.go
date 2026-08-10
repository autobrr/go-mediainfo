package mediainfo

// AAC raw_data_block syntactic-element walker, ported from MediaInfoLib
// File_Aac_GeneralAudio.cpp (v23.04). MediaInfo reports "Errors: Missing
// ID_END" on audio tracks whose AAC frames do not terminate with an ID_END
// element; finding that terminator requires decoding every element boundary,
// including huffman-coded section, scale-factor, and spectral data.
//
// Only the success path needs bit-exact fidelity: any parse failure in the
// reference implementation ends the walk without ID_END and raises the same
// error, so all error paths here just abort the frame.

// aacRDBConfig carries the AudioSpecificConfig facts the walker needs.
type aacRDBConfig struct {
	objectType             int
	samplingFrequencyIndex int
	frameLength            int // 1024, or 960 when frameLengthFlag is set
}

// aacRDBWalkable reports whether MediaInfoLib would walk raw_data_block for
// this configuration: File_Aac::payload only dispatches audioObjectType 2,
// and raw_data_block rejects sampling_frequency_index >= 13 without filling
// any error.
func (cfg aacRDBConfig) walkable() bool {
	return cfg.objectType == 2 && cfg.samplingFrequencyIndex >= 0 && cfg.samplingFrequencyIndex < 13
}

// parseAACRDBConfig extracts the walker configuration from a Matroska
// CodecPrivate AudioSpecificConfig, mirroring File_Aac::AudioSpecificConfig:
// explicit SBR (objectType 5/29) re-reads the core object type, and an escaped
// extensionSamplingFrequencyIndex overwrites sampling_frequency_index with the
// index mapped from the extension rate.
func parseAACRDBConfig(payload []byte) (aacRDBConfig, bool) {
	cfg := aacRDBConfig{frameLength: 1024}
	if len(payload) == 0 {
		return cfg, false
	}
	br := newBitReader(payload)
	objType, ok := readAACAudioObjectType(br)
	if !ok {
		return cfg, false
	}
	sfi := br.readBitsValue(4)
	if sfi == ^uint64(0) {
		return cfg, false
	}
	cfg.samplingFrequencyIndex = int(sfi)
	if sfi == 0xF {
		freq := br.readBitsValue(24)
		if freq == ^uint64(0) {
			return cfg, false
		}
		cfg.samplingFrequencyIndex = aacSamplingFrequencyIndexOf(int64(freq))
	}
	if br.readBitsValue(4) == ^uint64(0) { // channelConfiguration
		return cfg, false
	}
	if objType == 5 || objType == 29 {
		extSfi := br.readBitsValue(4)
		if extSfi == ^uint64(0) {
			return cfg, false
		}
		if extSfi == 0xF {
			extFreq := br.readBitsValue(24)
			if extFreq == ^uint64(0) {
				return cfg, false
			}
			cfg.samplingFrequencyIndex = aacSamplingFrequencyIndexOf(int64(extFreq))
		}
		objType, ok = readAACAudioObjectType(br)
		if !ok {
			return cfg, false
		}
		if objType == 22 {
			if br.readBitsValue(4) == ^uint64(0) {
				return cfg, false
			}
		}
	}
	cfg.objectType = objType
	if !isAACGASpecificObjectType(objType) {
		return cfg, false
	}
	frameLengthFlag := br.readBitsValue(1)
	if frameLengthFlag == ^uint64(0) {
		return cfg, false
	}
	if frameLengthFlag == 1 {
		cfg.frameLength = 960
	}
	return cfg, true
}

// aacSamplingFrequencyIndexOf mirrors Aac_AudioSpecificConfig_sampling_frequency_index.
func aacSamplingFrequencyIndexOf(frequency int64) int {
	thresholds := [...]int64{92017, 75132, 55426, 46009, 37566, 27713, 23004, 18783, 13856, 11502, 9391}
	for index, threshold := range thresholds {
		if frequency >= threshold {
			return index
		}
	}
	return 11
}

// aacRDBState holds the walker's bitstream cursor and the decoder state that
// File_Aac keeps as members across elements and frames.
type aacRDBState struct {
	cfg aacRDBConfig

	data []byte
	pos  int // bit cursor
	ok   bool

	commonWindow bool

	windowSequence      uint8
	maxSFB              uint8
	scaleFactorGrouping uint8
	numWindows          uint8
	numWindowGroups     uint8
	windowGroupLength   [8]uint8
	numSWB              uint8
	sectSFBOffset       [8][1024]uint16
	sfbCB               [8][64]uint8

	numSec    [8]uint8
	sectCB    [8][65]uint8
	sectStart [8][65]uint16
	sectEnd   [8][65]uint16
}

func newAACRDBState(cfg aacRDBConfig) *aacRDBState {
	return &aacRDBState{cfg: cfg}
}

func (s *aacRDBState) remain() int {
	return len(s.data)*8 - s.pos
}

// abort consumes the rest of the frame and marks the walk failed, matching the
// reference Skip_BS(Data_BS_Remain())/Trusted_IsNot outcomes: the walk ends
// without ID_END.
func (s *aacRDBState) abort() {
	s.pos = len(s.data) * 8
	s.ok = false
}

func (s *aacRDBState) read(n int) uint32 {
	if !s.ok {
		return 0
	}
	if n > s.remain() {
		s.abort()
		return 0
	}
	var value uint32
	for range n {
		bit := (s.data[s.pos>>3] >> (7 - uint(s.pos&7))) & 1
		value = value<<1 | uint32(bit)
		s.pos++
	}
	return value
}

func (s *aacRDBState) peek(n int) uint32 {
	pos := s.pos
	value := s.read(n)
	s.pos = pos
	return value
}

func (s *aacRDBState) skip(n int) {
	if !s.ok {
		return
	}
	// A negative count only happens on the reference's out-of-range huffman
	// offsets, where Skip_BS wraps to a huge unsigned size; both end the walk.
	if n < 0 || n > s.remain() {
		s.abort()
		return
	}
	s.pos += n
}

// walkFrame walks one raw_data_block and reports whether it terminated with an
// ID_END element.
func (s *aacRDBState) walkFrame(frame []byte) (hasEnd bool) {
	s.data = frame
	s.pos = 0
	s.ok = true
	for {
		id := uint8(s.read(3))
		if !s.ok {
			return false
		}
		switch id {
		case 0x00:
			s.singleChannelElement()
		case 0x01:
			s.channelPairElement()
		case 0x02:
			s.couplingChannelElement()
		case 0x03:
			s.lfeChannelElement()
		case 0x04:
			s.dataStreamElement()
		case 0x05:
			s.programConfigElement()
		case 0x06:
			s.fillElement()
		case 0x07:
			return true
		}
		if !s.ok || s.remain() == 0 {
			return false
		}
	}
}

func (s *aacRDBState) singleChannelElement() {
	s.skip(4) // element_instance_tag
	s.individualChannelStream(false)
}

func (s *aacRDBState) channelPairElement() {
	s.skip(4) // element_instance_tag
	s.commonWindow = s.read(1) == 1
	if s.commonWindow {
		s.icsInfo()
		msMaskPresent := s.read(2)
		if msMaskPresent == 1 {
			for g := uint8(0); g < s.numWindowGroups; g++ {
				s.skip(int(s.maxSFB)) // ms_used[g][sfb]
			}
		}
	}
	s.individualChannelStream(s.commonWindow)
	if !s.ok {
		return
	}
	s.individualChannelStream(s.commonWindow)
}

func (s *aacRDBState) icsInfo() {
	s.skip(1) // ics_reserved_bit
	s.windowSequence = uint8(s.read(2))
	s.skip(1)                  // window_shape
	if s.windowSequence == 2 { // EIGHT_SHORT_SEQUENCE
		s.maxSFB = uint8(s.read(4))
		s.scaleFactorGrouping = uint8(s.read(7))
	} else {
		s.maxSFB = uint8(s.read(6))
		if s.read(1) == 1 { // predictor_data_present
			// The walker only runs for object type 2 (LC), so the reference's
			// AAC Main prediction branch is unreachable here.
			if s.read(1) == 1 { // ltp_data_present
				s.ltpData()
			}
			if s.commonWindow {
				if s.read(1) == 1 {
					s.ltpData()
				}
			}
		}
	}
	if !s.ok {
		return
	}

	// Window computation.
	switch s.windowSequence {
	case 0, 1, 3: // long sequences
		s.numWindows = 1
		s.numWindowGroups = 1
		s.windowGroupLength[0] = 1
		table := aacSWBOffsetLongWindow[s.cfg.samplingFrequencyIndex]
		s.numSWB = uint8(table.numSWB)
		for i := 0; i <= table.numSWB; i++ {
			if int(table.offsets[i]) < s.cfg.frameLength {
				s.sectSFBOffset[0][i] = table.offsets[i]
			} else {
				s.sectSFBOffset[0][i] = uint16(s.cfg.frameLength)
			}
		}
	case 2: // EIGHT_SHORT_SEQUENCE
		s.numWindows = 8
		s.numWindowGroups = 1
		s.windowGroupLength[0] = 1
		table := aacSWBOffsetShortWindow[s.cfg.samplingFrequencyIndex]
		s.numSWB = uint8(table.numSWB)
		for i := uint8(0); i < s.numWindows-1; i++ {
			if s.scaleFactorGrouping&(1<<(6-i)) == 0 {
				s.numWindowGroups++
				s.windowGroupLength[s.numWindowGroups-1] = 1
			} else {
				s.windowGroupLength[s.numWindowGroups-1]++
			}
		}
		for g := 0; g < int(s.numWindowGroups); g++ {
			sectSFB := 0
			offset := uint16(0)
			for i := 0; i < table.numSWB; i++ {
				width := table.offsets[i+1] - table.offsets[i]
				width *= uint16(s.windowGroupLength[g])
				s.sectSFBOffset[g][sectSFB] = offset
				sectSFB++
				offset += width
			}
			s.sectSFBOffset[g][sectSFB] = offset
		}
	}
}

func (s *aacRDBState) pulseData() {
	numberPulse := s.read(2)
	s.skip(6) // pulse_start_sfb
	for i := uint32(0); i <= numberPulse; i++ {
		s.skip(9) // pulse_offset + pulse_amp
	}
}

func (s *aacRDBState) couplingChannelElement() {
	s.skip(4) // element_instance_tag
	indSwCCEFlag := s.read(1) == 1
	numCoupledElements := s.read(3)
	numGainElementLists := 0
	for c := uint32(0); c <= numCoupledElements; c++ {
		numGainElementLists++
		ccTargetIsCPE := s.read(1) == 1
		s.skip(4) // cc_target_tag_select
		if ccTargetIsCPE {
			ccL := s.read(1) == 1
			ccR := s.read(1) == 1
			if ccL && ccR {
				numGainElementLists++
			}
		}
	}
	s.skip(4) // cc_domain + gain_element_sign + gain_element_scale
	s.individualChannelStream(false)
	if !s.ok {
		return
	}
	for c := 1; c < numGainElementLists; c++ {
		cge := true
		if !indSwCCEFlag {
			cge = s.read(1) == 1
		}
		if cge {
			s.hcodSF()
		} else {
			for g := uint8(0); g < s.numWindowGroups; g++ {
				for sfb := uint8(0); sfb < s.maxSFB && sfb < 64; sfb++ {
					if s.sfbCB[g][sfb] != 0 {
						s.hcodSF()
					}
				}
			}
		}
		if !s.ok {
			return
		}
	}
}

func (s *aacRDBState) lfeChannelElement() {
	s.skip(4) // element_instance_tag
	s.individualChannelStream(false)
}

func (s *aacRDBState) dataStreamElement() {
	s.skip(4) // element_instance_tag
	dataByteAlignFlag := s.read(1) == 1
	cnt := s.read(8)
	if cnt == 255 {
		cnt += s.read(8)
	}
	if dataByteAlignFlag {
		if s.remain()%8 != 0 {
			s.skip(s.remain() % 8)
		}
	}
	s.skip(int(cnt) * 8)
}

// programConfigElement mirrors File_Aac::program_config_element, including its
// byte-aligned comment field.
func (s *aacRDBState) programConfigElement() {
	s.skip(4) // element_instance_tag
	s.skip(2) // object_type
	samplingFrequencyIndex := s.read(4)
	numFront := s.read(4)
	numSide := s.read(4)
	numBack := s.read(4)
	numLFE := s.read(2)
	numAssocData := s.read(3)
	numValidCC := s.read(4)
	if s.read(1) == 1 { // mono_mixdown_present
		s.skip(4)
	}
	if s.read(1) == 1 { // stereo_mixdown_present
		s.skip(4)
	}
	if s.read(1) == 1 { // matrix_mixdown_idx_present
		s.skip(3)
	}
	channels := uint32(0)
	for _, count := range [3]uint32{numFront, numSide, numBack} {
		for i := uint32(0); i < count; i++ {
			if s.read(1) == 1 { // element_is_cpe
				channels += 2
			} else {
				channels++
			}
			s.skip(4) // element_tag_select
		}
	}
	for i := uint32(0); i < numLFE; i++ {
		s.skip(4)
		channels++
	}
	for i := uint32(0); i < numAssocData; i++ {
		s.skip(4)
	}
	for i := uint32(0); i < numValidCC; i++ {
		s.skip(5)
	}
	// BS_End/Get_B1/BS_Begin: byte-align, then a byte-count-prefixed comment.
	if s.pos%8 != 0 {
		s.skip(8 - s.pos%8)
	}
	commentFieldBytes := s.read(8)
	s.skip(int(commentFieldBytes) * 8)
	if !s.ok {
		return
	}
	// Integrity test: reference rejects reserved sampling frequencies and
	// more than 24 channels.
	if samplingFrequencyIndex >= 13 || channels > 24 {
		s.abort()
	}
}

func (s *aacRDBState) fillElement() {
	cnt := int(s.read(4))
	if cnt == 15 {
		esc := int(s.read(8))
		cnt += esc - 1
	}
	if cnt > 0 {
		if s.remain() >= 8*cnt {
			// extension_payload parses SBR/DRC data but always lands exactly
			// at the announced end for well-formed payloads; the element
			// boundary alone decides ID_END detection.
			s.skip(8 * cnt)
		} else {
			s.skip(s.remain())
		}
	}
}

func (s *aacRDBState) gainControlData() {
	var wdMax, alocBits0, alocBits int
	switch s.windowSequence {
	case 0:
		wdMax, alocBits0, alocBits = 1, 5, 5
	case 1:
		wdMax, alocBits0, alocBits = 2, 4, 2
	case 2:
		wdMax, alocBits0, alocBits = 8, 2, 2
	case 3:
		wdMax, alocBits0, alocBits = 2, 4, 5
	default:
		return
	}
	maxBand := s.read(2)
	for bd := uint32(1); bd <= maxBand; bd++ {
		for wd := 0; wd < wdMax; wd++ {
			adjustNum := s.read(3)
			for ad := uint32(0); ad < adjustNum; ad++ {
				s.skip(4)
				if wd == 0 {
					s.skip(alocBits0)
				} else {
					s.skip(alocBits)
				}
			}
		}
	}
}

// individualChannelStream mirrors the reference with scale_flag always false:
// only ER scalable elements pass true, and the walker never parses those.
func (s *aacRDBState) individualChannelStream(commonWindow bool) {
	s.skip(8) // global_gain
	if !commonWindow {
		s.icsInfo()
	}
	if !s.ok {
		return
	}
	s.sectionData()
	if !s.ok {
		return
	}
	s.scaleFactorData()
	if !s.ok {
		return
	}
	if s.read(1) == 1 { // pulse_data_present
		s.pulseData()
	}
	if s.read(1) == 1 { // tns_data_present
		s.tnsData()
	}
	if s.read(1) == 1 { // gain_control_data_present
		s.gainControlData()
	}
	if !s.ok {
		return
	}
	s.spectralData()
}

func (s *aacRDBState) sectionData() {
	var sectEscVal uint8
	sectBits := 5
	if s.windowSequence == 2 {
		sectEscVal = 1<<3 - 1
		sectBits = 3
	} else {
		sectEscVal = 1<<5 - 1
	}
	for g := uint8(0); g < s.numWindowGroups; g++ {
		var k, i uint8
		for k < s.maxSFB {
			cb := uint8(s.read(4))
			s.sectCB[g][i] = cb
			var sectLen uint8
			for {
				if s.remain() == 0 {
					s.abort()
					return
				}
				incr := uint8(s.read(sectBits))
				if incr != sectEscVal {
					sectLen += incr
					break
				}
				sectLen += sectEscVal
			}
			s.sectStart[g][i] = uint16(k)
			s.sectEnd[g][i] = uint16(k) + uint16(sectLen)
			for sfb := int(k); sfb < int(k)+int(sectLen); sfb++ {
				if sfb < 64 {
					s.sfbCB[g][sfb] = cb
				}
			}
			k += sectLen
			i++
			if i > 64 {
				s.abort()
				return
			}
		}
		s.numSec[g] = i
	}
}

func (s *aacRDBState) scaleFactorData() {
	noisePCMFlag := true
	for g := uint8(0); g < s.numWindowGroups; g++ {
		for sfb := uint8(0); sfb < s.maxSFB && sfb < 64; sfb++ {
			cb := s.sfbCB[g][sfb]
			if cb == 0 {
				continue
			}
			if cb == 13 && noisePCMFlag { // first noise band is PCM-coded
				noisePCMFlag = false
				s.skip(9)
			} else {
				s.hcodSF()
			}
			if !s.ok {
				return
			}
		}
	}
}

func (s *aacRDBState) tnsData() {
	nFiltBits, lengthBits, orderBits := 2, 6, 5
	if s.windowSequence == 2 {
		nFiltBits, lengthBits, orderBits = 1, 4, 3
	}
	for w := uint8(0); w < s.numWindows; w++ {
		nFilt := s.read(nFiltBits)
		if nFilt == 0 {
			continue
		}
		startCoefBits := 3
		if s.read(1) == 1 { // coef_res
			startCoefBits = 4
		}
		for filt := uint32(0); filt < nFilt; filt++ {
			s.skip(lengthBits)
			order := s.read(orderBits)
			if order != 0 {
				s.skip(1) // direction
				coefBits := startCoefBits
				if s.read(1) == 1 { // coef_compress
					coefBits--
				}
				s.skip(int(order) * coefBits)
			}
		}
		if !s.ok {
			return
		}
	}
}

func (s *aacRDBState) ltpData() {
	s.skip(14) // ltp_lag + ltp_coef
	if s.windowSequence != 2 {
		n := int(s.maxSFB)
		if n > 40 {
			n = 40
		}
		s.skip(n)
	}
}

func (s *aacRDBState) spectralData() {
	for g := uint8(0); g < s.numWindowGroups; g++ {
		for i := uint8(0); i < s.numSec[g]; i++ {
			cb := s.sectCB[g][i]
			switch cb {
			case 0, 13, 14, 15:
				continue
			}
			if int(s.sectEnd[g][i]) >= int(s.numSWB)+1 {
				s.abort()
				return
			}
			step := 2
			if cb < 5 {
				step = 4
			}
			for k := s.sectSFBOffset[g][s.sectStart[g][i]]; k < s.sectSFBOffset[g][s.sectEnd[g][i]]; k += uint16(step) {
				s.hcod(cb)
				if !s.ok {
					return
				}
			}
		}
	}
}

// hcodSF walks the scale-factor huffman tree.
func (s *aacRDBState) hcodSF() {
	pos := 0
	for aacHuffmanSF[pos][1] != 0 {
		b := s.read(1)
		if !s.ok {
			return
		}
		pos += int(aacHuffmanSF[pos][b&1])
		if pos > 240 {
			s.abort()
			return
		}
	}
}

// hcod decodes one spectral huffman codeword (plus sign and escape bits).
func (s *aacRDBState) hcod(cb uint8) {
	var values [4]int8
	switch cb {
	case 1, 2, 4:
		s.hcod2Step(cb, values[:], 4)
	case 3:
		s.hcodBinary(cb, values[:], 4)
	case 5, 7, 9:
		s.hcodBinary(cb, values[:], 2)
	case 6, 8, 10, 11:
		s.hcod2Step(cb, values[:], 2)
	default:
		s.abort()
		return
	}
	if !s.ok {
		return
	}
	switch cb {
	case 1, 2, 5, 6:
	default: // with sign
		n := 2
		if cb < 5 {
			n = 4
		}
		for i := 0; i < n; i++ {
			if values[i] != 0 {
				s.skip(1)
			}
		}
	}
	if cb == 11 {
		for i := 0; i < 2; i++ {
			if values[i] == 16 || values[i] == -16 {
				bitCount := 3
				for {
					bitCount++
					escape := s.read(1)
					if !s.ok {
						return
					}
					if escape == 0 {
						break
					}
				}
				s.skip(bitCount)
			}
		}
	}
}

func (s *aacRDBState) hcod2Step(cb uint8, values []int8, count int) {
	toRead := int(aacHCB2StepBytes[cb])
	if toRead > s.remain() {
		toRead = s.remain()
	}
	codeWord := s.peek(toRead)
	step1 := aacHCB2Step[cb]
	if int(codeWord) >= len(step1) {
		s.abort()
		return
	}
	table := aacHCBTable[cb]
	offset := int(step1[codeWord].offset)
	extra := int(step1[codeWord].extra)
	if extra != 0 {
		s.skip(int(aacHCB2StepBytes[cb]))
		offset += int(s.peek(extra))
		if offset >= len(table) {
			s.abort()
			return
		}
		if diff := int(table[offset][0]) - int(aacHCB2StepBytes[cb]); diff != 0 {
			s.skip(diff)
		}
	} else {
		if offset >= len(table) {
			s.abort()
			return
		}
		s.skip(int(table[offset][0]))
	}
	if !s.ok {
		return
	}
	for i := 0; i < count; i++ {
		values[i] = table[offset][i+1]
	}
}

func (s *aacRDBState) hcodBinary(cb uint8, values []int8, count int) {
	table := aacHCBTable[cb]
	offset := 0
	for table[offset][0] == 0 {
		b := s.read(1)
		if !s.ok {
			return
		}
		offset += int(table[offset][1+(b&1)])
		if offset < 0 || offset >= len(table) {
			s.abort()
			return
		}
	}
	for i := 0; i < count; i++ {
		values[i] = table[offset][i+1]
	}
}
