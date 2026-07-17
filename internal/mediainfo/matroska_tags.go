package mediainfo

import (
	"sort"
	"strings"
)

// matroskaTagField preserves a Matroska tag's source name and normalized JSON
// projection without constraining unknown tag names to a fixed allowlist.
type matroskaTagField struct {
	rawName string
	name    string
	value   string
	order   int
}

// matroskaTagSet provides deterministic last-value-wins storage for one tag
// target while retaining encounter order for collision handling.
type matroskaTagSet struct {
	values map[string]matroskaTagField
	next   int
}

// matroskaScopedTags separates file-level tags from TrackUID-targeted tags.
type matroskaScopedTags struct {
	general matroskaTagSet
	tracks  map[uint64]*matroskaTagSet
}

// set stores a non-empty normalized field with last-value-wins semantics and a
// monotonic encounter position.
func (s *matroskaTagSet) set(field matroskaTagField) {
	if field.name == "" || field.value == "" {
		return
	}
	if s.values == nil {
		s.values = map[string]matroskaTagField{}
	}
	field.order = s.next
	s.next++
	s.values[field.name] = field
}

func (s matroskaTagSet) get(name string) string {
	return s.values[name].value
}

// sorted returns a deterministic copy ordered by normalized field name.
func (s matroskaTagSet) sorted() []matroskaTagField {
	fields := make([]matroskaTagField, 0, len(s.values))
	for _, field := range s.values {
		fields = append(fields, field)
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].name < fields[j].name })
	return fields
}

// set routes normalized fields to General or TrackUID storage. Tags without a
// Targets element and block-addition targets are intentionally not projected.
func (s *matroskaScopedTags) set(target matroskaTagTarget, fields []matroskaTagField) {
	if !target.present || target.blockAddID != 0 {
		return
	}
	if target.trackUID == 0 {
		for _, field := range fields {
			s.general.set(field)
		}
		return
	}
	if s.tracks == nil {
		s.tracks = map[uint64]*matroskaTagSet{}
	}
	set := s.tracks[target.trackUID]
	if set == nil {
		set = &matroskaTagSet{}
		s.tracks[target.trackUID] = set
	}
	for _, field := range fields {
		set.set(field)
	}
}

// mergeMatroskaScopedTags fills missing General and per-track fields without
// replacing values from an earlier, more authoritative parse window.
func mergeMatroskaScopedTags(dst *matroskaScopedTags, src matroskaScopedTags) {
	if dst == nil {
		return
	}
	mergeMatroskaTagSet(&dst.general, src.general)
	if len(src.tracks) == 0 {
		return
	}
	if dst.tracks == nil {
		dst.tracks = map[uint64]*matroskaTagSet{}
	}
	for uid, srcSet := range src.tracks {
		dstSet := dst.tracks[uid]
		if dstSet == nil {
			dstSet = &matroskaTagSet{}
			dst.tracks[uid] = dstSet
		}
		mergeMatroskaTagSet(dstSet, *srcSet)
	}
}

// mergeMatroskaTagSet copies only normalized names absent from dst.
func mergeMatroskaTagSet(dst *matroskaTagSet, src matroskaTagSet) {
	fields := make([]matroskaTagField, 0, len(src.values))
	for _, field := range src.values {
		fields = append(fields, field)
	}
	sort.SliceStable(fields, func(i, j int) bool {
		return fields[i].order < fields[j].order
	})
	for _, field := range fields {
		if _, exists := dst.values[field.name]; !exists {
			dst.set(field)
		}
	}
}

// normalizeMatroskaTag mirrors MediaInfoLib's Matroska tag aliases. It is a
// normalization policy, not an allowlist: unmatched names pass through.
func normalizeMatroskaTag(path []string, value string) []matroskaTagField {
	value = strings.TrimSpace(value)
	if len(path) == 0 || value == "" || value == "N/A" {
		return nil
	}
	rawName := strings.Join(path, "/")
	names := append([]string(nil), path...)
	value = sanitizeMatroskaTagValue(value)
	switch names[0] {
	case "AERMS_OF_USE":
		names[0] = "TermsOfUse"
	case "ARTIST":
		names[0] = "Performer"
	case "BITSPS", "COMPATIBLE_BRANDS", "FPS", "MAJOR_BRAND", "MINOR_VERSION", "STEREO_MODE":
		names[0] = ""
	case "CONTENT_TYPE":
		names[0] = "ContentType"
	case "COPYRIGHT":
		names[0] = "Copyright"
	case "CREATION_TIME":
		names[0] = "Encoded_Date"
		value += " UTC"
	case "DATE":
		names[0] = "Recorded_Date"
		value = strings.Replace(value, "T", " ", 1)
	case "DATE_DIGITIZED":
		names[0] = "Mastered_Date"
		value += " UTC"
	case "DATE_ENCODED":
		names[0] = "Encoded_Date"
	case "DATE_RECORDED":
		names[0] = "Recorded_Date"
	case "DATE_RELEASE", "DATE_RELEASED":
		names[0] = "Released_Date"
	case "DATE_TAGGED":
		names[0] = "Tagged_Date"
	case "DESCRIPTION":
		names[0] = "Description"
	case "ENCODED_BY":
		names[0] = "EncodedBy"
	case "ENCODER":
		names[0] = "Encoded_Library"
	case "ENCODER_SETTINGS", "ENCODER_OPTIONS":
		names[0] = "Encoded_Library_Settings"
	case "GENRE":
		names[0] = "Genre"
	case "HANDLER_NAME":
		if matroskaTagLooksTechnical(value) {
			names[0] = ""
		} else {
			names[0] = "Title"
		}
	case "LANGUAGE":
		names[0] = "Language"
	case "PART_NUMBER":
		names[0] = "Track/Position"
	case "PURL":
		names[0] = "Podcast_Url"
	case "ORIGINAL_MEDIA_TYPE", "ORIGINAL_SOURCE_FORM":
		names[0] = "OriginalSourceForm"
	case "SAMPLE":
		if len(names) == 2 && names[1] == "PART_NUMBER" {
			return nil
		}
		if len(names) == 2 && names[1] == "TITLE" {
			names = []string{"Title_More"}
		}
	case "SYNOPSIS":
		names[0] = "Synopsis"
	case "TERMS_OF_USE":
		names[0] = "TermsOfUse"
	case "TIMECODE":
		if matroskaTagLooksTechnical(value) {
			names[0] = ""
		} else {
			names[0] = "TimeCode_FirstFrame"
		}
	case "TITLE":
		names[0] = "Title"
	case "TOOL", "Tool":
		// MediaInfo's General field registry consumes Tool without exposing it
		// in Inform output; treat that observed schema name as non-dynamic.
		names[0] = ""
	case "TOTAL_PARTS":
		names[0] = "Track/Position_Total"
	}
	for i := range names {
		switch names[i] {
		case "BARCODE":
			names[i] = "BarCode"
		case "COMMENT":
			names[i] = "Comment"
		case "ORIGINAL":
			names[i] = "Original"
		case "URL":
			names[i] = "Url"
		}
	}
	name := strings.Join(names, "/")
	if name == "" {
		return nil
	}
	fields := []matroskaTagField{{rawName: rawName, name: name, value: value}}
	if name == "TimeCode_FirstFrame" {
		fields = append(fields, matroskaTagField{rawName: rawName, name: "TimeCode_Source", value: "Matroska tags"})
	}
	return fields
}

// matroskaTagLooksTechnical recognizes handler-style values that MediaInfo does
// not expose as descriptive titles or timecodes.
func matroskaTagLooksTechnical(value string) bool {
	return strings.Contains(value, "Handler") || strings.Contains(value, "handler") || strings.Contains(value, "vide") || strings.Contains(value, "soun")
}

// sanitizeMatroskaTagValue replaces disallowed C0 and C1 controls while
// retaining tab and line-ending characters accepted in metadata text.
func sanitizeMatroskaTagValue(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r >= 0x7F && r < 0xA0 {
			return '\uFFFD'
		}
		return r
	}, value)
}

// mediaInfoJSONName converts a normalized MediaInfo field name into its JSON
// key spelling and prefixes keys that would otherwise begin with a digit.
func mediaInfoJSONName(name string) string {
	var builder strings.Builder
	for _, r := range name {
		switch r {
		case ' ':
			r = '_'
		case '-', '(', ')':
			continue
		case '/', '*', ',', ':', '@', '.':
			r = '_'
		}
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			builder.WriteRune(r)
		}
	}
	out := builder.String()
	if out == "" {
		return "Unknown"
	}
	if out[0] >= '0' && out[0] <= '9' {
		return "_" + out
	}
	return out
}

// matroskaTagFieldsForJSON splits schema-backed fields from ordered canonical
// extra members while preserving arbitrary non-internal Matroska tags.
func matroskaTagFieldsForJSON(set matroskaTagSet, kind StreamKind, skip map[string]struct{}) (map[string]string, []structuredMember) {
	known := map[string]string{}
	dynamic := make([]structuredMember, 0, len(set.values))
	for _, field := range set.sorted() {
		if _, skipped := skip[field.name]; skipped || matroskaTagIsInternal(field.rawName, kind) {
			continue
		}
		fieldName := field.name
		if kind == StreamGeneral {
			switch fieldName {
			case "INTERNAL":
				fieldName = "Internal"
			case "SOURCE":
				fieldName = "Source"
			case "WRITING_LIBRARY":
				fieldName = "Writing_Library"
			}
		}
		name := mediaInfoJSONName(fieldName)
		if isKnownStructuredField(kind, name) || kind == StreamGeneral && isMatroskaGeneralTagJSONField(name) {
			known[name] = field.value
			continue
		}
		dynamic = append(dynamic, structuredMember{Key: name, Value: structuredNode{Kind: structuredString, Text: field.value}})
	}
	return known, dynamic
}

// isMatroskaGeneralTagJSONField reports whether a normalized Matroska tag maps
// to a schema-backed General JSON field.
func isMatroskaGeneralTagJSONField(name string) bool {
	switch name {
	case "Title", "Title_More", "Movie", "Movie_More", "Track_Position", "Track_Position_Total", "Performer", "EncodedBy", "Genre", "ContentType", "Description", "Synopsis", "Released_Date", "Recorded_Date", "Encoded_Date", "Tagged_Date", "Mastered_Date", "Encoded_Library", "Encoded_Library_Settings", "OriginalSourceForm", "BarCode", "Copyright", "TermsOfUse", "Comment":
		return true
	}
	return false
}

// matroskaTagIsInternal reports whether a raw tag is consumed by parser logic
// and must not also be emitted as dynamic JSON metadata.
func matroskaTagIsInternal(rawName string, kind StreamKind) bool {
	name := rawName
	if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
		name = name[slash+1:]
	}
	switch name {
	case "BPS", "DURATION", "NUMBER_OF_BYTES", "NUMBER_OF_FRAMES", "_STATISTICS_TAGS", "_STATISTICS_WRITING_APP", "_STATISTICS_WRITING_DATE_UTC":
		return true
	}
	if kind != StreamGeneral {
		switch name {
		case "CREATION_TIME", "ENCODER", "ENCODER_OPTIONS", "ENCODER_SETTINGS", "SOURCE_ID":
			return true
		}
	}
	return false
}

// applyMatroskaTrackTags projects TrackUID-scoped tags without replacing fields
// already derived from the container or bitstream.
func applyMatroskaTrackTags(info *MatroskaInfo) {
	if info == nil || len(info.scopedTags.tracks) == 0 {
		return
	}
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		set := info.scopedTags.tracks[streamTrackUID(*stream)]
		if set == nil {
			continue
		}
		known, dynamic := matroskaTagFieldsForJSON(*set, stream.Kind, nil)
		for name, value := range known {
			if !streamHasCanonicalStructuredField(*stream, name) {
				replaceCanonicalSeedFill(stream, fieldName(name), value, "", "")
			}
		}
		mergeMatroskaDynamicCanonicalExtras(stream, dynamic)
	}
}

// streamHasCanonicalStructuredField reports whether a stream already owns a
// canonical scalar or compound node for the structured field name.
func streamHasCanonicalStructuredField(stream Stream, name string) bool {
	if _, found := canonicalSeedValue(stream, fieldName(name)); found {
		return true
	}
	return canonicalSeedStructuredNode(&stream, fieldName(name)) != nil
}

// applyMatroskaGeneralTags projects file-level tags into schema fields or
// dynamic extras while preserving higher-priority General metadata.
func applyMatroskaGeneralTags(general *Stream, set matroskaTagSet) {
	if general == nil || len(set.values) == 0 {
		return
	}
	known, dynamic := matroskaTagFieldsForJSON(set, StreamGeneral, nil)
	if tagTitle := known["Title"]; tagTitle != "" {
		canonicalTitle, _ := canonicalSeedValue(*general, "Title")
		title := mergeMatroskaTitle(canonicalTitle, tagTitle)
		replaceCanonicalSeedFill(general, "Title", title, "", "")
		replaceCanonicalSeedFill(general, "Movie", title, "", "")
		delete(known, "Title")
	}
	for name, value := range known {
		if !streamHasCanonicalStructuredField(*general, name) {
			replaceCanonicalSeedFill(general, fieldName(name), value, "", "")
		}
	}
	title, _ := canonicalSeedValue(*general, "Title")
	if movie, _ := canonicalSeedValue(*general, "Movie"); title != "" && movie == "" {
		replaceCanonicalSeedFill(general, "Movie", title, "", "")
	}
	titleMore, _ := canonicalSeedValue(*general, "Title_More")
	if movieMore, _ := canonicalSeedValue(*general, "Movie_More"); titleMore != "" && movieMore == "" {
		replaceCanonicalSeedFill(general, "Movie_More", titleMore, "", "")
	}
	mergeMatroskaDynamicCanonicalExtras(general, dynamic)
}

// mergeMatroskaTitle mirrors MediaInfo's repeated-value handling: retain the
// more specific prefix match, otherwise preserve both container and tag text.
func mergeMatroskaTitle(containerTitle, tagTitle string) string {
	switch {
	case containerTitle == "":
		return tagTitle
	case tagTitle == "":
		return containerTitle
	case strings.HasPrefix(tagTitle, containerTitle):
		return tagTitle
	case strings.HasPrefix(containerTitle, tagTitle):
		return containerTitle
	default:
		return containerTitle + " / " + tagTitle
	}
}
