package mediainfo

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const uppercaseHexDigits = "0123456789ABCDEF"

// escapeOutputControls renders control characters visibly so untrusted media
// metadata cannot create terminal actions or forge text/CSV records.
func escapeOutputControls(value string) string {
	first := -1
	for i := 0; i < len(value); {
		r, size := utf8.DecodeRuneInString(value[i:])
		invalidControlByte := r == utf8.RuneError && size == 1 && value[i] >= 0x80 && value[i] <= 0x9F
		if r < 0x20 || r == 0x7F || r >= 0x80 && r <= 0x9F || invalidControlByte {
			first = i
			break
		}
		i += size
	}
	if first < 0 {
		return value
	}
	var out strings.Builder
	out.Grow(len(value) + 8)
	out.WriteString(value[:first])
	for i := first; i < len(value); {
		r, size := utf8.DecodeRuneInString(value[i:])
		if r == utf8.RuneError && size == 1 && value[i] >= 0x80 && value[i] <= 0x9F {
			out.WriteString(`\x`)
			out.WriteByte(uppercaseHexDigits[value[i]>>4])
			out.WriteByte(uppercaseHexDigits[value[i]&0x0F])
			i++
			continue
		}
		switch r {
		case '\t':
			out.WriteString(`\t`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\x1B':
			out.WriteString(`\x1B`)
		case '\x7F':
			out.WriteString(`\x7F`)
		default:
			switch {
			case r < 0x20:
				out.WriteString(`\x`)
				out.WriteByte(uppercaseHexDigits[byte(r)>>4])
				out.WriteByte(uppercaseHexDigits[byte(r)&0x0F])
			case r >= 0x80 && r <= 0x9F:
				out.WriteString(`\u00`)
				out.WriteByte(uppercaseHexDigits[byte(r)>>4])
				out.WriteByte(uppercaseHexDigits[byte(r)&0x0F])
			default:
				out.WriteString(value[i : i+size])
			}
		}
		i += size
	}
	return out.String()
}

// safeCSVOutputValue escapes record-breaking controls, neutralizes formula
// prefixes, and quotes values that contain quotes or would inject another
// formula-bearing cell. Ordinary delimiter-bearing values remain unchanged for
// CLI parity.
func safeCSVOutputValue(value string) string {
	escaped := escapeOutputControls(value)
	formula := csvFormulaCandidate(value)
	if formula {
		escaped = "'" + escaped
	}
	if strings.ContainsRune(value, '"') || csvInjectedFormulaCandidate(value) || formula && strings.ContainsRune(value, ',') {
		return `"` + strings.ReplaceAll(escaped, `"`, `""`) + `"`
	}
	return escaped
}

// csvInjectedFormulaCandidate detects an unquoted comma followed by a formula
// prefix, including a quoted injected cell such as `safe,"=1+1"`.
func csvInjectedFormulaCandidate(value string) bool {
	for offset := strings.IndexByte(value, ','); offset >= 0; {
		candidate := strings.TrimLeftFunc(value[offset+1:], csvFormulaPadding)
		candidate = strings.TrimLeft(candidate, `"`)
		candidate = strings.TrimLeftFunc(candidate, csvFormulaPadding)
		if csvFormulaCandidate(candidate) {
			return true
		}
		next := strings.IndexByte(value[offset+1:], ',')
		if next < 0 {
			break
		}
		offset += next + 1
	}
	return false
}

// csvFormulaCandidate reports whether a spreadsheet may interpret a metadata
// value as a formula rather than text.
func csvFormulaCandidate(value string) bool {
	trimmed := strings.TrimLeftFunc(value, csvFormulaPadding)
	if trimmed == "" {
		return false
	}
	switch trimmed[0] {
	case '=', '@':
		return true
	case '+', '-':
		return !isSignedMeasurement(trimmed)
	default:
		return false
	}
}

func csvFormulaPadding(r rune) bool {
	return unicode.IsSpace(r) || r < 0x20 || r == 0x7F || r == '\uFEFF' || r >= '\u200B' && r <= '\u200D' || r == '\u2060'
}

// isSignedMeasurement recognizes signed numeric values followed only by a
// whitespace-separated display unit. It prevents formula hardening from
// changing ordinary values such as "-83 ms".
func isSignedMeasurement(value string) bool {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return false
	}
	number := strings.TrimSuffix(parts[0], "%")
	if _, err := strconv.ParseFloat(number, 64); err != nil {
		return false
	}
	for _, part := range parts[1:] {
		for _, r := range part {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				continue
			}
			switch r {
			case '.', ',', '/', '%', '(', ')', '[', ']', '^':
				continue
			default:
				return false
			}
		}
	}
	return true
}
