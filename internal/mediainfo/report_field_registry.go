package mediainfo

import (
	"cmp"
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

// readReportTable fails initialization if a required checked-in asset is missing.
func readReportTable(version reportTableVersion, kind string) []byte {
	data, err := reportTableFiles.ReadFile("report_tables/" + string(version) + "-" + kind + ".tsv")
	if err != nil {
		panic(err)
	}
	return data
}

func loadReportTables() map[reportTableVersion]*reportVersionTables {
	result := make(map[reportTableVersion]*reportVersionTables)
	for _, version := range []reportTableVersion{reportTables23, reportTables26} {
		tables := &reportVersionTables{Kinds: make(map[StreamKind]*reportKindTable), Labels: make(map[string]string), Formats: make(map[string]string), Codecs: make(map[string]codecDescription), Values: make(map[string]string)}
		data := readReportTable(version, "fields")
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
		data = readReportTable(version, "formats")
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			cols := strings.Split(line, "\t")
			tables.Formats[cols[0]] = cols[1]
		}
		if version == reportTables23 {
			data = readReportTable(version, "codecs")
			for line := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "\n") {
				cols := strings.Split(line, "\t")
				key := strings.Join(cols[:3], "\t")
				if _, exists := tables.Codecs[key]; !exists {
					tables.Codecs[key] = codecDescription{Format: cols[3], Hint: cols[4], Info: cols[5]}
				}
			}
		}
		result[version] = tables
		data = readReportTable(version, "values")
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
		if row, exists := newTable.Fields["HDR_Format_Compression"]; exists {
			if anchor, anchored := table.Order["HDR_Format_Settings"]; anchored {
				table.Fields["HDR_Format_Compression"] = row
				table.Order["HDR_Format_Compression"] = anchor + 1
			}
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
	if storeUsesDVDReference(store) {
		return reportTables26
	}
	return reportTables23
}

// dvdReferenceFormat reports whether a General or Menu format selects the
// DVD v26.10 reference (ADR 0002).
func dvdReferenceFormat(kind StreamKind, format string) bool {
	return kind == StreamGeneral && format == "DVD Video" || kind == StreamMenu && format == "DVD-Video"
}

// storeUsesDVDReference reports whether any General or Menu stream of a store
// that the parser did not mark selects the DVD reference.
func storeUsesDVDReference(store *fieldStore) bool {
	if store == nil {
		return false
	}
	for index := range store.streams {
		stream := &store.streams[index]
		if stream.Kind != StreamGeneral && stream.Kind != StreamMenu {
			continue
		}
		if dvdReferenceFormat(stream.Kind, storedScalarValues(stream)["Format"]) {
			return true
		}
	}
	return false
}

// reportFieldIdentity unifies spellings using the selected reference's aliases.
// Unknown names remain literal metadata; no whitespace or underscore heuristic applies.
func reportFieldIdentity(version reportTableVersion, kind StreamKind, name, label string) string {
	name = strings.TrimPrefix(name, "Text/")
	name = canonicalTableName(canonicalRawFieldName(name))
	base := strings.TrimSuffix(name, "/String")
	table := reportKindPolicy(version, kind)
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
		if compat, ok := compatibilityLabels[candidate]; ok {
			return compat.identity
		}
		if _, ok := reportTables[version].Labels[candidate]; ok {
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
	if sourceDelayField(identity) {
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

func sourceDelayField(identity string) bool {
	return identity == "Source_Delay" || identity == "Source_Delay_Source"
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

// compatibilityLabel names a producer label that the pinned tables do not list.
// identity is the canonical field; raw is MediaInfo's raw name, or empty when
// raw text keeps the friendly label.
type compatibilityLabel struct{ identity, raw string }

var compatibilityLabels = map[string]compatibilityLabel{
	"Chroma subsampling position":        {identity: "ChromaSubsampling_Position"},
	"Format settings, Picture structure": {identity: "Format_Settings_PictureStructure", raw: "Format_Settings_PictureStructure"},
	"Delay relative to video":            {identity: "Video_Delay", raw: "Video_Delay/String"},
	"Original delay":                     {identity: "Delay_Original", raw: "Delay_Original/String"},
	"Original source medium ID":          {identity: "OriginalSourceMedium_ID", raw: "OriginalSourceMedium_ID/String"},
	"ID in the original source medium":   {identity: "OriginalSourceMedium_ID", raw: "OriginalSourceMedium_ID/String"},
	"Statistics Tags Issue":              {identity: "Statistics_Tags_Issue", raw: "Statistics Tags Issue"},
	"Statistics_Tags_Issue":              {identity: "Statistics_Tags_Issue", raw: "Statistics Tags Issue"},
	"Service name":                       {identity: "ServiceName", raw: "ServiceName"},
	"Service provider":                   {identity: "ServiceProvider", raw: "ServiceProvider"},
	"Service type":                       {identity: "ServiceType", raw: "ServiceType"},
}

// rawFieldLabelAlias returns MediaInfo's raw name for a friendly label, or an
// empty string when the label has no alias.
func rawFieldLabelAlias(friendly string) string {
	if label := rawFieldLabelAliases[friendly]; label != "" {
		return label
	}
	return compatibilityLabels[friendly].raw
}

// rawFieldLabel resolves a friendly label to MediaInfo's raw name. A label
// without an alias stays as it is.
func rawFieldLabel(friendly string) string {
	return cmp.Or(rawFieldLabelAlias(friendly), friendly)
}

// rawFieldLabelAliases maps compatibility/display labels to MediaInfo raw names.
// Labels that also need a canonical identity live in compatibilityLabels.
var rawFieldLabelAliases = map[string]string{
	"ID":                                 "ID/String",
	"Menu ID":                            "MenuID/String",
	"Unique ID":                          "UniqueID/String",
	"Complete name":                      "CompleteName",
	"Format":                             "Format/String",
	"Format version":                     "Format_Version",
	"Format profile":                     "Format_Profile",
	"Format settings":                    "Format_Settings",
	"Commercial name":                    "Format_Commercial_IfAny",
	"Muxing mode":                        "MuxingMode",
	"Muxing mode, more info":             "MuxingMode_MoreInfo",
	"HDR format":                         "HDR_Format/String",
	"Format settings, CABAC":             "Format_Settings_CABAC/String",
	"Format settings, Reference frames":  "Format_Settings_RefFrames/String",
	"Format settings, BVOP":              "Format_Settings_BVOP/String",
	"Format settings, QPel":              "Format_Settings_QPel/String",
	"Format settings, GMC":               "Format_Settings_GMC/String",
	"Format settings, Matrix":            "Format_Settings_Matrix/String",
	"Format settings, GOP":               "Format_Settings_GOP",
	"Format compression":                 "Format_Compression",
	"Codec ID":                           "CodecID",
	"Codec configuration box":            "CodecConfigurationBox",
	"Codec ID/Info":                      "CodecID/Info",
	"Duration":                           "Duration/String",
	"Source duration":                    "Source_Duration/String",
	"Overall bit rate mode":              "OverallBitRate_Mode/String",
	"Overall bit rate":                   "OverallBitRate/String",
	"Bit rate mode":                      "BitRate_Mode/String",
	"Bit rate":                           "BitRate/String",
	"Nominal bit rate":                   "BitRate_Nominal/String",
	"Maximum bit rate":                   "BitRate_Maximum/String",
	"File size":                          "FileSize/String",
	"Stream size":                        "StreamSize/String",
	"Source stream size":                 "Source_StreamSize/String",
	"Width":                              "Width/String",
	"Height":                             "Height/String",
	"Display aspect ratio":               "DisplayAspectRatio/String",
	"Original display aspect ratio":      "DisplayAspectRatio_Original/Stri",
	"Frame rate mode":                    "FrameRate_Mode/String",
	"Frame rate":                         "FrameRate/String",
	"Color space":                        "ColorSpace",
	"Chroma subsampling":                 "ChromaSubsampling/String",
	"Bit depth":                          "BitDepth/String",
	"Bit depth, detected":                "BitDepth_Detected/String",
	"Scan type":                          "ScanType/String",
	"Scan order":                         "ScanOrder/String",
	"Scan type, store method":            "ScanType_StoreMethod/String",
	"Active format description":          "ActiveFormatDescription/String",
	"Compression mode":                   "Compression_Mode/String",
	"Bits/(Pixel*Frame)":                 "Bits-(Pixel*Frame)",
	"Time code of first frame":           "TimeCode_FirstFrame",
	"Time code source":                   "TimeCode_Source",
	"GOP, Open/Closed":                   "Gop_OpenClosed/String",
	"GOP, Open/Closed of first frame":    "Gop_OpenClosed_FirstFrame/String",
	"Channel(s)":                         "Channel(s)/String",
	"Channel layout":                     "ChannelLayout",
	"Sampling rate":                      "SamplingRate/String",
	"Count of elements":                  "ElementCount",
	"Language":                           "Language/String",
	"Language, more info":                "Language_More",
	"Service kind":                       "ServiceKind/String",
	"Writing application":                "Encoded_Application/String",
	"Writing library":                    "Encoded_Library/String",
	flacUnencodedMD5Label:                "MD5_Unencoded",
	"Encoding settings":                  "Encoded_Library_Settings",
	"Encoded date":                       "Encoded_Date",
	"Tagged date":                        "Tagged_Date",
	"Default":                            "Default/String",
	"Forced":                             "Forced/String",
	"Alternate group":                    "AlternateGroup/String",
	"Color range":                        "colour_range",
	"Color primaries":                    "colour_primaries",
	"Transfer characteristics":           "transfer_characteristics",
	"Matrix coefficients":                "matrix_coefficients",
	"Mastering display color primaries":  "MasteringDisplay_ColorPrimaries",
	"Mastering display luminance":        "MasteringDisplay_Luminance",
	"Maximum Content Light Level":        "MaxCLL/String",
	"Maximum Frame-Average Light Level":  "MaxFALL/String",
	"Original source medium":             "OriginalSourceMedium",
	"Complexity index":                   "ComplexityIndex",
	"Number of dynamic objects":          "NumberOfDynamicObjects",
	"Bed channel count":                  "BedChannelCount",
	"Bed channel configuration":          "BedChannelConfiguration",
	"Dialog Normalization":               "dialnorm",
	"Law rating":                         "LawRating",
	"Recorded location":                  "Recorded/Location",
	"Recorded_Location":                  "Recorded/Location",
	"Terms of use":                       "TermsOfUse",
	"List":                               "List/String",
	"Duration of the visible content":    "Duration_Start2End/String",
	"Start time":                         "Duration_Start/String",
	"End time":                           "Duration_End/String",
	"Count of frames before first event": "FirstDisplay_Delay_Frames",
	"Type of the first event":            "FirstDisplay_Type",
	"ACTOR_CHARACTER":                    "ACTOR/CHARACTER",
	"Encoded_Application_Url":            "Encoded_Application/Url",
	"Writing_Library":                    "Writing Library",
	"Format settings, picture structure": "Format_Settings_PictureStructure",
}
