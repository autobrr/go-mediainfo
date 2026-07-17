package mediainfo

import (
	"bytes"
	"math"
	"reflect"
	"testing"
)

func TestMPEGPSBoundedWindowUsesPackMuxRate(t *testing.T) {
	for _, test := range []struct {
		name       string
		mpeg2      bool
		muxRate    uint64
		systemHead bool
		want       int64
	}{
		{name: "mpeg1", muxRate: 40_000, want: 8_000_000},
		{name: "mpeg2", mpeg2: true, muxRate: 60_000, want: 12_000_000},
		{name: "minimum", mpeg2: true, muxRate: 1, want: 2 << 20},
		{name: "maximum", mpeg2: true, muxRate: 100_000, want: 16 << 20},
		{name: "system header cap", mpeg2: true, muxRate: 60_000, systemHead: true, want: 8 << 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := makeMPEGPSPackHeader(test.mpeg2, test.muxRate)
			if test.systemHead {
				data = append(data, 0, 0, 1, 0xBB)
			}
			if got := mpegPSBoundedWindow(bytes.NewReader(data), int64(len(data))); got != test.want {
				t.Fatalf("mpegPSBoundedWindow() = %d, want %d", got, test.want)
			}
		})
	}

	data := []byte{0, 0, 1, 0xE0}
	if got := mpegPSBoundedWindow(bytes.NewReader(data), int64(len(data))); got != 8<<20 {
		t.Fatalf("mpegPSBoundedWindow() without pack = %d, want %d", got, 8<<20)
	}
}

func makeMPEGPSPackHeader(mpeg2 bool, muxRate uint64) []byte {
	data := make([]byte, 16)
	copy(data, []byte{0, 0, 1, 0xBA})
	if mpeg2 {
		data[4] = 0x44
		data[10] = byte(muxRate >> 14)
		data[11] = byte(muxRate >> 6)
		data[12] = byte(muxRate<<2) | 0x03
		return data
	}
	data[4] = 0x21
	data[9] = byte(muxRate >> 14)
	data[10] = byte(muxRate >> 6)
	data[11] = byte(muxRate<<2) | 0x03
	return data
}

func TestMPEGAudioHeaderRetainsJointStereoMode(t *testing.T) {
	header, ok := parseMPEGAudioHeader([]byte{0xFF, 0xFD, 0xA4, 0x40})
	if !ok {
		t.Fatal("parseMPEGAudioHeader() = false")
	}
	if header.channelMode != 1 || header.channels != 2 {
		t.Fatalf("channel mode, channels = %d, %d; want 1, 2", header.channelMode, header.channels)
	}
}

func TestTerminalMPEGAudioSyncLossAdjustsDuration(t *testing.T) {
	stream := &psStream{
		mpegAudioVersion: 3,
		mpegAudioLayer:   2,
		audioRate:        44_100,
		audioFrames:      101_965,
		programEndSeen:   true,
		audioBuffer:      []byte{0},
	}
	got := audioDurationPS(stream, mpegPSOptions{})
	want := float64(stream.audioFrames*1152)/stream.audioRate - float64(mpegAudioTerminalValidationFrames*1152)/stream.audioRate + 0.0025
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("audioDurationPS() = %.9f, want %.9f", got, want)
	}
}

func TestMPEGAudioConformanceExtra(t *testing.T) {
	stream := &psStream{
		mpegAudioVersion: 3,
		mpegAudioLayer:   2,
		audioRate:        44_100,
		audioFrames:      mpegAudioValidationThreshold,
		programEndSeen:   true,
		audioBuffer:      []byte{0},
	}
	short := *stream
	short.audioFrames = mpegAudioValidationThreshold - 1
	if extra := mpegAudioConformanceExtra(&short, 100, 1.2); extra != nil {
		t.Fatal("mpegAudioConformanceExtra() accepted an unvalidated short stream")
	}
	extra := mpegAudioConformanceExtra(stream, 100, 1.2)
	if extra == nil {
		t.Fatal("mpegAudioConformanceExtra() = nil")
	}
	wantJSON := `{"ConformanceInfos":[{"MPEGAudio":[{"GeneralCompliance":"Bitstream synchronisation is lost, zeroed bytes at the end (count 1 1.0000000%, time 00:00:02.660, offset 0x63)"}]}]}`
	if got := structuredNodeText(*extra); got != wantJSON {
		t.Fatalf("structuredNodeText() = %s, want %s", got, wantJSON)
	}
	wantRaw := []rawTextDerivedField{
		{Label: "ConformanceInfos", Value: "1"},
		{Label: " MPEG-Audio", Value: "Yes"},
		{Label: "  GeneralCompliance", Value: "Bitstream synchronisation is lost, zeroed bytes at the end (count 1 1.0000000%, time 00:00:02.660, offset 0x63)"},
	}
	if got := rawTextConformanceInfoFields(extra.Object[0].Value); !reflect.DeepEqual(got, wantRaw) {
		t.Fatalf("rawTextConformanceInfoFields() = %#v, want %#v", got, wantRaw)
	}
}

func TestMPEG2PictureHeaderDeterminesBitRateMode(t *testing.T) {
	for _, test := range []struct {
		name string
		vbv  uint32
		want string
	}{
		{name: "constant", vbv: 0x1234, want: "Constant"},
		{name: "variable", vbv: 0xFFFF, want: "Variable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			parser := &mpeg2VideoParser{}
			value := uint32(1)<<19 | test.vbv<<3
			parser.parsePictureHeader([]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
			if got := parser.finalize().BitRateMode; got != test.want {
				t.Fatalf("BitRateMode = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMPEG2SequenceDisplayStandard(t *testing.T) {
	parser := &mpeg2VideoParser{}
	parser.parseExtension([]byte{0x24, 0}) // extension 2, video_format 2 (NTSC)
	if parser.info.Standard != "NTSC" {
		t.Fatalf("Standard = %q, want NTSC", parser.info.Standard)
	}
}

func TestMPEG2FinalizeRecordsLastGOPOnce(t *testing.T) {
	parser := &mpeg2VideoParser{sawGOP: true, currentGOPCount: 12}
	first := parser.finalize()
	second := parser.finalize()
	if first.GOPSamples != 1 || second.GOPSamples != 1 || parser.gopLengthCounts[12] != 1 {
		t.Fatalf("GOP samples = %d, %d; count = %d; want 1, 1, 1", first.GOPSamples, second.GOPSamples, parser.gopLengthCounts[12])
	}
}

func TestRawTextTMPGEncLibraryNormalization(t *testing.T) {
	const input = "encoded by TMPGEnc (ver. 2.59.47.155)"
	if got := formatRawTextEncodedLibrary(input); got != "TMPGEnc 2.59.47.155" {
		t.Fatalf("formatRawTextEncodedLibrary() = %q", got)
	}
}
