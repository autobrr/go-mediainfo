package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// matroskaFLACCanonicalFacts contains the TrackEntry and STREAMINFO facts
// needed to build one FLAC stream without legacy JSON or display recovery.
type matroskaFLACCanonicalFacts struct {
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
	segmentDuration        float64
	durationPrec           int
	audioChannelsFromTrack bool
	channelsFromPrivate    bool
	defaultValue           bool
	forcedValue            bool
	serviceKinds           []string
	codec                  flacStreamInfo
	encoder                string
}

// matroskaFLACCanonicalSeed builds one FLAC audio stream directly from typed
// TrackEntry and codec-private facts before statistics tags refine measured data.
func matroskaFLACCanonicalSeed(facts matroskaFLACCanonicalFacts) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "FLAC", "Format", "FLAC")
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
	builder.Text("Format/Info", "Free Lossless Audio Codec")
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
	if facts.bitRate > 0 {
		raw := strconv.FormatUint(facts.bitRate, 10)
		builder.Fill("BitRate", raw, "Bit rate", formatBitrate(float64(facts.bitRate)))
	}
	if facts.audioChannels > 0 {
		value := strconv.FormatUint(facts.audioChannels, 10)
		builder.Fill("Channels", value, "Channel(s)", formatChannels(facts.audioChannels))
		layout := matroskaFLACChannelLayout(facts.audioChannels)
		positions := matroskaGoChannelPositions(facts.audioChannels)
		if !facts.channelsFromPrivate {
			if layout != "" {
				builder.Fill("ChannelLayout", layout, "Channel layout", layout)
			}
			builder.Structured("ChannelPositions", positions)
		}
	}
	if facts.audioSampleRate > 0 {
		value := strconv.FormatFloat(facts.audioSampleRate, 'f', -1, 64)
		builder.Fill("SamplingRate", value, "Sampling rate", formatSampleRate(facts.audioSampleRate))
	}
	if facts.codec.sampleRate > 0 {
		samplingCount := uint64(0)
		if facts.segmentDuration > 0 {
			samplingCount = uint64(math.Round(facts.segmentDuration * float64(facts.codec.sampleRate)))
		}
		if facts.codec.minBlockSize > 0 && facts.codec.minBlockSize == facts.codec.maxBlockSize {
			samplesPerFrame := uint64(facts.codec.maxBlockSize)
			builder.Structured("SamplesPerFrame", strconv.FormatUint(samplesPerFrame, 10))
			builder.Structured("FrameRate", fmt.Sprintf("%.3f", float64(facts.codec.sampleRate)/float64(samplesPerFrame)))
			if samplingCount > 0 {
				frameCount := (samplingCount + samplesPerFrame - 1) / samplesPerFrame
				builder.Structured("FrameCount", strconv.FormatUint(frameCount, 10))
			}
		}
		if samplingCount > 0 {
			builder.Structured("SamplingCount", strconv.FormatUint(samplingCount, 10))
		}
	}
	if facts.codec.bitsPerSample > 0 {
		bitDepth := strconv.Itoa(int(facts.codec.bitsPerSample))
		builder.Fill("BitDepth", bitDepth, "Bit depth", bitDepth+" bits")
		if facts.encoder == "" || strings.Contains(facts.encoder, "libFLAC") {
			builder.Structured("BitDepth_Detected", matroskaFLACDetectedBitDepth(facts.codec))
		}
	}
	builder.Fill("Compression_Mode", "Lossless", "Compression mode", "Lossless")
	builder.Structured("Delay", "0.000")
	builder.Structured("Delay_Source", "Container")
	builder.Structured("Video_Delay", "0.000")
	if facts.codecName != "" && strings.Contains(facts.codecName, "Lavc") {
		if facts.encoder == "" {
			builder.Fill("Encoded_Library", canonicalEncodedLibrary(facts.codecName), "Writing library", facts.codecName)
		} else {
			builder.Text("Writing library", facts.codecName)
		}
	}
	if facts.encoder != "" {
		builder.Fill("Encoded_Library", facts.encoder, "Writing library", facts.encoder)
		name, version, date := splitFLACEncodedLibrary(facts.encoder)
		builder.Structured("Encoded_Library_Name", name)
		builder.Structured("Encoded_Library_Version", version)
		builder.Structured("Encoded_Library_Date", date)
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
	if facts.codec.md5 != "" {
		builder.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{{
			Key: "MD5_Unencoded", Value: structuredNode{Kind: structuredString, Text: facts.codec.md5},
		}}})
	}
	return builder.Snapshot(canonicalStreamPolicy{}).canonicalSeed
}

// matroskaFLACChannelLayout maps FLAC channel counts to MediaInfo's channel
// ordering for layouts derived from codec-private metadata.
func matroskaFLACChannelLayout(channels uint64) string {
	switch channels {
	case 1:
		return "M"
	case 2:
		return "L R"
	case 6:
		return "L R C LFE Ls Rs"
	default:
		return channelLayout(channels)
	}
}
