package mediainfo

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
)

func TestFLACUnencodedMD5Output(t *testing.T) {
	const digest = "864D55F003143D8BAD47D3B997FAE64C"
	const label = "MD5 of the unencoded content"
	for _, container := range []string{"FLAC", "Matroska"} {
		for _, hasMD5 := range []bool{true, false} {
			name := container + "/present"
			if !hasMD5 {
				name = container + "/unspecified"
			}
			t.Run(name, func(t *testing.T) {
				streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
				if err != nil {
					t.Fatal(err)
				}
				if !hasMD5 {
					clear(streamInfo[18:])
				}
				private := append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...)
				var stream Stream
				if container == "Matroska" {
					payload := buildMatroskaElement(mkvIDTrackNumber, []byte{1})
					payload = append(payload, buildMatroskaElement(mkvIDTrackUID, []byte{42})...)
					payload = append(payload, buildMatroskaElement(mkvIDTrackType, []byte{2})...)
					payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_FLAC"))...)
					payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, private)...)
					var ok bool
					stream, ok = parseMatroskaTrackEntry(payload, 2, 3)
					if !ok {
						t.Fatal("FLAC TrackEntry did not parse")
					}
					tags := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, []byte{42}))
					tags = append(tags, buildMatroskaSimpleTag("SOURCE", "Blu-ray")...)
					_, _, _, _, _, scoped := parseMatroskaTags(buildMatroskaElement(mkvIDTag, tags), "", "")
					info := MatroskaInfo{Tracks: []Stream{stream}, scopedTags: scoped}
					applyMatroskaTrackTags(&info)
					stream = info.Tracks[0]
					refreshCanonicalCompatibilitySnapshot(&stream)
				} else {
					_, streams, _, _, ok := parseFLAC(bytes.NewReader(private), int64(len(private)))
					if !ok || len(streams) != 1 {
						t.Fatal("FLAC metadata did not parse")
					}
					stream = streams[0]
				}
				reports := []Report{{
					General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: container}}},
					Streams: []Stream{stream},
				}}
				wantCount := 0
				if hasMD5 {
					wantCount = 1
					if got := stream.Fields[len(stream.Fields)-1]; got != (Field{Name: label, Value: digest}) {
						t.Fatalf("last audio field = %#v, want unencoded MD5", got)
					}
				}
				for format, output := range map[string]string{
					"text": RenderText(reports),
					"raw":  RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"}),
					"JSON": RenderJSON(reports),
				} {
					if got := strings.Count(output, digest); got != wantCount {
						t.Errorf("%s digest count = %d, want %d:\n%s", format, got, wantCount, output)
					}
				}
				if got := strings.Contains(RenderText(reports), label); got != hasMD5 {
					t.Errorf("text MD5 label present = %v, want %v", got, hasMD5)
				}
				if got := strings.Contains(RenderJSON(reports), "MD5_Unencoded"); got != hasMD5 {
					t.Errorf("JSON MD5 key present = %v, want %v", got, hasMD5)
				}
			})
		}
	}
}

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

func TestParseMatroskaFLACPrivateReadsVorbisMetadata(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	vendor := []byte("reference libFLAC 1.2.1 20070917")
	validBits := []byte("VALID_BITS=15")
	comment := make([]byte, 4+len(vendor)+4+4+len(validBits))
	binary.LittleEndian.PutUint32(comment[:4], uint32(len(vendor)))
	copy(comment[4:], vendor)
	commentCount := 4 + len(vendor)
	binary.LittleEndian.PutUint32(comment[commentCount:commentCount+4], 1)
	commentPos := commentCount + 4
	binary.LittleEndian.PutUint32(comment[commentPos:commentPos+4], uint32(len(validBits)))
	copy(comment[commentPos+4:], validBits)
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
	if info.detectedBits != 15 {
		t.Fatalf("detected bits = %d, want 15", info.detectedBits)
	}
}

func TestParseFLACDetectedBits(t *testing.T) {
	for _, test := range []struct {
		value string
		want  uint8
	}{
		{"21", 21},
		{" 20 ", 20},
		{"0", 0},
		{"invalid", 0},
	} {
		if got := parseFLACDetectedBits(test.value); got != test.want {
			t.Errorf("parseFLACDetectedBits(%q) = %d, want %d", test.value, got, test.want)
		}
	}
}

func TestParseFLACChannelMask(t *testing.T) {
	for _, test := range []struct {
		value string
		want  uint32
		ok    bool
	}{
		{"0X0", 0, true},
		{"0x3F", 0x3f, true},
		{"invalid", 0, false},
	} {
		got, ok := parseFLACChannelMask(test.value)
		if got != test.want || ok != test.ok {
			t.Errorf("parseFLACChannelMask(%q) = %#x, %v; want %#x, %v", test.value, got, ok, test.want, test.ok)
		}
	}
}

func TestFLACDerivedLayoutIsOmitted(t *testing.T) {
	for _, test := range []struct {
		info flacStreamInfo
		want bool
	}{
		{flacStreamInfo{}, false},
		{flacStreamInfo{hasChannelMask: true}, true},
		{flacStreamInfo{hasChannelMask: true, channelMask: 3}, false},
	} {
		if got := flacDerivedLayoutIsOmitted(test.info); got != test.want {
			t.Errorf("flacDerivedLayoutIsOmitted(%+v) = %v, want %v", test.info, got, test.want)
		}
	}
}

func TestFLACChannelMaskOverridesCountDerivedLayout(t *testing.T) {
	const quadBackMask = 0x33 // FL, FR, BL, BR
	if got := flacChannelLayoutFromMask(quadBackMask); got != "L R Lb Rb" {
		t.Fatalf("layout = %q, want L R Lb Rb", got)
	}
	if got := flacChannelPositionsFromMask(quadBackMask); got != "Front: L R, Back: L R" {
		t.Fatalf("positions = %q, want mask-derived back channels", got)
	}
	stream := canonicalFLACAudioStream(flacAudioStreamParams{
		channels:       4,
		channelMask:    quadBackMask,
		hasChannelMask: true,
	})
	if got, _ := canonicalSeedValue(stream, "ChannelLayout"); got != "L R Lb Rb" {
		t.Fatalf("canonical ChannelLayout = %q", got)
	}
	if got, _ := canonicalSeedValue(stream, "ChannelPositions"); got != "Front: L R, Back: L R" {
		t.Fatalf("canonical ChannelPositions = %q", got)
	}
}

// FuzzParseMatroskaFLACPrivate verifies that bare STREAMINFO, complete fLaC
// metadata, and truncated metadata blocks remain bounded for arbitrary input.
func FuzzParseMatroskaFLACPrivate(f *testing.F) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		f.Fatal(err)
	}
	complete := append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...)
	f.Add(streamInfo)
	f.Add(complete)
	f.Add([]byte("fLaC\x00\x00\x00\x22\x10"))

	f.Fuzz(func(_ *testing.T, data []byte) {
		parseMatroskaFLACPrivate(data)
	})
}
