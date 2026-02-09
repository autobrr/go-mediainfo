package mediainfo

import (
	"bytes"
	"testing"
)

func TestMP3ModeExtensionUsesSecondFrameWhenAvailable(t *testing.T) {
	// Frame 1: joint stereo, mode extension 0.
	// Frame 2: joint stereo, mode extension 2 ("MS Stereo").
	//
	// This mirrors MediaInfo behavior: it may take Format settings from a subsequent frame.
	h1 := []byte{0xFF, 0xFB, 0x70, 0x40} // 96 kb/s, 44.1 kHz, joint stereo, modeExt=0
	h2 := []byte{0xFF, 0xFB, 0x70, 0x60} // 96 kb/s, 44.1 kHz, joint stereo, modeExt=2

	// MPEG1 Layer III @ 96 kb/s, 44.1 kHz, no padding => 313 bytes per frame.
	frameLen := 313
	buf := make([]byte, 0, frameLen*2)
	buf = append(buf, h1...)
	buf = append(buf, bytes.Repeat([]byte{0x00}, frameLen-len(h1))...)
	buf = append(buf, h2...)
	buf = append(buf, bytes.Repeat([]byte{0x00}, frameLen-len(h2))...)

	info, streams, _, _, ok := ParseMP3(bytes.NewReader(buf), int64(len(buf)))
	if !ok {
		t.Fatalf("ParseMP3 ok=false")
	}
	if info.DurationSeconds <= 0 {
		t.Fatalf("DurationSeconds=%v", info.DurationSeconds)
	}
	if len(streams) == 0 {
		t.Fatalf("no streams")
	}
	audio := streams[0]
	if audio.Kind != StreamAudio {
		t.Fatalf("stream[0].Kind=%v", audio.Kind)
	}
	if got := audio.JSON["Format_Settings_ModeExtension"]; got != "MS Stereo" {
		t.Fatalf("Format_Settings_ModeExtension=%q want %q", got, "MS Stereo")
	}
}
