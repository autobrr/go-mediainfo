package mediainfo

import (
	"fmt"
	"strings"
)

var languageNames = map[string]string{
	"en": "English",
	"fr": "French",
	"es": "Spanish",
	"de": "German",
	"it": "Italian",
	"pt": "Portuguese",
}

var languageMap3To2 = map[string]string{
	"eng": "en",
	"fra": "fr",
	"fre": "fr",
	"spa": "es",
	"deu": "de",
	"ger": "de",
	"ita": "it",
	"por": "pt",
}

func normalizeLanguageCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	code = strings.ReplaceAll(code, "_", "-")
	parts := strings.Split(code, "-")
	if len(parts) == 0 {
		return code
	}
	lang := strings.ToLower(parts[0])
	if lang == "und" {
		return ""
	}
	if mapped, ok := languageMap3To2[lang]; ok {
		lang = mapped
	}
	out := []string{lang}
	for i := 1; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])
		if part == "" {
			continue
		}
		// BCP-47-ish casing:
		// - script: 4 alpha, Title Case (Hans)
		// - region: 2 alpha or 3 digit, upper case (US / 419)
		if len(part) == 4 && isAlpha(part) {
			out = append(out, strings.ToUpper(part[:1])+strings.ToLower(part[1:]))
			continue
		}
		if (len(part) == 2 && isAlpha(part)) || (len(part) == 3 && isDigit(part)) {
			out = append(out, strings.ToUpper(part))
			continue
		}
		out = append(out, strings.ToLower(part))
	}
	return strings.Join(out, "-")
}

func formatLanguage(code string) string {
	normalized := normalizeLanguageCode(code)
	if normalized == "" {
		return ""
	}
	parts := strings.Split(normalized, "-")
	name := languageNames[parts[0]]
	if name == "" {
		return code
	}
	if len(parts) > 1 {
		return fmt.Sprintf("%s (%s)", name, strings.ToUpper(parts[1]))
	}
	return name
}

func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}

func isDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
