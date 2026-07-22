package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// matroskaTrueHDCanonicalFacts contains the TrackEntry facts needed to build
// one TrueHD stream before a bounded major-sync probe refines it.
type matroskaTrueHDCanonicalFacts struct {
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
	audioBitDepth   uint64
	bitRate         uint64
	segmentDuration float64
	durationPrec    int
	defaultValue    bool
	forcedValue     bool
	serviceKinds    []string
}

// matroskaTrueHDCanonicalSeed builds one TrueHD audio stream directly from
// TrackEntry facts while leaving major-sync metadata to the bounded probe.
func matroskaTrueHDCanonicalSeed(facts matroskaTrueHDCanonicalFacts) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "TrueHD", "Format", "TrueHD")
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
	builder.Text("Format/Info", "Dolby TrueHD")
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
	if facts.audioBitDepth > 0 {
		raw := strconv.FormatUint(facts.audioBitDepth, 10)
		builder.Fill("BitDepth", raw, "Bit depth", raw+" bits")
	}
	builder.Fill("Compression_Mode", "Lossless", "Compression mode", "Lossless")
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

// applyMatroskaTrueHDCanonicalProbe applies major-sync and Atmos presentation
// facts directly to a TrueHD seed and retains only exported map metadata.
func applyMatroskaTrueHDCanonicalProbe(stream *Stream, info trueHDInfo) {
	if stream == nil || len(stream.canonicalSeed) == 0 {
		return
	}
	replaceCanonicalSeedFill(stream, "Format", "MLP FBA", "Format", "MLP FBA")
	replaceCanonicalSeedText(stream, "Format/Info", "Dolby TrueHD")
	replaceCanonicalSeedFill(stream, "Format_Commercial_IfAny", "Dolby TrueHD", "Commercial name", "Dolby TrueHD")

	channels, _ := canonicalSeedValue(*stream, "Channels")
	switch channels {
	case "8":
		applyMatroskaTrueHDCanonicalLayout(stream, "L R C LFE Ls Rs Lb Rb", "Front: L C R, Side: L R, Back: L R, LFE")
	case "6":
		applyMatroskaTrueHDCanonicalLayout(stream, "L R C LFE Ls Rs", "Front: L C R, Side: L R, LFE")
	}

	if atmos, ok := trueHDAtmosPresentationInfo(info); ok {
		replaceCanonicalSeedFill(stream, "Format", "MLP FBA", "Format", "MLP FBA 16-ch")
		replaceCanonicalSeedText(stream, "Format/Info", "Meridian Lossless Packing FBA with 16-channel presentation")
		replaceCanonicalSeedFill(stream, "Format_Commercial_IfAny", "Dolby TrueHD with Dolby Atmos", "Commercial name", "Dolby TrueHD with Dolby Atmos")
		replaceCanonicalSeedFill(stream, "Format_AdditionalFeatures", atmos.additionalFeatures, "", "")
		replaceCanonicalSeedText(stream, "Number of dynamic objects", strconv.Itoa(atmos.dynamicObjects))
		replaceCanonicalSeedText(stream, "Bed channel count", formatChannels(atmos.bedChannelCount))
		replaceCanonicalSeedText(stream, "Bed channel configuration", atmos.bedChannelConfig)
		applyMatroskaTrueHDCanonicalLayout(stream, "L R C LFE Ls Rs Lb Rb", "Front: L C R, Side: L R, Back: L R, LFE")
		appendCanonicalSeedObjectMembers(stream, "extra", []structuredMember{
			{Key: "NumberOfDynamicObjects", Value: structuredNode{Kind: structuredString, Text: strconv.Itoa(atmos.dynamicObjects)}},
			{Key: "BedChannelCount", Value: structuredNode{Kind: structuredString, Text: strconv.FormatUint(atmos.bedChannelCount, 10)}},
			{Key: "BedChannelConfiguration", Value: structuredNode{Kind: structuredString, Text: atmos.bedChannelConfigShort}},
		})
	}
	if info.maxBitRate > 0 {
		raw := strconv.FormatInt(info.maxBitRate, 10)
		replaceCanonicalSeedFill(stream, "BitRate_Maximum", raw, "Maximum bit rate", formatBitrate(float64(info.maxBitRate)))
	}
	replaceCanonicalSeedProjection(stream, "BitRate_Mode", "Variable", "VBR", "Bit rate mode", "Variable")
	if info.sampleRate > 0 && info.samplesPerFrame > 0 {
		frameRate := float64(info.sampleRate) / float64(info.samplesPerFrame)
		frameRateRaw := fmt.Sprintf("%.3f", frameRate)
		replaceCanonicalSeedFill(stream, "FrameRate", frameRateRaw, "Frame rate", formatAudioFrameRate(frameRate, info.samplesPerFrame))
		if info.sampleRate%info.samplesPerFrame == 0 {
			replaceCanonicalSeedFill(stream, "FrameRate_Num", strconv.Itoa(info.sampleRate/info.samplesPerFrame), "", "")
			replaceCanonicalSeedFill(stream, "FrameRate_Den", "1", "", "")
		} else {
			replaceCanonicalSeedFill(stream, "FrameRate_Num", strconv.Itoa(info.sampleRate), "", "")
			replaceCanonicalSeedFill(stream, "FrameRate_Den", strconv.Itoa(info.samplesPerFrame), "", "")
		}
		replaceCanonicalSeedFill(stream, "SamplesPerFrame", strconv.Itoa(info.samplesPerFrame), "", "")
		if durationMilliseconds, ok := canonicalSeedValue(*stream, "Duration"); ok {
			if milliseconds, err := strconv.ParseFloat(durationMilliseconds, 64); err == nil && milliseconds > 0 {
				duration := milliseconds / 1000
				samplingCount := int64(math.Round(duration * float64(info.sampleRate)))
				replaceCanonicalSeedFill(stream, "SamplingCount", strconv.FormatInt(samplingCount, 10), "", "")
				if _, exists := canonicalSeedValue(*stream, "FrameCount"); !exists {
					frameCount := int64(math.Floor(duration * frameRate))
					replaceCanonicalSeedFill(stream, "FrameCount", strconv.FormatInt(frameCount, 10), "", "")
				}
			}
		}
	}
	replaceCanonicalSeedFill(stream, "Compression_Mode", "Lossless", "Compression mode", "Lossless")
}

// applyMatroskaTrueHDCanonicalLayout records the backward-compatible channel
// render shared by ordinary and Atmos TrueHD presentations.
func applyMatroskaTrueHDCanonicalLayout(stream *Stream, layout, positions string) {
	replaceCanonicalSeedFill(stream, "ChannelLayout", layout, "Channel layout", layout)
	replaceCanonicalSeedFill(stream, "ChannelPositions", positions, "", "")
}
