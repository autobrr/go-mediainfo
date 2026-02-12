package mediainfo

import "testing"

func writeBits(dst []byte, bitPos *int, value uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		bit := (value >> uint(i)) & 1
		bytePos := *bitPos >> 3
		shift := 7 - (*bitPos & 7)
		if bit == 1 {
			dst[bytePos] |= 1 << uint(shift)
		}
		*bitPos++
	}
}

func buildDTSCoreFrame(amode uint32, lfe uint32, brCode uint32) []byte {
	// Minimal DTS core frame matching parseDTSCoreFrame bit layout.
	out := make([]byte, 24)
	out[0] = 0x7F
	out[1] = 0xFE
	out[2] = 0x80
	out[3] = 0x01
	pos := 32
	writeBits(out, &pos, 0, 1)      // FrameType
	writeBits(out, &pos, 0, 5)      // Deficit sample count
	writeBits(out, &pos, 0, 1)      // CRC present
	writeBits(out, &pos, 15, 7)     // nblks (16 blocks -> 512 SPF)
	writeBits(out, &pos, 95, 14)    // primary frame bytes - 1
	writeBits(out, &pos, amode, 6)  // channel arrangement
	writeBits(out, &pos, 13, 4)     // sfCode 48 kHz
	writeBits(out, &pos, brCode, 5) // bit rate code
	writeBits(out, &pos, 0, 1)      // downmix
	writeBits(out, &pos, 0, 1)      // dynrng
	writeBits(out, &pos, 0, 1)      // time stamp
	writeBits(out, &pos, 0, 1)      // aux data
	writeBits(out, &pos, 0, 1)      // HDCD
	writeBits(out, &pos, 0, 3)      // ext audio descriptor
	writeBits(out, &pos, 0, 1)      // ext coding
	writeBits(out, &pos, 0, 1)      // sync insertion
	writeBits(out, &pos, lfe, 2)    // LFE
	writeBits(out, &pos, 0, 1)      // predictor history
	writeBits(out, &pos, 0, 1)      // multirate interpolator
	writeBits(out, &pos, 0, 4)      // encoder software rev
	writeBits(out, &pos, 0, 2)      // copy history
	writeBits(out, &pos, 2, 2)      // resolution code (24-bit)
	return out
}

func TestInferBDAVStreamDTS(t *testing.T) {
	payload := buildDTSCoreFrame(7, 1, 15)
	kind, format, stype, ok := inferBDAVStream(0x1101, payload)
	if !ok {
		t.Fatalf("inferBDAVStream: ok=false")
	}
	if kind != StreamAudio || format != "DTS" || stype != 0x82 {
		t.Fatalf("inferBDAVStream: got kind=%v format=%q stype=0x%02X", kind, format, stype)
	}
}

func TestConsumeDTSCoreAndHDExtension(t *testing.T) {
	entry := tsStream{format: "DTS"}
	core := buildDTSCoreFrame(7, 1, 15)
	consumeDTS(&entry, core)
	if !entry.hasAudioInfo {
		t.Fatalf("expected DTS core to set audio info")
	}
	if entry.dtsHD {
		t.Fatalf("expected core-only payload to keep dtsHD=false")
	}
	// MediaInfoLib channel mapping (DTS_Channels) yields AMODE=7 => 4ch plus LFE => 5ch.
	if entry.audioRate != 48000 || entry.audioSpf != 512 || entry.audioChannels != 5 {
		t.Fatalf("unexpected core parse: rate=%v spf=%d channels=%d", entry.audioRate, entry.audioSpf, entry.audioChannels)
	}
	if entry.audioBitRateMode != "Constant" || entry.audioBitRateKbps != 768 {
		t.Fatalf("unexpected core bitrate mode: mode=%q bitrate=%d", entry.audioBitRateMode, entry.audioBitRateKbps)
	}

	consumeDTS(&entry, []byte{0x00, 0x64, 0x58, 0x20, 0x25, 0x00})
	if !entry.dtsHD {
		t.Fatalf("expected DTS-HD extension sync to set dtsHD=true")
	}
	if entry.audioBitRateMode != "Variable" || entry.audioBitRateKbps != 0 {
		t.Fatalf("expected DTS-HD mode switch, got mode=%q bitrate=%d", entry.audioBitRateMode, entry.audioBitRateKbps)
	}
}
