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
	encoders, settings, langs, stats, _ := parseMatroskaTags(tagsPayload, "")
	if got := encoders[123]; got != "Lavf60.3.100" {
		t.Fatalf("unexpected encoder: %q", got)
	}
	if got := settings[123]; got != "" {
		t.Fatalf("unexpected settings: %q", got)
	}
	if got := langs[123]; got != "" {
		t.Fatalf("unexpected language: %q", got)
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

func TestParseMatroskaGeneralTags(t *testing.T) {
	body := append(buildMatroskaSimpleTag("IMDB", " tt32612507 "), buildMatroskaSimpleTag("TMDB", "movie/1304313")...)
	tagsPayload := buildMatroskaElement(mkvIDTag, body)

	_, _, _, _, generalTags := parseMatroskaTags(tagsPayload, "")
	if got := generalTags["IMDB"]; got != "tt32612507" {
		t.Fatalf("IMDB = %q, want tt32612507", got)
	}
	if got := generalTags["TMDB"]; got != "movie/1304313" {
		t.Fatalf("TMDB = %q, want movie/1304313", got)
	}
}

func TestParseMatroskaTrackAccessibilityFlags(t *testing.T) {
	payload := buildMatroskaElement(mkvIDTrackNumber, []byte{0x01})
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, []byte{0x11})...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("S_TEXT/UTF8"))...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagHearingImpaired, []byte{0x01})...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagOriginal, []byte{0x01})...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagCommentary, []byte{0x01})...)

	stream, ok := parseMatroskaTrackEntry(payload, 0, 0)
	if !ok {
		t.Fatal("expected valid Matroska text track")
	}
	if got := stream.JSON["ServiceKind"]; got != "HI / O / C" {
		t.Fatalf("ServiceKind = %q, want %q", got, "HI / O / C")
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

func TestApplyMatroskaTagStatsKeepsContainerCBR(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "Format", Value: "E-AC-3"},
			{Name: "ID", Value: "1"},
			{Name: "Bit rate mode", Value: "Constant"},
			{Name: "Bit rate", Value: "768 kb/s"},
		},
		JSON: map[string]string{
			"UniqueID":   "123",
			"BitRate":    "768000",
			"StreamSize": "371521536",
			"Duration":   "3870.019000000",
		},
	}}}

	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, bitRate: 767999, hasBitRate: true,
		dataBytes: 371521536, hasDataBytes: true,
		durationSeconds: 3870.019, hasDuration: true,
	}}, 0)

	if got := info.Tracks[0].JSON["BitRate"]; got != "768000" {
		t.Fatalf("BitRate = %q, want 768000", got)
	}
}

func TestApplyMatroskaTagStatsRoundsTextDurationToNanoseconds(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind:   StreamText,
		Fields: []Field{{Name: "ID", Value: "1"}},
		JSON:   map[string]string{"UniqueID": "123"},
	}}}

	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, durationSeconds: 8313.166, hasDuration: true,
	}}, 0)

	if got := info.Tracks[0].JSON["Duration"]; got != "8313.166000000" {
		t.Fatalf("Duration = %q, want 8313.166000000", got)
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
	}, "")
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

	stream, ok := parseMatroskaTrackEntry(entry, 0, 3)
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

	stream, ok := parseMatroskaTrackEntry(entry, 0, 3)
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

func TestParseMatroskaTrackEntryKeepsPixelDimensionsWithDisplaySize(t *testing.T) {
	videoPayload := make([]byte, 0, 32)
	videoPayload = append(videoPayload, buildMatroskaElement(mkvIDPixelWidth, encodeMatroskaUint(1920))...)
	videoPayload = append(videoPayload, buildMatroskaElement(mkvIDPixelHeight, encodeMatroskaUint(1038))...)
	videoPayload = append(videoPayload, buildMatroskaElement(mkvIDDisplayWidth, encodeMatroskaUint(320))...)
	videoPayload = append(videoPayload, buildMatroskaElement(mkvIDDisplayHeight, encodeMatroskaUint(173))...)
	video := buildMatroskaElement(mkvIDTrackVideo, videoPayload)
	entry := append(
		buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1)),
		buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))...,
	)
	entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEG4/ISO/AVC"))...)
	entry = append(entry, video...)

	stream, ok := parseMatroskaTrackEntry(entry, 0, 3)
	if !ok {
		t.Fatalf("expected parsed stream")
	}
	if got := findField(stream.Fields, "Width"); got != "1 920 pixels" {
		t.Fatalf("Width=%q", got)
	}
	if got := findField(stream.Fields, "Height"); got != "1 038 pixels" {
		t.Fatalf("Height=%q", got)
	}
	if got := findField(stream.Fields, "Display aspect ratio"); got != "1.85:1" {
		t.Fatalf("Display aspect ratio=%q", got)
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
	if videoProbeNeedsSample(probe) {
		t.Fatalf("expected empty probe to not need samples")
	}

	probe.codec = "AVC"
	if !videoProbeNeedsSample(probe) {
		t.Fatalf("expected AVC probe to need samples")
	}
	probe.exhausted = true
	if videoProbeNeedsSample(probe) {
		t.Fatalf("expected exhausted probe to stop sampling")
	}
	probe.exhausted = false
	probe.codec = "HEVC"
	probe.hdrInfo = hevcHDRInfo{
		hasMastering: true,
		maxCLL:       1000,
		maxFALL:      400,
		hdr10Plus:    true,
	}
	if !videoProbeNeedsSample(probe) {
		t.Fatalf("expected HDR-complete HEVC probe without x265 SEI to keep sampling")
	}
	probe.hdrInfo.x265Seen = true
	if videoProbeNeedsSample(probe) {
		t.Fatalf("expected HDR-complete HEVC probe with x265 SEI to stop sampling")
	}
}

func TestProbeMatroskaVideo_HEVCContinuesAfterHDRForX265SEI(t *testing.T) {
	probe := &matroskaVideoProbe{
		codec:         "HEVC",
		nalLengthSize: 4,
		hdrInfo: hevcHDRInfo{
			hasMastering: true,
			maxCLL:       1000,
			maxFALL:      400,
			hdr10Plus:    true,
		},
	}
	probes := map[uint64]*matroskaVideoProbe{1: probe}

	probeMatroskaVideo(probes, 1, buildHEVCX265LengthPrefixedSample(t))

	if !probe.hdrInfo.x265Seen {
		t.Fatalf("expected later x265 SEI to be parsed after HDR completion")
	}
	if probe.hdrInfo.x265Library != "x265 9.9" {
		t.Fatalf("x265Library = %q, want %q", probe.hdrInfo.x265Library, "x265 9.9")
	}
	if probe.hdrInfo.x265Settings != "wpp / me=0" {
		t.Fatalf("x265Settings = %q, want %q", probe.hdrInfo.x265Settings, "wpp / me=0")
	}
}

func TestScanMatroskaClusters_HEVCReadsLateX265AfterHDRComplete(t *testing.T) {
	cluster := mkvClusterWithSimpleBlock(mkvBlockNoLace(buildHEVCX265LengthPrefixedSample(t)))
	probe := &matroskaVideoProbe{
		codec:         "HEVC",
		nalLengthSize: 4,
		targetPackets: matroskaHEVCQuickProbePackets,
		hdrInfo: hevcHDRInfo{
			hasMastering: true,
			maxCLL:       1000,
			maxFALL:      400,
			hdr10Plus:    true,
		},
	}

	scanMatroskaClusters(bytes.NewReader(cluster), 0, int64(len(cluster)), 1000000, nil, map[uint64]*matroskaVideoProbe{1: probe}, false, false, 0.5, 1, nil)

	if !probe.hdrInfo.x265Seen {
		t.Fatalf("expected scanner to read later x265 SEI after HDR completion")
	}
	if probe.packetCount != 1 {
		t.Fatalf("packetCount = %d, want 1", probe.packetCount)
	}
	if probe.hdrInfo.x265Library != "x265 9.9" {
		t.Fatalf("x265Library = %q, want x265 9.9", probe.hdrInfo.x265Library)
	}
}

func TestScanMatroskaClusters_HEVCStopsAtPacketCapWithoutX265(t *testing.T) {
	cluster := mkvClusterWithSimpleBlocks(
		mkvBlockNoLace(buildHEVCNonX265LengthPrefixedSample()),
		mkvBlockNoLace(buildHEVCNonX265LengthPrefixedSample()),
	)
	probe := &matroskaVideoProbe{
		codec:         "HEVC",
		nalLengthSize: 4,
		targetPackets: 1,
		hdrInfo: hevcHDRInfo{
			hasMastering: true,
			maxCLL:       1000,
			maxFALL:      400,
			hdr10Plus:    true,
		},
	}
	videoProbes := map[uint64]*matroskaVideoProbe{1: probe}

	scanMatroskaClusters(bytes.NewReader(cluster), 0, int64(len(cluster)), 1000000, nil, videoProbes, false, false, 1, 1, nil)

	if !probe.exhausted {
		t.Fatalf("expected HEVC probe to exhaust at packet cap")
	}
	if probe.packetCount != 1 {
		t.Fatalf("packetCount = %d, want 1", probe.packetCount)
	}
	if !matroskaProbesComplete(nil, videoProbes) {
		t.Fatalf("expected exhausted non-x265 HEVC probe to be complete")
	}
}

func TestApplyMatroskaVideoProbes_X265SEIOverridesContainerEncoder(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamVideo,
		Fields: []Field{
			{Name: "ID", Value: "1"},
			{Name: "Writing library", Value: "HandBrake 1.7.0"},
			{Name: "Encoding settings", Value: "container settings"},
		},
	}}}
	probes := map[uint64]*matroskaVideoProbe{1: {
		codec: "HEVC",
		hdrInfo: hevcHDRInfo{
			x265Library:  "x265 9.9",
			x265Settings: "wpp / me=0",
			x265Seen:     true,
		},
	}}

	applyMatroskaVideoProbes(&info, probes)

	stream := info.Tracks[0]
	if got := findField(stream.Fields, "Writing library"); got != "x265 9.9" {
		t.Fatalf("Writing library = %q, want x265 9.9", got)
	}
	if got := findField(stream.Fields, "Encoding settings"); got != "wpp / me=0" {
		t.Fatalf("Encoding settings = %q, want wpp / me=0", got)
	}
}

func TestApplyMatroskaVideoProbes_DolbyVisionWithStaticHDR10(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamVideo,
		Fields: []Field{
			{Name: "ID", Value: "1"},
		},
		mkvHasDolbyVision: true,
		mkvDolbyVision: dolbyVisionConfig{
			versionMajor:    1,
			profile:         8,
			level:           6,
			rpuPresent:      true,
			blPresent:       true,
			compatibilityID: 1,
		},
	}}}
	probes := map[uint64]*matroskaVideoProbe{1: {
		codec: "HEVC",
		hdrInfo: hevcHDRInfo{
			hasMastering:          true,
			masteringLuminanceMin: 0.005,
			masteringLuminanceMax: 1000,
			maxCLL:                705,
			maxFALL:               144,
		},
	}}

	applyMatroskaVideoProbes(&info, probes)

	json := info.Tracks[0].JSON
	if got := json["HDR_Format"]; got != "Dolby Vision / SMPTE ST 2086" {
		t.Fatalf("HDR_Format = %q", got)
	}
	if got := json["HDR_Format_Compression"]; got != "None / " {
		t.Fatalf("HDR_Format_Compression = %q", got)
	}
	if got := json["MasteringDisplay_Luminance_Min"]; got != "0.0050" {
		t.Fatalf("MasteringDisplay_Luminance_Min = %q", got)
	}
	if got := json["MasteringDisplay_Luminance_Max"]; got != "1000" {
		t.Fatalf("MasteringDisplay_Luminance_Max = %q", got)
	}
	if got := json["MaxCLL"]; got != "705" {
		t.Fatalf("MaxCLL = %q", got)
	}
	if got := json["MaxFALL"]; got != "144" {
		t.Fatalf("MaxFALL = %q", got)
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

func buildHEVCX265LengthPrefixedSample(t *testing.T) []byte {
	t.Helper()

	uuid := []byte{0x2C, 0xA2, 0xDE, 0x09, 0xB5, 0x17, 0x47, 0xDB, 0xBB, 0x55, 0xA4, 0xFE, 0x7F, 0xC2, 0xFC, 0x4E}
	text := "x265 (build 1) - 9.9 - H.265/HEVC codec - c - u - options: wpp 320 bitdepth=8 fps=2 me=0"
	body := append(append([]byte{}, uuid...), []byte(text)...)
	if len(body) > 254 {
		t.Fatalf("test SEI payload too large for single-byte size: %d", len(body))
	}
	nal := append([]byte{0x4E, 0x01, 0x05, byte(len(body))}, body...)
	return append([]byte{0x00, 0x00, 0x00, byte(len(nal))}, nal...)
}

func buildHEVCNonX265LengthPrefixedSample() []byte {
	nal := []byte{0x40, 0x01, 0x0C, 0x01}
	return append([]byte{0x00, 0x00, 0x00, byte(len(nal))}, nal...)
}

func mkvClusterWithSimpleBlocks(blocks ...[]byte) []byte {
	payload := buildMatroskaElement(mkvIDTimecode, []byte{0x00})
	for _, block := range blocks {
		payload = append(payload, buildMatroskaElement(mkvIDSimpleBlock, block)...)
	}
	return buildMatroskaElement(mkvIDCluster, payload)
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
