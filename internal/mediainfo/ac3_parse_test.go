package mediainfo

import (
	"strings"
	"testing"
)

type ac3BitWriter struct {
	buf    []byte
	bitPos int
}

func (w *ac3BitWriter) writeBits(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		bit := (v >> uint(i)) & 1
		bytePos := w.bitPos >> 3
		shift := 7 - (w.bitPos & 7)
		if bit == 1 {
			w.buf[bytePos] |= 1 << uint(shift)
		}
		w.bitPos++
	}
}

func TestParseEAC3AudioProductionInfo(t *testing.T) {
	reader := ac3BitReader{data: []byte{0xCC}} // mixlevel=25, roomtyp=2, adconvtyp=0
	var info ac3Info

	if !parseEAC3AudioProductionInfo(&reader, &info) {
		t.Fatal("parseEAC3AudioProductionInfo returned false")
	}
	if !info.hasMixlevel || info.mixlevel != 105 {
		t.Fatalf("mixlevel=%d (present=%v), want 105", info.mixlevel, info.hasMixlevel)
	}
	if !info.hasRoomtyp || info.roomtyp != "Small" {
		t.Fatalf("roomtyp=%q (present=%v), want Small", info.roomtyp, info.hasRoomtyp)
	}
}

func TestParseEAC3FrameWithOptionsPreservesHDCDConversionMetadata(t *testing.T) {
	const frameSize = 32
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)
	bw.writeBits(0, 2)                // independent stream
	bw.writeBits(0, 3)                // substreamid
	bw.writeBits((frameSize/2)-1, 11) // frmsiz
	bw.writeBits(0, 2)                // fscod
	bw.writeBits(3, 2)                // numblkscod
	bw.writeBits(2, 3)                // acmod: stereo
	bw.writeBits(0, 1)                // lfeon
	bw.writeBits(16, 5)               // bsid
	bw.writeBits(25, 5)               // dialnorm
	bw.writeBits(0, 1)                // compre
	bw.writeBits(0, 1)                // mixmdate
	bw.writeBits(1, 1)                // infomdate
	bw.writeBits(0, 3)                // bsmod
	bw.writeBits(0, 2)                // copyright/original
	bw.writeBits(0, 2)                // dsurmod
	bw.writeBits(0, 2)                // dheadphonmod
	bw.writeBits(1, 1)                // audprodie
	bw.writeBits(25, 5)               // mixlevel
	bw.writeBits(2, 2)                // roomtyp: Small
	bw.writeBits(1, 1)                // adconvtyp: HDCD
	bw.writeBits(0, 1)                // source sample rate
	bw.writeBits(0, 1)                // addbsie

	frame, gotSize, ok := parseEAC3FrameWithOptions(buf, false)
	if !ok || gotSize != frameSize {
		t.Fatalf("parse result: ok=%v frameSize=%d want=%d", ok, gotSize, frameSize)
	}
	if !frame.hasAdconvtyp {
		t.Fatal("parsed adconvtyp was discarded during result construction")
	}

	info := MatroskaInfo{Tracks: []Stream{{
		Kind:          StreamAudio,
		Fields:        []Field{{Name: "ID", Value: "1"}, {Name: "Format", Value: "E-AC-3"}},
		canonicalSeed: matroskaAC3CanonicalSeed(matroskaAC3CanonicalFacts{format: "E-AC-3", trackNumber: 1}),
	}}}
	applyMatroskaAudioProbes(&info, map[uint64]*matroskaAudioProbe{1: {format: "E-AC-3", ok: true, info: frame}})
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	if extra := info.Tracks[0].JSONRaw["extra"]; !strings.Contains(extra, `"adconvtyp":"HDCD"`) {
		t.Fatalf("rendered extra missing HDCD adconvtyp: %s", extra)
	}
}

func TestMergeEAC3DependentDualMonoPreservesZeroACModAndLFE(t *testing.T) {
	var info ac3Info
	info.mergeFrame(ac3Info{eac3FrameType: 0, acmod: 2, lfeon: 0, hasLFE: true})
	info.mergeFrame(ac3Info{eac3FrameType: 1, acmod: 0, lfeon: 1, hasLFE: true})
	info.mergeFrame(ac3Info{eac3FrameType: 1, acmod: 2, lfeon: 0, hasLFE: true})
	if !info.hasDependentACMod || info.dependentACMod != 0 || info.dependentLFE != 1 {
		t.Fatalf("dependent metadata = present:%v acmod:%d lfe:%d, want true/0/1", info.hasDependentACMod, info.dependentACMod, info.dependentLFE)
	}

	container := MatroskaInfo{Tracks: []Stream{{
		Kind:          StreamAudio,
		Fields:        []Field{{Name: "ID", Value: "1"}, {Name: "Format", Value: "E-AC-3"}},
		JSON:          map[string]string{},
		canonicalSeed: matroskaAC3CanonicalSeed(matroskaAC3CanonicalFacts{format: "E-AC-3", trackNumber: 1}),
	}}}
	applyMatroskaAudioProbes(&container, map[uint64]*matroskaAudioProbe{1: {format: "E-AC-3", ok: true, info: info}})
	refreshCanonicalCompatibilitySnapshot(&container.Tracks[0])
	extra := container.Tracks[0].JSONRaw["extra"]
	if !strings.Contains(extra, `"acmod":"2 / 0"`) || !strings.Contains(extra, `"lfeon":"0 / 1"`) {
		t.Fatalf("dependent metadata not rendered: %s", extra)
	}
}

func TestParseAC3Frame_Mono_NoCmixlev(t *testing.T) {
	// Frame layout (subset) matching parseAC3Frame():
	// syncword, crc1, fscod, frmsizecod, bsid, bsmod, acmod=1 (mono),
	// lfeon, dialnorm, compre+compr, then enough padding for parseAC3Dynrng().
	const (
		fscod      = 0 // 48 kHz
		frmsizecod = 0 // 32 kbps @ 48 kHz -> 128 bytes
		bsid       = 6 // common in the wild; also exercises xbsi1/xbsi2 parsing branch
		bsmod      = 0
		acmod      = 1 // mono (C) -> must NOT carry cmixlev
		lfeon      = 0
		dialnorm   = 24 // -24 dB
		compre     = 1
		compr      = 0x20
	)

	frameSize := ac3FrameSizeBytes(fscod, frmsizecod)
	if frameSize == 0 {
		t.Fatalf("unexpected frameSize=0 for fscod=%d frmsizecod=%d", fscod, frmsizecod)
	}
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)    // syncword
	bw.writeBits(0x0000, 16)    // crc1
	bw.writeBits(fscod, 2)      // fscod
	bw.writeBits(frmsizecod, 6) // frmsizecod
	bw.writeBits(bsid, 5)       // bsid
	bw.writeBits(bsmod, 3)      // bsmod
	bw.writeBits(acmod, 3)      // acmod
	bw.writeBits(lfeon, 1)      // lfeon
	bw.writeBits(dialnorm, 5)   // dialnorm
	bw.writeBits(compre, 1)     // compre
	bw.writeBits(compr, 8)      // compr
	bw.writeBits(0, 1)          // langcode
	bw.writeBits(0, 1)          // audprodie
	bw.writeBits(0, 1)          // copyrightb
	bw.writeBits(0, 1)          // origbs
	bw.writeBits(0, 1)          // xbsi1e (bsid==6)
	bw.writeBits(0, 1)          // xbsi2e (bsid==6)
	bw.writeBits(0, 1)          // addbsie
	bw.writeBits(0, 1)          // (ac3 dynrng) skip bit 1/2 for nfchans=1
	bw.writeBits(0, 1)          // (ac3 dynrng) skip bit 2/2 for nfchans=1
	bw.writeBits(0, 1)          // dynrnge=0

	info, gotSize, ok := parseAC3Frame(buf)
	if !ok {
		t.Fatalf("parseAC3Frame returned ok=false")
	}
	if gotSize != frameSize {
		t.Fatalf("frameSize mismatch: got=%d want=%d", gotSize, frameSize)
	}
	if info.acmod != acmod {
		t.Fatalf("acmod mismatch: got=%d want=%d", info.acmod, acmod)
	}
	if info.lfeon != lfeon {
		t.Fatalf("lfeon mismatch: got=%d want=%d", info.lfeon, lfeon)
	}
	if info.dialnorm != -24 {
		t.Fatalf("dialnorm mismatch: got=%d want=-24", info.dialnorm)
	}
	if info.hasCmixlev {
		t.Fatalf("mono stream must not set cmixlev (bitstream alignment regression)")
	}
	if !info.compre || !info.hasComprField || !info.hasCompr || info.comprCode != compr {
		t.Fatalf("compr mismatch: compre=%v hasComprField=%v hasCompr=%v comprCode=0x%02x want=0x%02x", info.compre, info.hasComprField, info.hasCompr, info.comprCode, compr)
	}
}

func TestParseAC3Frame_RejectsInvalidBSID(t *testing.T) {
	const (
		fscod      = 0
		frmsizecod = 0
		bsid       = 11 // invalid for core AC-3
	)

	frameSize := ac3FrameSizeBytes(fscod, frmsizecod)
	if frameSize == 0 {
		t.Fatalf("unexpected frameSize=0 for fscod=%d frmsizecod=%d", fscod, frmsizecod)
	}
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)    // syncword
	bw.writeBits(0x0000, 16)    // crc1
	bw.writeBits(fscod, 2)      // fscod
	bw.writeBits(frmsizecod, 6) // frmsizecod
	bw.writeBits(bsid, 5)       // bsid
	bw.writeBits(0, 3)          // bsmod
	bw.writeBits(2, 3)          // acmod
	bw.writeBits(0, 1)          // lfeon
	bw.writeBits(24, 5)         // dialnorm
	bw.writeBits(0, 1)          // compre
	bw.writeBits(0, 1)          // langcode
	bw.writeBits(0, 1)          // audprodie
	bw.writeBits(0, 1)          // copyrightb
	bw.writeBits(0, 1)          // origbs
	bw.writeBits(0, 1)          // timecod1e
	bw.writeBits(0, 1)          // timecod2e
	bw.writeBits(0, 1)          // addbsie
	bw.writeBits(0, 1)          // blksw[0]
	bw.writeBits(0, 1)          // blksw[1]
	bw.writeBits(0, 1)          // dithflag[0]
	bw.writeBits(0, 1)          // dithflag[1]
	bw.writeBits(0, 1)          // dynrnge

	if _, _, ok := parseAC3Frame(buf); ok {
		t.Fatalf("parseAC3Frame unexpectedly accepted invalid bsid=%d", bsid)
	}
}

func TestParseEAC3FrameWithOptions_BoundsJOCScanToFrame(t *testing.T) {
	frame := buildEAC3Frame(20, 1, 0x00)
	trailer := buildEMDFJOCPayload()

	if meta, ok := parseEAC3EMDF(trailer); !ok || !meta.hasJOC {
		t.Fatalf("expected trailer EMDF payload to carry JOC metadata, ok=%v hasJOC=%v", ok, meta.hasJOC)
	}

	info, frameSize, ok := parseEAC3FrameWithOptions(append(append([]byte{}, frame...), trailer...), true)
	if !ok {
		t.Fatal("parseEAC3FrameWithOptions returned ok=false")
	}
	if frameSize != len(frame) {
		t.Fatalf("frameSize mismatch: got=%d want=%d", frameSize, len(frame))
	}
	if info.hasJOC {
		t.Fatal("trailing bytes outside the syncframe must not contribute JOC metadata")
	}
}

func TestParseEAC3DependentFrame_ChannelMap(t *testing.T) {
	const frameSize = 20
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)          // syncword
	bw.writeBits(1, 2)                // dependent stream
	bw.writeBits(0, 3)                // substreamid
	bw.writeBits((frameSize/2)-1, 11) // frmsiz
	bw.writeBits(0, 2)                // fscod: 48 kHz
	bw.writeBits(3, 2)                // numblkscod: 6 blocks / 1536 samples
	bw.writeBits(5, 3)                // acmod
	bw.writeBits(0, 1)                // lfeon
	bw.writeBits(16, 5)               // bsid
	bw.writeBits(25, 5)               // dialnorm
	bw.writeBits(1, 1)                // compre
	bw.writeBits(0xFF, 8)             // compr
	bw.writeBits(1, 1)                // chanmape
	bw.writeBits(0xA010, 16)          // L/R + top-front pair extension map

	info, gotSize, ok := parseEAC3FrameWithOptions(buf, false)
	if !ok {
		t.Fatal("parseEAC3FrameWithOptions returned ok=false")
	}
	if gotSize != frameSize {
		t.Fatalf("frameSize mismatch: got=%d want=%d", gotSize, frameSize)
	}
	if !info.hasEAC3ChannelMap {
		t.Fatal("expected dependent channel map")
	}
	if info.eac3ChannelMapLayout != "L R Tfl Tfr" || info.eac3ChannelMapChannel != 4 {
		t.Fatalf("channel map layout/count mismatch: got=%q/%d", info.eac3ChannelMapLayout, info.eac3ChannelMapChannel)
	}
	if channels, layout := mergeAudioChannelLayouts("L R C LFE Ls Rs", info.eac3ChannelMapLayout); channels != 8 || layout != "L R C LFE Ls Rs Tfl Tfr" {
		t.Fatalf("merged layout mismatch: got=%q/%d", layout, channels)
	}
}

func TestParseEAC3FrameWithOptions_AddBSITypeARequiresObjectMetadata(t *testing.T) {
	const frameSize = 32
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)          // syncword
	bw.writeBits(0, 2)                // independent stream
	bw.writeBits(0, 3)                // substreamid
	bw.writeBits((frameSize/2)-1, 11) // frmsiz
	bw.writeBits(0, 2)                // fscod: 48 kHz
	bw.writeBits(3, 2)                // numblkscod: 6 blocks / 1536 samples
	bw.writeBits(2, 3)                // acmod
	bw.writeBits(0, 1)                // lfeon
	bw.writeBits(16, 5)               // bsid
	bw.writeBits(25, 5)               // dialnorm
	bw.writeBits(0, 1)                // compre
	bw.writeBits(0, 1)                // mixmdate
	bw.writeBits(1, 1)                // infomdate
	bw.writeBits(5, 3)                // bsmod: Commentary
	bw.writeBits(0, 2)                // copyright/original bits
	bw.writeBits(0, 4)                // acmod=2 info metadata
	bw.writeBits(0, 1)                // no audio production info
	bw.writeBits(0, 1)                // source sample rate code
	bw.writeBits(1, 1)                // addbsie
	bw.writeBits(1, 6)                // addbsil: two bytes follow
	bw.writeBits(0, 7)                // reserved/type-A prefix
	bw.writeBits(1, 1)                // flag_ec3_extension_type_a
	bw.writeBits(16, 8)               // complexity_index_type_a

	info, gotSize, ok := parseEAC3FrameWithOptions(buf, false)
	if !ok {
		t.Fatal("parseEAC3FrameWithOptions returned ok=false")
	}
	if gotSize != frameSize {
		t.Fatalf("frameSize mismatch: got=%d want=%d", gotSize, frameSize)
	}
	if info.hasJOC || !info.hasJOCComplex || info.jocComplexity != 16 {
		t.Fatalf("type A metadata mismatch: hasJOC=%v hasComplex=%v complexity=%d", info.hasJOC, info.hasJOCComplex, info.jocComplexity)
	}
	if ac3HasJOCInfo(info) {
		t.Fatal("extension type A without object-audio metadata must not identify Atmos")
	}
	if info.bsmod != 5 || info.serviceKind != "Commentary" {
		t.Fatalf("service kind mismatch: bsmod=%d serviceKind=%q", info.bsmod, info.serviceKind)
	}
	var merged ac3Info
	merged.mergeFrame(info)
	if merged.bsmod != 5 || merged.serviceKind != "Commentary" {
		t.Fatalf("merged service kind mismatch: bsmod=%d serviceKind=%q", merged.bsmod, merged.serviceKind)
	}
	var base ac3Info
	base.mergeFrameBase(info)
	if base.bsmod != 5 || base.serviceKind != "Commentary" {
		t.Fatalf("base service kind mismatch: bsmod=%d serviceKind=%q", base.bsmod, base.serviceKind)
	}
}

func TestParseEAC3FrameWithOptions_MixingMetadata(t *testing.T) {
	const frameSize = 32
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)          // syncword
	bw.writeBits(0, 2)                // independent stream
	bw.writeBits(0, 3)                // substreamid
	bw.writeBits((frameSize/2)-1, 11) // frmsiz
	bw.writeBits(0, 2)                // fscod: 48 kHz
	bw.writeBits(3, 2)                // numblkscod: 6 blocks / 1536 samples
	bw.writeBits(7, 3)                // acmod: 3/2
	bw.writeBits(1, 1)                // lfeon
	bw.writeBits(16, 5)               // bsid
	bw.writeBits(31, 5)               // dialnorm
	bw.writeBits(0, 1)                // compre
	bw.writeBits(1, 1)                // mixmdate
	bw.writeBits(2, 2)                // dmixmod: Lo/Ro
	bw.writeBits(4, 3)                // ltrtcmixlev: -3.0 dB
	bw.writeBits(4, 3)                // lorocmixlev: -3.0 dB
	bw.writeBits(4, 3)                // ltrtsurmixlev: -3.0 dB
	bw.writeBits(4, 3)                // lorosurmixlev: -3.0 dB
	bw.writeBits(0, 1)                // lfemixlevcode
	bw.writeBits(0, 1)                // pgmscle
	bw.writeBits(0, 1)                // extpgmscle
	bw.writeBits(0, 2)                // mixdef
	bw.writeBits(0, 1)                // frmmixcfginfoe
	bw.writeBits(0, 1)                // infomdate
	bw.writeBits(0, 1)                // addbsie

	info, gotSize, ok := parseEAC3FrameWithOptions(buf, false)
	if !ok {
		t.Fatal("parseEAC3FrameWithOptions returned ok=false")
	}
	if gotSize != frameSize {
		t.Fatalf("frameSize mismatch: got=%d want=%d", gotSize, frameSize)
	}
	if !info.hasDmixmod || info.dmixmod != "Lo/Ro" {
		t.Fatalf("dmixmod mismatch: present=%v value=%q", info.hasDmixmod, info.dmixmod)
	}
	for name, got := range map[string]struct {
		present bool
		value   float64
	}{
		"ltrtcmixlev":   {info.hasLtrtcmixlev, info.ltrtcmixlevDB},
		"ltrtsurmixlev": {info.hasLtrtsurmixlev, info.ltrtsurmixlevDB},
		"lorocmixlev":   {info.hasLorocmixlev, info.lorocmixlevDB},
		"lorosurmixlev": {info.hasLorosurmixlev, info.lorosurmixlevDB},
	} {
		if !got.present || got.value != -3.0 {
			t.Errorf("%s mismatch: present=%v value=%v", name, got.present, got.value)
		}
	}
}

func TestParseEAC3FrameWithOptions_DefaultServiceKindWithoutInfoMetadata(t *testing.T) {
	info, _, ok := parseEAC3FrameWithOptions(buildEAC3Frame(20, 25, 0), false)
	if !ok {
		t.Fatal("parseEAC3FrameWithOptions returned ok=false")
	}
	if info.bsmod != 0 || info.serviceKind != "Complete Main" {
		t.Fatalf("default service kind mismatch: bsmod=%d serviceKind=%q", info.bsmod, info.serviceKind)
	}
}

func TestParseEAC3FrameWithOptions_ConvertedStreamMetadataAlignment(t *testing.T) {
	const frameSize = 32
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)
	bw.writeBits(2, 2)                // strmtyp: AC-3 converted
	bw.writeBits(0, 3)                // substreamid
	bw.writeBits((frameSize/2)-1, 11) // frmsiz
	bw.writeBits(0, 2)                // fscod
	bw.writeBits(3, 2)                // numblkscod
	bw.writeBits(2, 3)                // acmod
	bw.writeBits(0, 1)                // lfeon
	bw.writeBits(16, 5)               // bsid
	bw.writeBits(0, 5)                // dialnorm
	bw.writeBits(0, 1)                // compre
	bw.writeBits(0, 1)                // mixmdate
	bw.writeBits(1, 1)                // infomdate
	bw.writeBits(5, 3)                // bsmod: Commentary
	bw.writeBits(0, 2)                // copyright/original
	bw.writeBits(1, 2)                // dsurmod: Dolby Surround encoded
	bw.writeBits(0, 2)                // dheadphonmod
	bw.writeBits(0, 1)                // audprodie
	bw.writeBits(0, 1)                // source sample rate
	bw.writeBits(0, 6)                // convsync
	bw.writeBits(0, 1)                // addbsie

	info, gotSize, ok := parseEAC3FrameWithOptions(buf, false)
	if !ok || gotSize != frameSize {
		t.Fatalf("parse result: ok=%v frameSize=%d want=%d", ok, gotSize, frameSize)
	}
	if info.hasDmixmod {
		t.Fatalf("unexpected downmix metadata: %q", info.dmixmod)
	}
	if info.bsmod != 5 || info.serviceKind != "Commentary" {
		t.Fatalf("service kind mismatch: bsmod=%d serviceKind=%q", info.bsmod, info.serviceKind)
	}
	if !info.hasDsurmod || info.dsurmod != 1 {
		t.Fatalf("dsurmod mismatch: present=%v value=%d", info.hasDsurmod, info.dsurmod)
	}
}

func TestParseEAC3FrameWithOptions_DualMonoMetadataAlignment(t *testing.T) {
	const frameSize = 32
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)
	bw.writeBits(0, 2)                // independent stream
	bw.writeBits(0, 3)                // substreamid
	bw.writeBits((frameSize/2)-1, 11) // frmsiz
	bw.writeBits(0, 2)                // fscod
	bw.writeBits(3, 2)                // numblkscod
	bw.writeBits(0, 3)                // acmod: dual mono
	bw.writeBits(0, 1)                // lfeon
	bw.writeBits(16, 5)               // bsid
	bw.writeBits(25, 5)               // dialnorm program 1
	bw.writeBits(0, 1)                // compre program 1
	bw.writeBits(0, 5)                // dialnorm program 2
	bw.writeBits(0, 1)                // compre program 2
	bw.writeBits(0, 1)                // mixmdate
	bw.writeBits(1, 1)                // infomdate
	bw.writeBits(5, 3)                // bsmod: Commentary
	bw.writeBits(0, 2)                // copyright/original
	bw.writeBits(0, 1)                // audprodie program 1
	bw.writeBits(0, 1)                // audprodie program 2
	bw.writeBits(0, 1)                // source sample rate
	bw.writeBits(0, 1)                // addbsie

	info, gotSize, ok := parseEAC3FrameWithOptions(buf, false)
	if !ok || gotSize != frameSize {
		t.Fatalf("parse result: ok=%v frameSize=%d want=%d", ok, gotSize, frameSize)
	}
	if info.dialnorm != -25 || info.dialnormCount != 1 {
		t.Fatalf("first-program dialnorm mismatch: value=%d count=%d", info.dialnorm, info.dialnormCount)
	}
	if info.bsmod != 5 || info.serviceKind != "Commentary" {
		t.Fatalf("service kind mismatch: bsmod=%d serviceKind=%q", info.bsmod, info.serviceKind)
	}
}

func TestParseEAC3FrameWithOptions_MixDef3MetadataAlignment(t *testing.T) {
	const frameSize = 64
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)
	bw.writeBits(0, 2)                // independent stream
	bw.writeBits(0, 3)                // substreamid
	bw.writeBits((frameSize/2)-1, 11) // frmsiz
	bw.writeBits(0, 2)                // fscod
	bw.writeBits(3, 2)                // numblkscod
	bw.writeBits(7, 3)                // acmod: 3/2
	bw.writeBits(0, 1)                // lfeon
	bw.writeBits(16, 5)               // bsid
	bw.writeBits(31, 5)               // dialnorm
	bw.writeBits(0, 1)                // compre
	bw.writeBits(1, 1)                // mixmdate
	bw.writeBits(2, 2)                // dmixmod: Lo/Ro
	bw.writeBits(4, 3)                // ltrtcmixlev
	bw.writeBits(4, 3)                // lorocmixlev
	bw.writeBits(4, 3)                // ltrtsurmixlev
	bw.writeBits(4, 3)                // lorosurmixlev
	bw.writeBits(0, 1)                // pgmscle
	bw.writeBits(0, 1)                // extpgmscle
	bw.writeBits(3, 2)                // mixdef
	bw.writeBits(0, 5)                // mixdeflen: two-byte payload
	bw.writeBits(1, 1)                // mixdata2e
	bw.writeBits(0, 6)                // premixcmpsel
	bw.writeBits(0, 1)                // drcsrc
	bw.writeBits(0, 3)                // premixcmpscl
	for range 7 {
		bw.writeBits(0, 1) // extpgm*scle/dmixscle
	}
	bw.writeBits(1, 1) // addche
	bw.writeBits(0, 1) // extpgmaux1scle
	bw.writeBits(0, 1) // extpgmaux2scle
	bw.writeBits(1, 1) // mixdata3e
	bw.writeBits(0, 5) // spchdat
	bw.writeBits(1, 1) // addspchdate
	bw.writeBits(0, 7) // spchdat1 + spchan1att
	bw.writeBits(1, 1) // addspdat1e
	bw.writeBits(0, 7) // spchdat2 + spchan2att
	bw.writeBits(0, 16)
	bw.writeBits(0, 1) // frmmixcfginfoe
	bw.writeBits(1, 1) // infomdate
	bw.writeBits(5, 3) // bsmod: Commentary
	bw.writeBits(0, 2) // copyright/original
	bw.writeBits(0, 2) // acmod>=6 metadata
	bw.writeBits(0, 1) // audprodie
	bw.writeBits(0, 1) // source sample rate
	bw.writeBits(0, 1) // addbsie

	info, gotSize, ok := parseEAC3FrameWithOptions(buf, false)
	if !ok || gotSize != frameSize {
		t.Fatalf("parse result: ok=%v frameSize=%d want=%d", ok, gotSize, frameSize)
	}
	if !info.hasDmixmod || info.dmixmod != "Lo/Ro" {
		t.Fatalf("dmixmod mismatch: present=%v value=%q", info.hasDmixmod, info.dmixmod)
	}
	if info.bsmod != 5 || info.serviceKind != "Commentary" {
		t.Fatalf("service kind mismatch: bsmod=%d serviceKind=%q", info.bsmod, info.serviceKind)
	}
}

func TestParseEAC3FrameWithOptions_Acmod4ReadsSurroundMixLevels(t *testing.T) {
	const frameSize = 32
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)
	bw.writeBits(0, 2)                // independent stream
	bw.writeBits(0, 3)                // substreamid
	bw.writeBits((frameSize/2)-1, 11) // frmsiz
	bw.writeBits(0, 2)                // fscod
	bw.writeBits(3, 2)                // numblkscod
	bw.writeBits(4, 3)                // acmod: 2/1
	bw.writeBits(0, 1)                // lfeon
	bw.writeBits(16, 5)               // bsid
	bw.writeBits(31, 5)               // dialnorm
	bw.writeBits(0, 1)                // compre
	bw.writeBits(1, 1)                // mixmdate
	bw.writeBits(3, 2)                // dmixmod: reserved literal
	bw.writeBits(4, 3)                // ltrtsurmixlev: -3.0 dB
	bw.writeBits(4, 3)                // lorosurmixlev: -3.0 dB
	bw.writeBits(0, 1)                // pgmscle
	bw.writeBits(0, 1)                // extpgmscle
	bw.writeBits(0, 2)                // mixdef
	bw.writeBits(0, 1)                // frmmixcfginfoe
	bw.writeBits(1, 1)                // infomdate
	bw.writeBits(5, 3)                // bsmod: Commentary
	bw.writeBits(0, 2)                // copyright/original
	bw.writeBits(0, 1)                // audprodie
	bw.writeBits(0, 1)                // source sample rate
	bw.writeBits(0, 1)                // addbsie

	info, gotSize, ok := parseEAC3FrameWithOptions(buf, false)
	if !ok || gotSize != frameSize {
		t.Fatalf("parse result: ok=%v frameSize=%d want=%d", ok, gotSize, frameSize)
	}
	if !info.hasDmixmod || info.dmixmod != "3" {
		t.Fatalf("dmixmod mismatch: present=%v value=%q", info.hasDmixmod, info.dmixmod)
	}
	if !info.hasLtrtsurmixlev || !info.hasLorosurmixlev {
		t.Fatal("acmod=4 must read surround mix-level fields")
	}
	if info.bsmod != 5 || info.serviceKind != "Commentary" {
		t.Fatalf("service kind mismatch: bsmod=%d serviceKind=%q", info.bsmod, info.serviceKind)
	}
}

func TestParseEAC3FrameWithOptions_AddBSITypeARequiresTwoBytes(t *testing.T) {
	const frameSize = 32
	buf := make([]byte, frameSize)
	bw := ac3BitWriter{buf: buf}

	bw.writeBits(0x0B77, 16)
	bw.writeBits(0, 2)                // independent stream
	bw.writeBits(0, 3)                // substreamid
	bw.writeBits((frameSize/2)-1, 11) // frmsiz
	bw.writeBits(0, 2)                // fscod
	bw.writeBits(3, 2)                // numblkscod
	bw.writeBits(2, 3)                // acmod
	bw.writeBits(0, 1)                // lfeon
	bw.writeBits(16, 5)               // bsid
	bw.writeBits(25, 5)               // dialnorm
	bw.writeBits(0, 1)                // compre
	bw.writeBits(0, 1)                // mixmdate
	bw.writeBits(0, 1)                // infomdate
	bw.writeBits(1, 1)                // addbsie
	bw.writeBits(0, 6)                // addbsil: one byte follows
	bw.writeBits(0, 7)                // reserved/type-A prefix
	bw.writeBits(1, 1)                // flag_ec3_extension_type_a
	bw.writeBits(127, 8)              // first byte past addbsi; not complexity

	info, gotSize, ok := parseEAC3FrameWithOptions(buf, false)
	if !ok || gotSize != frameSize {
		t.Fatalf("parse result: ok=%v frameSize=%d want=%d", ok, gotSize, frameSize)
	}
	if info.hasJOCComplex || info.jocComplexity != 0 {
		t.Fatalf("single-byte addbsi fabricated complexity: present=%v value=%d", info.hasJOCComplex, info.jocComplexity)
	}
}

func buildEMDFJOCPayload() []byte {
	body := make([]byte, 6)
	bw := ac3BitWriter{buf: body}
	bw.writeBits(0, 2)  // version
	bw.writeBits(0, 3)  // key_id
	bw.writeBits(14, 5) // payload_id: JOC header
	bw.writeBits(0, 1)  // smploffste
	bw.writeBits(0, 1)  // duratione
	bw.writeBits(0, 1)  // groupide
	bw.writeBits(0, 1)  // codecdatae
	bw.writeBits(1, 1)  // discard_unknown_payload
	bw.writeBits(2, 8)  // payload_size
	bw.writeBits(0, 1)  // no variable-bit continuation
	bw.writeBits(0, 3)  // joc_dmx_config_idx
	bw.writeBits(0, 6)  // one JOC object
	bw.writeBits(0, 3)  // joc_ext_config_idx
	bw.writeBits(0, 5)  // payload_id terminator
	return append([]byte{0x58, 0x38, 0x00, byte(len(body))}, body...)
}

func TestAC3FrameCRCValid(t *testing.T) {
	frame := make([]byte, 128)
	frame[0], frame[1] = 0x0B, 0x77
	if !ac3FrameCRCValid(frame, 8) {
		t.Fatal("zero-remainder AC-3 frame rejected")
	}
	frame[10] = 1
	if ac3FrameCRCValid(frame, 8) {
		t.Fatal("corrupt AC-3 frame accepted")
	}
}
