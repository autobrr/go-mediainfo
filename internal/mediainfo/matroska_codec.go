package mediainfo

func mapMatroskaCodecID(codecID string, trackType uint64) (StreamKind, string) {
	switch codecID {
	case "V_MPEG4/ISO/AVC":
		return StreamVideo, "AVC"
	case "V_MPEGH/ISO/HEVC":
		return StreamVideo, "HEVC"
	case "V_AV1":
		return StreamVideo, "AV1"
	case "V_VP9":
		return StreamVideo, "VP9"
	case "V_VP8":
		return StreamVideo, "VP8"
	case "V_MPEG2":
		return StreamVideo, "MPEG Video"
	case "V_MS/VFW/FOURCC":
		return StreamVideo, "Video"
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
	case "A_MPEG/L2", "A_MPEG/L3":
		return StreamAudio, "MPEG Audio"
	case "A_VORBIS":
		return StreamAudio, "Vorbis"
	case "A_DTS":
		return StreamAudio, "DTS"
	case "A_TRUEHD":
		return StreamAudio, "TrueHD"
	case "A_PCM/INT/LIT":
		return StreamAudio, "PCM"
	case "A_PCM/INT/BIG":
		return StreamAudio, "PCM"
	case "A_PCM/FLOAT/IEEE":
		return StreamAudio, "PCM"
	case "A_MS/ACM":
		return StreamAudio, "Audio"
	case "S_TEXT/UTF8":
		return StreamText, "UTF-8"
	case "S_TEXT/ASS":
		return StreamText, "ASS"
	case "S_TEXT/SSA":
		return StreamText, "SSA"
	case "S_TEXT/WEBVTT":
		return StreamText, "S_TEXT/WEBVTT"
	case "D_WEBVTT/SUBTITLES":
		return StreamText, "D_WEBVTT/SUBTITLES"
	case "S_VOBSUB":
		return StreamText, "VobSub"
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
	case "VC-1":
		return "SMPTE VC-1"
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
	case "DTS":
		return "Digital Theater Systems"
	case "DTS XLL":
		return "Digital Theater Systems"
	case "DTS XBR":
		return "Digital Theater Systems"
	case "DTS ES":
		return "Digital Theater Systems"
	case "TrueHD":
		return "Dolby TrueHD"
	case "PCM":
		return "PCM"
	case "Vorbis":
		return "Vorbis"
	case "MPEG Video":
		return "MPEG Video"
	default:
		return ""
	}
}
