package mediainfo

// seedMatroskaLegacyTestStream imports an intentionally legacy-shaped fixture
// at the parser cutover seam so canonical-only post-processing can be tested.
func seedMatroskaLegacyTestStream(stream *Stream) {
	if stream == nil || len(stream.canonicalSeed) > 0 {
		return
	}
	report := Report{
		General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}, JSON: map[string]string{"Format": "Matroska"}},
		Streams: []Stream{*stream},
	}
	store := legacyReportToFieldStore(report)
	if len(store.streams) < 2 {
		return
	}
	stream.canonicalSeed = append([]fieldEntry(nil), store.streams[1].Fields...)
	for key, value := range stream.JSON {
		setCanonicalSeedLegacyValue(stream, fieldName(key), value, false)
	}
	for key, value := range stream.JSONRaw {
		setCanonicalSeedLegacyValue(stream, fieldName(key), value, true)
	}
	if seconds := stream.JSON["Duration"]; seconds != "" {
		if milliseconds, ok := decimalSecondsToMilliseconds(seconds); ok {
			replaceCanonicalSeedLegacyProjection(stream, "Duration", milliseconds, seconds, "", "")
			setCanonicalSeedStructuredDecimals(stream, "Duration", uint8(decimalFractionDigits(seconds)))
		}
	}
}
