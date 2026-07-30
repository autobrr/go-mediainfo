package mediainfo

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestParseAVIRejectsShortListChunk(t *testing.T) {
	data := []byte("RIFF0000AVI LIST\x00\x00\x00\x00hdrl")
	if _, _, _, ok := ParseAVI(bytes.NewReader(data), int64(len(data))); ok {
		t.Fatal("short LIST chunk parsed as AVI")
	}
}

func TestParseAVIIndex(t *testing.T) {
	streams := []*aviStream{
		{index: 0, kind: StreamVideo},
		{index: 1, kind: StreamAudio},
	}
	entry := func(id string, size uint32) []byte {
		buf := make([]byte, 16)
		copy(buf[0:4], []byte(id))
		binary.LittleEndian.PutUint32(buf[12:16], size)
		return buf
	}
	data := append(entry("00dc", 1200), entry("01wb", 400)...)
	data = append(data, entry("JUNK", 999)...)

	if ok, _ := parseAVIIndex(data, streams); !ok {
		t.Fatalf("expected index parse to find stream entries")
	}
	if streams[0].bytes != 1200 {
		t.Fatalf("unexpected stream 0 bytes: %d", streams[0].bytes)
	}
	if streams[1].bytes != 400 {
		t.Fatalf("unexpected stream 1 bytes: %d", streams[1].bytes)
	}
	if streams[0].packetCount != 1 || streams[1].packetCount != 1 {
		t.Fatalf("unexpected packet counts: video=%d audio=%d", streams[0].packetCount, streams[1].packetCount)
	}
}

func TestParseAVIIndexNoEntries(t *testing.T) {
	streams := []*aviStream{{index: 0}}
	if ok, _ := parseAVIIndex([]byte("short"), streams); ok {
		t.Fatalf("expected no index entries")
	}
	if streams[0].bytes != 0 {
		t.Fatalf("unexpected bytes update: %d", streams[0].bytes)
	}
}

func TestParseAVIIndexCountsPayloadPadding(t *testing.T) {
	streams := []*aviStream{{index: 0, kind: StreamAudio}}
	entry := make([]byte, 16)
	copy(entry[:4], "00wb")
	binary.LittleEndian.PutUint32(entry[12:16], 385)

	if ok, _ := parseAVIIndex(entry, streams); !ok {
		t.Fatal("expected audio index entry")
	}
	if streams[0].bytes != 385 || streams[0].paddingBytes != 1 {
		t.Fatalf("payload accounting = %d bytes + %d padding", streams[0].bytes, streams[0].paddingBytes)
	}
}

func TestParseAVILAMEInfo(t *testing.T) {
	data := make([]byte, 96)
	copy(data[0:4], "Xing")
	binary.BigEndian.PutUint32(data[4:8], 3)
	binary.BigEndian.PutUint32(data[8:12], 56889)
	binary.BigEndian.PutUint32(data[12:16], 17216160)
	copy(data[40:49], "LAME3.97 ")
	data[49] = 2
	data[50] = 170
	data[60] = 128

	info := parseAVILAMEInfo(data)
	if info.library != "LAME3.97 " || info.frameCount != 56889 || info.vbrBytes != 17216160 || !info.tagged {
		t.Fatalf("LAME identity/counts = %+v", info)
	}
	if !info.variable || info.targetBitRate != 128000 || info.settings != "-m j -V 4 -q 2 -lowpass 17 --abr 128" {
		t.Fatalf("LAME settings = %+v", info)
	}
}

func TestParseAVILAMEInfoRequiresXingTagForMethodFacts(t *testing.T) {
	data := append([]byte("LAME3.100"), bytes.Repeat([]byte{0xAA}, 24)...)
	info := parseAVILAMEInfo(data)
	if info.library != "LAME3.100" {
		t.Fatalf("LAME identity = %q", info.library)
	}
	if info.tagged || info.variable || info.targetBitRate != 0 || info.frameCount != 0 {
		t.Fatalf("untagged LAME facts = %+v", info)
	}
}

func TestFindFirstMP3HeaderAtRequiresFollowingFrame(t *testing.T) {
	valid := []byte{0xFF, 0xFB, 0x94, 0x64}
	header, ok := parseMP3Header(valid)
	if !ok {
		t.Fatal("test MPEG audio header is invalid")
	}
	frameLength := mp3FrameLengthBytes(header)
	data := make([]byte, 16+frameLength+4)
	copy(data[:4], valid)
	copy(data[16:20], valid)
	copy(data[16+frameLength:16+frameLength+4], valid)

	_, offset, ok := findFirstMP3HeaderAt(data)
	if !ok || offset != 16 {
		t.Fatalf("validated MPEG audio offset = %d, ok=%v", offset, ok)
	}
}

func TestAVIMP3DelayUsesStreamRate(t *testing.T) {
	delay := aviMP3DelaySeconds(&aviStream{rate: 48000}, 960)
	if math.Abs(delay-0.020) > 1e-9 {
		t.Fatalf("delay = %f", delay)
	}
}

func TestAVIAudioAlignment(t *testing.T) {
	if got := aviAudioAlignment(&aviStream{audioTag: 0x2000}); got != "Aligned" {
		t.Fatalf("AC-3 alignment = %q", got)
	}
	if got := aviAudioAlignment(&aviStream{audioTag: 0x2000, delayBytes: 1536}); got != "Split" {
		t.Fatalf("delayed AC-3 alignment = %q", got)
	}
	if got := aviAudioAlignment(&aviStream{audioTag: 0x0055}); got != "Split" {
		t.Fatalf("unsized MPEG audio alignment = %q", got)
	}
}
