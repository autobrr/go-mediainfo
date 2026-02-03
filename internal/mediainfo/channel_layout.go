package mediainfo

func channelLayout(channels uint64) string {
	switch channels {
	case 1:
		return "M"
	case 2:
		return "L R"
	case 3:
		return "L R C"
	case 4:
		return "L R Ls Rs"
	case 5:
		return "L R C Ls Rs"
	case 6:
		return "L R C LFE Ls Rs"
	default:
		return ""
	}
}
