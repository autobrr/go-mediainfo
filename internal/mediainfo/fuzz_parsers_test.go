package mediainfo

import (
	"bytes"
	"encoding/binary"
	"testing"
)

const fuzzParserMaxBytes = 1 << 20 // 1 MiB

func fuzzLimit(data []byte) []byte {
	if len(data) > fuzzParserMaxBytes {
		return data[:fuzzParserMaxBytes]
	}
	return data
}

func FuzzParseAC3FrameParsers(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x0B, 0x77})
	f.Add([]byte{0x0B, 0x77, 0x00, 0x00, 0x10, 0x00, 0x00})
	f.Add([]byte{0x0B, 0x77, 0x77, 0x00, 0x00, 0x40, 0x50, 0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		data = fuzzLimit(data)
		_, _, _ = parseAC3Frame(data)
		_, _, _ = parseEAC3FrameWithOptions(data, true)
		_, _, _ = parseEAC3FrameWithOptions(data, false)
	})
}

func FuzzParseMPEGTSPacketizers(f *testing.F) {
	f.Add([]byte{}, uint8(128))
	f.Add(bytes.Repeat([]byte{0x47}, 188), uint8(128))
	f.Add(bytes.Repeat([]byte{0x47}, 192), uint8(128))
	f.Add(append([]byte{0x47, 0x40, 0x00, 0x10}, bytes.Repeat([]byte{0xFF}, 184)...), uint8(160))

	f.Fuzz(func(t *testing.T, data []byte, speed uint8) {
		data = fuzzLimit(data)
		parseSpeed := float64(speed) / 255.0

		tsReader := bytes.NewReader(data)
		_, _, _, _ = ParseMPEGTS(tsReader, int64(len(data)), parseSpeed)

		bdavReader := bytes.NewReader(data)
		_, _, _, _ = ParseBDAV(bdavReader, int64(len(data)), parseSpeed)
	})
}

func FuzzParseMatroskaContainers(f *testing.F) {
	f.Add([]byte{}, uint8(128))
	f.Add([]byte{0x1A, 0x45, 0xDF, 0xA3}, uint8(128))
	f.Add([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x9F, 0x42, 0x86, 0x81, 0x01}, uint8(128))

	f.Fuzz(func(t *testing.T, data []byte, speed uint8) {
		data = fuzzLimit(data)
		opts := defaultAnalyzeOptions()
		opts.HasParseSpeed = true
		opts.ParseSpeed = float64(speed) / 255.0
		opts = normalizeAnalyzeOptions(opts)
		_, _ = ParseMatroskaWithOptions(bytes.NewReader(data), int64(len(data)), opts)
	})
}

// FuzzMatroskaAttachmentSecurity exercises checked size arithmetic, budget
// transitions, and the lazy attachment scanner with a logical file size that is
// independent of the bounded in-memory fuzz payload.
func FuzzMatroskaAttachmentSecurity(f *testing.F) {
	name := buildMatroskaElement(mkvIDFileName, []byte("cover.jpg"))
	attachment := buildMatroskaElement(mkvIDAttachedFile, name)
	f.Add(buildMatroskaElement(mkvIDAttachments, attachment), uint32(0))
	f.Add([]byte{0x19, 0x41, 0xA4, 0x69, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, ^uint32(0))
	f.Add([]byte{}, uint32(0))

	f.Fuzz(func(t *testing.T, data []byte, logicalExtra uint32) {
		data = fuzzLimit(data)
		if len(data) >= 24 {
			start := int64(binary.LittleEndian.Uint64(data[0:8]))
			size := binary.LittleEndian.Uint64(data[8:16])
			limit := int64(binary.LittleEndian.Uint64(data[16:24]))
			_, _ = checkedEmbeddedRange(start, size, limit)
			_, _ = checkedEmbeddedOffset(start, size, limit)
		}

		budget := &embeddedAssetBudget{}
		for pos := 0; pos+9 <= len(data); pos += 9 {
			size := binary.LittleEndian.Uint64(data[pos+1 : pos+9])
			switch data[pos] % 3 {
			case 0:
				_ = budget.reserveItem()
			case 1:
				_ = budget.reserveString(size, embeddedAssetMaxNameBytes)
			case 2:
				_ = budget.reservePayload(size, embeddedAssetMaxPayloadBytes)
			}
		}

		stored := append([]byte{0}, data...)
		extra := int64(logicalExtra % (1 << 20))
		reader := &sparseRecordingReaderAt{data: stored}
		_ = scanMatroskaAttachmentsFromFile(reader, 1, int64(len(stored))+extra, &embeddedAssetBudget{})
		if reader.maxRead > int(embeddedAssetMaxPayloadBytes) {
			t.Fatalf("lazy scanner requested %d bytes", reader.maxRead)
		}
	})
}

func FuzzParseMP4Containers(f *testing.F) {
	// Minimal ftyp box.
	f.Add([]byte{
		0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p',
		'i', 's', 'o', 'm',
		0x00, 0x00, 0x02, 0x00,
		'i', 's', 'o', 'm', 'm', 'p', '4', '2',
	})
	// Minimal moov/mvhd-ish header (not valid; just structure noise).
	f.Add([]byte{
		0x00, 0x00, 0x00, 0x10, 'm', 'o', 'o', 'v',
		0x00, 0x00, 0x00, 0x08, 'm', 'v', 'h', 'd',
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		data = fuzzLimit(data)
		_, _ = ParseMP4(bytes.NewReader(data), int64(len(data)))
	})
}

func FuzzParseAVIContainers(f *testing.F) {
	// Minimal RIFF AVI header.
	f.Add([]byte{
		'R', 'I', 'F', 'F', 0x24, 0x00, 0x00, 0x00,
		'A', 'V', 'I', ' ',
		'L', 'I', 'S', 'T', 0x14, 0x00, 0x00, 0x00,
		'h', 'd', 'r', 'l',
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		data = fuzzLimit(data)
		r := bytes.NewReader(data)
		_, _, _, _ = ParseAVI(r, int64(len(data)))
	})
}

func FuzzParseMPEGPSContainers(f *testing.F) {
	// Minimal pack header start code.
	f.Add([]byte{0x00, 0x00, 0x01, 0xBA, 0x44, 0x00, 0x04, 0x00})
	// Program stream map start code.
	f.Add([]byte{0x00, 0x00, 0x01, 0xBC, 0x00, 0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		data = fuzzLimit(data)
		r := bytes.NewReader(data)
		_, _, _ = ParseMPEGPS(r, int64(len(data)))
	})
}

func FuzzParseFLACContainers(f *testing.F) {
	f.Add([]byte{'f', 'L', 'a', 'C'})
	f.Add([]byte{'f', 'L', 'a', 'C', 0x00, 0x00, 0x00, 0x22})

	f.Fuzz(func(t *testing.T, data []byte) {
		data = fuzzLimit(data)
		r := bytes.NewReader(data)
		_, _, _, _, _ = parseFLAC(r, int64(len(data)))
	})
}

func FuzzParseMP3Containers(f *testing.F) {
	// Minimal ID3 header.
	f.Add([]byte{'I', 'D', '3', 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	// Minimal MPEG audio sync-ish bytes.
	f.Add([]byte{0xFF, 0xFB, 0x90, 0x64})

	f.Fuzz(func(t *testing.T, data []byte) {
		data = fuzzLimit(data)
		r := bytes.NewReader(data)
		_, _, _, _, _ = parseMP3(r, int64(len(data)))
	})
}

func FuzzReadMatroskaBlockHeader(f *testing.F) {
	f1 := makeEAC3FrameForFuzz(16, 1)
	f2 := makeEAC3FrameForFuzz(16, 2)
	seedNoLace := mkvBlockNoLace(append(append([]byte{}, f1...), f2...))
	seedXiph := mkvBlockXiph(f1, f2)
	seedEBML := mkvBlockEBML(f1, f2)

	f.Add(seedNoLace, uint16(len(seedNoLace)), uint8(1))
	f.Add(seedXiph, uint16(len(seedXiph)), uint8(2))
	f.Add(seedEBML, uint16(len(seedEBML)), uint8(3))

	f.Fuzz(func(t *testing.T, data []byte, sizeHint uint16, mode uint8) {
		data = fuzzLimit(data)
		size := int64(sizeHint)
		if max := int64(len(data)); size > max {
			size = max
		}
		audio, video := mkvProbeMaps(mode)
		er := newEBMLReader(bytes.NewReader(data))
		_, _, _, _, _ = readMatroskaBlockHeader(er, size, audio, video, 0)
	})
}

func FuzzScanMatroskaClusters(f *testing.F) {
	f1 := makeEAC3FrameForFuzz(16, 1)
	f2 := makeEAC3FrameForFuzz(16, 2)
	f.Add(mkvClusterWithSimpleBlock(mkvBlockNoLace(f1)), uint8(0), uint8(128))
	f.Add(mkvClusterWithSimpleBlock(mkvBlockNoLace(append(append([]byte{}, f1...), f2...))), uint8(1), uint8(128))
	f.Add(mkvClusterWithSimpleBlock(mkvBlockXiph(f1, f2)), uint8(2), uint8(128))
	f.Add(mkvClusterWithSimpleBlock(mkvBlockEBML(f1, f2)), uint8(3), uint8(128))
	f.Add(mkvClusterWithBlockGroup(mkvBlockNoLace(f1), 1), uint8(4), uint8(128))

	f.Fuzz(func(t *testing.T, data []byte, mode uint8, speed uint8) {
		data = fuzzLimit(data)
		payloadA, payloadB := splitFuzzPayload(data)

		var blob []byte
		switch mode % 4 {
		case 0:
			blob = data
		case 1:
			blob = mkvClusterWithSimpleBlock(mkvBlockNoLace(payloadA))
		case 2:
			blob = mkvClusterWithSimpleBlock(mkvBlockXiph(payloadA, payloadB))
		default:
			blob = mkvClusterWithSimpleBlock(mkvBlockEBML(payloadA, payloadB))
		}

		parseSpeed := float64(speed) / 255.0
		audio, video := mkvProbeMaps(mode >> 4)
		needFirstTimes := map[uint64]struct{}{1: {}}
		applyScan := mode&0x40 != 0
		collectBytes := mode&0x80 != 0
		_, _ = scanMatroskaClusters(bytes.NewReader(blob), 0, int64(len(blob)), 1000000, audio, video, applyScan, collectBytes, parseSpeed, 1, needFirstTimes)
	})
}

func splitFuzzPayload(data []byte) ([]byte, []byte) {
	if len(data) < 2 {
		return []byte{0}, []byte{1}
	}
	mid := len(data) / 2
	if mid <= 0 {
		mid = 1
	}
	return data[:mid], data[mid:]
}

func makeEAC3FrameForFuzz(frameSize int, dialnormCode uint32) []byte {
	if frameSize < 8 {
		frameSize = 8
	}
	if frameSize/2-1 > 0x7FF {
		frameSize = (0x7FF + 1) * 2
	}
	frmsiz := uint32(frameSize/2 - 1)
	b := make([]byte, frameSize)
	w := bitWriter{b: b}
	w.writeBits(0x0B77, 16) // syncword
	w.writeBits(0, 2)       // strmtyp: independent
	w.writeBits(0, 3)       // substreamid
	w.writeBits(frmsiz, 11)
	w.writeBits(0, 2)                 // fscod: 48kHz
	w.writeBits(3, 2)                 // numblkscod: 6 blocks
	w.writeBits(2, 3)                 // acmod: 2/0 (stereo)
	w.writeBits(0, 1)                 // lfeon
	w.writeBits(16, 5)                // bsid
	w.writeBits(dialnormCode&0x1F, 5) // dialnorm
	w.writeBits(0, 1)                 // compre
	return b
}

func mkvBlockNoLace(payload []byte) []byte {
	b := []byte{0x81, 0x00, 0x00, 0x00} // track=1, timecode=0, flags=no lacing
	return append(b, payload...)
}

func mkvBlockXiph(frames ...[]byte) []byte {
	if len(frames) <= 1 {
		if len(frames) == 1 {
			return mkvBlockNoLace(frames[0])
		}
		return mkvBlockNoLace(nil)
	}
	b := []byte{0x81, 0x00, 0x00, 0x02, byte(len(frames) - 1)} // xiph lacing
	for i := 0; i < len(frames)-1; i++ {
		n := len(frames[i])
		for n >= 255 {
			b = append(b, 0xFF)
			n -= 255
		}
		b = append(b, byte(n))
	}
	for _, frame := range frames {
		b = append(b, frame...)
	}
	return b
}

func mkvBlockEBML(frames ...[]byte) []byte {
	if len(frames) <= 1 {
		if len(frames) == 1 {
			return mkvBlockNoLace(frames[0])
		}
		return mkvBlockNoLace(nil)
	}
	b := []byte{0x81, 0x00, 0x00, 0x06, byte(len(frames) - 1)} // ebml lacing
	b = append(b, buildMatroskaSize(uint64(len(frames[0])))...)
	for i := 1; i < len(frames)-1; i++ {
		diff := int64(len(frames[i]) - len(frames[i-1]))
		b = append(b, mkvSignedVint(diff)...)
	}
	for _, frame := range frames {
		b = append(b, frame...)
	}
	return b
}

func mkvSignedVint(value int64) []byte {
	for length := 1; length <= 4; length++ {
		bias := int64(1<<(uint(length*7-1))) - 1
		min := -bias
		max := bias + 1
		if value < min || value > max {
			continue
		}
		u := uint64(value + bias)
		out := make([]byte, length)
		for i := length - 1; i >= 0; i-- {
			out[i] = byte(u)
			u >>= 8
		}
		out[0] |= byte(1 << uint(8-length))
		return out
	}
	return []byte{0x10, 0x00, 0x00, 0x00}
}

func mkvClusterWithSimpleBlock(block []byte) []byte {
	payload := append(buildMatroskaElement(mkvIDTimecode, []byte{0x00}), buildMatroskaElement(mkvIDSimpleBlock, block)...)
	return buildMatroskaElement(mkvIDCluster, payload)
}

func mkvClusterWithBlockGroup(block []byte, duration byte) []byte {
	group := append(buildMatroskaElement(mkvIDBlock, block), buildMatroskaElement(mkvIDBlockDuration, []byte{duration})...)
	payload := append(buildMatroskaElement(mkvIDTimecode, []byte{0x00}), buildMatroskaElement(mkvIDBlockGroup, group)...)
	return buildMatroskaElement(mkvIDCluster, payload)
}

func mkvProbeMaps(mode uint8) (map[uint64]*matroskaAudioProbe, map[uint64]*matroskaVideoProbe) {
	track := uint64(1)
	switch mode % 4 {
	case 0:
		return nil, nil
	case 1:
		return map[uint64]*matroskaAudioProbe{
			track: {
				format:         "E-AC-3",
				collect:        true,
				targetPackets:  8,
				jocStopPackets: 2,
				parseJOC:       true,
			},
		}, nil
	case 2:
		return map[uint64]*matroskaAudioProbe{
			track: {format: "AC-3", collect: true},
		}, nil
	default:
		return map[uint64]*matroskaAudioProbe{
				track: {format: "E-AC-3", collect: true, targetPackets: 8},
			}, map[uint64]*matroskaVideoProbe{
				track: {codec: "HEVC"},
			}
	}
}
