package mediainfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMPEGTSJOCFormatInfo(t *testing.T) {
	frame := buildEAC3Frame(64, 2, 0)
	copy(frame[20:], buildEMDFJOCPayload())
	crc := boundaryAC3CRC(frame[2 : len(frame)-2])
	frame[len(frame)-2], frame[len(frame)-1] = byte(crc>>8), byte(crc)
	if !ac3CRCValid(frame, 16) {
		t.Fatal("invalid synthetic E-AC-3 CRC")
	}
	// HDMV registration selects the existing E-AC-3 TS producer path.
	pmt := buildPMTSection(0x101)
	pmt[12] = 6
	pmt = append(append(append([]byte{}, pmt[:13]...), 5, 4, 'H', 'D', 'M', 'V'), pmt[13:]...)
	pmt[19] = 0x84
	n := len(pmt) - 4
	pmt[2], pmt[3] = 0xB0|byte(n>>8), byte(n)
	data := append(tsPacket(0, true, buildPATSection(0x100)), tsPacket(0x100, true, pmt)...)
	payload := append(append([]byte{}, frame...), frame...)
	pes := append([]byte{0, 0, 1, 0xBD, 0, byte(3 + len(payload)), 0x80, 0, 0}, payload...)
	data = append(data, tsPacket(0x101, true, pes)...)
	for range 3 {
		data = append(data, tsPacket(0x1fff, false, nil)...)
	}
	path := filepath.Join(t.TempDir(), "joc.ts")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Streams) != 1 || report.Streams[0].JSON["Format"] != "E-AC-3 JOC" || report.Streams[0].JSON["Format_AdditionalFeatures"] != "JOC" {
		t.Fatal("analyzed TS did not establish the existing JOC identity")
	}
	reports := []Report{report}
	before := RenderJSON(reports)
	for _, out := range []string{RenderText(reports), RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"}), RenderHTML(reports), RenderCSV(reports)} {
		if strings.Count(out, "Enhanced AC-3 with Joint Object Coding") != 1 {
			t.Errorf("analyzed JOC description missing or duplicated: %s", out)
		}
	}
	if RenderJSON(reports) != before {
		t.Fatal("description projection changed structured JOC facts")
	}
}

func TestRawComponentVisibilityPreservesStructuredFacts(t *testing.T) {
	for _, tc := range []struct {
		name, container         string
		kind                    StreamKind
		identity, stored, value string
	}{
		{"bdav-high", "BDAV", StreamVideo, "Format_Tier", "High", "High"},
		{"matroska-high", "Matroska", StreamVideo, "Format_Tier", "High", "High"},
		{"mp4-high", "MPEG-4", StreamVideo, "Format_Tier", "High", "High"},
		{"mp4-main", "MPEG-4", StreamVideo, "Format_Tier", "Main", "Main"},
		{"vc1-level", "Matroska", StreamVideo, "Format_Level", "3", "3"},
		{"mp4-video-12", "MPEG-4", StreamVideo, "Source_Duration_LastFrame", "-12", "-0.012"},
		{"mp4-audio-6", "MPEG-4", StreamAudio, "Source_Duration_LastFrame", "-6", "-0.006"},
		{"mp4-audio-4", "MPEG-4", StreamAudio, "Source_Duration_LastFrame", "-4", "-0.004"},
		{"mp4-video-9", "MPEG-4", StreamVideo, "Source_Duration_LastFrame", "-9", "-0.009"},
		{"mp4-audio-1", "MPEG-4", StreamAudio, "Source_Duration_LastFrame", "-1", "-0.001"},
		{"dvd-video", "DVD Video", StreamVideo, "Format_Level", "3", "3"},
		{"absent", "Matroska", StreamVideo, "Format_Tier", "", ""},
		{"absent-source", "MPEG-4", StreamAudio, "Source_Duration_LastFrame", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newCanonicalStreamBuilder(StreamGeneral)
			g.Fill("Format", tc.container, "Format", tc.container)
			b := newCanonicalStreamBuilder(tc.kind)
			b.Fill("Format", "AVC", "Format", "AVC")
			b.Fill("Format_Profile", "High", "Format profile", "High")
			if tc.value != "" {
				display := tc.value
				if tc.identity == "Source_Duration_LastFrame" {
					display = tc.stored + " ms"
				}
				b.Fill(fieldName(tc.identity), tc.stored, tc.identity, display)
			}
			b.Fill("Duration_LastFrame", "-4", "Duration of the last frame", "-4 ms")
			b.Text("USER_Source_Duration_LastFrame", "literal")
			report := Report{General: g.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{b.Snapshot(canonicalStreamPolicy{})}}
			attachCanonicalStore(&report)
			reports := []Report{report}
			beforeJSON, beforeXML, friendly := RenderJSON(reports), RenderXML(reports), RenderText(reports)
			raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"})
			for _, label := range []string{"Format_Tier", "Format_Level"} {
				if strings.Contains(raw, padRawTextLabel(label, 33)+":") {
					t.Fatalf("standalone raw component %s became visible: %s", label, raw)
				}
			}
			wantSource := tc.identity == "Source_Duration_LastFrame" && tc.value != ""
			if strings.Contains(raw, padRawTextLabel("Source_Duration_LastFrame/String", 33)+":") != wantSource {
				t.Fatalf("source-last-frame presence changed: %s", raw)
			}
			if !strings.Contains(raw, "Format_Profile") || !strings.Contains(raw, "Duration_LastFrame/String") || !strings.Contains(raw, "USER_Source_Duration_LastFrame") {
				t.Fatalf("component hiding removed a sibling or unknown name: %s", raw)
			}
			var decoded struct {
				Media struct{ Track []map[string]any }
			}
			if err := json.Unmarshal([]byte(beforeJSON), &decoded); err != nil {
				t.Fatal(err)
			}
			value, present := decoded.Media.Track[1][tc.identity]
			if present != (tc.value != "") || present && value != tc.value {
				t.Fatalf("structured component = %#v, want %q", value, tc.value)
			}
			if RenderJSON(reports) != beforeJSON || RenderXML(reports) != beforeXML || RenderText(reports) != friendly || RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"}) != raw {
				t.Fatal("raw rendering changed structured facts or repeated output")
			}
		})
	}
	t.Run("general-unknown-name", func(t *testing.T) {
		g := newCanonicalStreamBuilder(StreamGeneral)
		g.Fill("Format_Tier", "literal", "Format_Tier", "literal")
		report := Report{General: g.Snapshot(canonicalStreamPolicy{})}
		attachCanonicalStore(&report)
		if raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"}); !strings.Contains(raw, padRawTextLabel("Format_Tier", 33)+": literal") {
			t.Fatalf("known video component suppressed General metadata: %s", raw)
		}
	})
}

func TestRawCodecIDRetainsStreamIdentity(t *testing.T) {
	for _, kind := range []StreamKind{StreamGeneral, StreamVideo, StreamAudio, StreamText} {
		t.Run(string(kind), func(t *testing.T) {
			b := newCanonicalStreamBuilder(kind)
			b.Fill("CodecID", "27", "Codec ID", "27")
			report := Report{General: Stream{Kind: StreamGeneral}}
			if kind == StreamGeneral {
				report.General = b.Snapshot(canonicalStreamPolicy{})
			} else {
				report.Streams = []Stream{b.Snapshot(canonicalStreamPolicy{})}
			}
			attachCanonicalStore(&report)
			raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
			label := "CodecID"
			if kind == StreamGeneral {
				label += "/String"
			}
			if !strings.Contains(raw, padRawTextLabel(label, 33)+": 27") || strings.Count(raw, "CodecID") != 1 {
				t.Fatalf("raw codec identity: %s", raw)
			}
		})
	}
	for _, mutation := range []string{"JSON", "Fields"} {
		t.Run(mutation+" override", func(t *testing.T) {
			report, err := AnalyzeFile(filepath.Join("samples", "sample.mkv"))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for i := range report.Streams {
				stream := &report.Streams[i]
				if stream.Kind != StreamVideo {
					continue
				}
				found = true
				if mutation == "JSON" {
					stream.JSON["CodecID"] = "V_MPEG2"
				} else {
					stream.Fields = setFieldValue(stream.Fields, "Codec ID", "V_MPEG2")
				}
			}
			if !found {
				t.Fatal("public sample has no Video stream")
			}
			reports := []Report{report}
			before := RenderJSON(reports)
			raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"})
			if !strings.Contains(raw, padRawTextLabel("CodecID", 33)+": V_MPEG2") || strings.Count(raw, "MPEG 1 or 2 Video") != 1 {
				t.Fatalf("explicit codec override retained stale lookup: %s", raw)
			}
			if RenderJSON(reports) != before || RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"}) != raw {
				t.Fatal("codec lookup mutated facts or changed between renders")
			}
		})
	}
}

func TestFriendlyZeroSubtitleSize(t *testing.T) {
	for _, size := range []string{"0", "", "invalid"} {
		t.Run(size, func(t *testing.T) {
			g := newCanonicalStreamBuilder(StreamGeneral)
			g.Fill("FileSize", "1048576", "File size", "1.00 MiB")
			b := newCanonicalStreamBuilder(StreamText)
			b.Fill("Format", "EIA-608", "Format", "EIA-608")
			b.Structured("StreamSize", size)
			report := Report{General: g.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{b.Snapshot(canonicalStreamPolicy{})}}
			attachCanonicalStore(&report)
			before := RenderJSON([]Report{report})
			for _, output := range []string{RenderText([]Report{report}), RenderHTML([]Report{report}), RenderCSV([]Report{report})} {
				if strings.Contains(output, "0.00 Byte (0%)") != (size == "0") {
					t.Fatalf("subtitle size %q: %s", size, output)
				}
			}
			if RenderJSON([]Report{report}) != before {
				t.Fatal("display mutated structured size")
			}
		})
	}
}

func TestFriendlyDVDStatisticsPreserveOccurrences(t *testing.T) {
	g := newCanonicalStreamBuilder(StreamGeneral)
	g.Fill("Format", "DVD Video", "Format", "DVD Video")
	b := newCanonicalStreamBuilder(StreamAudio)
	b.Fill("Format", "AC-3", "Format", "AC-3")
	for range 2 {
		b.Text("Dialog Normalization", "-31 dB")
		b.Text("compr", "-0.28 dB")
	}
	report := Report{General: g.Snapshot(canonicalStreamPolicy{DVDOrder: true}), Streams: []Stream{b.Snapshot(canonicalStreamPolicy{})}}
	attachCanonicalStore(&report)
	for _, output := range []string{RenderText([]Report{report}), RenderHTML([]Report{report}), RenderCSV([]Report{report})} {
		if strings.Count(output, "Dialog Normalization") != 2 || strings.Count(output, "compr") != 2 {
			t.Fatalf("existing statistics occurrences lost: %s", output)
		}
	}
}

func TestRawRelativeDelayUsesCanonicalSeconds(t *testing.T) {
	for _, tc := range []struct {
		container           string
		kind                StreamKind
		delay, display, raw string
	}{
		{"Matroska", StreamAudio, "0.009", "9 ms", "9ms"},
		{"MPEG-TS", StreamText, "-0.009", "-9 ms", "-9ms"},
		{"BDAV", StreamAudio, "0", "0 ms", ""},
		{"DVD Video", StreamAudio, "0.009", "9 ms", "9ms"},
	} {
		t.Run(tc.container+string(tc.kind)+tc.delay, func(t *testing.T) {
			g := newCanonicalStreamBuilder(StreamGeneral)
			g.Fill("Format", tc.container, "Format", tc.container)
			b := newCanonicalStreamBuilder(tc.kind)
			b.Fill("Video_Delay", tc.delay, "Delay relative to video", tc.display)
			report := Report{General: g.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{b.Snapshot(canonicalStreamPolicy{})}}
			attachCanonicalStore(&report)
			before := RenderJSON([]Report{report})
			raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
			if tc.raw == "" {
				if strings.Contains(raw, "Video_Delay/String") {
					t.Fatalf("zero relative delay became visible: %s", raw)
				}
			} else if strings.Count(raw, "Video_Delay/String") != 1 || !strings.Contains(raw, padRawTextLabel("Video_Delay/String", 33)+": "+tc.raw) {
				t.Fatalf("raw relative delay: %s", raw)
			}
			if RenderJSON([]Report{report}) != before {
				t.Fatal("raw delay projection mutated canonical facts")
			}
		})
	}
}

func TestLegacyRelativeDelayIngestion(t *testing.T) {
	for _, tc := range []struct{ display, canonical, want string }{
		{"9 ms", "", "9ms"}, {"-9 ms", "", "-9ms"}, {"0 ms", "", ""},
		{"9 ms", "0.007", "7ms"}, {"9 ms", "0", ""},
	} {
		t.Run(tc.display+tc.canonical, func(t *testing.T) {
			stream := Stream{Kind: StreamAudio, Fields: []Field{{Name: "Delay relative to video", Value: tc.display}}}
			if tc.canonical != "" {
				stream.JSON = map[string]string{"Video_Delay": tc.canonical}
			}
			report := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{stream}}
			raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
			if tc.want == "" {
				if strings.Contains(raw, "Video_Delay/String") {
					t.Fatalf("canonical zero delay became visible: %s", raw)
				}
			} else if strings.Count(raw, "Video_Delay/String") != 1 || !strings.Contains(raw, padRawTextLabel("Video_Delay/String", 33)+": "+tc.want) {
				t.Fatalf("legacy relative delay: %s", raw)
			}
		})
	}
}
