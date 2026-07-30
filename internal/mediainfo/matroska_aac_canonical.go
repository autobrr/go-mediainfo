package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// matroskaAACCanonicalFacts contains AudioSpecificConfig and TrackEntry facts
// needed to build one AAC stream without legacy display or JSON recovery.
type matroskaAACCanonicalFacts struct {
	profile         string
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
	audioBaseRate   float64
	bitRate         uint64
	segmentDuration float64
	durationPrec    int
	sbrMode         string
	psMode          string
	defaultValue    bool
	forcedValue     bool
	serviceKinds    []string
}

// matroskaAACCanonicalSeed builds one AAC stream directly from TrackEntry and
// AudioSpecificConfig facts, including structured-only SBR and PS signaling.
func matroskaAACCanonicalSeed(facts matroskaAACCanonicalFacts) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamAudio)
	displayFormat := "AAC"
	if facts.profile != "" {
		displayFormat += " " + facts.profile
	}
	builder.Fill("Format", "AAC", "Format", displayFormat)
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
	if facts.profile == "LC" {
		builder.Text("Format/Info", "Advanced Audio Codec Low Complexity")
	} else {
		builder.Text("Format/Info", "Advanced Audio Codec")
	}
	additional := facts.profile
	if strings.HasPrefix(facts.sbrMode, "Yes") {
		additional = strings.TrimSpace(facts.profile + " SBR")
		builder.Fill("Format_Commercial_IfAny", "HE-AAC", "Commercial name", "HE-AAC")
	}
	if facts.sbrMode != "" {
		builder.Structured("Format_Settings_SBR", facts.sbrMode)
	}
	if facts.psMode != "" {
		builder.Structured("Format_Settings_PS", facts.psMode)
	} else if facts.sbrMode == "Yes (Explicit)" && facts.bitRate == 0 {
		builder.Structured("Format_Settings_PS", "No (Explicit)")
	}
	if additional != "" {
		builder.Structured("Format_AdditionalFeatures", additional)
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
		raw := strconv.FormatUint(facts.bitRate, 10)
		builder.Fill("BitRate", raw, "Bit rate", formatBitrate(float64(facts.bitRate)))
	}
	if facts.audioChannels > 0 {
		raw := strconv.FormatUint(facts.audioChannels, 10)
		builder.Fill("Channels", raw, "Channel(s)", formatChannels(facts.audioChannels))
		if layout := channelLayout(facts.audioChannels); layout != "" {
			builder.Fill("ChannelLayout", layout, "Channel layout", layout)
		}
		if positions := matroskaGoChannelPositions(facts.audioChannels); positions != "" {
			builder.Structured("ChannelPositions", positions)
		}
	}
	samplesPerFrame := int64(1024)
	if strings.HasPrefix(facts.sbrMode, "Yes") || facts.audioBaseRate > 0 && facts.audioSampleRate > facts.audioBaseRate {
		samplesPerFrame = 2048
	}
	if facts.audioSampleRate > 0 {
		raw := strconv.FormatFloat(facts.audioSampleRate, 'f', -1, 64)
		builder.Fill("SamplingRate", raw, "Sampling rate", formatSampleRate(facts.audioSampleRate))
		frameRate := facts.audioSampleRate / float64(samplesPerFrame)
		builder.Fill("FrameRate", fmt.Sprintf("%.3f", frameRate), "Frame rate", fmt.Sprintf("%.4f FPS (%d SPF)", frameRate, samplesPerFrame))
		builder.Structured("SamplesPerFrame", strconv.FormatInt(samplesPerFrame, 10))
		if facts.segmentDuration > 0 {
			samplingCount := int64(math.Round(facts.segmentDuration * facts.audioSampleRate))
			builder.Structured("SamplingCount", strconv.FormatInt(samplingCount, 10))
		}
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
