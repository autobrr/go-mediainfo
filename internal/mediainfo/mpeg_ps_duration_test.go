package mediainfo

import (
	"bytes"
	"testing"
	"time"
)

func TestPSStreamParserAcceptsMPEG1PackAndPESHeaders(t *testing.T) {
	data := []byte{
		0x00, 0x00, 0x01, 0xBA,
		0x21, 0x00, 0x01, 0x00, 0x01, 0x80, 0x1B, 0x91,
		0x00, 0x00, 0x01, 0xE0, 0x00, 0x09,
		0x21, 0x00, 0x01, 0x00, 0x01,
		0x00, 0x00, 0x01, 0xB3,
	}
	parser := newPSStreamParser(mpegPSOptions{})
	if !parser.parseReader(bytes.NewReader(data)) {
		t.Fatal("parseReader() = false, want MPEG-1 payload")
	}
	stream := parser.streams[psStreamKey(0xE0, psSubstreamNone)]
	if stream == nil {
		t.Fatal("MPEG-1 video stream was not registered")
	}
	if !stream.pts.has() || !parser.videoPTS.has() {
		t.Fatal("MPEG-1 PTS was not recorded")
	}
	if len(parser.streams) != 1 {
		t.Fatalf("streams = %d, want 1", len(parser.streams))
	}
}

func TestPSStreamParserRejectsMalformedMPEG1PESHeaders(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "declared packet end exceeds input",
			data: []byte{0, 0, 1, 0xE0, 0, 16, 0x0F, 0, 0, 1, 0xB3},
		},
		{
			name: "more than sixteen stuffing bytes",
			data: append(append([]byte{0, 0, 1, 0xE0, 0, 22}, bytes.Repeat([]byte{0xFF}, 17)...), 0x0F, 0, 0, 1, 0xB3),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := newPSStreamParser(mpegPSOptions{})
			if parser.parseReader(bytes.NewReader(test.data)) {
				t.Fatal("parseReader() accepted malformed MPEG-1 PES")
			}
			if len(parser.streams) != 0 || parser.anyPTS.has() {
				t.Fatalf("malformed PES produced %d streams, PTS=%v", len(parser.streams), parser.anyPTS.has())
			}
		})
	}
}

func TestPSStreamParserRejectsTruncatedMPEG1PESSTDHeader(t *testing.T) {
	data := []byte{0, 0, 1, 0xE0, 0, 2, 0x40, 0}
	result := make(chan bool, 1)
	parser := newPSStreamParser(mpegPSOptions{})
	go func() {
		result <- parser.parseReader(bytes.NewReader(data))
	}()

	select {
	case parsed := <-result:
		if parsed {
			t.Fatal("parseReader() accepted truncated MPEG-1 PES")
		}
		if len(parser.streams) != 0 || parser.anyPTS.has() {
			t.Fatalf("truncated PES produced %d streams, PTS=%v", len(parser.streams), parser.anyPTS.has())
		}
	case <-time.After(time.Second):
		t.Fatal("parseReader() did not terminate for truncated MPEG-1 PES")
	}
}

func TestMPEG1SequenceHeaderUsesMPEG1DisplaySemantics(t *testing.T) {
	parser := &mpeg2VideoParser{}
	header := append([]byte{0x16, 0x00, 0xF0, 0xC4, 0x02, 0x8A, 0xE0, 0x96}, make([]byte, 128)...)
	parser.parseSequenceHeader(header)
	info := parser.finalize()
	if info.Version != "Version 1" {
		t.Fatalf("Version = %q, want Version 1", info.Version)
	}
	if info.AspectRatio != "1.304" {
		t.Fatalf("AspectRatio = %q, want 1.304", info.AspectRatio)
	}
	if info.ScanType != "Progressive" {
		t.Fatalf("ScanType = %q, want Progressive", info.ScanType)
	}
}

func TestParseMPEGVideoWritingLibrary(t *testing.T) {
	library, name, version := parseMPEGVideoWritingLibrary([]byte("\x00\x87\x81\x00%encoded by TMPGEnc (ver. 2.59.47.155)\x00"))
	if library != "encoded by TMPGEnc (ver. 2.59.47.155)" || name != "TMPGEnc" || version != "2.59.47.155" {
		t.Fatalf("TMPGEnc = %q, %q, %q", library, name, version)
	}
	library, name, version = parseMPEGVideoWritingLibrary([]byte("(c)2004 Saar Software\x00"))
	if library != "(c)2004 Saar Software" || name != library || version != "" {
		t.Fatalf("Saar = %q, %q, %q", library, name, version)
	}
}

func TestParseMPEGVideoWritingLibraryUsesFirstRecognizedRun(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		wantLibrary string
		wantName    string
		wantVersion string
	}{
		{
			name:        "TMPGEnc before longer unrelated run",
			data:        "encoded by TMPGEnc (ver. 2.59.47.155)\x00this unrelated printable run is deliberately much longer than the encoder string",
			wantLibrary: "encoded by TMPGEnc (ver. 2.59.47.155)",
			wantName:    "TMPGEnc",
			wantVersion: "2.59.47.155",
		},
		{
			name:        "Saar before longer unrelated run",
			data:        "(c)2004 Saar Software\x00this unrelated printable run is deliberately much longer than the encoder string",
			wantLibrary: "(c)2004 Saar Software",
			wantName:    "(c)2004 Saar Software",
		},
		{
			name: "rejected runs only",
			data: "unrelated encoder\x00another longer unrelated printable run",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			library, name, version := parseMPEGVideoWritingLibrary([]byte(test.data))
			if library != test.wantLibrary || name != test.wantName || version != test.wantVersion {
				t.Fatalf("library = %q, %q, %q; want %q, %q, %q", library, name, version, test.wantLibrary, test.wantName, test.wantVersion)
			}
		})
	}
}

func TestFinalizeMPEGPSFallsBackToDerivedVideoDuration(t *testing.T) {
	streams := map[uint16]*psStream{
		psStreamKey(0xE0, psSubstreamNone): {
			id:              0xE0,
			subID:           psSubstreamNone,
			kind:            StreamVideo,
			format:          "MPEG Video",
			derivedDuration: 0.033,
		},
	}
	streamOrder := []uint16{psStreamKey(0xE0, psSubstreamNone)}

	info, _, ok := finalizeMPEGPS(streams, streamOrder, nil, ptsTracker{}, ptsTracker{}, 8<<10, mpegPSOptions{dvdParsing: true, parseSpeed: 0.5})
	if !ok {
		t.Fatalf("expected ok")
	}
	if info.DurationSeconds == 0 {
		t.Fatalf("expected DurationSeconds > 0")
	}
	if info.DurationSeconds < 0.032 || info.DurationSeconds > 0.034 {
		t.Fatalf("DurationSeconds = %f, want ~0.033", info.DurationSeconds)
	}
}

func TestDecimalSecondsToMilliseconds(t *testing.T) {
	for _, test := range []struct {
		seconds string
		want    string
		ok      bool
	}{
		{seconds: "4.008", want: "4008", ok: true},
		{seconds: "0.000001", want: "0.001", ok: true},
		{seconds: "-0.005", want: "-5", ok: true},
		{seconds: "12", want: "12000", ok: true},
		{seconds: ".", ok: false},
		{seconds: "bad", ok: false},
	} {
		got, ok := decimalSecondsToMilliseconds(test.seconds)
		if ok != test.ok || got != test.want {
			t.Fatalf("decimalSecondsToMilliseconds(%q) = %q, %v; want %q, %v", test.seconds, got, ok, test.want, test.ok)
		}
	}
}
