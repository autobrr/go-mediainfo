package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// matroskaDTSCanonicalFacts contains the TrackEntry facts needed to build one
// DTS stream before a bounded core or extension-header probe refines it.
type matroskaDTSCanonicalFacts struct {
	codecID         string
	codecName       string
	trackName       string
	languageCode    string
	displayLanguage string
	trackNumber     uint64
	trackUID        uint64
	contentCompAlgo uint64
	audioChannels   uint64
	audioSampleRate float64
	bitRate         uint64
	segmentDuration float64
	durationPrec    int
	defaultValue    bool
	forcedValue     bool
	serviceKinds    []string
}

// matroskaDTSCanonicalSeed builds one DTS stream directly from TrackEntry
// facts while leaving core and DTS-HD metadata to the bounded probe.
func matroskaDTSCanonicalSeed(facts matroskaDTSCanonicalFacts) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "DTS", "Format", "DTS")
	if facts.trackNumber > 0 {
		value := strconv.FormatUint(facts.trackNumber, 10)
		builder.Fill("ID", value, "ID", value)
	}
	if facts.contentCompAlgo == 3 {
		builder.Fill("MuxingMode", "Header stripping", "Muxing mode", "Header stripping")
	}
	if facts.codecID != "" {
		builder.Fill("CodecID", facts.codecID, "Codec ID", facts.codecID)
	}
	builder.Text("Format/Info", "Digital Theater Systems")
	if facts.trackUID > 0 {
		builder.Structured("UniqueID", strconv.FormatUint(facts.trackUID, 10))
	}
	if facts.segmentDuration > 0 {
		seconds := facts.segmentDuration
		decimals := uint8(9)
		if facts.durationPrec <= 3 {
			seconds = math.Round(seconds*1000) / 1000
			decimals = 3
		}
		secondsText := fmt.Sprintf("%.*f", decimals, seconds)
		if milliseconds, ok := decimalSecondsToMilliseconds(secondsText); ok {
			builder.Fill("Duration", milliseconds, "Duration", formatDuration(facts.segmentDuration))
			builder.SetStructuredDecimals("Duration", decimals)
		}
	}
	if facts.bitRate > 0 {
		builder.Fill("BitRate_Mode", "Constant", "Bit rate mode", "Constant")
		raw := strconv.FormatUint(facts.bitRate, 10)
		builder.Fill("BitRate", raw, "Bit rate", formatBitrate(float64(facts.bitRate)))
	}
	if facts.audioChannels > 0 {
		value := strconv.FormatUint(facts.audioChannels, 10)
		builder.Fill("Channels", value, "Channel(s)", formatChannels(facts.audioChannels))
		if layout := channelLayout(facts.audioChannels); layout != "" {
			builder.Fill("ChannelLayout", layout, "Channel layout", layout)
		}
		builder.Structured("ChannelPositions", matroskaGoChannelPositions(facts.audioChannels))
	}
	if facts.audioSampleRate > 0 {
		value := strconv.FormatFloat(facts.audioSampleRate, 'f', -1, 64)
		builder.Fill("SamplingRate", value, "Sampling rate", formatSampleRate(facts.audioSampleRate))
	}
	builder.Structured("Delay", "0.000")
	builder.Structured("Delay_Source", "Container")
	builder.Structured("Video_Delay", "0.000")
	if facts.codecName != "" && strings.Contains(facts.codecName, "Lavc") {
		builder.Fill("Encoded_Library", canonicalEncodedLibrary(facts.codecName), "Writing library", facts.codecName)
	}
	if facts.trackName != "" {
		builder.Fill("Title", facts.trackName, "Title", facts.trackName)
	}
	if facts.languageCode != "" {
		builder.Fill("Language", facts.languageCode, "Language", formatLanguage(facts.displayLanguage))
	}
	if len(facts.serviceKinds) > 0 {
		builder.Structured("ServiceKind", strings.Join(facts.serviceKinds, " / "))
	}
	defaultText := "No"
	if facts.defaultValue {
		defaultText = "Yes"
	}
	builder.Fill("Default", defaultText, "Default", defaultText)
	forcedText := "No"
	if facts.forcedValue {
		forcedText = "Yes"
	}
	builder.Fill("Forced", forcedText, "Forced", forcedText)
	return builder.Snapshot(canonicalStreamPolicy{}).canonicalSeed
}

// applyMatroskaDTSCanonicalProbe applies typed DTS core and extension facts
// directly to the canonical seed and retains only exported map metadata.
func applyMatroskaDTSCanonicalProbe(stream *Stream, dts dtsInfo, preserveContainerBitRate bool) {
	if stream == nil || len(stream.canonicalSeed) == 0 {
		return
	}
	if dts.lbr {
		replaceCanonicalSeedLegacyFill(stream, "Format", "DTS LBR", "Format", "DTS LBR")
		replaceCanonicalSeedLegacyFill(stream, "Format_Commercial_IfAny", "DTS Express", "Commercial name", "DTS Express")
	}
	if dts.hd {
		switch {
		case dts.hdXLL:
			format := "DTS XLL"
			commercial := "DTS-HD Master Audio"
			featureParts := make([]string, 0, 3)
			if dts.coreES {
				featureParts = append(featureParts, "ES")
			}
			if dts.coreXCh {
				featureParts = append(featureParts, "XCh")
			}
			featureParts = append(featureParts, "XLL")
			features := strings.Join(featureParts, " ")
			if dts.hdIMAX {
				format = "DTS XLL X IMAX"
				commercial = "DTS-HD MA + IMAX Enhanced"
				features = "XLL X IMAX"
			} else if dts.hdDTSX {
				format = "DTS XLL X"
				commercial = "DTS-HD MA + DTS:X"
				features = "XLL X"
			}
			replaceCanonicalSeedLegacyFill(stream, "Format", "DTS", "Format", format)
			replaceCanonicalSeedLegacyFill(stream, "Format_AdditionalFeatures", features, "", "")
			replaceCanonicalSeedLegacyFill(stream, "Format_Commercial_IfAny", commercial, "Commercial name", commercial)
		case dts.hdXBR:
			replaceCanonicalSeedFill(stream, "Format", "DTS", "Format", "DTS XBR")
			replaceCanonicalSeedLegacyFill(stream, "Format_AdditionalFeatures", "XBR", "", "")
			replaceCanonicalSeedLegacyFill(stream, "Format_Commercial_IfAny", "DTS-HD High Resolution Audio", "Commercial name", "DTS-HD High Resolution Audio")
		default:
			replaceCanonicalSeedLegacyFill(stream, "Format_Commercial_IfAny", "DTS-HD", "Commercial name", "DTS-HD")
		}
	}
	if !dts.hd && dts.coreES {
		containerLayout, _ := canonicalSeedValue(*stream, "ChannelLayout")
		containerPositions, _ := canonicalSeedValue(*stream, "ChannelPositions")
		commercial := "DTS-ES"
		features := "ES"
		if dts.coreXCh {
			commercial = "DTS-ES Discrete"
			features = "ES XCh"
		}
		replaceCanonicalSeedLegacyFill(stream, "Format_AdditionalFeatures", features, "", "")
		replaceCanonicalSeedLegacyFill(stream, "Format_Commercial_IfAny", commercial, "Commercial name", commercial)
		replaceCanonicalSeedLegacyFill(stream, "Channels_Original", strconv.Itoa(dts.channels+1), "", "")
		replaceCanonicalSeedLegacyFill(stream, "ChannelLayout_Original", "C L R Ls Rs Cb LFE", "", "")
		replaceCanonicalSeedLegacyFill(stream, "ChannelPositions_Original", "Front: L C R, Side: L R, Back: C, LFE", "", "")
		clearCanonicalSeedField(stream, "ChannelLayout", "Channel layout")
		clearCanonicalSeedField(stream, "ChannelPositions", "")
		replaceCanonicalSeedJSONOnly(stream, "ChannelLayout", containerLayout)
		replaceCanonicalSeedJSONOnly(stream, "ChannelPositions", containerPositions)
	}

	bitDepth := dts.bitDepth
	if dts.hd && dts.hdBitDepth > 0 {
		bitDepth = dts.hdBitDepth
	}
	channels := dts.channels
	if dts.hd && dts.hdChannels > 0 {
		channels = dts.hdChannels
	}
	if channels > 0 {
		channelsRaw := strconv.Itoa(channels)
		replaceCanonicalSeedLegacyFill(stream, "Channels", channelsRaw, "Channel(s)", formatChannels(uint64(channels)))
		layout := ""
		positions := ""
		textLayout := ""
		switch {
		case dts.hd && dts.hasSpeakerMask:
			layout = normalizeDTSHDChannelLayout(dtsHDSpeakerActivityMaskChannelLayout(dts.hdSpeakerMask))
			if dts.hdDTSX && layout != "" {
				layout = dtsXChannelLayout(layout)
			}
			textLayout = layout
			if dts.coreXCh || dts.coreES {
				layout = strings.ReplaceAll(layout, "Cs", "Cb")
				textLayout = layout
			} else if channels == 4 {
				layout = strings.ReplaceAll(layout, "Cs", "Cb")
			}
			positions = dtsHDSpeakerActivityMask(dts.hdSpeakerMask)
			if dts.hdDTSX && positions != "" {
				positions += ", Objects"
			}
		case dts.lbr:
			layout = dts.lbrLayout
			textLayout = layout
			positions = dts.lbrPositions
		case !dts.coreES:
			textLayout = channelLayout(uint64(channels))
			layout = textLayout
			positions = channelPositionsFromCount(channelsRaw)
			switch dts.coreAudioMode {
			case 4:
				layout = "Lt Rt"
				positions = "Front: L R"
			case 7:
				layout = "C L R Cb"
				positions = "Front: L C R, Back: C"
			}
		}
		if layout != "" {
			if textLayout != "" {
				replaceCanonicalSeedLegacyFill(stream, "ChannelLayout", layout, "Channel layout", textLayout)
			} else {
				replaceCanonicalSeedLegacyFill(stream, "ChannelLayout", layout, "", "")
			}
		}
		if positions != "" {
			replaceCanonicalSeedLegacyFill(stream, "ChannelPositions", positions, "", "")
		}
	}
	if bitDepth > 0 {
		value := strconv.Itoa(bitDepth)
		replaceCanonicalSeedLegacyFill(stream, "BitDepth", value, "Bit depth", value+" bits")
	}
	sampleRate := dts.sampleRate
	if dts.hd && dts.hdSampleRate > 0 {
		sampleRate = dts.hdSampleRate
	}
	if sampleRate > 0 {
		value := strconv.Itoa(sampleRate)
		replaceCanonicalSeedFill(stream, "SamplingRate", value, "Sampling rate", formatSampleRate(float64(sampleRate)))
	}
	if sampleRate > 0 && dts.samplesPerFrame > 0 {
		frameRate := float64(sampleRate) / float64(dts.samplesPerFrame)
		replaceCanonicalSeedLegacyFill(stream, "FrameRate", fmt.Sprintf("%.3f", frameRate), "Frame rate", formatAudioFrameRate(frameRate, dts.samplesPerFrame))
		replaceCanonicalSeedLegacyFill(stream, "SamplesPerFrame", strconv.Itoa(dts.samplesPerFrame), "", "")
	}

	_, hasStreamSize := canonicalSeedValue(*stream, "StreamSize")
	_, hasBitRate := canonicalSeedValue(*stream, "BitRate")
	_, hadBitRateMode := canonicalSeedValue(*stream, "BitRate_Mode")
	switch {
	case dts.hd:
		replaceCanonicalSeedLegacyProjection(stream, "BitRate_Mode", "Variable", "VBR", "Bit rate mode", "Variable")
		if hasBitRate && !dts.hdDTSX && !hasStreamSize {
			clearCanonicalSeedField(stream, "BitRate", "Bit rate")
		}
	case dts.bitRateBps > 0 && !preserveContainerBitRate:
		replaceCanonicalSeedLegacyProjection(stream, "BitRate_Mode", "Constant", "CBR", "Bit rate mode", "Constant")
		replaceCanonicalSeedLegacyFill(stream, "BitRate", strconv.FormatInt(dts.bitRateBps, 10), "Bit rate", formatBitrate(float64(dts.bitRateBps)))
	case hasBitRate:
		if _, hasMode := canonicalSeedValue(*stream, "BitRate_Mode"); !hasMode {
			replaceCanonicalSeedLegacyProjection(stream, "BitRate_Mode", "Constant", "CBR", "Bit rate mode", "Constant")
		}
	}
	if dts.lbr && !hadBitRateMode {
		clearCanonicalSeedText(stream, "Bit rate mode")
	}
	compression := "Lossy"
	if dts.hd && dts.hdXLL {
		compression = "Lossless"
	}
	replaceCanonicalSeedLegacyFill(stream, "Compression_Mode", compression, "Compression mode", compression)
	replaceCanonicalSeedLegacyFill(stream, "Format_Settings_Endianness", "Big", "", "")
	replaceCanonicalSeedLegacyFill(stream, "Format_Settings_Mode", "16", "", "")
	if sampleRate > 0 {
		if durationMilliseconds, ok := canonicalSeedValue(*stream, "Duration"); ok {
			if milliseconds, err := strconv.ParseFloat(durationMilliseconds, 64); err == nil && milliseconds > 0 {
				samplingCount := int64(math.Round(milliseconds * float64(sampleRate) / 1000))
				replaceCanonicalSeedLegacyFill(stream, "SamplingCount", strconv.FormatInt(samplingCount, 10), "", "")
			}
		}
	}
}
