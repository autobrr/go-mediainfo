package mediainfo

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
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
		{"Ogg", parseOggFLACMD5TestStream},
		{"MPEG-4", parseMP4FLACMD5TestStream},
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
				for _, row := range []string{"Format : FLAC", "Bit depth : 16 bits"} {
					if !strings.Contains(strings.Join(strings.Fields(outputs["text"]), " "), row) {
						t.Errorf("text lacks %q:\n%s", row, outputs["text"])
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

// parseOggFLACMD5TestStream wraps private in the Ogg FLAC mapping: a 0x7F
// "FLAC" first packet, a VORBIS_COMMENT packet, and one audio page.
func parseOggFLACMD5TestStream(t *testing.T, private []byte) Stream {
	t.Helper()
	data := buildOggFLACForTest(private, "Lavf63.1")
	_, streams, _, _, _, ok := parseOgg(bytes.NewReader(data), int64(len(data)))
	if !ok || len(streams) != 1 {
		t.Fatal("Ogg FLAC did not parse")
	}
	if got := findField(streams[0].Fields, "Writing library"); got != "Lavf63.1" {
		t.Errorf("Writing library = %q, want Lavf63.1", got)
	}
	return streams[0]
}

// buildOggFLACForTest wraps private (an fLaC metadata sequence) in the Ogg
// FLAC mapping: a 0x7F "FLAC" first packet, a VORBIS_COMMENT packet with
// vendor and tags, and one audio page that ends at granule 4096.
func buildOggFLACForTest(private []byte, vendor string, tags ...string) []byte {
	first := append([]byte("\x7fFLAC\x01\x00\x00\x01"), private...)
	first[13] &^= 0x80 // a VORBIS_COMMENT block follows
	comment := binary.LittleEndian.AppendUint32(nil, uint32(len(vendor)))
	comment = append(comment, vendor...)
	comment = binary.LittleEndian.AppendUint32(comment, uint32(len(tags)))
	for _, tag := range tags {
		comment = binary.LittleEndian.AppendUint32(comment, uint32(len(tag)))
		comment = append(comment, tag...)
	}
	comment = append([]byte{0x84, 0, 0, byte(len(comment))}, comment...)
	data := buildOggPageForTest(7, 0x02, 0, first)
	data = append(data, buildOggPageForTest(7, 0x00, 0, comment)...)
	return append(data, buildOggPageForTest(7, 0x04, 4096, make([]byte, 64))...)
}

// parseMP4FLACMD5TestStream analyzes private wrapped by buildMP4FLACForTest.
func parseMP4FLACMD5TestStream(t *testing.T, private []byte) Stream {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flac.mp4")
	if err := os.WriteFile(path, buildMP4FLACForTest(private, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range report.Streams {
		if stream.Kind == StreamAudio {
			return stream
		}
	}
	t.Fatal("MP4 FLAC has no audio stream")
	return Stream{}
}

// buildMP4FLACForTest wraps the STREAMINFO block of private (an fLaC
// metadata sequence) in the ISO FLAC mapping: an fLaC sample entry with a dfLa box.
// A positive mediaTime adds an edit list that starts at that media time.
func buildMP4FLACForTest(private []byte, mediaTime uint32) []byte {
	var dfLa bytes.Buffer
	writeMP4Box(&dfLa, "dfLa", append(make([]byte, 4), private[4:]...))
	entry := make([]byte, 36)
	copy(entry[4:8], "fLaC")
	binary.BigEndian.PutUint16(entry[24:26], 2)
	binary.BigEndian.PutUint32(entry[32:36], 44100<<16)
	entry = append(entry, dfLa.Bytes()...)
	binary.BigEndian.PutUint32(entry[0:4], uint32(len(entry)))
	stsd := append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, entry...)

	var stbl, minf, mdia, trak, moov, file bytes.Buffer
	writeMP4Box(&stbl, "stsd", stsd)
	writeMP4Box(&stbl, "stts", buildSttsBox())
	writeMP4Box(&minf, "stbl", stbl.Bytes())
	writeMP4Box(&mdia, "mdhd", buildMdhdBox())
	handler := make([]byte, 20)
	copy(handler[8:12], "soun")
	writeMP4Box(&mdia, "hdlr", handler)
	writeMP4Box(&mdia, "minf", minf.Bytes())
	if mediaTime > 0 {
		var edts bytes.Buffer
		elst := make([]byte, 20)
		binary.BigEndian.PutUint32(elst[4:8], 1)
		binary.BigEndian.PutUint32(elst[8:12], 9000)
		binary.BigEndian.PutUint32(elst[12:16], mediaTime)
		binary.BigEndian.PutUint32(elst[16:20], 0x00010000)
		writeMP4Box(&edts, "elst", elst)
		writeMP4Box(&trak, "edts", edts.Bytes())
	}
	writeMP4Box(&trak, "mdia", mdia.Bytes())
	mvhd := make([]byte, 20)
	binary.BigEndian.PutUint32(mvhd[12:16], 1000)
	binary.BigEndian.PutUint32(mvhd[16:20], 10000)
	writeMP4Box(&moov, "mvhd", mvhd)
	writeMP4Box(&moov, "trak", trak.Bytes())
	writeMP4Box(&file, "ftyp", []byte("isom"))
	writeMP4Box(&file, "moov", moov.Bytes())
	return file.Bytes()
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

func TestParseFLACMetadataBlocksReadsVorbisMetadata(t *testing.T) {
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

	info, gotVendor, ok := parseFLACMetadataBlocks(private)
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

// TestContainerFLACSamplingCountFollowsContainer checks that SamplingCount
// comes from container timing, not from STREAMINFO. A cut remux keeps the
// total sample count of its full-length source in STREAMINFO.
func TestContainerFLACSamplingCountFollowsContainer(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	private := append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...)
	for name, test := range map[string]struct {
		parse func(*testing.T, []byte) Stream
		want  string
	}{
		"Ogg":    {parseOggFLACMD5TestStream, "4096"},   // last granule position
		"MPEG-4": {parseMP4FLACMD5TestStream, "480000"}, // 10 s at 48 kHz
	} {
		if got, _ := canonicalSeedValue(test.parse(t, private), "SamplingCount"); got != test.want {
			t.Errorf("%s SamplingCount = %q, want %q", name, got, test.want)
		}
	}
}

// TestMP4FLACKeepsMD5WithSourceDelay checks that an edit list does not drop
// MD5_Unencoded from extra. Official v23.04 puts it after Source_Delay.
func TestMP4FLACKeepsMD5WithSourceDelay(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	private := append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...)
	path := filepath.Join(t.TempDir(), "flac.mp4")
	if err := os.WriteFile(path, buildMP4FLACForTest(private, 4500), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range report.Streams {
		if stream.Kind != StreamAudio {
			continue
		}
		want := `{"Source_Delay":"-50","Source_Delay_Source":"Container","MD5_Unencoded":"864D55F003143D8BAD47D3B997FAE64C"}`
		if node := canonicalSeedStructuredNode(&stream, "extra"); node == nil || structuredNodeText(*node) != want {
			t.Fatalf("extra = %v, want %s", node, want)
		}
		return
	}
	t.Fatal("MP4 FLAC has no audio stream")
}

// TestOggFLACReadsChannelMaskTag checks that Ogg FLAC applies the
// WAVEFORMATEXTENSIBLE_CHANNEL_MASK comment as standalone FLAC does. A zero
// mask omits the derived layout.
func TestOggFLACReadsChannelMaskTag(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	private := append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...)
	data := buildOggFLACForTest(private, "", "WAVEFORMATEXTENSIBLE_CHANNEL_MASK=0x0")
	_, streams, _, _, _, ok := parseOgg(bytes.NewReader(data), int64(len(data)))
	if !ok || len(streams) != 1 {
		t.Fatal("Ogg FLAC did not parse")
	}
	if got, found := canonicalSeedValue(streams[0], "ChannelLayout"); found {
		t.Fatalf("ChannelLayout = %q, want omitted for a zero mask", got)
	}
}

// TestOggFLACGeneralDeliberateDifferences pins two deliberate differences
// from official v23.04 for Ogg FLAC (see AGENTS.md). General keeps the
// ENCODER and TITLE comments, as for standalone FLAC and Ogg Vorbis. General
// has no codec Format/Info, because the General format is Ogg, not FLAC.
func TestOggFLACGeneralDeliberateDifferences(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	private := append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...)
	path := filepath.Join(t.TempDir(), "flac.oga")
	if err := os.WriteFile(path, buildOggFLACForTest(private, "Lavf63.1", "ENCODER=Lavf63.1", "TITLE=Mix"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"Writing application": "Lavf63.1", "Title": "Mix", "Format/Info": ""} {
		if got := findField(report.General.Fields, name); got != want {
			t.Errorf("General %s = %q, want %q", name, got, want)
		}
	}
}

// TestFLACDefaultChannelLayout pins the default masks that MediaInfoLib's
// File_Flac applies when no WAVEFORMATEXTENSIBLE_CHANNEL_MASK is present.
func TestFLACDefaultChannelLayout(t *testing.T) {
	for channels, want := range map[uint8][2]string{
		3: {"L R C", "Front: L C R"},
		4: {"L R Ls Rs", "Front: L R, Side: L R"},
		5: {"L R C Ls Rs", "Front: L C R, Side: L R"},
		6: {"L R C LFE Ls Rs", "Front: L C R, Side: L R, LFE"},
		7: {"L R C LFE Cb Ls Rs", "Front: L C R, Side: L R, Back: C, LFE"},
		8: {"L R C LFE Lb Rb Ls Rs", "Front: L C R, Side: L R, Back: L R, LFE"},
	} {
		stream := canonicalFLACAudioStream(flacAudioStreamParams{channels: channels})
		if got, _ := canonicalSeedValue(stream, "ChannelLayout"); got != want[0] {
			t.Errorf("%d channels: ChannelLayout = %q, want %q", channels, got, want[0])
		}
		if got, _ := canonicalSeedValue(stream, "ChannelPositions"); got != want[1] {
			t.Errorf("%d channels: ChannelPositions = %q, want %q", channels, got, want[1])
		}
	}
}

// FuzzParseFLACMetadataBlocks verifies that bare STREAMINFO, complete fLaC
// metadata, and truncated metadata blocks remain bounded for arbitrary input.
func FuzzParseFLACMetadataBlocks(f *testing.F) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		f.Fatal(err)
	}
	complete := append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...)
	f.Add(streamInfo)
	f.Add(complete)
	f.Add([]byte("fLaC\x00\x00\x00\x22\x10"))

	f.Fuzz(func(_ *testing.T, data []byte) {
		parseFLACMetadataBlocks(data)
	})
}

// TestMP4FLACSamplingCountIsExact checks that SamplingCount comes from the
// exact track duration. Official v23.04 rounds the duration to milliseconds
// first and reports 4080 for these 4096 samples (see AGENTS.md).
func TestMP4FLACSamplingCountIsExact(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	data := buildMP4FLACForTest(append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...), 0)
	mdhd := bytes.Index(data, []byte("mdhd")) + 4
	binary.BigEndian.PutUint32(data[mdhd+12:], 48000)
	binary.BigEndian.PutUint32(data[mdhd+16:], 4096)
	stts := bytes.Index(data, []byte("stts")) + 4
	binary.BigEndian.PutUint32(data[stts+8:], 1)
	binary.BigEndian.PutUint32(data[stts+12:], 4096)
	path := filepath.Join(t.TempDir(), "flac.mp4")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range report.Streams {
		if stream.Kind == StreamAudio {
			if got, _ := canonicalSeedValue(stream, "SamplingCount"); got != "4096" {
				t.Fatalf("SamplingCount = %q, want 4096", got)
			}
			return
		}
	}
	t.Fatal("MP4 FLAC has no audio stream")
}

// TestMP4FLACReadsVorbisCommentBlock checks that a dfLa box with a
// VORBIS_COMMENT block after STREAMINFO applies its vendor, channel mask, and
// VALID_BITS, as standalone and Ogg FLAC do.
func TestMP4FLACReadsVorbisCommentBlock(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	vendor := "reference libFLAC 1.4.3 20230623"
	comment := binary.LittleEndian.AppendUint32(nil, uint32(len(vendor)))
	comment = append(comment, vendor...)
	comment = binary.LittleEndian.AppendUint32(comment, 2)
	for _, tag := range []string{"WAVEFORMATEXTENSIBLE_CHANNEL_MASK=0x600", "VALID_BITS=12"} {
		comment = binary.LittleEndian.AppendUint32(comment, uint32(len(tag)))
		comment = append(comment, tag...)
	}
	private := append([]byte("fLaC\x00\x00\x00\x22"), streamInfo...)
	private = append(private, 0x84, 0, 0, byte(len(comment)))
	stream := parseMP4FLACMD5TestStream(t, append(private, comment...))
	for key, want := range map[fieldName]string{
		"ChannelLayout":           "Ls Rs",
		"ChannelPositions":        "Side: L R",
		"BitDepth_Detected":       "12",
		"Encoded_Library":         vendor,
		"Encoded_Library_Name":    "libFLAC",
		"Encoded_Library_Version": "1.4.3",
		"Encoded_Library_Date":    "2023-06-23",
	} {
		if got, _ := canonicalSeedValue(stream, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// TestOggTheoraFLACVideoBitRateIsMeasured checks that FLAC bytes are not
// counted as video. FLAC has no nominal bit rate, so the video bit rate comes
// from the measured Theora payload. Official v23.04 shows no video bit rate
// here (see AGENTS.md).
func TestOggTheoraFLACVideoBitRateIsMeasured(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	theora := make([]byte, 42)
	copy(theora, "\x80theora\x03\x02\x01")
	theora[16], theora[19] = 16, 16 // 16x16 picture
	theora[25], theora[29] = 25, 1  // 25 fps
	theora[32], theora[35] = 1, 1   // square pixels
	theora[41] = 0xC0               // granule shift 6
	data := buildOggPageForTest(1, 0x02, 0, theora)
	data = append(data, buildOggPageForTest(2, 0x02, 0, append([]byte("\x7fFLAC\x01\x00\x00\x00fLaC\x80\x00\x00\x22"), streamInfo...))...)
	for page := range 40 {
		data = append(data, buildOggPageForTest(1, 0, uint64(page+1)*5/4<<6, make([]byte, 250))...)
		data = append(data, buildOggPageForTest(2, 0, uint64(page+1)*2400, make([]byte, 250))...)
	}
	_, streams, _, _, _, ok := parseOgg(bytes.NewReader(data), int64(len(data)))
	if !ok || len(streams) != 2 || streams[0].Kind != StreamVideo {
		t.Fatal("Ogg Theora and FLAC did not parse")
	}
	// (42 + 40*250 bytes) * 8 / 2 s
	if got, _ := canonicalSeedValue(streams[0], "BitRate"); got != "40168" {
		t.Fatalf("video BitRate = %q, want 40168", got)
	}
}

// TestOggFLACSplitsLibFLACVendor checks that Ogg FLAC splits a libFLAC
// vendor into name, version, and date, as standalone and MP4 FLAC do.
func TestOggFLACSplitsLibFLACVendor(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	data := buildOggFLACForTest(append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...), "reference libFLAC 1.4.3 20230623")
	_, streams, _, _, _, ok := parseOgg(bytes.NewReader(data), int64(len(data)))
	if !ok || len(streams) != 1 {
		t.Fatal("Ogg FLAC did not parse")
	}
	for key, want := range map[fieldName]string{
		"Encoded_Library_Name":    "libFLAC",
		"Encoded_Library_Version": "1.4.3",
		"Encoded_Library_Date":    "2023-06-23",
	} {
		if got, _ := canonicalSeedValue(streams[0], key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// TestMP4FLACZeroChannelMaskOmitsLayout checks that a dfLa VORBIS_COMMENT
// with WAVEFORMATEXTENSIBLE_CHANNEL_MASK=0x0 omits the derived layout and
// positions, as standalone and Ogg FLAC do.
func TestMP4FLACZeroChannelMaskOmitsLayout(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	tag := "WAVEFORMATEXTENSIBLE_CHANNEL_MASK=0x0"
	comment := binary.LittleEndian.AppendUint32(nil, 0)
	comment = binary.LittleEndian.AppendUint32(comment, 1)
	comment = binary.LittleEndian.AppendUint32(comment, uint32(len(tag)))
	comment = append(comment, tag...)
	private := append([]byte("fLaC\x00\x00\x00\x22"), streamInfo...)
	private = append(private, 0x84, 0, 0, byte(len(comment)))
	stream := parseMP4FLACMD5TestStream(t, append(private, comment...))
	for _, key := range []fieldName{"ChannelLayout", "ChannelPositions"} {
		if got, found := canonicalSeedValue(stream, key); found {
			t.Errorf("%s = %q, want omitted for a zero mask", key, got)
		}
	}
}

// TestMP4FLACReadsDfLaAtChildBoxBoundaries checks that the fLaC sample entry
// finds dfLa by its child-box walk. A "dfLa" byte sequence inside a preceding
// free box must not win, and a dfLa with a 64-bit largesize must be read.
func TestMP4FLACReadsDfLaAtChildBoxBoundaries(t *testing.T) {
	block := func(streamInfo string) []byte {
		raw, err := hex.DecodeString(streamInfo)
		if err != nil {
			t.Fatal(err)
		}
		// FullBox version and flags, then a last STREAMINFO block.
		return append([]byte{0, 0, 0, 0, 0x80, 0, 0, 0x22}, raw...)
	}
	stored := block("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	decoy := block("1000100000001000210c01f400f00e5b6540864d55f003143d8bad47d3b997fae64c")
	header := make([]byte, 36)
	copy(header[4:8], "fLaC")
	binary.BigEndian.PutUint16(header[24:26], 2)
	binary.BigEndian.PutUint32(header[32:36], 44100<<16)

	var decoyBox, free, extended bytes.Buffer
	writeMP4Box(&decoyBox, "dfLa", decoy)
	writeMP4Box(&free, "free", decoyBox.Bytes())
	writeMP4Box(&free, "dfLa", stored)
	extended.Write([]byte{0, 0, 0, 1})
	extended.WriteString("dfLa")
	extended.Write(binary.BigEndian.AppendUint64(nil, uint64(16+len(stored))))
	extended.Write(stored)

	for name, children := range map[string][]byte{
		"decoy in free box": free.Bytes(),
		"largesize dfLa":    extended.Bytes(),
	} {
		t.Run(name, func(t *testing.T) {
			entry := append(bytes.Clone(header), children...)
			binary.BigEndian.PutUint32(entry[0:4], uint32(len(entry)))
			fields := parseAudioSampleEntry(entry, "fLaC").Fields
			if got := findField(fields, "Sampling rate"); got != "48.0 kHz" {
				t.Errorf("Sampling rate = %q, want 48.0 kHz", got)
			}
			if got := findField(fields, "Compression mode"); got != "Lossless" {
				t.Errorf("Compression mode = %q, want Lossless", got)
			}
		})
	}
}
