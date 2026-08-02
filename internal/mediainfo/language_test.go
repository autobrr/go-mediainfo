package mediainfo

import "testing"

func TestNormalizeLanguageCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hin", "hi"},
		{"spa-419", "es-419"},
		{"ES_419", "es-419"},
		{"zh_hans", "zh-Hans"},
		{"en-u-ca-gregory", "en-u-ca-gregory"},
		{"de-CH-1901", "de-CH-1901"},
		{"en-x-private-US", "en-x-private-us"},
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
		{"ES_419", "Spanish (Latin America)"},
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
		{"en-u-ca-gregory", "English (u-ca-gregory)"},
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
		"fil":    "Filipino",
		"fil-PH": "Filipino (PH)",
		"zxx":    "Silent",
		"pt-BR":  "Portuguese (BR)",
		"en-US":  "English (US)",
	}
	for code, want := range tests {
		if got := formatLanguage(code); got != want {
			t.Fatalf("formatLanguage(%q) = %q, want %q", code, got, want)
		}
	}
}
