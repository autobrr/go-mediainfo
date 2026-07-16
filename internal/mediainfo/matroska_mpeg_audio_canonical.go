package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// matroskaMPEGAudioCanonicalFacts contains the TrackEntry facts needed to
// build one MPEG Audio stream before a bounded frame-header probe refines it.
type matroskaMPEGAudioCanonicalFacts struct {
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
	bitRate                uint64
	structuredDuration     float64
	displayDuration        float64
	durationPrec           int
	audioChannelsFromTrack bool
	defaultValue           bool
	forcedValue            bool
	serviceKinds           []string
}

// matroskaMPEGAudioCanonicalSeed builds one MPEG Audio stream directly from
// TrackEntry facts while retaining JSON-only staged speaker metadata.
func matroskaMPEGAudioCanonicalSeed(facts matroskaMPEGAudioCanonicalFacts) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "MPEG Audio", "Format", "MPEG Audio")
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
	builder.Text("Format/Info", "MPEG Audio")
	if facts.trackUID > 0 {
		builder.Structured("UniqueID", strconv.FormatUint(facts.trackUID, 10))
	}
	if facts.structuredDuration > 0 {
		seconds := facts.structuredDuration
		decimals := uint8(9)
		if facts.durationPrec <= 3 {
			seconds = math.Round(seconds*1000) / 1000
			decimals = 3
		}
		secondsText := fmt.Sprintf("%.*f", decimals, seconds)
		if milliseconds, ok := decimalSecondsToMilliseconds(secondsText); ok {
			displayDuration := facts.displayDuration
			if displayDuration <= 0 {
				displayDuration = facts.structuredDuration
			}
			builder.Fill("Duration", milliseconds, "Duration", formatDuration(displayDuration))
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
		if facts.audioChannelsFromTrack {
			builder.StructuredJSONOnly("ChannelLayout", channelLayout(facts.audioChannels))
			builder.StructuredJSONOnly("ChannelPositions", matroskaGoChannelPositions(facts.audioChannels))
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
