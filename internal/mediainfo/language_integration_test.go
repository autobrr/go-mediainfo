package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func analyzeLanguageFixture(t *testing.T, name string, data []byte) Report {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestAnalyzeDVDPublicLanguageNamesUseDVDPolicy(t *testing.T) {
	data := make([]byte, 0x300)
	copy(data, "DVDVIDEO-VTS")
	data[dvdVideoAttrVTSOffset] = 0x4c
	binary.BigEndian.PutUint16(data[dvdAudioCountVTSOffset:], 1)
	data[dvdAudioAttrVTSOffset+1] = 1
	copy(data[dvdAudioAttrVTSOffset+2:], "en")
	report := analyzeLanguageFixture(t, "VTS_01_0.IFO", data)
	for _, tc := range []struct{ input, code, display string }{{"Filipino", "fil", "Filipino"}, {"No linguistic content or not applicable", "zxx", "No linguistic content or not applicable"}, {"Undetermined", "", ""}, {"Uncoded languages", "", ""}} {
		t.Run(tc.input, func(t *testing.T) {
			edited := report
			edited.Streams = append([]Stream(nil), report.Streams...)
			found := false
			for i := range edited.Streams {
				if edited.Streams[i].Kind == StreamAudio {
					found = true
					edited.Streams[i].Fields = append([]Field(nil), edited.Streams[i].Fields...)
					edited.Streams[i].Fields = setFieldValue(edited.Streams[i].Fields, "Language", tc.input)
				}
			}
			if !found {
				t.Fatal("missing audio")
			}
			assertLanguageProjection(t, edited, tc.code, tc.display, tc.code)
			if tc.input == "Filipino" {
				rebuilt := Report{General: Stream{Fields: report.General.Fields}}
				for _, stream := range edited.Streams {
					if stream.Kind == StreamAudio {
						rebuilt.Streams = append(rebuilt.Streams, Stream{Kind: StreamAudio, Fields: stream.Fields})
					}
				}
				assertLanguageProjection(t, rebuilt, "fil", "Filipino", "fil")
			}
		})
	}
}

func TestAnalyzeMP4ChapterLanguages(t *testing.T) {
	var file, movie, mdat bytes.Buffer
	writeMP4Box(&file, "ftyp", []byte("isom"))
	for _, title := range []string{"A", "B", "C"} {
		mdat.Write([]byte{0, 1, title[0]})
	}
	writeMP4Box(&file, "mdat", mdat.Bytes()) // sample offsets 20, 23, 26
	track := func(id uint32, code uint16, extended string, offset uint32) []byte {
		var out, media, minf, stbl bytes.Buffer
		tkhd := make([]byte, 36)
		binary.BigEndian.PutUint32(tkhd[12:], id)
		writeMP4Box(&out, "tkhd", tkhd)
		h := make([]byte, 24)
		copy(h[8:], "text")
		if id == 1 {
			copy(h[8:], "vide")
		}
		writeMP4Box(&media, "hdlr", h)
		mdhd := make([]byte, 24)
		binary.BigEndian.PutUint32(mdhd[12:], 1000)
		binary.BigEndian.PutUint32(mdhd[16:], 1000)
		binary.BigEndian.PutUint16(mdhd[20:], code)
		writeMP4Box(&media, "mdhd", mdhd)
		if extended != "" {
			writeMP4Box(&media, "elng", append([]byte{0, 0, 0, 0}, []byte(extended+"\x00")...))
		}
		if id == 1 {
			var refs bytes.Buffer
			writeMP4Box(&refs, "chap", []byte{0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0, 5})
			writeMP4Box(&out, "tref", refs.Bytes())
		} else {
			stsd := make([]byte, 24)
			binary.BigEndian.PutUint32(stsd[4:], 1)
			binary.BigEndian.PutUint32(stsd[8:], 16)
			copy(stsd[12:], "text")
			writeMP4Box(&stbl, "stsd", stsd)
			stts := make([]byte, 16)
			binary.BigEndian.PutUint32(stts[4:], 1)
			binary.BigEndian.PutUint32(stts[8:], 1)
			binary.BigEndian.PutUint32(stts[12:], 1000)
			writeMP4Box(&stbl, "stts", stts)
			stsz := make([]byte, 12)
			binary.BigEndian.PutUint32(stsz[4:], 3)
			binary.BigEndian.PutUint32(stsz[8:], 1)
			writeMP4Box(&stbl, "stsz", stsz)
			stco := make([]byte, 12)
			binary.BigEndian.PutUint32(stco[4:], 1)
			binary.BigEndian.PutUint32(stco[8:], offset)
			writeMP4Box(&stbl, "stco", stco)
			writeMP4Box(&minf, "stbl", stbl.Bytes())
			writeMP4Box(&media, "minf", minf.Bytes())
		}
		writeMP4Box(&out, "mdia", media.Bytes())
		return out.Bytes()
	}
	writeMP4Box(&movie, "trak", track(1, 0x15c7, "", 0))
	writeMP4Box(&movie, "trak", track(3, 1, "", 20))
	writeMP4Box(&movie, "trak", track(4, 0x15c7, "en-us", 23))
	writeMP4Box(&movie, "trak", track(5, 0x55c4, "", 26))
	var udta bytes.Buffer
	chpl := append(make([]byte, 9), make([]byte, 8)...)
	chpl[8] = 1
	chpl = append(chpl, 1, 'U')
	writeMP4Box(&udta, "chpl", chpl)
	writeMP4Box(&movie, "udta", udta.Bytes())
	writeMP4Box(&file, "moov", movie.Bytes())
	report := analyzeLanguageFixture(t, "chapters.mp4", file.Bytes())
	if len(report.Streams) != 5 {
		t.Fatalf("stream count %d: %v", len(report.Streams), report.Streams)
	}
	var menus []Stream
	for _, s := range report.Streams {
		if s.Kind == StreamMenu {
			menus = append(menus, s)
		}
	}
	if len(menus) != 4 {
		t.Fatalf("Menu count %d", len(menus))
	}
	for i, want := range []struct{ id, code, display, raw, title string }{{"3", "fr", "French", "fr", "A"}, {"4", "en-us", "English (United States)", "en (US)", "B"}, {"5", "", "", "", "C"}, {"", "", "", "", "U"}} {
		s := menus[i]
		if s.JSON["ID"] != want.id {
			t.Fatalf("Menu %d identity %v", i, s.JSON)
		}
		one := Report{General: report.General, Streams: []Stream{s}}
		// Reconstruct from public state rather than retaining the original report store.
		one.General.reportStore = nil
		one.General.reportSnapshot = nil
		assertLanguageProjection(t, one, want.code, want.display, want.raw)
		if strings.Count(RenderJSON([]Report{one}), `"_00_00_00_000":"`+want.title+`"`) != 1 {
			t.Fatalf("chapter contents changed for Menu %s", want.id)
		}
		if want.id != "" && (s.JSON["CodecID"] != "text" || s.JSON["Duration"] != "1.000") {
			t.Fatalf("chapter facts changed: %v", s.JSON)
		}
	}
}

func TestAnalyzeTSLanguagePositionsAndCaptions(t *testing.T) {
	for _, tc := range []struct {
		name          string
		codes         []string
		caption       string
		want, display string
	}{
		{"positions", []string{"", "eng", "und", "eng", ""}, "eng", " / en /  / en / ", " / English /  / English / "},
		{"several", []string{"", "fra", "scc"}, "und", " / fr / sr", " / French / Serbian"},
		{"unknown members", []string{"", "xyz", "fra"}, "xyz", " / xyz / fr", " / xyz / French"},
		{"all empty", []string{"", "und", ""}, "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A program registration creates the same Menu without depending on captions.
			pmt := []byte{2, 0, 0, 0, 1, 0xc1, 0, 0, 0xe1, 1, 0xf0, 6, 5, 4, 'G', 'A', '9', '4'}
			for i, code := range tc.codes {
				desc := []byte{}
				if code != "" {
					desc = append(desc, 0x0a, 4)
					desc = append(desc, code...)
					desc = append(desc, 0)
				}
				if i == 0 && tc.caption != "" {
					desc = append(desc, 0x86, 7, 0xe1)
					desc = append(desc, tc.caption...)
					desc = append(desc, 0, 0, 0)
				}
				typ := byte(0x81)
				if i == 0 {
					typ = 2
				}
				pid := uint16(257 + i)
				pmt = append(pmt, typ, byte(0xe0|pid>>8), byte(pid), byte(0xf0|len(desc)>>8), byte(len(desc)))
				pmt = append(pmt, desc...)
			}
			pmt = append(pmt, 0, 0, 0, 0)
			size := len(pmt) - 3
			pmt[1] = 0xb0 | byte(size>>8)
			pmt[2] = byte(size)
			data := tsPacket(0, true, buildPATSection(256))
			data = append(data, tsPacket(256, true, append([]byte{0}, pmt...))...)
			for frame := 0; frame < 2; frame++ {
				video := []byte{0, 0, 1, 0, 0, 8, 0, 0, 1, 0xb2, 'G', 'A', '9', '4', 3, 0x43, 0xff, 0xfc, 0x14, 0x20, 0xfc, 'H', 'I', 0xfc, 0x14, 0x2f, 0, 0, 1, 0xb8}
				data = append(data, tsPacket(257, true, testMPEGPSPTS(0xe0, uint64(90000+frame*90000), video))...)
			}
			data = append(data, tsPacket(0x1fff, false, nil)...)
			report := analyzeLanguageFixture(t, "languages.ts", data)
			menuCount, captionCount := 0, 0
			for _, s := range report.Streams {
				if s.Kind != StreamMenu && s.Kind != StreamText {
					continue
				}
				one := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{s}}
				if s.Kind == StreamMenu {
					menuCount++
					if s.JSON["ID"] != "256" {
						t.Fatalf("Menu identity: %v", s.JSON)
					}
					if got := s.JSON["List_StreamKind"]; len(strings.Split(got, " / ")) != len(tc.codes) {
						t.Fatalf("member alignment %q", got)
					}
					assertLanguageProjection(t, one, tc.want, tc.display, tc.want)
				} else {
					captionCount++
					if s.JSON["ID"] != "257-CC1" {
						t.Fatalf("caption identity: %v", s.JSON)
					}
					code, display := "", ""
					if tc.caption == "eng" {
						code, display = "en", "English"
					} else if tc.caption == "xyz" {
						code, display = "xyz", "xyz"
					}
					assertLanguageProjection(t, one, code, display, code)
				}
			}
			if menuCount != 1 || captionCount != 1 {
				t.Fatalf("Menu/caption counts %d/%d; streams %s", menuCount, captionCount, fmt.Sprint(report.Streams))
			}
		})
	}
}

func TestAnalyzeOggLanguageComments(t *testing.T) {
	for _, tc := range []struct{ format, input, code, display, raw string }{
		{"Vorbis", "scc-Latn-rs", "sr-Latn-rs", "Serbian-Latn-rs", "sr-Latn-rs"},
		{"Opus", "en-us", "en-us", "English (United States)", "en (US)"},
		{"Theora", "fil", "fil", "fil", "fil"},
		{"FLAC", "French", "fr", "French", "fr"},
		{"Vorbis", "und", "", "", ""},
		{"Opus", "", "", "", ""},
		{"Vorbis", "en / en", "en / en", "English / English", "en / en"},
	} {
		t.Run(tc.format+"/"+tc.input, func(t *testing.T) {
			comment := binary.LittleEndian.AppendUint32(nil, 0)
			comment = binary.LittleEndian.AppendUint32(comment, 1)
			tag := "LANGUAGE=" + tc.input
			comment = binary.LittleEndian.AppendUint32(comment, uint32(len(tag)))
			comment = append(comment, tag...)
			var data []byte
			switch tc.format {
			case "Vorbis":
				data = buildOggPageForTest(7, 2, 0, vorbisIdentificationPacketForTest())
				data = append(data, buildOggPageForTest(7, 0, 0, append([]byte("\x03vorbis"), comment...))...)
			case "Opus":
				head := make([]byte, 19)
				copy(head, "OpusHead")
				head[8] = 1
				head[9] = 2
				data = buildOggPageForTest(7, 2, 0, head)
				data = append(data, buildOggPageForTest(7, 0, 0, append([]byte("OpusTags"), comment...))...)
			case "Theora":
				head := make([]byte, 42)
				copy(head, "\x80theora")
				binary.BigEndian.PutUint32(head[22:], 30)
				binary.BigEndian.PutUint32(head[26:], 1)
				data = buildOggPageForTest(7, 2, 0, head)
				data = append(data, buildOggPageForTest(7, 0, 0, append([]byte("\x81theora"), comment...))...)
			case "FLAC":
				streamInfo := make([]byte, 34)
				binary.BigEndian.PutUint64(streamInfo[10:], uint64(48000)<<44|uint64(1)<<41|uint64(15)<<36|4096)
				data = buildOggFLACForTest(append([]byte("fLaC\x80\x00\x00\x22"), streamInfo...), "", tag)
			}
			if tc.format != "FLAC" {
				data = append(data, buildOggPageForTest(7, 4, 4096, []byte{0})...)
			}
			report := analyzeLanguageFixture(t, "language.ogg", data)
			if len(report.Streams) != 1 || report.Streams[0].JSON["ID"] != "7" || report.Streams[0].JSON["Format"] != tc.format {
				t.Fatalf("stream identity: %v", report.Streams)
			}
			assertLanguageProjection(t, report, tc.code, tc.display, tc.raw)
		})
	}
}
