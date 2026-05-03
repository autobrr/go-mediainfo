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