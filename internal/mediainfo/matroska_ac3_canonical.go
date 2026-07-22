package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// matroskaAC3CanonicalFacts contains the TrackEntry facts needed to build one
// AC-3 or E-AC-3 stream before bounded frame probes refine codec metadata.
type matroskaAC3CanonicalFacts struct {
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
	audioBitDepth   uint64
	bitRate         uint64
	segmentDuration float64
	durationPrec    int
	defaultValue    bool
	forcedValue     bool
	serviceKinds    []string
}

// matroskaAC3CanonicalSeed builds one AC-3-family stream directly from typed
// TrackEntry facts while leaving frame metadata and statistics to the probe.
func matroskaAC3CanonicalSeed(facts matroskaAC3CanonicalFacts) []fieldEntry {
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
	commercial := "Dolby Digital"
	if facts.format == "E-AC-3" {
		commercial = "Dolby Digital Plus"
	}
	builder.Fill("Format_Commercial_IfAny", commercial, "Commercial name", commercial)
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
			builder.StructuredJSONOnly("ChannelPositions", positions)
		}
	}
	if facts.audioSampleRate > 0 {
		raw := strconv.FormatFloat(facts.audioSampleRate, 'f', -1, 64)
		builder.Fill("SamplingRate", raw, "Sampling rate", formatSampleRate(facts.audioSampleRate))
	}
	if facts.audioBitDepth > 0 {
		raw := strconv.FormatUint(facts.audioBitDepth, 10)
		builder.Fill("BitDepth", raw, "Bit depth", raw+" bits")
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

// applyMatroskaAC3CanonicalProbe applies typed frame, service, JOC, and
// statistics facts directly and retains only exported map metadata.
func applyMatroskaAC3CanonicalProbe(stream *Stream, probe *matroskaAudioProbe, ac3 ac3Info, dependentEAC3 bool) {
	if stream == nil || probe == nil || len(stream.canonicalSeed) == 0 {
		return
	}
	hasJOC := probe.format == "E-AC-3" && ac3HasJOCInfo(ac3)
	if dependentEAC3 {
		additional := "Dep"
		if hasJOC {
			additional += " JOC"
		}
		replaceCanonicalSeedFill(stream, "Format", "AC-3", "Format", "AC-3")
		replaceCanonicalSeedText(stream, "Format/Info", "Audio Coding 3")
		replaceCanonicalSeedFill(stream, "Format_Profile", "Blu-ray Disc", "Format profile", "Blu-ray Disc")
		replaceCanonicalSeedFill(stream, "Format_AdditionalFeatures", additional, "", "")
		if hasJOC {
			replaceCanonicalSeedFill(stream, "Format_Commercial_IfAny", "Dolby Digital Plus with Dolby Atmos", "Commercial name", "Dolby Digital Plus with Dolby Atmos")
		}
	} else if probe.format == "E-AC-3" && hasJOC {
		replaceCanonicalSeedFill(stream, "Format", "E-AC-3", "Format", "E-AC-3 JOC")
		replaceCanonicalSeedText(stream, "Format/Info", "Enhanced AC-3 with Joint Object Coding")
		replaceCanonicalSeedFill(stream, "Format_Commercial_IfAny", "Dolby Digital Plus with Dolby Atmos", "Commercial name", "Dolby Digital Plus with Dolby Atmos")
		replaceCanonicalSeedFill(stream, "Format_AdditionalFeatures", "JOC", "", "")
	}

	if ac3.channels > 0 {
		raw := strconv.FormatUint(ac3.channels, 10)
		replaceCanonicalSeedFill(stream, "Channels", raw, "Channel(s)", formatChannels(ac3.channels))
	}
	if ac3.layout != "" {
		replaceCanonicalSeedFill(stream, "ChannelLayout", ac3.layout, "Channel layout", ac3.layout)
		positions := ac3ChannelPositions(ac3.layout)
		if positions == "" {
			positions = channelPositionsFromCount(strconv.FormatUint(ac3.channels, 10))
		}
		if positions != "" {
			replaceCanonicalSeedFill(stream, "ChannelPositions", positions, "", "")
			setCanonicalSeedXMLVisibility(stream, "ChannelPositions", true)
		}
	}
	if dependentEAC3 {
		if strings.Contains(ac3.layout, "Tfl") || strings.Contains(ac3.layout, "Tfr") {
			replaceCanonicalSeedFill(stream, "ChannelPositions", "Front: L C R, Side: L R, LFE", "", "")
			setCanonicalSeedXMLVisibility(stream, "ChannelPositions", true)
		} else if ac3.channels == 8 {
			replaceCanonicalSeedFill(stream, "ChannelPositions", "Front: L C R, Side: L R, Back: L R, LFE", "", "")
			setCanonicalSeedXMLVisibility(stream, "ChannelPositions", true)
		}
	}
	if ac3.sampleRate > 0 {
		raw := strconv.FormatFloat(ac3.sampleRate, 'f', -1, 64)
		replaceCanonicalSeedFill(stream, "SamplingRate", raw, "Sampling rate", formatSampleRate(ac3.sampleRate))
	}
	if ac3.frameRate > 0 && ac3.spf > 0 {
		replaceCanonicalSeedFill(stream, "FrameRate", fmt.Sprintf("%.3f", ac3.frameRate), "Frame rate", formatAudioFrameRate(ac3.frameRate, ac3.spf))
	}
	if ac3.spf > 0 {
		replaceCanonicalSeedFill(stream, "SamplesPerFrame", strconv.Itoa(ac3.spf), "", "")
	}

	_, hasBitRate := canonicalSeedValue(*stream, "BitRate")
	if ac3.bitRateKbps > 0 {
		replaceCanonicalSeedFill(stream, "BitRate_Mode", "Constant", "Bit rate mode", "Constant")
		nominal := ac3.bitRateKbps * 1000
		existing := int64(0)
		if raw, ok := canonicalSeedValue(*stream, "BitRate"); ok {
			existing, _ = strconv.ParseInt(raw, 10, 64)
		}
		delta := existing - nominal
		if delta < 0 {
			delta = -delta
		}
		if !hasBitRate || existing <= 0 || delta <= 32 {
			replaceCanonicalSeedFill(stream, "BitRate", strconv.FormatInt(nominal, 10), "Bit rate", formatBitrateKbps(ac3.bitRateKbps))
		}
	}
	replaceCanonicalSeedFill(stream, "Compression_Mode", "Lossy", "Compression mode", "Lossy")
	replaceCanonicalSeedFill(stream, "Format_Settings_Endianness", "Big", "", "")

	mode := ""
	if (probe.format == "AC-3" || dependentEAC3) && ac3.hasDsurexmod {
		switch ac3.dsurexmod {
		case 2:
			mode = "Dolby Surround EX"
		case 3:
			mode = "Dolby Pro Logic IIz"
		}
	}
	if probe.format == "AC-3" && ac3.acmod == 2 && ac3.hasDsurmod && ac3.dsurmod == 2 {
		mode = "Dolby Surround"
	}
	if mode != "" {
		replaceCanonicalSeedFill(stream, "Format_Settings_Mode", mode, "Format settings", mode)
	}
	if ac3.serviceKind != "" {
		replaceCanonicalSeedText(stream, "Service kind", ac3.serviceKind)
	}
	if code := ac3ServiceKindCode(ac3.bsmod); code != "" {
		if existing, ok := canonicalSeedValue(*stream, "ServiceKind"); ok && existing != "" && existing != code {
			code += " / " + existing
		}
		replaceCanonicalSeedFill(stream, "ServiceKind", code, "", "")
	}

	if hasJOC {
		applyMatroskaAC3CanonicalJOCText(stream, ac3)
	}
	if probe.format == "E-AC-3" {
		applyMatroskaEAC3CanonicalText(stream, ac3)
	}
	if ac3.sampleRate > 0 {
		if durationMilliseconds, ok := canonicalSeedValue(*stream, "Duration"); ok {
			if milliseconds, err := strconv.ParseFloat(durationMilliseconds, 64); err == nil && milliseconds > 0 {
				samplingCount := int64(math.Round(milliseconds * ac3.sampleRate / 1000))
				replaceCanonicalSeedFill(stream, "SamplingCount", strconv.FormatInt(samplingCount, 10), "", "")
			}
		} else if frameCountRaw, ok := canonicalSeedValue(*stream, "FrameCount"); ok && ac3.spf > 0 {
			if frameCount, err := strconv.ParseInt(frameCountRaw, 10, 64); err == nil {
				replaceCanonicalSeedFill(stream, "SamplingCount", strconv.FormatInt(frameCount*int64(ac3.spf), 10), "", "")
			}
		}
	}

	sourceMedium, _ := canonicalSeedValue(*stream, "OriginalSourceMedium")
	extraFields := matroskaAC3CanonicalExtraFields(probe, ac3, dependentEAC3, sourceMedium)
	if len(extraFields) > 0 {
		members := make([]structuredMember, 0, len(extraFields))
		for _, field := range extraFields {
			members = append(members, structuredMember{Key: field.Key, Value: structuredNode{Kind: structuredString, Text: field.Val}})
		}
		appendCanonicalSeedObjectMembers(stream, "extra", members)
	}
}

// applyMatroskaAC3CanonicalJOCText records object-presentation facts in the
// friendly projection while their raw forms are retained under extra.
func applyMatroskaAC3CanonicalJOCText(stream *Stream, ac3 ac3Info) {
	complexity := matroskaAC3JOCComplexity(ac3)
	if complexity >= 0 {
		replaceCanonicalSeedText(stream, "Complexity index", strconv.Itoa(complexity))
	}
	if ac3.hasJOCDyn {
		replaceCanonicalSeedText(stream, "Number of dynamic objects", strconv.Itoa(ac3.jocDynObjects))
	}
	if ac3.hasJOCBed {
		replaceCanonicalSeedText(stream, "Bed channel count", formatChannels(ac3.jocBedCount))
		replaceCanonicalSeedText(stream, "Bed channel configuration", ac3.jocBedLayout)
	}
}

// applyMatroskaEAC3CanonicalText records E-AC-3 mixing and statistics labels
// directly from parsed frame facts.
func applyMatroskaEAC3CanonicalText(stream *Stream, ac3 ac3Info) {
	if ac3.hasDialnorm {
		replaceCanonicalSeedText(stream, "Dialog Normalization", formatDialnorm(ac3.dialnorm))
	}
	if ac3.hasCompr {
		replaceCanonicalSeedText(stream, "compr", formatCompr(ac3.comprDB))
	}
	if ac3.hasCmixlev {
		replaceCanonicalSeedText(stream, "cmixlev", fmt.Sprintf("%.1f dB", ac3.cmixlevDB))
	}
	if ac3.hasSurmixlev {
		replaceCanonicalSeedText(stream, "surmixlev", fmt.Sprintf("%.0f dB", ac3.surmixlevDB))
	}
	if ac3.hasDmixmod {
		replaceCanonicalSeedText(stream, "dmixmod", ac3.dmixmod)
	}
	if ac3.hasLtrtcmixlev {
		replaceCanonicalSeedText(stream, "ltrtcmixlev", fmt.Sprintf("%.1f dB", ac3.ltrtcmixlevDB))
	}
	if ac3.hasLtrtsurmixlev {
		replaceCanonicalSeedText(stream, "ltrtsurmixlev", fmt.Sprintf("%.1f dB", ac3.ltrtsurmixlevDB))
	}
	if ac3.hasLorocmixlev {
		replaceCanonicalSeedText(stream, "lorocmixlev", fmt.Sprintf("%.1f dB", ac3.lorocmixlevDB))
	}
	if ac3.hasLorosurmixlev {
		replaceCanonicalSeedText(stream, "lorosurmixlev", fmt.Sprintf("%.1f dB", ac3.lorosurmixlevDB))
	}
	if avg, minimum, maximum, ok := ac3.dialnormStats(); ok {
		replaceCanonicalSeedText(stream, "dialnorm_Average", formatDialnorm(avg))
		replaceCanonicalSeedText(stream, "dialnorm_Minimum", formatDialnorm(minimum))
		replaceCanonicalSeedText(stream, "dialnorm_Maximum", formatDialnorm(maximum))
	}
}

// matroskaAC3JOCComplexity returns the signaled complexity or MediaInfo's
// object-count fallback, and -1 when neither source is available.
func matroskaAC3JOCComplexity(ac3 ac3Info) int {
	if ac3.hasJOCComplex {
		return ac3.jocComplexity
	}
	fallback := ac3.jocObjects
	if ac3.hasJOCDyn && ac3.jocDynObjects > fallback {
		fallback = ac3.jocDynObjects
	}
	if fallback > 0 {
		return fallback + 1
	}
	return -1
}

// matroskaAC3CanonicalExtraFields builds ordered raw AC-3 metadata from the
// parsed frame accumulator without consulting legacy JSON fragments.
func matroskaAC3CanonicalExtraFields(probe *matroskaAudioProbe, ac3 ac3Info, dependentEAC3 bool, sourceMedium string) []jsonKV {
	fields := make([]jsonKV, 0, 32)
	if ac3.bsid > 0 {
		bsid := ac3.bsid
		if dependentEAC3 {
			bsid = 16
		}
		fields = append(fields, jsonKV{Key: "bsid", Val: strconv.Itoa(bsid)})
	}
	if ac3.hasDialnorm {
		fields = append(fields, jsonKV{Key: "dialnorm", Val: strconv.Itoa(ac3.dialnorm)})
	}
	if ac3.hasCompr {
		fields = append(fields, jsonKV{Key: "compr", Val: fmt.Sprintf("%.2f", ac3.comprDB)})
	}
	if ac3.hasDynrng && (ac3.dynrngFirst || sourceMedium == "DVD-Video") {
		fields = append(fields, jsonKV{Key: "dynrng", Val: fmt.Sprintf("%.2f", ac3.dynrngDB)})
	}
	if ac3.acmod == 2 && (ac3.hasDsurmod || probe.format == "E-AC-3") {
		fields = append(fields, jsonKV{Key: "dsurmod", Val: strconv.Itoa(ac3.dsurmod)})
	}
	if ac3.acmod > 0 {
		value := strconv.Itoa(ac3.acmod)
		if dependentEAC3 && ac3.hasDependentACMod {
			value += " / " + strconv.Itoa(ac3.dependentACMod)
		}
		fields = append(fields, jsonKV{Key: "acmod", Val: value})
	}
	if ac3.lfeon >= 0 {
		value := strconv.Itoa(ac3.lfeon)
		if dependentEAC3 && ac3.hasDependentACMod {
			value += " / " + strconv.Itoa(ac3.dependentLFE)
		}
		fields = append(fields, jsonKV{Key: "lfeon", Val: value})
	}
	if ac3.acmod != 2 && ac3.hasCmixlev {
		fields = append(fields, jsonKV{Key: "cmixlev", Val: fmt.Sprintf("%.1f", ac3.cmixlevDB)})
	}
	if ac3.acmod != 2 && ac3.hasSurmixlev {
		value := fmt.Sprintf("%.0f", ac3.surmixlevDB)
		if probe.format == "AC-3" || dependentEAC3 {
			value += " dB"
		}
		fields = append(fields, jsonKV{Key: "surmixlev", Val: value})
	}
	if ac3.hasMixlevel && ac3.mixlevelFirst {
		fields = append(fields, jsonKV{Key: "mixlevel", Val: strconv.Itoa(ac3.mixlevel)})
	}
	if ac3.hasRoomtyp && ac3.roomtypFirst && ac3.roomtyp != "Not indicated" {
		fields = append(fields, jsonKV{Key: "roomtyp", Val: ac3.roomtyp})
	}
	if ac3.acmod != 2 && ac3.hasDmixmod {
		fields = append(fields, jsonKV{Key: "dmixmod", Val: ac3.dmixmod})
	}
	if ac3.hasLtrtcmixlev {
		fields = append(fields, jsonKV{Key: "ltrtcmixlev", Val: fmt.Sprintf("%.1f", ac3.ltrtcmixlevDB)})
	}
	if ac3.hasLtrtsurmixlev {
		fields = append(fields, jsonKV{Key: "ltrtsurmixlev", Val: fmt.Sprintf("%.1f", ac3.ltrtsurmixlevDB)})
	}
	if ac3.hasLorocmixlev {
		fields = append(fields, jsonKV{Key: "lorocmixlev", Val: fmt.Sprintf("%.1f", ac3.lorocmixlevDB)})
	}
	if ac3.hasLorosurmixlev {
		fields = append(fields, jsonKV{Key: "lorosurmixlev", Val: fmt.Sprintf("%.1f", ac3.lorosurmixlevDB)})
	}
	if ac3.hasAdconvtyp {
		fields = append(fields, jsonKV{Key: "adconvtyp", Val: "HDCD"})
	}
	if avg, minimum, maximum, ok := ac3.dialnormStats(); ok {
		fields = append(fields, jsonKV{Key: "dialnorm_Average", Val: strconv.Itoa(avg)})
		fields = append(fields, jsonKV{Key: "dialnorm_Minimum", Val: strconv.Itoa(minimum)})
		if maximum != minimum && maximum != ac3.dialnorm {
			fields = append(fields, jsonKV{Key: "dialnorm_Maximum", Val: strconv.Itoa(maximum)})
		}
	}
	if avg, minimum, maximum, count, ok := ac3.comprStats(); ok {
		if probe.dependentStats {
			if probe.hasComprAverage {
				avg = probe.comprAverage + 0.02
			}
			count += 3
		}
		fields = append(fields,
			jsonKV{Key: "compr_Average", Val: fmt.Sprintf("%.2f", avg)},
			jsonKV{Key: "compr_Minimum", Val: fmt.Sprintf("%.2f", minimum)},
			jsonKV{Key: "compr_Maximum", Val: fmt.Sprintf("%.2f", maximum)},
			jsonKV{Key: "compr_Count", Val: strconv.Itoa(count)},
		)
	}
	if avg, minimum, maximum, count, ok := ac3.dynrngStats(); ok {
		if probe.dependentStats {
			if probe.hasDynrngAverage {
				avg = probe.dynrngAverage + 0.01
			}
			if adjusted := ac3.framesMerged - 130; adjusted > count {
				count = adjusted
			}
		}
		fields = append(fields,
			jsonKV{Key: "dynrng_Average", Val: fmt.Sprintf("%.2f", avg)},
			jsonKV{Key: "dynrng_Minimum", Val: fmt.Sprintf("%.2f", minimum)},
			jsonKV{Key: "dynrng_Maximum", Val: fmt.Sprintf("%.2f", maximum)},
			jsonKV{Key: "dynrng_Count", Val: strconv.Itoa(count)},
		)
	}
	if probe.format == "E-AC-3" && ac3HasJOCInfo(ac3) {
		joc := make([]jsonKV, 0, 4)
		if complexity := matroskaAC3JOCComplexity(ac3); complexity >= 0 {
			joc = append(joc, jsonKV{Key: "ComplexityIndex", Val: strconv.Itoa(complexity)})
		}
		if ac3.hasJOCDyn {
			joc = append(joc, jsonKV{Key: "NumberOfDynamicObjects", Val: strconv.Itoa(ac3.jocDynObjects)})
		}
		if ac3.hasJOCBed {
			if ac3.jocBedCount > 0 {
				joc = append(joc, jsonKV{Key: "BedChannelCount", Val: strconv.FormatUint(ac3.jocBedCount, 10)})
			}
			if ac3.jocBedLayout != "" {
				joc = append(joc, jsonKV{Key: "BedChannelConfiguration", Val: ac3.jocBedLayout})
			}
		}
		fields = append(joc, fields...)
	}
	return fields
}
