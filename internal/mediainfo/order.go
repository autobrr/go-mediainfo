package mediainfo

var streamKindOrder = map[StreamKind]int{
	StreamGeneral: 0,
	StreamVideo:   1,
	StreamAudio:   2,
	StreamText:    3,
	StreamImage:   4,
	StreamMenu:    5,
}
