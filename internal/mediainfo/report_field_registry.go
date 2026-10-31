package mediainfo

import (
	"embed"
	"strings"
)

// reportTableVersion selects reporting conventions, independently of parser facts.
type reportTableVersion string

const (
	reportTables23 reportTableVersion = "23.04"
	reportTables26 reportTableVersion = "26.10"
)

//go:embed report_tables/*.tsv
var reportTableFiles embed.FS

type reportFieldRow struct {
	Label, Options string
}

type reportKindTable struct {
	Fields  map[string]reportFieldRow
	Aliases map[string]string
	Order   map[string]int
}

type reportVersionTables struct {
	Kinds   map[StreamKind]*reportKindTable
	Labels  map[string]string
	Formats map[string]string
	Codecs  map[string]codecDescription
	Values  map[string]string
}

// reportTables contains checked-in extracts of the pinned official tables.
// Source hashes and licenses accompany the data; regeneration never runs at runtime.
var reportTables = loadReportTables()

type codecDescription struct{ Format, Hint, Info string }

func loadReportTables() map[reportTableVersion]*reportVersionTables {
	result := make(map[reportTableVersion]*reportVersionTables)
	for _, version := range []reportTableVersion{reportTables23, reportTables26} {
		tables := &reportVersionTables{Kinds: make(map[StreamKind]*reportKindTable), Labels: make(map[string]string), Formats: make(map[string]string), Codecs: make(map[string]codecDescription), Values: make(map[string]string)}
		data, _ := reportTableFiles.ReadFile("report_tables/" + string(version) + "-fields.tsv")
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			cols := strings.Split(line, "\t")
			kind := StreamKind(cols[0])
			table := tables.Kinds[kind]
			if table == nil {
				table = &reportKindTable{Fields: make(map[string]reportFieldRow), Aliases: make(map[string]string), Order: map[string]int{"@type": -3, "@typeorder": -2, "extra": unregisteredFieldOrder - 1}}
				tables.Kinds[kind] = table
			}
			name := canonicalTableName(cols[1])
			row := reportFieldRow{Options: cols[2], Label: cols[3]}
			table.Order[name] = len(table.Fields) * 100
			// JSON/XML serialize schema slashes as underscores at the same position.
			table.Order[strings.ReplaceAll(name, "/", "_")] = table.Order[name]
			table.Fields[name] = row
			base := strings.Split(name, "/String")[0]
			if _, exists := table.Aliases[row.Label]; !exists || strings.HasPrefix(row.Options, "Y") {
				table.Aliases[row.Label] = base
			}
			if _, exists := tables.Labels[base]; !exists {
				tables.Labels[base] = row.Label
			}
		}
		data, _ = reportTableFiles.ReadFile("report_tables/" + string(version) + "-formats.tsv")
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			cols := strings.Split(line, "\t")
			tables.Formats[cols[0]] = cols[1]
		}
		data, _ = reportTableFiles.ReadFile("report_tables/" + string(version) + "-codecs.tsv")
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			cols := strings.Split(line, "\t")
			key := strings.Join(cols[:3], "\t")
			if _, exists := tables.Codecs[key]; !exists {
				tables.Codecs[key] = codecDescription{Format: cols[3], Hint: cols[4], Info: cols[5]}
			}
		}
		result[version] = tables
		data, _ = reportTableFiles.ReadFile("report_tables/" + string(version) + "-values.tsv")
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			key, value, _ := strings.Cut(line, "\t")
			tables.Values[key] = value
		}
	}
	// Keep the explicitly adopted HDR scalars in their compatible family position.
	old, newer := result[reportTables23], result[reportTables26]
	for _, kind := range []StreamKind{StreamGeneral, StreamVideo, StreamAudio, StreamText, StreamImage, StreamMenu} {
		table, newTable := old.Kinds[kind], newer.Kinds[kind]
		if table == nil || newTable == nil {
			continue
		}
		for offset, name := range []string{"MasteringDisplay_Luminance_Min", "MasteringDisplay_Luminance_Max"} {
			row, exists := newTable.Fields[name]
			anchor, anchored := table.Order["MasteringDisplay_Luminance_Source"]
			if _, present := table.Fields[name]; exists && anchored && !present {
				table.Fields[name] = row
				table.Order[name] = anchor + offset + 1
				table.Aliases[row.Label] = name
				old.Labels[name] = row.Label
			}
		}
		for _, name := range []string{"MaxCLL/String", "MaxFALL/String"} {
			row, exists := newTable.Fields[name]
			base := strings.TrimSuffix(name, "/String")
			anchor, anchored := table.Order[base]
			if _, present := table.Fields[name]; exists && anchored && !present {
				table.Fields[name] = row
				table.Order[name] = anchor + 1
			}
		}
	}
	return result
}

// canonicalTableName preserves the public structured names for reference aliases.
func canonicalTableName(name string) string {
	base, _, _ := strings.Cut(name, "/")
	if base == "Channel(s)" || base == "Channel(s)_Original" {
		return "Channels" + strings.TrimPrefix(name, "Channel(s)")
	}
	return name
}

func reportKindPolicy(version reportTableVersion, kind StreamKind) *reportKindTable {
	return reportTables[version].Kinds[kind]
}

func reportVersion(store *fieldStore) reportTableVersion {
	if store != nil && store.tableVersion != "" {
		return store.tableVersion
	}
	dvd, _ := rawTextDVDContext(store)
	if dvd {
		return reportTables26
	}
	return reportTables23
}

// reportFieldIdentity unifies canonical, raw-sibling and legacy-label spellings.
// Unknown names remain literal metadata; no whitespace or underscore heuristic applies.
func reportFieldIdentity(kind StreamKind, name, label string) string {
	name = strings.TrimPrefix(name, "Text/")
	name = canonicalTableName(canonicalRawFieldName(name))
	base := strings.TrimSuffix(name, "/String")
	table := reportKindPolicy(reportTables23, kind)
	if table != nil {
		if _, ok := table.Fields[name]; ok {
			return strings.Split(name, "/String")[0]
		}
		if _, ok := table.Fields[base]; ok {
			return base
		}
		if alias := table.Aliases[name]; alias != "" {
			return alias
		}
		if alias := table.Aliases[label]; alias != "" {
			return alias
		}
	}
	for _, candidate := range []string{name, label} {
		switch candidate {
		case "Chroma subsampling position":
			return "ChromaSubsampling_Position"
		case "Format settings, Picture structure":
			return "Format_Settings_PictureStructure"
		case "Delay relative to video":
			return "Video_Delay"
		case "Original delay":
			return "Delay_Original"
		case "Original source medium ID", "ID in the original source medium":
			return "OriginalSourceMedium_ID"
		case "Statistics Tags Issue", "Statistics_Tags_Issue":
			return "Statistics_Tags_Issue"
		case "Service name":
			return "ServiceName"
		case "Service provider":
			return "ServiceProvider"
		case "Service type":
			return "ServiceType"
		}
		if _, ok := reportTables[reportTables23].Labels[candidate]; ok {
			return candidate
		}
	}
	return name
}

func reportFieldLabel(version reportTableVersion, identity, fallback string) string {
	if label := reportTables[version].Labels[identity]; label != "" && (label != identity || fallback == identity) {
		return label
	}
	return fallback
}

func reportFieldRank(version reportTableVersion, kind StreamKind, name string) int {
	if table := reportKindPolicy(version, kind); table != nil {
		if rank, ok := table.Order[canonicalTableName(canonicalRawFieldName(name))]; ok {
			return rank
		}
	}
	return unregisteredFieldOrder
}

// reportTextVisible applies standard table options plus explicit runtime options
// for MP4's source-delay extras. Unknown user metadata keeps its supplied policy.
func reportTextVisible(version reportTableVersion, kind StreamKind, identity string) bool {
	if identity == "Source_Delay" || identity == "Source_Delay_Source" {
		return false
	}
	table := reportKindPolicy(version, kind)
	if table == nil {
		return true
	}
	for _, name := range []string{identity + "/String", identity} {
		if row, ok := table.Fields[name]; ok {
			return strings.HasPrefix(row.Options, "Y")
		}
	}
	return true
}

// These existing raw aliases are the reference CLI's shortened field names.
// They affect presentation only; canonical keys retain their full spelling.
var rawReportFieldNames = map[string]string{
	"DisplayAspectRatio_Original/String": "DisplayAspectRatio_Original/Stri",
	"Format_Settings_SliceCount/String":  "Format_Settings_SliceCount/Strin",
	"transfer_characteristics_Original":  "transfer_characteristics_Origina",
}

func canonicalRawFieldName(label string) string {
	for name, alias := range rawReportFieldNames {
		if alias == label {
			return name
		}
	}
	return label
}

func rawReportFieldName(name string) string {
	if alias := rawReportFieldNames[name]; alias != "" {
		return alias
	}
	return name
}
