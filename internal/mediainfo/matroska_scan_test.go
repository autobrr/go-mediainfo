package mediainfo

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

type bitWriter struct {
	b   []byte
	bit int
}

func (w *bitWriter) writeBits(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		bit := (v >> i) & 1
		byteIdx := w.bit >> 3
		bitIdx := 7 - (w.bit & 7)
		if bit != 0 {
			w.b[byteIdx] |= 1 << bitIdx
		}
		w.bit++
	}
}

func makeEAC3Frame(t *testing.T, frameSize int, dialnormCode uint32) []byte {
	t.Helper()
	if frameSize < 8 {
		t.Fatalf("frameSize too small: %d", frameSize)
	}
	frmsiz := uint32(frameSize/2 - 1)
	if frmsiz > 0x7FF {
		t.Fatalf("frameSize too large: %d", frameSize)
	}

	b := make([]byte, frameSize)
	w := bitWriter{b: b}
	w.writeBits(0x0B77, 16) // syncword
	w.writeBits(0, 2)       // strmtyp: independent
	w.writeBits(0, 3)       // substreamid
	w.writeBits(frmsiz, 11)
	w.writeBits(0, 2)  // fscod: 48kHz
	w.writeBits(3, 2)  // numblkscod: 6 blocks
	w.writeBits(2, 3)  // acmod: 2/0 (stereo)
	w.writeBits(0, 1)  // lfeon
	w.writeBits(16, 5) // bsid (>=10 for E-AC-3 sanity)
	w.writeBits(dialnormCode&0x1F, 5)
	w.writeBits(0, 1) // compre
	return b
}

func TestProbeMatroskaAudio_EAC3MultiFrameNonLacedPacket(t *testing.T) {
	const trackID = uint64(1)
	const frameSize = 16

	f1 := makeEAC3Frame(t, frameSize, 1)
	f2 := makeEAC3Frame(t, frameSize, 2)
	payload := append(append([]byte{}, f1...), f2...)

	// Non-laced packets may contain multiple E-AC-3 syncframes back-to-back; ensure the
	// probe path aggregates them when packetAligned=false.
	probes := map[uint64]*matroskaAudioProbe{
		trackID: {format: "E-AC-3", collect: true},
	}
	probeMatroskaAudio(probes, trackID, payload, 1, int64(len(payload)), false)
	p := probes[trackID]
	if !p.ok {
		t.Fatalf("expected probe ok")
	}
	if got := p.info.dialnormCount; got != 2 {
		t.Fatalf("expected dialnormCount=2, got %d", got)
	}
	if p.info.dialnormMin != -2 || p.info.dialnormMax != -1 {
		t.Fatalf("expected dialnormMin=-2 dialnormMax=-1, got min=%d max=%d", p.info.dialnormMin, p.info.dialnormMax)
	}

	// With packetAligned=true, probe expects exactly one frame per packet and rejects mismatched sizes.
	probes2 := map[uint64]*matroskaAudioProbe{
		trackID: {format: "E-AC-3", collect: true},
	}
	probeMatroskaAudio(probes2, trackID, payload, 1, int64(len(payload)), true)
	p2 := probes2[trackID]
	if p2.ok {
		t.Fatalf("expected probe not ok with packetAligned=true")
	}
	if got := p2.info.dialnormCount; got != 0 {
		t.Fatalf("expected dialnormCount=0, got %d", got)
	}
}

func TestReadMatroskaBlockHeader_JOCStopDoesNotCapStatsLaces(t *testing.T) {
	block := []byte{0x81, 0x00, 0x00, 0x04, 0x03} // track 1, fixed lacing, 4 frames
	for code := uint32(1); code <= 4; code++ {
		block = append(block, makeEAC3Frame(t, 16, code)...)
	}
	probe := &matroskaAudioProbe{
		format:         "E-AC-3",
		collect:        true,
		parseJOC:       false,
		jocStopPackets: 1,
		targetPackets:  10,
		packetCount:    1,
	}
	probe.info.hasJOC = true
	er := newEBMLReader(bytes.NewReader(block))

	_, _, _, frames, err := readMatroskaBlockHeader(er, int64(len(block)), map[uint64]*matroskaAudioProbe{1: probe}, nil, 0)
	if err != nil {
		t.Fatalf("readMatroskaBlockHeader: %v", err)
	}
	if frames != 4 {
		t.Fatalf("frames = %d, want 4", frames)
	}
	if got := probe.info.dialnormCount; got != 4 {
		t.Fatalf("dialnormCount = %d, want 4", got)
	}
}

func TestReadMatroskaBlockHeader_FrameLimitStopsMidLace(t *testing.T) {
	block := []byte{0x81, 0x00, 0x00, 0x04, 0x03} // track 1, fixed lacing, 4 frames
	for code := uint32(1); code <= 4; code++ {
		block = append(block, makeEAC3Frame(t, 16, code)...)
	}
	probe := &matroskaAudioProbe{format: "E-AC-3", collect: true}
	er := newEBMLReader(bytes.NewReader(block))

	_, _, _, frames, err := readMatroskaBlockHeader(er, int64(len(block)), map[uint64]*matroskaAudioProbe{1: probe}, nil, 1)
	if err != nil {
		t.Fatalf("readMatroskaBlockHeader: %v", err)
	}
	if frames != 1 {
		t.Fatalf("frames = %d, want 1", frames)
	}
	if got := probe.info.dialnormCount; got != 1 {
		t.Fatalf("dialnormCount = %d, want 1", got)
	}
}

func TestReadMatroskaBlockHeader_TargetFramesStopsMidLace(t *testing.T) {
	block := []byte{0x81, 0x00, 0x00, 0x04, 0x03} // track 1, fixed lacing, 4 frames
	for code := uint32(1); code <= 4; code++ {
		block = append(block, makeEAC3Frame(t, 16, code)...)
	}
	probe := &matroskaAudioProbe{format: "E-AC-3", collect: true, targetFrames: 2, targetPackets: 10}
	er := newEBMLReader(bytes.NewReader(block))

	_, _, _, frames, err := readMatroskaBlockHeader(er, int64(len(block)), map[uint64]*matroskaAudioProbe{1: probe}, nil, 0)
	if err != nil {
		t.Fatalf("readMatroskaBlockHeader: %v", err)
	}
	if frames != 4 {
		t.Fatalf("frames = %d, want 4 container frames", frames)
	}
	if got := probe.info.dialnormCount; got != 2 {
		t.Fatalf("dialnormCount = %d, want 2 bounded probe frames", got)
	}
	if probe.collect {
		t.Fatal("probe must stop collecting at target frame count")
	}
}

func TestMatroskaBlockFrameLimitIncludesCrossingFrame(t *testing.T) {
	globalFrames := int64(2560)
	if got := matroskaBlockFrameLimit(&globalFrames, 2560); got != 1 {
		t.Fatalf("matroskaBlockFrameLimit() = %d, want 1", got)
	}
}

func TestApplyMatroskaStats_AudioDurationAlsoSetsJSON(t *testing.T) {
	info := MatroskaInfo{
		Tracks: []Stream{
			{
				Kind: StreamAudio,
				Fields: []Field{
					{Name: "ID", Value: "1"},
					{Name: "Format", Value: "AAC"},
				},
			},
		},
	}

	stats := map[uint64]*matroskaTrackStats{
		1: {
			hasTime:   true,
			minTimeNs: 0,
			maxTimeNs: int64(4.321 * 1e9),
		},
	}

	seedMatroskaLegacyTestStream(&info.Tracks[0])
	applyMatroskaStats(&info, stats, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	if got := findField(info.Tracks[0].Fields, "Duration"); got == "" {
		t.Fatalf("expected Duration field set")
	}
	if info.Tracks[0].JSON == nil || info.Tracks[0].JSON["Duration"] == "" {
		t.Fatalf("expected JSON Duration set")
	}
}

func TestApplyMatroskaAudioProbesEmitsAC3DynrngStats(t *testing.T) {
	dynrngs := make([]uint32, 256)
	dynrngs[0] = 3
	dynrngs[1] = 1
	info := MatroskaInfo{Tracks: []Stream{{
		Kind:          StreamAudio,
		Fields:        []Field{{Name: "ID", Value: "1"}, {Name: "Format", Value: "AC-3"}, {Name: "Bit rate mode", Value: "Constant"}},
		JSON:          map[string]string{"BitRate": "767999"},
		canonicalSeed: matroskaAC3CanonicalSeed(matroskaAC3CanonicalFacts{format: "AC-3", trackNumber: 1, bitRate: 767999}),
	}}}
	probes := map[uint64]*matroskaAudioProbe{1: {
		format: "AC-3",
		ok:     true,
		info: ac3Info{
			bsid:        6,
			bitRateKbps: 768,
			dynrngs:     dynrngs,
			dynrngeSeen: true,
		},
	}}

	applyMatroskaAudioProbes(&info, probes)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	if got := info.Tracks[0].JSON["BitRate"]; got != "768000" {
		t.Fatalf("BitRate = %q, want 768000", got)
	}
	extra := info.Tracks[0].JSONRaw["extra"]
	for field, want := range map[string]string{
		"dynrng_Average": "0.07",
		"dynrng_Minimum": "0.00",
		"dynrng_Maximum": "0.27",
		"dynrng_Count":   "4",
	} {
		if !strings.Contains(extra, `"`+field+`":"`+want+`"`) {
			t.Fatalf("extra %s mismatch, want %q: %s", field, want, extra)
		}
	}
}

func TestReadMatroskaBlockHeaderPacketCapIncludesCrossingPacket(t *testing.T) {
	block := []byte{0x81, 0x00, 0x00, 0x04, 0x03} // track 1, fixed lacing, 4 frames
	for code := uint32(1); code <= 4; code++ {
		block = append(block, makeEAC3Frame(t, 16, code)...)
	}
	for _, target := range []int{212, 300} {
		t.Run(strconv.Itoa(target), func(t *testing.T) {
			below := &matroskaAudioProbe{format: "E-AC-3", collect: true, targetPackets: target, packetCount: target - 2}
			er := newEBMLReader(bytes.NewReader(block))
			_, _, _, _, err := readMatroskaBlockHeader(er, int64(len(block)), map[uint64]*matroskaAudioProbe{1: below}, nil, 0)
			if err != nil {
				t.Fatalf("readMatroskaBlockHeader below cap: %v", err)
			}
			if below.packetCount != target-1 || below.info.dialnormCount != 4 || !below.collect {
				t.Fatalf("below-cap state packet=%d dialnorm=%d collect=%v, want %d/4/true", below.packetCount, below.info.dialnormCount, below.collect, target-1)
			}

			probe := &matroskaAudioProbe{format: "E-AC-3", collect: true, targetPackets: target, packetCount: target - 1}
			er = newEBMLReader(bytes.NewReader(block))
			_, _, _, _, err = readMatroskaBlockHeader(er, int64(len(block)), map[uint64]*matroskaAudioProbe{1: probe}, nil, 0)
			if err != nil {
				t.Fatalf("readMatroskaBlockHeader: %v", err)
			}
			if probe.packetCount != target || probe.info.dialnormCount != 1 || probe.collect {
				t.Fatalf("cap state packet=%d dialnorm=%d collect=%v, want %d/1/false", probe.packetCount, probe.info.dialnormCount, probe.collect, target)
			}
		})
	}
}

func TestParseDTSCoreFrameDoesNotUsePCMResolutionAsESFlag(t *testing.T) {
	plain := buildMatroskaDTSCoreFrame(1, false, false)
	parsed, ok := parseDTSCoreFrame(plain)
	if !ok {
		t.Fatal("plain DTS core did not parse")
	}
	if parsed.coreES || parsed.coreXCh {
		t.Fatalf("odd PCM resolution fabricated ES metadata: %+v", parsed)
	}

	matrixES := buildMatroskaDTSCoreFrame(3, true, false)
	parsed, ok = parseDTSCoreFrame(matrixES)
	if !ok || !parsed.coreES || parsed.coreXCh {
		t.Fatalf("matrix ES extension descriptor not recognized: ok=%v info=%+v", ok, parsed)
	}

	es := buildMatroskaDTSCoreFrame(3, true, true)
	parsed, ok = parseDTSCoreFrame(es)
	if !ok || !parsed.coreES || !parsed.coreXCh {
		t.Fatalf("explicit XCh extension not recognized: ok=%v info=%+v", ok, parsed)
	}
}

func TestParseDTSCoreFrameTwoBitPCMResolution(t *testing.T) {
	for code, want := range []int{16, 20, 24, 24} {
		parsed, ok := parseDTSCoreFrame(buildMatroskaDTSCoreFrame(uint32(code), false, false))
		if !ok || parsed.bitDepth != want || parsed.coreES {
			t.Fatalf("PCM resolution code %d: ok=%v info=%+v, want depth %d without ES", code, ok, parsed, want)
		}
	}
}

func TestApplyMatroskaAudioProbesAC3StatsIgnoreDTSCompanion(t *testing.T) {
	makeInfo := func(withDTS bool) (MatroskaInfo, map[uint64]*matroskaAudioProbe) {
		dynrngs := make([]uint32, 256)
		dynrngs[0], dynrngs[1] = 3, 1
		info := MatroskaInfo{Tracks: []Stream{{Kind: StreamAudio, Fields: []Field{{Name: "ID", Value: "1"}, {Name: "Format", Value: "AC-3"}}}}}
		probes := map[uint64]*matroskaAudioProbe{1: {format: "AC-3", ok: true, info: ac3Info{bsid: 6, dynrngs: dynrngs, dynrngeSeen: true}}}
		if withDTS {
			info.Tracks = append(info.Tracks, Stream{Kind: StreamAudio, Fields: []Field{{Name: "ID", Value: "2"}, {Name: "Format", Value: "DTS"}}})
			probes[2] = &matroskaAudioProbe{format: "DTS", ok: true, dts: dtsInfo{sampleRate: 48000, samplesPerFrame: 512, channels: 2, bitDepth: 24}}
		}
		return info, probes
	}
	without, withoutProbes := makeInfo(false)
	with, withProbes := makeInfo(true)
	applyMatroskaAudioProbes(&without, withoutProbes)
	applyMatroskaAudioProbes(&with, withProbes)
	if got, want := with.Tracks[0].JSONRaw["extra"], without.Tracks[0].JSONRaw["extra"]; got != want {
		t.Fatalf("DTS companion changed AC-3 statistics:\nwith=%s\nwithout=%s", got, want)
	}
}

func buildMatroskaDTSCoreFrame(pcmResCode uint32, es, includeXChSync bool) []byte {
	out := make([]byte, 96)
	copy(out, []byte{0x7F, 0xFE, 0x80, 0x01})
	pos := 32
	writeBits(out, &pos, 0, 1)
	writeBits(out, &pos, 0, 5)
	writeBits(out, &pos, 0, 1)
	writeBits(out, &pos, 15, 7)
	writeBits(out, &pos, 95, 14)
	writeBits(out, &pos, 2, 6)
	writeBits(out, &pos, 13, 4)
	writeBits(out, &pos, 15, 5)
	writeBits(out, &pos, 0, 5)
	writeBits(out, &pos, 0, 3) // XCh descriptor
	if includeXChSync {
		writeBits(out, &pos, 1, 1)
	} else {
		writeBits(out, &pos, 0, 1)
	}
	writeBits(out, &pos, 0, 1)
	writeBits(out, &pos, 0, 2)
	writeBits(out, &pos, 0, 1)
	writeBits(out, &pos, 0, 1)
	writeBits(out, &pos, 0, 4)
	writeBits(out, &pos, 0, 2)
	writeBits(out, &pos, pcmResCode, 2)
	if es {
		writeBits(out, &pos, 1, 1)
	} else {
		writeBits(out, &pos, 0, 1)
	}
	if includeXChSync {
		copy(out[20:], []byte{0x5A, 0x5A, 0x5A, 0x5A})
	}
	return out
}

func TestReadMatroskaBlockHeader_InvalidEBMLLacingCount(t *testing.T) {
	// track=1, timecode=0, flags=EBML lacing, lace count byte=0 (frameCount=1; malformed for EBML lacing)
	block := []byte{0x81, 0x00, 0x00, 0x06, 0x00, 0x81, 0x00}
	er := newEBMLReader(bytes.NewReader(block))
	audio := map[uint64]*matroskaAudioProbe{
		1: {format: "E-AC-3", collect: true},
	}
	if _, _, _, _, err := readMatroskaBlockHeader(er, int64(len(block)), audio, nil, 0); err == nil {
		t.Fatalf("expected error for malformed EBML lacing frame count")
	}
}

func TestReadMatroskaBlockHeader_EBMLLacingOversizedLaceRejected(t *testing.T) {
	// EBML lacing: make the first lace size absurdly large compared to the block payload.
	// Without bounds checks, this can trigger huge allocations when probing E-AC-3 with parseJOC=true.
	//
	// track=1, timecode=0, flags=EBML lacing, lace count byte=1 (frameCount=2),
	// first lace size vint = 0x1FFFFFFF (length=4) -> 268435455.
	block := []byte{
		0x81,       // track=1
		0x00, 0x00, // timecode=0
		0x06, // EBML lacing
		0x01, // lace count=1 => frameCount=2
		0x1F, 0xFF, 0xFF, 0xFF,
		0x00, // payload byte
	}
	er := newEBMLReader(bytes.NewReader(block))
	audio := map[uint64]*matroskaAudioProbe{
		1: {format: "E-AC-3", collect: true, parseJOC: true, targetPackets: 1},
	}
	if _, _, _, _, err := readMatroskaBlockHeader(er, int64(len(block)), audio, nil, 0); err == nil {
		t.Fatalf("expected error for oversized EBML lacing frame size")
	}
}

func TestReadMatroskaElementHeader_SizeBeyondRemaining(t *testing.T) {
	// id=Timecode (0xE7), size=1, but no payload remains.
	er := newEBMLReader(bytes.NewReader([]byte{0xE7, 0x81}))
	if _, _, err := readMatroskaElementHeader(er, 2, 0); err == nil {
		t.Fatalf("expected error for element size beyond remaining bytes")
	}
}

func TestScanMatroskaClustersVideoProbeAggregateByteBudget(t *testing.T) {
	budget := &matroskaVideoProbeBudget{remaining: 8}
	video := &matroskaVideoProbe{
		codec:         "HEVC",
		nalLengthSize: 4,
		targetPackets: matroskaHEVCQuickProbePackets,
		budget:        budget,
	}
	cluster := mkvClusterWithSimpleBlocks(
		mkvBlockNoLace(make([]byte, 32)),
		mkvBlockNoLace(make([]byte, 32)),
	)
	// An unresolved dependent E-AC-3 probe raises the global frame horizon and
	// forces the scanner to visit both video blocks after the video budget ends.
	audio := map[uint64]*matroskaAudioProbe{
		2: {format: "E-AC-3", dependentEAC3: true},
	}

	scanMatroskaClusters(bytes.NewReader(cluster), 0, int64(len(cluster)), 1000000, audio, map[uint64]*matroskaVideoProbe{1: video}, false, false, 0.5, 2, nil, nil)

	if budget.remaining != 0 {
		t.Fatalf("video probe budget remaining = %d, want 0", budget.remaining)
	}
	if video.packetCount != 1 {
		t.Fatalf("video packet count = %d, want 1 sampled packet", video.packetCount)
	}
	if videoProbeNeedsSample(video) {
		t.Fatal("aggregate byte budget exhaustion must stop video sampling")
	}
}

func TestScanMatroskaClustersInitializesOneSharedVideoProbeBudget(t *testing.T) {
	first := &matroskaVideoProbe{codec: "AVC", targetPackets: 2}
	second := &matroskaVideoProbe{codec: "AVC", targetPackets: 2}
	block1 := mkvBlockNoLace(make([]byte, 32))
	block2 := mkvBlockNoLace(make([]byte, 32))
	block2[0] = 0x82
	cluster := mkvClusterWithSimpleBlocks(block1, block2)

	scanMatroskaClusters(bytes.NewReader(cluster), 0, int64(len(cluster)), 1000000, nil, map[uint64]*matroskaVideoProbe{1: first, 2: second}, false, false, 0.5, 2, nil, nil)

	if first.budget == nil || second.budget == nil || first.budget != second.budget {
		t.Fatalf("video probes did not receive one shared initialized budget: first=%p second=%p", first.budget, second.budget)
	}
	if got, want := first.budget.remaining, int64(matroskaVideoProbeMaxTotalBytes-64); got != want {
		t.Fatalf("shared budget remaining = %d, want %d", got, want)
	}
}

func TestReadMatroskaBlockHeaderReconstructsPrefixOnlyFrames(t *testing.T) {
	block := []byte{0x81, 0x00, 0x00, 0x00}
	audioProbe := &matroskaAudioProbe{format: "DTS", headerStrip: buildMatroskaDTSCoreFrame(2, false, false)}
	er := newEBMLReader(bytes.NewReader(block))
	if _, _, _, _, err := readMatroskaBlockHeader(er, int64(len(block)), map[uint64]*matroskaAudioProbe{1: audioProbe}, nil, 0); err != nil {
		t.Fatalf("prefix-only audio block: %v", err)
	}
	if !audioProbe.ok || audioProbe.dts.bitDepth != 24 {
		t.Fatalf("prefix-only audio was not reconstructed: %+v", audioProbe)
	}

	videoProbe := &matroskaVideoProbe{codec: "HEVC", nalLengthSize: 4, targetPackets: 1, headerStrip: buildHEVCX265LengthPrefixedSample(t)}
	er = newEBMLReader(bytes.NewReader(block))
	if _, _, _, _, err := readMatroskaBlockHeader(er, int64(len(block)), nil, map[uint64]*matroskaVideoProbe{1: videoProbe}, 0); err != nil {
		t.Fatalf("prefix-only video block: %v", err)
	}
	if videoProbe.hdrInfo.x265Library != "x265 9.9" {
		t.Fatalf("prefix-only video was not reconstructed: %+v", videoProbe.hdrInfo)
	}

	if got := applyMatroskaAudioHeaderStrip(nil, &matroskaAudioProbe{}); len(got) != 0 {
		t.Fatalf("empty payload without stripping reconstructed % X", got)
	}
}

func TestProbeMatroskaAudioDTSCoreExtensionOrdering(t *testing.T) {
	core := buildMatroskaDTSCoreFrame(2, false, false)
	xll := append(append([]byte{}, core...), 0x64, 0x58, 0x20, 0x25, 0x41, 0xA2, 0x95, 0x47)
	probe := &matroskaAudioProbe{format: "DTS"}
	probeMatroskaAudio(map[uint64]*matroskaAudioProbe{1: probe}, 1, xll, 1, int64(len(xll)), true)
	if !probe.ok || !probe.dts.hd || !probe.dts.hdXLL || probe.dts.lbr {
		t.Fatalf("core plus XLL classification = %+v", probe.dts)
	}

	lbr := append(append([]byte{}, core...), 0x0A, 0x80, 0x19, 0x21, 0x01)
	probe = &matroskaAudioProbe{format: "DTS"}
	probeMatroskaAudio(map[uint64]*matroskaAudioProbe{1: probe}, 1, lbr, 1, int64(len(lbr)), true)
	if !probe.ok || probe.dts.lbr || probe.dts.hd {
		t.Fatalf("core plus LBR marker must remain a core stream: %+v", probe.dts)
	}
}

func TestApplyMatroskaAudioProbesUsesDTSBitstreamEvidenceForCoreES(t *testing.T) {
	makeTrack := func(uid uint64) Stream {
		return Stream{
			Kind: StreamAudio, Fields: []Field{{Name: "ID", Value: "1"}, {Name: "Format", Value: "DTS"}},
			JSON:          map[string]string{"UniqueID": strconv.FormatUint(uid, 10)},
			canonicalSeed: matroskaDTSCanonicalSeed(matroskaDTSCanonicalFacts{trackNumber: 1, trackUID: uid}),
		}
	}
	coreProbe := &matroskaAudioProbe{format: "DTS", ok: true, dts: dtsInfo{bitRateBps: 768000, bitDepth: 24, sampleRate: 48000, samplesPerFrame: 512, channels: 6}}
	for _, uid := range []uint64{9826214264200667624, 12894577728004814758} {
		info := MatroskaInfo{Tracks: []Stream{makeTrack(uid)}}
		applyMatroskaAudioProbes(&info, map[uint64]*matroskaAudioProbe{1: coreProbe})
		refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
		if got := info.Tracks[0].JSON["Format_AdditionalFeatures"]; got != "" {
			t.Fatalf("TrackUID %d fabricated DTS feature %q without bitstream evidence", uid, got)
		}
	}
	esProbe := &matroskaAudioProbe{format: "DTS", ok: true, dts: dtsInfo{bitRateBps: 768000, bitDepth: 24, sampleRate: 48000, samplesPerFrame: 512, channels: 6, coreES: true, coreXCh: true}}
	es := MatroskaInfo{Tracks: []Stream{makeTrack(1)}}
	applyMatroskaAudioProbes(&es, map[uint64]*matroskaAudioProbe{1: esProbe})
	refreshCanonicalCompatibilitySnapshot(&es.Tracks[0])
	if got := es.Tracks[0].JSON["Format_AdditionalFeatures"]; got != "ES XCh" {
		t.Fatalf("bitstream-proven DTS feature = %q, want ES XCh", got)
	}
}

func TestParseVP9FrameHeaderUsesBitstreamProfile(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		profile int
		depth   int
		space   string
		chroma  string
	}{
		{name: "profile 0", payload: buildVP9KeyFrame(0, 0, 2, 0, 0), profile: 0, depth: 8, space: "YUV", chroma: "4:2:0"},
		{name: "profile 1", payload: buildVP9KeyFrame(1, 0, 2, 0, 0), profile: 1, depth: 8, space: "YUV", chroma: "4:4:4"},
		{name: "profile 2", payload: buildVP9KeyFrame(2, 0, 2, 0, 0), profile: 2, depth: 10, space: "YUV", chroma: "4:2:0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseVP9FrameHeader(tc.payload)
			if !ok || got.profile != tc.profile || got.bitDepth != tc.depth || got.colorSpace != tc.space || got.chroma != tc.chroma || got.matrixCoefficients != "BT.709" || got.colorRange != "Limited" {
				t.Fatalf("VP9 header = %+v, %v", got, ok)
			}
		})
	}
	inter := buildVP9KeyFrame(0, 0, 2, 0, 0)
	inter[0] |= 1 << 2
	if got, ok := parseVP9FrameHeader(inter); ok {
		t.Fatalf("inter frame accepted: %+v", got)
	}
}

func buildVP9KeyFrame(profile int, twelveBit, colorSpace, subsamplingX, subsamplingY uint32) []byte {
	payload := make([]byte, 16)
	pos := 0
	writeBits(payload, &pos, 2, 2) // frame marker
	writeBits(payload, &pos, uint32(profile&1), 1)
	writeBits(payload, &pos, uint32(profile>>1), 1)
	if profile == 3 {
		writeBits(payload, &pos, 0, 1) // reserved
	}
	writeBits(payload, &pos, 0, 1)         // show_existing_frame
	writeBits(payload, &pos, 0, 1)         // key frame
	writeBits(payload, &pos, 1, 1)         // show_frame
	writeBits(payload, &pos, 0, 1)         // error_resilient_mode
	writeBits(payload, &pos, 0x498342, 24) // sync code
	if profile >= 2 {
		writeBits(payload, &pos, twelveBit, 1)
	}
	writeBits(payload, &pos, colorSpace, 3)
	if colorSpace != 7 {
		writeBits(payload, &pos, 0, 1) // limited range
		if profile == 1 || profile == 3 {
			writeBits(payload, &pos, subsamplingX, 1)
			writeBits(payload, &pos, subsamplingY, 1)
			writeBits(payload, &pos, 0, 1) // reserved
		}
	}
	return payload[:(pos+7)/8]
}

func TestParseAV1SequenceHeaderOBUUsesBitstreamColorConfig(t *testing.T) {
	bits := make([]byte, 16)
	pos := 0
	writeBits(bits, &pos, 0, 3) // seq_profile
	writeBits(bits, &pos, 1, 1) // still_picture
	writeBits(bits, &pos, 1, 1) // reduced_still_picture_header
	writeBits(bits, &pos, 0, 5) // seq_level_idx
	writeBits(bits, &pos, 0, 4) // frame_width_bits_minus_1
	writeBits(bits, &pos, 0, 4) // frame_height_bits_minus_1
	writeBits(bits, &pos, 0, 1) // max_frame_width_minus_1
	writeBits(bits, &pos, 0, 1) // max_frame_height_minus_1
	writeBits(bits, &pos, 0, 3) // superblock/filter flags
	writeBits(bits, &pos, 0, 3) // superres/CDEF/restoration
	writeBits(bits, &pos, 1, 1) // high_bitdepth
	writeBits(bits, &pos, 0, 1) // mono_chrome
	writeBits(bits, &pos, 1, 1) // color_description_present_flag
	writeBits(bits, &pos, 1, 8) // BT.709 primaries
	writeBits(bits, &pos, 1, 8) // BT.709 transfer
	writeBits(bits, &pos, 1, 8) // BT.709 matrix
	writeBits(bits, &pos, 0, 1) // limited range
	writeBits(bits, &pos, 0, 2) // chroma sample position
	writeBits(bits, &pos, 0, 1) // separate_uv_delta_q
	payload := bits[:(pos+7)/8]
	obu := append([]byte{0x0A, byte(len(payload))}, payload...)

	got, ok := parseAV1SequenceHeaderOBU(obu)
	if !ok || !got.descriptionPresent || got.colorRange != "Limited" || got.colorPrimaries != "BT.709" || got.transferCharacteristics != "BT.709" || got.matrixCoefficients != "BT.709" {
		t.Fatalf("AV1 color config = %+v, %v", got, ok)
	}
}

func TestScanMatroskaClustersVideoProbeBudgetAllowsLateX265(t *testing.T) {
	budget := &matroskaVideoProbeBudget{remaining: 4096}
	video := &matroskaVideoProbe{
		codec:         "HEVC",
		nalLengthSize: 4,
		targetPackets: matroskaHEVCQuickProbePackets,
		budget:        budget,
	}
	cluster := mkvClusterWithSimpleBlocks(
		mkvBlockNoLace(buildHEVCNonX265LengthPrefixedSample()),
		mkvBlockNoLace(buildHEVCX265LengthPrefixedSample(t)),
	)

	scanMatroskaClusters(bytes.NewReader(cluster), 0, int64(len(cluster)), 1000000, nil, map[uint64]*matroskaVideoProbe{1: video}, false, false, 0.5, 1, nil, nil)

	if video.hdrInfo.x265Library != "x265 9.9" {
		t.Fatalf("late x265 library = %q, want x265 9.9", video.hdrInfo.x265Library)
	}
	if budget.remaining <= 0 {
		t.Fatal("normal small frames unexpectedly exhausted video probe budget")
	}
}

func TestApplyMatroskaAudioProbesDTSPreservesAuthoritativeBitRate(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "ID", Value: "1"},
			{Name: "Format", Value: "DTS"},
			{Name: "Bit rate mode", Value: "Constant"},
			{Name: "Bit rate", Value: "767 kb/s"},
		},
		JSON:          map[string]string{"BitRate": "767000", "BitRate_Mode": "CBR"},
		canonicalSeed: matroskaDTSCanonicalSeed(matroskaDTSCanonicalFacts{trackNumber: 1, bitRate: 767000}),
	}}}
	probes := map[uint64]*matroskaAudioProbe{1: {
		format: "DTS",
		ok:     true,
		dts:    dtsInfo{bitRateBps: 768000},
	}}

	applyMatroskaAudioProbes(&info, probes)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	if got := findField(info.Tracks[0].Fields, "Bit rate"); got != "767 kb/s" {
		t.Fatalf("text bit rate = %q, want authoritative 767 kb/s", got)
	}
	if got := info.Tracks[0].JSON["BitRate"]; got != "767000" {
		t.Fatalf("JSON BitRate = %q, want authoritative 767000", got)
	}
}

func TestApplyMatroskaAudioProbesDTSNormalizesEquivalentBitRate(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "ID", Value: "1"},
			{Name: "Format", Value: "DTS"},
			{Name: "Bit rate", Value: "768 kb/s"},
		},
		JSON:          map[string]string{"BitRate": "767999"},
		canonicalSeed: matroskaDTSCanonicalSeed(matroskaDTSCanonicalFacts{trackNumber: 1, bitRate: 767999}),
	}}}
	probes := map[uint64]*matroskaAudioProbe{1: {
		format: "DTS",
		ok:     true,
		dts:    dtsInfo{bitRateBps: 768000},
	}}

	applyMatroskaAudioProbes(&info, probes)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	if got := info.Tracks[0].JSON["BitRate"]; got != "768000" {
		t.Fatalf("JSON BitRate = %q, want equivalent core value 768000", got)
	}
}

func TestApplyMatroskaAudioProbesDTSUsesCoreBitRateWhenAbsent(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind:          StreamAudio,
		Fields:        []Field{{Name: "ID", Value: "1"}, {Name: "Format", Value: "DTS"}},
		canonicalSeed: matroskaDTSCanonicalSeed(matroskaDTSCanonicalFacts{trackNumber: 1}),
	}}}
	probes := map[uint64]*matroskaAudioProbe{1: {
		format: "DTS",
		ok:     true,
		dts:    dtsInfo{bitRateBps: 768000},
	}}

	applyMatroskaAudioProbes(&info, probes)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	if got := findField(info.Tracks[0].Fields, "Bit rate"); got != "768 kb/s" {
		t.Fatalf("text bit rate = %q, want core-derived 768 kb/s", got)
	}
	if got := info.Tracks[0].JSON["BitRate"]; got != "768000" {
		t.Fatalf("JSON BitRate = %q, want core-derived 768000", got)
	}
	if got := info.Tracks[0].JSON["BitRate_Mode"]; got != "CBR" {
		t.Fatalf("JSON BitRate_Mode = %q, want CBR", got)
	}
}
