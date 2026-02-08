package mediainfo

func channelLayout(channels uint64) string {
	switch channels {
	case 1:
		return "M"
	case 2:
		return "L R"
	case 3:
		return "C L R"
	case 4:
		return "L R Ls Rs"
	case 5:
		return "C L R Ls Rs"
	case 6:
		// MediaInfo's canonical 5.1 ordering (LFE last).
		return "C L R Ls Rs LFE"
	case 7:
		return "C L R Ls Rs Lb Rb"
	case 8:
		return "C L R Ls Rs Lb Rb LFE"
	default:
		return ""
	}
}
