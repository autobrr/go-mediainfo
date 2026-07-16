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
