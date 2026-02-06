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
	i := 1
	if i < len(parts) && isAlphaString(parts[i]) && len(parts[i]) == 4 {
		// Script subtag: title case (e.g. Hant, Latn)
		s := parts[i]
		out = append(out, strings.ToUpper(s[:1])+strings.ToLower(s[1:]))
		i++
	}
	if i < len(parts) && ((isAlphaString(parts[i]) && len(parts[i]) == 2) || (isDigitString(parts[i]) && len(parts[i]) == 3)) {
		// Region subtag: upper case (e.g. US, 419)
		out = append(out, strings.ToUpper(parts[i]))
		i++
	}
	for ; i < len(parts); i++ {
		p := strings.TrimSpace(parts[i])
		if p == "" {
			continue
		}
		out = append(out, strings.ToLower(p))
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

func isAlphaString(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			continue
		}
		return false
	}
	return true
}

func isDigitString(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			continue
		}
		return false
	}
	return true
}
