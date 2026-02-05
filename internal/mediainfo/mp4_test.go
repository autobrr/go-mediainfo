package mediainfo

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

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

func writeMP4Box(buf *bytes.Buffer, typ string, payload []byte) {
	size := uint32(8 + len(payload))
	if err := binary.Write(buf, binary.BigEndian, size); err != nil {
		panic(err)
	}
	buf.WriteString(typ)
	buf.Write(payload)
}
