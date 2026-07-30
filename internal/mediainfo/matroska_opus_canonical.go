package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// matroskaOpusCanonicalFacts contains the TrackEntry facts needed to build one
// Opus stream before statistics tags add packet counts and measured sizes.
type matroskaOpusCanonicalFacts struct {
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

// matroskaOpusCanonicalSeed builds one Opus audio stream directly from typed
// TrackEntry facts while leaving packet cadence to trusted statistics tags.
func matroskaOpusCanonicalSeed(facts matroskaOpusCanonicalFacts) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "Opus", "Format", "Opus")
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
	builder.Text("Format/Info", "Opus")
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
		layout := channelLayout(facts.audioChannels)
		positions := channelPositionsFromCount(value)
		if facts.audioChannels == 6 {
			layout = "L R C Lb Rb LFE"
			positions = "Front: L C R, Back: L R, LFE"
		}
		if layout != "" {
			builder.Fill("ChannelLayout", layout, "Channel layout", layout)
		}
		builder.Structured("ChannelPositions", positions)
	}
	if facts.audioSampleRate > 0 {
		value := strconv.FormatFloat(facts.audioSampleRate, 'f', -1, 64)
		builder.Fill("SamplingRate", value, "Sampling rate", formatSampleRate(facts.audioSampleRate))
	}
	builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
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
