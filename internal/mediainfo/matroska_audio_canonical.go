package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// matroskaPCMCanonicalFacts contains the TrackEntry facts needed to build one
// PCM audio stream without consulting legacy display or JSON projections.
type matroskaPCMCanonicalFacts struct {
	codecID                string
	codecName              string
	trackName              string
	languageCode           string
	displayLanguage        string
	trackNumber            uint64
	trackUID               uint64
	contentCompAlgo        uint64
	audioChannels          uint64
	audioSampleRate        float64
	audioBitDepth          uint64
	bitRate                uint64
	defaultDuration        uint64
	segmentDuration        float64
	durationPrec           int
	audioChannelsFromTrack bool
	defaultValue           bool
	forcedValue            bool
	serviceKinds           []string
}

// matroskaFallbackAudioCanonicalFacts contains common TrackEntry facts for an
// audio CodecID that has no format-specific canonical builder.
type matroskaFallbackAudioCanonicalFacts struct {
	format          string
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

// matroskaFallbackAudioCanonicalSeed builds common identity, timing, channel,
// and disposition fields for otherwise unsupported Matroska audio codecs.
func matroskaFallbackAudioCanonicalSeed(facts matroskaFallbackAudioCanonicalFacts) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", facts.format, "Format", facts.format)
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
	if info := mapMatroskaFormatInfo(facts.format); info != "" {
		builder.Text("Format/Info", info)
	}
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
		value := strconv.FormatUint(facts.bitRate, 10)
		builder.Fill("BitRate", value, "Bit rate", formatBitrate(float64(facts.bitRate)))
	}
	if facts.audioChannels > 0 {
		value := strconv.FormatUint(facts.audioChannels, 10)
		builder.Fill("Channels", value, "Channel(s)", formatChannels(facts.audioChannels))
		if layout := channelLayout(facts.audioChannels); layout != "" {
			builder.Fill("ChannelLayout", layout, "Channel layout", layout)
		}
		if positions := matroskaGoChannelPositions(facts.audioChannels); positions != "" {
			builder.Structured("ChannelPositions", positions)
		}
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

// matroskaPCMCanonicalSeed builds one PCM audio stream directly from typed
// TrackEntry facts, including Matroska's structured-only PCM extensions.
func matroskaPCMCanonicalSeed(facts matroskaPCMCanonicalFacts) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "PCM", "Format", "PCM")
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
	builder.Text("Format/Info", "PCM")
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
		builder.Structured("BitRate_Mode", "Constant")
		value := strconv.FormatUint(facts.bitRate, 10)
		builder.Fill("BitRate", value, "Bit rate", formatBitrate(float64(facts.bitRate)))
	}
	if facts.audioChannels > 0 {
		value := strconv.FormatUint(facts.audioChannels, 10)
		builder.Fill("Channels", value, "Channel(s)", formatChannels(facts.audioChannels))
		if facts.audioChannelsFromTrack && strings.HasPrefix(facts.codecID, "A_MS/ACM / 00000001-") {
			layout := channelLayout(facts.audioChannels)
			if facts.audioChannels == 6 {
				layout = "L R C LFE Ls Rs"
			}
			if layout != "" {
				builder.StructuredJSONOnly("ChannelLayout", layout)
			}
			if positions := matroskaGoChannelPositions(facts.audioChannels); positions != "" {
				builder.StructuredJSONOnly("ChannelPositions", positions)
			}
		}
	}
	if facts.audioSampleRate > 0 {
		value := strconv.FormatFloat(facts.audioSampleRate, 'f', -1, 64)
		builder.Fill("SamplingRate", value, "Sampling rate", formatSampleRate(facts.audioSampleRate))
	}
	if facts.audioChannelsFromTrack && facts.defaultDuration > 0 && facts.audioSampleRate > 0 && facts.audioChannels > 0 && facts.audioBitDepth > 0 && !strings.HasPrefix(facts.codecID, "A_MS/ACM") {
		samplesPerFrame := int64(math.Round(facts.audioSampleRate * float64(facts.defaultDuration) / 1e9))
		if samplesPerFrame > 0 {
			frameRate := facts.audioSampleRate / float64(samplesPerFrame)
			builder.Structured("SamplesPerFrame", strconv.FormatInt(samplesPerFrame, 10))
			builder.Structured("FrameRate", fmt.Sprintf("%.3f", frameRate))
			declaredRate := 1e9 / float64(facts.defaultDuration)
			if math.Abs(declaredRate-math.Round(declaredRate)) < 1e-9 {
				builder.Structured("FrameRate_Num", strconv.FormatInt(int64(math.Round(frameRate)), 10))
				builder.Structured("FrameRate_Den", "1")
			}
		}
	}
	if facts.segmentDuration > 0 && facts.audioSampleRate > 0 {
		samplingCount := int64(math.RoundToEven(facts.segmentDuration * facts.audioSampleRate))
		builder.Structured("SamplingCount", strconv.FormatInt(samplingCount, 10))
	}
	if facts.audioBitDepth > 0 {
		value := strconv.FormatUint(facts.audioBitDepth, 10)
		builder.Fill("BitDepth", value, "Bit depth", fmt.Sprintf("%d bits", facts.audioBitDepth))
	}
	builder.Structured("BitRate_Mode", "CBR")
	switch {
	case strings.Contains(facts.codecID, "/LIT"):
		builder.Structured("Format_Settings_Endianness", "Little")
	case strings.Contains(facts.codecID, "/BIG"):
		builder.Structured("Format_Settings_Endianness", "Big")
	}
	if strings.Contains(facts.codecID, "/INT/") {
		builder.Structured("Format_Settings_Sign", "Signed")
	}
	if strings.HasPrefix(facts.codecID, "A_MS/ACM / 00000001-") {
		builder.Structured("Format_Settings_Endianness", "Little")
		builder.Structured("Format_Settings_Sign", "Signed")
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
