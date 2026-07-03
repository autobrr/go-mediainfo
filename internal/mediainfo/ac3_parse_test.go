package mediainfo

import "testing"

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

func TestParseEAC3FrameWithOptions_AddBSITypeAAtmos(t *testing.T) {
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
	if !info.hasJOC || !info.hasJOCComplex || info.jocComplexity != 16 {
		t.Fatalf("JOC metadata mismatch: hasJOC=%v hasComplex=%v complexity=%d", info.hasJOC, info.hasJOCComplex, info.jocComplexity)
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
