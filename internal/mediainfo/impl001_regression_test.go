package mediainfo

import (
	"bytes"
	"math"
	"strconv"
	"testing"
)

func TestImpl001MatroskaDynamicTagKeysStayDistinct(t *testing.T) {
	var set matroskaTagSet
	for _, field := range []matroskaTagField{
		{rawName: "©", name: "©", value: "copyright"},
		{rawName: "Ω", name: "Ω", value: "omega"},
		{rawName: "A-B", name: "A-B", value: "hyphen"},
		{rawName: "AB", name: "AB", value: "plain"},
	} {
		set.set(field)
	}
	_, dynamic := matroskaTagFieldsForJSON(set, StreamAudio, nil)
	if len(dynamic) != 4 {
		t.Fatalf("dynamic member count = %d, want 4", len(dynamic))
	}
	seen := make(map[string]struct{}, len(dynamic))
	for _, member := range dynamic {
		if _, exists := seen[member.Key]; exists {
			t.Fatalf("duplicate dynamic key %q", member.Key)
		}
		seen[member.Key] = struct{}{}
	}
	if mediaInfoJSONName("ASCII_NAME") != "ASCII_NAME" {
		t.Fatal("ASCII normalization changed")
	}
}

func TestImpl001JSONCleanupKeepsTextAndXML(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamText)
	builder.Fill("Duration", "1000", "Duration", "1 s 0 ms")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	clearCanonicalSeedJSONField(&stream, "Duration")

	found := false
	textVisible := false
	xmlVisible := false
	for _, entry := range stream.canonicalSeed {
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if key != "Duration" && entry.TextLabel != "Duration" {
			continue
		}
		found = true
		if entry.Options.ShowStructured && key == "Duration" {
			t.Fatal("Duration remains visible in JSON")
		}
		textVisible = textVisible || entry.Options.ShowText && entry.TextLabel == "Duration"
		xmlVisible = xmlVisible || entry.Options.ShowXML && key == "Duration"
	}
	if !found {
		t.Fatal("Duration seed missing")
	}
	if !textVisible || !xmlVisible {
		t.Fatalf("Duration visibility = text:%v XML:%v", textVisible, xmlVisible)
	}
}

func TestImpl001DX50DoesNotSynthesizeAudioTitle(t *testing.T) {
	divx := &aviStream{kind: StreamVideo, compression: "DX50"}
	audio := &aviStream{kind: StreamAudio, index: 1, audioTag: 1}
	stream := canonicalAVIAudioStream(audio, []*aviStream{divx, audio}, 0, 0, 0, nil)
	if title, ok := canonicalSeedValue(stream, "Title"); ok {
		t.Fatalf("DX50 synthesized title = %q", title)
	}

	audio.title = "Commentary"
	stream = canonicalAVIAudioStream(audio, []*aviStream{divx, audio}, 0, 0, 0, nil)
	if title, _ := canonicalSeedValue(stream, "Title"); title != "Commentary" {
		t.Fatalf("explicit title = %q", title)
	}

	audio.title = ""
	plain := &aviStream{kind: StreamVideo, compression: "XVID"}
	stream = canonicalAVIAudioStream(audio, []*aviStream{plain, audio}, 0, 0, 0, nil)
	if title, ok := canonicalSeedValue(stream, "Title"); ok {
		t.Fatalf("non-DivX title = %q", title)
	}
}

func TestImpl001TailMatroskaStatsMergeUsesBoundedPolicy(t *testing.T) {
	tail := matroskaTagStats{trusted: true, hasFrameCount: true, frameCount: 31, hasDataBytes: true, dataBytes: 100}
	merged := mergeMatroskaTailTagStats(nil, map[uint64]matroskaTagStats{7: tail})
	if len(merged) != 0 {
		t.Fatalf("tail-only numeric stats admitted: %+v", merged[7])
	}

	head := matroskaTagStats{trusted: true, hasFrameCount: true, frameCount: 40, hasDataBytes: true, dataBytes: 200}
	merged = mergeMatroskaTailTagStats(map[uint64]matroskaTagStats{7: head}, map[uint64]matroskaTagStats{7: tail})
	if got := merged[7]; got.frameCount != 40 || got.dataBytes != 200 {
		t.Fatalf("trusted head stats overwritten: %+v", got)
	}

	tailSource := matroskaTagStats{source: "mkvmerge", hasSource: true, sourceID: 80, hasSourceID: true}
	merged = mergeMatroskaTailTagStats(nil, map[uint64]matroskaTagStats{7: tailSource})
	if got, ok := merged[7]; !ok || got.source != "mkvmerge" || got.sourceID != 80 {
		t.Fatalf("tail source metadata = %+v, %v", got, ok)
	}

	bareDuration := matroskaTagStats{trusted: true, bareDuration: true, hasDuration: true, durationSeconds: 12.5}
	if merged := mergeMatroskaTailTagStats(nil, map[uint64]matroskaTagStats{7: bareDuration}); len(merged) != 0 {
		t.Fatalf("bare DURATION gained statistics provenance: %+v", merged[7])
	}
}

func TestImpl001AACObjectType29SignalsParametricStereo(t *testing.T) {
	payload := packImpl001Bits(
		impl001Bits{29, 5}, impl001Bits{4, 4}, impl001Bits{2, 4},
		impl001Bits{3, 4}, impl001Bits{2, 5}, impl001Bits{0, 3},
	)
	_, objectType, sbrMode, psMode, _ := parseMatroskaAACProfile(payload)
	if objectType != 29 || sbrMode != "Yes (NBC)" || psMode != "Yes (NBC)" {
		t.Fatalf("AOT29 = object:%d SBR:%q PS:%q", objectType, sbrMode, psMode)
	}
	if _, _, _, psMode, _ := parseMatroskaAACProfile(nil); psMode != "" {
		t.Fatalf("malformed ASC PS = %q", psMode)
	}
}

func TestImpl001ERAACResilienceFlagsParsing(t *testing.T) {
	// ER AAC LC (objType 17), 44.1kHz (sfIndex 4), stereo (channelConfig 2)
	// frameLengthFlag=0, dependsOnCoreCoder=0, extFlag=1
	// resilience flags = 7 (3 bits: 111)
	// extensionFlag3 = 0 (1 bit)
	// syncExtensionType = 0x2b7 (11 bits)
	// extensionAudioObjectType = 5 (5 bits: SBR)
	// sbrPresentFlag = 1 (1 bit)
	payload := packImpl001Bits(
		impl001Bits{17, 5},
		impl001Bits{4, 4},
		impl001Bits{2, 4},
		impl001Bits{0, 1},
		impl001Bits{0, 1},
		impl001Bits{1, 1},
		impl001Bits{7, 3},
		impl001Bits{0, 1},
		impl001Bits{0x2b7, 11},
		impl001Bits{5, 5},
		impl001Bits{1, 1},
	)
	_, objectType, sbrMode, _, sampleRate := parseMatroskaAACProfile(payload)
	if objectType != 17 || sbrMode != "Yes (Explicit)" || sampleRate != 44100 {
		t.Fatalf("ERAAC objType=%d sbrMode=%q sampleRate=%d, want 17, Yes (Explicit), 44100", objectType, sbrMode, sampleRate)
	}

	// Truncated payload: 16 bits (2 bytes), truncated after extFlag before resilience flags.
	truncated := packImpl001Bits(
		impl001Bits{17, 5},
		impl001Bits{4, 4},
		impl001Bits{2, 4},
		impl001Bits{0, 1},
		impl001Bits{0, 1},
		impl001Bits{1, 1},
	)
	_, objectTypeTrunc, sbrModeTrunc, _, _ := parseMatroskaAACProfile(truncated)
	if objectTypeTrunc != 17 || sbrModeTrunc != "" {
		t.Fatalf("truncated ERAAC objType=%d sbrMode=%q, want 17, empty sbrMode", objectTypeTrunc, sbrModeTrunc)
	}

	// Truncated payload: 19 bits (3 bytes), contains resilience flags but truncated before syncExtensionType.
	truncatedSync := packImpl001Bits(
		impl001Bits{17, 5},
		impl001Bits{4, 4},
		impl001Bits{2, 4},
		impl001Bits{0, 1},
		impl001Bits{0, 1},
		impl001Bits{1, 1},
		impl001Bits{7, 3},
	)
	_, objectTypeSync, sbrModeSync, _, _ := parseMatroskaAACProfile(truncatedSync)
	if objectTypeSync != 17 || sbrModeSync != "" {
		t.Fatalf("truncated sync extension ERAAC objType=%d sbrMode=%q, want 17, empty sbrMode", objectTypeSync, sbrModeSync)
	}
}

func TestImpl001MPEG4ProbeWaitsForVOL(t *testing.T) {
	probe := &matroskaVideoProbe{codec: "MPEG-4 Visual"}
	probes := map[uint64]*matroskaVideoProbe{1: probe}
	probeMatroskaVideo(probes, 1, []byte{0, 0, 1, 0xB0, 0x01})
	if probe.mpeg4Seen || !videoProbeNeedsSample(probe) {
		t.Fatalf("sequence-only probe complete=%v needs=%v", probe.mpeg4Seen, videoProbeNeedsSample(probe))
	}
	vol := []byte{0, 0, 1, 0x20, 0x00, 0xc8, 0x0d, 0xc0, 0x01, 0xf0, 0x61, 0xc0, 0x18, 0x40}
	probeMatroskaVideo(probes, 1, vol)
	if !probe.mpeg4Seen || !videoProbeNeedsSample(probe) || probe.mpeg4Visual.Profile != "Simple@L1" {
		t.Fatalf("VOL probe complete=%v needs=%v profile=%q", probe.mpeg4Seen, videoProbeNeedsSample(probe), probe.mpeg4Visual.Profile)
	}
	exhausted := &matroskaVideoProbe{codec: "MPEG-4 Visual", exhausted: true}
	probeMatroskaVideo(map[uint64]*matroskaVideoProbe{1: exhausted}, 1, vol)
	if exhausted.mpeg4Seen {
		t.Fatal("exhausted probe consumed VOL")
	}
}

func TestImpl001AC3SamplingCountPrefersExactFrameCount(t *testing.T) {
	makeStream := func(withFrames bool) Stream {
		builder := newCanonicalStreamBuilder(StreamAudio)
		builder.Fill("ID", "1", "ID", "1")
		builder.Fill("Format", "AC-3", "Format", "AC-3")
		builder.Fill("Duration", "1000", "Duration", "1 s 0 ms")
		if withFrames {
			builder.Structured("FrameCount", "31")
		}
		stream := builder.Snapshot(canonicalStreamPolicy{})
		stream.mkvTagFrameCount = withFrames
		return stream
	}
	probe := &matroskaAudioProbe{format: "AC-3", ok: true, info: ac3Info{sampleRate: 48000, spf: 1536}}

	info := MatroskaInfo{Tracks: []Stream{makeStream(true)}}
	applyMatroskaAudioProbes(&info, map[uint64]*matroskaAudioProbe{1: probe})
	if count, _ := canonicalSeedValue(info.Tracks[0], "SamplingCount"); count != "47616" {
		t.Fatalf("frame-derived SamplingCount = %q", count)
	}

	info = MatroskaInfo{Tracks: []Stream{makeStream(false)}}
	applyMatroskaAudioProbes(&info, map[uint64]*matroskaAudioProbe{1: probe})
	if count, _ := canonicalSeedValue(info.Tracks[0], "SamplingCount"); count != "48000" {
		t.Fatalf("duration-derived SamplingCount = %q", count)
	}

	maxSafe := int64(math.MaxInt64 / 1536)
	for _, tc := range []struct {
		name      string
		frames    int64
		wantCount string
	}{
		{name: "maximum safe", frames: maxSafe, wantCount: strconv.FormatInt(maxSafe*1536, 10)},
		{name: "first overflow", frames: maxSafe + 1, wantCount: "48000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := makeStream(false)
			stream.mkvTagFrameCount = true
			replaceCanonicalSeedFill(&stream, "FrameCount", strconv.FormatInt(tc.frames, 10), "", "")
			bounded := MatroskaInfo{Tracks: []Stream{stream}}
			applyMatroskaAudioProbes(&bounded, map[uint64]*matroskaAudioProbe{1: probe})
			if count, _ := canonicalSeedValue(bounded.Tracks[0], "SamplingCount"); count != tc.wantCount {
				t.Fatalf("SamplingCount = %q, want %q", count, tc.wantCount)
			}
		})
	}
}

func TestImpl001MPEGPSSystemHeaderCapIsMonotonic(t *testing.T) {
	data := makeMPEGPSPackHeader(true, 60_000)
	data = append(data, 0, 0, 1, 0xBB)
	data = append(data, makeMPEGPSPackHeader(true, 100_000)...)
	if got := mpegPSBoundedWindow(bytes.NewReader(data), int64(len(data))); got != 8<<20 {
		t.Fatalf("pack-system-pack window = %d", got)
	}

	data = append(makeMPEGPSPackHeader(true, 60_000), makeMPEGPSPackHeader(true, 100_000)...)
	if got := mpegPSBoundedWindow(bytes.NewReader(data), int64(len(data))); got != 16<<20 {
		t.Fatalf("no-system maximum window = %d", got)
	}
}

type impl001Bits struct {
	value uint64
	width int
}

func packImpl001Bits(fields ...impl001Bits) []byte {
	bitCount := 0
	for _, field := range fields {
		bitCount += field.width
	}
	out := make([]byte, (bitCount+7)/8)
	position := 0
	for _, field := range fields {
		for bit := field.width - 1; bit >= 0; bit-- {
			if field.value&(uint64(1)<<bit) != 0 {
				out[position/8] |= 1 << (7 - position%8)
			}
			position++
		}
	}
	return out
}
