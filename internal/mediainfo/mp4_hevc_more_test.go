package mediainfo

import "testing"

// hevcSPSOpts configures buildHEVCSPS to emit a complete, valid HEVC SPS NAL
// (1 temporal sub-layer, 4:2:0, 8-bit) with selectable picture dimensions,
// conformance window, and VUI colour / chroma-loc signaling.
type hevcSPSOpts struct {
	picWidth, picHeight                      uint64
	confWin                                  bool
	confLeft, confRight, confTop, confBottom uint64
	vui                                      bool
	videoSignalType                          bool
	fullRange                                bool
	colourDesc                               bool
	primaries, transfer, matrix              uint64
	chromaLoc                                int // -1 = not signaled
}

// emulationEncode inserts emulation-prevention bytes so the RBSP round-trips through
// nalToRBSPWithHeader unchanged (00 00 {00..03} -> 00 00 03 {..}).
func emulationEncode(rbsp []byte) []byte {
	out := make([]byte, 0, len(rbsp)+len(rbsp)/3+1)
	zeros := 0
	for _, b := range rbsp {
		if zeros >= 2 && b <= 0x03 {
			out = append(out, 0x03)
			zeros = 0
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

func buildHEVCSPS(o hevcSPSOpts) []byte {
	w := &hevcBitBuf{}
	w.put(0, 4) // sps_video_parameter_set_id
	w.put(0, 3) // sps_max_sub_layers_minus1 = 0
	w.put(0, 1) // sps_temporal_id_nesting_flag
	// profile_tier_level (96 bits): all-ones compat/constraint keep this region dense.
	w.put(0, 2)
	w.put(0, 1)
	w.put(1, 5)
	w.put(0xFFFFFFFF, 32)
	w.put(0xFFFFFFFFFFFF, 48)
	w.put(120, 8)
	w.ue(0) // sps_seq_parameter_set_id
	w.ue(1) // chroma_format_idc = 1 (4:2:0)
	w.ue(o.picWidth)
	w.ue(o.picHeight)
	if o.confWin {
		w.put(1, 1)
		w.ue(o.confLeft)
		w.ue(o.confRight)
		w.ue(o.confTop)
		w.ue(o.confBottom)
	} else {
		w.put(0, 1)
	}
	w.ue(0) // bit_depth_luma_minus8
	w.ue(0) // bit_depth_chroma_minus8
	w.ue(0) // log2_max_pic_order_cnt_lsb_minus4
	w.put(0, 1)
	w.ue(0)
	w.ue(0)
	w.ue(0) // sub_layer_ordering (1 iteration)
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.ue(0) // coding/transform block sizes
	w.ue(0)
	w.ue(0)     // max_transform_hierarchy_depth inter/intra
	w.put(0, 1) // scaling_list_enabled_flag
	w.put(0, 1) // amp_enabled_flag
	w.put(0, 1) // sample_adaptive_offset_enabled_flag
	w.put(0, 1) // pcm_enabled_flag
	w.ue(0)     // num_short_term_ref_pic_sets = 0
	w.put(0, 1) // long_term_ref_pics_present_flag
	w.put(0, 1) // sps_temporal_mvp_enabled_flag
	w.put(0, 1) // strong_intra_smoothing_enabled_flag
	if o.vui {
		w.put(1, 1) // vui_parameters_present_flag
		w.put(0, 1) // aspect_ratio_info_present_flag
		w.put(0, 1) // overscan_info_present_flag
		if o.videoSignalType {
			w.put(1, 1) // video_signal_type_present_flag
			w.put(5, 3) // video_format
			if o.fullRange {
				w.put(1, 1)
			} else {
				w.put(0, 1)
			}
			if o.colourDesc {
				w.put(1, 1) // colour_description_present_flag
				w.put(o.primaries, 8)
				w.put(o.transfer, 8)
				w.put(o.matrix, 8)
			} else {
				w.put(0, 1)
			}
		} else {
			w.put(0, 1)
		}
		if o.chromaLoc >= 0 {
			w.put(1, 1) // chroma_loc_info_present_flag
			w.ue(uint64(o.chromaLoc))
			w.ue(uint64(o.chromaLoc))
		} else {
			w.put(0, 1)
		}
		w.put(0, 1) // neutral_chroma_indication_flag
		w.put(0, 1) // field_seq_flag
		w.put(0, 1) // frame_field_info_present_flag
		w.put(0, 1) // default_display_window_flag
		w.put(0, 1) // vui_timing_info_present_flag
		w.put(0, 1) // bitstream_restriction_flag
	} else {
		w.put(0, 1)
	}
	nal := append([]byte{0x42, 0x01}, emulationEncode(w.bytes())...)
	return append(nal, 0x80) // rbsp_stop bit padding
}

func hevcField(fs []Field, name string) string {
	for _, f := range fs {
		if f.Name == name {
			return f.Value
		}
	}
	return ""
}

// TestBuildHEVCSPSRoundTrip validates the test SPS builder: a built SPS parses back to
// the dimensions and colour it encoded (guards the builder used by the tests below).
func TestBuildHEVCSPSRoundTrip(t *testing.T) {
	sps := buildHEVCSPS(hevcSPSOpts{picWidth: 1920, picHeight: 1080, vui: true, videoSignalType: true, colourDesc: true, primaries: 1, transfer: 1, matrix: 1, chromaLoc: -1})
	got := parseHEVCSPS(sps)
	if got.Width != 1920 || got.Height != 1080 {
		t.Fatalf("round-trip dims = %dx%d, want 1920x1080", got.Width, got.Height)
	}
	if !got.HasColorDescription || got.ColorPrimaries == "" {
		t.Fatalf("round-trip colour: HasColorDescription=%v primaries=%q, want signaled", got.HasColorDescription, got.ColorPrimaries)
	}
}

// TestParseVisualSampleEntryDispatchesHEVC covers the hvc1/hev1 dispatch branch in
// parseVisualSampleEntry (the integration point), for both codec IDs, and asserts
// "Color space" is emitted exactly once.
func TestParseVisualSampleEntryDispatchesHEVC(t *testing.T) {
	for _, sampleType := range []string{"hvc1", "hev1"} {
		t.Run(sampleType, func(t *testing.T) {
			entry := buildHEVCSampleEntry(buildHVCCRecord(hevcSPS8bit, buildX265SEINAL()))
			res := parseVisualSampleEntry(entry, sampleType)
			if v := hevcField(res.Fields, "Format profile"); v != "Main@L2" {
				t.Errorf("Format profile = %q, want Main@L2", v)
			}
			if v := hevcField(res.Fields, "Format tier"); v != "Main" {
				t.Errorf("Format tier = %q, want Main", v)
			}
			if v := hevcField(res.Fields, "Writing library"); v != "x265 9.9" {
				t.Errorf("Writing library = %q, want x265 9.9", v)
			}
			n := 0
			for _, f := range res.Fields {
				if f.Name == "Color space" {
					n++
				}
			}
			if n != 1 {
				t.Errorf("Color space emitted %d times, want exactly 1", n)
			}
		})
	}
}

// TestParseMP4HEVC10Bit covers the 10-bit bit-depth path (hvcC byte 17 lower bits).
func TestParseMP4HEVC10Bit(t *testing.T) {
	rec := buildHVCCRecord(hevcSPS8bit, nil)
	rec[17] = 0xF8 | 0x02 // bitDepthLumaMinus8 = 2 -> 10-bit
	fields, _ := parseMP4HEVCSampleEntry(buildHEVCSampleEntry(rec), 320, 240, &mp4StructuredFacts{})
	if v := hevcField(fields, "Bit depth"); v != "10 bits" {
		t.Errorf("Bit depth = %q, want 10 bits", v)
	}
}

// TestParseMP4HEVCColourDescriptionStreamSource covers the positive colour-description
// path: primaries/transfer/matrix are emitted with Source "Stream" (NOT the AVC path's
// "Container / Stream"), the most parity-sensitive HEVC-vs-AVC divergence.
func TestParseMP4HEVCColourDescriptionStreamSource(t *testing.T) {
	sps := buildHEVCSPS(hevcSPSOpts{picWidth: 320, picHeight: 240, vui: true, videoSignalType: true, colourDesc: true, primaries: 1, transfer: 1, matrix: 1, chromaLoc: -1})
	extras := &mp4StructuredFacts{}
	parseMP4HEVCSampleEntry(buildHEVCSampleEntry(buildHVCCRecord(sps, nil)), 320, 240, extras)

	if extras.Get("colour_description_present") != "Yes" || extras.Get("colour_description_present_Source") != "Stream" {
		t.Errorf("colour_description_present=%q Source=%q, want Yes/Stream",
			extras.Get("colour_description_present"), extras.Get("colour_description_present_Source"))
	}
	for _, k := range []string{"colour_primaries", "transfer_characteristics", "matrix_coefficients"} {
		if extras.Get(fieldName(k)) == "" {
			t.Errorf("%s is empty, want signaled value", k)
		}
		if extras.Get(fieldName(k+"_Source")) != "Stream" {
			t.Errorf("%s_Source = %q, want Stream (not Container / Stream)", k, extras.Get(fieldName(k+"_Source")))
		}
	}
}

// TestParseMP4HEVCStoredHeight1088 covers the documented 1080->1088 conformance-window
// crop: coded luma height 1088 surfaces as Stored_Height while display is 1080.
func TestParseMP4HEVCStoredHeight1088(t *testing.T) {
	// 4:2:0 cropUnitY = 2; confBottom = 4 -> crop 8 lines: coded 1088 -> display 1080.
	sps := buildHEVCSPS(hevcSPSOpts{picWidth: 1920, picHeight: 1088, confWin: true, confBottom: 4, chromaLoc: -1})
	if got := parseHEVCSPS(sps); got.CodedHeight != 1088 || got.Height != 1080 {
		t.Fatalf("SPS coded/display height = %d/%d, want 1088/1080", got.CodedHeight, got.Height)
	}
	extras := &mp4StructuredFacts{}
	parseMP4HEVCSampleEntry(buildHEVCSampleEntry(buildHVCCRecord(sps, nil)), 1920, 1080, extras)
	if extras.Get("Stored_Height") != "1088" {
		t.Errorf("Stored_Height = %q, want 1088", extras.Get("Stored_Height"))
	}
	if extras.Get("Stored_Width") != "" {
		t.Errorf("unexpected Stored_Width %q (width not cropped)", extras.Get("Stored_Width"))
	}
}

// TestParseMP4HEVCStoredWidthPositive covers the Stored_Width emission branch (the AVC
// path never emits Stored_Width, so this path is HEVC-only).
func TestParseMP4HEVCStoredWidthPositive(t *testing.T) {
	sps := buildHEVCSPS(hevcSPSOpts{picWidth: 320, picHeight: 240, chromaLoc: -1})
	if got := parseHEVCSPS(sps); got.CodedWidth != 320 {
		t.Fatalf("coded width = %d, want 320", got.CodedWidth)
	}
	extras := &mp4StructuredFacts{}
	// Displayed width 318 differs from coded 320 -> Stored_Width must surface.
	parseMP4HEVCSampleEntry(buildHEVCSampleEntry(buildHVCCRecord(sps, nil)), 318, 240, extras)
	if extras.Get("Stored_Width") != "320" {
		t.Errorf("Stored_Width = %q, want 320", extras.Get("Stored_Width"))
	}
}

// TestParseMP4HEVCMalformed exercises missing/truncated hvcC and a bogus SEI length:
// the parser must not panic and must not invent format data.
func TestParseMP4HEVCMalformed(t *testing.T) {
	t.Run("no hvcC box", func(t *testing.T) {
		entry := make([]byte, mp4VisualSampleEntryHeaderSize+16) // header + junk, no hvcC
		fields, _ := parseMP4HEVCSampleEntry(entry, 320, 240, &mp4StructuredFacts{})
		if fields != nil {
			t.Errorf("expected nil fields for missing hvcC, got %v", fields)
		}
	})

	t.Run("truncated hvcC does not panic or invent format", func(t *testing.T) {
		short := buildHEVCSampleEntry([]byte{0x01, 0x01, 0x60}) // <23-byte hvcC
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked on truncated hvcC: %v", r)
			}
		}()
		fields, _ := parseMP4HEVCSampleEntry(short, 320, 240, &mp4StructuredFacts{})
		if v := hevcField(fields, "Format profile"); v != "" {
			t.Errorf("invented Format profile %q from truncated hvcC", v)
		}
		if v := hevcField(fields, "Bit depth"); v != "" {
			t.Errorf("invented Bit depth %q from truncated hvcC", v)
		}
	})

	t.Run("oversized SEI length does not panic", func(t *testing.T) {
		// hvcC with an SEI array whose NAL length prefix exceeds the buffer.
		rec := buildHVCCRecord(hevcSPS8bit, nil)
		rec = append(rec, 0x27, 0x00, 0x01, 0xFF, 0xFF, 0x4E, 0x01) // type39, numNalus=1, len=0xFFFF, 2 bytes
		rec[22]++                                                   // bump array count
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked on oversized SEI length: %v", r)
			}
		}()
		parseMP4HEVCSampleEntry(buildHEVCSampleEntry(rec), 320, 240, &mp4StructuredFacts{})
	})
}
