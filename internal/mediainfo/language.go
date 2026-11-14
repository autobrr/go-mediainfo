package mediainfo

import "strings"

type languagePolicy struct {
	names, reverse, aliases map[string]string
}

var languagePolicies = loadLanguagePolicies()

func loadLanguagePolicies() map[reportTableVersion]languagePolicy {
	policies := make(map[reportTableVersion]languagePolicy)
	for _, version := range []reportTableVersion{reportTables23, reportTables26} {
		policy := languagePolicy{names: map[string]string{}, reverse: map[string]string{}, aliases: map[string]string{}}
		// The extract is sorted by the reference Translation map's key order.
		for line := range strings.SplitSeq(strings.TrimSuffix(string(readReportTable(version, "languages")), "\n"), "\n") {
			code, name, _ := strings.Cut(line, "\t")
			policy.names[code] = name
			key := strings.ToLower(name)
			if _, exists := policy.reverse[key]; !exists {
				policy.reverse[key] = code
			}
		}
		for line := range strings.SplitSeq(strings.TrimSuffix(string(readReportTable(version, "language-aliases")), "\n"), "\n") {
			code, primary, _ := strings.Cut(line, "\t")
			policy.aliases[code] = primary
		}
		policies[version] = policy
	}
	return policies
}

func normalizeLanguageCode(code string) string {
	return normalizeLanguage(code, reportTables23)
}

// normalizeLanguage maps only the primary component. Serialized suffix spelling
// and positional list members are metadata, independent of friendly display.
func normalizeLanguage(code string, version reportTableVersion) string {
	if strings.Contains(code, " / ") {
		members := strings.Split(code, " / ")
		for i, member := range members {
			members[i] = normalizeLanguage(member, version)
		}
		return strings.Join(members, " / ")
	}
	code = strings.TrimSpace(code)
	primary, suffix, hasSuffix := strings.Cut(code, "-")
	lower := strings.ToLower(primary)
	policy := languagePolicies[version]
	if len(primary) == 3 && policy.aliases[lower] != "" {
		primary = policy.aliases[lower]
	} else if len(primary) > 3 {
		if found := policy.reverse[lower]; found != "" {
			primary = found
		}
	} else if isAlphaNum(primary) {
		primary = lower
	}
	// These deprecated two-letter codes are already accepted by container readers.
	switch primary {
	case "iw":
		primary = "he"
	case "in":
		primary = "id"
	case "ji":
		primary = "yi"
	}
	switch primary {
	case "", "und", "mis", "???":
		return ""
	}
	if hasSuffix {
		return primary + "-" + suffix
	}
	return primary
}

func formatLanguage(code string) string {
	return formatLanguageVersion(code, reportTables23)
}

func formatLanguageVersion(code string, version reportTableVersion) string {
	if strings.Contains(code, " / ") {
		members := strings.Split(code, " / ")
		for i, member := range members {
			members[i] = formatLanguageVersion(member, version)
		}
		return strings.Join(members, " / ")
	}
	code = normalizeLanguage(code, version)
	if code == "" {
		return ""
	}
	policy := languagePolicies[version]
	if name := policy.names[code]; name != "" {
		return name
	}
	for _, char := range code {
		if char != '-' && !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			return code
		}
	}
	primary, suffix, hasSuffix := strings.Cut(code, "-")
	name := policy.names[primary]
	if name == "" {
		name = primary
	}
	if !hasSuffix {
		return name
	}
	if strings.Contains(suffix, "-") {
		return name + "-" + suffix
	}
	if len(suffix) == 2 && suffix[0] >= 'a' && suffix[0] <= 'z' && suffix[1] >= 'a' && suffix[1] <= 'z' {
		suffix = strings.ToUpper(suffix)
	}
	return name + " (" + suffix + ")"
}

func isAlphaNum(s string) bool {
	for i := range len(s) {
		if (s[i] < 'A' || s[i] > 'Z') && (s[i] < 'a' || s[i] > 'z') && (s[i] < '0' || s[i] > '9') {
			return false
		}
	}
	return true
}
