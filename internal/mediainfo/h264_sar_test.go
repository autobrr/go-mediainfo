package mediainfo

import "testing"

func TestParseH264SPS_ExtendedSAR(t *testing.T) {
	// SPS extracted from a real-world MP4 sample; official mediainfo reports PixelAspectRatio=1.001 (1001/1000).
	sps := []byte{
		0x67, 0x64, 0x00, 0x1e, 0xac, 0xd9, 0x40, 0xa0, 0x2f, 0xf9,
		0x7f, 0xf0, 0x50, 0x10, 0x50, 0x01, 0x00, 0x00, 0x03, 0x00,
		0x01, 0x00, 0x00, 0x03, 0x00, 0x28, 0x0f, 0x16, 0x2d, 0x96,
	}
	info := parseH264SPS(sps)
	if !info.HasSAR {
		t.Fatalf("expected HasSAR")
	}
	if info.SARWidth != 1281 || info.SARHeight != 1280 {
		t.Fatalf("sar=%d:%d, want 1281:1280", info.SARWidth, info.SARHeight)
	}
}

func TestParseAVCConfig_StandardSAR(t *testing.T) {
	config := []byte{
		0x01, 0x64, 0x00, 0x29, 0xff, 0xe1, 0x00, 0x1e,
		0x67, 0x64, 0x00, 0x29, 0xac, 0x72, 0x04, 0x40,
		0xb4, 0x3d, 0xbf, 0xf0, 0x00, 0x80, 0x00, 0x91,
		0x00, 0x00, 0x03, 0x03, 0xe9, 0x00, 0x00, 0xea,
		0x60, 0x8f, 0x18, 0x31, 0x84, 0x60, 0x01, 0x00,
		0x07, 0x68, 0xe8, 0x43, 0x82, 0xb2, 0xc8, 0xb0,
	}

	_, _, info := parseAVCConfig(config)
	if !info.HasSAR || info.SARWidth != 8 || info.SARHeight != 9 {
		t.Fatalf("SAR=%d:%d (present=%v), want 8:9", info.SARWidth, info.SARHeight, info.HasSAR)
	}
}

func TestParseAVCConfigUsesLaterValidSPS(t *testing.T) {
	validSPS := []byte{
		0x67, 0x64, 0x00, 0x29, 0xac, 0x72, 0x04, 0x40,
		0xb4, 0x3d, 0xbf, 0xf0, 0x00, 0x80, 0x00, 0x91,
		0x00, 0x00, 0x03, 0x03, 0xe9, 0x00, 0x00, 0xea,
		0x60, 0x8f, 0x18, 0x31, 0x84, 0x60,
	}
	config := []byte{1, 0x64, 0, 0x29, 0xff, 0xe2, 0, 1, 0}
	config = append(config, byte(len(validSPS)>>8), byte(len(validSPS)))
	config = append(config, validSPS...)
	config = append(config, 0) // no PPS
	_, _, info, details := parseAVCConfigDetails(config)
	if info.ProfileID == 0 || !info.HasSAR || info.SARWidth != 8 || info.SARHeight != 9 {
		t.Fatalf("later valid SPS was not selected: %+v", info)
	}
	if len(details.parameterSets) != 4+1+4+len(validSPS) {
		t.Fatalf("Annex-B parameter sets length = %d", len(details.parameterSets))
	}
}

func TestParseH264SPS_DualHRD(t *testing.T) {
	sps := []byte{
		0x67, 0x64, 0x00, 0x29, 0xac, 0x1b, 0x1a, 0x50, 0x1e, 0x00,
		0x89, 0xf9, 0x70, 0x11, 0x00, 0x00, 0x03, 0x03, 0xe9, 0x00,
		0x00, 0xbb, 0x80, 0xe2, 0x60, 0x00, 0x04, 0x69, 0x26, 0x00,
		0x00, 0x72, 0x70, 0xe8, 0xc4, 0xb8, 0xc4, 0xc0, 0x00, 0x08,
		0xd2, 0x4c, 0x00, 0x00, 0xe4, 0xe1, 0xd1, 0x89, 0x70, 0xf8,
		0xe1, 0x85, 0x2c,
	}
	info := parseH264SPS(sps)
	if !info.HasBitRateNAL || !info.HasBitRateVCL || info.BitRateNAL != 36_999_936 || info.BitRateVCL != 36_999_936 {
		t.Fatalf("HRD bitrates = %d/%d (present %v/%v)", info.BitRateNAL, info.BitRateVCL, info.HasBitRateNAL, info.HasBitRateVCL)
	}
	if !info.HasBufferSizeNAL || !info.HasBufferSizeVCL || info.BufferSizeNAL != 30_000_000 || info.BufferSizeVCL != 30_000_000 {
		t.Fatalf("HRD buffers = %d/%d (present %v/%v)", info.BufferSizeNAL, info.BufferSizeVCL, info.HasBufferSizeNAL, info.HasBufferSizeVCL)
	}
	if info.BitRateCBR || !info.HasBitRateCBR {
		t.Fatalf("HRD mode CBR=%v present=%v; want explicit VBR", info.BitRateCBR, info.HasBitRateCBR)
	}
}

func TestParseH264SPSMonochromeInterlacedCropUnit(t *testing.T) {
	w := &hevcBitBuf{}
	w.put(100, 8) // profile_idc
	w.put(0, 8)   // constraints
	w.put(30, 8)  // level_idc
	w.ue(0)       // seq_parameter_set_id
	w.ue(0)       // chroma_format_idc: monochrome
	w.ue(0)       // bit_depth_luma_minus8
	w.ue(0)       // bit_depth_chroma_minus8
	w.put(0, 1)   // qpprime_y_zero_transform_bypass_flag
	w.put(0, 1)   // seq_scaling_matrix_present_flag
	w.ue(0)       // log2_max_frame_num_minus4
	w.ue(0)       // pic_order_cnt_type
	w.ue(0)       // log2_max_pic_order_cnt_lsb_minus4
	w.ue(0)       // max_num_ref_frames
	w.put(0, 1)   // gaps_in_frame_num_value_allowed_flag
	w.ue(0)       // pic_width_in_mbs_minus1: 16
	w.ue(1)       // pic_height_in_map_units_minus1: 64 interlaced
	w.put(0, 1)   // frame_mbs_only_flag
	w.put(0, 1)   // mb_adaptive_frame_field_flag
	w.put(1, 1)   // direct_8x8_inference_flag
	w.put(1, 1)   // frame_cropping_flag
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.ue(1)     // bottom crop: 1 * CropUnitY(2)
	w.put(0, 1) // vui_parameters_present_flag

	info := parseH264SPS(append([]byte{0x67}, w.bytes()...))
	if info.CodedHeight != 64 || info.Height != 62 {
		t.Fatalf("monochrome interlaced height = %d/%d, want 64/62", info.CodedHeight, info.Height)
	}
}

func TestParseH264SPSSeparateColourPlaneReadsTwelveScalingLists(t *testing.T) {
	w := &hevcBitBuf{}
	w.put(100, 8)
	w.put(0, 8)
	w.put(30, 8)
	w.ue(0)
	w.ue(3)     // chroma_format_idc: 4:4:4
	w.put(1, 1) // separate_colour_plane_flag
	w.ue(0)
	w.ue(0)
	w.put(0, 1)
	w.put(1, 1) // seq_scaling_matrix_present_flag
	for range 12 {
		w.put(0, 1) // seq_scaling_list_present_flag
	}
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.put(0, 1)
	w.ue(0)
	w.ue(0)
	w.put(1, 1) // frame_mbs_only_flag
	w.put(1, 1) // direct_8x8_inference_flag
	w.put(0, 1) // frame_cropping_flag
	w.put(0, 1) // vui_parameters_present_flag

	info := parseH264SPS(append([]byte{0x67}, w.bytes()...))
	if info.Width != 16 || info.Height != 16 || !info.SeparateColourPlane {
		t.Fatalf("separate-plane 4:4:4 SPS desynchronized: %+v", info)
	}
}
