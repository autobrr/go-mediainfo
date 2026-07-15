package mediainfo

import (
	"encoding/json"
	"testing"
)

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

	_, _, _, _, raw, scoped := parseMatroskaTags(tags, "")
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
	fields := buildJSONGeneralFields(Report{General: general})
	extra := jsonFieldValue(fields, "extra")
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

	_, _, _, _, _, scoped := parseMatroskaTags(tags, "")
	general := Stream{Kind: StreamGeneral, JSON: map[string]string{}}
	applyMatroskaGeneralTags(&general, scoped.general)
	fields := buildJSONGeneralFields(Report{General: general})
	if got := jsonFieldValue(fields, "OriginalSourceForm"); got != "source release" {
		t.Fatalf("OriginalSourceForm = %q, want source release", got)
	}
	var extra map[string]string
	if err := json.Unmarshal([]byte(jsonFieldValue(fields, "extra")), &extra); err != nil {
		t.Fatalf("General extra is invalid: %v", err)
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
	_, _, _, _, _, scoped := parseMatroskaTags(buildMatroskaElement(mkvIDTag, body), "")
	general := Stream{
		Kind:   StreamGeneral,
		Fields: []Field{{Name: "Title", Value: "Container title"}},
		JSON:   map[string]string{},
	}

	applyMatroskaGeneralTags(&general, scoped.general)
	want := "Container title / Tag title"
	if general.JSON["Title"] != want || general.JSON["Movie"] != want {
		t.Fatalf("Title/Movie = %q/%q, want %q", general.JSON["Title"], general.JSON["Movie"], want)
	}
}

func TestParseMatroskaTagsUsesTargetsAndIgnoresMissingTargets(t *testing.T) {
	untargeted := buildMatroskaElement(mkvIDTag, buildMatroskaSimpleTag("IGNORED", "value"))
	_, _, _, _, _, scoped := parseMatroskaTags(untargeted, "")
	if len(scoped.general.values) != 0 || len(scoped.tracks) != 0 {
		t.Fatalf("tag without Targets was retained: %#v", scoped)
	}

	targets := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(42)))
	body := append([]byte(nil), targets...)
	body = append(body, buildMatroskaSimpleTag("NEW_TRACK_ID", "track-42")...)
	body = append(body, buildMatroskaSimpleTag("COMMENT", "track comment")...)
	targeted := buildMatroskaElement(mkvIDTag, body)
	_, _, _, _, general, scoped := parseMatroskaTags(targeted, "")
	if len(general) != 0 || scoped.tracks[42] == nil {
		t.Fatalf("targeted tags were promoted or dropped: general=%#v scoped=%#v", general, scoped)
	}

	info := MatroskaInfo{Tracks: []Stream{{Kind: StreamAudio, JSON: map[string]string{"UniqueID": "42"}}}, scopedTags: scoped}
	applyMatroskaTrackTags(&info)
	fields := buildJSONStreamFields(info.Tracks[0], 0, 0, "Matroska")
	extra := jsonFieldValue(fields, "extra")
	var decoded map[string]string
	if err := json.Unmarshal([]byte(extra), &decoded); err != nil {
		t.Fatalf("track extra is invalid: %v: %s", err, extra)
	}
	if decoded["NEW_TRACK_ID"] != "track-42" || decoded["Comment"] != "track comment" {
		t.Fatalf("track extra = %#v", decoded)
	}
}

func TestMatroskaTagBlockAdditionTargetIsNotProjected(t *testing.T) {
	targetPayload := buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(42))
	targetPayload = append(targetPayload, buildMatroskaElement(mkvIDTagBlockAddIDValue, encodeMatroskaUint(1))...)
	body := buildMatroskaElement(mkvIDTagTargets, targetPayload)
	body = append(body, buildMatroskaSimpleTag("BLOCK_TAG", "value")...)
	tags := buildMatroskaElement(mkvIDTag, body)

	_, _, _, _, _, scoped := parseMatroskaTags(tags, "")
	if len(scoped.general.values) != 0 || len(scoped.tracks) != 0 {
		t.Fatalf("block-addition tag was projected: %#v", scoped)
	}
}
