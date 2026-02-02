package mediainfo

type psStream struct {
	id     byte
	kind   StreamKind
	format string
	bytes  uint64
	frames uint64
	pts    ptsTracker
}
