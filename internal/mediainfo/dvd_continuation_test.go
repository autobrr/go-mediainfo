package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAnalyzeDVDJoinedParsing(t *testing.T) {
	for _, tc := range []struct {
		name, directory          string
		ifo, disabled, singleton bool
		wantCount                string
	}{
		{name: "direct inside", directory: "VIDEO_TS", wantCount: "130"},
		{name: "direct outside", directory: "standalone", wantCount: "130"},
		{name: "IFO inside", directory: "VIDEO_TS", ifo: true, wantCount: "130"},
		{name: "IFO outside", directory: "standalone", ifo: true, wantCount: "130"},
		{name: "disabled outside", directory: "standalone", disabled: true, wantCount: "128"},
		{name: "singleton outside", directory: "standalone", singleton: true, wantCount: "128"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), tc.directory)
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			// More than 128 complete AC-3 frames distinguishes DVD parsing from
			// generic MPEG-PS's bounded statistics pass at ParseSpeed 0.5.
			frame := buildBoundaryAC3Frame(t, 0x20)
			packet := append([]byte{0, 0, 1, 0xBD, 0, 0, 0x80, 0, 0, 0x80, 0, 0, 0}, frame...)
			binary.BigEndian.PutUint16(packet[4:6], uint16(len(packet)-6))
			first := filepath.Join(root, "VTS_01_1.VOB")
			if err := os.WriteFile(first, bytes.Repeat(packet, 130), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(first, 0x3F000000); err != nil {
				t.Fatal(err)
			}
			if !tc.singleton {
				if err := os.WriteFile(filepath.Join(root, "VTS_01_2.VOB"), []byte{0, 0, 1, 0xB9}, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			input := first
			if tc.ifo {
				data, _ := remappedSubpictureDVDFixture()
				input = filepath.Join(root, "VTS_01_0.IFO")
				if err := os.WriteFile(input, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			report, err := AnalyzeFileWithOptions(input, AnalyzeOptions{ParseSpeed: 0.5, HasTestContinuousFileNames: tc.disabled})
			if err != nil {
				t.Fatal(err)
			}
			for _, stream := range report.Streams {
				if stream.Kind != StreamAudio {
					continue
				}
				extra := canonicalSeedStructuredNode(&stream, "extra")
				if extra != nil {
					for _, member := range extra.Object {
						if member.Key == "compr_Count" {
							if member.Value.Text != tc.wantCount {
								t.Errorf("compr_Count = %q, want %q", member.Value.Text, tc.wantCount)
							}
							return
						}
					}
				}
			}
			t.Fatal("missing AC-3 compression statistics")
		})
	}
}

func TestAnalyzeDVDContinuationEligibility(t *testing.T) {
	for _, tc := range []struct {
		name      string
		firstSize int64
		selected  string
		joined    bool
		disable   bool
		enable    bool
		directory string
		second    string
	}{
		{name: "below lower boundary", firstSize: 0x3F000000 - 1, selected: "VTS_01_1.VOB"},
		{name: "lower boundary", firstSize: 0x3F000000, selected: "VTS_01_1.VOB", joined: true},
		{name: "below upper boundary", firstSize: 0x40000000 - 1, selected: "VTS_01_1.VOB", joined: true},
		{name: "upper boundary", firstSize: 0x40000000, selected: "VTS_01_1.VOB"},
		{name: "later member", firstSize: 0x3F000000, selected: "VTS_01_2.VOB"},
		{name: "explicitly disabled", firstSize: 0x3F000000, selected: "VTS_01_1.VOB", disable: true},
		{name: "explicitly enabled", firstSize: 0x3F000000, selected: "VTS_01_1.VOB", joined: true, enable: true},
		{name: "outside VIDEO_TS", firstSize: 0x3F000000, selected: "VTS_01_1.VOB", joined: true, directory: "standalone"},
		{name: "lowercase extension", firstSize: 0x3F000000, selected: "VTS_01_1.vob"},
		{name: "lowercase prefix", firstSize: 0x3F000000, selected: "vts_01_1.VOB", joined: true},
		{name: "retains member after gap", firstSize: 0x3F000000, selected: "VTS_01_1.VOB", joined: true, second: "VTS_01_3.VOB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), firstNonEmpty(tc.directory, "VIDEO_TS"))
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			// The sparse MPEG-PS fixture tests selection/accounting, not decoded video parity.
			payload := []byte{0, 0, 1, 0xBA, 0x21, 0, 1, 0, 1, 0x80, 0x1B, 0x91, 0, 0, 1, 0xE0, 0, 9, 0x21, 0, 1, 0, 1, 0, 0, 1, 0xB3}
			secondSize := int64(len(payload))
			if tc.selected == "VTS_01_2.VOB" {
				secondSize = 0x3F000000
			}
			firstName := tc.selected
			if tc.selected == "VTS_01_2.VOB" {
				firstName = "VTS_01_1.VOB"
			}
			secondName := firstNonEmpty(tc.second, "VTS_01_2.VOB")
			for name, size := range map[string]int64{firstName: tc.firstSize, secondName: secondSize} {
				f, err := os.OpenFile(filepath.Join(root, name), os.O_CREATE|os.O_RDWR, 0o600)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.Truncate(size); err != nil {
					f.Close()
					t.Fatal(err)
				}
				if _, err := f.Write(payload); err != nil {
					f.Close()
					t.Fatal(err)
				}
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
			}
			report, err := AnalyzeFileWithOptions(filepath.Join(root, tc.selected), AnalyzeOptions{ParseSpeed: 0.5, HasTestContinuousFileNames: tc.disable || tc.enable, TestContinuousFileNames: tc.enable})
			if err != nil {
				t.Fatal(err)
			}
			wantSize := tc.firstSize
			if tc.selected == "VTS_01_2.VOB" {
				wantSize = secondSize
			}
			wantLast := ""
			if tc.joined {
				wantSize += secondSize
				wantLast = filepath.Join(root, secondName)
			}
			if got, _ := canonicalSeedValue(report.General, "FileSize"); got != strconv.FormatInt(wantSize, 10) {
				t.Errorf("FileSize = %q, want %d", got, wantSize)
			}
			if got := findField(report.General.Fields, "CompleteName_Last"); got != wantLast {
				t.Errorf("CompleteName_Last = %q, want %q", got, wantLast)
			}
		})
	}
}

func TestAnalyzeDVDIFOPayloadSelection(t *testing.T) {
	for _, tc := range []struct {
		name             string
		first            bool
		backup           bool
		firstSize        int64
		disable          bool
		directory        string
		ifoName          string
		metadataOnly     bool
		lowercasePayload bool
		second           string
		wantDisplay      string
	}{
		{name: "small first without backup", first: true, wantDisplay: "6.01 KiB"},
		{name: "small first with backup", first: true, backup: true, wantDisplay: "6.01 KiB"},
		{name: "missing first", backup: true, wantDisplay: "6.00 KiB"},
		{name: "eligible first without backup", first: true, firstSize: 0x3F000000, wantDisplay: "1008 MiB"},
		{name: "eligible first with backup", first: true, backup: true, firstSize: 0x3F000000, wantDisplay: "1008 MiB"},
		{name: "eligible first outer continuation disabled", first: true, backup: true, firstSize: 0x3F000000, disable: true, wantDisplay: "1008 MiB"},
		{name: "outside VIDEO_TS", first: true, firstSize: 0x3F000000, directory: "standalone", wantDisplay: "1008 MiB"},
		{name: "lowercase IFO extension", first: true, firstSize: 0x3F000000, ifoName: "VTS_01_0.ifo", metadataOnly: true, wantDisplay: "6.00 KiB"},
		{name: "lowercase payload spelling", first: true, firstSize: 0x3F000000, lowercasePayload: true, wantDisplay: "1008 MiB"},
		{name: "retains payload after gap", first: true, firstSize: 0x3F000000, second: "VTS_01_3.VOB", wantDisplay: "1008 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), firstNonEmpty(tc.directory, "VIDEO_TS"))
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			ifo, vob := remappedSubpictureDVDFixture()
			second := append([]byte(nil), vob...)
			second[9] = 0x22
			ifoName := firstNonEmpty(tc.ifoName, "VTS_01_0.IFO")
			firstName, secondName := "VTS_01_1.VOB", firstNonEmpty(tc.second, "VTS_01_2.VOB")
			if tc.lowercasePayload {
				firstName, secondName = "vts_01_1.vob", "vts_01_2.vob"
			}
			files := map[string][]byte{ifoName: ifo, secondName: second}
			if tc.first {
				files[firstName] = vob
			}
			if tc.backup {
				files["VTS_01_0.BUP"] = ifo
			}
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.firstSize > 0 {
				if err := os.Truncate(filepath.Join(root, firstName), tc.firstSize); err != nil {
					t.Fatal(err)
				}
			}
			report, err := AnalyzeFileWithOptions(filepath.Join(root, ifoName), AnalyzeOptions{HasTestContinuousFileNames: tc.disable})
			if err != nil {
				t.Fatal(err)
			}
			wantSize := int64(len(ifo))
			if tc.first && !tc.metadataOnly {
				wantSize += int64(len(vob))
				if tc.firstSize > 0 {
					wantSize = int64(len(ifo)) + tc.firstSize + int64(len(second))
				}
			}
			if got, _ := canonicalSeedValue(report.General, "FileSize"); got != strconv.FormatInt(wantSize, 10) {
				t.Errorf("FileSize = %q, want %d", got, wantSize)
			}
			reports := []Report{report}
			for _, output := range []struct {
				name, rendered, want string
			}{
				{"text", RenderText(reports), fmt.Sprintf("%-41s: %s\n", "File size", tc.wantDisplay)},
				{"HTML", RenderHTML(reports), "<td>File size</td><td>" + tc.wantDisplay + "</td>"},
				{"CSV", RenderCSV(reports), "File size," + tc.wantDisplay + "\n"},
				{"JSON", RenderJSON(reports), fmt.Sprintf(`"FileSize":"%d"`, wantSize)},
				{"XML", RenderXML(reports), fmt.Sprintf("<FileSize>%d</FileSize>", wantSize)},
				{"raw text", normalizeText(RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"})), fmt.Sprintf("%-33s: %s\n", "FileSize/String", tc.wantDisplay)},
			} {
				t.Run(output.name, func(t *testing.T) {
					if !strings.Contains(output.rendered, output.want) {
						t.Errorf("missing file size %q in:\n%s", output.want, output.rendered)
					}
				})
			}
			secondPayload := false
			for _, stream := range report.Streams {
				if id, _ := canonicalSeedValue(stream, "ID"); id == "189-34" {
					secondPayload = true
				}
			}
			if secondPayload != (tc.firstSize > 0 && !tc.metadataOnly) {
				t.Errorf("second VOB payload present = %v, want %v", secondPayload, tc.firstSize > 0 && !tc.metadataOnly)
			}
		})
	}
}
