package mediainfo

import "encoding/binary"

var pcmM2TSChannelAssignment = [16]uint8{
	0,
	1,
	0,
	2,
	3,
	3,
	4,
	4,
	5,
	6,
	7,
	8,
	0,
	0,
	0,
	0,
}

var pcmM2TSSamplingFrequency = [16]uint32{
	0,
	48000,
	0,
	0,
	96000,
	192000,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
	0,
}

var pcmM2TSBitsPerSample = [4]uint8{
	0,
	16,
	20,
	24,
}

func parsePCMM2TSHeader(payload []byte) (channels uint8, sampleRate uint32, bitDepth uint8, channelAssignment byte, ok bool) {
	// MediaInfoLib File_Pcm_M2ts:
	// - audio_data_payload_size (16)
	// - channel_assignment (4)
	// - sampling_frequency (4)
	// - bits_per_sample (2)
	// - start_flag (1)
	// - reserved (5)
	if len(payload) < 4 {
		return 0, 0, 0, 0, false
	}
	_ = binary.BigEndian.Uint16(payload[0:2]) // audio_data_payload_size
	flags := binary.BigEndian.Uint16(payload[2:4])
	channelAssignment = byte((flags >> 12) & 0x0F)
	samplingCode := byte((flags >> 8) & 0x0F)
	bpsCode := byte((flags >> 6) & 0x03)

	channels = pcmM2TSChannelAssignment[channelAssignment]
	sampleRate = pcmM2TSSamplingFrequency[samplingCode]
	bitDepth = pcmM2TSBitsPerSample[bpsCode]
	if channels == 0 || sampleRate == 0 || bitDepth == 0 {
		return 0, 0, 0, 0, false
	}
	return channels, sampleRate, bitDepth, channelAssignment, true
}

func pcmVOBChannelPositions(channelAssignment byte) string {
	switch channelAssignment {
	case 1:
		return "Front: C"
	case 3:
		return "Front: L R"
	case 4:
		return "Front: L C R"
	case 5:
		return "Front: L R, LFE"
	case 6:
		return "Front: L C R, LFE"
	case 7:
		return "Front: L R, Side: L R"
	case 8:
		return "Front: L C R, Side: L R"
	case 9:
		return "Front: L C R, Side: L R, LFE"
	case 10:
		return "Front: L C R, Side: L R, Back: L R"
	case 11:
		return "Front: L C R, Side: L R, Back: L R, LFE"
	default:
		return ""
	}
}

func pcmVOBChannelLayout(channelAssignment byte) string {
	switch channelAssignment {
	case 1:
		return "M"
	case 3:
		return "L R"
	case 4:
		return "L R C"
	case 5:
		return "L R LFE"
	case 6:
		return "L C R LFE"
	case 7:
		return "L R Ls Rs"
	case 8:
		return "L R C Ls Rs"
	case 9:
		return "L R C Ls Rs LFE"
	case 10:
		return "L R C Ls Rs Lrs Rrs"
	case 11:
		return "L R C Ls Rs Lrs Rrs LFE"
	default:
		return ""
	}
}
