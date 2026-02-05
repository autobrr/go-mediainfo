package mediainfo

func forEachStreamWithKindIndex(streams []Stream, fn func(stream Stream, index, total, order int)) {
	counts := countStreams(streams)
	kindIndex := map[StreamKind]int{}
	for order, stream := range streams {
		kindIndex[stream.Kind]++
		fn(stream, kindIndex[stream.Kind], counts[stream.Kind], order)
	}
}
