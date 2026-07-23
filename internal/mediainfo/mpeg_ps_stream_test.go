package mediainfo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewPSStreamParserQuickAC3(t *testing.T) {
	if !newPSStreamParser(mpegPSOptions{parseSpeed: 0.5}).quickAC3 {
		t.Fatalf("expected quickAC3 at low parse speed")
	}
	if newPSStreamParser(mpegPSOptions{parseSpeed: 1}).quickAC3 {
		t.Fatalf("did not expect quickAC3 at full parse speed")
	}
	if newPSStreamParser(mpegPSOptions{parseSpeed: 0.5, dvdExtras: true}).quickAC3 {
		t.Fatalf("did not expect quickAC3 with dvd extras")
	}
}

func TestMPEGPSDVDEdgeWindows(t *testing.T) {
	const window = int64((1 << 20) + 1)
	tests := []struct {
		name     string
		opts     mpegPSOptions
		wantHead int64
		wantTail int64
	}{
		{name: "non-DVD", wantHead: window, wantTail: window},
		{name: "DVD title", opts: mpegPSOptions{dvdParsing: true}, wantHead: 17 << 16, wantTail: window},
		{name: "DVD menu", opts: mpegPSOptions{dvdParsing: true, dvdMenu: true}, wantHead: window, wantTail: window},
		{name: "DVD wide title", opts: mpegPSOptions{dvdParsing: true, dvdWideWindow: true}, wantHead: 17 << 16, wantTail: window - (8 << 10)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			head, tail := mpegPSEdgeWindows(window, test.opts)
			if head != test.wantHead || tail != test.wantTail {
				t.Fatalf("mpegPSEdgeWindows() = (%d, %d), want (%d, %d)", head, tail, test.wantHead, test.wantTail)
			}
		})
	}

	_, tail := mpegPSEdgeWindows(4<<10, mpegPSOptions{dvdParsing: true, dvdWideWindow: true})
	if tail != 0 {
		t.Fatalf("small wide tail = %d, want 0", tail)
	}
}

func TestParseMPEGPSFileEdgesDoesNotRescanShortFile(t *testing.T) {
	data := []byte{
		0x00, 0x00, 0x01, 0xBA,
		0x21, 0x00, 0x01, 0x00, 0x01, 0x80, 0x1B, 0x91,
		0x00, 0x00, 0x01, 0xE0, 0x00, 0x09,
		0x21, 0x00, 0x01, 0x00, 0x01,
		0x00, 0x00, 0x01, 0xB3,
	}
	path := filepath.Join(t.TempDir(), "short.mpg")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	single := newPSStreamParser(mpegPSOptions{})
	if !single.parseReader(bytes.NewReader(data)) {
		t.Fatal("single parse did not find MPEG-PS data")
	}

	parser, sampled, parsed := parseMPEGPSFileEdges([]string{path}, int64(len(data)), mpegPSOptions{})
	if !parsed {
		t.Fatal("edge parse did not find MPEG-PS data")
	}
	if sampled != int64(len(data)) {
		t.Fatalf("sampled bytes = %d, want %d", sampled, len(data))
	}
	if parser.section != 0 {
		t.Fatalf("parser section = %d, want no tail section", parser.section)
	}
	key := psStreamKey(0xE0, psSubstreamNone)
	if parser.streams[key].bytes != single.streams[key].bytes {
		t.Fatalf("stream bytes = %d, want single-pass %d", parser.streams[key].bytes, single.streams[key].bytes)
	}
}

func TestParseMPEGPSFileEdgesStillSamplesNonOverlappingTail(t *testing.T) {
	data := []byte{
		0x00, 0x00, 0x01, 0xBA,
		0x21, 0x00, 0x01, 0x00, 0x01, 0x80, 0x1B, 0x91,
		0x00, 0x00, 0x01, 0xE0, 0x00, 0x09,
		0x21, 0x00, 0x01, 0x00, 0x01,
		0x00, 0x00, 0x01, 0xB3,
	}
	payload := append(append(append([]byte(nil), data...), bytes.Repeat([]byte{0xFF}, len(data))...), data...)
	path := filepath.Join(t.TempDir(), "long.mpg")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	single := newPSStreamParser(mpegPSOptions{})
	if !single.parseReader(bytes.NewReader(data)) {
		t.Fatal("single parse did not find MPEG-PS data")
	}

	parser, sampled, parsed := parseMPEGPSFileEdges([]string{path}, int64(len(data)), mpegPSOptions{})
	if !parsed {
		t.Fatal("edge parse did not find MPEG-PS data")
	}
	if sampled != int64(2*len(data)) {
		t.Fatalf("sampled bytes = %d, want %d", sampled, 2*len(data))
	}
	if parser.section != 1 {
		t.Fatalf("parser section = %d, want tail section", parser.section)
	}
	key := psStreamKey(0xE0, psSubstreamNone)
	if parser.streams[key].bytes != 2*single.streams[key].bytes {
		t.Fatalf("stream bytes = %d, want two passes of %d", parser.streams[key].bytes, single.streams[key].bytes)
	}
}

func TestPSStreamParserBeginSectionDropsDiscontinuousState(t *testing.T) {
	parser := newPSStreamParser(mpegPSOptions{})
	stream := parser.ensureStream(0xE0, psSubstreamNone, StreamVideo, "MPEG Video")
	stream.audioBuffer = []byte{1}
	stream.videoHeaderCarry = []byte{2}
	stream.videoFrameCarry = []byte{3}
	stream.videoCCCarry = []byte{4}
	stream.videoBuffer = []byte{5}
	stream.clockHasPTS = true
	stream.programEndSeen = true
	stream.terminalTracked = true
	stream.terminalBytes = []byte{6}
	videoParser := &mpeg2VideoParser{
		carry:             []byte{7},
		rescanFromZero:    true,
		expectPictureExt:  true,
		currentGOPCount:   8,
		finalGOPRecorded:  true,
		sawGOP:            true,
		framesSinceI:      9,
		framesSinceAnchor: 10,
		lastISeen:         true,
		lastAnchorSeen:    true,
	}
	parser.videoParsers[psStreamKey(0xE0, psSubstreamNone)] = videoParser

	parser.beginSection()
	if parser.section != 1 || stream.sampleSection != 1 {
		t.Fatalf("section = %d, stream section = %d", parser.section, stream.sampleSection)
	}
	if stream.audioBuffer != nil || stream.videoHeaderCarry != nil || stream.videoFrameCarry != nil || stream.videoCCCarry != nil || stream.videoBuffer != nil {
		t.Fatalf("stream carry survived section jump: %#v", stream)
	}
	if stream.clockHasPTS || stream.programEndSeen || stream.terminalTracked || stream.terminalBytes != nil {
		t.Fatalf("stream lifecycle survived section jump: %#v", stream)
	}
	if videoParser.carry != nil || videoParser.rescanFromZero || videoParser.expectPictureExt || videoParser.currentGOPCount != 0 || videoParser.finalGOPRecorded || videoParser.sawGOP || videoParser.framesSinceI != 0 || videoParser.framesSinceAnchor != 0 || videoParser.lastISeen || videoParser.lastAnchorSeen {
		t.Fatalf("video parser reconstruction survived section jump: %#v", videoParser)
	}
}

func TestPSStreamParserBoundsProgramEndTailForEveryStream(t *testing.T) {
	parser := newPSStreamParser(mpegPSOptions{parseSpeed: 1})
	first := parser.ensureStream(0xC0, psSubstreamNone, StreamAudio, "MPEG Audio")
	second := parser.ensureStream(0xE0, psSubstreamNone, StreamVideo, "MPEG Video")
	tail := bytes.Repeat([]byte{0xA5}, mpegPSTerminalTailMax*8)
	data := append([]byte{0, 0, 1, 0xB9}, tail...)
	if !parser.parseReader(bytes.NewReader(data)) {
		t.Fatal("parseReader() did not consume program end")
	}
	for name, stream := range map[string]*psStream{"audio": first, "video": second} {
		if !stream.terminalTracked || len(stream.terminalBytes) != mpegPSTerminalTailMax {
			t.Fatalf("%s terminal state = tracked %v, bytes %d", name, stream.terminalTracked, len(stream.terminalBytes))
		}
		if !bytes.Equal(stream.terminalBytes, tail[:mpegPSTerminalTailMax]) {
			t.Fatalf("%s terminal bytes = %x", name, stream.terminalBytes)
		}
	}
}

func TestPSStreamParserZeroLengthPayloadAtBufferEnd(_ *testing.T) {
	data := []byte{0x00, 0x00, 0x01, 0xDE, 0x00, 0x04, 0xAC, 0x30, 0x01, 0x30}
	parser := newPSStreamParser(mpegPSOptions{})
	parser.parseReader(bytes.NewReader(data))
}

func TestPSConsumePayloadQuickAC3SkipsDecode(t *testing.T) {
	parser := &psStreamParser{quickAC3: true, quickAC3Max: 4}
	entry := &psStream{
		kind:        StreamAudio,
		format:      "AC-3",
		hasAC3:      true,
		audioFrames: 7,
	}
	payload := []byte{0x0B, 0x77, 0x00, 0x00, 0x00, 0x00}
	parser.consumePayload(entry, 0, 0, 0, false, payload)

	if entry.bytes != uint64(len(payload)) {
		t.Fatalf("unexpected bytes: got %d want %d", entry.bytes, len(payload))
	}
	if entry.audioFrames != 7 {
		t.Fatalf("unexpected audio frame decode: got %d want 7", entry.audioFrames)
	}
}
