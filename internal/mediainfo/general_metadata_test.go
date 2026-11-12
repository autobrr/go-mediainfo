package mediainfo

import (
	"encoding/binary"
	"encoding/hex"
	encodingjson "encoding/json"
	encodingxml "encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneralTitleSelection(t *testing.T) {
	for _, tc := range []struct {
		name                                                  string
		kind                                                  StreamKind
		collection, movie, track, title, wantMovie, wantTrack string
	}{
		{name: "video", kind: StreamVideo, title: "Title", wantMovie: "Title"},
		{name: "audio", kind: StreamAudio, title: "Title", wantTrack: "Title"},
		{name: "cover", kind: StreamImage, title: "Title", wantTrack: "Title"},
		{name: "collection", kind: StreamVideo, collection: "Series", title: "Title", wantTrack: "Title"},
		{name: "explicit track", kind: StreamVideo, title: "Title", track: "Track", wantTrack: "Track"},
		{name: "explicit both", kind: StreamAudio, title: "Title", movie: "Movie", track: "Track", wantMovie: "Movie", wantTrack: "Track"},
		{name: "backfill movie", kind: StreamVideo, movie: "Movie", wantMovie: "Movie"},
		{name: "backfill track", kind: StreamAudio, track: "Track", wantTrack: "Track"},
		{name: "absent", kind: StreamVideo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			general := newCanonicalStreamBuilder(StreamGeneral)
			for key, value := range map[fieldName]string{"Title": tc.title, "Collection": tc.collection, "Movie": tc.movie, "Track": tc.track} {
				general.Structured(key, value)
			}
			report := Report{General: general.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{{Kind: tc.kind}}}
			attachCanonicalStore(&report)
			if report.General.JSON["Movie"] != tc.wantMovie || report.General.JSON["Track"] != tc.wantTrack {
				t.Fatalf("Movie/Track = %q/%q, want %q/%q", report.General.JSON["Movie"], report.General.JSON["Track"], tc.wantMovie, tc.wantTrack)
			}
			wantTitle := firstNonEmpty(tc.title, tc.movie, tc.track)
			if report.General.JSON["Title"] != wantTitle {
				t.Errorf("Title = %q, want %q", report.General.JSON["Title"], wantTitle)
			}
			text := RenderText([]Report{report})
			for _, row := range []struct{ label, value string }{{"Movie name", tc.wantMovie}, {"Track name", tc.wantTrack}} {
				if row.value != "" && !strings.Contains(text, row.label) {
					t.Errorf("missing %s", row.label)
				}
			}
			before := RenderJSON([]Report{report})
			if RenderJSON([]Report{report}) != before {
				t.Fatal("repeat changed facts")
			}
		})
	}
}

func TestVorbisDiscMetadata(t *testing.T) {
	tags := map[string]string{"TITLE": "Title", "ALBUM": "Album", "ALBUMARTIST": "Artist", "TRACKNUMBER": "02", "TRACKTOTAL": "09", "DISCNUMBER": "01", "TOTALDISCS": "03", "DISCTOTAL": "04"}
	facts, extra := flacTagsToGeneralFacts(tags, []string{"TITLE", "ALBUM", "ALBUMARTIST", "TRACKNUMBER", "TRACKTOTAL", "DISCNUMBER", "TOTALDISCS", "DISCTOTAL"}, "")
	if facts.Canonical("Part_Position") != "01" || facts.Canonical("Part_Position_Total") != "03" || facts.Canonical("Part") != "" {
		t.Fatal("incorrect canonical disc fields")
	}
	if extra != nil {
		t.Fatal("consumed aliases leaked to extra")
	}
	_, oggFacts, _ := oggGeneralMetadata(map[uint32]*oggLogicalStream{1: {tags: tags}}, []uint32{1})
	if oggFacts.Canonical("Part_Position") != "01" || oggFacts.Canonical("Part_Position_Total") != "03" {
		t.Fatal("Ogg disc mapping differs")
	}
}

func TestGeneralTitleCompanionsAndDVDPolicy(t *testing.T) {
	for _, format := range []string{"MPEG-4", "DVD Video"} {
		general := newCanonicalStreamBuilder(StreamGeneral)
		for key, value := range map[fieldName]string{"Format": format, "Title": "Title", "Title_More": "More", "Movie_More": "Explicit", "Title/Url": "https://example.invalid/title"} {
			general.Structured(key, value)
		}
		report := Report{General: general.Snapshot(canonicalStreamPolicy{}), Streams: []Stream{{Kind: StreamVideo}}}
		attachCanonicalStore(&report)
		if report.General.JSON["Movie_More"] != "Explicit" || report.General.JSON["Title_More"] != "More" {
			t.Fatal("companion fill replaced an explicit value")
		}
		text := RenderText([]Report{report})
		if strings.Contains(text, "Movie name") != (format != "DVD Video") {
			t.Fatalf("wrong title visibility for %s: %s", format, text)
		}
		if !strings.Contains(RenderJSON([]Report{report}), `"Title":"Title"`) {
			t.Fatal("friendly policy removed structured title")
		}
	}
}

func TestFLACMetadataProjection(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	tags := []string{"TITLE=Song", "ALBUM=Album", "ALBUMARTIST=Artist", "TRACKNUMBER=02", "TRACKTOTAL=09", "DISCNUMBER=01", "TOTALDISCS=03", "CUSTOM_FIELD=kept"}
	comment := binary.LittleEndian.AppendUint32(nil, 0)
	comment = binary.LittleEndian.AppendUint32(comment, uint32(len(tags)))
	for _, tag := range tags {
		comment = binary.LittleEndian.AppendUint32(comment, uint32(len(tag)))
		comment = append(comment, tag...)
	}
	data := append([]byte("fLaC\x00\x00\x00\x22"), streamInfo...)
	data = append(data, 0x84, byte(len(comment)>>16), byte(len(comment)>>8), byte(len(comment)))
	data = append(data, comment...)
	vorbis := buildOggPageForTest(7, 0x02, 0, vorbisIdentificationPacketForTest())
	vorbis = append(vorbis, buildOggPageForTest(7, 0, 0, append([]byte("\x03vorbis"), comment...))...)
	vorbis = append(vorbis, buildOggPageForTest(7, 0x04, 4096, []byte{0})...)
	for _, fixture := range []struct {
		name string
		data []byte
	}{
		{"standalone.flac", data},
		{"flac.ogg", buildOggFLACForTest(append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...), "", tags...)},
		{"vorbis.ogg", vorbis},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), fixture.name)
			if err := os.WriteFile(path, fixture.data, 0o600); err != nil {
				t.Fatal(err)
			}
			report, err := AnalyzeFile(path)
			if err != nil {
				t.Fatal(err)
			}
			reports := []Report{report}
			for _, output := range []string{RenderText(reports), RenderCSV(reports), RenderHTML(reports)} {
				for _, label := range []string{"Album/Performer", "Track name", "Track name/Position", "Track name/Total", "Part/Position", "Part/Total"} {
					if fixture.name != "standalone.flac" && label != "Track name" && label != "Part/Position" && label != "Part/Total" {
						continue
					}
					if !strings.Contains(output, label) {
						t.Errorf("missing %s in %s", label, output)
					}
				}
			}

			for _, row := range []struct{ label, value string }{{"Track name/Position", "02"}, {"Track name/Total", "09"}, {"Part/Position", "01"}, {"Part/Total", "03"}} {
				if fixture.name != "standalone.flac" && strings.HasPrefix(row.label, "Track") {
					continue
				}
				for _, check := range []struct{ output, want string }{
					{RenderText(reports), padRight(row.label, textLabelWidth) + ": " + row.value + "\n"},
					{RenderCSV(reports), row.label + "," + row.value + "\n"},
					{RenderHTML(reports), "<tr><td>" + row.label + "</td><td>" + row.value + "</td></tr>"},
				} {
					if !strings.Contains(strings.ReplaceAll(check.output, "\r\n", "\n"), check.want) {
						t.Errorf("missing exact row %q in %s", check.want, check.output)
					}
				}
			}
			xml := RenderXML(reports)
			raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"})
			for _, row := range []struct{ key, value string }{{"Part/Position", "01"}, {"Part/Position_Total", "03"}} {
				if !strings.Contains(strings.ReplaceAll(raw, "\r\n", "\n"), padRawTextLabel(row.key, 33)+": "+row.value+"\n") {
					t.Errorf("missing raw %s=%s: %s", row.key, row.value, raw)
				}
			}
			json := RenderJSON(reports)
			for _, fact := range []string{`"Title":"Song"`, `"Track":"Song"`, `"Track_Position":"02"`, `"Part_Position":"01"`, `"Part_Position_Total":"03"`, `"CUSTOM_FIELD":"kept"`} {
				if fixture.name != "standalone.flac" && (strings.Contains(fact, "Track_Position") || strings.Contains(fact, "CUSTOM_FIELD")) {
					continue
				}
				if !strings.Contains(json, fact) {
					t.Errorf("missing %s in %s", fact, json)
				}
			}
			// Decode the rendered outputs so Audio fields cannot satisfy General assertions.
			type discTrack struct {
				Kind     string `json:"@type" xml:"type,attr"`
				Position string `json:"Part_Position" xml:"Part_Position"`
				Total    string `json:"Part_Position_Total" xml:"Part_Position_Total"`
			}
			var jsonDocument struct {
				Media struct {
					Tracks []discTrack `json:"track"`
				} `json:"media"`
			}
			if err := encodingjson.Unmarshal([]byte(json), &jsonDocument); err != nil {
				t.Fatal(err)
			}
			var xmlDocument struct {
				Tracks []discTrack `xml:"media>track"`
			}
			if err := encodingxml.Unmarshal([]byte(xml), &xmlDocument); err != nil {
				t.Fatal(err)
			}
			for _, output := range []struct {
				name   string
				tracks []discTrack
			}{{"JSON", jsonDocument.Media.Tracks}, {"XML", xmlDocument.Tracks}} {
				generalCount := 0
				for _, track := range output.tracks {
					if track.Kind == "General" {
						generalCount++
						if track.Position != "01" || track.Total != "03" {
							t.Errorf("%s General disc=%q/%q", output.name, track.Position, track.Total)
						}
					} else if track.Position != "" || track.Total != "" {
						t.Errorf("%s disc fields leaked to %s", output.name, track.Kind)
					}
				}
				if generalCount != 1 {
					t.Errorf("%s General count=%d", output.name, generalCount)
				}
			}
			if strings.Contains(json, `"Part":`) || strings.Contains(json, `"DISCNUMBER":`) {
				t.Fatal("disc aliases leaked")
			}
		})
	}
}

func TestAdoptedOperatingSystemSerializedAlias(t *testing.T) {
	const alias = "Encoded_OperatingSystem_String"
	if got := reportFieldIdentity(reportTables23, StreamGeneral, alias, ""); got != "Encoded_OperatingSystem" {
		t.Errorf("identity = %q", got)
	}
	if got, want := reportFieldRank(reportTables23, StreamGeneral, alias), reportFieldRank(reportTables23, StreamGeneral, "Encoded_OperatingSystem/String"); got != want {
		t.Errorf("rank = %d, want %d", got, want)
	}
}
