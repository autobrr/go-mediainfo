package mediainfo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMatroskaTagsPrecedeRawOnlyCodecExtras(t *testing.T) {
	for _, tag := range []string{"A_TAG", "ServiceName"} {
		t.Run(tag, func(t *testing.T) {
			stream := Stream{Kind: StreamAudio, canonicalSeed: matroskaAC3CanonicalSeed(matroskaAC3CanonicalFacts{format: "AC-3", trackUID: 42})}
			applyMatroskaAC3CanonicalProbe(&stream, &matroskaAudioProbe{format: "AC-3"}, ac3Info{hasDialnorm: true, dialnorm: -27}, false)
			set := &matroskaTagSet{}
			set.set(matroskaTagField{rawName: tag, name: tag, value: "value"})
			info := MatroskaInfo{Tracks: []Stream{stream}, scopedTags: matroskaScopedTags{tracks: map[uint64]*matroskaTagSet{42: set}}}
			applyMatroskaTrackTags(&info)
			refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
			report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}}, Streams: info.Tracks}
			attachCanonicalStore(&report)
			for _, mutate := range []bool{false, true} {
				extra := `{"dialnorm":"-27","lfeon":"0","` + tag + `":"value"`
				wantExtra := `"extra":` + extra + `}`
				if mutate {
					report.Streams[0].JSONRaw["extra"] = extra + `,"CALLER":"changed"}`
					wantExtra = `"extra":` + report.Streams[0].JSONRaw["extra"]
				}
				before := RenderJSON([]Report{report})
				raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
				assertRowOrder(t, raw, tag, padRawTextLabel("dialnorm", 33)+": -27")
				if !strings.Contains(before, wantExtra) || RenderJSON([]Report{report}) != before || RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"}) != raw {
					t.Fatalf("raw-only codec ordering changed canonical facts or repeated output (mutation=%v): %s", mutate, before)
				}
			}
		})
	}
}

func TestMatroskaTrackTagsWithoutExistingTagText(t *testing.T) {
	stream := Stream{Kind: StreamAudio, canonicalSeed: matroskaTrueHDCanonicalSeed(matroskaTrueHDCanonicalFacts{trackUID: 42})}
	applyMatroskaTrueHDCanonicalProbe(&stream, trueHDInfo{atmos: true, hasDynamicObjects: true, dynamicObjects: 3})
	set := &matroskaTagSet{}
	for _, name := range []string{"Z_TAG", "A_TAG"} {
		set.set(matroskaTagField{rawName: name, name: name, value: "value"})
	}
	info := MatroskaInfo{Tracks: []Stream{stream}, scopedTags: matroskaScopedTags{tracks: map[uint64]*matroskaTagSet{42: set}}}
	applyMatroskaTrackTags(&info)
	report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}}, Streams: info.Tracks}
	attachCanonicalStore(&report)
	json := RenderJSON([]Report{report})
	text := RenderText([]Report{report})
	raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
	assertRowOrder(t, text, "A_TAG", "Z_TAG", "Number of dynamic objects")
	assertRowOrder(t, raw, "A_TAG", "Z_TAG", "NumberOfDynamicObjects")
	applyMatroskaTrackTags(&info)
	repeated := Report{General: report.General, Streams: info.Tracks}
	attachCanonicalStore(&repeated)
	if RenderJSON([]Report{repeated}) != json || RenderText([]Report{repeated}) != text || RenderTextWithOptions([]Report{repeated}, TextRenderOptions{Language: "raw"}) != raw {
		t.Fatal("first tag fill or repeated merge changed ordering or canonical facts")
	}
}

func TestMatroskaTrackTagTextPrecedesCodecExtras(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Structured("UniqueID", "42")
	builder.Fill("Format", "MLP FBA", "Format", "MLP FBA")
	builder.Text("Original source medium", "synthetic medium")
	builder.Text("Source", "disc")
	builder.Text("Number of dynamic objects", "3")
	builder.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{
		{Key: "OriginalSourceMedium", Value: structuredNode{Kind: structuredString, Text: "synthetic medium"}},
		{Key: "NumberOfDynamicObjects", Value: structuredNode{Kind: structuredString, Text: "3"}},
		{Key: "Source", Value: structuredNode{Kind: structuredString, Text: "disc"}},
	}})
	set := &matroskaTagSet{}
	for _, tag := range []matroskaTagField{{rawName: "Z_TAG", name: "Z_TAG", value: "z"}, {rawName: "Source", name: "Source", value: "disc"}, {rawName: "A_TAG", name: "A_TAG", value: "a"}} {
		set.set(tag)
	}
	info := MatroskaInfo{Tracks: []Stream{builder.Snapshot(canonicalStreamPolicy{})}, scopedTags: matroskaScopedTags{tracks: map[uint64]*matroskaTagSet{42: set}}}
	applyMatroskaTrackTags(&info)
	report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}}, Streams: info.Tracks}
	attachCanonicalStore(&report)
	json := RenderJSON([]Report{report})
	if !strings.Contains(json, `"extra":{"OriginalSourceMedium":"synthetic medium","NumberOfDynamicObjects":"3","Source":"disc","A_TAG":"a","Z_TAG":"z"}`) {
		t.Fatalf("structured extra order or values changed: %s", json)
	}
	text := RenderText([]Report{report})
	raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
	assertRowOrder(t, text, "Original source medium", "A_TAG", padRight("Source", textLabelWidth)+": disc", "Z_TAG", "Number of dynamic objects")
	assertRowOrder(t, raw, "OriginalSourceMedium", "A_TAG", padRawTextLabel("Source", 33)+": disc", "Z_TAG", "NumberOfDynamicObjects")
	applyMatroskaTrackTags(&info)
	repeated := Report{General: report.General, Streams: info.Tracks}
	attachCanonicalStore(&repeated)
	if RenderJSON([]Report{repeated}) != json || RenderText([]Report{repeated}) != text || RenderTextWithOptions([]Report{repeated}, TextRenderOptions{Language: "raw"}) != raw {
		t.Fatal("reapplying tags changed duplicate handling or presentation")
	}
}

func TestParseMatroskaTagsRetainsArbitraryGeneralMetadata(t *testing.T) {
	targets := buildMatroskaElement(mkvIDTagTargets, nil)
	parent := buildMatroskaElement(mkvIDTagName, []byte("PARENT"))
	parent = append(parent, buildMatroskaSimpleTag("CHILD", "nested")...)
	body := append([]byte(nil), targets...)
	body = append(body, buildMatroskaSimpleTag("NEW_DATABASE_ID", "db-42")...)
	body = append(body, buildMatroskaSimpleTag("MYANIMELIST", "upper")...)
	body = append(body, buildMatroskaSimpleTag("MyAnimeList", "mixed")...)
	body = append(body, buildMatroskaSimpleTag("DUPLICATE", "first")...)
	body = append(body, buildMatroskaSimpleTag("DUPLICATE", "second")...)
	body = append(body, buildMatroskaSimpleTag("ARTIST", "Performer Name")...)
	body = append(body, buildMatroskaSimpleTag("BITSPS", "discard")...)
	body = append(body, buildMatroskaElement(mkvIDSimpleTag, parent)...)
	tags := buildMatroskaElement(mkvIDTag, body)

	_, _, _, _, raw, scoped := parseMatroskaTags(tags, "", "")
	if raw["NEW_DATABASE_ID"] != "db-42" || raw["DUPLICATE"] != "second" {
		t.Fatalf("raw General tags = %#v", raw)
	}
	if scoped.general.get("Performer") != "Performer Name" || scoped.general.get("PARENT/CHILD") != "nested" {
		t.Fatalf("normalized General tags = %#v", scoped.general.values)
	}
	if scoped.general.get("BITSPS") != "" {
		t.Fatalf("suppressed tag retained: %#v", scoped.general.values)
	}

	general := Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}, JSON: map[string]string{}}
	applyMatroskaGeneralTags(&general, scoped.general)
	refreshCanonicalCompatibilitySnapshot(&general)
	fields := buildJSONGeneralFields(Report{General: general})
	node := canonicalSeedStructuredNode(&general, "extra")
	if node == nil {
		t.Fatal("General canonical extra is missing")
	}
	extra := structuredNodeText(*node)
	var decoded map[string]string
	if err := json.Unmarshal([]byte(extra), &decoded); err != nil {
		t.Fatalf("General extra is invalid: %v: %s", err, extra)
	}
	for name, want := range map[string]string{
		"NEW_DATABASE_ID": "db-42", "MYANIMELIST": "upper", "MyAnimeList": "mixed", "DUPLICATE": "second", "PARENT_CHILD": "nested",
	} {
		if decoded[name] != want {
			t.Fatalf("extra[%q] = %q, want %q: %s", name, decoded[name], want, extra)
		}
	}
	if jsonFieldValue(fields, "Performer") != "Performer Name" {
		t.Fatalf("known alias was not projected as Performer: %#v", fields)
	}
}

func TestMergeMatroskaTagSetPreservesSourceEncounterOrder(t *testing.T) {
	dst := matroskaTagSet{}
	dst.set(matroskaTagField{rawName: "HEAD", name: "HEAD", value: "head"})
	src := matroskaTagSet{}
	src.set(matroskaTagField{rawName: "Z_LAST_WINDOW_FIRST", name: "Z_LAST_WINDOW_FIRST", value: "first"})
	src.set(matroskaTagField{rawName: "A_LAST_WINDOW_SECOND", name: "A_LAST_WINDOW_SECOND", value: "second"})

	mergeMatroskaTagSet(&dst, src)

	if got := dst.values["Z_LAST_WINDOW_FIRST"].order; got != 1 {
		t.Fatalf("first source field order = %d, want 1", got)
	}
	if got := dst.values["A_LAST_WINDOW_SECOND"].order; got != 2 {
		t.Fatalf("second source field order = %d, want 2", got)
	}
}

func TestApplyMatroskaGeneralTagsPreservesParserDerivedFields(t *testing.T) {
	set := matroskaTagSet{}
	set.set(matroskaTagField{rawName: "Format", name: "Format", value: "Spoofed"})
	set.set(matroskaTagField{rawName: "COMMENT", name: "Comment", value: "Tag comment"})
	general := Stream{Kind: StreamGeneral}
	replaceCanonicalSeedFill(&general, "Format", "Matroska", "Format", "Matroska")

	applyMatroskaGeneralTags(&general, set)
	refreshCanonicalCompatibilitySnapshot(&general)
	fields := buildJSONGeneralFields(Report{General: general})
	if got := jsonFieldValue(fields, "Format"); got != "Matroska" {
		t.Fatalf("Format = %q, want parser-derived Matroska", got)
	}
	if got := jsonFieldValue(fields, "Comment"); got != "Tag comment" {
		t.Fatalf("Comment = %q, want Tag comment", got)
	}
}

func TestMatroskaGeneralTagAliasesUseMediaInfoJSONNames(t *testing.T) {
	body := buildMatroskaElement(mkvIDTagTargets, nil)
	for name, value := range map[string]string{
		"INTERNAL":             "private",
		"WRITING_LIBRARY":      "encoder build",
		"SOURCE":               "disc",
		"ORIGINAL_SOURCE_FORM": "source release",
	} {
		body = append(body, buildMatroskaSimpleTag(name, value)...)
	}
	tags := buildMatroskaElement(mkvIDTag, body)

	_, _, _, _, _, scoped := parseMatroskaTags(tags, "", "")
	general := Stream{Kind: StreamGeneral, JSON: map[string]string{}}
	applyMatroskaGeneralTags(&general, scoped.general)
	refreshCanonicalCompatibilitySnapshot(&general)
	if got := general.JSON["OriginalSourceForm"]; got != "source release" {
		t.Fatalf("OriginalSourceForm = %q, want source release", got)
	}
	node := canonicalSeedStructuredNode(&general, "extra")
	if node == nil || node.Kind != structuredObject {
		t.Fatalf("General canonical extra = %#v", node)
	}
	extra := make(map[string]string, len(node.Object))
	for _, member := range node.Object {
		extra[member.Key] = member.Value.Text
	}
	for name, want := range map[string]string{
		"Internal": "private", "Writing_Library": "encoder build", "Source": "disc",
	} {
		if extra[name] != want {
			t.Fatalf("extra[%q] = %q, want %q: %#v", name, extra[name], want, extra)
		}
	}
}

func TestApplyMatroskaGeneralTagsKeepsTitleAndMovieAligned(t *testing.T) {
	body := buildMatroskaElement(mkvIDTagTargets, nil)
	body = append(body, buildMatroskaSimpleTag("TITLE", "Tag title")...)
	_, _, _, _, _, scoped := parseMatroskaTags(buildMatroskaElement(mkvIDTag, body), "", "")
	general := Stream{Kind: StreamGeneral}
	replaceCanonicalSeedFill(&general, "Title", "Container title", "Title", "Container title")

	applyMatroskaGeneralTags(&general, scoped.general)
	refreshCanonicalCompatibilitySnapshot(&general)
	want := "Container title / Tag title"
	if general.JSON["Title"] != want || general.JSON["Movie"] != want {
		t.Fatalf("Title/Movie = %q/%q, want %q", general.JSON["Title"], general.JSON["Movie"], want)
	}
}

func TestParseMatroskaTagsUsesTargetsAndIgnoresMissingTargets(t *testing.T) {
	untargeted := buildMatroskaElement(mkvIDTag, buildMatroskaSimpleTag("IGNORED", "value"))
	_, _, _, _, _, scoped := parseMatroskaTags(untargeted, "", "")
	if len(scoped.general.values) != 0 || len(scoped.tracks) != 0 {
		t.Fatalf("tag without Targets was retained: %#v", scoped)
	}

	targets := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(42)))
	body := append([]byte(nil), targets...)
	body = append(body, buildMatroskaSimpleTag("NEW_TRACK_ID", "track-42")...)
	body = append(body, buildMatroskaSimpleTag("COMMENT", "track comment")...)
	targeted := buildMatroskaElement(mkvIDTag, body)
	_, _, _, _, general, scoped := parseMatroskaTags(targeted, "", "")
	if len(general) != 0 || scoped.tracks[42] == nil {
		t.Fatalf("targeted tags were promoted or dropped: general=%#v scoped=%#v", general, scoped)
	}

	stream := Stream{Kind: StreamAudio}
	replaceCanonicalSeedFill(&stream, "UniqueID", "42", "Unique ID", "42")
	info := MatroskaInfo{Tracks: []Stream{stream}, scopedTags: scoped}
	applyMatroskaTrackTags(&info)
	node := canonicalSeedStructuredNode(&info.Tracks[0], "extra")
	if node == nil {
		t.Fatal("track canonical extra is missing")
	}
	extra := structuredNodeText(*node)
	var decoded map[string]string
	if err := json.Unmarshal([]byte(extra), &decoded); err != nil {
		t.Fatalf("track extra is invalid: %v: %s", err, extra)
	}
	if decoded["NEW_TRACK_ID"] != "track-42" || decoded["Comment"] != "track comment" {
		t.Fatalf("track extra = %#v", decoded)
	}
}

func TestApplyMatroskaTrackTagsSeedsKnownCompatibilityField(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Structured("UniqueID", "42")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	set := &matroskaTagSet{}
	set.set(matroskaTagField{rawName: "LANGUAGE", name: "Language", value: "ja"})
	info := MatroskaInfo{
		Tracks: []Stream{stream},
		scopedTags: matroskaScopedTags{tracks: map[uint64]*matroskaTagSet{
			42: set,
		}},
	}

	applyMatroskaTrackTags(&info)

	if got, found := canonicalSeedValue(info.Tracks[0], "Language"); !found || got != "ja" {
		t.Fatalf("canonical Language = %q, %v; want ja", got, found)
	}
	if got := info.Tracks[0].JSON["Language"]; got != "" {
		t.Fatalf("parser retained transient JSON Language = %q", got)
	}
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	if got := info.Tracks[0].JSON["Language"]; got != "ja" {
		t.Fatalf("compatibility Language = %q, want ja", got)
	}
}

func TestMatroskaTagBlockAdditionTargetIsNotProjected(t *testing.T) {
	targetPayload := buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(42))
	targetPayload = append(targetPayload, buildMatroskaElement(mkvIDTagBlockAddIDValue, encodeMatroskaUint(1))...)
	body := buildMatroskaElement(mkvIDTagTargets, targetPayload)
	body = append(body, buildMatroskaSimpleTag("BLOCK_TAG", "value")...)
	tags := buildMatroskaElement(mkvIDTag, body)

	_, _, _, _, _, scoped := parseMatroskaTags(tags, "", "")
	if len(scoped.general.values) != 0 || len(scoped.tracks) != 0 {
		t.Fatalf("block-addition tag was projected: %#v", scoped)
	}
}
