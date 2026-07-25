package mediainfo

import (
	"encoding/hex"
	"testing"
)

func TestParseHEVCSPSInterPredictedRPSKeepsVUIAlignment(t *testing.T) {
	// Real hvcC with an inter-predicted short-term RPS followed by BT.2020/PQ VUI
	// and chroma location type 2. Reading delta_idx_minus1 inside the SPS shifts
	// the remaining syntax and loses all VUI metadata.
	raw := "01022000000090000000000096f000fcfdfafa00000f04a00001001840010c01ffff02200000030090000003000003009695c090a100010049420101022000000300900000030000030096a001e020021c4d9e5792429185164aaacb9b9ebce40977eb9978f016a1220136c2000007d20000bb81f455ef7e00e5001ca801ca003951a2000100084401c13ca2958f24a70001001a4e01051559bffb2fb66311e2a3550050c249004801640a289380"
	config, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, sps := parseHEVCConfig(config)
	if !sps.HasColorRange || sps.ColorRange != "Limited" {
		t.Fatalf("color range = %q, present=%v", sps.ColorRange, sps.HasColorRange)
	}
	if sps.ColorPrimaries != "BT.2020" || sps.TransferCharacteristics != "PQ" || sps.MatrixCoefficients != "BT.2020 non-constant" {
		t.Fatalf("unexpected color description: %+v", sps)
	}
	if !sps.HasChromaLoc || sps.ChromaSampleLoc != 2 {
		t.Fatalf("chroma location = %d, present=%v", sps.ChromaSampleLoc, sps.HasChromaLoc)
	}
}

func TestParseHEVCVUIPreservesVideoFormat(t *testing.T) {
	w := &hevcBitBuf{}
	w.put(0, 1) // aspect_ratio_info_present_flag
	w.put(0, 1) // overscan_info_present_flag
	w.put(1, 1) // video_signal_type_present_flag
	w.put(2, 3) // video_format: NTSC
	w.put(0, 1) // video_full_range_flag
	w.put(0, 1) // colour_description_present_flag
	w.put(0, 1) // chroma_loc_info_present_flag
	w.put(0, 3) // neutral_chroma, field_seq, frame_field_info
	w.put(0, 1) // default_display_window_flag
	w.put(0, 1) // vui_timing_info_present_flag
	w.put(0, 1) // bitstream_restriction_flag
	info := parseHEVCVUI(newBitReader(w.bytes()))
	if !info.hasVideoFormat || info.videoFormat != 2 {
		t.Fatalf("video format = %d, present=%v; want NTSC signal", info.videoFormat, info.hasVideoFormat)
	}
}

// hevcBitBuf is a minimal MSB-first bit accumulator for crafting HEVC SPS RBSP in
// tests, with an Exp-Golomb (ue) writer.
type hevcBitBuf struct{ bits []byte }

func (w *hevcBitBuf) put(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		w.bits = append(w.bits, byte((v>>uint(i))&1))
	}
}

// ue writes a standard unsigned Exp-Golomb code.
func (w *hevcBitBuf) ue(v uint64) {
	m := v + 1
	n := 0
	for t := m; t > 0; t >>= 1 {
		n++
	}
	for i := 0; i < n-1; i++ {
		w.bits = append(w.bits, 0)
	}
	w.put(m, n)
}

// ueHugeZeros writes an Exp-Golomb code with `zeros` leading zero bits (decoding to
// (1<<zeros)-1), used to drive an attacker-controlled count to a huge value.
func (w *hevcBitBuf) ueHugeZeros(zeros int) {
	for i := 0; i < zeros; i++ {
		w.bits = append(w.bits, 0)
	}
	w.bits = append(w.bits, 1)
	for i := 0; i < zeros; i++ {
		w.bits = append(w.bits, 0)
	}
}

func (w *hevcBitBuf) bytes() []byte {
	out := make([]byte, (len(w.bits)+7)/8)
	for i, b := range w.bits {
		if b != 0 {
			out[i>>3] |= 1 << (7 - (i & 7))
		}
	}
	return out
}

// buildHEVCSPSHugeShortTermRPS builds a syntactically valid HEVC SPS NAL (1 temporal
// sub-layer) whose num_short_term_ref_pic_sets is encoded as a huge value, to exercise
// the slice allocation at parseHEVCSPS's short-term RPS loop.
func buildHEVCSPSHugeShortTermRPS(rpsZeros int) []byte {
	w := &hevcBitBuf{}
	w.put(0, 4) // sps_video_parameter_set_id
	w.put(0, 3) // sps_max_sub_layers_minus1 = 0
	w.put(0, 1) // sps_temporal_id_nesting_flag
	// profile_tier_level (maxSubLayersMinus1 == 0): fixed 96 bits.
	w.put(0, 2)               // general_profile_space
	w.put(0, 1)               // general_tier_flag (Main)
	w.put(1, 5)               // general_profile_idc
	w.put(0xFFFFFFFF, 32)     // general_profile_compatibility_flags (non-zero: avoid 00 00 03)
	w.put(0xFFFFFFFFFFFF, 48) // general_constraint_indicator_flags
	w.put(120, 8)             // general_level_idc
	w.ue(0)                   // sps_seq_parameter_set_id
	w.ue(1)                   // chroma_format_idc = 1 (4:2:0)
	w.ue(2)                   // pic_width_in_luma_samples
	w.ue(2)                   // pic_height_in_luma_samples
	w.put(0, 1)               // conformance_window_flag = 0
	w.ue(0)                   // bit_depth_luma_minus8
	w.ue(0)                   // bit_depth_chroma_minus8
	w.ue(0)                   // log2_max_pic_order_cnt_lsb_minus4
	w.put(0, 1)               // sps_sub_layer_ordering_info_present_flag = 0 -> 1 iteration
	w.ue(0)                   // sps_max_dec_pic_buffering_minus1[0]
	w.ue(0)                   // sps_max_num_reorder_pics[0]
	w.ue(0)                   // sps_max_latency_increase_plus1[0]
	w.ue(0)                   // log2_min_luma_coding_block_size_minus3
	w.ue(0)                   // log2_diff_max_min_luma_coding_block_size
	w.ue(0)                   // log2_min_luma_transform_block_size_minus2
	w.ue(0)                   // log2_diff_max_min_luma_transform_block_size
	w.ue(0)                   // max_transform_hierarchy_depth_inter
	w.ue(0)                   // max_transform_hierarchy_depth_intra
	w.put(0, 1)               // scaling_list_enabled_flag = 0
	w.put(0, 1)               // amp_enabled_flag = 0
	w.put(0, 1)               // sample_adaptive_offset_enabled_flag = 0
	w.put(0, 1)               // pcm_enabled_flag = 0
	w.ueHugeZeros(rpsZeros)   // num_short_term_ref_pic_sets (huge)

	nal := append([]byte{0x42, 0x01}, w.bytes()...) // nal_unit_type 33 header
	// Trailing bytes so the huge ue's value field never hits EOF before allocation.
	nal = append(nal, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF)
	return nal
}

// TestParseHEVCSPSRejectsHugeShortTermRPS ensures a crafted SPS with an absurd
// num_short_term_ref_pic_sets is rejected rather than triggering an unbounded slice
// allocation (panic/OOM). This path is fuzz-reachable via MP4 hvcC, Matroska
// CodecPrivate, and the MPEG-TS/BDAV Annex-B parser.
func TestParseHEVCSPSRejectsHugeShortTermRPS(t *testing.T) {
	// (1<<52)-1 ~ 4.5e15: positive (no int overflow), and *8 exceeds maxAlloc, so an
	// unbounded make() would panic with "makeslice: len out of range".
	nal := buildHEVCSPSHugeShortTermRPS(52)

	var got h264SPSInfo
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("parseHEVCSPS panicked on crafted SPS (unbounded allocation): %v", r)
			}
		}()
		got = parseHEVCSPS(nal)
	}()

	if got.Width != 0 || got.ProfileID != 0 || got.ChromaFormat != "" {
		t.Errorf("expected empty h264SPSInfo for rejected SPS, got %+v", got)
	}
}
