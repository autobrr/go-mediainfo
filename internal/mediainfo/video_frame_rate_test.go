package mediainfo

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeAVINormalizesCopiedGeneralFrameRate(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("samples", "sample.avi"))
	if err != nil {
		t.Fatal(err)
	}
	offset := bytes.Index(data, []byte("strh")) + 8
	if offset < 8 || offset+56 > len(data) || string(data[offset:offset+4]) != "vids" {
		t.Fatal("fixture has no video stream header")
	}
	binary.LittleEndian.PutUint32(data[offset+20:], 1000)
	binary.LittleEndian.PutUint32(data[offset+24:], 25010)
	path := filepath.Join(t.TempDir(), "copied-rate.avi")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range []Stream{report.General, report.Streams[0]} {
		if got := findField(stream.Fields, "Frame rate"); extractLeadingNumber(got) != "25.000" {
			t.Errorf("%s text rate = %q, want 25.000 FPS", stream.Kind, got)
		}
		if got := stream.JSON["FrameRate"]; got != "25.000" {
			t.Errorf("%s JSON rate = %q, want 25.000", stream.Kind, got)
		}
	}
}

func TestAnalyzeMPEGTSNormalizesCopiedGeneralFrameRate(t *testing.T) {
	w := &hevcBitBuf{}
	w.put(66, 8) // profile_idc
	w.put(0, 8)
	w.put(30, 8) // level_idc
	w.ue(0)      // SPS ID
	w.ue(0)      // log2_max_frame_num_minus4
	w.ue(0)      // pic_order_cnt_type
	w.ue(0)      // log2_max_pic_order_cnt_lsb_minus4
	w.ue(1)      // reference frames
	w.put(0, 1)  // gaps
	w.ue(3)      // 64 pixels wide
	w.ue(3)      // 64 pixels high
	w.put(1, 1)  // frame_mbs_only_flag
	w.put(1, 1)  // direct_8x8_inference_flag
	w.put(0, 1)  // cropping
	w.put(1, 1)  // VUI
	w.put(0, 4)  // aspect ratio, overscan, video signal, chroma location
	w.put(1, 1)  // timing_info_present_flag
	w.put(500, 32)
	w.put(25010, 32) // 25010 / (2 * 500) = 25.010 FPS
	w.put(1, 1)      // fixed_frame_rate_flag
	w.put(0, 4)      // NAL/VCL HRD, pic_struct, bitstream restriction
	w.put(1, 1)      // rbsp_stop_one_bit
	sps := append([]byte{0, 0, 1, 0x67}, emulationEncode(w.bytes())...)
	pmt := buildPMTSection(0x101)
	pmt[13] = 0x1B // AVC
	data := tsPacket(0, true, buildPATSection(0x100))
	data = append(data, tsPacket(0x100, true, pmt)...)
	pes := append([]byte{0, 0, 1, 0xE0, 0, 0, 0x80, 0, 0}, sps...)
	data = append(data, tsPacket(0x101, true, pes)...)
	data = append(data, tsPacket(0x1FFF, false, nil)...)
	data = append(data, tsPacket(0x1FFF, false, nil)...)
	path := filepath.Join(t.TempDir(), "copied-rate.ts")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.General.JSON["FrameRate"]; got != "25.000" {
		t.Errorf("General JSON rate = %q, want 25.000", got)
	}
	if got := findField(report.General.Fields, "Frame rate"); got != "25.000 FPS" {
		t.Errorf("General text rate = %q, want 25.000 FPS", got)
	}
}

func TestAnalyzeMatroskaStatisticsFrameRateNormalization(t *testing.T) {
	// Timing facts from parity sample mkv-local-20261016-01. The statistics
	// quotient is correct, but the CLI normalizes its three-decimal value.
	entry := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	entry = append(entry, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(123))...)
	entry = append(entry, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1))...)
	entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEG4/ISO/AVC"))...)
	entry = append(entry, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(40_000_000))...)
	entry = append(entry, buildMatroskaVideoSettings(1920, 1080)...)
	segment := append(buildMatroskaInfo(), buildMatroskaElement(mkvIDTracks, buildMatroskaElement(mkvIDTrackEntry, entry))...)
	tag := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(123)))
	tag = append(tag, buildMatroskaSimpleTag("_STATISTICS_TAGS", "DURATION NUMBER_OF_FRAMES")...)
	tag = append(tag, buildMatroskaSimpleTag("DURATION", "00:18:41.682000000")...)
	tag = append(tag, buildMatroskaSimpleTag("NUMBER_OF_FRAMES", "28043")...)
	segment = append(segment, buildMatroskaElement(mkvIDTags, buildMatroskaElement(mkvIDTag, tag))...)
	file := append(buildMatroskaElement(0x1A45DFA3, nil), buildMatroskaElement(mkvIDSegment, segment)...)
	path := filepath.Join(t.TempDir(), "statistics-rate.mkv")
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Streams) != 1 {
		t.Fatalf("streams = %d, want one video", len(report.Streams))
	}
	for _, stream := range []Stream{report.General, report.Streams[0]} {
		if got := findField(stream.Fields, "Frame rate"); got != "25.000 FPS" {
			t.Errorf("%s text frame rate = %q, want 25.000 FPS", stream.Kind, got)
		}
		if got := stream.JSON["FrameRate"]; got != "25.000" {
			t.Errorf("%s JSON frame rate = %q, want 25.000", stream.Kind, got)
		}
	}
	if got := report.Streams[0].JSON["Duration"]; got != "1121.682000000" {
		t.Errorf("video duration = %q, want unchanged statistics duration", got)
	}
	if got := report.Streams[0].JSON["FrameCount"]; got != "28043" {
		t.Errorf("video frame count = %q, want 28043", got)
	}
	for name, output := range map[string]string{
		"text": RenderText([]Report{report}),
		"JSON": RenderJSON([]Report{report}),
		"XML":  RenderXML([]Report{report}),
	} {
		if strings.Contains(output, "25.001") || !strings.Contains(output, "25.000") {
			t.Errorf("%s did not retain the normalized rate: %s", name, output)
		}
	}
}

func TestRoundVideoFrameRateStandardCadencesAndBoundaries(t *testing.T) {
	for _, test := range []struct {
		name       string
		lower      float64
		upper      float64
		want       float64
		lowerValue float64
	}{
		{"10", 9.990, 10.010, 10, 9.990},
		{"11.988", 11.984, 11.994, 11.988, 11.984},
		{"12", 11.994, 12.010, 12, 11.988},
		{"14.985", 14.980, 14.990, 14.985, 14.980},
		{"15", 14.990, 15.010, 15, 14.985},
		{"23.976", 23.952, 23.988, 23.976, 23.952},
		{"24", 23.988, 24.024, 24, 23.976},
		{"25", 24.975, 25.025, 25, 24.975},
		{"29.970", 29.940, 29.985, 29.970, 29.940},
		{"30", 29.985, 30.030, 30, 29.970},
		{"47.952", 47.904, 47.976, 47.952, 47.904},
		{"48", 47.976, 48.048, 48, 47.952},
		{"50", 49.950, 50.050, 50, 49.950},
		{"59.940", 59.880, 59.970, 59.940, 59.880},
		{"60", 59.970, 60.060, 60, 59.940},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := roundVideoFrameRate(test.lower); got != test.lowerValue {
				t.Errorf("lower boundary %.6f = %.6f, want %.6f", test.lower, got, test.lowerValue)
			}
			if got := roundVideoFrameRate(test.lower + 0.000001); got != test.want {
				t.Errorf("inside lower boundary = %.6f, want %.6f", got, test.want)
			}
			if got := roundVideoFrameRate(test.upper); got != test.want {
				t.Errorf("upper boundary %.6f = %.6f, want %.6f", test.upper, got, test.want)
			}
		})
	}
	for _, test := range []struct{ rate, want float64 }{
		{25.001, 25}, {24.999, 25}, {29.980, 29.970}, {59.960, 59.940},
		{25.0005, 25.0005}, {25.000999998, 25.000999998}, {25.000999999, 25},
		{24000.0 / 1001, 24000.0 / 1001}, {30000.0 / 1001, 30000.0 / 1001},
		{50.051, 50.051}, {27.5, 27.5}, {120.001, 120.001}, {0, 0}, {-25, -25},
	} {
		if got := roundVideoFrameRate(test.rate); got != test.want {
			t.Errorf("roundVideoFrameRate(%.12f) = %.12f, want %.12f", test.rate, got, test.want)
		}
	}
	if !math.IsNaN(roundVideoFrameRate(math.NaN())) {
		t.Error("NaN was changed")
	}
}

func TestNormalizeVideoFrameRatesPreservesProjectionAndTimingEvidence(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Fill("Format", "AVC", "Format", "AVC")
	builder.Fill("FrameRate", "25.001", "Frame rate", "25.001 (25/1) FPS")
	builder.Fill("FrameRate_Nominal", "29.980", "Nominal frame rate", "29.980 FPS")
	builder.DirectStructured("FrameRate_Original", "50.002")
	builder.DirectStructured("FrameRate_Num", "25")
	builder.DirectStructured("FrameRate_Den", "1")
	streams := []Stream{builder.Snapshot(canonicalStreamPolicy{})}
	originalEntries := append([]fieldEntry(nil), streams[0].canonicalSeed...)
	normalizeVideoFrameRates(nil, streams)
	for key, want := range map[string]string{
		"FrameRate": "25.000", "FrameRate_Nominal": "29.970", "FrameRate_Original": "50.000",
		"FrameRate_Num": "25", "FrameRate_Den": "1",
	} {
		if got := streams[0].JSON[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if got := findField(streams[0].Fields, "Frame rate"); got != "25.000 (25/1) FPS" {
		t.Errorf("frame-rate suffix = %q", got)
	}
	if got := findField(streams[0].Fields, "Nominal frame rate"); got != "29.970 FPS" {
		t.Errorf("nominal text = %q", got)
	}
	if got := findField(streams[0].Fields, "Original frame rate"); got != "" {
		t.Errorf("added a missing text field: %q", got)
	}
	if len(originalEntries) != len(streams[0].canonicalSeed) {
		t.Fatal("normalization changed the field count")
	}
	for i, entry := range originalEntries {
		if got := streams[0].canonicalSeed[i]; got.Name != entry.Name || got.Options != entry.Options {
			t.Errorf("field %d changed identity, order, or visibility", i)
		}
	}
	for _, kind := range []StreamKind{StreamGeneral, StreamAudio, StreamText, StreamMenu} {
		builder := newCanonicalStreamBuilder(kind)
		builder.Fill("FrameRate", "25.001", "Frame rate", "25.001 FPS")
		untouched := []Stream{builder.Snapshot(canonicalStreamPolicy{})}
		before := append([]fieldEntry(nil), untouched[0].canonicalSeed...)
		normalizeVideoFrameRates(nil, untouched)
		if !reflect.DeepEqual(before, untouched[0].canonicalSeed) {
			t.Errorf("normalization changed %s", kind)
		}
	}
	for _, rate := range []string{"25.0005", "23.976", "27.500", "invalid", "NaN", "Inf"} {
		builder := newCanonicalStreamBuilder(StreamVideo)
		builder.Fill("FrameRate", rate, "Frame rate", rate+" FPS")
		untouched := []Stream{builder.Snapshot(canonicalStreamPolicy{})}
		before := append([]fieldEntry(nil), untouched[0].canonicalSeed...)
		normalizeVideoFrameRates(nil, untouched)
		if !reflect.DeepEqual(before, untouched[0].canonicalSeed) {
			t.Errorf("normalization changed preserved rate %q", rate)
		}
	}
	builder.DirectStructured("FrameRate_Original_Num", "25001")
	builder.DirectStructured("FrameRate_Original_Den", "1000")
	guarded := []Stream{builder.Snapshot(canonicalStreamPolicy{})}
	before := append([]fieldEntry(nil), guarded[0].canonicalSeed...)
	normalizeVideoFrameRates(nil, guarded)
	if !reflect.DeepEqual(before, guarded[0].canonicalSeed) {
		t.Error("normalization changed an explicit original rational discrepancy")
	}
}

func TestNormalizeVideoFrameRatesUpdatesOnlyCopiedGeneralValues(t *testing.T) {
	for _, test := range []struct{ name, before, want string }{
		{"copied rate", "25.010", "25.000"},
		{"independent rate", "30.000", "30.000"},
		{"missing rate", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			generalBuilder := newCanonicalStreamBuilder(StreamGeneral)
			generalBuilder.Fill("FrameRate", test.before, "Frame rate", test.before+" FPS")
			general := generalBuilder.Snapshot(canonicalStreamPolicy{})
			generalEntries := len(general.canonicalSeed)
			hiddenVideo := newCanonicalStreamBuilder(StreamVideo)
			hiddenVideo.DirectStructured("FrameRate", "10.005")
			video := newCanonicalStreamBuilder(StreamVideo)
			video.Fill("FrameRate", "25.010", "Frame rate", "25.010 FPS")
			streams := []Stream{hiddenVideo.Snapshot(canonicalStreamPolicy{}), video.Snapshot(canonicalStreamPolicy{})}
			normalizeVideoFrameRates(&general, streams)
			got, _ := canonicalSeedValue(general, "FrameRate")
			if got != test.want {
				t.Errorf("General canonical rate = %q, want %q", got, test.want)
			}
			wantDisplay := ""
			if test.want != "" {
				wantDisplay = test.want + " FPS"
			}
			if got := findField(general.Fields, "Frame rate"); got != wantDisplay {
				t.Errorf("General public display = %q, want %q", got, wantDisplay)
			}
			if got, _ := canonicalSeedTextValue(general, "Frame rate"); got != wantDisplay {
				t.Errorf("General canonical display = %q, want %q", got, wantDisplay)
			}
			if len(general.canonicalSeed) != generalEntries {
				t.Error("normalization changed General field visibility")
			}
		})
	}
}
