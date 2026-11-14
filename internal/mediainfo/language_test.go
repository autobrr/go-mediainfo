package mediainfo

import (
	"strings"
	"testing"
)

func TestImportedLanguageMappings(t *testing.T) {
	for _, version := range []reportTableVersion{reportTables23, reportTables26} {
		for line := range strings.SplitSeq(strings.TrimSpace(string(readReportTable(version, "languages"))), "\n") {
			code, want, _ := strings.Cut(line, "\t")
			if code == "mis" || code == "und" {
				continue
			}
			if got := formatLanguageVersion(code, version); got != want {
				t.Errorf("%s %s: %q, want %q", version, code, got, want)
			}
		}
	}
	for _, tc := range []struct {
		version    reportTableVersion
		code, want string
	}{
		{reportTables23, "fil", "fil"}, {reportTables26, "fil", "Filipino"},
		{reportTables23, "zxx", "zxx"}, {reportTables26, "zxx", "No linguistic content or not applicable"},
	} {
		if got := formatLanguageVersion(tc.code, tc.version); got != tc.want {
			t.Errorf("%s %s: %q", tc.version, tc.code, got)
		}
	}
}

func TestNormalizeLanguageCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hin", "hi"},
		{"spa-419", "es-419"},
		{"ES_419", "ES_419"},
		{"zh_hans", "zh_hans"},
		{"en-u-ca-gregory", "en-u-ca-gregory"},
		{"de-CH-1901", "de-CH-1901"},
		{"en-x-private-US", "en-x-private-US"},
		{"EN-us", "en-us"},
		{"scc-Latn-rs", "sr-Latn-rs"},
		{"mol", "mo"},
		{"tib", "bo"},
		{"bod", "bod"},
		{"eNgLiSh", "en"},
		{"Luba-Katanga", "lu"},
		{" LUBA-KATANGA ", "lu"},
		{"und / mis / ???", ""},
		{"Unrecognized Name", "Unrecognized Name"},
		{"mis", ""},
		{"???", ""},
		{" / ENG-us / und / en / ", " / en-us /  / en / "},
		{"en--US", "en--US"},
		{"en-a", "en-a"},
		{"en-abc-abc-abc-abc", "en-abc-abc-abc-abc"},
		{"en-variant-variant", "en-variant-variant"},
		{"en-u-ca-gregory-u-nu-latn", "en-u-ca-gregory-u-nu-latn"},
		{"und", ""},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := normalizeLanguageCode(tc.input); got != tc.want {
				t.Errorf("normalizeLanguageCode(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatLanguage(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"es-419", "Spanish (Latin America)"},
		{"spa-419", "Spanish (Latin America)"},
		{"ES_419", "ES_419"},
		{"en-GB", "English (GB)"},
		{"es-ES", "Spanish (ES)"},
		{"es-CL", "Spanish (CL)"},
		{"es-DO", "Spanish (DO)"},
		{"id-ID", "Indonesian (ID)"},
		{"ms-MY", "Malay (MY)"},
		{"cmn-Hans", "Mandarin (Hans)"},
		{"cmn-Hant", "Mandarin (Hant)"},
		{"yue-Hant", "Cantonese (Hant)"},
		{"pa", "Panjabi"},
		{"zh-Hans", "Chinese (Simplified)"},
		{"zh-Hant", "Chinese (Traditional)"},
		{"zh-TW", "Chinese (Taiwan)"},
		{"en-u-ca-gregory", "English-u-ca-gregory"},
		{"en-us", "English (United States)"},
		{"Luba-Katanga", "Luba-Katanga"},
		{"und / mis / ???", ""},
		{"khk-Cyrl", "khk (Cyrl)"},
		{"zh-Hant-HK", "Chinese-Hant-HK"},
		{"yue-Hant-HK", "Cantonese-Hant-HK"},
		{" / en / und / en / ", " / English /  / English / "},
		{"und", ""},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := formatLanguage(tc.input); got != tc.want {
				t.Errorf("formatLanguage(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatLanguageCommonISO639Codes(t *testing.T) {
	tests := map[string]string{
		"jpn":    "Japanese",
		"kor":    "Korean",
		"nld":    "Dutch",
		"iw":     "Hebrew",
		"uk":     "Ukrainian",
		"vi":     "Vietnamese",
		"fil":    "fil",
		"fil-PH": "fil (PH)",
		"zxx":    "zxx",
		"km":     "Khmer",
		"pt-BR":  "Portuguese (BR)",
		"en-US":  "English (US)",
	}
	for code, want := range tests {
		if got := formatLanguage(code); got != want {
			t.Fatalf("formatLanguage(%q) = %q, want %q", code, got, want)
		}
	}
}
