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

// TestAnalyzeAVINormalizesCopiedGeneralFrameRate covers the header ratio and its General copy.
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
		if got := findField(stream.Fields, "Frame rate"); got != "25.000 FPS" {
			t.Errorf("%s text rate = %q, want 25.000 FPS", stream.Kind, got)
		}
		if got := stream.JSON["FrameRate"]; got != "25.000" {
			t.Errorf("%s JSON rate = %q, want 25.000", stream.Kind, got)
		}
	}
	if video := report.Streams[0]; video.JSON["FrameRate_Num"] != "25" || video.JSON["FrameRate_Den"] != "1" {
		t.Errorf("normalized video ratio = %s/%s, want 25/1", video.JSON["FrameRate_Num"], video.JSON["FrameRate_Den"])
	}
}

// TestAnalyzeMPEGTSNormalizesCopiedGeneralFrameRate covers a rate sourced from AVC timing.
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

// TestAnalyzeTheoraNormalizesCombinedCanonicalRate keeps Ogg's numeric and text projections distinct.
func TestAnalyzeTheoraNormalizesCombinedCanonicalRate(t *testing.T) {
	packet := make([]byte, 42)
	packet[0] = 0x80
	copy(packet[1:7], "theora")
	packet[16], packet[19] = 64, 64
	binary.BigEndian.PutUint32(packet[22:26], 25010)
	binary.BigEndian.PutUint32(packet[26:30], 1000)
	data := buildOggPageForTest(1, 0x02, 0, packet)
	data = append(data, buildOggPageForTest(1, 0x04, 16, []byte{0})...)
	path := filepath.Join(t.TempDir(), "normalized-theora.ogv")
	if err := os.WriteFile(path, data, 0o600); err != nil {
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
		if got := stream.JSON["FrameRate"]; got != "25.000" {
			t.Errorf("%s JSON rate = %q, want numeric 25.000", stream.Kind, got)
		}
		if got := findField(stream.Fields, "Frame rate"); got != "25.000 FPS" {
			t.Errorf("%s display = %q, want 25.000 FPS", stream.Kind, got)
		}
	}
	if video := report.Streams[0]; video.JSON["FrameRate_Num"] != "25" || video.JSON["FrameRate_Den"] != "1" {
		t.Errorf("Theora ratio = %s/%s, want 25/1", video.JSON["FrameRate_Num"], video.JSON["FrameRate_Den"])
	}
	if output := RenderXML([]Report{report}); strings.Count(output, "<FrameRate>25.000</FrameRate>") != 2 {
		t.Errorf("XML did not retain both numeric rates: %s", output)
	}
}

// TestAnalyzeMatroskaStatisticsFrameRateNormalization preserves measured timing while normalizing its report.
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
	tag = append(tag, buildMatroskaSimpleTag("_STATISTICS_TAGS", "DURATION NUMBER_OF_FRAMES BPS")...)
	tag = append(tag, buildMatroskaSimpleTag("DURATION", "00:18:41.682000000")...)
	tag = append(tag, buildMatroskaSimpleTag("NUMBER_OF_FRAMES", "28043")...)
	tag = append(tag, buildMatroskaSimpleTag("BPS", "2928960")...)
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
	if video := report.Streams[0]; video.JSON["FrameRate_Num"] != "25" || video.JSON["FrameRate_Den"] != "1" {
		t.Errorf("statistics ratio = %s/%s, want 25/1", video.JSON["FrameRate_Num"], video.JSON["FrameRate_Den"])
	}
	if got := findField(report.Streams[0].Fields, "Bits/(Pixel*Frame)"); got != "0.056" {
		t.Errorf("density = %q, want CLI float32 result 0.056", got)
	}
	for _, output := range []string{RenderText([]Report{report}), RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})} {
		if !strings.Contains(output, "0.056") || strings.Contains(output, "0.057") {
			t.Errorf("text density did not retain CLI precision: %s", output)
		}
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

// TestRoundVideoFrameRateStandardCadencesAndBoundaries pins the CLI's ordered windows and minimum correction.
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

// TestNormalizeVideoFrameRatesPreservesProjectionAndTimingEvidence checks visibility and protected timing evidence.
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
	if got := findField(streams[0].Fields, "Frame rate"); got != "25.000 FPS" {
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

// TestNormalizeVideoFrameRatesUsesFirstVideo checks that later streams never supply General's rate.
func TestNormalizeVideoFrameRatesUsesFirstVideo(t *testing.T) {
	for _, test := range []struct{ name, first, before, want string }{
		{"copied later rate", "10.005", "25.010", "10.000"},
		{"different General rate", "10.005", "30.000", "10.000"},
		{"missing General rate", "10.005", "", "10.000"},
		{"missing first rate", "", "25.010", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			generalBuilder := newCanonicalStreamBuilder(StreamGeneral)
			generalBuilder.Fill("FrameRate", test.before, "Frame rate", test.before+" FPS")
			general := generalBuilder.Snapshot(canonicalStreamPolicy{})
			hiddenVideo := newCanonicalStreamBuilder(StreamVideo)
			hiddenVideo.DirectStructured("FrameRate", test.first)
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
		})
	}
}

// TestAnalyzeMatroskaUsesFirstVideo keeps later track timing out of General when the first rate is absent.
func TestAnalyzeMatroskaUsesFirstVideo(t *testing.T) {
	for _, firstDuration := range []uint64{0, 100_000_000} {
		var tracks []byte
		for i, duration := range []uint64{firstDuration, 40_000_000} {
			entry := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(uint64(i+1)))
			entry = append(entry, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1))...)
			entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEG4/ISO/AVC"))...)
			if duration != 0 {
				entry = append(entry, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(duration))...)
			}
			entry = append(entry, buildMatroskaVideoSettings(64, 64)...)
			tracks = append(tracks, buildMatroskaElement(mkvIDTrackEntry, entry)...)
		}
		segment := append(buildMatroskaInfo(), buildMatroskaElement(mkvIDTracks, tracks)...)
		file := append(buildMatroskaElement(0x1A45DFA3, nil), buildMatroskaElement(mkvIDSegment, segment)...)
		path := filepath.Join(t.TempDir(), "first-video.mkv")
		if err := os.WriteFile(path, file, 0o600); err != nil {
			t.Fatal(err)
		}
		report, err := AnalyzeFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := ""
		if firstDuration != 0 {
			want = "10.000"
		}
		if got := report.General.JSON["FrameRate"]; got != want {
			t.Errorf("General rate with first duration %d = %q, want %q", firstDuration, got, want)
		}
		if len(report.Streams) != 2 || report.Streams[1].JSON["FrameRate"] != "25.000" {
			t.Fatalf("later video rate was lost: %+v", report.Streams)
		}
		wantText := ""
		if want != "" {
			wantText = want + " FPS"
		}
		if got := findField(report.General.Fields, "Frame rate"); got != wantText {
			t.Errorf("General text with first duration %d = %q, want %q", firstDuration, got, wantText)
		}
	}
}

// TestNormalizeVideoFrameRatesCleansRatios checks numeric-fill cleanup without replacing unchanged precise ratios.
func TestNormalizeVideoFrameRatesCleansRatios(t *testing.T) {
	for _, test := range []struct{ rate, num, den, wantRate, wantNum, wantDen, display string }{
		{"23.980", "24000", "1001", "23.976", "23976", "1000", "23.976 (23976/1000) FPS"},
		{"29.980", "29980", "1000", "29.970", "29970", "1000", "29.970 (29970/1000) FPS"},
		{"25.010", "25010", "1000", "25.000", "25", "1", "25.000 FPS"},
		{"23.976", "24000", "1001", "23.976", "24000", "1001", "23.976 (24000/1001) FPS"},
	} {
		t.Run(test.rate, func(t *testing.T) {
			builder := newCanonicalStreamBuilder(StreamVideo)
			builder.Fill("FrameRate", test.rate, "Frame rate", test.rate+" ("+test.num+"/"+test.den+") FPS")
			builder.DirectStructured("FrameRate_Num", test.num)
			builder.DirectStructured("FrameRate_Den", test.den)
			streams := []Stream{builder.Snapshot(canonicalStreamPolicy{})}
			normalizeVideoFrameRates(nil, streams)
			for key, want := range map[string]string{"FrameRate": test.wantRate, "FrameRate_Num": test.wantNum, "FrameRate_Den": test.wantDen} {
				if got := streams[0].JSON[key]; got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if got := findField(streams[0].Fields, "Frame rate"); got != test.display {
				t.Errorf("display = %q, want %q", got, test.display)
			}
		})
	}
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Fill("FrameRate", "23.976", "Frame rate", "23.976 (24000/1001) FPS")
	builder.DirectStructured("FrameRate_Num", "24000")
	builder.DirectStructured("FrameRate_Den", "1001")
	builder.Fill("FrameRate_Nominal", "50.010", "Nominal frame rate", "50.010 FPS")
	builder.Fill("FrameRate_Original", "29.980", "Original frame rate", "29.980 FPS")
	streams := []Stream{builder.Snapshot(canonicalStreamPolicy{})}
	normalizeVideoFrameRates(nil, streams)
	if got := streams[0].JSON; got["FrameRate_Num"] != "24000" || got["FrameRate_Den"] != "1001" || got["FrameRate_Nominal"] != "50.000" || got["FrameRate_Original"] != "29.970" {
		t.Errorf("rounding secondary rates changed the main ratio: %+v", got)
	}
}

// TestNormalizeVideoFrameRatesRemovesDuplicates checks nominal tolerance and equality after rounding.
func TestNormalizeVideoFrameRatesRemovesDuplicates(t *testing.T) {
	for _, test := range []struct{ rate, nominal, original, want string }{
		{"25.010", "25.000", "24.999", "25.000"},
		{"27.500", "27.510", "27.510", "27.510"},
	} {
		builder := newCanonicalStreamBuilder(StreamVideo)
		builder.Fill("FrameRate", test.rate, "Frame rate", test.rate+" FPS")
		builder.Fill("FrameRate_Nominal", test.nominal, "Nominal frame rate", test.nominal+" FPS")
		builder.Fill("FrameRate_Original", test.original, "Original frame rate", test.original+" FPS")
		builder.DirectStructured("FrameRate_Num", "27500")
		builder.DirectStructured("FrameRate_Den", "1000")
		streams := []Stream{builder.Snapshot(canonicalStreamPolicy{})}
		normalizeVideoFrameRates(nil, streams)
		if got := streams[0].JSON["FrameRate"]; got != test.want {
			t.Errorf("rate = %q, want %q", got, test.want)
		}
		for _, key := range []string{"FrameRate_Nominal", "FrameRate_Original"} {
			if got := streams[0].JSON[key]; got != "" {
				t.Errorf("duplicate %s = %q", key, got)
			}
		}
		for _, label := range []string{"Nominal frame rate", "Original frame rate"} {
			if got := findField(streams[0].Fields, label); got != "" {
				t.Errorf("duplicate display %s = %q", label, got)
			}
		}
		if test.want == "27.510" && (streams[0].JSON["FrameRate_Num"] != "" || streams[0].JSON["FrameRate_Den"] != "") {
			t.Error("nonstandard replacement retained a stale ratio")
		}
	}
}

// TestNormalizeVideoFrameRatesRecomputesPixelDensity covers actual and fallback nominal bit rates.
func TestNormalizeVideoFrameRatesRecomputesPixelDensity(t *testing.T) {
	for _, bitRate := range []string{"1264", "0", "", "text-only"} {
		builder := newCanonicalStreamBuilder(StreamVideo)
		builder.Fill("FrameRate", "25.025", "Frame rate", "25.025 FPS")
		builder.DirectStructured("Width", "10")
		builder.DirectStructured("Height", "10")
		builder.DirectStructured("BitRate", bitRate)
		builder.DirectStructured("BitRate_Nominal", "1264")
		if bitRate == "text-only" {
			builder.Text("Bits/(Pixel*Frame)", "0.505")
		} else {
			builder.Fill("Bits-(Pixel*Frame)", "0.505", "Bits/(Pixel*Frame)", "0.505")
		}
		streams := []Stream{builder.Snapshot(canonicalStreamPolicy{})}
		normalizeVideoFrameRates(nil, streams)
		want := "0.506"
		if bitRate == "text-only" {
			want = ""
		}
		if got := streams[0].JSON["Bits-(Pixel*Frame)"]; got != want {
			t.Errorf("density for bit rate %q = %q, want %q", bitRate, got, want)
		}
		if got := findField(streams[0].Fields, "Bits/(Pixel*Frame)"); got != "0.506" {
			t.Errorf("density display for bit rate %q = %q", bitRate, got)
		}
	}
}
