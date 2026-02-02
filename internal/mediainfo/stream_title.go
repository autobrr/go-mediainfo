package mediainfo

type titledStream struct {
	Title  string
	Stream Stream
}

func enumerateStreams(streams []Stream) []titledStream {
	ordered := orderTracks(streams)
	counts := map[StreamKind]int{}
	for _, stream := range ordered {
		counts[stream.Kind]++
	}
	index := map[StreamKind]int{}
	out := make([]titledStream, 0, len(ordered))
	for _, stream := range ordered {
		index[stream.Kind]++
		title := streamTitle(stream.Kind, index[stream.Kind], counts[stream.Kind])
		out = append(out, titledStream{Title: title, Stream: stream})
	}
	return out
}
