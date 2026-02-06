package mediainfo

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestParseMatroskaTracks(t *testing.T) {
	buf := buildMatroskaSample()
	info, ok := parseMatroska(buf)
	if !ok {
		t.Fatalf("expected matroska info")
	}
	if !info.Container.HasDuration() {
		t.Fatalf("expected duration")
	}
	if len(info.Tracks) != 1 {
		t.Fatalf("expected 1 track")
	}
	if info.Tracks[0].Kind != StreamVideo {
		t.Fatalf("expected video track")
	}
	if findField(info.Tracks[0].Fields, "Width") == "" {
		t.Fatalf("missing width")
	}
	if findField(info.Tracks[0].Fields, "Frame rate") == "" {
		t.Fatalf("missing frame rate")
	}
	if findField(info.Tracks[0].Fields, "Bit rate") == "" {
		t.Fatalf("missing bit rate")
	}
}

func TestParseMatroskaStatsTags(t *testing.T) {
	tagsPayload := buildMatroskaTagForStats(123)
	encoders, stats := parseMatroskaTags(tagsPayload, "mkvmerge v82.0.0", "libebml v1.4.5 + libmatroska v1.7.1")
	if len(encoders) != 1 || encoders[0] != "Lavf60.3.100" {
		t.Fatalf("unexpected encoders: %#v", encoders)
	}
	entry := stats[123]
	if !entry.trusted {
		t.Fatalf("expected trusted stats")
	}
	if !entry.hasDataBytes || entry.dataBytes != 1048576 {
		t.Fatalf("unexpected bytes: %+v", entry)
	}
	if !entry.hasFrameCount || entry.frameCount != 1200 {
		t.Fatalf("unexpected frame count: %+v", entry)
	}
	if !entry.hasDuration || entry.durationSeconds != 50 {
		t.Fatalf("unexpected duration: %+v", entry)
	}
	if !entry.hasBitRate || entry.bitRate != 166000 {
		t.Fatalf("unexpected bitrate: %+v", entry)
	}
}

func TestApplyMatroskaTagStats(t *testing.T) {
	info := MatroskaInfo{
		Tracks: []Stream{
			{
				Kind: StreamVideo,
				Fields: []Field{
					{Name: "Format", Value: "AVC"},
					{Name: "ID", Value: "1"},
					{Name: "Width", Value: "1920 pixels"},
					{Name: "Height", Value: "1080 pixels"},
					{Name: "Frame rate", Value: "24.000 FPS"},
				},
				JSON: map[string]string{"UniqueID": "123"},
			},
		},
	}
	complete := applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{
		123: {
			trusted:         true,
			dataBytes:       1048576,
			hasDataBytes:    true,
			frameCount:      1200,
			hasFrameCount:   true,
			durationSeconds: 50,
			hasDuration:     true,
			bitRate:         166000,
			hasBitRate:      true,
		},
	}, 2*1048576)
	if !complete {
		t.Fatalf("expected complete stats coverage")
	}
	if findField(info.Tracks[0].Fields, "Stream size") == "" {
		t.Fatalf("expected stream size")
	}
	if findField(info.Tracks[0].Fields, "Duration") == "" {
		t.Fatalf("expected duration")
	}
	if findField(info.Tracks[0].Fields, "Bit rate") == "" {
		t.Fatalf("expected bitrate")
	}
	if info.Tracks[0].JSON["FrameCount"] != "1200" {
		t.Fatalf("unexpected frame count json: %#v", info.Tracks[0].JSON)
	}
}

func TestParseMatroskaTagStatsWithoutDate(t *testing.T) {
	stats, ok := parseMatroskaTagStats(map[string]string{
		"_STATISTICS_TAGS":        "BPS DURATION NUMBER_OF_FRAMES NUMBER_OF_BYTES",
		"_STATISTICS_WRITING_APP": "mkvmerge v94.0 ('Initiate') 64-bit",
		"BPS":                     "5913898",
		"DURATION":                "00:42:01.080000000",
		"NUMBER_OF_FRAMES":        "63027",
		"NUMBER_OF_BYTES":         "1863676305",
	}, "mkvmerge v94.0 ('Initiate') 64-bit", "libebml v1.4.5")
	if !ok || !stats.trusted {
		t.Fatalf("expected trusted stats, got: %+v", stats)
	}
	if !stats.hasDataBytes || !stats.hasDuration || !stats.hasFrameCount || !stats.hasBitRate {
		t.Fatalf("missing parsed stats: %+v", stats)
	}
}

func TestParseMatroskaTrackEntryHeaderStripping(t *testing.T) {
	compression := buildMatroskaElement(mkvIDContentCompression,
		append(
			buildMatroskaElement(mkvIDContentCompAlgo, encodeMatroskaUint(3)),
			buildMatroskaElement(mkvIDContentCompSettings, []byte{0x0B, 0x77})...,
		),
	)
	encoding := buildMatroskaElement(mkvIDContentEncoding, compression)
	entry := append(
		buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2)),
		buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))...,
	)
	entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("A_AC3"))...)
	entry = append(entry, buildMatroskaElement(mkvIDContentEncodings, encoding)...)

	stream, ok := parseMatroskaTrackEntry(entry, 0)
	if !ok {
		t.Fatalf("expected parsed stream")
	}
	if got := findField(stream.Fields, "Muxing mode"); got != "Header stripping" {
		t.Fatalf("unexpected muxing mode: %q", got)
	}
	if !bytes.Equal(stream.mkvHeaderStripBytes, []byte{0x0B, 0x77}) {
		t.Fatalf("unexpected header strip bytes: %#v", stream.mkvHeaderStripBytes)
	}
}

func TestParseMatroskaTrackEntryNonHeaderCompression(t *testing.T) {
	compression := buildMatroskaElement(mkvIDContentCompression,
		append(
			buildMatroskaElement(mkvIDContentCompAlgo, encodeMatroskaUint(0)),
			buildMatroskaElement(mkvIDContentCompSettings, []byte{0x01})...,
		),
	)
	encoding := buildMatroskaElement(mkvIDContentEncoding, compression)
	entry := append(
		buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2)),
		buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))...,
	)
	entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("A_AC3"))...)
	entry = append(entry, buildMatroskaElement(mkvIDContentEncodings, encoding)...)

	stream, ok := parseMatroskaTrackEntry(entry, 0)
	if !ok {
		t.Fatalf("expected parsed stream")
	}
	if got := findField(stream.Fields, "Muxing mode"); got != "" {
		t.Fatalf("unexpected muxing mode: %q", got)
	}
	if len(stream.mkvHeaderStripBytes) != 0 {
		t.Fatalf("unexpected header strip bytes: %#v", stream.mkvHeaderStripBytes)
	}
}

func TestParseMatroskaInfoDateUTC(t *testing.T) {
	base := time.Date(2001, time.January, 1, 0, 0, 0, 0, time.UTC)
	target := time.Date(2012, time.November, 28, 15, 41, 23, 0, time.UTC)
	delta := target.Sub(base).Nanoseconds()
	datePayload := make([]byte, 8)
	binary.BigEndian.PutUint64(datePayload, uint64(delta))

	infoBuf := append(buildMatroskaElement(mkvIDTimecodeScale, []byte{0x0F, 0x42, 0x40}),
		buildMatroskaElement(mkvIDDuration, []byte{0x41, 0x20, 0x00, 0x00})...)
	infoBuf = append(infoBuf, buildMatroskaElement(mkvIDDateUTC, datePayload)...)

	seg, ok := parseMatroskaInfo(infoBuf)
	if !ok {
		t.Fatalf("expected parsed segment info")
	}
	if got := findField(seg.Fields, "Encoded date"); got != "2012-11-28 15:41:23 UTC" {
		t.Fatalf("unexpected encoded date: %q", got)
	}
}

func TestApplyMatroskaAudioProbesSkipsDialnormTextFields(t *testing.T) {
	info := &MatroskaInfo{
		Tracks: []Stream{
			{
				Kind: StreamAudio,
				Fields: []Field{
					{Name: "Format", Value: "AC-3"},
					{Name: "ID", Value: "1"},
				},
				JSON: map[string]string{},
			},
		},
	}
	probes := map[uint64]*matroskaAudioProbe{
		1: {
			format: "AC-3",
			ok:     true,
			info: ac3Info{
				channels:      6,
				layout:        "L R C LFE Ls Rs",
				sampleRate:    48000,
				frameRate:     31.25,
				spf:           1536,
				bitRateKbps:   640,
				hasDialnorm:   true,
				dialnorm:      -18,
				hasCompr:      true,
				comprDB:       -1.16,
				hasComprField: true,
				comprFieldDB:  -1.16,
			},
		},
	}

	applyMatroskaAudioProbes(info, probes)
	stream := info.Tracks[0]
	if got := findField(stream.Fields, "Dialog Normalization"); got != "" {
		t.Fatalf("expected no dialog normalization field, got %q", got)
	}
	if got := findField(stream.Fields, "compr"); got != "" {
		t.Fatalf("expected no compr field, got %q", got)
	}
	if got := findField(stream.Fields, "dialnorm_Average"); got != "" {
		t.Fatalf("expected no dialnorm_Average field, got %q", got)
	}
}

func TestShouldApplyMatroskaClusterStats(t *testing.T) {
	nonEmptyTags := map[uint64]matroskaTagStats{
		1: {trusted: true, hasDataBytes: true, dataBytes: 42},
	}
	tests := []struct {
		name       string
		parseSpeed float64
		size       int64
		tagStats   map[uint64]matroskaTagStats
		complete   bool
		want       bool
	}{
		{
			name:       "full parse speed always applies",
			parseSpeed: 1,
			size:       mkvMaxScan * 10,
			tagStats:   nonEmptyTags,
			complete:   true,
			want:       false,
		},
		{
			name:       "default parse speed applies",
			parseSpeed: 0.5,
			size:       mkvMaxScan,
			tagStats:   nil,
			complete:   false,
			want:       false,
		},
		{
			name:       "large file with no tag stats applies",
			parseSpeed: 0.5,
			size:       mkvMaxScan + 1,
			tagStats:   nil,
			complete:   false,
			want:       false,
		},
		{
			name:       "large file with some tag stats applies",
			parseSpeed: 0.5,
			size:       mkvMaxScan + 1,
			tagStats:   nonEmptyTags,
			complete:   false,
			want:       false,
		},
		{
			name:       "complete tag stats applies",
			parseSpeed: 0.5,
			size:       mkvMaxScan + 1,
			tagStats:   nonEmptyTags,
			complete:   true,
			want:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldApplyMatroskaClusterStats(tc.parseSpeed, tc.size, tc.tagStats, tc.complete)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestVideoProbeNeedsSample(t *testing.T) {
	probe := &matroskaVideoProbe{}
	if !videoProbeNeedsSample(probe) {
		t.Fatalf("expected probe to need samples")
	}
	probe.exhausted = true
	if videoProbeNeedsSample(probe) {
		t.Fatalf("expected exhausted probe to stop sampling")
	}
	probe.exhausted = false
	probe.hdrInfo = hevcHDRInfo{
		hasMastering: true,
		maxCLL:       1000,
		maxFALL:      400,
		hdr10Plus:    true,
	}
	if videoProbeNeedsSample(probe) {
		t.Fatalf("expected complete probe to stop sampling")
	}
}

func TestMatroskaProbesCompleteRequiresParsedAudio(t *testing.T) {
	audio := map[uint64]*matroskaAudioProbe{
		1: {format: "AC-3"},
	}
	if matroskaProbesComplete(audio, nil) {
		t.Fatalf("expected unparsed audio probe to be incomplete")
	}

	audio[1].ok = true
	if !matroskaProbesComplete(audio, nil) {
		t.Fatalf("expected parsed AC-3 audio probe to be complete")
	}

	audio[1] = &matroskaAudioProbe{format: "E-AC-3", ok: true, collect: true}
	if matroskaProbesComplete(audio, nil) {
		t.Fatalf("expected collecting E-AC-3 probe to be incomplete")
	}

	audio[1].collect = false
	if !matroskaProbesComplete(audio, nil) {
		t.Fatalf("expected non-collecting E-AC-3 probe to be complete")
	}
}

func buildMatroskaSample() []byte {
	segment := append(
		buildMatroskaInfo(),
		buildMatroskaTracks()...,
	)
	segmentElem := append(buildMatroskaID(mkvIDSegment), buildMatroskaSize(uint64(len(segment)))...)
	segmentElem = append(segmentElem, segment...)
	return segmentElem
}

func buildMatroskaInfo() []byte {
	info := make([]byte, 0, 32)
	info = append(info, buildMatroskaElement(mkvIDTimecodeScale, []byte{0x0F, 0x42, 0x40})...)
	info = append(info, buildMatroskaElement(mkvIDDuration, []byte{0x41, 0x20, 0x00, 0x00})...)
	return buildMatroskaElement(mkvIDInfo, info)
}

func buildMatroskaTracks() []byte {
	trackEntry := buildMatroskaElement(mkvIDTrackType, []byte{0x01})
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEG4/ISO/AVC"))...)
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(41708333))...)
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDBitRate, encodeMatroskaUint(1000000))...)
	trackEntry = append(trackEntry, buildMatroskaVideoSettings(1920, 1080)...)
	trackEntry = buildMatroskaElement(mkvIDTrackEntry, trackEntry)
	return buildMatroskaElement(mkvIDTracks, trackEntry)
}

func buildMatroskaTagForStats(trackUID uint64) []byte {
	targets := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(trackUID)))
	body := append(targets, buildMatroskaSimpleTag("ENCODER", "Lavf60.3.100")...)
	body = append(body, buildMatroskaSimpleTag("_STATISTICS_TAGS", "BPS DURATION NUMBER_OF_FRAMES NUMBER_OF_BYTES")...)
	body = append(body, buildMatroskaSimpleTag("_STATISTICS_WRITING_APP", "mkvmerge v82.0.0")...)
	body = append(body, buildMatroskaSimpleTag("_STATISTICS_WRITING_DATE_UTC", "2024-01-01 12:00:00")...)
	body = append(body, buildMatroskaSimpleTag("BPS", "166000")...)
	body = append(body, buildMatroskaSimpleTag("DURATION", "00:00:50.000000000")...)
	body = append(body, buildMatroskaSimpleTag("NUMBER_OF_FRAMES", "1200")...)
	body = append(body, buildMatroskaSimpleTag("NUMBER_OF_BYTES", "1048576")...)
	return buildMatroskaElement(mkvIDTag, body)
}

func buildMatroskaSimpleTag(name, value string) []byte {
	body := buildMatroskaElement(mkvIDTagName, []byte(name))
	body = append(body, buildMatroskaElement(mkvIDTagString, []byte(value))...)
	return buildMatroskaElement(mkvIDSimpleTag, body)
}

func buildMatroskaVideoSettings(width, height uint64) []byte {
	video := make([]byte, 0, 24)
	video = append(video, buildMatroskaElement(mkvIDPixelWidth, encodeMatroskaUint(width))...)
	video = append(video, buildMatroskaElement(mkvIDPixelHeight, encodeMatroskaUint(height))...)
	return buildMatroskaElement(mkvIDTrackVideo, video)
}

func encodeMatroskaUint(value uint64) []byte {
	if value <= 0xFF {
		return []byte{byte(value)}
	}
	if value <= 0xFFFF {
		return []byte{byte(value >> 8), byte(value)}
	}
	if value <= 0xFFFFFF {
		return []byte{byte(value >> 16), byte(value >> 8), byte(value)}
	}
	return []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
}

func buildMatroskaElement(id uint64, payload []byte) []byte {
	buf := append(buildMatroskaID(id), buildMatroskaSize(uint64(len(payload)))...)
	buf = append(buf, payload...)
	return buf
}

func buildMatroskaID(id uint64) []byte {
	if id <= 0xFF {
		return []byte{byte(id)}
	}
	if id <= 0xFFFF {
		return []byte{byte(id >> 8), byte(id)}
	}
	if id <= 0xFFFFFF {
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	}
	return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
}

func buildMatroskaSize(size uint64) []byte {
	if size < 0x7F {
		return []byte{byte(0x80 | size)}
	}
	if size < 0x3FFF {
		return []byte{byte(0x40 | (size >> 8)), byte(size)}
	}
	return []byte{byte(0x20 | (size >> 16)), byte(size >> 8), byte(size)}
}
