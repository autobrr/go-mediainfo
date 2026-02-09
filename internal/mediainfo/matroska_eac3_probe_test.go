package mediainfo

import "testing"

func putBits(dst []byte, bitPos *int, value uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		bit := (value >> uint(i)) & 1
		byteIdx := *bitPos >> 3
		shift := 7 - (*bitPos & 7)
		if bit == 1 {
			dst[byteIdx] |= 1 << uint(shift)
		}
		*bitPos++
	}
}

func buildEAC3Frame(frameSize int, dialnorm uint64, comprByte uint64) []byte {
	if frameSize%2 != 0 || frameSize < 8 {
		panic("invalid frameSize")
	}
	frmsiz := uint64(frameSize/2 - 1)
	out := make([]byte, frameSize)
	pos := 0
	putBits(out, &pos, 0x0B77, 16)   // syncword
	putBits(out, &pos, 0, 2)         // strmtyp (independent)
	putBits(out, &pos, 0, 3)         // substreamid
	putBits(out, &pos, frmsiz, 11)   // frmsiz
	putBits(out, &pos, 0, 2)         // fscod (48kHz)
	putBits(out, &pos, 3, 2)         // numblkscod (6 blocks => 1536 samples)
	putBits(out, &pos, 2, 3)         // acmod
	putBits(out, &pos, 0, 1)         // lfeon
	putBits(out, &pos, 16, 5)        // bsid (>=10)
	putBits(out, &pos, dialnorm, 5)  // dialnorm
	putBits(out, &pos, 1, 1)         // compre
	putBits(out, &pos, comprByte, 8) // compr (0xFF would mean "unset")
	return out
}

func TestProbeMatroskaAudio_EAC3MultiFramePacket(t *testing.T) {
	const track = 1
	frame := buildEAC3Frame(20, 1, 0x00)
	payload := append(append([]byte{}, frame...), frame...)

	t.Run("packetAligned=false parses multiple frames", func(t *testing.T) {
		probes := map[uint64]*matroskaAudioProbe{
			track: {format: "E-AC-3", collect: true, parseJOC: false},
		}
		probeMatroskaAudio(probes, track, payload, 1, int64(len(payload)), false)
		p := probes[track]
		if p == nil || !p.ok {
			t.Fatalf("probe ok=%v, want true", p != nil && p.ok)
		}
		if p.info.comprCount != 2 {
			t.Fatalf("comprCount=%d, want 2", p.info.comprCount)
		}
		if p.info.dialnormCount != 2 {
			t.Fatalf("dialnormCount=%d, want 2", p.info.dialnormCount)
		}
	})

	t.Run("packetAligned=true rejects multi-frame packet", func(t *testing.T) {
		probes := map[uint64]*matroskaAudioProbe{
			track: {format: "E-AC-3", collect: true, parseJOC: false},
		}
		probeMatroskaAudio(probes, track, payload, 1, int64(len(payload)), true)
		p := probes[track]
		if p == nil {
			t.Fatal("missing probe")
		}
		if p.ok {
			t.Fatalf("probe ok=true, want false")
		}
	})
}
