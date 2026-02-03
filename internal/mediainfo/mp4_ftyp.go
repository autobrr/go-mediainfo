package mediainfo

import "strings"

func parseFtyp(payload []byte) []Field {
	if len(payload) < 8 {
		return nil
	}
	major := string(payload[0:4])
	compat := []string{}
	for i := 8; i+4 <= len(payload); i += 4 {
		compat = append(compat, string(payload[i:i+4]))
	}
	fields := []Field{}
	if profile := mapMP4Profile(major); profile != "" {
		fields = append(fields, Field{Name: "Format profile", Value: profile})
	}
	if len(compat) > 0 {
		codec := major + " (" + strings.Join(compat, "/") + ")"
		fields = append(fields, Field{Name: "Codec ID", Value: codec})
	}
	return fields
}

func mapMP4Profile(major string) string {
	switch major {
	case "isom", "iso2", "iso3", "iso4", "iso5", "iso6", "iso7", "iso8", "iso9":
		return "Base Media"
	case "qt  ":
		return "QuickTime"
	default:
		return ""
	}
}
