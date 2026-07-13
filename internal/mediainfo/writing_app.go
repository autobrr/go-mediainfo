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
	name := parts[0]
	versionRaw := strings.TrimSpace(strings.TrimPrefix(raw, name))
	version := strings.TrimPrefix(versionRaw, "v")
	return name, version, versionRaw
}

// exposeWritingApplicationComponents reports whether MediaInfo splits the
// Matroska writing application into its optional name and version fields.
func exposeWritingApplicationComponents(name string, version string) bool {
	if name != "mkvmerge" {
		return false
	}
	majorText, _, _ := strings.Cut(version, ".")
	major, err := strconv.Atoi(majorText)
	if err != nil || major <= 0 {
		return false
	}
	// MediaInfo's splitter leaves mkvmerge v19 as a single application string.
	if major == 19 {
		return false
	}
	// MediaInfo's mkvmerge application splitter does not recognize codenames
	// containing an apostrophe, such as v97's "You Don't Have A Clue".
	return strings.Count(version, "'") <= 2
}
