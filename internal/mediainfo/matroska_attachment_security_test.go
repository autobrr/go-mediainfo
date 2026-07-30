package mediainfo

import (
	"bytes"
	"encoding/binary"
	"io"
	"strconv"
	"testing"
)

type sparseRecordingReaderAt struct {
	data    []byte
	maxRead int
}

type partialReadFailureReaderAt struct {
	data       []byte
	failOffset int64
	failed     bool
}

var benchmarkEmbeddedAssetBudgetSink embeddedAssetBudget

func (r *sparseRecordingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if len(p) > r.maxRead {
		r.maxRead = len(p)
	}
	if off < 0 || off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n != len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *partialReadFailureReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if !r.failed && off == r.failOffset && len(p) > int(embeddedAssetMaxImageProbe) {
		r.failed = true
		n := len(p) / 2
		copy(p[:n], r.data[off:])
		return n, io.ErrUnexpectedEOF
	}
	if off < 0 || off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n != len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestScanMatroskaAttachmentsRejectsOversizedStringsBeforeRead(t *testing.T) {
	for _, tt := range []struct {
		name    string
		fieldID uint64
	}{
		{name: "name", fieldID: mkvIDFileName},
		{name: "mime", fieldID: mkvIDFileMimeType},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const declaredSize = uint64(64 << 20)
			fieldHeader := append(buildMatroskaID(tt.fieldID), buildMatroskaSize(declaredSize)...)
			childSize := uint64(len(fieldHeader)) + declaredSize
			childHeader := append(buildMatroskaID(mkvIDAttachedFile), buildMatroskaSize(childSize)...)
			elementSize := uint64(len(childHeader)) + childSize
			elementHeader := append(buildMatroskaID(mkvIDAttachments), buildMatroskaSize(elementSize)...)
			stored := append([]byte{0}, elementHeader...)
			stored = append(stored, childHeader...)
			stored = append(stored, fieldHeader...)
			logicalSize := int64(1+len(elementHeader)+len(childHeader)+len(fieldHeader)) + int64(declaredSize)
			reader := &sparseRecordingReaderAt{data: stored}

			attachments := scanMatroskaAttachmentsFromFile(reader, 1, logicalSize, &embeddedAssetBudget{})
			if len(attachments) != 0 {
				t.Fatalf("oversized %s produced attachments: %#v", tt.name, attachments)
			}
			if reader.maxRead == 0 {
				t.Fatalf("oversized %s did not enter lazy scanner", tt.name)
			}
			if reader.maxRead > 256<<10 {
				t.Fatalf("oversized %s requested %d bytes", tt.name, reader.maxRead)
			}
		})
	}
}

func TestScanMatroskaAttachmentsHonorsItemBudget(t *testing.T) {
	payload := make([]byte, 0, int(embeddedAssetMaxItems+1)*32)
	for i := range embeddedAssetMaxItems + 1 {
		name := []byte("cover-" + strconv.FormatInt(i, 10) + ".jpg")
		attachedFile := buildMatroskaElement(mkvIDFileName, name)
		payload = append(payload, buildMatroskaElement(mkvIDAttachedFile, attachedFile)...)
	}
	element := buildMatroskaElement(mkvIDAttachments, payload)
	file := append([]byte{0}, element...)
	attachments := scanMatroskaAttachmentsFromFile(bytes.NewReader(file), 1, int64(len(file)), &embeddedAssetBudget{})
	if got := int64(len(attachments)); got != embeddedAssetMaxItems {
		t.Fatalf("attachments = %d, want %d", got, embeddedAssetMaxItems)
	}
}

func TestScanMatroskaAttachmentsAcceptsExactNameLimit(t *testing.T) {
	name := bytes.Repeat([]byte{'a'}, int(embeddedAssetMaxNameBytes))
	attachedFile := buildMatroskaElement(mkvIDFileName, name)
	element := buildMatroskaElement(mkvIDAttachments, buildMatroskaElement(mkvIDAttachedFile, attachedFile))
	file := append([]byte{0}, element...)
	attachments := scanMatroskaAttachmentsFromFile(bytes.NewReader(file), 1, int64(len(file)), &embeddedAssetBudget{})
	if len(attachments) != 1 || len(attachments[0].name) != len(name) {
		t.Fatalf("exact-limit attachment = %#v", attachments)
	}
}

func TestScanMatroskaAttachmentsRejectsEscapingChild(t *testing.T) {
	child := append(buildMatroskaID(mkvIDAttachedFile), buildMatroskaSize(1024)...)
	element := buildMatroskaElement(mkvIDAttachments, child)
	file := append([]byte{0}, element...)
	if attachments := scanMatroskaAttachmentsFromFile(bytes.NewReader(file), 1, int64(len(file)), &embeddedAssetBudget{}); len(attachments) != 0 {
		t.Fatalf("escaping child produced attachments: %#v", attachments)
	}
}

func TestScanMatroskaAttachmentsRejectsUnknownChildSize(t *testing.T) {
	child := append(buildMatroskaID(mkvIDAttachedFile), 0xFF)
	if attachments := parseMatroskaAttachments(child); len(attachments) != 0 {
		t.Fatalf("initial unknown-size child produced attachments: %#v", attachments)
	}
	element := buildMatroskaElement(mkvIDAttachments, child)
	file := append([]byte{0}, element...)
	if attachments := scanMatroskaAttachmentsFromFile(bytes.NewReader(file), 1, int64(len(file)), &embeddedAssetBudget{}); len(attachments) != 0 {
		t.Fatalf("unknown-size child produced attachments: %#v", attachments)
	}
}

func TestScanMatroskaAttachmentsRejectsUnknownContainerSize(t *testing.T) {
	element := append(buildMatroskaID(mkvIDAttachments), 0xFF)
	element = append(element, buildMatroskaElement(mkvIDAttachedFile, buildMatroskaElement(mkvIDFileName, []byte("cover.png")))...)
	if info, _ := parseMatroskaSegmentWithBudget(element, &embeddedAssetBudget{}); len(info.attachments) != 0 {
		t.Fatalf("initial unknown-size container produced attachments: %#v", info.attachments)
	}
	file := append([]byte{0}, element...)
	if attachments := scanMatroskaAttachmentsFromFile(bytes.NewReader(file), 1, int64(len(file)), &embeddedAssetBudget{}); len(attachments) != 0 {
		t.Fatalf("unknown-size container produced attachments: %#v", attachments)
	}
}

func TestScanMatroskaAttachmentsAcceptsDataBeforeIdentity(t *testing.T) {
	image := minimalPNG(16, 9, 2)
	attachedFile := append(buildMatroskaElement(mkvIDFileData, image), buildMatroskaElement(mkvIDFileName, []byte("cover.png"))...)
	attachedFile = append(attachedFile, buildMatroskaElement(mkvIDFileMimeType, []byte("image/png"))...)
	element := buildMatroskaElement(mkvIDAttachments, buildMatroskaElement(mkvIDAttachedFile, attachedFile))
	file := append([]byte{0}, element...)
	attachments := scanMatroskaAttachmentsFromFile(bytes.NewReader(file), 1, int64(len(file)), &embeddedAssetBudget{})
	if len(attachments) != 1 || !attachments[0].complete || !bytes.Equal(attachments[0].data, image) {
		t.Fatalf("data-before-identity attachment = %#v", attachments)
	}
}

func TestScanMatroskaAttachmentsRollsBackPayloadBudgetAfterPartialRead(t *testing.T) {
	failedImage := append(minimalPNG(16, 9, 2), make([]byte, int(embeddedAssetMaxImageProbe))...)
	validImage := minimalPNG(32, 18, 2)
	failedFields := append(buildMatroskaElement(mkvIDFileName, []byte("failed.png")), buildMatroskaElement(mkvIDFileData, failedImage)...)
	validFields := append(buildMatroskaElement(mkvIDFileName, []byte("valid.png")), buildMatroskaElement(mkvIDFileData, validImage)...)
	payload := append(buildMatroskaElement(mkvIDAttachedFile, failedFields), buildMatroskaElement(mkvIDAttachedFile, validFields)...)
	file := append([]byte{0}, buildMatroskaElement(mkvIDAttachments, payload)...)
	failOffset := int64(bytes.Index(file, failedImage))
	if failOffset < 0 {
		t.Fatal("failed attachment payload not found")
	}

	reader := &partialReadFailureReaderAt{data: file, failOffset: failOffset}
	initialRetained := embeddedAssetMaxRetainedBytes - int64(len(failedImage))
	budget := &embeddedAssetBudget{retainedBytes: initialRetained}
	attachments := scanMatroskaAttachmentsFromFile(reader, 1, int64(len(file)), budget)
	if !reader.failed {
		t.Fatal("full attachment read did not fail")
	}

	var valid *matroskaAttachment
	for i := range attachments {
		if attachments[i].name == "valid.png" {
			valid = &attachments[i]
			break
		}
	}
	if valid == nil || !valid.complete || !bytes.Equal(valid.data, validImage) {
		t.Fatalf("valid attachment after partial read = %#v", valid)
	}
	if want := initialRetained + int64(len(validImage)); budget.retainedBytes != want {
		t.Fatalf("retained payload bytes = %d, want %d", budget.retainedBytes, want)
	}
}

func TestParseMatroskaAttachmentsDefersPayloadUntilIdentity(t *testing.T) {
	image := minimalPNG(16, 9, 2)
	imageFields := append(buildMatroskaElement(mkvIDFileData, image), buildMatroskaElement(mkvIDFileName, []byte("cover.png"))...)
	font := bytes.Repeat([]byte{'f'}, 1024)
	fontFields := append(buildMatroskaElement(mkvIDFileData, font), buildMatroskaElement(mkvIDFileName, []byte("subtitle.ttf"))...)
	payload := append(buildMatroskaElement(mkvIDAttachedFile, imageFields), buildMatroskaElement(mkvIDAttachedFile, fontFields)...)
	attachments := parseMatroskaAttachmentsWithBudget(payload, &embeddedAssetBudget{})
	if len(attachments) != 2 || !bytes.Equal(attachments[0].data, image) {
		t.Fatalf("image payload was not retained after late identity: %#v", attachments)
	}
	if len(attachments[1].data) != 0 || attachments[1].size != int64(len(font)) {
		t.Fatalf("unsupported payload was retained: %#v", attachments[1])
	}
}

func TestParseJPEGAttachmentAcceptsSegmentEndingAtPayloadBoundary(t *testing.T) {
	data := []byte{
		0xFF, 0xD8,
		0xFF, 0xC0, 0x00, 0x0B,
		0x08, 0x00, 0x10, 0x00, 0x10, 0x01, 0x01, 0x11, 0x00,
	}

	width, height, depth, _, _, _, _, ok := parseJPEGAttachment(data)
	if !ok || width != 16 || height != 16 || depth != 8 {
		t.Fatalf("parseJPEGAttachment() = %dx%d depth %d, ok %v", width, height, depth, ok)
	}
}

func BenchmarkParseJPEGAttachmentAPP2(b *testing.B) {
	for _, size := range []int{64 << 10, 1 << 20, 2 << 20, 4 << 20} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			data := buildJPEGAPP2BenchmarkData(size)
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, _, _, _, _, _, _ = parseJPEGAttachment(data)
			}
		})
	}
}

func BenchmarkParsePNGAttachmentMetadataMalformedChunks(b *testing.B) {
	for _, size := range []int{64 << 10, 1 << 20, 2 << 20, 4 << 20} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			data := buildMalformedPNGMetadataBenchmarkData(size)
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, _ = parsePNGAttachmentMetadata(data)
			}
		})
	}
}

func BenchmarkParseMatroskaAttachmentSets(b *testing.B) {
	for _, count := range []int{1, 16, 128, 256, 257} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			payload := make([]byte, 0, count*32)
			for i := range count {
				name := buildMatroskaElement(mkvIDFileName, []byte("font-"+strconv.Itoa(i)+".ttf"))
				payload = append(payload, buildMatroskaElement(mkvIDAttachedFile, name)...)
			}
			file := append([]byte{0}, buildMatroskaElement(mkvIDAttachments, payload)...)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = scanMatroskaAttachmentsFromFile(bytes.NewReader(file), 1, int64(len(file)), &embeddedAssetBudget{})
			}
		})
	}
}

func BenchmarkEmbeddedAssetBudgetRetainedPayload(b *testing.B) {
	for _, total := range []int64{4 << 20, 16 << 20, 32 << 20, 33 << 20} {
		b.Run(strconv.FormatInt(total>>20, 10)+"MiB", func(b *testing.B) {
			b.SetBytes(total)
			for i := 0; i < b.N; i++ {
				budget := &embeddedAssetBudget{}
				for remaining := total; remaining > 0; remaining -= embeddedAssetMaxPayloadBytes {
					chunk := min(remaining, embeddedAssetMaxPayloadBytes)
					_ = budget.reservePayload(uint64(chunk), embeddedAssetMaxPayloadBytes)
				}
				benchmarkEmbeddedAssetBudgetSink = *budget
			}
		})
	}
}

func buildJPEGAPP2BenchmarkData(target int) []byte {
	data := []byte{0xFF, 0xD8}
	payload := []byte("ICC_PROFILE")
	for len(data)+4+len(payload)+15 <= target {
		data = append(data, 0xFF, 0xE2)
		var length [2]byte
		binary.BigEndian.PutUint16(length[:], uint16(len(payload)+2))
		data = append(data, length[:]...)
		data = append(data, payload...)
	}
	data = append(data,
		0xFF, 0xC0, 0x00, 0x0B,
		0x08, 0x00, 0x10, 0x00, 0x10, 0x01, 0x01, 0x11, 0x00,
		0xFF, 0xD9,
	)
	return data
}

func buildMalformedPNGMetadataBenchmarkData(target int) []byte {
	data := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	for len(data)+24 <= target {
		data = append(data, 0, 0, 0, 0, 't', 'E', 'X', 't', 0, 0, 0, 0)
	}
	var oversized [8]byte
	binary.BigEndian.PutUint32(oversized[0:4], ^uint32(0))
	copy(oversized[4:8], "iCCP")
	return append(data, oversized[:]...)
}
