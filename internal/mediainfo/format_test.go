package mediainfo

import "testing"

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		name     string
		header   []byte
		filename string
		want     string
	}{
		{name: "matroska", header: []byte{0x1A, 0x45, 0xDF, 0xA3}, filename: "movie.mkv", want: "Matroska"},
		{name: "mp4", header: []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, filename: "movie.mp4", want: "MPEG-4"},
		{name: "quicktime", header: []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'q', 't', ' ', ' '}, filename: "movie.mov", want: "QuickTime"},
		{name: "avi", header: []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'A', 'V', 'I', ' '}, filename: "movie.avi", want: "AVI"},
		{name: "wave", header: []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'A', 'V', 'E'}, filename: "sound.wav", want: "Wave"},
		{name: "flac", header: []byte{'f', 'L', 'a', 'C'}, filename: "sound.flac", want: "FLAC"},
		{name: "ogg", header: []byte{'O', 'g', 'g', 'S'}, filename: "sound.ogg", want: "Ogg"},
		{name: "mp3", header: []byte{'I', 'D', '3'}, filename: "sound.mp3", want: "MPEG Audio"},
		{name: "mp2ts", header: makeTSHeader(), filename: "stream.ts", want: "MPEG-TS"},
		{name: "mpegps", header: []byte{0x00, 0x00, 0x01, 0xBA}, filename: "movie.vob", want: "MPEG-PS"},
		{name: "ifo", header: []byte{0x00, 0x00, 0x00, 0x00}, filename: "VIDEO_TS.IFO", want: "DVD Video"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectFormat(tc.header, tc.filename)
			if got != tc.want {
				t.Fatalf("DetectFormat(%s)=%q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

func makeTSHeader() []byte {
	buf := make([]byte, 377)
	buf[0] = 0x47
	buf[188] = 0x47
	buf[376] = 0x47
	return buf
}

func TestFormatFrameRateRatioDisplaysIntegralRatesPlain(t *testing.T) {
	if got := formatFrameRateRatio(25, 1); got != "25.000 FPS" {
		t.Fatalf("25/1 = %q", got)
	}
	if got := formatFrameRateRatio(24000, 1001); got != "23.976 (24000/1001) FPS" {
		t.Fatalf("24000/1001 = %q", got)
	}
}
