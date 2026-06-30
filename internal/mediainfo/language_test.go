package mediainfo

import "testing"

func TestNormalizeLanguageCodeHindi(t *testing.T) {
	if got := normalizeLanguageCode("hin"); got != "hi" {
		t.Fatalf("normalizeLanguageCode(\"hin\") = %q, want hi", got)
	}
}

func TestFormatLanguageHindi(t *testing.T) {
	if got := formatLanguage("hi"); got != "Hindi" {
		t.Fatalf("formatLanguage(\"hi\") = %q, want Hindi", got)
	}
}

func TestFormatLanguageCommonISO639Codes(t *testing.T) {
	tests := map[string]string{
		"jpn":     "Japanese",
		"kor":     "Korean",
		"nld":     "Dutch",
		"iw":      "Hebrew",
		"uk":      "Ukrainian",
		"vi":      "Vietnamese",
		"pt-BR":   "Portuguese (BR)",
		"zh-Hant": "Chinese (Hant)",
		"en-US":   "English (US)",
	}
	for code, want := range tests {
		if got := formatLanguage(code); got != want {
			t.Fatalf("formatLanguage(%q) = %q, want %q", code, got, want)
		}
	}
}
