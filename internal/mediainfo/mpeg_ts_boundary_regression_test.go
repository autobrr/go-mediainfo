package mediainfo

import (
	"math"
	"testing"
)

func TestBDAVHeadBoundaryRetainsAC3CompressionStats(t *testing.T) {
	const (
		headEnd    = int64(64 << 10)
		packetSize = int64(192)
		comprCode  = byte(0x20)
	)
	alignedEnd := boundedTransportHeadScanEnd(headEnd, 0, packetSize, true)
	if alignedEnd != 98_304 {
		t.Fatalf("aligned end=%d, want 98304", alignedEnd)
	}

	frame := buildBoundaryAC3Frame(t, comprCode)
	start := int(headEnd) - len(frame)/2
	data := make([]byte, alignedEnd)
	copy(data[start:], frame)
	copy(data[start+len(frame):], frame)

	entry := &tsStream{format: "AC-3", audioStarted: true}
	consumeAC3(entry, data[:headEnd], true, true, true)
	if entry.audioFramesStats != 0 || len(entry.audioBuffer) != len(frame)/2 {
		t.Fatalf("64 KiB edge prematurely completed frame: frames=%d buffered=%d", entry.audioFramesStats, len(entry.audioBuffer))
	}
	consumeAC3(entry, data[headEnd:alignedEnd], true, true, true)
	average, _, _, count, ok := entry.ac3Stats.comprStats()
	if !ok || count != 2 {
		t.Fatalf("BDAV refill compression stats ok=%v count=%d", ok, count)
	}
	if want := ac3ComprDB(comprCode); math.Abs(average-want) > 0.0001 {
		t.Fatalf("compression average=%f, want %f", average, want)
	}
	if entry.audioFramesStats != 2 {
		t.Fatalf("stats frames=%d, want 2", entry.audioFramesStats)
	}
}

func buildBoundaryAC3Frame(t *testing.T, compr byte) []byte {
	t.Helper()
	const frameSize = 128
	frame := make([]byte, frameSize)
	writer := ac3BitWriter{buf: frame}
	writer.writeBits(0x0B77, 16)
	writer.writeBits(0, 16) // crc1, fixed below
	writer.writeBits(0, 2)  // fscod: 48 kHz
	writer.writeBits(0, 6)  // frmsizecod: 128 bytes
	writer.writeBits(8, 5)  // bsid
	writer.writeBits(0, 3)  // bsmod
	writer.writeBits(1, 3)  // acmod: mono
	writer.writeBits(0, 1)  // lfeon
	writer.writeBits(24, 5) // dialnorm
	writer.writeBits(1, 1)  // compre
	writer.writeBits(uint32(compr), 8)
	writer.writeBits(0, 1) // langcode
	writer.writeBits(0, 1) // audprodie
	writer.writeBits(0, 1) // copyrightb
	writer.writeBits(0, 1) // origbs
	writer.writeBits(0, 1) // timecod1e
	writer.writeBits(0, 1) // timecod2e
	writer.writeBits(0, 1) // addbsie
	writer.writeBits(0, 1) // dynrng skip
	writer.writeBits(0, 1) // dynrng skip
	writer.writeBits(0, 1) // dynrnge
	fixBoundaryAC3CRC(t, frame)
	info, size, ok := parseAC3Frame(frame)
	if !ok || size != frameSize || !info.compre || info.comprCode != compr {
		t.Fatalf("generated frame invalid: ok=%v size=%d compre=%v compr=0x%02x", ok, size, info.compre, info.comprCode)
	}
	if !ac3CRCValid(frame, info.bsid) {
		t.Fatal("generated frame CRC invalid")
	}
	return frame
}

func fixBoundaryAC3CRC(t *testing.T, frame []byte) {
	t.Helper()
	const intermediateEnd = 80
	found := false
	for value := 0; value <= 0xFFFF; value++ {
		frame[2], frame[3] = byte(value>>8), byte(value)
		if boundaryAC3CRC(frame[2:intermediateEnd]) == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("could not synthesize AC-3 crc1")
	}
	frame[len(frame)-3] &^= 1 // disable legacy final-byte inversion branch
	found = false
	for value := 0; value <= 0xFFFF; value++ {
		frame[len(frame)-2], frame[len(frame)-1] = byte(value>>8), byte(value)
		if ac3CRCValid(frame, 8) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("could not synthesize AC-3 crc2")
	}
}

func boundaryAC3CRC(data []byte) uint16 {
	var crc uint16
	for _, value := range data {
		crc = (crc << 8) ^ ac3CRC16Table[byte(crc>>8)^value]
	}
	return crc
}
