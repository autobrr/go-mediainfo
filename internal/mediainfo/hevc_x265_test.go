package mediainfo

import (
	"strings"
	"testing"
)

const x265RealInfo = "x265 (build 216) - 4.2+1-e444744:[Mac OS X][clang 21.0.0][64 bit] 8bit+10bit+12bit - H.265/HEVC codec - Copyright 2013-2018 (c) Multicoreware, Inc - http://x265.org - options: cpuid=98 frame-threads=3 wpp bitdepth=8 fps=24/1 input-res=320x240 deblock=0:0 scenecut-aware-qp=0conformance-window-offsets"

const x265WantLibrary = "x265 4.2+1-e444744:[Mac OS X][clang 21.0.0][64 bit] 8bit+10bit+12bit"
const x265WantSettings = "cpuid=98 / frame-threads=3 / wpp / input-res=320x240 / deblock=0:0 / scenecut-aware-qp=0conformance-window-offsets"

func TestParseX265InfoString(t *testing.T) {
	var info hevcHDRInfo
	parseX265InfoString([]byte(x265RealInfo), &info)

	if !info.x265Seen {
		t.Fatalf("x265Seen = false, want true")
	}
	if info.x265Library != x265WantLibrary {
		t.Fatalf("x265Library:\n got %q\nwant %q", info.x265Library, x265WantLibrary)
	}
	// bitdepth=, fps=, and digit-leading tokens are dropped; the x265 no-space bug
	// token survives intact.
	if info.x265Settings != x265WantSettings {
		t.Fatalf("x265Settings:\n got %q\nwant %q", info.x265Settings, x265WantSettings)
	}
}

func TestParseX265InfoStringTrailingNUL(t *testing.T) {
	// A single trailing NUL is tolerated (matches MediaInfo Peek_String behaviour).
	var info hevcHDRInfo
	parseX265InfoString(append([]byte(x265RealInfo), 0x00), &info)
	if !info.x265Seen || info.x265Library != x265WantLibrary {
		t.Fatalf("trailing NUL not tolerated: seen=%v lib=%q", info.x265Seen, info.x265Library)
	}

	// An embedded NUL that is not the final byte means the payload is not text.
	var info2 hevcHDRInfo
	parseX265InfoString([]byte("x265 - 9.9\x00 - options: wpp x"), &info2)
	if info2.x265Seen {
		t.Fatalf("embedded NUL payload should be ignored")
	}
}

func TestParseHEVCUserDataUnregistered(t *testing.T) {
	uuid := []byte{0x2C, 0xA2, 0xDE, 0x09, 0xB5, 0x17, 0x47, 0xDB, 0xBB, 0x55, 0xA4, 0xFE, 0x7F, 0xC2, 0xFC, 0x4E}
	payload := append(append([]byte{}, uuid...), []byte(x265RealInfo)...)

	var info hevcHDRInfo
	parseHEVCUserDataUnregistered(payload, &info)
	if info.x265Library != x265WantLibrary {
		t.Fatalf("x265Library = %q, want %q", info.x265Library, x265WantLibrary)
	}

	// A different (non-x265) UUID must be ignored even if the body looks like text.
	wrongUUID := append([]byte{0x42, 0x7F, 0xCC, 0x9B, 0xB8, 0x92, 0x48, 0x21}, uuid[8:]...)
	payload2 := append(append([]byte{}, wrongUUID...), []byte(x265RealInfo)...)
	var info2 hevcHDRInfo
	parseHEVCUserDataUnregistered(payload2, &info2)
	if info2.x265Seen {
		t.Fatalf("non-x265 UUID should be ignored")
	}
}

func TestParseHEVCUserDataUnregisteredATEME(t *testing.T) {
	uuid := []byte{0x42, 0x7F, 0xCC, 0x9B, 0xB8, 0x92, 0x48, 0x21, 0, 0, 0, 0, 0, 0, 0, 0}
	payload := append(uuid, []byte("ATEME Titan KFE 3.7.3 (4.7.3.1001)\x00")...)
	var info hevcHDRInfo
	parseHEVCUserDataUnregistered(payload, &info)
	if info.encoderLibrary != "ATEME Titan KFE 3.7.3 (4.7.3.1001)" || info.encoderName != "ATEME Titan KFE" || info.encoderVersion != "3.7.3 (4.7.3.1001)" {
		t.Fatalf("unexpected ATEME metadata: %+v", info)
	}
	wrongUUID := append(make([]byte, 16), payload[16:]...)
	var ignored hevcHDRInfo
	parseHEVCUserDataUnregistered(wrongUUID, &ignored)
	if ignored.encoderLibrary != "" {
		t.Fatalf("ATEME text with unrelated UUID was accepted: %+v", ignored)
	}
}

func TestParseHEVCConfigSEI(t *testing.T) {
	// Some muxers place the x265 user-data SEI in the hvcC NAL arrays. Build a minimal
	// HEVCDecoderConfigurationRecord with one PREFIX_SEI (nal_unit_type 39) array.
	uuid := []byte{0x2C, 0xA2, 0xDE, 0x09, 0xB5, 0x17, 0x47, 0xDB, 0xBB, 0x55, 0xA4, 0xFE, 0x7F, 0xC2, 0xFC, 0x4E}
	text := "x265 (build 1) - 9.9 - H.265/HEVC codec - c - u - options: wpp 320 bitdepth=8 fps=2 me=0"
	body := append(append([]byte{}, uuid...), []byte(text)...)
	if len(body) > 254 {
		t.Fatalf("test SEI payload too large for single-byte size: %d", len(body))
	}
	nal := append([]byte{0x4E, 0x01, 0x05, byte(len(body))}, body...) // PREFIX_SEI, payloadType 5

	cfg := make([]byte, 23)
	cfg[22] = 1             // numOfArrays
	cfg = append(cfg, 0x27) // array_completeness/type: NAL_unit_type 39
	cfg = append(cfg, 0x00, 0x01)
	cfg = append(cfg, byte(len(nal)>>8), byte(len(nal)))
	cfg = append(cfg, nal...)

	var info hevcHDRInfo
	parseHEVCConfigSEI(cfg, &info)
	if !info.x265Seen {
		t.Fatalf("x265 SEI not extracted from hvcC arrays")
	}
	if info.x265Library != "x265 9.9" {
		t.Fatalf("x265Library = %q, want %q", info.x265Library, "x265 9.9")
	}
	if info.x265Settings != "wpp / me=0" {
		t.Fatalf("x265Settings = %q, want %q", info.x265Settings, "wpp / me=0")
	}
}

func TestSplitEncodedLibraryX265(t *testing.T) {
	name, version := splitEncodedLibrary("x265 - 4.2+1-e444744:[Mac OS X] 8bit")
	if name != "x265" || version != "4.2+1-e444744:[Mac OS X] 8bit" {
		t.Fatalf("splitEncodedLibrary x265 = (%q,%q)", name, version)
	}
	// Bare name with no version yields an empty version (MediaInfo leaves it unset).
	if n, v := splitEncodedLibrary("x265"); n != "x265" || v != "" {
		t.Fatalf("splitEncodedLibrary bare = (%q,%q), want (x265, \"\")", n, v)
	}
}

func TestMapStreamFieldsToJSONX265EncodedLibrary(t *testing.T) {
	fields := []Field{
		{Name: "Writing library", Value: x265WantLibrary},
		{Name: "Encoding settings", Value: x265WantSettings},
	}
	got := mapStreamFieldsToJSON(StreamVideo, fields)

	if v := jsonFieldValue(got, "Encoded_Library"); v != "x265 - 4.2+1-e444744:[Mac OS X][clang 21.0.0][64 bit] 8bit+10bit+12bit" {
		t.Fatalf("Encoded_Library = %q", v)
	}
	if v := jsonFieldValue(got, "Encoded_Library_Name"); v != "x265" {
		t.Fatalf("Encoded_Library_Name = %q", v)
	}
	if v := jsonFieldValue(got, "Encoded_Library_Version"); v != "4.2+1-e444744:[Mac OS X][clang 21.0.0][64 bit] 8bit+10bit+12bit" {
		t.Fatalf("Encoded_Library_Version = %q", v)
	}
	if v := jsonFieldValue(got, "Encoded_Library_Settings"); v != x265WantSettings {
		t.Fatalf("Encoded_Library_Settings = %q", v)
	}
}

func TestApplyMatroskaVideoProbesX265OutranksGenericEncoder(t *testing.T) {
	var hdr hevcHDRInfo
	atemePayload := append([]byte{0x42, 0x7F, 0xCC, 0x9B, 0xB8, 0x92, 0x48, 0x21, 0, 0, 0, 0, 0, 0, 0, 0}, []byte("ATEME Titan KFE 3.7.3 (4.7.3.1001)\x00")...)
	parseHEVCUserDataUnregistered(atemePayload, &hdr)
	x265Payload := make([]byte, 0, 16+len(x265RealInfo))
	x265Payload = append(x265Payload, 0x2C, 0xA2, 0xDE, 0x09, 0xB5, 0x17, 0x47, 0xDB, 0xBB, 0x55, 0xA4, 0xFE, 0x7F, 0xC2, 0xFC, 0x4E)
	x265Payload = append(x265Payload, x265RealInfo...)
	parseHEVCUserDataUnregistered(x265Payload, &hdr)
	hdr.timeCode = "01:02:03:04"

	stream := Stream{Kind: StreamVideo}
	replaceCanonicalSeedFill(&stream, "ID", "1", "ID", "1")
	replaceCanonicalSeedFill(&stream, "Encoded_Library", "container encoder", "Writing library", "container encoder")
	replaceCanonicalSeedFill(&stream, "Encoded_Library_Name", "container", "", "")
	replaceCanonicalSeedFill(&stream, "Encoded_Library_Version", "1", "", "")
	replaceCanonicalSeedFill(&stream, "Encoded_Library_Settings", "container settings", "Encoding settings", "container settings")
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaVideoProbes(&info, map[uint64]*matroskaVideoProbe{1: {codec: "HEVC", hdrInfo: hdr}})

	stream = info.Tracks[0]
	if got := matroskaStreamDisplay(stream, "Writing library"); got != x265WantLibrary {
		t.Fatalf("Writing library = %q, want %q", got, x265WantLibrary)
	}
	if got := matroskaStreamDisplay(stream, "Encoding settings"); got != x265WantSettings {
		t.Fatalf("Encoding settings = %q, want %q", got, x265WantSettings)
	}
	if got, _ := canonicalSeedValue(stream, "Encoded_Library"); got != "x265 - "+strings.TrimPrefix(x265WantLibrary, "x265 ") {
		t.Fatalf("Encoded_Library = %q", got)
	}
	if got, _ := canonicalSeedValue(stream, "Encoded_Library_Name"); got != "x265" {
		t.Fatalf("Encoded_Library_Name = %q, want x265", got)
	}
	if got, _ := canonicalSeedValue(stream, "Encoded_Library_Version"); got != "4.2+1-e444744:[Mac OS X][clang 21.0.0][64 bit] 8bit+10bit+12bit" {
		t.Fatalf("Encoded_Library_Version = %q, stale container version survived", got)
	}
	if got, _ := canonicalSeedValue(stream, "Encoded_Library_Settings"); got != x265WantSettings {
		t.Fatalf("Encoded_Library_Settings = %q, want %q", got, x265WantSettings)
	}
	if got, _ := canonicalSeedValue(stream, "TimeCode_FirstFrame"); got != "01:02:03:04" {
		t.Fatalf("TimeCode_FirstFrame = %q, want independent HEVC time code", got)
	}
}

func TestApplyMatroskaVideoProbesGenericEncoderWithoutX265(t *testing.T) {
	hdr := hevcHDRInfo{
		encoderLibrary: "ATEME Titan KFE 3.7.3",
		encoderName:    "ATEME Titan KFE",
		encoderVersion: "3.7.3",
		timeCode:       "01:02:03:04",
	}
	stream := Stream{Kind: StreamVideo}
	replaceCanonicalSeedFill(&stream, "ID", "1", "ID", "1")
	info := MatroskaInfo{Tracks: []Stream{stream}}

	applyMatroskaVideoProbes(&info, map[uint64]*matroskaVideoProbe{1: {codec: "HEVC", hdrInfo: hdr}})

	stream = info.Tracks[0]
	if got := matroskaStreamDisplay(stream, "Writing library"); got != hdr.encoderLibrary {
		t.Fatalf("Writing library = %q, want generic %q", got, hdr.encoderLibrary)
	}
	if got, _ := canonicalSeedValue(stream, "Encoded_Library"); got != hdr.encoderLibrary {
		t.Fatalf("Encoded_Library = %q, want generic %q", got, hdr.encoderLibrary)
	}
	if got, _ := canonicalSeedValue(stream, "TimeCode_FirstFrame"); got != hdr.timeCode {
		t.Fatalf("TimeCode_FirstFrame = %q, want %q", got, hdr.timeCode)
	}
}
