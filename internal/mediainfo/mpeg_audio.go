package mediainfo

type mpegAudioHeader struct {
	versionID   byte
	layerID     byte
	bitrateKbps int
	sampleRate  int
	channels    int
	channelMode byte
	padding     bool
}

func parseMPEGAudioHeader(hdr []byte) (mpegAudioHeader, bool) {
	if len(hdr) < 4 {
		return mpegAudioHeader{}, false
	}
	if hdr[0] != 0xFF || (hdr[1]&0xE0) != 0xE0 {
		return mpegAudioHeader{}, false
	}
	versionID := (hdr[1] >> 3) & 0x03
	layerID := (hdr[1] >> 1) & 0x03
	if versionID == 0x01 || layerID == 0x00 {
		return mpegAudioHeader{}, false
	}
	bitrateIndex := (hdr[2] >> 4) & 0x0F
	sampleRateIndex := (hdr[2] >> 2) & 0x03
	if bitrateIndex == 0x00 || bitrateIndex == 0x0F || sampleRateIndex == 0x03 {
		return mpegAudioHeader{}, false
	}
	bitrate := mpegAudioBitrateKbps(versionID, layerID, bitrateIndex)
	sampleRate := mp3SampleRate(versionID, sampleRateIndex)
	if bitrate == 0 || sampleRate == 0 {
		return mpegAudioHeader{}, false
	}
	channelMode := (hdr[3] >> 6) & 0x03
	channels := 2
	if channelMode == 0x03 {
		channels = 1
	}
	padding := ((hdr[2] >> 1) & 0x01) != 0
	return mpegAudioHeader{
		versionID:   versionID,
		layerID:     layerID,
		bitrateKbps: bitrate,
		sampleRate:  sampleRate,
		channels:    channels,
		channelMode: channelMode,
		padding:     padding,
	}, true
}

func mpegAudioBitrateKbps(versionID, layerID, index byte) int {
	// Tables per MPEG version/layer. Only common Layer I/II/III handled.
	var rates []int
	switch layerID {
	case 0x03: // Layer I
		switch versionID {
		case 0x03:
			rates = []int{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448}
		case 0x02, 0x00:
			rates = []int{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256}
		}
	case 0x02: // Layer II
		switch versionID {
		case 0x03:
			rates = []int{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384}
		case 0x02, 0x00:
			rates = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
		}
	case 0x01: // Layer III
		switch versionID {
		case 0x03:
			rates = []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
		case 0x02, 0x00:
			rates = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
		}
	}
	if len(rates) == 0 {
		return 0
	}
	idx := int(index)
	if idx < 0 || idx >= len(rates) {
		return 0
	}
	return rates[idx]
}

func mpegAudioSamplesPerFrame(versionID, layerID byte) int {
	switch layerID {
	case 0x03: // Layer I
		return 384
	case 0x02: // Layer II
		return 1152
	case 0x01: // Layer III
		if versionID == 0x03 {
			return 1152
		}
		return 576
	default:
		return 0
	}
}

func mpegAudioFrameLengthBytes(h mpegAudioHeader) int {
	if h.bitrateKbps <= 0 || h.sampleRate <= 0 {
		return 0
	}
	pad := 0
	if h.padding {
		pad = 1
	}
	switch h.layerID {
	case 0x03: // Layer I
		// (12 * bitrate / sampleRate + pad) * 4
		return ((12000*h.bitrateKbps)/h.sampleRate + pad) * 4
	case 0x02: // Layer II
		coef := 144000
		if h.versionID != 0x03 {
			coef = 72000
		}
		return (coef*h.bitrateKbps)/h.sampleRate + pad
	case 0x01: // Layer III
		coef := 144000
		if h.versionID != 0x03 {
			coef = 72000
		}
		return (coef*h.bitrateKbps)/h.sampleRate + pad
	default:
		return 0
	}
}
