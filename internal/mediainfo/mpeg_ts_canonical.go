package mediainfo

import (
	"math"
	"strconv"
	"strings"
)

// mpegTSStructuredFacts stages exact TS projections alongside canonical
// base-unit values without exposing serializer-shaped parser state.
type mpegTSStructuredFacts struct {
	canonicalStructuredFacts
}

// Set records one projected TS scalar after converting measured seconds to
// canonical base units.
func (f *mpegTSStructuredFacts) Set(name fieldName, value string) {
	if f == nil {
		return
	}
	f.canonicalStructuredFacts.Set(name, canonicalMPEGTSStructuredValue(name, value), value)
}

// Delete removes one staged TS scalar.
func (f *mpegTSStructuredFacts) Delete(name fieldName) {
	if f != nil {
		f.canonicalStructuredFacts.Delete(name)
	}
}

// buildCanonicalMPEGTSStream assembles one TS-family stream from demuxer facts.
// canonicalSeed remains the rendering source; deprecated public maps are
// published from it only by the compatibility adapter.
func buildCanonicalMPEGTSStream(kind StreamKind, stream *tsStream, fields []Field, values *mpegTSStructuredFacts, extra *structuredNode, isBDAV bool) Stream {
	builder := newCanonicalStreamBuilder(kind)
	facts := canonicalMPEGTSFacts(stream, fields, values, isBDAV)
	for _, fact := range facts.ordered(kind) {
		name := fact.name
		label := mpegTSStructuredTextLabel(name)
		display := findField(fields, label)
		if label != "" && display != "" {
			builder.Fill(name, fact.canonical, label, display)
		} else {
			builder.DirectStructured(name, fact.canonical)
		}
		if spec, known := structuredFieldSpec(kind, string(name)); known && spec.Measure == fieldMeasureMilliseconds {
			if decimals := decimalFractionDigits(fact.projection); decimals > 3 {
				builder.SetStructuredDecimals(name, uint8(decimals))
			}
		}
	}
	if extra != nil {
		builder.StructuredNode("extra", *extra)
	}
	appendMissingMPEGTSCanonicalText(builder, fields)
	return builder.Snapshot(canonicalStreamPolicy{})
}

// replaceMPEGTSCanonicalSnapshotField updates one authoritative canonical fact
// and refreshes its deprecated public snapshot at the TS parser boundary.
func replaceMPEGTSCanonicalSnapshotField(stream *Stream, name fieldName, value, label, display string) {
	if stream == nil || name == "" || value == "" {
		return
	}
	replaceCanonicalSeedFill(stream, name, value, label, display)
	refreshCanonicalCompatibilitySnapshot(stream)
}

// decimalFractionDigits returns the number of digits after a decimal point.
func decimalFractionDigits(value string) int {
	_, fraction, found := strings.Cut(value, ".")
	if !found {
		return 0
	}
	return len(fraction)
}

// canonicalMPEGTSFacts combines exact structured overrides with raw demuxer
// and codec values that legacy JSON previously recovered from display fields.
func canonicalMPEGTSFacts(stream *tsStream, fields []Field, values *mpegTSStructuredFacts, isBDAV bool) *canonicalStructuredFacts {
	facts := &canonicalStructuredFacts{}
	if values != nil {
		facts.values = append(facts.values, values.values...)
	}
	if stream == nil {
		copyMPEGTSFieldFacts(facts, fields)
		return facts
	}
	fillCanonicalMPEGTSIdentityFacts(facts, stream, isBDAV)

	if facts.Canonical("Width") == "" && stream.width > 0 {
		facts.SetCanonical("Width", strconv.FormatUint(stream.width, 10))
	}
	if facts.Canonical("Height") == "" && stream.height > 0 {
		facts.SetCanonical("Height", strconv.FormatUint(stream.height, 10))
	}
	if facts.Canonical("DisplayAspectRatio") == "" && stream.width > 0 && stream.height > 0 {
		displayAspect := float64(stream.width) / float64(stream.height)
		if stream.format == "MPEG Video" && stream.hasMPEG2Info && stream.mpeg2Info.AspectRatio != "" {
			if parsed, ok := parseRatioFloat(stream.mpeg2Info.AspectRatio); ok && parsed > 0 {
				displayAspect = parsed
			}
		}
		facts.SetCanonical("DisplayAspectRatio", formatJSONFloat(displayAspect))
	}

	fillCanonicalMPEGTSVideoFacts(facts, stream)
	fillCanonicalMPEGTSAudioFacts(facts, stream, isBDAV)
	copyMPEGTSFieldFacts(facts, fields)
	if stream.kind == StreamAudio && facts.Canonical("ChannelLayout") != "" && facts.Canonical("ChannelPositions") == "" {
		if channels := facts.Canonical("Channels"); channels != "" {
			setMPEGTSFactIfMissing(facts, "ChannelPositions", channelPositionsFromCount(channels))
		}
	}
	return facts
}

// fillCanonicalMPEGTSIdentityFacts records stream identity and codec values
// directly from PMT descriptors and parsed codec state.
func fillCanonicalMPEGTSIdentityFacts(facts *canonicalStructuredFacts, stream *tsStream, isBDAV bool) {
	format := stream.format
	switch {
	case isBDAV && (stream.hasTrueHD || stream.streamType == 0x83):
		format = "MLP FBA"
	case stream.kind == StreamAudio && stream.format == "E-AC-3" && ac3HasJOCInfo(stream.ac3Info):
		format = "E-AC-3 JOC"
	case stream.kind == StreamAudio && stream.audioProfile != "":
		format = "AAC"
		setMPEGTSFactIfMissing(facts, "Format_AdditionalFeatures", stream.audioProfile)
	}
	setMPEGTSFactIfMissing(facts, "Format", format)

	if stream.kind == StreamAudio && stream.audioProfile != "" && stream.audioMPEGVersion > 0 {
		setMPEGTSFactIfMissing(facts, "Format_Version", strconv.Itoa(stream.audioMPEGVersion))
	}
	if stream.kind == StreamVideo && stream.hasMPEG2Info {
		version := strings.TrimPrefix(stream.mpeg2Info.Version, "Version ")
		setMPEGTSFactIfMissing(facts, "Format_Version", version)
		profile, level := splitMPEGTSProfileLevel(stream.mpeg2Info.Profile)
		setMPEGTSFactIfMissing(facts, "Format_Profile", profile)
		setMPEGTSFactIfMissing(facts, "Format_Level", level)
	}
	if stream.streamType != 0 {
		codecID := formatTSCodecID(stream.streamType)
		if stream.kind == StreamAudio && stream.format == "DTS" {
			if isBDAV && stream.dtsHD {
				codecID = formatTSCodecID(0x86)
			} else {
				codecID = formatTSCodecID(0x82)
			}
		}
		if stream.kind == StreamAudio {
			if stream.audioObject > 0 {
				codecID += "-" + strconv.Itoa(stream.audioObject)
			} else if stream.streamType == 0x11 {
				codecID += "-2"
			}
		}
		setMPEGTSFactIfMissing(facts, "CodecID", codecID)
	}
}

// splitMPEGTSProfileLevel separates the parser's semantic profile-level token.
func splitMPEGTSProfileLevel(value string) (string, string) {
	profile, level, found := strings.Cut(value, "@")
	if !found {
		return value, ""
	}
	return profile, strings.TrimPrefix(level, "L")
}

// copyMPEGTSFieldFacts copies string-valued parser facts whose structured form
// does not require interpreting a formatted unit or numeric display value.
func copyMPEGTSFieldFacts(facts *canonicalStructuredFacts, fields []Field) {
	for _, field := range fields {
		key := ""
		value := field.Value
		switch field.Name {
		case "Format":
			key = "Format"
		case "Format profile":
			key = "Format_Profile"
		case "Format tier":
			key = "Format_Tier"
		case "Format level":
			key = "Format_Level"
		case "Format version":
			key = "Format_Version"
		case "Format settings, CABAC":
			key = "Format_Settings_CABAC"
		case "Format settings, BVOP":
			key = "Format_Settings_BVOP"
		case "Format settings, Matrix":
			key = "Format_Settings_Matrix"
		case "Format settings, GOP":
			key = "Format_Settings_GOP"
		case "Format settings, Picture structure":
			key = "Format_Settings_PictureStructure"
		case "Codec ID":
			key = "CodecID"
		case "Commercial name":
			key = "Format_Commercial_IfAny"
		case "Muxing mode":
			key = "MuxingMode"
		case "Muxing mode, more info":
			key = "MuxingMode_MoreInfo"
		case "Bit rate mode":
			key = "BitRate_Mode"
		case "Chroma subsampling":
			key = "ChromaSubsampling"
		case "Chroma subsampling position":
			key = "ChromaSubsampling_Position"
		case "Color space":
			key = "ColorSpace"
		case "Scan type":
			key = "ScanType"
		case "Scan order":
			key = "ScanOrder"
		case "Writing library":
			value = canonicalEncodedLibrary(value)
			key = "Encoded_Library"
			if name, version := splitEncodedLibrary(value); name != "" {
				setMPEGTSFactIfMissing(facts, "Encoded_Library_Name", name)
				setMPEGTSFactIfMissing(facts, "Encoded_Library_Version", version)
			}
		case "Encoding settings":
			key = "Encoded_Library_Settings"
		case "Language":
			key = "Language"
		case "Channel layout":
			key = "ChannelLayout"
		case "Compression mode":
			key = "Compression_Mode"
		case "Time code of first frame":
			key = "TimeCode_FirstFrame"
		case "Time code source":
			key = "TimeCode_Source"
		case "GOP, Open/Closed":
			key = "Gop_OpenClosed"
		case "GOP, Open/Closed of first frame":
			key = "Gop_OpenClosed_FirstFrame"
		case "Standard":
			key = "Standard"
		case "Service kind":
			key = "ServiceKind"
		case "Service name":
			key = "ServiceName"
		case "Service provider":
			key = "ServiceProvider"
		case "Service type":
			key = "ServiceType"
		}
		setMPEGTSFactIfMissing(facts, key, value)
	}
}

// fillCanonicalMPEGTSVideoFacts adds numeric video facts from codec state.
func fillCanonicalMPEGTSVideoFacts(facts *canonicalStructuredFacts, stream *tsStream) {
	if stream == nil || stream.kind != StreamVideo {
		return
	}
	if stream.videoFrameRate > 0 {
		setMPEGTSFactIfMissing(facts, "FrameRate", formatJSONFloat(stream.videoFrameRate))
		if numerator, denominator := rationalizeFrameRate(stream.videoFrameRate); numerator > 0 && denominator > 0 {
			setMPEGTSFactIfMissing(facts, "FrameRate_Num", strconv.Itoa(numerator))
			setMPEGTSFactIfMissing(facts, "FrameRate_Den", strconv.Itoa(denominator))
		}
	}
	if stream.hasH264SPS {
		profile := mapAVCProfile(stream.h264SPS.ProfileID)
		if profile == "High" && stream.h264SPS.ConstraintFlags&0x08 != 0 {
			profile = "Progressive High"
		}
		setMPEGTSFactIfMissing(facts, "Format_Profile", profile)
		setMPEGTSFactIfMissing(facts, "Format_Level", strings.TrimPrefix(formatAVCLevel(stream.h264SPS.LevelID), "L"))
		if stream.h264SPS.RefFrames > 0 {
			setMPEGTSFactIfMissing(facts, "Format_Settings_RefFrames", strconv.Itoa(stream.h264SPS.RefFrames))
		}
		if stream.h264SPS.BitDepth > 0 {
			setMPEGTSFactIfMissing(facts, "BitDepth", strconv.Itoa(stream.h264SPS.BitDepth))
		}
		if stream.h264SPS.ChromaFormat != "" {
			setMPEGTSFactIfMissing(facts, "ColorSpace", "YUV")
			setMPEGTSFactIfMissing(facts, "ChromaSubsampling", stream.h264SPS.ChromaFormat)
		}
	}
	if stream.hasHEVCSPS {
		setMPEGTSFactIfMissing(facts, "Format_Profile", hevcProfileName(stream.hevcSPS.ProfileID))
		setMPEGTSFactIfMissing(facts, "Format_Level", hevcLevelName(stream.hevcSPS.LevelID))
		setMPEGTSFactIfMissing(facts, "Format_Tier", stream.hevcSPS.HEVCTier)
		if stream.hevcSPS.BitDepth > 0 {
			setMPEGTSFactIfMissing(facts, "BitDepth", strconv.Itoa(stream.hevcSPS.BitDepth))
		}
		if stream.hevcSPS.ChromaFormat != "" {
			setMPEGTSFactIfMissing(facts, "ColorSpace", "YUV")
			setMPEGTSFactIfMissing(facts, "ChromaSubsampling", stream.hevcSPS.ChromaFormat)
		}
	}
	if stream.hasMPEG2Info {
		info := stream.mpeg2Info
		if info.FrameRateNumer > 0 && info.FrameRateDenom > 0 {
			setMPEGTSFactIfMissing(facts, "FrameRate", formatJSONFloat(float64(info.FrameRateNumer)/float64(info.FrameRateDenom)))
			setMPEGTSFactIfMissing(facts, "FrameRate_Num", strconv.FormatUint(uint64(info.FrameRateNumer), 10))
			setMPEGTSFactIfMissing(facts, "FrameRate_Den", strconv.FormatUint(uint64(info.FrameRateDenom), 10))
		}
		if info.BitDepth != "" {
			setMPEGTSFactIfMissing(facts, "BitDepth", strings.TrimSuffix(info.BitDepth, " bits"))
		}
		setMPEGTSFactIfMissing(facts, "ColorSpace", info.ColorSpace)
		setMPEGTSFactIfMissing(facts, "ChromaSubsampling", info.ChromaSubsampling)
		setMPEGTSFactIfMissing(facts, "ScanType", info.ScanType)
		setMPEGTSFactIfMissing(facts, "ScanOrder", info.ScanOrder)
	}
	if stream.vc1Parsed {
		setMPEGTSFactIfMissing(facts, "BitDepth", "8")
	}
	if stream.encoding != "" && stream.format != "HEVC" {
		if bitrate, ok := findX264Bitrate(stream.encoding); ok {
			setMPEGTSFactIfMissing(facts, "BitRate_Nominal", strconv.FormatInt(int64(math.Round(bitrate)), 10))
		}
	}
}

// fillCanonicalMPEGTSAudioFacts adds numeric audio facts from decoded headers.
func fillCanonicalMPEGTSAudioFacts(facts *canonicalStructuredFacts, stream *tsStream, isBDAV bool) {
	if stream == nil || stream.kind != StreamAudio {
		return
	}
	channels := stream.audioChannels
	if isBDAV && (stream.hasTrueHD || stream.streamType == 0x83) {
		if stream.hasTrueHDInfo && stream.trueHDInfo.channelMap != 0 {
			channels = trueHDChannels(stream.trueHDInfo.channelMap)
		} else if channels < 8 {
			channels = 8
		}
	}
	if isBDAV && stream.format == "DTS" && !stream.dtsHDX && channels > 6 {
		channels = 6
	}
	if channels > 0 {
		setMPEGTSFactIfMissing(facts, "Channels", strconv.FormatUint(channels, 10))
		if layout := facts.Canonical("ChannelLayout"); layout != "" {
			setMPEGTSFactIfMissing(facts, "ChannelPositions", channelPositionsFromCount(strconv.FormatUint(channels, 10)))
		}
	}
	rate := stream.audioRate
	spf := stream.audioSpf
	if stream.hasAC3 {
		if rate == 0 {
			rate = float64(stream.ac3Info.sampleRate)
		}
		if spf == 0 {
			spf = stream.ac3Info.spf
		}
	}
	if rate > 0 {
		setMPEGTSFactIfMissing(facts, "SamplingRate", strconv.FormatInt(int64(math.Round(rate)), 10))
	}
	if spf > 0 {
		setMPEGTSFactIfMissing(facts, "SamplesPerFrame", strconv.Itoa(spf))
		if rate > 0 {
			setMPEGTSFactIfMissing(facts, "FrameRate", formatJSONFloat(rate/float64(spf)))
		}
	}
	if stream.audioBitRateKbps > 0 {
		setMPEGTSFactIfMissing(facts, "BitRate", strconv.FormatInt(stream.audioBitRateKbps*1000, 10))
	}
	if stream.audioBitDepth > 0 {
		setMPEGTSFactIfMissing(facts, "BitDepth", strconv.Itoa(stream.audioBitDepth))
	}
	if duration, ok := valuesDurationSeconds(facts); ok && rate > 0 && strings.HasPrefix(stream.format, "AAC") {
		setMPEGTSFactIfMissing(facts, "SamplingCount", strconv.FormatInt(int64(math.Round(duration*rate)), 10))
	}
}

// valuesDurationSeconds reads canonical milliseconds as seconds for count derivation.
func valuesDurationSeconds(facts *canonicalStructuredFacts) (float64, bool) {
	milliseconds, err := strconv.ParseFloat(facts.Canonical("Duration"), 64)
	return milliseconds / 1000, err == nil && milliseconds > 0
}

// canonicalMPEGTSStructuredValue converts serializer seconds to canonical milliseconds.
func canonicalMPEGTSStructuredValue(name fieldName, value string) string {
	switch name {
	case "Duration", "Source_Duration", "Source_Duration_LastFrame",
		"Duration_Start2End", "Duration_Start_Command", "Duration_Start", "Duration_End", "Duration_End_Command":
		if milliseconds, ok := decimalSecondsToMilliseconds(value); ok {
			return milliseconds
		}
	}
	return value
}

// canonicalEncodedLibrary normalizes x264/x265 library strings for structured output.
func canonicalEncodedLibrary(value string) string {
	if strings.HasPrefix(value, "x264 ") && !strings.HasPrefix(value, "x264 - ") {
		return "x264 - " + strings.TrimPrefix(value, "x264 ")
	}
	if strings.HasPrefix(value, "x265 ") && !strings.HasPrefix(value, "x265 - ") {
		return "x265 - " + strings.TrimPrefix(value, "x265 ")
	}
	return value
}

// setMPEGTSFactIfMissing retains the first non-empty parser fact for key.
func setMPEGTSFactIfMissing(facts *canonicalStructuredFacts, key, value string) {
	if facts == nil || key == "" || value == "" || facts.Canonical(fieldName(key)) != "" {
		return
	}
	facts.SetCanonical(fieldName(key), value)
}

// mpegTSStructuredTextLabel maps structured TS keys to legacy display labels.
func mpegTSStructuredTextLabel(name fieldName) string {
	switch name {
	case "ID":
		return "ID"
	case "MenuID":
		return "Menu ID"
	case "Format":
		return "Format"
	case "Format_Profile":
		return "Format profile"
	case "Format_Tier":
		return "Format tier"
	case "Format_Level":
		return "Format level"
	case "Format_Version":
		return "Format version"
	case "Format_Settings_CABAC":
		return "Format settings, CABAC"
	case "Format_Settings_BVOP":
		return "Format settings, BVOP"
	case "Format_Settings_Matrix":
		return "Format settings, Matrix"
	case "Format_Settings_GOP":
		return "Format settings, GOP"
	case "Format_Settings_PictureStructure":
		return "Format settings, Picture structure"
	case "Format_Settings_RefFrames":
		return "Format settings, Reference frames"
	case "CodecID":
		return "Codec ID"
	case "Format_Commercial_IfAny":
		return "Commercial name"
	case "MuxingMode":
		return "Muxing mode"
	case "MuxingMode_MoreInfo":
		return "Muxing mode, more info"
	case "Duration":
		return "Duration"
	case "Duration_Start2End":
		return "Duration of the visible content"
	case "Duration_Start":
		return "Start time"
	case "Duration_End":
		return "End time"
	case "BitRate_Mode":
		return "Bit rate mode"
	case "BitRate":
		return "Bit rate"
	case "BitRate_Nominal":
		return "Nominal bit rate"
	case "BitRate_Maximum":
		return "Maximum bit rate"
	case "FrameRate":
		return "Frame rate"
	case "Width":
		return "Width"
	case "Height":
		return "Height"
	case "DisplayAspectRatio":
		return "Display aspect ratio"
	case "ChromaSubsampling":
		return "Chroma subsampling"
	case "ChromaSubsampling_Position":
		return "Chroma subsampling position"
	case "ColorSpace":
		return "Color space"
	case "BitDepth":
		return "Bit depth"
	case "ScanType":
		return "Scan type"
	case "ScanOrder":
		return "Scan order"
	case "StreamSize":
		return "Stream size"
	case "Encoded_Library":
		return "Writing library"
	case "Encoded_Library_Settings":
		return "Encoding settings"
	case "Language":
		return "Language"
	case "Channels":
		return "Channel(s)"
	case "ChannelLayout":
		return "Channel layout"
	case "SamplingRate":
		return "Sampling rate"
	case "Compression_Mode":
		return "Compression mode"
	case "TimeCode_FirstFrame":
		return "Time code of first frame"
	case "TimeCode_Source":
		return "Time code source"
	case "Gop_OpenClosed":
		return "GOP, Open/Closed"
	case "Gop_OpenClosed_FirstFrame":
		return "GOP, Open/Closed of first frame"
	case "Standard":
		return "Standard"
	case "ServiceKind":
		return "Service kind"
	case "ServiceName":
		return "Service name"
	case "ServiceProvider":
		return "Service provider"
	case "ServiceType":
		return "Service type"
	case "FirstDisplay_Delay_Frames":
		return "Count of frames before first event"
	case "FirstDisplay_Type":
		return "Type of the first event"
	case "Title":
		return "Title"
	case "LawRating":
		return "Law rating"
	default:
		return ""
	}
}

// appendMissingMPEGTSCanonicalText retains display-only and repeated TS fields.
func appendMissingMPEGTSCanonicalText(builder *canonicalStreamBuilder, fields []Field) {
	if builder == nil || builder.store == nil {
		return
	}
	stream := builder.store.stream(builder.ref)
	for _, field := range fields {
		found := false
		for _, entry := range stream.Fields {
			if entry.Options.ShowText && entry.TextLabel == field.Name && entry.Value.Text == field.Value {
				found = true
				break
			}
		}
		if !found {
			builder.Text(field.Name, field.Value)
		}
	}
}
