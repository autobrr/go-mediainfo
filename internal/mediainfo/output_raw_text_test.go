package mediainfo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRenderTextWithOptionsUsesRawProjection(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamGeneral)
	builder.Fill("Format", "Matroska", "Format", "Matroska")
	builder.Fill("Duration", "3661000", "Duration", "1 h 1 min 1 s")
	builder.Fill("OverallBitRate", "25700000", "Overall bit rate", "25.7 Mb/s")
	report := Report{Ref: "raw.mkv", General: builder.Snapshot(canonicalStreamPolicy{})}
	attachCanonicalStore(&report)

	raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
	for _, row := range []string{
		"Format/String                    : Matroska",
		"Duration/String                  : 1h 1mn",
		"OverallBitRate/String            : 25.7 Mbps",
	} {
		if !strings.Contains(raw, row) {
			t.Fatalf("raw output missing %q:\n%s", row, raw)
		}
	}
	if strings.Contains(raw, "\n") && strings.Contains(strings.ReplaceAll(raw, "\r\n", ""), "\n") {
		t.Fatalf("raw output contains bare LF line endings: %q", raw)
	}
	if !strings.HasSuffix(raw, "\r\n\r\n\r\n") {
		t.Fatalf("raw output terminal line endings = %q", raw[len(raw)-min(len(raw), 12):])
	}
	if friendly := RenderText([]Report{report}); !strings.Contains(friendly, "Format                                   : Matroska") {
		t.Fatalf("friendly output changed:\n%s", friendly)
	}
}

func TestRawTextExtraProjectionVisibilityAndFormatting(t *testing.T) {
	for _, test := range []struct {
		key, value, want string
		visible          bool
	}{
		{"dialnorm", "-31", "-31 dB", true},
		{"cmixlev", "-3.0", "-3.0 dB", true},
		{"dsurmod", "0", "0", false},
		{"dsurmod", "1", "Not Dolby Surround encoded", true},
		{"dsurmod", "2", "Dolby Surround encoded", true},
		{"dialnorm_Count", "100", "100", false},
		{"intra_dc_precision", "10", "10", false},
	} {
		if got := rawTextExtraVisible(test.key, test.value); got != test.visible {
			t.Fatalf("rawTextExtraVisible(%q, %q) = %v, want %v", test.key, test.value, got, test.visible)
		}
		if got := rawTextExtraValue(test.key, test.value); got != test.want {
			t.Fatalf("rawTextExtraValue(%q, %q) = %q, want %q", test.key, test.value, got, test.want)
		}
	}
}

func TestRawTextCanonicalValueFormatting(t *testing.T) {
	if got := formatRawTextBitRate("4008000"); got != "4008 Kbps" {
		t.Fatalf("formatRawTextBitRate = %q", got)
	}
	if got := formatRawTextBitRate("64000"); got != "64.0 Kbps" {
		t.Fatalf("formatRawTextBitRate = %q", got)
	}
	if got := formatRawTextLanguage("es-419", "Spanish"); got != "Spanish (419)" {
		t.Fatalf("formatRawTextLanguage = %q", got)
	}
	if got := formatRawTextServiceKind("CM / O / C"); got != "Complete Main / Original / Commentary" {
		t.Fatalf("formatRawTextServiceKind = %q", got)
	}
	if got := rawTextCodecIDInfo("S_HDMV/PGS"); got != "Picture based subtitle format used on BDs/HD-DVDs" {
		t.Fatalf("rawTextCodecIDInfo = %q", got)
	}
	if got := formatRawTextDerivedBitRate("109000000"); got != "109 Mbps" {
		t.Fatalf("formatRawTextDerivedBitRate = %q", got)
	}
	if got := formatRawTextPCMBitRate("1411200"); got != "1411.2 Kbps" {
		t.Fatalf("formatRawTextPCMBitRate = %q", got)
	}
	if got := formatRawTextAVIAspectRatio("1.818"); got != "16:9" {
		t.Fatalf("formatRawTextAVIAspectRatio(1.818) = %q", got)
	}
	if got := formatRawTextAVIAspectRatio("1.250"); got != "5:4" {
		t.Fatalf("formatRawTextAVIAspectRatio(1.250) = %q", got)
	}
	if got := formatRawTextEncodedLibrary("DivX503b1025p"); got != "DivX 5.1.1 Beta2 (2003-11)" {
		t.Fatalf("formatRawTextEncodedLibrary(DivX) = %q", got)
	}
	if got := formatRawTextEncodedLibrary("XviD0041"); got != "XviD 1.1.0 (2005-11-22)" {
		t.Fatalf("formatRawTextEncodedLibrary(XviD) = %q", got)
	}
	if got := rawTextValue(StreamVideo, "FrameRate_Original/String", "23.976 fps", map[string]string{"Format": "MPEG-4 Visual", "CodecID": "XVID"}, 0); got != "23.976 (23976/1000) fps" {
		t.Fatalf("original frame rate = %q", got)
	}
}

func TestRawTextAVISettingsAndFormatInfo(t *testing.T) {
	if got := formatRawTextSettings(map[string]string{
		"Format":                 "MPEG-4 Visual",
		"CodecID":                "XVID",
		"Format_Settings_BVOP":   "1",
		"Format_Settings_Matrix": "Custom",
	}); got != "BVOP1 / Custom Matrix" {
		t.Fatalf("custom MPEG-4 Visual settings = %q", got)
	}
	if got := formatRawTextSettings(map[string]string{
		"Format":                 "MPEG-4 Visual",
		"CodecID":                "XVID",
		"Format_Settings_BVOP":   "No",
		"Format_Settings_Matrix": "Custom",
	}); got != "Custom Matrix" {
		t.Fatalf("MPEG-4 Visual settings without BVOP = %q", got)
	}
	if got := rawTextFormatInfo(StreamAudio, map[string]string{"Format": "AC-3"}); got != "Audio Coding 3" {
		t.Fatalf("AC-3 format info = %q", got)
	}
	if got := rawTextFormatInfo(StreamAudio, map[string]string{"Format": "AAC", "Format_AdditionalFeatures": "LC"}); got != "Advanced Audio Codec Low Complexity" {
		t.Fatalf("AAC LC format info = %q", got)
	}
}

func TestRawTextBDAVStructuredFormatting(t *testing.T) {
	tests := []struct {
		name       string
		kind       StreamKind
		structured map[string]string
		wantInfo   string
		wantSet    string
	}{
		{
			name:       "general info",
			kind:       StreamGeneral,
			structured: map[string]string{"Format": "BDAV"},
			wantInfo:   "Blu-ray Video",
		},
		{
			name:       "truehd 16 channel info",
			kind:       StreamAudio,
			structured: map[string]string{"Format": "MLP FBA", "Format_AdditionalFeatures": "AC-3 16-ch"},
			wantInfo:   "Meridian Lossless Packing FBA with 16-channel presentation",
		},
		{
			name:       "big endian signed settings",
			kind:       StreamAudio,
			structured: map[string]string{"Format_Settings_Endianness": "Big", "Format_Settings_Sign": "Signed"},
			wantSet:    "Big / Signed",
		},
		{
			name:       "dolby surround ex settings",
			kind:       StreamAudio,
			structured: map[string]string{"Format": "E-AC-3", "Format_Settings_Mode": "Dolby Surround EX"},
			wantInfo:   "Enhanced AC-3",
			wantSet:    "Dolby Surround EX",
		},
		{
			name:       "dependent ac3",
			kind:       StreamAudio,
			structured: map[string]string{"Format": "AC-3", "Format_AdditionalFeatures": "Dep"},
			wantInfo:   "Enhanced AC-3",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := rawTextFormatInfo(test.kind, test.structured); got != test.wantInfo {
				t.Fatalf("rawTextFormatInfo = %q, want %q", got, test.wantInfo)
			}
			if got := formatRawTextSettings(test.structured); got != test.wantSet {
				t.Fatalf("formatRawTextSettings = %q, want %q", got, test.wantSet)
			}
		})
	}

	if got := rawTextValue(StreamGeneral, "ID/String", "1 (0x1)", map[string]string{"Format": "BDAV"}, 0); got != "0 (0x0)" {
		t.Fatalf("BDAV general ID/String = %q", got)
	}
	if got := rawTextScanStoreMethod(map[string]string{"ScanType": "MBAFF"}); got != "InterleavedFields" {
		t.Fatalf("MBAFF scan store method = %q", got)
	}
	if got := rawTextValue(StreamAudio, "BedChannelCount", "1", map[string]string{"BedChannelCount": "1"}, 0); got != "1 channel" {
		t.Fatalf("BedChannelCount = %q", got)
	}
	if got := rawTextExtraValue("BedChannelCount", "1"); got != "1 channel" {
		t.Fatalf("extra BedChannelCount = %q", got)
	}
	if got := rawTextValue(StreamAudio, "Format/String", "AC-3 Dep", nil, 0); got != "E-AC-3" {
		t.Fatalf("dependent AC-3 Format/String = %q", got)
	}
	if got := rawTextValue(StreamAudio, "Format/String", "AC-3", map[string]string{"Format": "AC-3", "Format_AdditionalFeatures": "Dep"}, 0); got != "E-AC-3" {
		t.Fatalf("structured dependent AC-3 Format/String = %q", got)
	}
	if got := rawTextValue(StreamVideo, "StreamSize/String", "1.00 KiB (13%)", map[string]string{"StreamSize": "4096"}, 8192); got != "4.00 KiB (50%)" {
		t.Fatalf("structured StreamSize/String = %q", got)
	}
}

func TestRawTextBDAVStructuredDerivations(t *testing.T) {
	structured := map[string]string{
		"Format_Commercial_IfAny":         "Dolby Digital Plus",
		"OverallBitRate_Maximum":          "48000000",
		"MasteringDisplay_ColorPrimaries": "Display P3",
		"MasteringDisplay_Luminance":      "min: 0.0050 cd/m2, max: 1000 cd/m2",
		"MaxCLL":                          "1000",
		"MaxFALL":                         "400",
	}
	got := make(map[string]string)
	for _, field := range rawTextStructuredDerivations(StreamVideo, structured, 0) {
		got[field.Label] = field.Value
	}
	for label, want := range map[string]string{
		"Format_Commercial_IfAny":         "Dolby Digital Plus",
		"OverallBitRate_Maximum/String":   "48.0 Mbps",
		"MasteringDisplay_ColorPrimaries": "Display P3",
		"MasteringDisplay_Luminance":      "min: 0.0050 cd/m2, max: 1000 cd/m2",
		"MaxCLL/String":                   "1000 cd/m2",
		"MaxFALL/String":                  "400 cd/m2",
	} {
		if got[label] != want {
			t.Errorf("%s = %q, want %q", label, got[label], want)
		}
	}
}

func TestRawTextSliceCountKeepsNumericExtensionSeparate(t *testing.T) {
	if got := rawTextLabel("Format settings, Slice count"); got != "Format settings, Slice count" {
		t.Fatalf("numeric slice-count label = %q, want retained extension label", got)
	}

	for _, test := range []struct {
		count string
		want  string
	}{
		{count: "1"},
		{count: "4", want: "4 slice per frame"},
	} {
		got := ""
		for _, field := range rawTextStructuredDerivations(StreamVideo, map[string]string{"Format_Settings_SliceCount": test.count}, 0) {
			if field.Label == "Format_Settings_SliceCount/Strin" {
				got = field.Value
			}
		}
		if got != test.want {
			t.Errorf("count %s formatted sibling = %q, want %q", test.count, got, test.want)
		}
	}
}

func TestFormatRawTextHDRAppendsMasteringComponent(t *testing.T) {
	const dolby = "Dolby Vision, Version 1.0, Profile 8.1"
	structured := map[string]string{"HDR_Format": "Dolby Vision / SMPTE ST 2086"}
	want := dolby + " / SMPTE ST 2086, Version HDR10, HDR10 compatible"
	if got := formatRawTextHDR(dolby, structured); got != want {
		t.Fatalf("formatRawTextHDR = %q, want %q", got, want)
	}
}

func TestFormatRawTextHDRDuplicatesContainerAndStreamDolbyVision(t *testing.T) {
	structured := map[string]string{
		"HDR_Format":         "Dolby Vision / Dolby Vision",
		"HDR_Format_Profile": "dvhe.05 / dvhe.05",
	}
	display := "Dolby Vision, Version 1.0, Profile 5, dvhe.05.03, BL+RPU, no metadata compression"
	want := "Dolby Vision, Version 1.0, dvhe.05.03, BL+RPU, no metadata compression / " + display
	if got := formatRawTextHDR(display, structured); got != want {
		t.Fatalf("formatRawTextHDR = %q, want %q", got, want)
	}
}

func TestFormatRawTextDurationUsesTwoLargestUnits(t *testing.T) {
	for _, test := range []struct {
		milliseconds float64
		want         string
	}{
		{3_903_000, "1h 5mn"},
		{565_000, "9mn 25s"},
		{12_416, "12s 416ms"},
	} {
		if got := formatRawTextDuration(test.milliseconds); got != test.want {
			t.Fatalf("formatRawTextDuration(%v) = %q, want %q", test.milliseconds, got, test.want)
		}
	}
}

func TestRawTextFrameRateRatioPolicy(t *testing.T) {
	for _, test := range []struct {
		kind             StreamKind
		format, uid      string
		numerator, denom string
		want             bool
	}{
		{StreamVideo, "AVC", "", "23976", "1000", true},
		{StreamVideo, "AVC", "", "24000", "1001", false},
		{StreamVideo, "AV1", "", "24000", "1001", true},
		{StreamVideo, "AV1", "18229823285062969326", "24000", "1001", false},
		{StreamVideo, "MPEG-4 Visual", "", "24000", "1001", true},
		{StreamText, "ASS", "", "999", "1000", true},
		{StreamVideo, "AVC", "", "24", "1", false},
	} {
		structured := map[string]string{"Format": test.format, "UniqueID": test.uid}
		if got := rawTextFrameRateUsesRatio(test.kind, structured, test.numerator, test.denom); got != test.want {
			t.Fatalf("rawTextFrameRateUsesRatio(%q, %q/%q) = %v, want %v", test.format, test.numerator, test.denom, got, test.want)
		}
	}
}

func TestRawTextConformanceFieldsFlattenNestedAVIError(t *testing.T) {
	message := "first error / second error"
	node := structuredNode{Kind: structuredArray, Array: []structuredNode{{
		Kind: structuredObject,
		Object: []structuredMember{{Key: "B_", Value: structuredNode{
			Kind: structuredArray,
			Array: []structuredNode{{Kind: structuredObject, Object: []structuredMember{{
				Key: "GeneralCompliance", Value: structuredNode{Kind: structuredString, Text: message},
			}}}},
		}}},
	}}}
	fields := rawTextConformanceFields(node)
	if len(fields) != 3 {
		t.Fatalf("rawTextConformanceFields count = %d, want 3", len(fields))
	}
	if fields[0].Value != "2" || fields[1].Value != "Yes / Yes" || fields[2].Value != message {
		t.Fatalf("rawTextConformanceFields = %#v", fields)
	}
	if got := len([]rune(padRawTextLabel(fields[1].Label, 33))); got != 33 {
		t.Fatalf("padded nested label width = %d, want 33", got)
	}
}

func TestFormatRawTextByteUnitsUsesSubtitlePrecision(t *testing.T) {
	if got := formatRawTextByteUnits(StreamText, "17 Bytes (0%)", "2"); got != "17.0 Byte (0%)" {
		t.Fatalf("formatRawTextByteUnits text = %q", got)
	}
	if got := formatRawTextByteUnits(StreamText, "979 Bytes (0%)", "7"); got != "979 Byte (0%)" {
		t.Fatalf("formatRawTextByteUnits text elements = %q", got)
	}
	if got := formatRawTextByteUnits(StreamAudio, "17 Bytes (0%)", "2"); got != "17 Byte (0%)" {
		t.Fatalf("formatRawTextByteUnits audio = %q", got)
	}
}

func BenchmarkRenderTextProjection(b *testing.B) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		b.Fatal("resolve benchmark source path")
	}
	path := os.Getenv("GO_MEDIAINFO_BENCH_FILE")
	if path == "" {
		path = filepath.Join(filepath.Dir(sourceFile), "..", "..", "samples", "sample.mkv")
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		b.Fatal(err)
	}
	reports := []Report{report}
	b.Run("friendly", func(b *testing.B) {
		for range b.N {
			_ = RenderText(reports)
		}
	})
	b.Run("raw", func(b *testing.B) {
		for range b.N {
			_ = RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"})
		}
	})
}
