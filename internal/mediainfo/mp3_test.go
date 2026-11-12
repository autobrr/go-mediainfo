package mediainfo

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	info, streams, _, _, ok := parseMP3(bytes.NewReader(buf), int64(len(buf)))
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

func TestMPEGAudioSamplesPerFrameUsesVersionAndLayer(t *testing.T) {
	for _, test := range []struct {
		version, layer byte
		want           int
	}{
		{0x03, 0x03, 384},  // MPEG-1 Layer I
		{0x02, 0x02, 1152}, // MPEG-2 Layer II
		{0x03, 0x01, 1152}, // MPEG-1 Layer III
		{0x02, 0x01, 576},  // MPEG-2 Layer III
		{0x00, 0x01, 576},  // MPEG-2.5 Layer III
	} {
		if got := mpegAudioSamplesPerFrame(test.version, test.layer); got != test.want {
			t.Errorf("mpegAudioSamplesPerFrame(%d, %d) = %d, want %d", test.version, test.layer, got, test.want)
		}
	}
}

func TestMP3UserFieldsKeepTagOrder(t *testing.T) {
	for _, version := range []byte{3, 4} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := writeTaggedMP3ForTest(t, version, []id3TestTag{
				{"TXXX", "Zeta", "old"},
				{"TXXX", "ISRC", "test-isrc"},
				{"WXXX", "", "https://example.invalid/"},
				{"TXXX", "Alpha", "second"},
				{"WXXX", "Ignored", "https://example.invalid/ignored"},
				{"TXXX", "Zeta", "replacement"},
			})
			for range 20 {
				report, err := AnalyzeFile(path)
				if err != nil {
					t.Fatal(err)
				}
				json := RenderJSON([]Report{report})
				if !strings.Contains(json, `"extra":{"Zeta":"replacement","URL":"https://example.invalid/","Alpha":"second"}`) {
					t.Fatalf("user fields changed order or values: %s", json)
				}
				if report.General.JSON["ISRC"] != "test-isrc" {
					t.Fatalf("mapped ISRC = %q", report.General.JSON["ISRC"])
				}
				raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
				previous := -1
				for _, label := range []string{"Zeta", "URL", "Alpha"} {
					index := strings.Index(raw, "\n"+label+" ")
					if index <= previous {
						t.Fatalf("raw user fields changed order: %s", raw)
					}
					previous = index
				}
			}
		})
	}
}

type id3TestTag struct{ id, description, value string }

func writeTaggedMP3ForTest(t *testing.T, version byte, tags []id3TestTag) string {
	t.Helper()
	var frames []byte
	for _, tag := range tags {
		data := append([]byte{3}, tag.description...)
		data = append(data, 0)
		data = append(data, tag.value...)
		frame := make([]byte, 10)
		copy(frame, tag.id)
		if version == 4 {
			writeSynchsafe32ForTest(frame[4:8], uint32(len(data)))
		} else {
			binary.BigEndian.PutUint32(frame[4:8], uint32(len(data)))
		}
		frames = append(frames, append(frame, data...)...)
	}
	data := make([]byte, 10)
	copy(data, "ID3")
	data[3] = version
	writeSynchsafe32ForTest(data[6:10], uint32(len(frames)))
	data = append(data, frames...)
	audio := make([]byte, 626)
	copy(audio, []byte{0xff, 0xfb, 0x70, 0x40})
	copy(audio[313:], []byte{0xff, 0xfb, 0x70, 0x40})
	path := filepath.Join(t.TempDir(), "ordered.mp3")
	if err := os.WriteFile(path, append(data, audio...), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMP3CollidingURLKeepsExistingValues(t *testing.T) {
	for _, tags := range [][]id3TestTag{
		{{"TXXX", "URL", "text-value"}, {"WXXX", "", "https://example.invalid/"}},
		{{"WXXX", "", "https://example.invalid/"}, {"TXXX", "URL", "text-value"}},
	} {
		report, err := AnalyzeFile(writeTaggedMP3ForTest(t, 3, tags))
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Media struct {
				Track []struct{ Extra map[string]string }
			}
		}
		if err := json.Unmarshal([]byte(RenderJSON([]Report{report})), &decoded); err != nil {
			t.Fatal(err)
		}
		if got := decoded.Media.Track[0].Extra["URL"]; got != "text-value" {
			t.Fatalf("JSON URL = %q", got)
		}
		raw := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
		var urls []string
		for _, line := range strings.Split(raw, "\n") {
			key, value, _ := strings.Cut(line, ":")
			if strings.TrimSpace(key) == "URL" {
				urls = append(urls, strings.TrimSpace(value))
			}
		}
		if len(urls) != 1 || urls[0] != "https://example.invalid/" {
			t.Fatalf("raw URL values = %q", urls)
		}
	}
}
