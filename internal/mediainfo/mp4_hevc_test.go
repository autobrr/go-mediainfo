package mediainfo

import "testing"

// Real SPS NAL units (nal_unit_type 33) extracted from x265-encoded MP4 samples.
// They carry no encoder version, so they are safe to assert against across builds.
//
//	x265_8bit:       320x240, Main@L2, 4:2:0, 8-bit, range=Limited, chroma_loc unsignaled.
//	x265_chromaloc2: same, but with chroma_loc_info signaled (top_field = 2).
var (
	hevcSPS8bit = []byte{
		0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03, 0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00,
		0x03, 0x00, 0x3c, 0xa0, 0x0a, 0x08, 0x0f, 0x16, 0x59, 0x59, 0x52, 0x93, 0x0b, 0xc0, 0x5a,
		0x02, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00, 0x00, 0x03, 0x00, 0x30, 0x10,
	}
	hevcSPSChromaLoc2 = []byte{
		0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03, 0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00,
		0x03, 0x00, 0x3c, 0xa0, 0x0a, 0x08, 0x0f, 0x16, 0x59, 0x59, 0x52, 0x93, 0x0b, 0xc0, 0x5a,
		0x5b, 0x08, 0x00, 0x00, 0x03, 0x00, 0x08, 0x00, 0x00, 0x03, 0x00, 0xc0, 0x40,
	}
)

// buildHVCCRecord assembles a minimal HEVCDecoderConfigurationRecord (hvcC payload)
// for Main@L2 / 4:2:0 / 8-bit, optionally embedding an SPS array (type 33) and a
// prefix-SEI array (type 39).
func buildHVCCRecord(sps, sei []byte) []byte {
	cfg := make([]byte, 23)
	cfg[0] = 0x01         // configurationVersion
	cfg[1] = 0x01         // general_profile_space=0, tier_flag=0 (Main), profile_idc=1 (Main)
	cfg[12] = 60          // general_level_idc -> "2"
	cfg[16] = 0xFC | 0x01 // reserved bits + chromaFormat=1 (4:2:0)
	cfg[17] = 0xF8        // reserved bits + bitDepthLumaMinus8=0 (8-bit)
	cfg[21] = 0xFC | 0x03 // reserved bits + lengthSizeMinusOne=3

	var arrays []byte
	var count byte
	add := func(nalType byte, nal []byte) {
		arrays = append(arrays, nalType, 0x00, 0x01, byte(len(nal)>>8), byte(len(nal)))
		arrays = append(arrays, nal...)
		count++
	}
	if len(sps) > 0 {
		add(0x21, sps) // nal_unit_type 33 (SPS)
	}
	if len(sei) > 0 {
		add(0x27, sei) // nal_unit_type 39 (PREFIX_SEI)
	}
	cfg[22] = count
	return append(cfg, arrays...)
}

// buildX265SEINAL (the x265 user_data_unregistered prefix-SEI NAL builder used by
// these tests) is defined in hevc_x265_container_test.go.

// buildHEVCSampleEntry wraps an hvcC payload in a VisualSampleEntry (78-byte header
// followed by the hvcC box) so parseMP4HEVCSampleEntry can locate it.
func buildHEVCSampleEntry(config []byte) []byte {
	box := make([]byte, 8+len(config))
	box[0] = byte((8 + len(config)) >> 24)
	box[1] = byte((8 + len(config)) >> 16)
	box[2] = byte((8 + len(config)) >> 8)
	box[3] = byte(8 + len(config))
	copy(box[4:8], "hvcC")
	copy(box[8:], config)
	return append(make([]byte, mp4VisualSampleEntryHeaderSize), box...)
}

func TestParseMP4HEVCSampleEntry(t *testing.T) {
	config := buildHVCCRecord(hevcSPS8bit, buildX265SEINAL())
	entry := buildHEVCSampleEntry(config)
	extras := &mp4StructuredFacts{}
	fields, spsInfo := parseMP4HEVCSampleEntry(entry, 320, 240, extras)

	want := map[string]string{
		"Format profile":          "Main@L2",
		"Format tier":             "Main",
		"Chroma subsampling":      "4:2:0",
		"Bit depth":               "8 bits",
		"Codec configuration box": "hvcC",
		"Scan type":               "Progressive",
		"Color space":             "YUV",
		"Writing library":         "x265 9.9",
		"Encoding settings":       "wpp / me=0",
	}
	for name, value := range want {
		if got := findField(fields, name); got != value {
			t.Errorf("field %q = %q, want %q", name, got, value)
		}
	}
	// chroma_loc is not signaled in this SPS, so no position is reported.
	if got := findField(fields, "Chroma subsampling position"); got != "" {
		t.Errorf("Chroma subsampling position = %q, want unset", got)
	}
	// MP4 HEVC colour info is bitstream-only -> Source "Stream".
	if extras.Get("colour_range") != "Limited" || extras.Get("colour_range_Source") != "Stream" {
		t.Errorf("colour_range=%q Source=%q, want Limited/Stream", extras.Get("colour_range"), extras.Get("colour_range_Source"))
	}
	// Unspecified primaries/transfer/matrix -> no colour_description_present.
	if extras.Get("colour_description_present") != "" {
		t.Errorf("unexpected colour_description_present for unspecified colour description")
	}
	// 240 lines are coded without padding, so no Stored_* is emitted.
	if extras.Get("Stored_Height") != "" {
		t.Errorf("unexpected Stored_Height for 16-aligned height")
	}
	if !spsInfo.HasColorRange {
		t.Errorf("spsInfo.HasColorRange = false, want true")
	}
}

func TestParseMP4HEVCChromaLocSignaled(t *testing.T) {
	config := buildHVCCRecord(hevcSPSChromaLoc2, nil)
	entry := buildHEVCSampleEntry(config)
	fields, _ := parseMP4HEVCSampleEntry(entry, 320, 240, &mp4StructuredFacts{})
	if got := findField(fields, "Chroma subsampling position"); got != "Type 2" {
		t.Errorf("Chroma subsampling position = %q, want Type 2", got)
	}
}

func TestParseMP4HEVCStoredDimensions(t *testing.T) {
	// Pretend the displayed size is 320x238 while the coded SPS size is 320x240, so the
	// (coded) Stored_Height must surface and Stored_Width must not.
	config := buildHVCCRecord(hevcSPS8bit, nil)
	entry := buildHEVCSampleEntry(config)
	extras := &mp4StructuredFacts{}
	parseMP4HEVCSampleEntry(entry, 320, 238, extras)
	if extras.Get("Stored_Height") != "240" {
		t.Errorf("Stored_Height = %q, want 240", extras.Get("Stored_Height"))
	}
	if extras.Get("Stored_Width") != "" {
		t.Errorf("unexpected Stored_Width %q for matching width", extras.Get("Stored_Width"))
	}
}

func TestParseHEVCConfigTierAlwaysEmitted(t *testing.T) {
	// Main tier (the regression target: it used to be omitted unless High).
	_, mainFields, _, _ := parseHEVCConfig(buildHVCCRecord(nil, nil))
	if got := findField(mainFields, "Format tier"); got != "Main" {
		t.Errorf("Main tier: Format tier = %q, want Main", got)
	}
	if got := findField(mainFields, "Chroma subsampling position"); got != "" {
		t.Errorf("no SPS: Chroma subsampling position = %q, want unset", got)
	}

	highConfig := buildHVCCRecord(nil, nil)
	highConfig[1] = 0x21 // tier_flag=1 (High)
	_, highFields, _, _ := parseHEVCConfig(highConfig)
	if got := findField(highFields, "Format tier"); got != "High" {
		t.Errorf("High tier: Format tier = %q, want High", got)
	}
}
