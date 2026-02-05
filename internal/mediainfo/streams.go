package mediainfo

import "sort"

func sortStreams(streams []Stream) {
	sort.SliceStable(streams, func(i, j int) bool {
		ai := streamKindOrder[streams[i].Kind]
		aj := streamKindOrder[streams[j].Kind]
		if ai == aj {
			return i < j
		}
		return ai < aj
	})
}
