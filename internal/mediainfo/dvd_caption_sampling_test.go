package mediainfo

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestAnalyzeDVDStandaloneRetainsPayloadSubpictures(t *testing.T) {
	for _, tc := range []struct {
		name, input, sibling string
		size                 int64
		captions             bool
	}{
		{"eligible first", "VTS_01_1.VOB", "VTS_01_2.VOB", 0x3F000000, true},
		{"later member", "VTS_01_2.VOB", "VTS_01_1.VOB", 64 << 20, true},
		{"no captions", "VTS_01_2.VOB", "VTS_01_1.VOB", 64 << 20, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "VIDEO_TS")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, tc.input)
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			size := tc.size
			if err := file.Truncate(size); err != nil {
				t.Fatal(err)
			}
			// Renderable captions trigger the existing DVD-wide sampling window.
			video := []byte{0, 0, 1, 0, 0, 0, 1, 0xB2, 'C', 'C', 1, 0xF8, 0x82, 0xFF, 0x14, 0x2F, 0xFE, 0x80, 0x80, 0, 0, 1, 0xB3}
			if !tc.captions {
				video = []byte{0, 0, 1, 0, 0, 0, 1, 0xB3}
			}
			packet := append([]byte{0, 0, 1, 0xE0, 0, byte(3 + len(video)), 0x80, 0, 0}, video...)
			if _, err := file.WriteAt(packet, 0); err != nil {
				t.Fatal(err)
			}
			// Complete structural SPUs lie beyond the original 8 MiB sample.
			// Real-file validation separately covers bitmap decoding.
			for i, id := range []byte{0x21, 0x22} {
				packet := testDVDSPUPES(id, testDVDSPU())
				if _, err := file.WriteAt(packet, 12<<20+int64(i*len(packet))); err != nil {
					t.Fatal(err)
				}
			}

			// The widened pass sees extra AC-3 data that must not replace the
			// original audio statistics or any existing stream metadata.
			audio := append([]byte{0x80, 0, 0, 0}, buildAC3FrameForMatroskaProbe()...)
			audioPacket := append([]byte{0, 0, 1, 0xBD, 0, byte(3 + len(audio)), 0x80, 0, 0}, audio...)
			for _, offset := range []int64{512, 12<<20 + 1024} {
				if _, err := file.WriteAt(audioPacket, offset); err != nil {
					t.Fatal(err)
				}
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			// An unrelated sibling must not supply a third subtitle or alter sizing.
			if err := os.WriteFile(filepath.Join(root, tc.sibling), []byte{0, 0, 1, 0xBD, 0, 5, 0x80, 0, 0, 0x20, 0}, 0o600); err != nil {
				t.Fatal(err)
			}

			selected, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer selected.Close()
			opts := mpegPSOptions{parseSpeed: 0.5, dvdParsing: true}
			original := newPSStreamParser(opts)
			original.parseReader(bufio.NewReaderSize(io.NewSectionReader(selected, 0, 8<<20), 1<<20))
			original.beginSection()
			original.parseReader(bufio.NewReaderSize(io.NewSectionReader(selected, size-(8<<20), 8<<20), 1<<20))
			sampled := opts
			sampled.sampled, sampled.sampledBytes = true, 16<<20
			wantInfo, wantStreams, wantOK := finalizeMPEGPS(original.streams, original.streamOrder, original.videoParsers, original.videoPTS, original.anyPTS, size, sampled)
			counted := &dvdSamplingReader{File: selected}
			gotInfo, gotStreams, gotOK := ParseMPEGPSWithOptions(counted, size, opts)
			if !wantOK || !gotOK || !reflect.DeepEqual(gotInfo, wantInfo) || len(gotStreams) < len(wantStreams) || !reflect.DeepEqual(gotStreams[:len(wantStreams)], wantStreams) {
				t.Fatal("supplemental discovery changed original container or existing streams")
			}
			if !tc.captions && !reflect.DeepEqual(gotStreams, wantStreams) {
				t.Fatal("unexpected supplemental scan without renderable captions")
			}
			if !tc.captions {
				if counted.bytes != 16<<20 {
					t.Fatalf("non-caption sample read %d bytes", counted.bytes)
				}
				counted.bytes = 0
				opts.parseSpeed = 1
				if _, _, ok := ParseMPEGPSWithOptions(counted, size, opts); !ok || counted.bytes != 0 {
					t.Fatal("full parse unexpectedly performed supplemental sampling")
				}
			} else if counted.bytes > (16<<20)+20_185_088+20_151_808 {
				t.Fatalf("unbounded supplemental sample: %d bytes", counted.bytes)
			}
			report, err := AnalyzeFileWithOptions(path, AnalyzeOptions{ParseSpeed: 0.5, HasTestContinuousFileNames: true, TestContinuousFileNames: false})
			if err != nil {
				t.Fatal(err)
			}
			ids := map[string]bool{}
			for _, stream := range report.Streams {
				if stream.Kind == StreamText {
					id, _ := canonicalSeedValue(stream, "ID")
					ids[id] = true
				}
			}
			wantIDs := []string{}
			if tc.captions {
				wantIDs = []string{"189-33", "189-34", "224-CC1"}
			}
			for _, id := range wantIDs {
				if !ids[id] {
					t.Errorf("missing payload text stream %s; got %v", id, ids)
				}
			}
			if len(ids) != len(wantIDs) {
				t.Errorf("text identities = %v, want %v", ids, wantIDs)
			}
			if got, _ := canonicalSeedValue(report.General, "FileSize"); got != strconv.FormatInt(size, 10) {
				t.Errorf("FileSize = %q, want %d", got, size)
			}
			if got := findField(report.General.Fields, "CompleteName_Last"); got != "" {
				t.Errorf("unexpected continued file %q", got)
			}
		})
	}
}

// dvdSamplingReader counts sampled bytes independently of sequential full parsing.
type dvdSamplingReader struct {
	*os.File
	bytes int64
}

func (r *dvdSamplingReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := r.File.ReadAt(p, off)
	r.bytes += int64(n)
	return n, err
}
