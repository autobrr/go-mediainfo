package mediainfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMatroskaTaggedFixture writes a minimal Matroska file with file-level
// tags and one PNG attachment.
func writeMatroskaTaggedFixture(t *testing.T) string {
	t.Helper()
	attachment := buildMatroskaElement(mkvIDFileData, minimalPNG(16, 9, 2))
	attachment = append(attachment, buildMatroskaElement(mkvIDFileName, []byte("cover.png"))...)
	attachment = append(attachment, buildMatroskaElement(mkvIDFileMimeType, []byte("image/png"))...)
	attachment = append(attachment, buildMatroskaElement(mkvIDFileDescription, []byte("Front cover"))...)
	attachment = append(attachment, buildMatroskaElement(mkvIDFileUID, encodeMatroskaUint(7))...)
	attachments := buildMatroskaElement(mkvIDAttachments, buildMatroskaElement(mkvIDAttachedFile, attachment))

	simpleTags := buildMatroskaElement(mkvIDTagTargets, nil)
	simpleTags = append(simpleTags, buildMatroskaSimpleTag("TITLE", "Sample Movie")...)
	simpleTags = append(simpleTags, buildMatroskaSimpleTag("IMDB", "tt0111161")...)
	simpleTags = append(simpleTags, buildMatroskaSimpleTag("TMDB", "movie/278")...)
	simpleTags = append(simpleTags, buildMatroskaSimpleTag("TVDB", "12345")...)
	trackTags := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(9)))
	trackTags = append(trackTags, buildMatroskaSimpleTag("FILENAME", "source.avc")...)
	trackTags = append(trackTags, buildMatroskaSimpleTag("MIMETYPE", "video/avc")...)
	tags := buildMatroskaElement(mkvIDTags, append(buildMatroskaElement(mkvIDTag, simpleTags), buildMatroskaElement(mkvIDTag, trackTags)...))

	segment := append(buildMatroskaInfo(), buildMatroskaVideoTrackWithUID(9)...)
	segment = append(segment, tags...)
	segment = append(segment, attachments...)
	file := buildMatroskaElement(mkvIDEBML, nil)
	file = append(file, buildMatroskaElement(mkvIDSegment, segment)...)
	path := filepath.Join(t.TempDir(), "tagged.mkv")
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// TestRenderTextIncludesMatroskaTagsAndAttachments locks default text output
// to MediaInfo behavior: General shows Movie name, Cover, Attachments, and
// dynamic tags such as database IDs.
func TestRenderTextIncludesMatroskaTagsAndAttachments(t *testing.T) {
	report, err := AnalyzeFileWithOptions(writeMatroskaTaggedFixture(t), defaultAnalyzeOptions())
	if err != nil {
		t.Fatalf("analyze fixture: %v", err)
	}
	text := RenderText([]Report{report})
	general := text
	if idx := strings.Index(text, "\n\nVideo"); idx >= 0 {
		general = text[:idx]
	}

	wantOrdered := []struct{ label, value string }{
		{"Movie name", "Sample Movie"},
		{"Cover", "Yes"},
		{"Attachments", "cover.png"},
		{"IMDB", "tt0111161"},
		{"TMDB", "movie/278"},
		{"TVDB", "12345"},
	}
	last := -1
	for _, want := range wantOrdered {
		line := "\n" + want.label + strings.Repeat(" ", 41-len(want.label)) + ": " + want.value
		idx := strings.Index(general, line)
		if idx < 0 {
			t.Fatalf("General text lacks %q = %q:\n%s", want.label, want.value, general)
		}
		if idx < last {
			t.Fatalf("General text order wrong at %q:\n%s", want.label, general)
		}
		last = idx
	}
	if strings.Contains(general, "\nTitle ") {
		t.Fatalf("General text still shows Title label:\n%s", general)
	}
	video := text[len(general):]
	for label, value := range map[string]string{"FILENAME": "source.avc", "MIMETYPE": "video/avc"} {
		line := "\n" + label + strings.Repeat(" ", 41-len(label)) + ": " + value
		if !strings.Contains(video, line) {
			t.Fatalf("Video text lacks track tag %q = %q:\n%s", label, value, video)
		}
	}
}

// TestMatroskaTextCodecIDInfoForASSAndSSA locks the subtitle Codec ID/Info
// lines to MediaInfo strings.
func TestMatroskaTextCodecIDInfoForASSAndSSA(t *testing.T) {
	for codecID, want := range map[string]string{
		"S_TEXT/ASS": "Advanced Sub Station Alpha",
		"S_TEXT/SSA": "Sub Station Alpha",
	} {
		seed := matroskaTextCanonicalSeed("ASS", codecID, 3, 0, 0, false, "", "", "", false, false, nil)
		found := false
		for _, entry := range seed {
			if entry.TextLabel == "Codec ID/Info" && entry.Value.Text == want {
				found = entry.Options.ShowText
			}
		}
		if !found {
			t.Fatalf("%s seed lacks Codec ID/Info %q: %+v", codecID, want, seed)
		}
	}
}
