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
		if r == utf8.RuneError && size == 1 || unsafeOutputControl(r) {
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
		if r == utf8.RuneError && size == 1 {
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
		case '\u2028':
			out.WriteString(`\u2028`)
		case '\u2029':
			out.WriteString(`\u2029`)
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

// unsafeOutputControl reports code points that can trigger terminal actions or
// split a text record without using the renderer's own framing.
func unsafeOutputControl(r rune) bool {
	return r < 0x20 || r == 0x7F || r >= 0x80 && r <= 0x9F || r == '\u2028' || r == '\u2029'
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
	attachedPercent := strings.HasSuffix(parts[0], "%")
	if !isCSVDisplayNumber(strings.TrimSuffix(parts[0], "%"), true) {
		return false
	}
	if len(parts) == 1 {
		return true
	}
	if attachedPercent {
		return false
	}
	for i := 1; i < len(parts); {
		if !isCSVDisplayUnit(parts[i]) {
			return false
		}
		i++
		if i == len(parts) {
			return true
		}
		if !isCSVDisplayNumber(parts[i], false) {
			return false
		}
		i++
		if i == len(parts) {
			return false
		}
	}
	return true
}

// isCSVDisplayNumber reports whether value is a signed or unsigned decimal
// display number, optionally requiring an explicit sign.
func isCSVDisplayNumber(value string, requireSign bool) bool {
	if value == "" {
		return false
	}
	if requireSign {
		if value[0] != '+' && value[0] != '-' {
			return false
		}
		value = value[1:]
	}
	if value == "" {
		return false
	}
	hasDigit := false
	for _, r := range value {
		if unicode.IsDigit(r) {
			hasDigit = true
			continue
		}
		switch r {
		case '.', 'e', 'E', '+', '-':
		default:
			return false
		}
	}
	if !hasDigit {
		return false
	}
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

// isCSVDisplayUnit reports whether value is a recognized human-readable CSV
// unit suffix used by the output sanitizer.
func isCSVDisplayUnit(value string) bool {
	switch value {
	case "%", "ns", "us", "µs", "ms", "s", "min", "h",
		"dB", "dBFS", "LU", "LUFS",
		"frame", "frames", "sample", "samples",
		"fps", "FPS", "SPF", "Hz", "kHz", "MHz", "GHz",
		"b/s", "kb/s", "Mb/s", "Gb/s", "bps", "Kbps", "Mbps", "Gbps",
		"bit", "bits", "Byte", "Bytes", "pixel", "pixels", "channel", "channels", "cd/m2":
		return true
	default:
		return false
	}
}
