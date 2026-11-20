package mediainfo

import (
	"encoding/binary"
	"io"
	"testing"
)

func TestFileFormatChecksMovieBeyondMPEGShapedMdat(t *testing.T) {
	header := make([]byte, 580)
	copy(header, "\xff\xf5\xa4\xc4mdat")
	copy(header[576:], header[:4])
	movieOffset := int64(binary.BigEndian.Uint32(header[:4]))
	for _, movie := range []bool{false, true} {
		reader := &formatMovieProbeReader{header: header, movieOffset: movieOffset}
		size, want := movieOffset, "MPEG Audio"
		if movie {
			size += 8
			want = "QuickTime"
		}
		if got := detectFormat(header, "movie.mov", reader, size); got != want {
			t.Errorf("movie=%v: format %q, want %q", movie, got, want)
		}
		if reader.readBytes > maxSniffBytes {
			t.Fatalf("format confirmation read %d bytes", reader.readBytes)
		}
	}
}

type formatMovieProbeReader struct {
	header      []byte
	movieOffset int64
	readBytes   int
}

func (r *formatMovieProbeReader) ReadAt(p []byte, offset int64) (int, error) {
	var data []byte
	switch {
	case offset == 0:
		data = r.header
	case offset == r.movieOffset:
		data = []byte("\x00\x00\x00\x08moov")
	default:
		return 0, io.EOF // The mdat payload must be skipped, not scanned.
	}
	n := copy(p, data)
	r.readBytes += n
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestDetectFormat(t *testing.T) {
	tsAtom := func(atom string) []byte {
		header := makeTSHeader()
		copy(header[1:4], []byte{1, 0, 0x10})
		copy(header[4:], atom)
		return header
	}
	quickTimeMdat := make([]byte, 450)
	copy(quickTimeMdat, "\x00\x00\x01\xbamdat")
	copy(quickTimeMdat[442:], "\x00\x00\x00\x08moov")
	quickTimeFree := append([]byte(nil), quickTimeMdat...)
	copy(quickTimeFree[4:], "free\x04\x01\x86\x66\xcf\xf8\x00\x00\x01\xbb")
	audioAtom := func(header []byte, frameLength int) []byte {
		data := make([]byte, frameLength+4)
		copy(data, header)
		copy(data[4:], "mdat")
		copy(data[frameLength:], header)
		return data
	}
	cases := []struct {
		name     string
		header   []byte
		filename string
		want     string
	}{
		{name: "matroska", header: []byte{0x1A, 0x45, 0xDF, 0xA3}, filename: "movie.mkv", want: "Matroska"},
		{name: "mp4", header: []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, filename: "movie.mp4", want: "MPEG-4"},
		{name: "quicktime", header: []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'q', 't', ' ', ' '}, filename: "movie.mov", want: "QuickTime"},
		{name: "empty ftyp after padding", header: []byte("\x00\x00\x00\x08free\x00\x00\x00\x08ftyp\x00\x00\x00\x08free"), filename: "movie.mp4", want: "Unknown"},
		{name: "short ftyp brand", header: []byte("\x00\x00\x00\x0bftypqt \x00\x00\x00\x08free"), filename: "movie.mov", want: "Unknown"},
		{name: "complete ftyp brand after padding", header: []byte("\x00\x00\x00\x08free\x00\x00\x00\x0cftypqt  \x00\x00\x00\x08free"), filename: "movie.mov", want: "QuickTime"},
		{name: "avi", header: []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'A', 'V', 'I', ' '}, filename: "movie.avi", want: "AVI"},
		{name: "wave", header: []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'A', 'V', 'E'}, filename: "sound.wav", want: "Wave"},
		{name: "wave size spells mdat", header: []byte("RIFFmdatWAVE"), filename: "sound.wav", want: "Wave"},
		{name: "avi size spells moov", header: []byte("RIFFmoovAVI "), filename: "movie.avi", want: "AVI"},
		{name: "aiff size spells mdat", header: []byte("FORMmdatAIFF"), filename: "sound.aiff", want: "AIFF"},
		{name: "wave size spells ftyp", header: []byte("RIFFftypWAVE"), filename: "sound.wav", want: "Wave"},
		{name: "flac", header: []byte{'f', 'L', 'a', 'C'}, filename: "sound.flac", want: "FLAC"},
		{name: "ogg", header: []byte{'O', 'g', 'g', 'S'}, filename: "sound.ogg", want: "Ogg"},
		{name: "mp3", header: []byte{'I', 'D', '3'}, filename: "sound.mp3", want: "MPEG Audio"},
		{name: "mp2ts", header: makeTSHeader(), filename: "stream.ts", want: "MPEG-TS"},
		{name: "TS payload starts moov", header: tsAtom("moov"), filename: "stream.ts", want: "MPEG-TS"},
		{name: "TS payload starts mdat", header: tsAtom("mdat"), filename: "stream.ts", want: "MPEG-TS"},
		{name: "MP4 moov without TS sync", header: []byte("\x00\x00\x00\x08moov"), filename: "movie.mov", want: "QuickTime"},
		{name: "MP4 mdat without TS sync", header: []byte("\x00\x00\x00\x08mdat"), filename: "movie.mov", want: "QuickTime"},
		{name: "QuickTime mdat size matches PS signature", header: quickTimeMdat, filename: "movie.mov", want: "QuickTime"},
		{name: "QuickTime free contains PS-like bytes", header: quickTimeFree, filename: "movie.mov", want: "QuickTime"},
		{name: "mpegps", header: []byte{0x00, 0x00, 0x01, 0xBA}, filename: "movie.vob", want: "MPEG-PS"},
		{name: "MPEG-PS pack starts moov", header: []byte("\x00\x00\x01\xbamoov\x04\x01\x86\x66\xcf\xf8\x00\x00\x01\xbb"), filename: "movie.mpg", want: "MPEG-PS"},
		{name: "MPEG audio payload starts mdat", header: audioAtom([]byte{0xFF, 0xFD, 0xE4, 0xC0}, 1152), filename: "sound.mp2", want: "MPEG Audio"},
		{name: "MPEG2 Layer II payload starts mdat", header: audioAtom([]byte{0xFF, 0xF5, 0xA4, 0xC0}, 576), filename: "sound.mp2", want: "MPEG Audio"},
		{name: "MPEG2 Layer III payload starts mdat", header: audioAtom([]byte{0xFF, 0xF3, 0xA4, 0xC0}, 288), filename: "sound.mp3", want: "MPEG Audio"},
		{name: "QuickTime audio-like size without following frame", header: []byte("\xff\xfd\xe4\xc0mdat"), filename: "movie.mov", want: "QuickTime"},
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
