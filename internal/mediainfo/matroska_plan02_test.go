package mediainfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatroskaNonTrackSelectorsRetainTags(t *testing.T) {
	for _, selector := range []uint64{mkvIDTagChapterUID, mkvIDTagEditionUID, mkvIDTagAttachmentUID} {
		for _, track := range []bool{false, true} {
			target := buildMatroskaElement(selector, encodeMatroskaUint(9))
			if track {
				target = append(target, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(42))...)
			}
			body := buildMatroskaElement(mkvIDTagTargets, target)
			body = append(body, buildMatroskaSimpleTag("CUSTOM_TAG", "value")...)
			_, _, _, _, raw, scoped := parseMatroskaTags(buildMatroskaElement(mkvIDTag, body), "", "")
			if track {
				if scoped.tracks[42] == nil || scoped.tracks[42].get("CUSTOM_TAG") != "value" || len(raw) != 0 {
					t.Fatalf("selector %x lost track routing: %#v", selector, scoped)
				}
			} else if scoped.general.get("CUSTOM_TAG") != "value" || raw["CUSTOM_TAG"] != "value" {
				t.Fatalf("selector %x lost General routing: %#v", selector, scoped)
			}
		}
	}
	// Block-addition targets still have no supported output destination.
	target := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagBlockAddIDValue, encodeMatroskaUint(1)))
	body := append(target, buildMatroskaSimpleTag("CUSTOM_TAG", "value")...)
	_, _, _, _, raw, scoped := parseMatroskaTags(buildMatroskaElement(mkvIDTag, body), "", "")
	if len(raw) != 0 || len(scoped.general.values) != 0 || len(scoped.tracks) != 0 {
		t.Fatal("block-addition scope expanded")
	}
}

func TestMatroskaSelectorEncoderScope(t *testing.T) {
	for _, selector := range []uint64{0, mkvIDTagChapterUID, mkvIDTagEditionUID, mkvIDTagAttachmentUID} {
		for _, uid := range []uint64{0, 42} {
			var targets []byte
			if selector != 0 {
				targets = buildMatroskaElement(selector, encodeMatroskaUint(9))
			}
			if uid != 0 {
				targets = append(targets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(uid))...)
			}
			body := buildMatroskaElement(mkvIDTagTargets, targets)
			body = append(body, buildMatroskaSimpleTag("ENCODER", "x264")...)
			body = append(body, buildMatroskaSimpleTag("ENCODER_SETTINGS", "cabac=1")...)
			encoders, settings, _, _, general, _ := parseMatroskaTags(buildMatroskaElement(mkvIDTag, body), "", "")
			if uid == 0 && (general["ENCODER"] != "x264" || general["ENCODER_SETTINGS"] != "cabac=1") {
				t.Fatalf("selector %x lost General values", selector)
			}
			if selector != 0 && uid == 0 {
				if len(encoders) != 0 || len(settings) != 0 {
					t.Errorf("selector %x leaked encoder fallback: %v %v", selector, encoders, settings)
				}
			} else if encoders[uid] != "x264" || settings[uid] != "cabac=1" {
				t.Errorf("selector %x lost encoder scope %d", selector, uid)
			}
		}
	}
}

func TestMatroskaDateNormalization(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"TBA", "TBA"}, {"DATE TBA", "DATE TBA"}, {"2026", "2026"}, {"2026-11", "2026-11"}, {"2026-11-12", "2026-11-12"},
		{"2026-11-12T03:04:05Z", "2026-11-12 03:04:05 UTC"},
		{"2026-11-12T03:04:05.123+0000", "2026-11-12 03:04:05.123 UTC"},
		{"2026-11-12T03:04:05+00:00", "2026-11-12 03:04:05 UTC"},
		{"2026-11-12T03:04:05-04:30", "2026-11-12 03:04:05-04:30"},
		{"2026-11-12T03:04", "2026-11-12T03:04"}, {"2026-11-12Tbad", "2026-11-12Tbad"},
	} {
		fields := normalizeMatroskaTag([]string{"DATE"}, tt.in)
		if len(fields) != 1 || fields[0].value != tt.want {
			t.Errorf("DATE %q = %#v, want %q", tt.in, fields, tt.want)
		}
	}
}

func TestMatroskaMappedMetadataPresentation(t *testing.T) {
	var set matroskaTagSet
	for _, name := range []string{"Performer", "EncodedBy", "Genre", "ContentType", "Synopsis", "Released_Date", "Recorded_Date", "Copyright", "Comment"} {
		set.set(matroskaTagField{rawName: name, name: name, value: "sample"})
	}
	set.set(matroskaTagField{rawName: "Writing frontend", name: "Writing frontend", value: "tool"})
	general := Stream{Kind: StreamGeneral}
	replaceCanonicalSeedFill(&general, "Format", "Matroska", "Format", "Matroska")
	applyMatroskaGeneralTags(&general, set)
	refreshCanonicalCompatibilitySnapshot(&general)
	report := Report{General: general}
	attachCanonicalStore(&report)
	text := RenderText([]Report{report})
	for _, label := range []string{"Performer", "Encoded by", "Genre", "ContentType", "Synopsis", "Released date", "Recorded date", "Copyright", "Comment", "Writing frontend"} {
		if !strings.Contains(text, label) {
			t.Errorf("missing %s: %s", label, text)
		}
	}
	if !strings.Contains(RenderJSON([]Report{report}), `"Writing_frontend":"tool"`) {
		t.Fatal("writing frontend structured identity changed")
	}
	for _, kind := range []StreamKind{StreamVideo, StreamAudio} {
		stream := Stream{Kind: kind}
		replaceCanonicalSeedFill(&stream, "UniqueID", "42", "Unique ID", "42")
		trackSet := &matroskaTagSet{}
		trackSet.set(matroskaTagField{rawName: "ENCODED_BY", name: "EncodedBy", value: "credit"})
		info := MatroskaInfo{Tracks: []Stream{stream}, scopedTags: matroskaScopedTags{tracks: map[uint64]*matroskaTagSet{42: trackSet}}}
		applyMatroskaTrackTags(&info)
		r := Report{General: general, Streams: info.Tracks}
		attachCanonicalStore(&r)
		for _, rendered := range []string{RenderText([]Report{r}), RenderHTML([]Report{r}), RenderCSV([]Report{r})} {
			if strings.Count(rendered, "Encoded by") != 2 {
				t.Fatalf("kind %v missing label: %s", kind, rendered)
			}
		}
		if !strings.Contains(RenderJSON([]Report{r}), `"EncodedBy":"credit"`) || !strings.Contains(RenderTextWithOptions([]Report{r}, TextRenderOptions{Language: "raw"}), "EncodedBy") {
			t.Fatal("EncodedBy identity lost")
		}
	}
}

func TestMatroskaMultiviewPresentation(t *testing.T) {
	stream := Stream{Kind: StreamVideo, canonicalSeed: matroskaAVCCanonicalSeed(matroskaVideoCanonicalFacts{format: "AVC", video: matroskaVideoInfo{stereoMode: 13}})}
	report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}}, Streams: []Stream{stream}}
	attachCanonicalStore(&report)
	for _, output := range []string{RenderText([]Report{report}), RenderHTML([]Report{report}), RenderCSV([]Report{report})} {
		if !strings.Contains(output, "MultiView_Count") || strings.Contains(output, "Count of views") || !strings.Contains(output, "MultiView_Layout") {
			t.Fatalf("missing multiview text: %s", output)
		}
	}
	if !strings.Contains(RenderJSON([]Report{report}), `"MultiView_Count":"2"`) {
		t.Fatal("canonical multiview count lost")
	}
}

func TestMatroskaGeneralMappedFieldsUsePinnedLabels(t *testing.T) {
	fields := []struct{ name, label string }{
		{"Description", "Description"}, {"Tagged_Date", "Tagged date"}, {"Mastered_Date", "Mastered date"},
		{"OriginalSourceForm", "Original source form"}, {"BarCode", "BarCode"}, {"TermsOfUse", "Terms of use"},
		{"Track_Position", "Track name/Position"},
	}
	var set matroskaTagSet
	for _, f := range fields {
		set.set(matroskaTagField{rawName: f.name, name: f.name, value: "sample"})
	}
	general := Stream{Kind: StreamGeneral}
	replaceCanonicalSeedFill(&general, "Format", "Matroska", "Format", "Matroska")
	applyMatroskaGeneralTags(&general, set)
	refreshCanonicalCompatibilitySnapshot(&general)
	r := Report{General: general}
	attachCanonicalStore(&r)
	for _, f := range fields {
		assertMatroskaTextRow(t, RenderText([]Report{r}), f.label, "sample")
		for _, check := range []struct{ output, row string }{
			{RenderHTML([]Report{r}), "<tr><td>" + f.label + "</td><td>sample</td></tr>"},
			{RenderCSV([]Report{r}), f.label + ",sample\n"},
		} {
			if strings.Count(strings.ReplaceAll(check.output, "\r\n", "\n"), check.row) != 1 {
				t.Errorf("expected one exact mapped row %q: %s", check.row, check.output)
			}
		}
	}
	for _, f := range fields {
		if !strings.Contains(RenderJSON([]Report{r}), `"`+f.name+`":"sample"`) {
			t.Errorf("lost structured %s", f.name)
		}
	}
}

func TestMatroskaDynamicLabelsKeepSourceAndRawIdentity(t *testing.T) {
	for _, kind := range []StreamKind{StreamGeneral, StreamVideo, StreamAudio} {
		var set matroskaTagSet
		for _, name := range []string{"Writing frontend", "Custom field"} {
			set.set(matroskaTagField{rawName: name, name: name, value: "tool"})
		}
		set.set(matroskaTagField{rawName: "Custom_field", name: "Custom_field", value: "distinct"})
		stream := Stream{Kind: kind}
		replaceCanonicalSeedFill(&stream, "UniqueID", "42", "Unique ID", "42")
		r := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}}}
		if kind == StreamGeneral {
			applyMatroskaGeneralTags(&stream, set)
			refreshCanonicalCompatibilitySnapshot(&stream)
			r.General = stream
		} else {
			info := MatroskaInfo{Tracks: []Stream{stream}, scopedTags: matroskaScopedTags{tracks: map[uint64]*matroskaTagSet{42: &set}}}
			applyMatroskaTrackTags(&info)
			refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
			r.Streams = info.Tracks
		}
		attachCanonicalStore(&r)
		for _, label := range []string{"Writing frontend", "Custom field"} {
			for _, output := range []string{RenderText([]Report{r}), RenderHTML([]Report{r}), RenderCSV([]Report{r})} {
				if strings.Count(output, label) != 1 {
					t.Errorf("kind %v friendly %s count: %s", kind, label, output)
				}
			}
			if !strings.Contains(RenderHTML([]Report{r}), "<td>"+label+"</td><td>tool</td>") || !strings.Contains(RenderCSV([]Report{r}), label+",tool\n") {
				t.Errorf("kind %v wrong friendly value for %s", kind, label)
			}
			assertMatroskaTextRow(t, RenderText([]Report{r}), label, "tool")
		}
		var decoded struct {
			Media struct {
				Track []struct {
					Type  string            `json:"@type"`
					Extra map[string]string `json:"extra"`
				} `json:"track"`
			} `json:"media"`
		}
		if err := json.Unmarshal([]byte(RenderJSON([]Report{r})), &decoded); err != nil {
			t.Fatal(err)
		}
		var extra map[string]string
		for _, track := range decoded.Media.Track {
			if track.Type == string(kind) {
				extra = track.Extra
			}
		}
		raw := RenderTextWithOptions([]Report{r}, TextRenderOptions{Language: "raw"})
		for _, key := range []string{"Writing_frontend", "Custom_field"} {
			label := strings.ReplaceAll(key, "_", " ")
			assertMatroskaTextRow(t, raw, label, "tool")
			if key != "Custom_field" && strings.Contains(raw, key) {
				t.Errorf("kind %v duplicate normalized row %s: %s", kind, key, raw)
			}
			if extra[key] != "tool" {
				t.Errorf("kind %v structured %s=%q", kind, key, extra[key])
			}
			if !strings.Contains(RenderXML([]Report{r}), "<"+key+">tool</"+key+">") {
				t.Errorf("kind %v XML lost %s", kind, key)
			}
		}
		assertMatroskaTextRow(t, raw, "Custom_field", "distinct")
		// A caller changing an unrelated public field must not lose parser-owned tag identities.
		r.General.Fields = append(r.General.Fields, Field{Name: "Caller field", Value: "changed"})
		target := &r.General
		if kind != StreamGeneral {
			target = &r.Streams[0]
		}
		extra["CALLER"] = "changed"
		encodedExtra, err := json.Marshal(extra)
		if err != nil {
			t.Fatal(err)
		}
		target.JSONRaw["extra"] = string(encodedExtra)
		raw = RenderTextWithOptions([]Report{r}, TextRenderOptions{Language: "raw"})
		for _, label := range []string{"Writing frontend", "Custom field"} {
			assertMatroskaTextRow(t, raw, label, "tool")
			if label != "Custom field" && strings.Contains(raw, strings.ReplaceAll(label, " ", "_")) {
				t.Errorf("kind %v mutation duplicates normalized tag: %s", kind, raw)
			}
		}
		assertMatroskaTextRow(t, raw, "Custom_field", "distinct")
	}
}

// assertMatroskaTextRow checks identity, value and cardinality independently of alignment width.
func assertMatroskaTextRow(t *testing.T, output, label, value string) {
	t.Helper()
	count := 0
	for _, line := range strings.Split(output, "\n") {
		key, got, ok := strings.Cut(strings.TrimSuffix(line, "\r"), ": ")
		if ok && strings.TrimSpace(key) == label {
			count++
			if got != value {
				t.Errorf("%s=%q, want %q", label, got, value)
			}
		}
	}
	if count != 1 {
		t.Errorf("%s row count %d, want 1: %s", label, count, output)
	}
}

func TestMatroskaDynamicTagPreservesParserTextRow(t *testing.T) {
	for _, tt := range []struct {
		kind                    StreamKind
		name, label, value, key string
	}{
		{StreamVideo, "UniqueID", "Unique ID", "42", "Unique_ID"},
		{StreamGeneral, "Encoded_Application", "Writing application", "parser-app", "Writing_application"},
		{StreamGeneral, "Encoded_Application", "Writing application", "tag-value", "Writing_application"},
	} {
		stream := Stream{Kind: tt.kind}
		replaceCanonicalSeedFill(&stream, fieldName(tt.name), tt.value, tt.label, tt.value)
		var set matroskaTagSet
		set.set(matroskaTagField{rawName: tt.label, name: tt.label, value: "tag-value"})
		if tt.kind == StreamGeneral {
			applyMatroskaGeneralTags(&stream, set)
		} else {
			info := MatroskaInfo{Tracks: []Stream{stream}, scopedTags: matroskaScopedTags{tracks: map[uint64]*matroskaTagSet{42: &set}}}
			applyMatroskaTrackTags(&info)
			stream = info.Tracks[0]
		}
		if got, _ := canonicalSeedValue(stream, fieldName(tt.name)); got != tt.value {
			t.Errorf("%s parser fact=%q want %q", tt.name, got, tt.value)
		}
		var found bool
		for _, entry := range stream.canonicalSeed {
			if entry.Options.ShowText && entry.TextLabel == tt.label && !entry.MatroskaTag && entry.Value.Text == tt.value {
				found = true
			}
		}
		if !found {
			t.Errorf("%s parser-owned text row overwritten or relabelled", tt.name)
		}
		extra := canonicalSeedStructuredNode(&stream, "extra")
		if extra == nil || !strings.Contains(structuredNodeText(*extra), `"`+tt.key+`":"tag-value"`) {
			t.Errorf("%s tag value lost", tt.name)
		}
		if tt.kind == StreamGeneral {
			refreshCanonicalCompatibilitySnapshot(&stream)
			r := Report{General: stream}
			attachCanonicalStore(&r)
			for _, mutated := range []bool{false, true} {
				if mutated {
					r.General.Fields = append(r.General.Fields, Field{Name: "Caller field", Value: "changed"})
					r.General.JSONRaw["extra"] = `{"Writing_application":"tag-value","CALLER":"changed"}`
				}
				text, html, csv := RenderText([]Report{r}), RenderHTML([]Report{r}), RenderCSV([]Report{r})
				for _, output := range []string{text, html, csv} {
					if strings.Count(output, "Writing application") != 2 {
						t.Errorf("mutated=%v expected two distinct same-label rows: %s", mutated, output)
					}
				}
				textValues := map[string]int{}
				for _, line := range strings.Split(text, "\n") {
					label, value, ok := strings.Cut(strings.TrimSuffix(line, "\r"), ": ")
					if ok && strings.TrimSpace(label) == "Writing application" {
						textValues[value]++
					}
				}
				wantValues := map[string]int{}
				wantValues[tt.value]++
				wantValues["tag-value"]++
				for value, count := range wantValues {
					if textValues[value] != count {
						t.Errorf("mutated=%v text value %q count=%d, want %d", mutated, value, textValues[value], count)
					}
				}
				for _, value := range []string{tt.value, "tag-value"} {
					if !strings.Contains(html, "<td>Writing application</td><td>"+value+"</td>") || !strings.Contains(csv, "Writing application,"+value+"\n") {
						t.Errorf("mutated=%v missing independent value %s", mutated, value)
					}
				}
				raw := RenderTextWithOptions([]Report{r}, TextRenderOptions{Language: "raw"})
				assertMatroskaTextRow(t, raw, "Encoded_Application/String", tt.value)
				assertMatroskaTextRow(t, raw, "Writing application", "tag-value")
				if strings.Contains(raw, "MatroskaTag/") {
					t.Fatal("internal identity leaked")
				}
				jsonOutput := RenderJSON([]Report{r})
				for _, pair := range []string{`"Encoded_Application":"` + tt.value + `"`, `"Writing_application":"tag-value"`} {
					if strings.Count(jsonOutput, pair) != 1 {
						t.Errorf("mutated=%v structured identity lost: %s", mutated, jsonOutput)
					}
				}
			}
		}
	}
}

func TestMatroskaEqualUIDTagSurvivesPublicEdit(t *testing.T) {
	stream := Stream{Kind: StreamVideo, canonicalSeed: matroskaAVCCanonicalSeed(matroskaVideoCanonicalFacts{format: "AVC", trackUID: 42})}
	var set matroskaTagSet
	set.set(matroskaTagField{rawName: "Unique ID", name: "Unique ID", value: "42"})
	info := MatroskaInfo{Tracks: []Stream{stream}, scopedTags: matroskaScopedTags{tracks: map[uint64]*matroskaTagSet{42: &set}}}
	applyMatroskaTrackTags(&info)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}}, Streams: info.Tracks}
	attachCanonicalStore(&report)
	for _, mutated := range []bool{false, true} {
		if mutated {
			report.General.Fields = append(report.General.Fields, Field{Name: "Caller field", Value: "changed"})
			report.Streams[0].JSONRaw["extra"] = `{"Unique_ID":"42","CALLER":"changed"}`
		}
		assertMatroskaTextRow(t, RenderText([]Report{report}), "Unique ID", "42")
		raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
		assertMatroskaTextRow(t, raw, "Unique ID", "42")
		if strings.Contains(raw, "UniqueID/String") || strings.Contains(raw, "Unique_ID") {
			t.Errorf("mutated=%v UID projection gained extra row: %s", mutated, raw)
		}
		output := RenderJSON([]Report{report})
		if strings.Count(output, `"Unique_ID":"42"`) != 1 {
			t.Errorf("mutated=%v lost dynamic UID tag identity: %s", mutated, output)
		}
		// Unrelated public edits currently drop structured-only parser facts in
		// the legacy adapter, independently of tags. Keep that separate from
		// this collision contract: tags must not overwrite or duplicate facts.
		if !mutated && strings.Count(output, `"UniqueID":"42"`) != 1 {
			t.Errorf("parser UID missing before public edit: %s", output)
		}
		if strings.Count(output, `"UniqueID":`) > 1 ||
			(strings.Contains(output, `"UniqueID":`) && !strings.Contains(output, `"UniqueID":"42"`)) {
			t.Errorf("mutated=%v tag overwrote or duplicated parser UID: %s", mutated, output)
		}
	}
}

func TestMatroskaAnalyzeApplicationTagCollision(t *testing.T) {
	for _, tc := range []struct {
		label, key string
		element    uint64
	}{
		{"Writing application", "Encoded_Application", mkvIDWritingApp},
		{"Writing library", "Encoded_Library", mkvIDMuxingApp},
	} {
		t.Run(tc.label, func(t *testing.T) {
			payload := append(buildMatroskaElement(mkvIDDuration, []byte{0x41, 0x20, 0, 0}), buildMatroskaElement(tc.element, []byte("Lavf63.1.102"))...)
			info := buildMatroskaElement(mkvIDInfo, payload)
			segment := append(info, buildMatroskaVideoTrackWithUID(42)...)
			tag := append(buildMatroskaElement(mkvIDTagTargets, nil), buildMatroskaSimpleTag(tc.label, "tag-value")...)
			segment = append(segment, buildMatroskaElement(mkvIDTags, buildMatroskaElement(mkvIDTag, tag))...)
			data := append(buildMatroskaElement(mkvIDEBML, nil), buildMatroskaElement(mkvIDSegment, segment)...)
			path := filepath.Join(t.TempDir(), "collision.mkv")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			report, err := AnalyzeFile(path)
			if err != nil {
				t.Fatal(err)
			}
			reports := []Report{report}
			for _, output := range []string{RenderText(reports), RenderHTML(reports), RenderCSV(reports)} {
				if strings.Count(output, tc.label) != 2 || !strings.Contains(output, "Lavf63.1.102") || !strings.Contains(output, "tag-value") {
					t.Errorf("lost independent parser/tag display: %s", output)
				}
			}
			values := map[string]int{}
			for _, line := range strings.Split(RenderText(reports), "\n") {
				label, value, ok := strings.Cut(strings.TrimSuffix(line, "\r"), ": ")
				if ok && strings.TrimSpace(label) == tc.label {
					values[value]++
				}
			}
			for _, value := range []string{"Lavf63.1.102", "tag-value"} {
				if values[value] != 1 || strings.Count(RenderHTML(reports), "<td>"+tc.label+"</td><td>"+value+"</td>") != 1 || strings.Count(RenderCSV(reports), tc.label+","+value+"\n") != 1 {
					t.Errorf("incorrect exact display count for %s=%s", tc.label, value)
				}
			}
			raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"})
			assertMatroskaTextRow(t, raw, tc.key+"/String", "Lavf63.1.102")
			assertMatroskaTextRow(t, raw, tc.label, "tag-value")
			output := RenderJSON(reports)
			for _, pair := range []string{`"` + tc.key + `":"Lavf63.1.102"`, `"` + strings.ReplaceAll(tc.label, " ", "_") + `":"tag-value"`} {
				if strings.Count(output, pair) != 1 {
					t.Errorf("lost distinct structured identity: %s", output)
				}
			}
		})
	}
}
