package mediainfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatroskaAttachmentDetectionUsesPayloadContent(t *testing.T) {
	png := minimalPNG(16, 9, 4)
	attachment := parseSingleMatroskaAttachment(t, "payload.bin", "Poster art", "application/octet-stream", png)
	stream, ok := matroskaAttachmentImageStream(attachment)
	if !ok || findField(stream.Fields, "Format") != "PNG" || findField(stream.Fields, "Title") != "Poster art" {
		t.Fatalf("content-detected PNG = ok:%v stream:%+v attachment:%#v", ok, stream, attachment)
	}
	if stream.JSON["ColorSpace"] != "YA" || attachment.mime != "application/octet-stream" {
		t.Fatalf("PNG metadata or declared MIME changed: stream=%+v attachment=%#v", stream, attachment)
	}

	fake := parseSingleMatroskaAttachment(t, "fake.png", "Not an image", "image/png", []byte("not image data"))
	if _, ok := matroskaAttachmentImageStream(fake); ok {
		t.Fatalf("image metadata promoted non-image payload: %#v", fake)
	}
}

func TestMatroskaAttachmentDetectionIncludesGIF(t *testing.T) {
	gif := []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	attachment := parseSingleMatroskaAttachment(t, "animation.dat", "animation", "application/octet-stream", gif)
	stream, ok := matroskaAttachmentImageStream(attachment)
	if !ok || findField(stream.Fields, "Format") != "GIF" || stream.JSON["Format_Profile"] != "89a" || stream.JSON["Width"] != "1" || stream.JSON["Height"] != "1" {
		t.Fatalf("GIF stream = ok:%v %+v", ok, stream)
	}
}

func TestMatroskaAttachmentCoverClassification(t *testing.T) {
	for _, test := range []struct {
		name        string
		description string
		mime        string
		want        string
	}{
		{name: "cover.bin", mime: "application/octet-stream", want: "Cover"},
		{name: "art.png", description: "Front cover", mime: "image/png", want: "Cover"},
		{name: "art.png", description: "Back cover", mime: "image/png", want: "Cover_Back"},
		{name: "disc.png", description: "CD art", mime: "image/png", want: "Cover_Media"},
		{name: "thumb.png", description: "thumbnail", mime: "image/png", want: "Thumbnail"},
		{name: "c2pa.thumbnail", mime: "image/png", want: ""},
		{name: "c2pa.thumbnail", description: "unrelated description", mime: "image/png", want: ""},
		{name: "art.png", description: "c2pa.thumbnail metadata", mime: "image/png", want: ""},
		{name: "frontcover.png", mime: "image/png", want: "Cover"},
		{name: "frontcovering.png", mime: "image/png", want: ""},
		{name: "backcover.png", mime: "image/png", want: "Cover_Back"},
		{name: "backcovering.png", mime: "image/png", want: ""},
		{name: "art.bin", description: "Front cover", mime: "application/octet-stream", want: ""},
	} {
		attachment := matroskaAttachment{name: test.name, description: test.description, mime: test.mime}
		if got := matroskaAttachmentCoverType(attachment); got != test.want {
			t.Fatalf("cover type for %#v = %q, want %q", attachment, got, test.want)
		}
	}

	cover := matroskaAttachment{name: "cover.bin", mime: "application/octet-stream", size: 4}
	if stream, ok := matroskaAttachmentImageStream(cover); !ok || stream.JSON["Type"] != "Cover" || findField(stream.Fields, "Format") != "" {
		t.Fatalf("explicit unparsed cover = ok:%v stream:%+v", ok, stream)
	}
}

func TestAnalyzeMatroskaAttachmentProjectsAcceptedCoverOnly(t *testing.T) {
	for _, test := range []struct {
		name        string
		fileName    string
		description string
		wantCover   bool
	}{
		{name: "accepted", fileName: "cover.png", description: "Front cover", wantCover: true},
		{name: "rejected", fileName: "frontcovering.png", wantCover: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := writeMatroskaAttachmentFixture(t, test.fileName, test.description, minimalPNG(16, 9, 2))
			report, err := AnalyzeFileWithOptions(path, defaultAnalyzeOptions())
			if err != nil {
				t.Fatalf("AnalyzeFileWithOptions: %v", err)
			}
			imageCount := 0
			for _, stream := range report.Streams {
				if stream.Kind == StreamImage {
					imageCount++
				}
			}
			if imageCount != 1 {
				t.Fatalf("Image streams = %d, want 1", imageCount)
			}
			if got := report.General.JSON["Cover"] != ""; got != test.wantCover {
				t.Fatalf("Cover present = %v, want %v: %#v", got, test.wantCover, report.General.JSON)
			}
			if test.wantCover && (report.General.JSON["Cover_Description"] != test.description || report.General.JSON["Cover_Type"] != "Cover") {
				t.Fatalf("cover fields = %#v", report.General.JSON)
			}
		})
	}
}

func parseSingleMatroskaAttachment(t *testing.T, name, description, mime string, data []byte) matroskaAttachment {
	t.Helper()
	fields := buildMatroskaElement(mkvIDFileData, data)
	fields = append(fields, buildMatroskaElement(mkvIDFileName, []byte(name))...)
	if description != "" {
		fields = append(fields, buildMatroskaElement(mkvIDFileDescription, []byte(description))...)
	}
	if mime != "" {
		fields = append(fields, buildMatroskaElement(mkvIDFileMimeType, []byte(mime))...)
	}
	fields = append(fields, buildMatroskaElement(mkvIDFileUID, encodeMatroskaUint(7))...)
	attachments := parseMatroskaAttachments(buildMatroskaElement(mkvIDAttachedFile, fields))
	if len(attachments) != 1 {
		t.Fatalf("attachments = %#v", attachments)
	}
	if attachments[0].uid != 7 || attachments[0].description != description || attachments[0].size != int64(len(data)) {
		t.Fatalf("attachment metadata = %#v", attachments[0])
	}
	return attachments[0]
}

func writeMatroskaAttachmentFixture(t *testing.T, name, description string, data []byte) string {
	t.Helper()
	fields := buildMatroskaElement(mkvIDFileData, data)
	fields = append(fields, buildMatroskaElement(mkvIDFileName, []byte(name))...)
	fields = append(fields, buildMatroskaElement(mkvIDFileMimeType, []byte("image/png"))...)
	if description != "" {
		fields = append(fields, buildMatroskaElement(mkvIDFileDescription, []byte(description))...)
	}
	fields = append(fields, buildMatroskaElement(mkvIDFileUID, encodeMatroskaUint(7))...)
	attachments := buildMatroskaElement(mkvIDAttachments, buildMatroskaElement(mkvIDAttachedFile, fields))
	segment := append(buildMatroskaInfo(), buildMatroskaTracks()...)
	segment = append(segment, attachments...)
	file := buildMatroskaElement(mkvIDEBML, nil)
	file = append(file, buildMatroskaElement(mkvIDSegment, segment)...)
	path := filepath.Join(t.TempDir(), "attachment.mkv")
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}
