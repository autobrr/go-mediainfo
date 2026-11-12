package mediainfo

import (
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
		if !strings.Contains(output, "Count of views") || !strings.Contains(output, "MultiView_Layout") {
			t.Fatalf("missing multiview text: %s", output)
		}
	}
	if !strings.Contains(RenderJSON([]Report{report}), `"MultiView_Count":"2"`) {
		t.Fatal("canonical multiview count lost")
	}
}
