package mediainfo

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
)

func TestFLACUnencodedMD5Output(t *testing.T) {
	const label = "MD5 of the unencoded content"
	for _, container := range []struct {
		name  string
		parse func(*testing.T, []byte) Stream
	}{
		{"FLAC", parseStandaloneFLACMD5TestStream},
		{"Matroska", parseMatroskaFLACMD5TestStream},
	} {
		for _, checksum := range []struct{ name, digest string }{
			{"stored", "864D55F003143D8BAD47D3B997FAE64C"},
			{"leading_zero", "004D55F003143D8BAD47D3B997FAE64C"},
			{"all_zero", "00000000000000000000000000000000"},
		} {
			t.Run(container.name+"/"+checksum.name, func(t *testing.T) {
				streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
				if err != nil {
					t.Fatal(err)
				}
				md5, err := hex.DecodeString(checksum.digest)
				if err != nil {
					t.Fatal(err)
				}
				copy(streamInfo[18:], md5)
				private := append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...)
				stream := container.parse(t, private)
				reports := []Report{{
					General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: container.name}}},
					Streams: []Stream{stream},
				}}
				if got := stream.Fields[len(stream.Fields)-1]; got != (Field{Name: label, Value: checksum.digest}) {
					t.Fatalf("last audio field = %#v, want unencoded MD5 %s", got, checksum.digest)
				}
				outputs := map[string]string{
					"text": RenderText(reports),
					"raw":  RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"}),
					"JSON": RenderJSON(reports),
					"XML":  RenderXML(reports),
					"HTML": RenderHTML(reports),
					"CSV":  RenderCSV(reports),
				}
				for format, output := range outputs {
					if got := strings.Count(output, checksum.digest); got != 1 {
						t.Errorf("%s digest count = %d, want 1:\n%s", format, got, output)
					}
				}
				if got := strings.Count(outputs["text"], label); got != 1 {
					t.Errorf("text MD5 label count = %d, want 1", got)
				}
				if got := strings.Count(outputs["JSON"], "MD5_Unencoded"); got != 1 {
					t.Errorf("JSON MD5 key count = %d, want 1", got)
				}
			})
		}
	}
}

func parseStandaloneFLACMD5TestStream(t *testing.T, private []byte) Stream {
	t.Helper()
	_, streams, _, _, ok := parseFLAC(bytes.NewReader(private), int64(len(private)))
	if !ok || len(streams) != 1 {
		t.Fatal("FLAC metadata did not parse")
	}
	return streams[0]
}

func parseMatroskaFLACMD5TestStream(t *testing.T, private []byte) Stream {
	t.Helper()
	payload := buildMatroskaElement(mkvIDTrackNumber, []byte{1})
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, []byte{42})...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, []byte{2})...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_FLAC"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, private)...)
	stream, ok := parseMatroskaTrackEntry(payload, 2, 3)
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
	return stream
}

func TestParseFLACStreamInfoDetailsRejectsTruncatedMD5(t *testing.T) {
	for _, size := range []int{18, 33} {
		info, ok := parseFLACStreamInfoDetails(make([]byte, size))
		if ok || info.md5 != "" {
			t.Fatalf("%d-byte STREAMINFO produced MD5 %q, ok=%v", size, info.md5, ok)
		}
	}
}

func TestMatroskaFLACMD5FollowsDistrustedStatistics(t *testing.T) {
	stream := parseMatroskaFLACMD5TestStream(t, buildMatroskaFLACPrivateForTest(""))
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{42: {
		extras: []jsonKV{
			{Key: "FromStats_BitRate", Val: "160000"},
			{Key: "FromStats_Duration", Val: "00:00:02.500000000"},
		},
	}}, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	output := RenderText([]Report{{
		General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}},
		Streams: info.Tracks,
	}})
	md5Index := strings.Index(output, "MD5 of the unencoded content")
	if md5Index < 0 {
		t.Fatal("MD5 text row is missing")
	}
	for _, field := range []Field{
		{Name: "FromStats_BitRate", Value: "160000"},
		{Name: "FromStats_Duration", Value: "00:00:02.500000000"},
		{Name: "SOURCE", Value: "Blu-ray"},
	} {
		if got := findField(info.Tracks[0].Fields, field.Name); got != field.Value {
			t.Fatalf("%s = %q, want %q", field.Name, got, field.Value)
		}
		if index := strings.Index(output, field.Name); index < 0 || index >= md5Index {
			t.Fatalf("%s must precede MD5:\n%s", field.Name, output)
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
