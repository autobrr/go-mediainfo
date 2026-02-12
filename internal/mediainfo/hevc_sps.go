package mediainfo

import "encoding/binary"

// findHEVCSPSInConfig extracts the first SPS NAL (nal_unit_type 33) from an HEVC
// DecoderConfigurationRecord (hvcC).
func findHEVCSPSInConfig(payload []byte) []byte {
	// HEVCDecoderConfigurationRecord header is 23 bytes, then numOfArrays, then arrays.
	// We already validate len(payload) >= 23 in parseHEVCConfig().
	if len(payload) < 23 {
		return nil
	}
	numArrays := int(payload[22])
	offset := 23
	for a := 0; a < numArrays; a++ {
		if offset+3 > len(payload) {
			return nil
		}
		nalUnitType := payload[offset] & 0x3F
		offset++
		numNalus := int(binary.BigEndian.Uint16(payload[offset : offset+2])) //nolint:gosec // bounds checked above
		offset += 2
		for n := 0; n < numNalus; n++ {
			if offset+2 > len(payload) {
				return nil
			}
			nalLen := int(binary.BigEndian.Uint16(payload[offset : offset+2])) //nolint:gosec // bounds checked above
			offset += 2
			if nalLen <= 0 || offset+nalLen > len(payload) {
				return nil
			}
			nal := payload[offset : offset+nalLen]
			offset += nalLen
			if nalUnitType == 33 && len(nal) > 0 {
				return nal
			}
		}
	}
	return nil
}

type hevcShortTermRPS struct {
	numDeltaPocs int
}

func parseHEVCSPS(nal []byte) h264SPSInfo {
	rbsp := nalToRBSPWithHeader(nal, 2)
	if len(rbsp) == 0 {
		return h264SPSInfo{}
	}
	br := newBitReader(rbsp)

	if br.readBitsValue(4) == ^uint64(0) { // sps_video_parameter_set_id
		return h264SPSInfo{}
	}
	maxSubLayersMinus1 := int(br.readBitsValue(3)) // sps_max_sub_layers_minus1
	if maxSubLayersMinus1 < 0 || maxSubLayersMinus1 > 7 {
		return h264SPSInfo{}
	}
	if br.readBitsValue(1) == ^uint64(0) { // sps_temporal_id_nesting_flag
		return h264SPSInfo{}
	}
	profileIDC, tierName, levelIDC, ok := readHEVCProfileTierLevel(br, maxSubLayersMinus1)
	if !ok {
		return h264SPSInfo{}
	}
	if _, ok := br.readUEWithOk(); !ok { // sps_seq_parameter_set_id
		return h264SPSInfo{}
	}

	chromaFormatIDC, ok := br.readUEWithOk()
	if !ok {
		return h264SPSInfo{}
	}
	separateColourPlane := false
	if chromaFormatIDC == 3 {
		separateColourPlane = br.readBitsValue(1) == 1
	}

	picWidth, ok := br.readUEWithOk()
	if !ok {
		return h264SPSInfo{}
	}
	picHeight, ok := br.readUEWithOk()
	if !ok {
		return h264SPSInfo{}
	}

	confWinFlag := br.readBitsValue(1) == 1
	confLeft := 0
	confRight := 0
	confTop := 0
	confBottom := 0
	if confWinFlag {
		if confLeft, ok = br.readUEWithOk(); !ok {
			return h264SPSInfo{}
		}
		if confRight, ok = br.readUEWithOk(); !ok {
			return h264SPSInfo{}
		}
		if confTop, ok = br.readUEWithOk(); !ok {
			return h264SPSInfo{}
		}
		if confBottom, ok = br.readUEWithOk(); !ok {
			return h264SPSInfo{}
		}
	}

	bitDepthLumaMinus8, _ := br.readUEWithOk()
	_, _ = br.readUEWithOk() // bit_depth_chroma_minus8
	bitDepth := int(bitDepthLumaMinus8) + 8
	log2MaxPicOrderCntLSBMinus4, ok := br.readUEWithOk()
	if !ok {
		return h264SPSInfo{}
	}
	log2MaxPicOrderCntLSB := log2MaxPicOrderCntLSBMinus4 + 4

	subLayerOrderingInfoPresent := br.readBitsValue(1) == 1
	startLayer := 0
	if !subLayerOrderingInfoPresent {
		startLayer = maxSubLayersMinus1
	}
	for i := startLayer; i <= maxSubLayersMinus1; i++ {
		if _, ok := br.readUEWithOk(); !ok { // sps_max_dec_pic_buffering_minus1
			return h264SPSInfo{}
		}
		if _, ok := br.readUEWithOk(); !ok { // sps_max_num_reorder_pics
			return h264SPSInfo{}
		}
		if _, ok := br.readUEWithOk(); !ok { // sps_max_latency_increase_plus1
			return h264SPSInfo{}
		}
	}

	// Coding / transform block sizes and related knobs.
	for i := 0; i < 4; i++ {
		if _, ok := br.readUEWithOk(); !ok {
			return h264SPSInfo{}
		}
	}
	if _, ok := br.readUEWithOk(); !ok { // max_transform_hierarchy_depth_inter
		return h264SPSInfo{}
	}
	if _, ok := br.readUEWithOk(); !ok { // max_transform_hierarchy_depth_intra
		return h264SPSInfo{}
	}

	scalingListEnabled := br.readBitsValue(1) == 1
	if scalingListEnabled {
		if br.readBitsValue(1) == 1 { // sps_scaling_list_data_present_flag
			if !skipHEVCScalingListData(br) {
				return h264SPSInfo{}
			}
		}
	}

	// amp_enabled_flag, sample_adaptive_offset_enabled_flag
	if br.readBitsValue(1) == ^uint64(0) {
		return h264SPSInfo{}
	}
	if br.readBitsValue(1) == ^uint64(0) {
		return h264SPSInfo{}
	}

	pcmEnabled := br.readBitsValue(1) == 1
	if pcmEnabled {
		if br.readBitsValue(4) == ^uint64(0) { // pcm_sample_bit_depth_luma_minus1
			return h264SPSInfo{}
		}
		if br.readBitsValue(4) == ^uint64(0) { // pcm_sample_bit_depth_chroma_minus1
			return h264SPSInfo{}
		}
		if _, ok := br.readUEWithOk(); !ok { // log2_min_pcm_luma_coding_block_size_minus3
			return h264SPSInfo{}
		}
		if _, ok := br.readUEWithOk(); !ok { // log2_diff_max_min_pcm_luma_coding_block_size
			return h264SPSInfo{}
		}
		if br.readBitsValue(1) == ^uint64(0) { // pcm_loop_filter_disabled_flag
			return h264SPSInfo{}
		}
	}

	numShortTermRPS, ok := br.readUEWithOk()
	if !ok || numShortTermRPS < 0 {
		return h264SPSInfo{}
	}
	rps := make([]hevcShortTermRPS, numShortTermRPS)
	for i := 0; i < numShortTermRPS; i++ {
		nd, ok := skipHEVCShortTermRefPicSet(br, i, rps)
		if !ok {
			return h264SPSInfo{}
		}
		rps[i].numDeltaPocs = nd
	}

	if br.readBitsValue(1) == 1 { // long_term_ref_pics_present_flag
		numLongTerm, ok := br.readUEWithOk()
		if !ok || numLongTerm < 0 {
			return h264SPSInfo{}
		}
		for i := 0; i < numLongTerm; i++ {
			if br.readBitsValue(uint8(log2MaxPicOrderCntLSB)) == ^uint64(0) { // lt_ref_pic_poc_lsb_sps
				return h264SPSInfo{}
			}
			if br.readBitsValue(1) == ^uint64(0) { // used_by_curr_pic_lt_sps_flag
				return h264SPSInfo{}
			}
		}
	}

	// sps_temporal_mvp_enabled_flag, strong_intra_smoothing_enabled_flag
	if br.readBitsValue(1) == ^uint64(0) {
		return h264SPSInfo{}
	}
	if br.readBitsValue(1) == ^uint64(0) {
		return h264SPSInfo{}
	}

	colorRange := ""
	hasColorRange := false
	colorPrimaries := ""
	transfer := ""
	matrix := ""
	hasColorDescription := false
	frameRate := float64(0)

	if br.readBitsValue(1) == 1 { // vui_parameters_present_flag
		colorRange, hasColorRange, colorPrimaries, transfer, matrix, hasColorDescription, frameRate, _ = parseHEVCVUI(br)
	}

	// Conformance window cropping.
	width := picWidth
	height := picHeight
	if confWinFlag {
		subWidthC := 1
		subHeightC := 1
		switch chromaFormatIDC {
		case 1:
			subWidthC = 2
			subHeightC = 2
		case 2:
			subWidthC = 2
			subHeightC = 1
		case 3:
			subWidthC = 1
			subHeightC = 1
			if separateColourPlane {
				subWidthC = 1
				subHeightC = 1
			}
		}
		cropUnitX := subWidthC
		cropUnitY := subHeightC
		if width > (confLeft+confRight)*cropUnitX {
			width -= (confLeft + confRight) * cropUnitX
		}
		if height > (confTop+confBottom)*cropUnitY {
			height -= (confTop + confBottom) * cropUnitY
		}
	}

	info := h264SPSInfo{
		ChromaFormat:            hevcChromaFormatName(byte(chromaFormatIDC)),
		BitDepth:                bitDepth,
		ColorRange:              colorRange,
		HasColorRange:           hasColorRange,
		ColorPrimaries:          colorPrimaries,
		TransferCharacteristics: transfer,
		MatrixCoefficients:      matrix,
		HasColorDescription:     hasColorDescription,
		ProfileID:               profileIDC,
		LevelID:                 levelIDC,
		HEVCTier:                tierName,
		Width:                   uint64(width),
		Height:                  uint64(height),
		CodedWidth:              uint64(picWidth),
		CodedHeight:             uint64(picHeight),
		FrameRate:               frameRate,
	}
	return info
}

func readHEVCProfileTierLevel(br *bitReader, maxSubLayersMinus1 int) (byte, string, byte, bool) {
	// general_profile_space (2), general_tier_flag (1), general_profile_idc (5)
	if br.readBitsValue(2) == ^uint64(0) {
		return 0, "", 0, false
	}
	tierFlag := br.readBitsValue(1)
	profileIDC := br.readBitsValue(5)
	if tierFlag == ^uint64(0) || profileIDC == ^uint64(0) {
		return 0, "", 0, false
	}
	// general_profile_compatibility_flags (32)
	if br.readBitsValue(32) == ^uint64(0) {
		return 0, "", 0, false
	}
	// general_constraint_indicator_flags (48)
	if br.readBitsValue(48) == ^uint64(0) {
		return 0, "", 0, false
	}
	// general_level_idc (8)
	levelIDC := br.readBitsValue(8)
	if levelIDC == ^uint64(0) {
		return 0, "", 0, false
	}

	subProfilePresent := make([]bool, maxSubLayersMinus1)
	subLevelPresent := make([]bool, maxSubLayersMinus1)
	for i := 0; i < maxSubLayersMinus1; i++ {
		v := br.readBitsValue(1)
		if v == ^uint64(0) {
			return 0, "", 0, false
		}
		subProfilePresent[i] = v == 1
		v = br.readBitsValue(1)
		if v == ^uint64(0) {
			return 0, "", 0, false
		}
		subLevelPresent[i] = v == 1
	}
	if maxSubLayersMinus1 > 0 {
		for i := maxSubLayersMinus1; i < 8; i++ {
			if br.readBitsValue(2) == ^uint64(0) {
				return 0, "", 0, false
			}
		}
	}
	for i := 0; i < maxSubLayersMinus1; i++ {
		if subProfilePresent[i] {
			if br.readBitsValue(2) == ^uint64(0) || br.readBitsValue(1) == ^uint64(0) || br.readBitsValue(5) == ^uint64(0) {
				return 0, "", 0, false
			}
			if br.readBitsValue(32) == ^uint64(0) {
				return 0, "", 0, false
			}
			if br.readBitsValue(48) == ^uint64(0) {
				return 0, "", 0, false
			}
		}
		if subLevelPresent[i] {
			if br.readBitsValue(8) == ^uint64(0) {
				return 0, "", 0, false
			}
		}
	}
	return byte(profileIDC), hevcTierName(byte(tierFlag)), byte(levelIDC), true
}

func skipHEVCScalingListData(br *bitReader) bool {
	for sizeID := 0; sizeID < 4; sizeID++ {
		step := 1
		if sizeID == 3 {
			step = 3
		}
		for matrixID := 0; matrixID < 6; matrixID += step {
			predMode := br.readBitsValue(1)
			if predMode == ^uint64(0) {
				return false
			}
			if predMode == 0 {
				if _, ok := br.readUEWithOk(); !ok { // scaling_list_pred_matrix_id_delta
					return false
				}
				continue
			}
			coefNum := 1 << (4 + (sizeID << 1))
			if coefNum > 64 {
				coefNum = 64
			}
			if sizeID > 1 {
				if _, ok := br.readSEWithOk(); !ok { // scaling_list_dc_coef_minus8
					return false
				}
			}
			for i := 0; i < coefNum; i++ {
				if _, ok := br.readSEWithOk(); !ok { // scaling_list_delta_coef
					return false
				}
			}
		}
	}
	return true
}

func skipHEVCShortTermRefPicSet(br *bitReader, idx int, sets []hevcShortTermRPS) (int, bool) {
	interPred := false
	if idx != 0 {
		v := br.readBitsValue(1) // inter_ref_pic_set_prediction_flag
		if v == ^uint64(0) {
			return 0, false
		}
		interPred = v == 1
	}
	if interPred {
		deltaIdxMinus1, ok := br.readUEWithOk()
		if !ok || deltaIdxMinus1 < 0 {
			return 0, false
		}
		refIdx := idx - (deltaIdxMinus1 + 1)
		if refIdx < 0 || refIdx >= idx {
			refIdx = idx - 1
		}
		if br.readBitsValue(1) == ^uint64(0) { // delta_rps_sign
			return 0, false
		}
		if _, ok := br.readUEWithOk(); !ok { // abs_delta_rps_minus1
			return 0, false
		}
		numRef := 0
		if refIdx >= 0 && refIdx < len(sets) {
			numRef = sets[refIdx].numDeltaPocs
		}
		numRefIdc := numRef + 1
		newNumDelta := 0
		for j := 0; j < numRefIdc; j++ {
			used := br.readBitsValue(1) // used_by_curr_pic_flag
			if used == ^uint64(0) {
				return 0, false
			}
			if used == 1 {
				newNumDelta++
				continue
			}
			useDelta := br.readBitsValue(1) // use_delta_flag
			if useDelta == ^uint64(0) {
				return 0, false
			}
			if useDelta == 1 {
				newNumDelta++
			}
		}
		return newNumDelta, true
	}

	numNeg, ok := br.readUEWithOk()
	if !ok || numNeg < 0 {
		return 0, false
	}
	numPos, ok := br.readUEWithOk()
	if !ok || numPos < 0 {
		return 0, false
	}
	for i := 0; i < numNeg; i++ {
		if _, ok := br.readUEWithOk(); !ok { // delta_poc_s0_minus1
			return 0, false
		}
		if br.readBitsValue(1) == ^uint64(0) { // used_by_curr_pic_s0_flag
			return 0, false
		}
	}
	for i := 0; i < numPos; i++ {
		if _, ok := br.readUEWithOk(); !ok { // delta_poc_s1_minus1
			return 0, false
		}
		if br.readBitsValue(1) == ^uint64(0) { // used_by_curr_pic_s1_flag
			return 0, false
		}
	}
	return numNeg + numPos, true
}

func parseHEVCVUI(br *bitReader) (string, bool, string, string, string, bool, float64, bool) {
	colorRange := ""
	hasColorRange := false
	colorPrimaries := ""
	transfer := ""
	matrix := ""
	hasColorDescription := false
	frameRate := float64(0)
	hasTiming := false

	if br.readBitsValue(1) == 1 { // aspect_ratio_info_present_flag
		aspectRatioIDC := br.readBitsValue(8)
		if aspectRatioIDC == 255 {
			_ = br.readBitsValue(16)
			_ = br.readBitsValue(16)
		}
	}
	if br.readBitsValue(1) == 1 { // overscan_info_present_flag
		_ = br.readBitsValue(1)
	}
	if br.readBitsValue(1) == 1 { // video_signal_type_present_flag
		_ = br.readBitsValue(3) // video_format
		fullRange := br.readBitsValue(1) == 1
		if fullRange {
			colorRange = "Full"
		} else {
			colorRange = "Limited"
		}
		hasColorRange = true
		if br.readBitsValue(1) == 1 { // colour_description_present_flag
			primaries := br.readBitsValue(8)
			trc := br.readBitsValue(8)
			mat := br.readBitsValue(8)
			colorPrimaries = matroskaColorPrimariesName(primaries)
			transfer = matroskaTransferName(trc)
			matrix = matroskaMatrixName(mat)
			hasColorDescription = true
		}
	}
	if br.readBitsValue(1) == 1 { // chroma_loc_info_present_flag
		_, _ = br.readUEWithOk()
		_, _ = br.readUEWithOk()
	}
	// neutral_chroma_indication_flag, field_seq_flag, frame_field_info_present_flag
	_ = br.readBitsValue(1)
	_ = br.readBitsValue(1)
	_ = br.readBitsValue(1)
	if br.readBitsValue(1) == 1 { // default_display_window_flag
		_, _ = br.readUEWithOk()
		_, _ = br.readUEWithOk()
		_, _ = br.readUEWithOk()
		_, _ = br.readUEWithOk()
	}
	if br.readBitsValue(1) == 1 { // vui_timing_info_present_flag
		numUnitsInTick := br.readBitsValue(32)
		timeScale := br.readBitsValue(32)
		if numUnitsInTick != ^uint64(0) && timeScale != ^uint64(0) && numUnitsInTick > 0 && timeScale > 0 {
			// HEVC VUI: frame rate = time_scale / num_units_in_tick (unlike AVC which uses /2).
			frameRate = float64(timeScale) / float64(numUnitsInTick)
			hasTiming = true
		}
		if br.readBitsValue(1) == 1 { // poc_proportional_to_timing_flag
			_, _ = br.readUEWithOk()
		}
		if br.readBitsValue(1) == 1 { // hrd_parameters_present_flag
			// Skip hrd_parameters() - not needed for Matroska parity work right now.
			// If present, abort VUI parsing to avoid desync.
			return colorRange, hasColorRange, colorPrimaries, transfer, matrix, hasColorDescription, frameRate, hasTiming
		}
	}
	if br.readBitsValue(1) == 1 { // bitstream_restriction_flag
		_ = br.readBitsValue(1)
		_ = br.readBitsValue(1)
		_ = br.readBitsValue(1)
		_ = br.readBitsValue(1)
		_, _ = br.readUEWithOk()
		_, _ = br.readUEWithOk()
		_, _ = br.readUEWithOk()
		_, _ = br.readUEWithOk()
		_, _ = br.readUEWithOk()
	}

	return colorRange, hasColorRange, colorPrimaries, transfer, matrix, hasColorDescription, frameRate, hasTiming
}
