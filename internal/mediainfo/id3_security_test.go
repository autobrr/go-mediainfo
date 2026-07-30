package mediainfo

import (
	"bytes"
	"io"
	"testing"
)

func TestParseID3RejectsTruncatedInBudgetTagBeforeAllocation(t *testing.T) {
	const tagSize = uint32(1 << 20)
	header := []byte{'I', 'D', '3', 0x04, 0x00, 0x00,
		byte(tagSize >> 21 & 0x7F), byte(tagSize >> 14 & 0x7F), byte(tagSize >> 7 & 0x7F), byte(tagSize & 0x7F)}
	reader := bytes.NewReader(header)
	allocations := 0
	id3, ok := parseID3v2WithAllocator(reader, int64(len(header)), &embeddedAssetBudget{}, func(size int) []byte {
		allocations++
		return make([]byte, size)
	})
	if !ok {
		t.Fatal("ID3 header was not recognized")
	}
	if id3.Offset != int64(10+synchsafe32(header[6:10])) {
		t.Fatalf("offset = %d", id3.Offset)
	}
	if pos, err := reader.Seek(0, io.SeekCurrent); err != nil || pos != int64(len(header)) {
		t.Fatalf("reader position = %d, %v", pos, err)
	}
	if allocations != 0 {
		t.Fatalf("payload allocations = %d, want 0", allocations)
	}
}

func TestParseID3RejectsOverBudgetTagBeforeRead(t *testing.T) {
	const tagSize = uint32(40 << 20)
	header := []byte{'I', 'D', '3', 0x04, 0x00, 0x00,
		byte(tagSize >> 21 & 0x7F), byte(tagSize >> 14 & 0x7F), byte(tagSize >> 7 & 0x7F), byte(tagSize & 0x7F)}
	reader := bytes.NewReader(header)
	allocations := 0
	id3, ok := parseID3v2WithAllocator(reader, int64(10+tagSize), &embeddedAssetBudget{}, func(size int) []byte {
		allocations++
		return make([]byte, size)
	})
	if !ok || id3.Offset != int64(10+tagSize) {
		t.Fatalf("id3 = %#v, ok=%v", id3, ok)
	}
	if pos, err := reader.Seek(0, io.SeekCurrent); err != nil || pos != int64(10+tagSize) {
		t.Fatalf("reader position = %d, %v", pos, err)
	}
	if allocations != 0 {
		t.Fatalf("payload allocations = %d, want 0", allocations)
	}
}

func TestParseID3AcceptedTagUsesObservedAllocation(t *testing.T) {
	header := make([]byte, 0, 20)
	header = append(header, 'I', 'D', '3', 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0A)
	reader := bytes.NewReader(append(header, make([]byte, 10)...))
	allocations := 0
	id3, ok := parseID3v2WithAllocator(reader, int64(reader.Len()), &embeddedAssetBudget{}, func(size int) []byte {
		allocations++
		return make([]byte, size)
	})
	if !ok || id3.Offset != 20 {
		t.Fatalf("id3 = %#v, ok=%v", id3, ok)
	}
	if allocations != 1 {
		t.Fatalf("payload allocations = %d, want 1", allocations)
	}
}

func TestParseID3APICRejectsOversizedMIME(t *testing.T) {
	data := append([]byte{0x03}, bytes.Repeat([]byte{'a'}, int(embeddedAssetMaxMIMEBytes+1))...)
	data = append(data, 0, 0x03, 0, 0xFF, 0xD8, 0xFF)
	budget := &embeddedAssetBudget{}
	if picture, ok := parseID3APIC(data, budget); ok {
		t.Fatalf("oversized MIME produced picture %#v", picture)
	}
	if budget.stringBytes != 0 || budget.retainedBytes != 0 {
		t.Fatalf("failed APIC consumed budget: %#v", budget)
	}
}
