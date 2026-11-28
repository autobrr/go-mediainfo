package mediainfo

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

func TestParseMP4BoundsFileTypeBrands(t *testing.T) {
	const retainedBrands = 256
	const maxPayloadRead = 8 + retainedBrands*4
	var movie, file bytes.Buffer
	writeMP4Box(&file, "free", nil)
	writeMP4Box(&file, "ftyp", []byte("isom\x00\x00\x00\x00"+strings.Repeat("mp41", retainedBrands)+"mp42"))
	writeMP4Box(&file, "ftyp", []byte("qt  \x00\x00\x00\x00qt  "))
	writeMP4Box(&movie, "trak", buildTrackWithStsd("vide", "avc1"))
	writeMP4Box(&file, "moov", movie.Bytes())
	reader := &sparseRecordingReaderAt{data: file.Bytes()}
	info, ok := ParseMP4(reader, int64(file.Len()))
	if !ok || len(info.Tracks) != 1 || info.Tracks[0].Format != "AVC" {
		t.Fatalf("expected the video track after ftyp, got %v", info.Tracks)
	}
	if reader.maxRead != maxPayloadRead {
		t.Errorf("largest read = %d, want %d", reader.maxRead, maxPayloadRead)
	}
	codecID := findField(info.General, "Codec ID")
	if len(info.General) != 2 || codecID != "isom ("+strings.Repeat("mp41/", retainedBrands-1)+"mp41)" {
		t.Errorf("expected one file type with %d compatible brands, got %d general fields and %d mp41 entries", retainedBrands, len(info.General), strings.Count(codecID, "mp41"))
	}
}

func TestParseMP4Duration(t *testing.T) {
	var buf bytes.Buffer
	writeMP4Box(&buf, "ftyp", []byte{'i', 's', 'o', 'm'})
	mvhd := make([]byte, 20)
	mvhd[0] = 0
	binary.BigEndian.PutUint32(mvhd[12:16], 1000)
	binary.BigEndian.PutUint32(mvhd[16:20], 10000)
	var moov bytes.Buffer
	writeMP4Box(&moov, "mvhd", mvhd)
	writeMP4Box(&buf, "moov", moov.Bytes())

	file, err := os.CreateTemp(t.TempDir(), "sample-*.mp4")
	if err != nil {
		t.Fatalf("temp: %v", err)
	}
	if _, err := file.Write(buf.Bytes()); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	stat, err := os.Stat(file.Name())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	f, err := os.Open(file.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	info, ok := ParseMP4(f, stat.Size())
	if !ok {
		t.Fatalf("expected mp4 info")
	}
	if info.Container.DurationSeconds != 10 {
		t.Fatalf("duration=%v", info.Container.DurationSeconds)
	}
}

func TestParseMP4RejectsOversizedExtendedBox(t *testing.T) {
	data := []byte("\x00\x00\x00\x01ftypisom\x00\x00\x02\x00isommp42")
	if _, ok := ParseMP4(bytes.NewReader(data), int64(len(data))); ok {
		t.Fatal("oversized extended box was accepted")
	}
}

func writeMP4Box(buf *bytes.Buffer, typ string, payload []byte) {
	size := uint32(8 + len(payload))
	if err := binary.Write(buf, binary.BigEndian, size); err != nil {
		panic(err)
	}
	buf.WriteString(typ)
	buf.Write(payload)
}
