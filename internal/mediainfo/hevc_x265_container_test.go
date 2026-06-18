package mediainfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatroskaHEVCX265CodecPrivatePropagatesToOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x265-hvcc.mkv")
	if err := os.WriteFile(path, buildMatroskaHEVCX265Sample(), 0o644); err != nil {
		t.Fatalf("write matroska fixture: %v", err)
	}

	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}

	stream := requireVideoStream(t, report)
	requireX265Fields(t, stream)
	requireX265JSON(t, report)
}

func TestMPEGTSHEVCX265AnnexBPropagatesToOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x265-annexb.ts")
	if err := os.WriteFile(path, buildMPEGTSHEVCX265Sample(), 0o644); err != nil {
		t.Fatalf("write ts fixture: %v", err)
	}

	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}

	stream := requireVideoStream(t, report)
	requireX265Fields(t, stream)
	requireX265JSON(t, report)
}

func requireVideoStream(t *testing.T, report Report) Stream {
	t.Helper()
	for _, stream := range report.Streams {
		if stream.Kind == StreamVideo {
			return stream
		}
	}
	t.Fatalf("missing video stream: %+v", report.Streams)
	return Stream{}
}

func requireX265Fields(t *testing.T, stream Stream) {
	t.Helper()
	if got := findField(stream.Fields, "Format"); got != "HEVC" {
		t.Fatalf("Format = %q, want HEVC", got)
	}
	if got := findField(stream.Fields, "Writing library"); got != "x265 9.9" {
		t.Fatalf("Writing library = %q, want x265 9.9", got)
	}
	if got := findField(stream.Fields, "Encoding settings"); got != "wpp / me=0" {
		t.Fatalf("Encoding settings = %q, want wpp / me=0", got)
	}
}

func requireX265JSON(t *testing.T, report Report) {
	t.Helper()
	payload := buildJSONPayload(report)
	var video jsonTrackOut
	for _, track := range payload.Media.Tracks {
		if jsonFieldValue(track.Fields, "@type") == string(StreamVideo) {
			video = track
			break
		}
	}
	if len(video.Fields) == 0 {
		t.Fatalf("missing video JSON track")
	}
	if got := jsonFieldValue(video.Fields, "Encoded_Library"); got != "x265 - 9.9" {
		t.Fatalf("Encoded_Library = %q, want x265 - 9.9", got)
	}
	if got := jsonFieldValue(video.Fields, "Encoded_Library_Name"); got != "x265" {
		t.Fatalf("Encoded_Library_Name = %q, want x265", got)
	}
	if got := jsonFieldValue(video.Fields, "Encoded_Library_Version"); got != "9.9" {
		t.Fatalf("Encoded_Library_Version = %q, want 9.9", got)
	}
	if got := jsonFieldValue(video.Fields, "Encoded_Library_Settings"); got != "wpp / me=0" {
		t.Fatalf("Encoded_Library_Settings = %q, want wpp / me=0", got)
	}
	requireJSONOrder(t, video.Fields, "Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version", "Encoded_Library_Settings")
}

func requireJSONOrder(t *testing.T, fields []jsonKV, keys ...string) {
	t.Helper()
	last := -1
	for _, key := range keys {
		found := -1
		for i, field := range fields {
			if field.Key == key {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("missing JSON key %q in %+v", key, fields)
		}
		if found <= last {
			t.Fatalf("JSON key %q at %d, want after %d", key, found, last)
		}
		last = found
	}
}

func buildMatroskaHEVCX265Sample() []byte {
	track := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	track = append(track, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(1))...)
	track = append(track, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1))...)
	track = append(track, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEGH/ISO/HEVC"))...)
	track = append(track, buildMatroskaElement(mkvIDCodecPrivate, buildHEVCConfigWithX265SEI())...)
	track = append(track, buildMatroskaVideoSettings(320, 240)...)
	tracks := buildMatroskaElement(mkvIDTracks, buildMatroskaElement(mkvIDTrackEntry, track))

	cluster := mkvClusterWithSimpleBlock(mkvBlockNoLace([]byte{0}))
	segment := append(buildMatroskaInfo(), tracks...)
	segment = append(segment, cluster...)
	return append(buildMatroskaElement(mkvIDEBML, buildMatroskaElement(mkvIDDocType, []byte("matroska"))), buildMatroskaElement(mkvIDSegment, segment)...)
}

func buildMPEGTSHEVCX265Sample() []byte {
	const (
		pmtPID   = uint16(0x0100)
		videoPID = uint16(0x0101)
	)
	data := make([]byte, 0, 188*5)
	data = append(data, tsPacket(0, true, buildPATSection(pmtPID))...)
	data = append(data, tsPacket(pmtPID, true, buildPMTSection(videoPID))...)
	pes := append([]byte{0x00, 0x00, 0x01, 0xE0, 0x00, 0x00, 0x80, 0x00, 0x00}, buildAnnexBX265SEI()...)
	data = append(data, tsPacket(videoPID, true, pes)...)
	data = append(data, tsPacket(0x1FFF, false, nil)...)
	data = append(data, tsPacket(0x1FFF, false, nil)...)
	return data
}

func buildHEVCConfigWithX265SEI() []byte {
	nal := buildX265SEINAL()
	cfg := make([]byte, 23)
	cfg[1] = 0x01           // Main profile
	cfg[12] = 0x78          // Level 4
	cfg[16] = 0x01          // 4:2:0
	cfg[21] = 0x00          // one-byte NAL lengths in samples
	cfg[22] = 0x01          // numOfArrays
	cfg = append(cfg, 0x27) // array_completeness/type: NAL_unit_type 39
	cfg = append(cfg, 0x00, 0x01)
	cfg = append(cfg, byte(len(nal)>>8), byte(len(nal)))
	cfg = append(cfg, nal...)
	return cfg
}

func buildAnnexBX265SEI() []byte {
	out := []byte{0x00, 0x00, 0x01}
	out = append(out, buildX265SEINAL()...)
	return out
}

func buildX265SEINAL() []byte {
	uuid := []byte{0x2C, 0xA2, 0xDE, 0x09, 0xB5, 0x17, 0x47, 0xDB, 0xBB, 0x55, 0xA4, 0xFE, 0x7F, 0xC2, 0xFC, 0x4E}
	text := "x265 (build 1) - 9.9 - H.265/HEVC codec - c - u - options: wpp 320 bitdepth=8 fps=2 me=0"
	payload := append(append([]byte{}, uuid...), []byte(text)...)
	if len(payload) > 254 {
		panic("x265 SEI test payload too large")
	}
	nal := []byte{0x4E, 0x01, 0x05, byte(len(payload))}
	nal = append(nal, payload...)
	return nal
}

func buildPATSection(pmtPID uint16) []byte {
	section := []byte{
		0x00,
		0x00, 0x01,
		0x00, 0x01,
		0xC1,
		0x00,
		0x00,
		0x00, 0x01,
		byte(0xE0 | (pmtPID >> 8)), byte(pmtPID),
		0x00, 0x00, 0x00, 0x00,
	}
	sectionLen := len(section) - 3
	section[1] = 0xB0 | byte(sectionLen>>8)
	section[2] = byte(sectionLen)
	return append([]byte{0x00}, section...)
}

func buildPMTSection(videoPID uint16) []byte {
	section := []byte{
		0x02,
		0x00, 0x01,
		0x00, 0x01,
		0xC1,
		0x00,
		0x00,
		byte(0xE0 | (videoPID >> 8)), byte(videoPID),
		0xF0, 0x00,
		0x24, byte(0xE0 | (videoPID >> 8)), byte(videoPID), 0xF0, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	sectionLen := len(section) - 3
	section[1] = 0xB0 | byte(sectionLen>>8)
	section[2] = byte(sectionLen)
	return append([]byte{0x00}, section...)
}

func tsPacket(pid uint16, payloadStart bool, payload []byte) []byte {
	packet := make([]byte, 188)
	for i := range packet {
		packet[i] = 0xFF
	}
	packet[0] = 0x47
	packet[1] = byte(pid >> 8)
	if payloadStart {
		packet[1] |= 0x40
	}
	packet[2] = byte(pid)
	packet[3] = 0x10
	copy(packet[4:], payload)
	return packet
}
