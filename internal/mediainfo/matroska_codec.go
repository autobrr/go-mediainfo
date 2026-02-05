package mediainfo

func mapMatroskaCodecID(codecID string, trackType uint64) (StreamKind, string) {
	switch codecID {
	case "V_MPEG4/ISO/AVC":
		return StreamVideo, "AVC"
	case "V_MPEGH/ISO/HEVC":
		return StreamVideo, "HEVC"
	case "V_VP9":
		return StreamVideo, "VP9"
	case "V_VP8":
		return StreamVideo, "VP8"
	case "A_AAC":
		return StreamAudio, "AAC"
	case "A_AAC-2":
		return StreamAudio, "AAC"
	case "A_AC3":
		return StreamAudio, "AC-3"
	case "A_EAC3":
		return StreamAudio, "E-AC-3"
	case "A_OPUS":
		return StreamAudio, "Opus"
	case "A_FLAC":
		return StreamAudio, "FLAC"
	case "A_MPEG/L2":
		return StreamAudio, "MPEG Audio"
	case "S_TEXT/UTF8":
		return StreamText, "UTF-8"
	case "S_TEXT/ASS":
		return StreamText, "ASS"
	case "S_HDMV/PGS":
		return StreamText, "PGS"
	default:
		return fallbackMatroskaTrackType(trackType)
	}
}

func mapMatroskaFormatInfo(format string) string {
	switch format {
	case "AVC":
		return "Advanced Video Codec"
	case "HEVC":
		return "High Efficiency Video Coding"
	case "VP9":
		return "Google VP9"
	case "VP8":
		return "Google VP8"
	case "AAC":
		return "Advanced Audio Codec"
	case "AC-3":
		return "Audio Coding 3"
	case "E-AC-3":
		return "Enhanced AC-3"
	case "E-AC-3 JOC":
		return "Enhanced AC-3 with Joint Object Coding"
	case "Opus":
		return "Opus"
	case "FLAC":
		return "Free Lossless Audio Codec"
	case "MPEG Audio":
		return "MPEG Audio"
	default:
		return ""
	}
}
