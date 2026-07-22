package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// matroskaVorbisCanonicalFacts contains the TrackEntry and codec-private facts
// needed to build one Vorbis stream without legacy JSON or display recovery.
type matroskaVorbisCanonicalFacts struct {
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
	segmentDuration        float64
	durationPrec           int
	audioChannelsFromTrack bool
	defaultValue           bool
	forcedValue            bool
	serviceKinds           []string
	codec                  matroskaVorbisInfo
}

// matroskaVorbisCanonicalSeed builds one Vorbis audio stream directly from
// TrackEntry and codec-private facts, preserving JSON-only channel extensions.
func matroskaVorbisCanonicalSeed(facts matroskaVorbisCanonicalFacts) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "Vorbis", "Format", "Vorbis")
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
	builder.Text("Format/Info", "Vorbis")
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
	builder.Fill("BitRate_Mode", "Variable", "Bit rate mode", "Variable")
	if facts.codec.nominalBitRate > 0 {
		raw := strconv.FormatInt(facts.codec.nominalBitRate, 10)
		builder.Fill("BitRate", raw, "Bit rate", formatBitrate(float64(facts.codec.nominalBitRate)))
		if facts.segmentDuration > 0 {
			streamSize := int64(math.Round(float64(facts.codec.nominalBitRate) * facts.segmentDuration / 8))
			builder.Structured("StreamSize", strconv.FormatInt(streamSize, 10))
		}
	}
	if facts.codec.minimumBitRate > 0 {
		builder.Structured("BitRate_Minimum", strconv.FormatInt(facts.codec.minimumBitRate, 10))
	}
	if facts.codec.maximumBitRate > 0 {
		builder.Structured("BitRate_Maximum", strconv.FormatInt(facts.codec.maximumBitRate, 10))
	}
	if facts.audioChannels > 0 {
		value := strconv.FormatUint(facts.audioChannels, 10)
		builder.Fill("Channels", value, "Channel(s)", formatChannels(facts.audioChannels))
	}
	if facts.audioSampleRate > 0 {
		value := strconv.FormatFloat(facts.audioSampleRate, 'f', -1, 64)
		builder.Fill("SamplingRate", value, "Sampling rate", formatSampleRate(facts.audioSampleRate))
		if facts.segmentDuration > 0 {
			samplingCount := int64(math.RoundToEven(facts.audioSampleRate * facts.segmentDuration))
			builder.Structured("SamplingCount", strconv.FormatInt(samplingCount, 10))
		}
	}
	builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
	builder.Structured("Format_Settings_Floor", "1")
	builder.Structured("Delay", "0.000")
	builder.Structured("Delay_Source", "Container")
	builder.Structured("Video_Delay", "0.000")
	if facts.codec.encoder != "" {
		builder.Fill("Encoded_Application", facts.codec.encoder, "Writing application", facts.codec.encoder)
	}
	if facts.codec.vendor != "" {
		builder.Fill("Encoded_Library", facts.codec.vendor, "Writing library", facts.codec.vendor)
		name, version, date := splitMatroskaVorbisLibrary(facts.codec.vendor)
		builder.Structured("Encoded_Library_Name", name)
		builder.Structured("Encoded_Library_Version", version)
		builder.Structured("Encoded_Library_Date", date)
	}
	if facts.codecName != "" && strings.Contains(facts.codecName, "Lavc") {
		if facts.codec.vendor == "" {
			builder.Fill("Encoded_Library", canonicalEncodedLibrary(facts.codecName), "Writing library", facts.codecName)
		} else {
			builder.Text("Writing library", facts.codecName)
		}
	}
	if facts.codec.applicationURL != "" {
		builder.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{{
			Key: "Encoded_Application_Url", Value: structuredNode{Kind: structuredString, Text: facts.codec.applicationURL},
		}}})
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
