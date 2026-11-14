package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"html"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMP4LanguageDecode(t *testing.T) {
	for _, tc := range []struct {
		code uint16
		want string
	}{
		{0, "en"}, {1, "fr"}, {86, ""}, {94, "eo"}, {95, ""}, {127, ""}, {128, "cy"}, {138, "jv"},
		{0x7fff, ""}, {0xffff, ""}, {0x8001, ""}, {0x0400, ""}, {0x15c7, "eng"},
	} {
		if got := decodeMP4Language(tc.code); got != tc.want {
			t.Errorf("code %d: %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestAnalyzeMP4LanguageIngress(t *testing.T) {
	for _, tc := range []struct {
		name, brand   string
		version       byte
		code          uint16
		before        bool
		elng          []byte
		want, display string
	}{
		{"legacy ISO", "mp42", 0, 0, false, nil, "en", "English"},
		{"legacy QuickTime", "qt  ", 1, 1, false, nil, "fr", "French"},
		{"Welsh no ftyp", "", 0, 128, false, nil, "cy", "Welsh"},
		{"extended before", "isom", 0, 0, true, []byte("\x00\x00\x00\x00en-us\x00"), "en-us", "English (United States)"},
		{"extended after", "isom", 1, 1, false, []byte("\x00\x00\x00\x00zh-Hant-HK\x00"), "zh-Hant-HK", "Chinese-Hant-HK"},
		{"unsupported", "isom", 0, 1, false, []byte("\x01\x00\x00\x00en\x00"), "fr", "French"},
		{"truncated", "isom", 0, 1, false, []byte{0, 0, 0}, "fr", "French"},
		{"invalid UTF8", "isom", 0, 1, false, []byte{0, 0, 0, 0, 255, 0}, "fr", "French"},
		{"empty", "isom", 0, 1, true, []byte{0, 0, 0, 0, 0}, "fr", "French"},
		{"sentinel", "isom", 0, 0x7fff, false, nil, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var media, track, movie, file bytes.Buffer
			h := make([]byte, 24)
			copy(h[8:], "vide")
			writeMP4Box(&media, "hdlr", h)
			mdhd := make([]byte, 24)
			mdhd[0] = tc.version
			if tc.version == 1 {
				mdhd = make([]byte, 36)
				mdhd[0] = 1
				binary.BigEndian.PutUint32(mdhd[20:], 1000)
				binary.BigEndian.PutUint64(mdhd[24:], 1000)
				binary.BigEndian.PutUint16(mdhd[32:], tc.code)
			} else {
				binary.BigEndian.PutUint32(mdhd[12:], 1000)
				binary.BigEndian.PutUint32(mdhd[16:], 1000)
				binary.BigEndian.PutUint16(mdhd[20:], tc.code)
			}
			if tc.before {
				writeMP4Box(&media, "elng", tc.elng)
			}
			writeMP4Box(&media, "mdhd", mdhd)
			if !tc.before && tc.elng != nil {
				writeMP4Box(&media, "elng", tc.elng)
			}
			writeMP4Box(&track, "mdia", media.Bytes())
			writeMP4Box(&movie, "trak", track.Bytes())
			if tc.brand != "" {
				writeMP4Box(&file, "ftyp", []byte(tc.brand))
			} else {
				// Exercise a valid leading atom and an extended-size movie header.
				writeMP4Box(&file, "free", nil)
				writeMP4Box(&file, "mdat", []byte{0, 0, 0, 0})
				_ = binary.Write(&file, binary.BigEndian, uint32(1))
				file.WriteString("moov")
				_ = binary.Write(&file, binary.BigEndian, uint64(movie.Len()+16))
				file.Write(movie.Bytes())
			}
			if tc.brand != "" {
				writeMP4Box(&file, "moov", movie.Bytes())
			}
			path := filepath.Join(t.TempDir(), "language.mp4")
			if err := os.WriteFile(path, file.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := AnalyzeFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Streams) != 1 || report.Streams[0].Kind != StreamVideo {
				t.Fatalf("streams: %v", report.Streams)
			}
			rawWant := map[string]string{"en": "en", "fr": "fr", "cy": "cy", "en-us": "en (US)", "zh-Hant-HK": "zh-Hant-HK"}[tc.want]
			assertLanguageProjection(t, report, tc.want, tc.display, rawWant)
			// A Ref edit rebuilds the store from public projections. It must not
			// replace a canonical code with the friendly row or recase its suffix.
			report.Ref = "edited.mp4"
			assertLanguageProjection(t, report, tc.want, tc.display, rawWant)
			publicStream := func(s Stream) Stream {
				return Stream{Kind: s.Kind, Fields: append([]Field(nil), s.Fields...), JSON: maps.Clone(s.JSON), JSONRaw: maps.Clone(s.JSONRaw)}
			}
			rebuilt := Report{Ref: "reconstructed.mp4", General: publicStream(report.General), Streams: []Stream{publicStream(report.Streams[0])}}
			assertLanguageProjection(t, rebuilt, tc.want, tc.display, rawWant)
			if tc.name == "extended before" {
				edited := report
				edited.Streams = []Stream{publicStream(report.Streams[0])}
				edited.Streams[0].JSON["Language"] = "fr"
				assertLanguageProjection(t, edited, "fr", "French", "fr")
				edited.Streams[0] = publicStream(report.Streams[0])
				delete(edited.Streams[0].JSON, "Language")
				if strings.Contains(RenderJSON([]Report{edited}), `"Language":`) || strings.Contains(RenderXML([]Report{edited}), "<Language>") {
					t.Fatal("deleted structured language was restored")
				}
				// Fields are an independent public text projection. Deleting only
				// its structured override must not delete the caller's friendly row.
				if !strings.Contains(RenderText([]Report{edited}), padRight("Language", textLabelWidth)+": English (United States)") {
					t.Fatal("structured deletion erased independent text")
				}
				edited.Streams[0].Fields = removeField(edited.Streams[0].Fields, "Language")
				assertLanguageProjection(t, edited, "", "", "")
				edited.Streams[0] = publicStream(report.Streams[0])
				for i := range edited.Streams[0].Fields {
					if edited.Streams[0].Fields[i].Name == "Language" {
						edited.Streams[0].Fields[i].Value = "French"
					}
				}
				assertLanguageProjection(t, edited, "fr", "French", "fr")
				edited.Streams[0] = publicStream(report.Streams[0])
				edited.Streams[0].Fields = removeField(edited.Streams[0].Fields, "Language")
				assertLanguageProjection(t, edited, "", "", "")
			}
		})
	}
}

func TestAnalyzeMP4ExtendedLanguageBounds(t *testing.T) {
	full := func(value string) []byte { return append([]byte{0, 0, 0, 0}, []byte(value)...) }
	exact := strings.Repeat("x", int(embeddedAssetMaxNameBytes))
	for _, tc := range []struct {
		name               string
		values             [][]byte
		code, display, raw string
	}{
		{"last valid wins", [][]byte{full("en-us\x00"), full("scc-Latn-rs\x00")}, "sr-Latn-rs", "Serbian-Latn-rs", "sr-Latn-rs"},
		{"invalid does not erase", [][]byte{full("en-us\x00"), {1, 0, 0, 0, 'd', 'e', 0}, full("\x00"), full("fr\x00de")}, "en-us", "English (United States)", "en (US)"},
		{"unterminated UTF8", [][]byte{full("en-us")}, "en-us", "English (United States)", "en (US)"},
		{"exact limit", [][]byte{full(exact)}, exact, exact, exact},
		{"over limit falls back", [][]byte{full(exact + "x")}, "fr", "French", "fr"},
		{"undefined overrides mdhd", [][]byte{full("und\x00")}, "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var media, track, movie, file bytes.Buffer
			h := make([]byte, 24)
			copy(h[8:], "vide")
			writeMP4Box(&media, "hdlr", h)
			mdhd := make([]byte, 24)
			binary.BigEndian.PutUint32(mdhd[12:], 1000)
			binary.BigEndian.PutUint32(mdhd[16:], 1000)
			binary.BigEndian.PutUint16(mdhd[20:], 1)
			writeMP4Box(&media, "mdhd", mdhd)
			for _, value := range tc.values {
				writeMP4Box(&media, "elng", value)
			}
			writeMP4Box(&track, "mdia", media.Bytes())
			writeMP4Box(&movie, "trak", track.Bytes())
			writeMP4Box(&file, "ftyp", []byte("isom"))
			writeMP4Box(&file, "moov", movie.Bytes())
			report := analyzeLanguageFixture(t, "bounds.mp4", file.Bytes())
			if len(report.Streams) != 1 {
				t.Fatalf("stream count %d", len(report.Streams))
			}
			assertLanguageProjection(t, report, tc.code, tc.display, tc.raw)
		})
	}
	budget := &embeddedAssetBudget{stringBytes: embeddedAssetMaxStringBytes - 2}
	if got := parseMP4ExtendedLanguage(full("fr"), budget); got != "fr" {
		t.Fatalf("exact aggregate remainder: %q", got)
	}
	if got := parseMP4ExtendedLanguage(full("en"), budget); got != "" {
		t.Fatalf("exhausted aggregate accepted %q", got)
	}
}

func assertLanguageProjection(t *testing.T, report Report, code, display, rawWant string) {
	t.Helper()
	xml := RenderXML([]Report{report})
	json := RenderJSON([]Report{report})
	friendly := RenderText([]Report{report})
	raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
	htmlOutput := RenderHTML([]Report{report})
	if RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"}) != raw || RenderHTML([]Report{report}) != htmlOutput || RenderJSON([]Report{report}) != json || RenderText([]Report{report}) != friendly || RenderXML([]Report{report}) != xml {
		t.Fatal("render order changed language report")
	}
	if code == "" {
		if strings.Contains(json, `"Language":`) || strings.Contains(xml, "<Language>") || strings.Contains(friendly, padRight("Language", textLabelWidth)+":") || strings.Contains(raw, padRawTextLabel("Language/String", 33)+":") || strings.Contains(raw, padRawTextLabel("Language", 33)+":") || strings.Contains(htmlOutput, "<td>Language</td>") {
			t.Fatalf("unexpected language: %s\n%s\n%s\n%s\n%s", json, xml, friendly, raw, htmlOutput)
		}
		return
	}
	if strings.Count(json, `"Language":"`+code+`"`) != 1 || strings.Count(json, `"Language":`) != 1 || strings.Count(xml, "<Language>"+code+"</Language>") != 1 || strings.Count(xml, "<Language>") != 1 {
		t.Fatalf("structured language: %s\n%s", json, xml)
	}
	if strings.Count(friendly, fmt.Sprintf("%-40s : %s", "Language", display)) != 1 || strings.Count(htmlOutput, "<tr><td>Language</td><td>"+html.EscapeString(display)+"</td></tr>") != 1 {
		t.Fatalf("friendly language: %s\n%s", friendly, htmlOutput)
	}
	if strings.Count(strings.ReplaceAll(raw, "\r\n", "\n"), padRawTextLabel("Language/String", 33)+": "+rawWant+"\n") != 1 {
		t.Fatalf("raw language: %s", raw)
	}
}
