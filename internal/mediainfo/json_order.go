package mediainfo

func orderedJSONTrack(stream Stream) map[string]string {
	entry := map[string]string{"@type": string(stream.Kind)}
	for _, field := range stream.Fields {
		entry[field.Name] = field.Value
	}
	return entry
}

func orderTracks(tracks []Stream) []Stream {
	sorted := append([]Stream(nil), tracks...)
	sortStreams(sorted)
	return sorted
}
