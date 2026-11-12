package mediainfo

// finalizeGeneralTitle applies the native fill-if-empty rule after every reported
// stream and General tag is known. Single-stream parser snapshots must not call it.
func finalizeGeneralTitle(store *fieldStore) {
	var general streamRef = -1
	hasVideo := false
	for i, stream := range store.streams {
		if stream.Kind == StreamGeneral {
			general = streamRef(i)
		}
		hasVideo = hasVideo || stream.Kind == StreamVideo
	}
	if general < 0 {
		return
	}
	collection, _ := store.Get(general, "Collection")
	destination := fieldName("Track")
	if hasVideo && collection == "" {
		destination = "Movie"
	}
	for _, suffix := range []string{"", "_More", "/Url"} {
		titleKey := fieldName("Title" + suffix)
		title, _ := store.Get(general, titleKey)
		movie, _ := store.Get(general, fieldName("Movie"+suffix))
		track, _ := store.Get(general, fieldName("Track"+suffix))
		if title != "" && movie == "" && track == "" {
			store.Fill(general, destination+fieldName(suffix), title, fillFirstNonEmpty)
		}
		if backfill := firstNonEmpty(movie, track); title == "" && backfill != "" {
			store.Fill(general, titleKey, backfill, fillFirstNonEmpty)
		}
	}
	for i := range store.streams[general].Fields {
		entry := &store.streams[general].Fields[i]
		switch entry.Name {
		case "Title", "Movie", "Track", "Title_More", "Movie_More", "Track_More", "Title/Url", "Movie/Url", "Track/Url":
			entry.Options.ShowText = reportTextVisible(reportVersion(store), StreamGeneral, string(entry.Name))
			entry.TextLabel = reportFieldLabel(reportVersion(store), string(entry.Name), string(entry.Name))
		}
	}
}

// exposeGeneralMetadata adds the text sibling for already mapped metadata only.
// It preserves canonical keys and lets the selected table control presentation.
func exposeGeneralMetadata(stream *Stream, names ...fieldName) {
	for _, name := range names {
		if value, ok := canonicalSeedValue(*stream, name); ok && value != "" {
			identity := reportFieldIdentity(reportTables23, StreamGeneral, string(name), "")
			label := reportFieldLabel(reportTables23, identity, identity)
			replaceCanonicalSeedFill(stream, name, value, label, value)
		}
	}
}

// vorbisDiscMetadata preserves leading zeros and the existing total-key priority.
func vorbisDiscMetadata(tags map[string]string, facts *canonicalStructuredFacts) {
	facts.SetSame("Part_Position", tags["DISCNUMBER"])
	facts.SetSame("Part_Position_Total", firstNonEmpty(tags["TOTALDISCS"], tags["DISCTOTAL"]))
}
