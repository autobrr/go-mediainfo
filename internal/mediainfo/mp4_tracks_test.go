package mediainfo

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

func TestParseMP4Tracks(t *testing.T) {
	var buf bytes.Buffer
	writeMP4Box(&buf, "ftyp", []byte{'i', 's', 'o', 'm'})
	mvhd := make([]byte, 20)
	mvhd[0] = 0
	binary.BigEndian.PutUint32(mvhd[12:16], 1000)
	binary.BigEndian.PutUint32(mvhd[16:20], 10000)

	trak := buildTrack("vide")
	var moov bytes.Buffer
	writeMP4Box(&moov, "mvhd", mvhd)
	writeMP4Box(&moov, "trak", trak)
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
	if len(info.Tracks) != 1 {
		t.Fatalf("expected 1 track")
	}
	if info.Tracks[0].Kind != StreamVideo {
		t.Fatalf("expected video track")
	}
}

func buildTrack(handler string) []byte {
	var mdia bytes.Buffer
	payload := make([]byte, 20)
	copy(payload[8:12], []byte(handler))
	writeMP4Box(&mdia, "hdlr", payload)

	var trak bytes.Buffer
	writeMP4Box(&trak, "mdia", mdia.Bytes())
	return trak.Bytes()
}
