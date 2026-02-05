package mediainfo

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

func TestParseMP4CodecFromStsd(t *testing.T) {
	var buf bytes.Buffer
	writeMP4Box(&buf, "ftyp", []byte{'i', 's', 'o', 'm'})
	mvhd := make([]byte, 20)
	mvhd[0] = 0
	binary.BigEndian.PutUint32(mvhd[12:16], 1000)
	binary.BigEndian.PutUint32(mvhd[16:20], 10000)

	trak := buildTrackWithStsd("vide", "avc1")
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
	if info.Tracks[0].Format != "AVC" {
		t.Fatalf("format=%q", info.Tracks[0].Format)
	}
	if findField(info.Tracks[0].Fields, "Width") == "" {
		t.Fatalf("missing width")
	}
	if info.Tracks[0].SampleCount == 0 || info.Tracks[0].DurationSeconds == 0 {
		t.Fatalf("missing timing data")
	}
}

func buildTrackWithStsd(handler, sample string) []byte {
	var stsd bytes.Buffer
	stsd.Write([]byte{0x00, 0x00, 0x00, 0x00})
	if err := binary.Write(&stsd, binary.BigEndian, uint32(1)); err != nil {
		panic(err)
	}
	entry := make([]byte, 86)
	binary.BigEndian.PutUint32(entry[0:4], uint32(len(entry)))
	copy(entry[4:8], []byte(sample))
	binary.BigEndian.PutUint16(entry[32:34], 1920)
	binary.BigEndian.PutUint16(entry[34:36], 1080)
	stsd.Write(entry)

	var stbl bytes.Buffer
	writeMP4Box(&stbl, "stsd", stsd.Bytes())
	writeMP4Box(&stbl, "stts", buildSttsBox())

	var minf bytes.Buffer
	writeMP4Box(&minf, "stbl", stbl.Bytes())

	var mdia bytes.Buffer
	writeMP4Box(&mdia, "mdhd", buildMdhdBox())
	payload := make([]byte, 20)
	copy(payload[8:12], []byte(handler))
	writeMP4Box(&mdia, "hdlr", payload)
	writeMP4Box(&mdia, "minf", minf.Bytes())

	var trak bytes.Buffer
	writeMP4Box(&trak, "mdia", mdia.Bytes())
	return trak.Bytes()
}

func buildMdhdBox() []byte {
	payload := make([]byte, 24)
	payload[0] = 0
	binary.BigEndian.PutUint32(payload[12:16], 90000)
	binary.BigEndian.PutUint32(payload[16:20], 900000)
	return payload
}

func buildSttsBox() []byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint32(buf[4:8], 1)
	binary.BigEndian.PutUint32(buf[8:12], 300)
	binary.BigEndian.PutUint32(buf[12:16], 3000)
	return buf
}
