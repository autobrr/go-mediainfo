package mediainfo

import (
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	mp4CodecProbeSamples = 32
	mp4CodecProbeBytes   = 16 << 20
)

// mp4AVCProbe contains bounded sample-derived AVC metadata.
type mp4AVCProbe struct {
	writingLibrary string
	settings       string
	timeCode       string
	gopM           int
	gopN           int
	hasGOP         bool
}

// readMP4SampleHead reads a bounded prefix of samples by combining stsc, stco
// or co64, and stsz. It does not assume adjacent chunks belong to one track.
func readMP4SampleHead(reader io.ReaderAt, track MP4Track, limit int) [][]byte {
	if reader == nil || limit <= 0 || len(track.SampleSizeHead) == 0 || len(track.chunkOffsetsHead) == 0 {
		return nil
	}
	limit = min(limit, len(track.SampleSizeHead))
	result := make([][]byte, 0, limit)
	sampleIndex := 0
	totalBytes := 0
	for chunkIndex, chunkOffset := range track.chunkOffsetsHead {
		if sampleIndex >= limit || totalBytes >= mp4CodecProbeBytes {
			break
		}
		samplesPerChunk := mp4SamplesPerChunk(track.sampleToChunk, uint32(chunkIndex+1))
		if samplesPerChunk == 0 {
			if chunkIndex > 0 {
				break
			}
			samplesPerChunk = uint32(limit)
		}
		offset := chunkOffset
		for index := uint32(0); index < samplesPerChunk && sampleIndex < limit; index++ {
			size := int(track.SampleSizeHead[sampleIndex])
			sampleIndex++
			if size <= 0 {
				continue
			}
			if size > mp4CodecProbeBytes-totalBytes {
				return result
			}
			payload := make([]byte, size)
			if _, err := reader.ReadAt(payload, int64(offset)); err != nil && err != io.EOF {
				return result
			}
			result = append(result, payload)
			totalBytes += size
			offset += uint64(size)
		}
	}
	return result
}

// mp4SamplesPerChunk resolves the stsc run active for one one-based chunk.
func mp4SamplesPerChunk(entries []mp4SampleToChunkEntry, chunk uint32) uint32 {
	var result uint32
	for _, entry := range entries {
		if entry.firstChunk > chunk {
			break
		}
		result = entry.samplesPerChunk
	}
	return result
}

// probeMP4AC3 accumulates the first MediaInfo-sized AC-3 or E-AC-3 sample
// window, including multiple syncframes carried by one E-AC-3 sample.
func probeMP4AC3(reader io.ReaderAt, track MP4Track) (ac3Info, bool) {
	samples := readMP4SampleHead(reader, track, mp4CodecProbeSamples)
	var result ac3Info
	parsed := false
	for _, sample := range samples {
		for offset := 0; offset+7 <= len(sample); {
			var frame ac3Info
			var frameSize int
			var ok bool
			if track.sampleEntryType == "ec-3" {
				frame, frameSize, ok = parseEAC3FrameWithOptions(sample[offset:], true)
			} else {
				frame, frameSize, ok = parseAC3Frame(sample[offset:])
			}
			if !ok || frameSize <= 0 {
				break
			}
			result.mergeFrame(frame)
			parsed = true
			offset += frameSize
		}
	}
	return result, parsed
}

// probeMP4HEVC merges configuration-record SEI with a bounded prefix of
// length-prefixed HEVC samples.
func probeMP4HEVC(reader io.ReaderAt, track MP4Track) hevcHDRInfo {
	result := track.hevcSEI
	if track.hevcNALLengthSize <= 0 {
		return result
	}
	for _, sample := range readMP4SampleHead(reader, track, mp4CodecProbeSamples) {
		parseHEVCSampleHDR(sample, track.hevcNALLengthSize, &result)
		if result.hasMastering && result.maxCLL > 0 && result.maxFALL > 0 && result.x265Seen {
			break
		}
	}
	return result
}

// probeMP4AVC extracts x264 metadata, the first pic-timing clock timestamp,
// and GOP cadence from a bounded prefix of length-prefixed AVC samples.
func probeMP4AVC(reader io.ReaderAt, track MP4Track) mp4AVCProbe {
	result := mp4AVCProbe{}
	if track.avcNALLengthSize <= 0 {
		return result
	}
	annexB := append(make([]byte, 0, 1<<20), track.avcParameterSets...)
	x264Payloads := 0
	for _, sample := range readMP4SampleHead(reader, track, mp4SampleSizeHeadMax) {
		converted := h264LengthPrefixedToAnnexB(sample, track.avcNALLengthSize)
		if x264Payloads < 2 {
			if writingLibrary, settings := findLastX264Info(sample); writingLibrary != "" || settings != "" {
				result.writingLibrary = writingLibrary
				result.settings = settings
				x264Payloads++
			}
		}
		if result.timeCode == "" {
			result.timeCode = h264TimeCodeFromAnnexB(converted, track.avcSPS)
		}
		remaining := mp4CodecProbeBytes - len(annexB)
		if remaining <= 0 {
			break
		}
		if len(converted) > remaining {
			converted = converted[:remaining]
		}
		annexB = append(annexB, converted...)
	}
	result.gopM, result.gopN, result.hasGOP = inferH264GOP(annexB)
	return result
}

// applyMP4HEVCProbe projects stream- and container-derived HEVC encoder, HDR,
// and Dolby Vision metadata.
func applyMP4HEVCProbe(builder *canonicalStreamBuilder, hdr hevcHDRInfo, track MP4Track) {
	if builder == nil {
		return
	}
	if hdr.x265Library != "" {
		encoded := hdr.x265Library
		if strings.HasPrefix(encoded, "x265 ") && !strings.HasPrefix(encoded, "x265 - ") {
			encoded = "x265 - " + strings.TrimPrefix(encoded, "x265 ")
		}
		builder.Fill("Encoded_Library", encoded, "Writing library", hdr.x265Library)
		builder.Structured("Encoded_Library_Name", "x265")
		if _, version := splitEncodedLibrary(encoded); version != "" {
			builder.Structured("Encoded_Library_Version", version)
		}
		if hdr.x265Settings != "" {
			builder.Fill("Encoded_Library_Settings", hdr.x265Settings, "Encoding settings", hdr.x265Settings)
		}
	}
	if track.hasDolbyVision {
		cfg := track.dolbyVision
		formats := []string{"Dolby Vision"}
		compatibility := []string{dolbyVisionCompatibilityName(cfg.compatibilityID)}
		if track.hevcContainerMastering {
			formats = append(formats, "SMPTE ST 2086")
			compatibility = append(compatibility, "HDR10")
		}
		if hdr.hasMastering {
			formats = append(formats, "SMPTE ST 2086")
			compatibility = append(compatibility, "HDR10")
		}
		builder.Fill("HDR_Format", strings.Join(formats, " / "), "HDR format", strings.Join(formats, " / "))
		displayFormats := []string{formatDolbyVisionHDR(cfg)}
		for range len(formats) - 1 {
			displayFormats = append(displayFormats, "SMPTE ST 2086, Version HDR10, HDR10 compatible")
		}
		builder.ReplaceText("HDR format", strings.Join(displayFormats, " / "))
		builder.ReplaceText("Codec configuration box", "hvcC+dvvC")
		builder.Structured("HDR_Format_Version", fmt.Sprintf("%d.%d / ", cfg.versionMajor, cfg.versionMinor))
		builder.Structured("HDR_Format_Profile", fmt.Sprintf("%s.%02d / ", dolbyVisionProfilePrefix(cfg.profile), cfg.profile))
		builder.Structured("HDR_Format_Level", fmt.Sprintf("%02d / ", cfg.level))
		builder.Structured("HDR_Format_Settings", dolbyVisionLayers(cfg)+" / ")
		builder.Structured("HDR_Format_Compression", "None / ")
		builder.Structured("HDR_Format_Compatibility", strings.Join(compactStrings(compatibility), " / "))
		node := structuredObjectFromKVs([]jsonKV{{Key: "CodecConfigurationBox", Val: "hvcC+dvvC"}})
		builder.OverrideStructuredNode("extra", node)
	} else if hdr.hasMastering {
		builder.Fill("HDR_Format", "SMPTE ST 2086", "HDR format", "SMPTE ST 2086, HDR10 compatible")
		builder.Structured("HDR_Format_Compatibility", "HDR10")
	}
	if hdr.masteringPrimaries != "" {
		builder.Fill("MasteringDisplay_ColorPrimaries", hdr.masteringPrimaries, "Mastering display color primaries", hdr.masteringPrimaries)
		source := "Stream"
		if track.hevcContainerMastering {
			source = "Container / Stream"
		}
		builder.Structured("MasteringDisplay_ColorPrimaries_Source", source)
	}
	if hdr.hasMastering && hdr.masteringLuminanceMin >= 0 && hdr.masteringLuminanceMax > 0 {
		luminance := formatMasteringLuminance(hdr.masteringLuminanceMin, hdr.masteringLuminanceMax)
		builder.Fill("MasteringDisplay_Luminance", luminance, "Mastering display luminance", luminance)
		source := "Stream"
		if track.hevcContainerMastering {
			source = "Container / Stream"
		}
		builder.Structured("MasteringDisplay_Luminance_Source", source)
		builder.Structured("MasteringDisplay_Luminance_Min", formatHDRLuminance(hdr.masteringLuminanceMin))
		builder.Structured("MasteringDisplay_Luminance_Max", formatHDRLuminanceMaximum(hdr.masteringLuminanceMax))
	}
	if hdr.maxCLL > 0 {
		value := strconv.FormatUint(hdr.maxCLL, 10)
		builder.Fill("MaxCLL", value, "Maximum Content Light Level", value+" cd/m2")
		if track.hevcContainerCLL || track.hasDolbyVision {
			builder.Structured("MaxCLL_Source", "Container")
		} else {
			builder.Structured("MaxCLL_Source", "Stream")
		}
	}
	if hdr.maxFALL > 0 {
		value := strconv.FormatUint(hdr.maxFALL, 10)
		builder.Fill("MaxFALL", value, "Maximum Frame-Average Light Level", value+" cd/m2")
		if track.hevcContainerCLL || track.hasDolbyVision {
			builder.Structured("MaxFALL_Source", "Container")
		} else {
			builder.Structured("MaxFALL_Source", "Stream")
		}
	}
	if track.hevcContainerCLL && hdr.maxCLL == 0 {
		builder.Structured("MaxCLL_Source", "Container")
	}
	if track.hevcContainerCLL && hdr.maxFALL == 0 {
		builder.Structured("MaxFALL_Source", "Container")
	}
}

// compactStrings removes empty values while preserving order.
func compactStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

// applyMP4AC3Probe projects bounded AC-3 or E-AC-3 frame facts and returns the
// ordered JSON extra object.
func applyMP4AC3Probe(builder *canonicalStreamBuilder, facts *mp4StructuredFacts, info ac3Info, format string) structuredNode {
	if builder == nil || facts == nil {
		return structuredNode{}
	}
	if info.channels > 0 {
		builder.Fill("Channels", strconv.FormatUint(info.channels, 10), "Channel(s)", formatChannels(info.channels))
		facts.Set("Channels", strconv.FormatUint(info.channels, 10))
	}
	if info.layout != "" {
		layout := info.layout
		if info.channels == 1 {
			layout = "M"
		}
		builder.Fill("ChannelLayout", layout, "Channel layout", layout)
		facts.Set("ChannelLayout", layout)
	}
	if info.sampleRate > 0 {
		builder.Fill("SamplingRate", strconv.FormatInt(int64(info.sampleRate), 10), "Sampling rate", formatSampleRate(info.sampleRate))
		facts.Set("SamplingRate", strconv.FormatInt(int64(info.sampleRate), 10))
	}
	if info.frameRate > 0 {
		value := formatJSONFloat(info.frameRate)
		builder.Fill("FrameRate", value, "Frame rate", formatAudioFrameRate(info.frameRate, info.spf))
		facts.Set("FrameRate", value)
	}
	if info.spf > 0 {
		value := strconv.Itoa(info.spf)
		builder.Structured("SamplesPerFrame", value)
		facts.Set("SamplesPerFrame", value)
	}
	commercial := "Dolby Digital"
	if format == "E-AC-3" {
		commercial = "Dolby Digital Plus"
	}
	builder.Fill("Format_Commercial_IfAny", commercial, "Commercial name", commercial)
	builder.Structured("Format_Settings_Endianness", "Big")
	facts.Set("Format_Settings_Endianness", "Big")
	if code := ac3ServiceKindCode(info.bsmod); code != "" {
		facts.Set("ServiceKind", code)
		builder.Structured("ServiceKind", code)
	}
	applyAVIAC3Text(builder, info)
	extraFields := matroskaAC3CanonicalExtraFields(&matroskaAudioProbe{format: format}, info, false, "")
	return structuredObjectFromKVs(extraFields)
}

// buildMP4ChapterMenu decodes a tref/chap text track as a Menu stream.
func buildMP4ChapterMenu(reader io.ReaderAt, track MP4Track) Stream {
	builder := newCanonicalStreamBuilder(StreamMenu)
	if track.ID > 0 {
		value := strconv.FormatUint(uint64(track.ID), 10)
		builder.Fill("ID", value, "ID", value)
	}
	builder.Fill("Format", "Timed Text", "Format", "Timed Text")
	builder.Fill("CodecID", track.sampleEntryType, "Codec ID", track.sampleEntryType)
	if track.DurationSeconds > 0 {
		builder.Fill("Duration", strconv.FormatFloat(track.DurationSeconds*1000, 'f', -1, 64), "Duration", formatDuration(track.DurationSeconds))
		builder.OverrideStructured("Duration", formatJSONSeconds(track.DurationSeconds))
	}
	extras := make([]jsonKV, 0, len(track.sampleStartsHead)+3)
	if encoded := formatMP4UTCTime(track.CreationTime); encoded != "" {
		builder.Text("Encoded_Date", encoded)
		extras = append(extras, jsonKV{Key: "Encoded_Date", Val: encoded})
	}
	if tagged := formatMP4UTCTime(track.ModificationTime); tagged != "" {
		builder.Text("Tagged_Date", tagged)
		extras = append(extras, jsonKV{Key: "Tagged_Date", Val: tagged})
	}
	if track.menuForTrackID > 0 {
		value := strconv.FormatUint(uint64(track.menuForTrackID), 10)
		builder.Text("Menu For", value)
		extras = append(extras, jsonKV{Key: "Menu_For", Val: value})
	}
	samples := readMP4SampleHead(reader, track, len(track.sampleStartsHead))
	for index, sample := range samples {
		if index >= len(track.sampleStartsHead) || len(sample) < 2 {
			break
		}
		length := int(binary.BigEndian.Uint16(sample[:2]))
		if length <= 0 || length > len(sample)-2 {
			continue
		}
		title := string(sample[2 : 2+length])
		if track.Timescale == 0 {
			continue
		}
		startMs := int64(track.sampleStartsHead[index] * 1000 / uint64(track.Timescale))
		builder.Text(formatMP4ChapterTimeText(startMs), title)
		extras = append(extras, jsonKV{Key: "_" + formatMP4ChapterTimeKey(startMs), Val: title})
	}
	node := structuredObjectFromKVs(extras)
	builder.StructuredNode("extra", node)
	return builder.Snapshot(canonicalStreamPolicy{})
}
