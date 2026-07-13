package mediainfo

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestParseFLACStreamInfoDetails(t *testing.T) {
	data, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}

	info, ok := parseFLACStreamInfoDetails(data)
	if !ok {
		t.Fatal("expected valid STREAMINFO")
	}
	if info.minBlockSize != 4096 || info.maxBlockSize != 4096 {
		t.Fatalf("block sizes = %d/%d", info.minBlockSize, info.maxBlockSize)
	}
	if info.sampleRate != 48000 || info.channels != 2 || info.bitsPerSample != 16 {
		t.Fatalf("audio metadata = %d Hz, %d channels, %d bits", info.sampleRate, info.channels, info.bitsPerSample)
	}
	if info.totalSamples != 240870720 {
		t.Fatalf("total samples = %d", info.totalSamples)
	}
	if info.md5 != "864D55F003143D8BAD47D3B997FAE64C" {
		t.Fatalf("MD5 = %q", info.md5)
	}
}

func TestParseMatroskaFLACPrivateReadsVorbisVendor(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	vendor := []byte("reference libFLAC 1.2.1 20070917")
	comment := make([]byte, 4+len(vendor)+4)
	binary.LittleEndian.PutUint32(comment[:4], uint32(len(vendor)))
	copy(comment[4:], vendor)
	private := append([]byte("fLaC"), 0x00, 0x00, 0x00, 0x22)
	private = append(private, streamInfo...)
	private = append(private, 0x84, byte(len(comment)>>16), byte(len(comment)>>8), byte(len(comment)))
	private = append(private, comment...)

	info, gotVendor, ok := parseMatroskaFLACPrivate(private)
	if !ok {
		t.Fatal("expected valid Matroska FLAC private data")
	}
	if info.sampleRate != 48000 || info.bitsPerSample != 16 {
		t.Fatalf("unexpected STREAMINFO: %+v", info)
	}
	if gotVendor != string(vendor) {
		t.Fatalf("vendor = %q", gotVendor)
	}
}

func TestFLACDerivedLayoutIsOmitted(t *testing.T) {
	for _, test := range []struct {
		vendor string
		want   bool
	}{
		{"reference libFLAC 1.2.1 20070917", false},
		{"reference libFLAC 1.3.4 20220220", true},
		{"reference libFLAC 1.5.0 20250211", true},
		{"", true},
	} {
		if got := flacDerivedLayoutIsOmitted(test.vendor); got != test.want {
			t.Errorf("flacDerivedLayoutIsOmitted(%q) = %v, want %v", test.vendor, got, test.want)
		}
	}
}
