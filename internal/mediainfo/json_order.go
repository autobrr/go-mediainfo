package mediainfo

func orderTracks(tracks []Stream) []Stream {
	sorted := append([]Stream(nil), tracks...)
	sortStreams(sorted)
	return sorted
}
