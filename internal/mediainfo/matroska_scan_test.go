package mediainfo

import (
	"bytes"
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

	applyMatroskaStats(&info, stats, 0)

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
		Kind:   StreamAudio,
		Fields: []Field{{Name: "ID", Value: "1"}, {Name: "Format", Value: "AC-3"}, {Name: "Bit rate mode", Value: "Constant"}},
		JSON:   map[string]string{"BitRate": "767999"},
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
	if got := info.Tracks[0].JSON["BitRate"]; got != "768000" {
		t.Fatalf("BitRate = %q, want 768000", got)
	}
	extra := info.Tracks[0].JSONRaw["extra"]
	for _, field := range []string{"dynrng_Average", "dynrng_Minimum", "dynrng_Maximum", "dynrng_Count"} {
		if !strings.Contains(extra, `"`+field+`"`) {
			t.Fatalf("extra missing %s: %s", field, extra)
		}
	}
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
