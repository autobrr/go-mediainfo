package mediainfo

import (
	"strconv"
	"strings"
)

func normalizeWritingApplication(raw string) string {
	return strings.TrimSpace(raw)
}

// splitWritingApplication separates an application name from its version text.
// The normalized version omits a leading "v" while versionRaw preserves it.
func splitWritingApplication(raw string) (string, string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ""
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return "", "", ""
	}
	if len(parts) == 3 && parts[0] == "HandBrake" && len(parts[2]) == 10 {
		if _, err := strconv.ParseUint(parts[2], 10, 64); err == nil {
			return strings.Join(parts[:2], " "), parts[2], parts[2]
		}
	}
	name := parts[0]
	versionRaw := strings.TrimSpace(strings.TrimPrefix(raw, name))
	version := strings.TrimPrefix(versionRaw, "v")
	return name, version, versionRaw
}

// exposeWritingApplicationComponents reports whether MediaInfo splits the
// Matroska writing application into its optional name and version fields.
func exposeWritingApplicationComponents(name string, version string) bool {
	if name == "MakeMKV" || name == "libmakemkv" || name == "HandBrake" || strings.HasPrefix(name, "HandBrake ") || name == "libmkv" || name == "WKSmerge" {
		return version != ""
	}
	if name != "mkvmerge" {
		return false
	}
	versionNumber, _, _ := strings.Cut(version, " ")
	majorText, _, _ := strings.Cut(versionNumber, ".")
	major, err := strconv.Atoi(majorText)
	if err != nil || major <= 0 {
		return false
	}
	// MediaInfoLib leaves a small set of historical mkvmerge releases unsplit.
	switch versionNumber {
	case "5.3.0", "6.1.0", "6.5.0", "7.1.0", "7.2.0", "7.7.0", "7.8.0", "8.2.0", "8.3.0",
		"11.0.0", "15.0.0", "19.0.0", "35.0.0", "37.0.0", "45.0.0", "63.0.0", "92.0", "97.0":
		return false
	}
	return true
}
