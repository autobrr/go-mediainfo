package mediainfo

func fallbackMatroskaTrackType(trackType uint64) (StreamKind, string) {
	switch trackType {
	case 1:
		return StreamVideo, "Video"
	case 2:
		return StreamAudio, "Audio"
	case 17:
		return StreamText, "Text"
	default:
		return "", ""
	}
}
