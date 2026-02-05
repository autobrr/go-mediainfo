package mediainfo

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestGoldenOutputs(t *testing.T) {
	type goldenCase struct {
		name   string
		render func([]Report) string
		ext    string
		norm   func(string) string
	}

	cases := []goldenCase{
		{name: "text", render: RenderText, ext: ".txt", norm: normalizeText},
		{name: "json", render: RenderJSON, ext: ".json", norm: normalizeJSONLike},
		{name: "xml", render: RenderXML, ext: ".xml", norm: normalizeXMLLike},
		{name: "html", render: RenderHTML, ext: ".html", norm: normalizeText},
		{name: "csv", render: RenderCSV, ext: ".csv", norm: normalizeText},
	}

	samples := []string{
		"sample.mp4",
		"sample.mkv",
		"sample.ts",
		"sample.avi",
		"sample.mpg",
		"sample.vob",
		"sample_ac3.vob",
		"sample.mp3",
		"sample.flac",
		"sample.wav",
		"sample.ogg",
	}

	goldenDir := filepath.Join("internal", "mediainfo", "testdata", "golden")

	for _, sample := range samples {
		t.Run(sample, func(t *testing.T) {
			report, err := AnalyzeFile(filepath.Join("samples", sample))
			if err != nil {
				t.Fatalf("analyze sample: %v", err)
			}
			reports := []Report{report}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					got := tc.norm(tc.render(reports))
					goldenPath := filepath.Join(goldenDir, sample+tc.ext)

					if *updateGolden {
						if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
							t.Fatalf("mkdir: %v", err)
						}
						if err := os.WriteFile(goldenPath, []byte(got), 0o600); err != nil {
							t.Fatalf("write golden: %v", err)
						}
						return
					}

					wantBytes, err := os.ReadFile(goldenPath)
					if err != nil {
						t.Fatalf("read golden (%s): %v (run: go test ./... -update)", goldenPath, err)
					}
					want := tc.norm(string(wantBytes))

					if got != want {
						t.Fatalf("golden mismatch: %s (run: go test ./... -update)", goldenPath)
					}
				})
			}
		})
	}
}

func normalizeText(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

var (
	reJSONCreatedUTC   = regexp.MustCompile(`"File_Created_Date":"[^"]*"`)
	reJSONCreatedLocal = regexp.MustCompile(`"File_Created_Date_Local":"[^"]*"`)
	reJSONModUTC       = regexp.MustCompile(`"File_Modified_Date":"[^"]*"`)
	reJSONModLocal     = regexp.MustCompile(`"File_Modified_Date_Local":"[^"]*"`)

	reXMLCreatedUTC   = regexp.MustCompile(`<File_Created_Date>[^<]*</File_Created_Date>`)
	reXMLCreatedLocal = regexp.MustCompile(`<File_Created_Date_Local>[^<]*</File_Created_Date_Local>`)
	reXMLModUTC       = regexp.MustCompile(`<File_Modified_Date>[^<]*</File_Modified_Date>`)
	reXMLModLocal     = regexp.MustCompile(`<File_Modified_Date_Local>[^<]*</File_Modified_Date_Local>`)
)

func normalizeJSONLike(s string) string {
	s = normalizeText(s)
	s = reJSONCreatedUTC.ReplaceAllString(s, `"File_Created_Date":"<redacted>"`)
	s = reJSONCreatedLocal.ReplaceAllString(s, `"File_Created_Date_Local":"<redacted>"`)
	s = reJSONModUTC.ReplaceAllString(s, `"File_Modified_Date":"<redacted>"`)
	s = reJSONModLocal.ReplaceAllString(s, `"File_Modified_Date_Local":"<redacted>"`)
	return s
}

func normalizeXMLLike(s string) string {
	s = normalizeText(s)
	s = reXMLCreatedUTC.ReplaceAllString(s, `<File_Created_Date><redacted></File_Created_Date>`)
	s = reXMLCreatedLocal.ReplaceAllString(s, `<File_Created_Date_Local><redacted></File_Created_Date_Local>`)
	s = reXMLModUTC.ReplaceAllString(s, `<File_Modified_Date><redacted></File_Modified_Date>`)
	s = reXMLModLocal.ReplaceAllString(s, `<File_Modified_Date_Local><redacted></File_Modified_Date_Local>`)
	return s
}
