package mediainfo

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestRawMissingExtraRowsKeepOrderedSiblingPosition(t *testing.T) {
	for _, tc := range []struct{ first, second, sibling string }{
		{"Statistics_Tags_Issue", "FromStats_BitRate", "FromStats_BitRate"},
		{"Menus", "CodecConfigurationBox", "Codec configuration box"},
	} {
		t.Run(tc.first, func(t *testing.T) {
			builder := newCanonicalStreamBuilder(StreamVideo)
			builder.Fill("Format", "AVC", "Format", "AVC")
			if tc.first == "Statistics_Tags_Issue" {
				builder.Text("Statistics Tags Issue", "friendly")
			}
			builder.Text(tc.sibling, "second")
			builder.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{
				{Key: tc.first, Value: structuredNode{Kind: structuredString, Text: "canonical"}},
				{Key: tc.second, Value: structuredNode{Kind: structuredString, Text: "second"}},
			}})
			report := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{builder.Snapshot(canonicalStreamPolicy{})}}
			attachCanonicalStore(&report)
			before := RenderJSON([]Report{report})
			raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
			first := rawTextLabel(tc.first)
			assertRowOrder(t, raw, first, tc.second)
			if strings.Count(raw, first) != 1 || !strings.Contains(raw, padRawTextLabel(first, 33)+": canonical") {
				t.Fatal("missing-extra projection lost canonical value or duplicate policy")
			}
			if RenderJSON([]Report{report}) != before || RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"}) != raw {
				t.Fatal("ordered-extra projection mutated canonical facts or repeated output")
			}
		})
	}
}

func TestRawExtraRowsFollowCanonicalEncounterSequence(t *testing.T) {
	general := newCanonicalStreamBuilder(StreamGeneral)
	general.Fill("Format", "Matroska", "Format", "Matroska")
	general.Fill("FileSize", "1024", "File size", "1.00 KiB")
	general.Fill("Duration", "1000", "Duration", "1 s 0 ms")
	video := newCanonicalStreamBuilder(StreamVideo)
	video.Structured("Width", "1920")
	video.Structured("Height", "1080")
	audio := newCanonicalStreamBuilder(StreamAudio)
	audio.Fill("Format", "AC-3", "Format", "AC-3")
	audio.Text("A_TAG", "first")
	audio.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{{Key: "Z_CODEC", Value: structuredNode{Kind: structuredString, Text: "later"}}}})
	report := Report{General: general.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{video.Snapshot(canonicalStreamPolicy{}), audio.Snapshot(canonicalStreamPolicy{})}}
	attachCanonicalStore(&report)
	json := RenderJSON([]Report{report})
	raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
	assertRowOrder(t, raw, "A_TAG", "Z_CODEC")
	if RenderJSON([]Report{report}) != json || RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"}) != raw {
		t.Fatal("raw ordering mutated structured facts or repeated output")
	}
}

func TestRawVisibilityUsesSelectedReportVersion(t *testing.T) {
	for _, dvd := range []bool{false, true} {
		t.Run(strconv.FormatBool(dvd), func(t *testing.T) {
			general := newCanonicalStreamBuilder(StreamGeneral)
			general.Fill("Format", "MPEG-PS", "Format", "MPEG-PS")
			general.Fill("Title", "Example", "Title", "Example")
			general.Fill("Title_More", "Details", "More", "Details")
			video := newCanonicalStreamBuilder(StreamVideo)
			video.Fill("Format", "MPEG Video", "Format", "MPEG Video")
			report := Report{General: general.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{video.Snapshot(canonicalStreamPolicy{DVDOrder: dvd})}}
			attachCanonicalStore(&report)
			before := RenderJSON([]Report{report})
			raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
			if got := strings.Contains(normalizeText(raw), padRawTextLabel("Title", 33)+": Example\n"); got != dvd {
				t.Errorf("raw Title present=%v, want %v: %s", got, dvd, raw)
			}
			if got := strings.Contains(normalizeText(raw), padRawTextLabel("Title_More", 33)+": Details\n"); got != dvd {
				t.Errorf("raw Title_More present=%v, want %v: %s", got, dvd, raw)
			}
			if !strings.Contains(before, `"Title":"Example"`) || RenderJSON([]Report{report}) != before || RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"}) != raw {
				t.Fatal("visibility changed structured facts or repeated rendering")
			}
		})
	}
}

func TestPublicDVDVOBMutationRetainsReportVersion(t *testing.T) {
	for _, dvd := range []bool{false, true} {
		general := newCanonicalStreamBuilder(StreamGeneral)
		general.Fill("Format", "MPEG-PS", "Format", "MPEG-PS")
		video := newCanonicalStreamBuilder(StreamVideo)
		video.Fill("Format", "MPEG Video", "Format", "MPEG Video")
		report := Report{General: general.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{video.Snapshot(canonicalStreamPolicy{DVDOrder: dvd})}}
		attachCanonicalStore(&report)
		original := report.General.reportStore
		if canonicalStoreForReport(report) != original {
			t.Fatal("unchanged analysis store was rebuilt")
		}
		wantVersion := reportTables23
		if dvd {
			wantVersion = reportTables26
		}
		report.General.Fields = append(report.General.Fields, Field{Name: "Title", Value: "Caller title"})
		before := captureLegacyReportState(report, true)
		if got := reportVersion(canonicalStoreForReport(report)); got != wantVersion {
			t.Errorf("DVD=%v rebuilt version=%s, want %s", dvd, got, wantVersion)
		}
		json := RenderJSON([]Report{report})
		raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
		if got := strings.Contains(normalizeText(raw), padRawTextLabel("Title", 33)+": Caller title\n"); got != dvd {
			t.Errorf("DVD=%v raw caller Title present=%v: %s", dvd, got, raw)
		}
		for _, want := range []string{`"Format":"MPEG-PS"`, `"Title":"Caller title"`} {
			if !strings.Contains(json, want) {
				t.Errorf("DVD=%v mutation lost %s", dvd, want)
			}
		}
		if RenderJSON([]Report{report}) != json || RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"}) != raw || !reflect.DeepEqual(before, captureLegacyReportState(report, true)) || report.General.reportStore != original {
			t.Fatal("mutation rendering changed the caller or repeated output")
		}
	}
}

func TestBDAVGeneralZeroIDPreservesTransportFallback(t *testing.T) {
	for _, tc := range []struct {
		bdav      bool
		transport uint16
		want      string
	}{
		{true, 0, "0"}, {false, 0, "1"}, {false, 7, "7"},
	} {
		data := buildMPEGTSHEVCX265Sample()
		binary.BigEndian.PutUint16(data[8:10], tc.transport)
		extension := ".ts"
		if tc.bdav {
			var padded []byte
			for offset := 0; offset+188 <= len(data); offset += 188 {
				padded = append(padded, 0, 0, 0, 0)
				padded = append(padded, data[offset:offset+188]...)
			}
			data = padded
			extension = ".m2ts"
		}
		path := filepath.Join(t.TempDir(), "id"+extension)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		report, err := AnalyzeFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := report.General.JSON["ID"]; got != tc.want {
			t.Fatalf("%s General.ID=%s, want %s", extension, got, tc.want)
		}
		id, _ := strconv.ParseUint(tc.want, 10, 64)
		if got := findField(report.General.Fields, "ID"); got != formatID(id) {
			t.Fatalf("public General ID=%q, want %q", got, formatID(id))
		}
		for _, stream := range report.Streams {
			if stream.Kind == StreamVideo && stream.JSON["ID"] != "257" {
				t.Fatalf("video PID changed: %s", stream.JSON["ID"])
			}
		}
	}
}

func TestReportPresentationFromCanonicalFacts(t *testing.T) {
	general := newCanonicalStreamBuilder(StreamGeneral)
	general.Fill("Format", "Matroska", "Format", "Matroska")
	general.Fill("FileSize", "1073739776", "File size", "1024 MiB")
	audio := newCanonicalStreamBuilder(StreamAudio)
	audio.Structured("Format", "FLAC")
	audio.Fill("SamplingRate", "48000", "Sampling rate", "48.0 kHz")
	audio.Structured("FrameRate", "11.719")
	audio.Structured("SamplesPerFrame", "4096")
	audio.Fill("BitDepth", "24", "Bit depth", "24 bits")
	audio.Structured("StreamSize", "60046080")
	audio.Fill("Video_Delay", "9", "Delay relative to video", "9 ms")
	video := newCanonicalStreamBuilder(StreamVideo)
	video.Fill("Format", "MPEG Video", "Format", "MPEG Video")
	video.Text("Format/Info", "MPEG Video")
	video.Fill("Delay", "467", "Delay", "467 ms")
	video.Fill("Delay_Original", "3599000", "Original delay", "59 min 59 s")
	video.Structured("FrameRate", "25.000")
	video.Structured("FrameRate_Minimum", "23.976")
	video.Structured("FrameRate_Maximum", "29.970")
	video.Structured("BitRate_Maximum", "7499968")
	video.Fill("ScanOrder", "TFF", "Scan order", "TFF")
	video.Structured("TimeCode_Source", "Group of pictures header")
	video.Fill("Format_Settings_PictureStructure", "Frame", "Format settings, Picture structure", "Frame")
	video.Fill("OriginalSourceMedium_ID", "189-128", "Original source medium ID", "189-128")
	menu := newCanonicalStreamBuilder(StreamMenu)
	menu.Fill("ServiceType", "Digital television", "Service type", "Digital television")
	menu.Fill("ServiceProvider", "Example", "Service provider", "Example")
	menu.Fill("ServiceName", "Service", "Service name", "Service")
	menu.Text("Encoded_Date", "2020-01-01 00:00:00 UTC")
	menu.Text("BitRate_Maximum", "50000000")
	menu.Text("USER_TAG", "literal")
	report := Report{General: general.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{
		video.Snapshot(canonicalStreamPolicy{SkipComputed: true}), audio.Snapshot(canonicalStreamPolicy{}), menu.Snapshot(canonicalStreamPolicy{}),
	}}
	attachCanonicalStore(&report)
	before := captureLegacyReportState(report, true)
	json := RenderJSON([]Report{report})
	text := RenderText([]Report{report})
	raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
	for _, want := range []string{"1 024 MiB", "11.719 FPS (4096 SPF)", "Maximum bit rate", "7 500 kb/s", "Minimum frame rate", "Maximum frame rate", "23.976 FPS", "29.970 FPS", "Top Field First", "Format settings, picture structure", "ID in the original source medium", "189 (0xBD)128 (0x80)", "Encoded date", "USER_TAG"} {
		if !strings.Contains(text, want) {
			t.Errorf("friendly output missing %q", want)
		}
	}
	for _, unwanted := range []string{"Original delay", "Delay                                    :", "Format/Info                              : MPEG Video", "Encoded_Date", "Format settings, Picture structure"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("friendly output contains %q", unwanted)
		}
	}
	assertRowOrder(t, text, "Sampling rate", "Frame rate                               : 11.719", "Bit depth")
	for _, want := range []string{"1024 MiB", "FrameRate_Minimum/String", "23.976 fps", "FrameRate_Maximum/String", "ServiceName", "ServiceProvider", "ServiceType", "Video_Delay/String"} {
		if !strings.Contains(raw, want) {
			t.Errorf("raw output missing %q", want)
		}
	}
	for _, unwanted := range []string{"1 024 MiB", "Delay relative to video", "\nDelay/String", "Delay_Original/String", "Service name", "Service provider"} {
		if strings.Contains(raw, unwanted) {
			t.Errorf("raw output contains %q", unwanted)
		}
	}
	if !strings.Contains(json, `"FrameRate":"11.719"`) || !strings.Contains(json, `"FileSize":"1073739776"`) {
		t.Error("canonical numeric facts changed")
	}
	if RenderJSON([]Report{report}) != json || RenderText([]Report{report}) != text || RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"}) != raw {
		t.Error("rendering is not idempotent")
	}
	if !reflect.DeepEqual(before, captureLegacyReportState(report, true)) {
		t.Error("rendering mutated the public report")
	}
}

func TestRawQuantitiesIgnoreFriendlyWording(t *testing.T) {
	report := Report{
		General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Duration", Value: "friendly duration"}, {Name: "Overall bit rate", Value: "friendly bitrate"}, {Name: "File size", Value: "friendly size"}}, JSON: map[string]string{"Duration": "1.234", "OverallBitRate": "25700000", "FileSize": "1073739776"}},
		Streams: []Stream{
			{Kind: StreamVideo, Fields: []Field{{Name: "Width", Value: "640 pixels"}, {Name: "Height", Value: "360 pixels"}, {Name: "Format settings, Reference frames", Value: "4 frames"}}, JSON: map[string]string{"Width": "1920", "Height": "1080", "Format_Settings_RefFrames": "1"}},
			{Kind: StreamAudio, Fields: []Field{{Name: "Sampling rate", Value: "44.1 kHz"}, {Name: "Channel(s)", Value: "1 channel"}, {Name: "Bit depth", Value: "16 bits"}, {Name: "Source_Duration_LastFrame", Value: "-99 ms"}}, JSON: map[string]string{"SamplingRate": "48000", "Channels": "2", "BitDepth": "24", "Source_Duration_LastFrame": "-0.011"}},
		},
	}
	beforeJSON, beforeXML := RenderJSON([]Report{report}), RenderXML([]Report{report})
	raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
	for _, want := range []string{"1s 234ms", "25.7 Mbps", "1024 MiB", "1920 pixel", "1080 pixel", "1 frame", "48.0 KHz", "2 channel", "24 bit", "-11ms"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("missing canonical quantity %q: %s", want, raw)
		}
	}
	if strings.Contains(raw, "friendly") {
		t.Fatalf("friendly wording leaked into quantities: %s", raw)
	}
	if RenderJSON([]Report{report}) != beforeJSON || RenderXML([]Report{report}) != beforeXML || RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"}) != raw {
		t.Fatal("raw scalar rendering changed facts or repeated output")
	}
	legacy := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{{Kind: StreamAudio, Fields: []Field{{Name: "Sampling rate", Value: "48.0 kHz"}, {Name: "Channel(s)", Value: "2 channels"}, {Name: "Bit depth", Value: "24 bits"}, {Name: "Source_Duration_LastFrame", Value: "-11 ms"}}}}}
	for _, want := range []string{"48.0 KHz", "2 channel", "24 bit", "-11ms"} {
		if output := RenderTextWithOptions([]Report{legacy}, TextRenderOptions{Language: "raw"}); !strings.Contains(output, want) {
			t.Fatalf("legacy quantity missing %q: %s", want, output)
		}
	}
	for _, value := range []string{"invalid", "0"} {
		invalid := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{{Kind: StreamVideo, Fields: []Field{{Name: "Width", Value: "640 pixels"}}, JSON: map[string]string{"Width": value}}}}
		if output := RenderTextWithOptions([]Report{invalid}, TextRenderOptions{Language: "raw"}); strings.Contains(output, "Width/String") {
			t.Fatalf("invalid canonical width borrowed a friendly value: %s", output)
		}
	}
	zero := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{{Kind: StreamAudio, Fields: []Field{{Name: "Source_Duration_LastFrame", Value: "-99 ms"}}, JSON: map[string]string{"Source_Duration_LastFrame": "0.000"}}}}
	if output := RenderTextWithOptions([]Report{zero}, TextRenderOptions{Language: "raw"}); !strings.Contains(output, padRawTextLabel("Source_Duration_LastFrame/String", 33)+": 0ms") {
		t.Fatalf("present zero changed: %s", output)
	}
	lists := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{{Kind: StreamAudio, Fields: []Field{{Name: "Channel(s)", Value: "1 channel"}, {Name: "Sampling rate", Value: "1 Hz"}}, JSON: map[string]string{"Channels": "6 / 2", "SamplingRate": "48000 / 96000"}}}}
	for _, want := range []string{"6 / 2 channel", "48.0 KHz / 96.0 KHz"} {
		if output := RenderTextWithOptions([]Report{lists}, TextRenderOptions{Language: "raw"}); !strings.Contains(output, want) {
			t.Fatalf("canonical quantity list missing %q: %s", want, output)
		}
	}
}

func TestLegacyPresentationAliasesPreserveCanonicalFacts(t *testing.T) {
	report := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{{Kind: StreamAudio, Fields: []Field{
		{Name: "ID", Value: "4294967295 (0xFFFFFFFF)"},
		{Name: "Frame rate", Value: "1 200.000 FPS (80 SPF)"},
		{Name: "ID in the original source medium", Value: "189 (0xBD)128 (0x80)"},
		{Name: "Scan order", Value: "Top Field First"},
		{Name: "Muxing mode", Value: "Packed bitstream"},
	}}}}
	json := RenderJSON([]Report{report})
	for _, want := range []string{`"ID":"4294967295"`, `"FrameRate":"1200.000"`, `"OriginalSourceMedium_ID":"189-128"`, `"ScanOrder":"TFF"`, `"MuxingMode":"MuxingMode_PackedBitstream"`} {
		if !strings.Contains(json, want) {
			t.Fatalf("missing canonical value %q: %s", want, json)
		}
	}
}

func TestOggUnsignedIDAndTheoraRatePresentation(t *testing.T) {
	for _, serial := range []uint32{0, 2695936771, 4294967295} {
		stream := canonicalOggVideoStream(&oggLogicalStream{serial: serial, frameRateNum: 30, frameRateDen: 1}, 1, 30)
		report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Ogg"}}}, Streams: []Stream{stream}}
		attachCanonicalStore(&report)
		friendly := RenderText([]Report{report})
		if !strings.Contains(friendly, formatID(uint64(serial))) || !strings.Contains(friendly, "30.000 FPS") {
			t.Fatalf("Ogg presentation: %s", friendly)
		}
		for _, label := range []string{"Frame count", "FrameCount", "Pixel aspect ratio", "PixelAspectRatio"} {
			if strings.Contains(friendly, label) {
				t.Errorf("Theora friendly output exposes %q", label)
			}
		}
		if stream.JSON["FrameCount"] != "30" || stream.JSON["PixelAspectRatio"] != "1.000" {
			t.Fatal("Theora visibility changed structured facts")
		}
		if stream.JSON["ID"] != strconv.FormatUint(uint64(serial), 10) {
			t.Fatal("unsigned serial changed")
		}
	}
}

func TestReferenceFormatPresentationFamilies(t *testing.T) {
	for _, tc := range []struct{ format, profile, features, name, info string }{
		{"VP8", "", "", "VP8", ""}, {"PCM", "", "", "PCM", ""}, {"unknown", "", "fake", "unknown", ""},
		{"FLAC", "", "", "FLAC", "Free Lossless Audio Codec"},
		{"BDAV", "", "", "BDAV", "Blu-ray Video"},
		{"AAC", "", "LC SBR PS", "AAC LC SBR PS", "Advanced Audio Codec Low Complexity with Spectral Band Replication and Parametric Stereo"},
		{"AAC", "HE-AAC+eSBR", "", "AAC LC SBR eSBR", "Advanced Audio Codec Low Complexity with Enhanced Spectral Band Replication"},
		{"DTS", "Express", "", "DTS LBR", "Digital Theater Systems"},
		{"DTS LBR", "", "", "DTS LBR", "Digital Theater Systems"},
		{"DTS", "MA / Core", "", "DTS XLL", "Digital Theater Systems"},
		{"DTS", "", "LC SBR", "DTS", "Digital Theater Systems"},
		{"AC-3", "", "Dep JOC", "E-AC-3 JOC", "Enhanced AC-3 with Joint Object Coding"},
		{"E-AC-3 JOC", "", "JOC", "E-AC-3 JOC", "Enhanced AC-3 with Joint Object Coding"},
		{"MLP FBA", "", "16-ch AC-3", "MLP FBA 16-ch AC-3", "Meridian Lossless Packing FBA with 16-channel presentation"},
		{"MLP FBA", "", "Not16-ch", "MLP FBA", "Meridian Lossless Packing FBA"},
	} {
		name, info := reportFormatPresentation(reportTables23, StreamAudio, map[string]string{"Format": tc.format, "Format_Profile": tc.profile, "Format_AdditionalFeatures": tc.features})
		if name != tc.name || info != tc.info {
			t.Errorf("%s/%s/%s = %q, %q; want %q, %q", tc.format, tc.profile, tc.features, name, info, tc.name, tc.info)
		}
	}
}

func assertRowOrder(t *testing.T, text string, rows ...string) {
	t.Helper()
	previous := -1
	for _, row := range rows {
		index := strings.Index(text, row)
		if index < 0 || index <= previous {
			t.Fatalf("row %q missing or out of order", row)
		}
		previous = index
	}
}

func TestFriendlyScalarBoundaries(t *testing.T) {
	for _, tc := range []struct {
		bytes int64
		want  string
	}{
		{1047527424, "999 MiB"}, {1048051712, "1 000 MiB"}, {1073739776, "1 024 MiB"}, {1073741824, "1.00 GiB"},
	} {
		if got := formatBytes(tc.bytes); got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
	if got := formatFrameRate(1200); got != "1 200.000 FPS" {
		t.Errorf("frame rate = %q", got)
	}
}

func TestAudioFrameRateProjectionPresence(t *testing.T) {
	for _, tc := range []struct{ name, rate, spf, sampling, want string }{
		{"flac-4608", "10.417", "4608", "48000", "10.417 FPS (4608 SPF)"},
		{"flac-4096", "11.719", "4096", "48000", "11.719 FPS (4096 SPF)"},
		{"rate-only", "50.000", "", "48000", "50.000 FPS"},
		{"fractional", "1200", "80.5", "96000", "1 200.000 FPS (80.5 SPF)"},
		{"list", "50", "480 / 960", "48000", "50.000 FPS (480 / 960 SPF)"},
		{"invalid-spf", "50", "-1", "48000", "50.000 FPS"},
		{"sample-frequency", "48000", "1", "48000", ""},
		{"absent", "", "4096", "48000", ""},
		{"invalid", "NaN", "4096", "48000", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			builder := newCanonicalStreamBuilder(StreamAudio)
			builder.Structured("FrameRate", tc.rate)
			builder.Structured("SamplesPerFrame", tc.spf)
			builder.Structured("SamplingRate", tc.sampling)
			report := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{builder.Snapshot(canonicalStreamPolicy{})}}
			attachCanonicalStore(&report)
			friendly := RenderText([]Report{report})
			if tc.want == "" {
				if strings.Contains(friendly, "Frame rate") {
					t.Fatal("unsupported frame-rate row")
				}
				return
			}
			if !strings.Contains(friendly, tc.want) || strings.Count(friendly, "Frame rate") != 1 {
				t.Fatalf("frame-rate row: %s", friendly)
			}
			raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
			if strings.Contains(raw, "1 200") || strings.Count(raw, "FrameRate/String") != 1 {
				t.Fatalf("raw frame rate: %s", raw)
			}
		})
	}
}

func TestReportRegistryStandardAndExtraOrder(t *testing.T) {
	for _, container := range []string{"Matroska", "MPEG-4", "AVI", "DVD Video"} {
		general := newCanonicalStreamBuilder(StreamGeneral)
		general.Structured("Format", container)
		for _, key := range []fieldName{"Track_Position_Total", "Track_Position", "Part_Position_Total", "Album_Performer", "File_Modified_Date", "Performer", "Track", "Part", "Album", "Title"} {
			general.Structured(key, "value")
		}
		video := newCanonicalStreamBuilder(StreamVideo)
		for _, key := range []fieldName{"Comment", "Standard", "BitDepth", "DisplayAspectRatio_Original", "Encoded_Library", "StreamSize", "Width"} {
			video.Structured(key, "1")
		}
		video.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{{Key: "z", Value: structuredNode{Kind: structuredString, Text: "first"}}, {Key: "a", Value: structuredNode{Kind: structuredString, Text: "second"}}}})
		report := Report{General: general.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{video.Snapshot(canonicalStreamPolicy{SkipComputed: true})}}
		attachCanonicalStore(&report)
		json := RenderJSON([]Report{report})
		assertRowOrder(t, json, `"Title":`, `"Album":`, `"Album_Performer":`, `"Part":`, `"Part_Position_Total":`, `"Track":`, `"Track_Position":`, `"Track_Position_Total":`, `"Performer":`, `"File_Modified_Date":`, `"extra":`)
		assertRowOrder(t, json, `"Width":`, `"DisplayAspectRatio_Original":`, `"Standard":`, `"BitDepth":`, `"StreamSize":`, `"Encoded_Library":`, `"extra":`)
		assertRowOrder(t, json, `"z":`, `"a":`)
		xml := RenderXML([]Report{report})
		assertRowOrder(t, xml, "<Album_Performer>", "<Part>", "<Part_Position_Total>", "<Track>", "<Track_Position>", "<Track_Position_Total>", "<Performer>")
		if RenderJSON([]Report{report}) != json || RenderXML([]Report{report}) != xml {
			t.Fatal("schema ordering changed values or repeated results")
		}
	}
}

func TestRawStatisticsAndSourceDelayIdentity(t *testing.T) {
	report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}}}
	for range 2 {
		b := newCanonicalStreamBuilder(StreamAudio)
		b.Text("Statistics Tags Issue", "untrusted")
		b.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{
			{Key: "dialnorm_Minimum", Value: structuredNode{Kind: structuredString, Text: "-27"}},
			{Key: "Statistics_Tags_Issue", Value: structuredNode{Kind: structuredString, Text: "untrusted"}},
			{Key: "Source_Delay", Value: structuredNode{Kind: structuredString, Text: "0.009"}},
			{Key: "Source_Delay_Source", Value: structuredNode{Kind: structuredString, Text: "Container"}},
			{Key: "User_Issue", Value: structuredNode{Kind: structuredString, Text: "literal"}},
		}})
		report.Streams = append(report.Streams, b.Snapshot(canonicalStreamPolicy{}))
	}
	attachCanonicalStore(&report)
	raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
	if strings.Count(raw, "Statistics Tags Issue") != 2 || strings.Contains(raw, "  Issue ") || strings.Contains(raw, "Source_Delay") || strings.Count(raw, "User_Issue") != 2 || strings.Contains(raw, "dialnorm_Maximum") {
		t.Fatalf("raw aliases: %s", raw)
	}
	if json := RenderJSON([]Report{report}); strings.Count(json, `"Statistics_Tags_Issue"`) != 2 || strings.Count(json, `"Source_Delay"`) != 2 {
		t.Fatalf("structured extras changed: %s", json)
	}
	for _, tc := range []struct {
		name, extra, want string
		kind              StreamKind
	}{
		{"no extra", "", "untrusted", StreamAudio},
		{"empty extra", `{}`, "untrusted", StreamAudio},
		{"unrelated extra", `{"User_Tag":"literal"}`, "untrusted", StreamAudio},
		{"canonical warning", `{"Statistics_Tags_Issue":"canonical"}`, "canonical", StreamAudio},
		{"canonical empty warning", `{"Statistics_Tags_Issue":""}`, "", StreamAudio},
		{"Menu extra is structured only", `{"Statistics_Tags_Issue":"untrusted"}`, "untrusted", StreamMenu},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}}, Streams: []Stream{{
				Kind: tc.kind, Fields: []Field{{Name: "Statistics Tags Issue", Value: "untrusted"}},
			}}}
			if tc.extra != "" {
				legacy.Streams[0].JSONRaw = map[string]string{"extra": tc.extra}
			}
			before := RenderJSON([]Report{legacy})
			raw := RenderTextWithOptions([]Report{legacy}, TextRenderOptions{Language: "raw"})
			if strings.Count(raw, "Statistics Tags Issue") != 1 {
				t.Fatalf("legacy warning missing or duplicated: %s", raw)
			}
			for _, line := range strings.Split(raw, "\n") {
				label, value, ok := strings.Cut(line, ":")
				if ok && strings.TrimSpace(label) == "Statistics Tags Issue" && strings.TrimSpace(value) != tc.want {
					t.Fatalf("warning value = %q, want %q", strings.TrimSpace(value), tc.want)
				}
			}
			if raw != RenderTextWithOptions([]Report{legacy}, TextRenderOptions{Language: "raw"}) || before != RenderJSON([]Report{legacy}) {
				t.Fatal("warning projection changed between renders or mutated structured facts")
			}
		})
	}
}

func TestRawMeasuredRateAndUnknownNamesStayIndependent(t *testing.T) {
	b := newCanonicalStreamBuilder(StreamVideo)
	b.Fill("Format", "AVC", "Format", "AVC")
	b.Fill("CodecID", "avc1", "Codec ID", "avc1")
	b.Fill("BitRate", "9600000", "Bit rate", "unrelated friendly wording")
	b.Structured("Width", "1920")
	b.Structured("Height", "1080")
	b.Structured("FrameRate", "25.000")
	b.Structured("ScanType", "MBAFF")
	b.Structured("Encoded_Library_Settings", "rc=abr / bitrate=10000")
	b.Text("USER/Stringly", "first")
	b.Text("USER", "second")
	report := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{b.Snapshot(canonicalStreamPolicy{})}}
	attachCanonicalStore(&report)
	raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
	for _, want := range []string{padRawTextLabel("BitRate/String", 33) + ": 9600 Kbps", padRawTextLabel("Bits-(Pixel*Frame)", 33) + ": 0.185", "USER/Stringly", "first", "USER", "second"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("raw fact missing %q: %s", want, raw)
		}
	}
	if strings.Contains(raw, "ScanType_StoreMethod") {
		t.Fatalf("raw invented storage method: %s", raw)
	}
}

func TestReferenceCodecNamespaces(t *testing.T) {
	for _, tc := range []struct {
		container      string
		kind           StreamKind
		id, hint, info string
	}{
		{"Matroska", StreamText, "S_HDMV/PGS", "", "Picture based subtitle format used on BDs/HD-DVDs"},
		{"AVI", StreamVideo, "DX50", "DivX 5", ""},
		{"AVI", StreamAudio, "55", "MP3", ""},
		{"AVI", StreamVideo, "WVC1", "Microsoft", ""},
		{"AVI", StreamAudio, "42", "Microsoft", ""},
		{"AVI", StreamAudio, "0055", "", ""},
		{"MPEG-4", StreamVideo, "avc1", "", "Advanced Video Coding"},
		{"Matroska", StreamAudio, "unknown_XVID", "", ""},
	} {
		got := reportCodecDescription(reportTables23, tc.container, tc.kind, tc.id)
		if got.Hint != tc.hint || got.Info != tc.info {
			t.Errorf("%s/%s/%s = %#v", tc.container, tc.kind, tc.id, got)
		}
	}
}

func TestWAVCodecLookupKeepsDecimalPublicID(t *testing.T) {
	for _, tc := range []struct {
		tag        uint16
		hint, info string
	}{{0x11, "Intel", ""}, {0xA, "", "Windows Media Audio"}} {
		stream := canonicalWAVAudioStream("Unknown", tc.tag, 1, 48000, 16, 1, 128000, "Constant", 16000, 1)
		report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Wave"}}}, Streams: []Stream{stream}}
		attachCanonicalStore(&report)
		if stream.JSON["CodecID"] != strconv.FormatUint(uint64(tc.tag), 10) {
			t.Fatal("public WAV codec ID changed")
		}
		report.General.JSON["Title"] = "unrelated mutation"
		for label, want := range map[string]string{"Codec ID/Hint": tc.hint, "Codec ID/Info": tc.info} {
			friendly := RenderText([]Report{report})
			if want != "" && !strings.Contains(friendly, padRight(label, textLabelWidth)+": "+want) {
				t.Fatalf("wrong WAV codec lookup: %s", friendly)
			}
			if want == "" && strings.Contains(friendly, label) {
				t.Fatalf("invented WAV codec metadata: %s", friendly)
			}
		}
	}
}

func TestSourceStreamSizePreservesCanonicalPercentage(t *testing.T) {
	g := newCanonicalStreamBuilder(StreamGeneral)
	g.Fill("FileSize", "1048576", "File size", "stale")
	b := newCanonicalStreamBuilder(StreamAudio)
	b.Fill("Source_StreamSize", "524288", "Source stream size", "stale")
	report := Report{General: g.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{b.Snapshot(canonicalStreamPolicy{})}}
	attachCanonicalStore(&report)
	for _, output := range []string{RenderText([]Report{report}), RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})} {
		if !strings.Contains(output, "512 KiB (50%)") {
			t.Fatalf("source stream size lost percentage: %s", output)
		}
	}
}

func TestRawAACSettingsRequireEstablishedPNSFact(t *testing.T) {
	for _, setting := range []string{"", "PNS"} {
		b := newCanonicalStreamBuilder(StreamAudio)
		b.Structured("Format", "AAC")
		b.Structured("Format_Settings_SBR", "No (Explicit)")
		b.Structured("Format_Settings", setting)
		report := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{b.Snapshot(canonicalStreamPolicy{})}}
		attachCanonicalStore(&report)
		raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
		if strings.Contains(raw, "PNS") != (setting != "") {
			t.Fatalf("raw invented or lost PNS fact: %s", raw)
		}
	}
}

func TestUntaggedAVIMPEGAudioCadence(t *testing.T) {
	for _, tc := range []struct {
		rate, frameSize int
		header          byte
		want            string
	}{
		{44100, 417, 0x90, "38.281 FPS (1152 SPF)"},
		{48000, 384, 0x94, "41.667 FPS (1152 SPF)"},
	} {
		data := make([]byte, 2*tc.frameSize)
		copy(data, []byte{0xff, 0xfb, tc.header, 0x64})
		copy(data[tc.frameSize:], []byte{0xff, 0xfb, tc.header, 0x64})
		stream := canonicalAVIAudioStream(&aviStream{index: 1, audioTag: 0x55, audioRate: uint32(tc.rate), audioChans: 2}, nil, 0, 0, 0, data)
		if stream.JSON["SamplesPerFrame"] != "1152" || stream.JSON["FrameCount"] != "" {
			t.Fatalf("header cadence or tag-gated total changed at %d Hz", tc.rate)
		}
		if got := findField(stream.Fields, "Frame rate"); got != tc.want {
			t.Fatalf("cadence = %q, want %q", got, tc.want)
		}
	}
}

func TestCanonicalFixedAudioCadenceDerivation(t *testing.T) {
	for _, tc := range []struct{ format, spf, want string }{
		{"FLAC", "4096", "11.719"}, {"AC-3", "1536", "31.250"}, {"MPEG Audio", "1152", "41.667"},
		{"FLAC", "4096 / 4608", ""}, {"PCM", "1", ""}, {"Opus", "960", ""},
	} {
		b := newCanonicalStreamBuilder(StreamAudio)
		b.Structured("Format", tc.format)
		b.Structured("SamplingRate", "48000")
		b.Structured("SamplesPerFrame", tc.spf)
		stream := b.Snapshot(canonicalStreamPolicy{})
		if got := stream.JSON["FrameRate"]; got != tc.want {
			t.Errorf("%s/%s derived rate = %q, want %q", tc.format, tc.spf, got, tc.want)
		}
	}
}

func TestReportRegistryOrderForEveryKind(t *testing.T) {
	for _, tc := range []struct {
		kind              StreamKind
		keys, labels, raw []string
	}{
		{StreamGeneral, []string{"Format", "FileSize", "Duration", "Encoded_Application", "Comment"}, []string{"Format", "File size", "Duration", "Writing application", "Comment"}, []string{"Format/String", "FileSize/String", "Duration/String", "Encoded_Application/String", "Comment"}},
		{StreamVideo, []string{"Format", "Width", "Standard", "BitDepth", "StreamSize"}, []string{"Format", "Width", "Standard", "Bit depth", "Stream size"}, []string{"Format/String", "Width/String", "Standard", "BitDepth/String", "StreamSize/String"}},
		{StreamAudio, []string{"Format", "SamplingRate", "BitDepth", "StreamSize", "Encoded_Library"}, []string{"Format", "Sampling rate", "Bit depth", "Stream size", "Writing library"}, []string{"Format/String", "SamplingRate/String", "BitDepth/String", "StreamSize/String", "Encoded_Library/String"}},
		{StreamText, []string{"Format", "MuxingMode", "Duration", "Language"}, []string{"Format", "Muxing mode", "Duration", "Language"}, []string{"Format/String", "MuxingMode", "Duration/String", "Language/String"}},
		{StreamImage, []string{"Format", "Width", "Height", "BitDepth", "StreamSize"}, []string{"Format", "Width", "Height", "Bit depth", "Stream size"}, []string{"Format/String", "Width/String", "Height/String", "BitDepth/String", "StreamSize/String"}},
		{StreamKind("Other"), []string{"Format", "Duration", "TimeCode_FirstFrame", "Title"}, []string{"Format", "Duration", "Time code of first frame", "Title"}, []string{"Format/String", "Duration/String", "TimeCode_FirstFrame", "Title"}},
		{StreamMenu, []string{"Format", "Duration", "ServiceName", "ServiceProvider", "ServiceType"}, []string{"Format", "Duration", "Service name", "Service provider", "Service type"}, []string{"Format/String", "Duration/String", "ServiceName", "ServiceProvider", "ServiceType"}},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			b := newCanonicalStreamBuilder(tc.kind)
			for i := len(tc.keys) - 1; i >= 0; i-- {
				b.Fill(fieldName(tc.keys[i]), "1", tc.labels[i], "1")
			}
			report := Report{General: Stream{Kind: StreamGeneral}}
			if tc.kind == StreamGeneral {
				report.General = b.Snapshot(canonicalStreamPolicy{})
			} else {
				report.Streams = []Stream{b.Snapshot(canonicalStreamPolicy{})}
			}
			attachCanonicalStore(&report)
			for format, output := range map[string]string{"JSON": RenderJSON([]Report{report}), "XML": RenderXML([]Report{report}), "text": RenderText([]Report{report}), "HTML": RenderHTML([]Report{report}), "CSV": RenderCSV([]Report{report}), "raw": RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})} {
				rows := make([]string, len(tc.keys))
				for i, key := range tc.keys {
					switch format {
					case "JSON":
						rows[i] = `"` + key + `":`
					case "XML":
						rows[i] = "<" + key + ">"
					case "text":
						rows[i] = padRight(tc.labels[i], textLabelWidth) + ":"
					case "HTML":
						rows[i] = "<td>" + tc.labels[i] + "</td>"
					case "CSV":
						rows[i] = tc.labels[i] + ","
					case "raw":
						rows[i] = padRawTextLabel(tc.raw[i], 33) + ":"
					}
				}
				assertRowOrder(t, output, rows...)
			}
		})
	}
}

func TestScanPresentationUsesExactReferenceTranslations(t *testing.T) {
	for _, tc := range []struct{ method, fields, want string }{
		{"SeparatedFields", "", "Separated fields"}, {"SeparatedFields", "1", "Separated fields (1 field per block)"}, {"SeparatedFields", "2", "Separated fields (2 fields per block)"},
		{"InterleavedFields", "", "Interleaved fields"}, {"InterleavedFields", "1", "InterleavedFields"}, {"Unknown", "2", "Unknown"},
	} {
		b := newCanonicalStreamBuilder(StreamVideo)
		b.Structured("ScanType_StoreMethod", tc.method)
		b.Structured("ScanType_StoreMethod_FieldsPerBlock", tc.fields)
		b.Structured("ScanOrder_Original", "BFF")
		b.Structured("ScanOrder_Stored", "2:3 Pulldown")
		report := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{b.Snapshot(canonicalStreamPolicy{})}}
		attachCanonicalStore(&report)
		friendly := RenderText([]Report{report})
		for _, row := range []string{padRight("Scan type, store method", textLabelWidth) + ": " + tc.want, "Bottom Field First", "2:3 Pulldown"} {
			if !strings.Contains(friendly, row) {
				t.Fatalf("missing scan translation %q: %s", row, friendly)
			}
		}
		raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
		if strings.Contains(raw, "Bottom Field First") || !strings.Contains(raw, "BFF") || !strings.Contains(raw, tc.method) {
			t.Fatalf("raw scan values: %s", raw)
		}
	}
}
