package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// SampleInfo contains codec and sample-table facts collected from an MP4 stbl.
type SampleInfo struct {
	// Format is the codec format inferred from stsd.
	Format string
	// Fields contains sample-entry display metadata.
	Fields []Field
	// SampleCount is the number of samples described by timing tables.
	SampleCount uint64
	// SampleBytes is the total byte count described by sample sizes.
	SampleBytes uint64
	// SampleSizeHead retains bounded leading sample sizes.
	SampleSizeHead []uint32
	// SampleSizeTail retains bounded trailing sample sizes.
	SampleSizeTail []uint32
	// SampleDelta is the dominant stts delta.
	SampleDelta         uint32
	sampleDurationTicks uint64
	// LastSampleDelta is the final stts delta.
	LastSampleDelta uint32
	// VariableDeltas reports whether stts contains differing deltas.
	VariableDeltas bool
	// MinimumSampleDelta is the shortest stts delta.
	MinimumSampleDelta uint32
	// MaximumSampleDelta is the longest stts delta.
	MaximumSampleDelta uint32
	// FirstChunkOff is the first absolute media chunk offset.
	FirstChunkOff uint64
	// Width is the sample entry width in pixels.
	Width uint64
	// Height is the sample entry height in pixels.
	Height        uint64
	canonicalSeed []fieldEntry
	// SampleEntryType is the four-character stsd sample-entry code.
	SampleEntryType string
	// NonEmptySampleCount counts samples whose payload exceeds an empty tx3g record.
	NonEmptySampleCount    uint64
	chunkOffsetsHead       []uint64
	sampleToChunk          []mp4SampleToChunkEntry
	hevcNALLengthSize      int
	avcNALLengthSize       int
	avcSPS                 h264SPSInfo
	avcParameterSets       []byte
	hevcSEI                hevcHDRInfo
	sampleStartsHead       []uint64
	dolbyVision            dolbyVisionConfig
	hasDolbyVision         bool
	hevcContainerMastering bool
	hevcContainerCLL       bool
}

// sampleEntryResult contains the canonical and compatibility metadata decoded
// from one visual or audio sample entry.
type sampleEntryResult struct {
	Fields                 []Field
	Format                 string
	Width                  uint64
	Height                 uint64
	canonicalSeed          []fieldEntry
	nalLengthSize          int
	avcNALLengthSize       int
	avcSPS                 h264SPSInfo
	avcParameterSets       []byte
	hevcSEI                hevcHDRInfo
	dolbyVision            dolbyVisionConfig
	hasDolbyVision         bool
	hevcContainerMastering bool
	hevcContainerCLL       bool
}

// mp4VisualCanonicalFacts carries raw codec configuration values from the MP4
// sample-entry parser to its canonical seed builder.
type mp4VisualCanonicalFacts struct {
	sampleType   string
	width        uint64
	height       uint64
	hasAVC       bool
	avc          avcConfigInfo
	hasHEVC      bool
	hevc         hevcConfigInfo
	sps          h264SPSInfo
	x265Library  string
	x265Settings string
}

// parseStsdForSample parses the first supported stsd entry and returns its
// codec metadata and canonical seed.
func parseStsdForSample(buf []byte) (SampleInfo, bool) {
	if len(buf) < 16 {
		return SampleInfo{}, false
	}
	count := binary.BigEndian.Uint32(buf[4:8])
	offset := 8
	for i := 0; i < int(count); i++ {
		if offset+8 > len(buf) {
			return SampleInfo{}, false
		}
		size := int(binary.BigEndian.Uint32(buf[offset : offset+4]))
		if size < 8 || offset+size > len(buf) {
			return SampleInfo{}, false
		}
		entry := buf[offset : offset+size]
		typ := string(entry[4:8])
		format := mapMP4SampleEntry(typ)
		info := SampleInfo{Format: format, SampleEntryType: typ}
		if isVideoSampleEntry(typ) {
			result := parseVisualSampleEntry(entry, typ)
			info.Fields = append(info.Fields, result.Fields...)
			if result.Format != "" {
				info.Format = result.Format
			}
			if result.Width > 0 {
				info.Width = result.Width
			}
			if result.Height > 0 {
				info.Height = result.Height
			}
			info.canonicalSeed = append(info.canonicalSeed, result.canonicalSeed...)
			info.hevcNALLengthSize = result.nalLengthSize
			info.avcNALLengthSize = result.avcNALLengthSize
			info.avcSPS = result.avcSPS
			info.avcParameterSets = append([]byte(nil), result.avcParameterSets...)
			info.hevcSEI = result.hevcSEI
			info.dolbyVision = result.dolbyVision
			info.hasDolbyVision = result.hasDolbyVision
			info.hevcContainerMastering = result.hevcContainerMastering
			info.hevcContainerCLL = result.hevcContainerCLL
		}
		if isAudioSampleEntry(typ) {
			result := parseAudioSampleEntry(entry, typ)
			info.Fields = append(info.Fields, result.Fields...)
			if result.Format != "" {
				info.Format = result.Format
			}
			info.canonicalSeed = append(info.canonicalSeed, result.canonicalSeed...)
		}
		if info.Format != "" || len(info.Fields) > 0 {
			return info, true
		}
		offset += size
	}
	return SampleInfo{}, false
}

// mapMP4SampleEntry maps a supported MP4 sample-entry code to its display
// codec format.
func mapMP4SampleEntry(sample string) string {
	switch sample {
	case "av01":
		return "AV1"
	case "avc1", "avc3":
		return "AVC"
	case "hvc1", "hev1":
		return "HEVC"
	case "mp4v":
		return "MPEG-4 Visual"
	case "vp09":
		return "VP9"
	case "mp4a":
		return "AAC"
	case "ac-3", "ac-4":
		return "AC-3"
	case "ec-3":
		return "E-AC-3"
	case "alac":
		return "ALAC"
	case "flac":
		return "FLAC"
	case "Opus", "opus":
		return "Opus"
	case "mp4s":
		return "MPEG-4 Systems"
	case "tx3g", "text":
		return "Timed Text"
	case "wvtt":
		return "WebVTT"
	default:
		return ""
	}
}

// isVideoSampleEntry reports whether sample is a supported MP4 video entry.
func isVideoSampleEntry(sample string) bool {
	switch sample {
	case "av01", "avc1", "avc3", "hvc1", "hev1", "mp4v", "vp09":
		return true
	default:
		return false
	}
}

// isAudioSampleEntry reports whether sample is a supported MP4 audio entry.
func isAudioSampleEntry(sample string) bool {
	switch sample {
	case "mp4a", "ac-3", "ec-3", "alac", "flac", "Opus", "opus":
		return true
	default:
		return false
	}
}

// parseVisualSampleEntry decodes dimensions and codec-private metadata from
// one visual sample entry.
func parseVisualSampleEntry(entry []byte, sampleType string) sampleEntryResult {
	if len(entry) < 36 {
		return sampleEntryResult{}
	}
	width := binary.BigEndian.Uint16(entry[32:34])
	height := binary.BigEndian.Uint16(entry[34:36])
	canonicalFacts := mp4VisualCanonicalFacts{sampleType: sampleType, width: uint64(width), height: uint64(height)}
	fields := []Field{
		{Name: "Codec ID", Value: sampleType},
	}
	structuredFacts := &mp4StructuredFacts{}
	if formatInfo := mapVideoFormatInfo(sampleType); formatInfo != "" {
		fields = append(fields, Field{Name: "Format/Info", Value: formatInfo})
	}
	if width > 0 {
		fields = append(fields, Field{Name: "Width", Value: formatPixels(uint64(width))})
	}
	if height > 0 {
		fields = append(fields, Field{Name: "Height", Value: formatPixels(uint64(height))})
	}
	if width > 0 && height > 0 {
		if ar := formatAspectRatio(uint64(width), uint64(height)); ar != "" {
			fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
		}
	}
	if compressor := parseMP4VisualCompressorName(entry); compressor != "" {
		fields = appendFieldUnique(fields, Field{Name: "Writing library", Value: compressor})
	}
	var spsInfo h264SPSInfo
	var hevcSEI hevcHDRInfo
	if sampleType == "avc1" || sampleType == "avc3" {
		if payload, ok := findMP4ChildBox(entry, mp4VisualSampleEntryHeaderSize, "avcC"); ok {
			_, avcFields, parsedSPS, avcInfo := parseAVCConfigDetails(payload)
			spsInfo = parsedSPS
			canonicalFacts.hasAVC = true
			canonicalFacts.avc = avcInfo
			canonicalFacts.sps = parsedSPS
			fields = append(fields, avcFields...)
			fields = append(fields, Field{Name: "Codec configuration box", Value: "avcC"})
			// Stored dimensions: mediainfo reports a macroblock-aligned Stored_Height for AVC.
			storedHeight := spsInfo.CodedHeight
			if storedHeight == 0 && height > 0 {
				storedHeight = uint64(height)
				if storedHeight%16 != 0 {
					storedHeight = ((storedHeight + 15) / 16) * 16
				}
			}
			if storedHeight > 0 && uint64(height) > 0 && storedHeight != uint64(height) {
				structuredFacts.Set("Stored_Height", strconv.FormatUint(storedHeight, 10))
			}
			if spsInfo.CodedWidth > 0 && uint64(width) > 0 && spsInfo.CodedWidth != uint64(width) {
				structuredFacts.Set("Stored_Width", strconv.FormatUint(spsInfo.CodedWidth, 10))
			}
			if spsInfo.HasColorRange || spsInfo.HasColorDescription {
				colorSource := "Stream"
				structuredFacts.Set("colour_description_present", "Yes")
				structuredFacts.Set("colour_description_present_Source", colorSource)
				if spsInfo.ColorRange != "" {
					structuredFacts.Set("colour_range", spsInfo.ColorRange)
					structuredFacts.Set("colour_range_Source", colorSource)
				}
				if spsInfo.ColorPrimaries != "" {
					structuredFacts.Set("colour_primaries", spsInfo.ColorPrimaries)
					structuredFacts.Set("colour_primaries_Source", colorSource)
				}
				if spsInfo.TransferCharacteristics != "" {
					structuredFacts.Set("transfer_characteristics", spsInfo.TransferCharacteristics)
					structuredFacts.Set("transfer_characteristics_Source", colorSource)
				}
				if spsInfo.MatrixCoefficients != "" {
					structuredFacts.Set("matrix_coefficients", spsInfo.MatrixCoefficients)
					structuredFacts.Set("matrix_coefficients_Source", colorSource)
				}
			}
			if spsInfo.HasSAR && spsInfo.SARWidth > 0 && spsInfo.SARHeight > 0 && spsInfo.SARWidth != spsInfo.SARHeight && width > 0 && height > 0 {
				par := float64(spsInfo.SARWidth) / float64(spsInfo.SARHeight)
				structuredFacts.Set("PixelAspectRatio", formatJSONFloat(par))
				structuredFacts.Set("DisplayAspectRatio_Original", formatJSONFloat((float64(width)/float64(height))*par))
			}
			if spsInfo.CodedWidth > uint64(width) && width > 0 && height > 0 {
				structuredFacts.Set("PixelAspectRatio", "1.000")
				structuredFacts.Set("DisplayAspectRatio", formatJSONFloat(float64(width)/float64(height)))
			}
		} else if payload, ok := findMP4BoxByName(entry, "avcC"); ok {
			_, avcFields, parsedSPS, avcInfo := parseAVCConfigDetails(payload)
			spsInfo = parsedSPS
			canonicalFacts.hasAVC = true
			canonicalFacts.avc = avcInfo
			canonicalFacts.sps = parsedSPS
			fields = append(fields, avcFields...)
			fields = append(fields, Field{Name: "Codec configuration box", Value: "avcC"})
			storedHeight := spsInfo.CodedHeight
			if storedHeight == 0 && height > 0 {
				storedHeight = uint64(height)
				if storedHeight%16 != 0 {
					storedHeight = ((storedHeight + 15) / 16) * 16
				}
			}
			if storedHeight > 0 && uint64(height) > 0 && storedHeight != uint64(height) {
				structuredFacts.Set("Stored_Height", strconv.FormatUint(storedHeight, 10))
			}
			if spsInfo.CodedWidth > 0 && uint64(width) > 0 && spsInfo.CodedWidth != uint64(width) {
				structuredFacts.Set("Stored_Width", strconv.FormatUint(spsInfo.CodedWidth, 10))
			}
			if spsInfo.HasColorRange || spsInfo.HasColorDescription {
				colorSource := "Stream"
				structuredFacts.Set("colour_description_present", "Yes")
				structuredFacts.Set("colour_description_present_Source", colorSource)
				if spsInfo.ColorRange != "" {
					structuredFacts.Set("colour_range", spsInfo.ColorRange)
					structuredFacts.Set("colour_range_Source", colorSource)
				}
				if spsInfo.ColorPrimaries != "" {
					structuredFacts.Set("colour_primaries", spsInfo.ColorPrimaries)
					structuredFacts.Set("colour_primaries_Source", colorSource)
				}
				if spsInfo.TransferCharacteristics != "" {
					structuredFacts.Set("transfer_characteristics", spsInfo.TransferCharacteristics)
					structuredFacts.Set("transfer_characteristics_Source", colorSource)
				}
				if spsInfo.MatrixCoefficients != "" {
					structuredFacts.Set("matrix_coefficients", spsInfo.MatrixCoefficients)
					structuredFacts.Set("matrix_coefficients_Source", colorSource)
				}
			}
			if spsInfo.HasSAR && spsInfo.SARWidth > 0 && spsInfo.SARHeight > 0 && spsInfo.SARWidth != spsInfo.SARHeight && width > 0 && height > 0 {
				par := float64(spsInfo.SARWidth) / float64(spsInfo.SARHeight)
				structuredFacts.Set("PixelAspectRatio", formatJSONFloat(par))
				structuredFacts.Set("DisplayAspectRatio_Original", formatJSONFloat((float64(width)/float64(height))*par))
			}
			if spsInfo.CodedWidth > uint64(width) && width > 0 && height > 0 {
				structuredFacts.Set("PixelAspectRatio", "1.000")
				structuredFacts.Set("DisplayAspectRatio", formatJSONFloat(float64(width)/float64(height)))
			}
		}
		fields = appendFieldUnique(fields, Field{Name: "Color space", Value: "YUV"})
	} else if sampleType == "hvc1" || sampleType == "hev1" {
		var hevcFields []Field
		var hevcInfo hevcConfigInfo
		hevcFields, spsInfo, hevcInfo, hevcSEI = parseMP4HEVCSampleEntryDetails(entry, uint64(width), uint64(height), structuredFacts)
		canonicalFacts.hasHEVC = true
		canonicalFacts.hevc = hevcInfo
		canonicalFacts.sps = spsInfo
		canonicalFacts.x265Library = hevcSEI.x265Library
		canonicalFacts.x265Settings = hevcSEI.x265Settings
		fields = append(fields, hevcFields...)
	} else if sampleType == "vp09" {
		parseMP4VP9SampleEntry(entry, structuredFacts, &fields)
	}
	applyMP4ContainerColor(entry, structuredFacts)
	applyMP4ContainerColorText(&fields, structuredFacts)
	if sampleType == "vp09" {
		for _, key := range []fieldName{"colour_description_present_Source", "colour_primaries_Source", "transfer_characteristics_Source"} {
			if structuredFacts.Get(key) != "" {
				structuredFacts.Set(key, "Container")
			}
		}
	}
	// When AVC bitstream says "not fixed" but container timing is CFR, official MediaInfo keeps CFR
	// and reports the bitstream hint as FrameRate_Mode_Original=VFR.
	if spsInfo.HasFixedFrameRate && !spsInfo.FixedFrameRate {
		structuredFacts.Set("FrameRate_Mode_Original", "VFR")
	}
	if _, maxRate, avgRate, ok := parseBtrt(entry, mp4VisualSampleEntryHeaderSize); ok {
		bps := uint64(avgRate)
		if bps == 0 {
			bps = uint64(maxRate)
		}
		if bps > 0 {
			fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: formatBitrate(float64(bps))})
			// Match official JSON: btrt bitrate is emitted with exact b/s (text is rounded).
			structuredFacts.Set("BitRate", strconv.FormatUint(bps, 10))
		}
		// Official MediaInfo omits BitRate_Maximum when it equals the average bitrate.
		if avgRate > 0 && maxRate > avgRate {
			fields = appendFieldUnique(fields, Field{Name: "Maximum bit rate", Value: formatBitrate(float64(maxRate))})
			// Match official JSON: btrt max bitrate is emitted with exact b/s (text is rounded).
			structuredFacts.Set("BitRate_Maximum", strconv.FormatUint(uint64(maxRate), 10))
		}
	}
	if info := mapVideoCodecIDInfo(sampleType); info != "" {
		fields = append(fields, Field{Name: "Codec ID/Info", Value: info})
	}
	result := sampleEntryResult{
		Fields:        fields,
		Width:         uint64(width),
		Height:        uint64(height),
		canonicalSeed: canonicalMP4VisualSampleSeed(fields, structuredFacts, canonicalFacts),
	}
	if canonicalFacts.hasHEVC {
		result.nalLengthSize = canonicalFacts.hevc.nalLengthSize
		result.hevcSEI = hevcSEI
		if payload := findDolbyVisionConfig(entry); len(payload) > 0 {
			result.dolbyVision, result.hasDolbyVision = parseDolbyVisionConfig(payload)
		}
		_, result.hevcContainerMastering = findMP4BoxByName(entry, "mdcv")
		_, result.hevcContainerCLL = findMP4BoxByName(entry, "clli")
	} else if canonicalFacts.hasAVC {
		result.avcNALLengthSize = canonicalFacts.avc.nalLengthSize
		result.avcSPS = canonicalFacts.sps
		result.avcParameterSets = append([]byte(nil), canonicalFacts.avc.parameterSets...)
	}
	return result
}

// applyMP4ContainerColorText projects container-selected colour values and
// preserves conflicting bitstream values as raw-text Original fields.
func applyMP4ContainerColorText(fields *[]Field, facts *mp4StructuredFacts) {
	if fields == nil || facts == nil {
		return
	}
	pairs := []struct {
		key   fieldName
		label string
	}{
		{key: "colour_range", label: "Color range"},
		{key: "colour_primaries", label: "Color primaries"},
		{key: "transfer_characteristics", label: "Transfer characteristics"},
		{key: "matrix_coefficients", label: "Matrix coefficients"},
	}
	for _, pair := range pairs {
		originalKey := fieldName(string(pair.key) + "_Original")
		original := facts.Get(originalKey)
		if original == "" {
			continue
		}
		if selected := facts.Get(pair.key); selected != "" {
			*fields = setFieldValue(*fields, pair.label, selected)
		}
		*fields = appendFieldUnique(*fields, Field{Name: string(originalKey), Value: original})
	}
}

// parseMP4VisualCompressorName decodes the Pascal compressor-name field in a
// visual sample entry.
func parseMP4VisualCompressorName(entry []byte) string {
	if len(entry) < 82 {
		return ""
	}
	length := min(int(entry[50]), 31)
	if length <= 0 || 51+length > len(entry) {
		return ""
	}
	return strings.TrimSpace(strings.TrimRight(string(entry[51:51+length]), "\x00"))
}

// parseMP4VP9SampleEntry projects vpcC codec and colour facts for a VP9 visual
// sample entry.
func parseMP4VP9SampleEntry(entry []byte, facts *mp4StructuredFacts, fields *[]Field) {
	payload, ok := findMP4ChildBox(entry, mp4VisualSampleEntryHeaderSize, "vpcC")
	if !ok {
		payload, ok = findMP4BoxByName(entry, "vpcC")
	}
	if ok && len(payload) >= 12 {
		profile := payload[4]
		packed := payload[6]
		bitDepth := packed >> 4
		chroma := (packed >> 1) & 0x07
		facts.Set("Format_Profile", strconv.Itoa(int(profile)))
		if bitDepth > 0 {
			facts.Set("BitDepth", strconv.Itoa(int(bitDepth)))
			*fields = appendFieldUnique(*fields, Field{Name: "Bit depth", Value: formatBitDepth(bitDepth)})
		}
		chromaSubsampling := ""
		switch chroma {
		case 0, 1:
			chromaSubsampling = "4:2:0"
		case 2:
			chromaSubsampling = "4:2:2"
		case 3:
			chromaSubsampling = "4:4:4"
		}
		if chromaSubsampling != "" {
			facts.Set("ChromaSubsampling", chromaSubsampling)
			*fields = appendFieldUnique(*fields, Field{Name: "Chroma subsampling", Value: chromaSubsampling})
		}
		facts.Set("ColorSpace", "YUV")
		*fields = appendFieldUnique(*fields, Field{Name: "Color space", Value: "YUV"})
		streamColor := mp4ColorInfo{
			primaries: mp4ColorPrimariesName(binary.BigEndian.Uint16([]byte{0, payload[7]})),
			transfer:  mp4TransferName(binary.BigEndian.Uint16([]byte{0, payload[8]})),
			matrix:    mp4MatrixName(binary.BigEndian.Uint16([]byte{0, payload[9]})),
			hasRange:  true,
		}
		if packed&0x01 != 0 {
			streamColor.rangeName = "Full"
		} else {
			streamColor.rangeName = "Limited"
		}
		applyMP4ColorFacts(facts, streamColor, "Stream")
	}
	if len(entry) >= 36 {
		width := uint64(binary.BigEndian.Uint16(entry[32:34]))
		height := uint64(binary.BigEndian.Uint16(entry[34:36]))
		if width > 0 && height > 0 {
			facts.Set("PixelAspectRatio", "1.000")
			ratio := float64(width) / float64(height)
			facts.Set("DisplayAspectRatio", formatJSONFloat(ratio))
			*fields = setFieldValue(*fields, "Display aspect ratio", strconv.FormatFloat(ratio, 'f', 3, 64))
		}
	}
}

// mp4ColorInfo contains colour-description fields decoded from colr or codec
// configuration boxes.
type mp4ColorInfo struct {
	primaries string
	transfer  string
	matrix    string
	rangeName string
	hasRange  bool
}

// applyMP4ContainerColor merges a visual sample entry's nclc/nclx colr atom
// with any stream-derived colour facts already present.
func applyMP4ContainerColor(entry []byte, facts *mp4StructuredFacts) {
	payload, ok := findMP4ChildBox(entry, mp4VisualSampleEntryHeaderSize, "colr")
	if !ok {
		payload, ok = findMP4BoxByName(entry, "colr")
	}
	if !ok || len(payload) < 10 {
		for _, key := range []string{"colour_description_present", "colour_range", "colour_primaries", "transfer_characteristics", "matrix_coefficients"} {
			if facts.Get(fieldName(key)) != "" {
				facts.Set(fieldName(key+"_Source"), "Stream")
			}
		}
		return
	}
	typeName := string(payload[:4])
	if typeName != "nclc" && typeName != "nclx" {
		return
	}
	container := mp4ColorInfo{
		primaries: mp4ColorPrimariesName(binary.BigEndian.Uint16(payload[4:6])),
		transfer:  mp4TransferName(binary.BigEndian.Uint16(payload[6:8])),
		matrix:    mp4MatrixName(binary.BigEndian.Uint16(payload[8:10])),
	}
	if typeName == "nclx" && len(payload) >= 11 {
		container.hasRange = true
		if payload[10]&0x80 != 0 {
			container.rangeName = "Full"
		} else {
			container.rangeName = "Limited"
		}
	}
	mergeMP4ContainerColorFact(facts, "colour_primaries", container.primaries)
	mergeMP4ContainerColorFact(facts, "transfer_characteristics", container.transfer)
	mergeMP4ContainerColorFact(facts, "matrix_coefficients", container.matrix)
	if container.hasRange {
		mergeMP4ContainerColorFact(facts, "colour_range", container.rangeName)
	}
	if container.primaries != "" || container.transfer != "" || container.matrix != "" {
		mergeMP4ContainerColorFact(facts, "colour_description_present", "Yes")
	}
}

// mp4ColorPrimariesName uses MediaInfo's MP4 labels for ISO colour-primary IDs.
func mp4ColorPrimariesName(value uint16) string {
	if value == 0 || value == 2 {
		return ""
	}
	if value == 6 {
		return "BT.601 NTSC"
	}
	return matroskaColorPrimariesName(uint64(value))
}

// mp4TransferName uses MediaInfo's MP4 labels for ISO transfer IDs.
func mp4TransferName(value uint16) string {
	if value == 0 || value == 2 {
		return ""
	}
	return matroskaTransferName(uint64(value))
}

// mp4MatrixName uses MediaInfo's MP4 labels for ISO matrix IDs.
func mp4MatrixName(value uint16) string {
	if value == 0 || value == 2 {
		return ""
	}
	if value == 6 {
		return "BT.601"
	}
	return matroskaMatrixName(uint64(value))
}

// applyMP4ColorFacts records colour values from one source without replacing
// facts already supplied by a more authoritative container merge.
func applyMP4ColorFacts(facts *mp4StructuredFacts, color mp4ColorInfo, source string) {
	if color.primaries != "" {
		facts.Set("colour_primaries", color.primaries)
		facts.Set("colour_primaries_Source", source)
	}
	if color.transfer != "" {
		facts.Set("transfer_characteristics", color.transfer)
		facts.Set("transfer_characteristics_Source", source)
	}
	if color.matrix != "" {
		facts.Set("matrix_coefficients", color.matrix)
		facts.Set("matrix_coefficients_Source", source)
	}
	if color.hasRange && color.rangeName != "" {
		facts.Set("colour_range", color.rangeName)
		facts.Set("colour_range_Source", source)
	}
	if color.primaries != "" || color.transfer != "" || color.matrix != "" {
		facts.Set("colour_description_present", "Yes")
		facts.Set("colour_description_present_Source", source)
	}
}

// mergeMP4ContainerColorFact retains differing stream values as Original and
// records whether the selected value came from one or both sources.
func mergeMP4ContainerColorFact(facts *mp4StructuredFacts, key, containerValue string) {
	if containerValue == "" {
		return
	}
	streamValue := facts.Get(fieldName(key))
	switch streamValue {
	case "":
		facts.Set(fieldName(key), containerValue)
		facts.Set(fieldName(key+"_Source"), "Container")
	case containerValue:
		facts.Set(fieldName(key+"_Source"), "Container / Stream")
	default:
		facts.Set(fieldName(key+"_Original"), streamValue)
		facts.Set(fieldName(key+"_Original_Source"), "Stream")
		facts.Set(fieldName(key), containerValue)
		facts.Set(fieldName(key+"_Source"), "Container")
	}
}

// parseMP4HEVCSampleEntry parses the hvcC configuration box of an HEVC (hvc1/hev1)
// MP4 visual sample entry. It returns the stream fields (Format profile / tier,
// chroma subsampling, bit depth, codec configuration box, colour space,
// and the x265 writing library / encoding settings when present) plus the parsed
// SPS. Colour facts use a "Stream" source: unlike the AVC path (which merges a
// container colr atom and reports "Container / Stream"), HEVC MP4 streams here
// carry colour info only in the bitstream SPS VUI.
func parseMP4HEVCSampleEntry(entry []byte, width, height uint64, facts *mp4StructuredFacts) ([]Field, h264SPSInfo) {
	fields, spsInfo, _, _ := parseMP4HEVCSampleEntryDetails(entry, width, height, facts)
	return fields, spsInfo
}

// parseMP4HEVCSampleEntryDetails returns raw hvcC and SEI facts in addition to
// the compatibility fields exposed by parseMP4HEVCSampleEntry.
func parseMP4HEVCSampleEntryDetails(entry []byte, width, height uint64, facts *mp4StructuredFacts) ([]Field, h264SPSInfo, hevcConfigInfo, hevcHDRInfo) {
	payload, ok := findMP4ChildBox(entry, mp4VisualSampleEntryHeaderSize, "hvcC")
	if !ok {
		payload, ok = findMP4BoxByName(entry, "hvcC")
	}
	if !ok {
		return nil, h264SPSInfo{}, hevcConfigInfo{}, hevcHDRInfo{}
	}
	_, hevcFields, configInfo, spsInfo := parseHEVCConfig(payload)
	fields := append([]Field{}, hevcFields...)
	fields = append(fields, Field{Name: "Codec configuration box", Value: "hvcC"})
	fields = append(fields, Field{Name: "Color space", Value: "YUV"})

	// Stored (coded, pre-conformance-crop) dimensions are reported when they differ from
	// the displayed size, e.g. 1080-line HEVC is coded as 1088 luma samples.
	if spsInfo.CodedWidth > 0 && width > 0 && spsInfo.CodedWidth != width {
		facts.Set("Stored_Width", strconv.FormatUint(spsInfo.CodedWidth, 10))
	}
	if spsInfo.CodedHeight > 0 && height > 0 && spsInfo.CodedHeight != height {
		facts.Set("Stored_Height", strconv.FormatUint(spsInfo.CodedHeight, 10))
	}

	if spsInfo.HasColorRange && spsInfo.ColorRange != "" {
		facts.Set("colour_range", spsInfo.ColorRange)
		facts.Set("colour_range_Source", "Stream")
	}
	if spsInfo.HasColorDescription {
		facts.Set("colour_description_present", "Yes")
		facts.Set("colour_description_present_Source", "Stream")
		if spsInfo.ColorPrimaries != "" {
			facts.Set("colour_primaries", spsInfo.ColorPrimaries)
			facts.Set("colour_primaries_Source", "Stream")
		}
		if spsInfo.TransferCharacteristics != "" {
			facts.Set("transfer_characteristics", spsInfo.TransferCharacteristics)
			facts.Set("transfer_characteristics_Source", "Stream")
		}
		if spsInfo.MatrixCoefficients != "" {
			facts.Set("matrix_coefficients", spsInfo.MatrixCoefficients)
			facts.Set("matrix_coefficients_Source", "Stream")
		}
	}

	// x265 (and similar encoders with global headers) place the writing-library +
	// encoding-settings SEI in the hvcC NAL arrays rather than in frame data.
	var sei hevcHDRInfo
	parseHEVCConfigSEI(payload, &sei)
	if sei.x265Library != "" {
		fields = append(fields, Field{Name: "Writing library", Value: sei.x265Library})
		if sei.x265Settings != "" {
			fields = append(fields, Field{Name: "Encoding settings", Value: sei.x265Settings})
		}
	}
	return fields, spsInfo, configInfo, sei
}

// parseAudioSampleEntry decodes channel, sample-rate, codec, and bitrate facts
// from one audio sample entry.
func parseAudioSampleEntry(entry []byte, sampleType string) sampleEntryResult {
	if len(entry) < 36 {
		return sampleEntryResult{}
	}
	channels := binary.BigEndian.Uint16(entry[24:26])
	sampleRate := binary.BigEndian.Uint32(entry[32:36])
	codecID := sampleType
	fields := []Field{}
	structuredFacts := &mp4StructuredFacts{}
	canonicalBitRate := ""
	fields = appendChannelFields(fields, uint64(channels))
	if sampleRate > 0 {
		rate := float64(sampleRate) / 65536
		fields = appendSampleRateField(fields, rate)
		if sampleType == "mp4a" {
			frameRate := rate / 1024.0
			fields = append(fields, Field{Name: "Frame rate", Value: fmt.Sprintf("%.3f FPS (1024 SPF)", frameRate)})
		}
	}
	format := ""
	if sampleType == "mp4a" {
		if profile, codecIDValue, info, sbrExplicitNo := parseESDSProfile(entry); profile != "" {
			if info != "" {
				fields = append(fields, Field{Name: "Format/Info", Value: info})
			}
			if codecIDValue != "" {
				codecID = codecIDValue
			}
			format = "AAC " + profile
			if sbrExplicitNo {
				structuredFacts.Set("Format_Settings_SBR", "No (Explicit)")
			}
		} else if info := mapAudioFormatInfo(sampleType); info != "" {
			fields = append(fields, Field{Name: "Format/Info", Value: info})
		}
	} else if info := mapAudioFormatInfo(sampleType); info != "" {
		fields = append(fields, Field{Name: "Format/Info", Value: info})
	}
	if sampleType == "Opus" || sampleType == "opus" {
		codecID = "Opus"
		format = "Opus"
		fields = appendFieldUnique(fields, Field{Name: "Compression mode", Value: "Lossy"})
		if payload, ok := findMP4BoxByName(entry, "dOps"); ok && len(payload) >= 8 {
			if payload[1] > 0 {
				channels = uint16(payload[1])
				fields = setFieldValue(fields, "Channel(s)", formatChannels(uint64(channels)))
			}
			sampleRate = 48000 << 16
			fields = setFieldValue(fields, "Sampling rate", formatSampleRate(48000))
		}
	}
	if sampleType == "mp4a" {
		fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
	} else if sampleType == "ac-3" || sampleType == "ec-3" {
		fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
	}
	if sampleType == "mp4a" {
		// Prefer ESDS avgBitrate/maxBitrate (DecoderConfigDescriptor) over container-level btrt.
		if avg, max, ok := parseESDSBitrates(entry); ok {
			bps := avg
			if bps == 0 {
				bps = max
			}
			if bps > 0 {
				mode := "Constant"
				if avg > 0 && max > avg {
					mode = "Variable"
					fields = appendFieldUnique(fields, Field{Name: "Maximum bit rate", Value: formatBitrate(float64(max))})
					structuredFacts.Set("BitRate_Maximum", strconv.FormatUint(uint64(max), 10))
				} else {
					// Constant AAC rates are displayed and serialized at whole kb/s.
					bps = (bps / 1000) * 1000
				}
				fields = appendFieldUnique(fields, Field{Name: "Bit rate mode", Value: mode})
				fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: formatBitrate(float64(bps))})
				canonicalBitRate = strconv.FormatUint(uint64(bps), 10)
			}
		}
	}
	if _, maxRate, avgRate, ok := parseBtrt(entry, mp4AudioSampleEntryHeaderSize); ok {
		// AAC: if ESDS provided a bitrate, do not override/augment it with btrt.
		if sampleType == "mp4a" && findField(fields, "Bit rate") != "" {
			// keep btrt as fallback only
		} else {
			bps := uint64(avgRate)
			if bps == 0 {
				bps = uint64(maxRate)
			}
			if bps > 0 {
				fields = appendFieldUnique(fields, Field{Name: "Bit rate mode", Value: "Constant"})
				fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: formatBitrate(float64(bps))})
				// Match official JSON: btrt bitrate is emitted with exact b/s (text is rounded).
				structuredFacts.Set("BitRate", strconv.FormatUint(bps, 10))
			}
			// Official MediaInfo omits BitRate_Maximum when it equals the average bitrate.
			if avgRate > 0 && maxRate > avgRate {
				fields = appendFieldUnique(fields, Field{Name: "Maximum bit rate", Value: formatBitrate(float64(maxRate))})
				structuredFacts.Set("BitRate_Maximum", strconv.FormatUint(uint64(maxRate), 10))
			}
		}
	}
	fields = append(fields, Field{Name: "Codec ID", Value: codecID})
	return sampleEntryResult{
		Fields:        fields,
		Format:        format,
		canonicalSeed: canonicalMP4AudioSampleSeed(fields, structuredFacts, sampleType, codecID, canonicalBitRate, channels, sampleRate),
	}
}

// canonicalMP4VisualSampleSeed converts MP4 sample-entry and shared AVC/HEVC
// configuration facts to direct canonical entries.
func canonicalMP4VisualSampleSeed(fields []Field, structuredFacts *mp4StructuredFacts, facts mp4VisualCanonicalFacts) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamVideo)
	appendMP4TextOnlyFields(builder, StreamVideo, fields)
	builder.Fill("CodecID", facts.sampleType, "Codec ID", facts.sampleType)
	if facts.width > 0 {
		builder.Fill("Width", strconv.FormatUint(facts.width, 10), "Width", formatPixels(facts.width))
	}
	if facts.height > 0 {
		builder.Fill("Height", strconv.FormatUint(facts.height, 10), "Height", formatPixels(facts.height))
	}
	if ratio := structuredFacts.Get("DisplayAspectRatio"); ratio != "" {
		display := findField(fields, "Display aspect ratio")
		if display == "" {
			display = formatRawTextAspectRatio(ratio)
		}
		builder.Fill("DisplayAspectRatio", ratio, "Display aspect ratio", display)
	} else if facts.width > 0 && facts.height > 0 {
		if display := formatAspectRatio(facts.width, facts.height); display != "" {
			if ratio, ok := parseRatioFloat(display); ok {
				builder.Fill("DisplayAspectRatio", formatJSONFloat(ratio), "Display aspect ratio", display)
			}
		}
	}
	profile := ""
	level := ""
	chroma := ""
	bitDepth := 0
	if facts.hasAVC {
		profile = facts.avc.profile
		if profile == "High" && facts.sps.ConstraintFlags&0x08 != 0 {
			profile = "Progressive High"
		}
		level = strings.TrimPrefix(facts.avc.level, "L")
		chroma = facts.sps.ChromaFormat
		bitDepth = facts.sps.BitDepth
		if facts.sps.RefFrames > 0 {
			display := findField(fields, "Format settings, Reference frames")
			builder.Fill("Format_Settings_RefFrames", strconv.Itoa(facts.sps.RefFrames), "Format settings, Reference frames", display)
		}
		if facts.avc.cabac != nil {
			value := formatYesNo(*facts.avc.cabac)
			builder.Fill("Format_Settings_CABAC", value, "Format settings, CABAC", value)
		}
		if facts.sps.HasBitRate && facts.sps.BitRate > 0 {
			if facts.sps.HasBitRateCBR && facts.sps.BitRateCBR {
				structuredFacts.Set("BitRate", strconv.FormatInt(facts.sps.BitRate, 10))
				structuredFacts.Set("BitRate_Mode", "CBR")
			} else {
				structuredFacts.Set("BitRate_Maximum", strconv.FormatInt(facts.sps.BitRate, 10))
				structuredFacts.Set("BitRate_Mode", "VBR")
			}
		}
		if facts.sps.HasBufferSize && facts.sps.BufferSize > 0 {
			structuredFacts.Set("BufferSize", strconv.FormatInt(facts.sps.BufferSize, 10))
		}
	} else if facts.hasHEVC {
		profile = facts.hevc.profileName
		level = facts.hevc.levelName
		chroma = facts.hevc.chromaFormat
		bitDepth = int(facts.hevc.bitDepth)
		builder.Fill("Format_Tier", facts.hevc.tierName, "Format tier", facts.hevc.tierName)
	} else if value := structuredFacts.Get("ChromaSubsampling"); value != "" {
		chroma = value
	}
	if profile != "" {
		builder.Fill("Format_Profile", profile, "Format profile", findField(fields, "Format profile"))
		builder.Structured("Format_Level", level)
	}
	if colorSpace := findField(fields, "Color space"); colorSpace != "" {
		builder.Fill("ColorSpace", colorSpace, "Color space", colorSpace)
	}
	if chroma != "" {
		builder.Fill("ChromaSubsampling", chroma, "Chroma subsampling", findField(fields, "Chroma subsampling"))
	}
	if facts.sps.HasChromaLoc {
		value := fmt.Sprintf("Type %d", facts.sps.ChromaSampleLoc)
		builder.Fill("ChromaSubsampling_Position", value, "Chroma subsampling position", value)
	}
	if bitDepth > 0 {
		builder.Fill("BitDepth", strconv.Itoa(bitDepth), "Bit depth", formatBitDepth(uint8(bitDepth)))
	}
	if scanType := findField(fields, "Scan type"); scanType != "" {
		builder.Fill("ScanType", scanType, "Scan type", scanType)
	}
	if scanOrder := findField(fields, "Scan order"); scanOrder != "" {
		builder.Fill("ScanOrder", scanOrder, "Scan order", scanOrder)
	}
	if standard := findField(fields, "Standard"); standard != "" {
		builder.Fill("Standard", standard, "Standard", standard)
	}
	if bitRate := structuredFacts.Get("BitRate"); bitRate != "" {
		builder.Fill("BitRate", bitRate, "Bit rate", findField(fields, "Bit rate"))
	}
	if maximum := structuredFacts.Get("BitRate_Maximum"); maximum != "" {
		builder.Fill("BitRate_Maximum", maximum, "Maximum bit rate", findField(fields, "Maximum bit rate"))
	}
	if mode := structuredFacts.Get("BitRate_Mode"); mode != "" {
		display := map[string]string{"CBR": "Constant", "VBR": "Variable"}[mode]
		builder.Fill("BitRate_Mode", mode, "Bit rate mode", display)
	}
	if writingLibrary := firstNonEmpty(facts.x265Library, findField(fields, "Writing library")); writingLibrary != "" {
		encoded := writingLibrary
		if strings.HasPrefix(encoded, "x264 ") && !strings.HasPrefix(encoded, "x264 - ") {
			encoded = "x264 - " + strings.TrimPrefix(encoded, "x264 ")
		}
		if strings.HasPrefix(encoded, "x265 ") && !strings.HasPrefix(encoded, "x265 - ") {
			encoded = "x265 - " + strings.TrimPrefix(encoded, "x265 ")
		}
		builder.Fill("Encoded_Library", encoded, "Writing library", writingLibrary)
		if name, version := splitEncodedLibrary(encoded); name != "" {
			builder.Structured("Encoded_Library_Name", name)
			builder.Structured("Encoded_Library_Version", version)
		}
	}
	if settings := firstNonEmpty(facts.x265Settings, findField(fields, "Encoding settings")); settings != "" {
		builder.Fill("Encoded_Library_Settings", settings, "Encoding settings", settings)
	}
	structuredFacts.Apply(builder)
	if configuration := findField(fields, "Codec configuration box"); configuration != "" {
		builder.Text("Codec configuration box", configuration)
		node := structuredObjectFromKVs([]jsonKV{{Key: "CodecConfigurationBox", Val: configuration}})
		builder.OverrideStructuredNode("extra", node)
	}
	return builder.Snapshot(canonicalStreamPolicy{}).canonicalSeed
}

// canonicalMP4AudioSampleSeed converts audio sample-entry header facts to
// direct canonical entries while retaining text-only codec descriptions.
func canonicalMP4AudioSampleSeed(fields []Field, structuredFacts *mp4StructuredFacts, sampleType, codecID, canonicalBitRate string, channels uint16, sampleRate uint32) []fieldEntry {
	builder := newCanonicalStreamBuilder(StreamAudio)
	appendMP4TextOnlyFields(builder, StreamAudio, fields)
	if channels > 0 {
		raw := strconv.FormatUint(uint64(channels), 10)
		builder.Fill("Channels", raw, "Channel(s)", formatChannels(uint64(channels)))
		if layout := channelLayout(uint64(channels)); layout != "" {
			builder.Fill("ChannelLayout", layout, "Channel layout", layout)
		}
	}
	rate := float64(sampleRate) / 65536
	if rate > 0 {
		builder.Fill("SamplingRate", strconv.FormatInt(int64(math.Round(rate)), 10), "Sampling rate", formatSampleRate(rate))
		if sampleType == "mp4a" {
			frameRate := rate / 1024
			builder.Fill("FrameRate", formatJSONFloat(frameRate), "Frame rate", fmt.Sprintf("%.3f FPS (1024 SPF)", frameRate))
		}
	}
	if mode := findField(fields, "Bit rate mode"); mode != "" {
		builder.Fill("BitRate_Mode", mode, "Bit rate mode", mode)
	}
	if bitRate := firstNonEmpty(structuredFacts.Get("BitRate"), canonicalBitRate); bitRate != "" {
		builder.Fill("BitRate", bitRate, "Bit rate", findField(fields, "Bit rate"))
	}
	if maximum := structuredFacts.Get("BitRate_Maximum"); maximum != "" {
		builder.Fill("BitRate_Maximum", maximum, "Maximum bit rate", findField(fields, "Maximum bit rate"))
	}
	if compression := findField(fields, "Compression mode"); compression != "" {
		builder.Fill("Compression_Mode", compression, "Compression mode", compression)
	}
	builder.Fill("CodecID", codecID, "Codec ID", codecID)
	structuredFacts.Apply(builder)
	return builder.Snapshot(canonicalStreamPolicy{}).canonicalSeed
}

// appendMP4TextOnlyFields preserves sample-entry descriptions that have no
// structured schema representation.
func appendMP4TextOnlyFields(builder *canonicalStreamBuilder, kind StreamKind, fields []Field) {
	for _, field := range fields {
		if len(mapStreamFieldsToJSON(kind, []Field{field})) == 0 {
			builder.Text(field.Name, field.Value)
		}
	}
}

const (
	mp4VisualSampleEntryHeaderSize = 78
	mp4AudioSampleEntryHeaderSize  = 36
	mp4AudioSampleEntryHeaderAlt   = 28
)

func mapAVCProfile(profileID byte) string {
	switch profileID {
	case 66:
		return "Baseline"
	case 77:
		return "Main"
	case 88:
		return "Extended"
	case 100:
		return "High"
	case 110:
		return "High 10"
	case 122:
		return "High 4:2:2"
	case 244:
		return "High 4:4:4 Predictive"
	default:
		return ""
	}
}

func formatAVCLevel(levelID byte) string {
	if levelID == 0 {
		return ""
	}
	major := int(levelID) / 10
	minor := int(levelID) % 10
	if minor == 0 {
		return fmt.Sprintf("L%d", major)
	}
	return fmt.Sprintf("L%d.%d", major, minor)
}

func parseESDSProfile(entry []byte) (string, string, string, bool) {
	payload, ok := findMP4ChildBox(entry, mp4AudioSampleEntryHeaderSize, "esds")
	if !ok {
		payload, ok = findMP4ChildBox(entry, mp4AudioSampleEntryHeaderAlt, "esds")
	}
	if !ok {
		payload, ok = findMP4BoxByName(entry, "esds")
	}
	if !ok || len(payload) <= 4 {
		return "", "", "", false
	}
	decoder := findESDSDecoderSpecificInfo(payload[4:])
	if len(decoder) == 0 {
		return "", "", "", false
	}
	profile, objType, sbrExplicitNo := parseAACProfileFromASC(decoder)
	info := "Advanced Audio Codec"
	if profile == "LC" {
		info = "Advanced Audio Codec Low Complexity"
	}
	codecID := ""
	if objType > 0 {
		codecID = fmt.Sprintf("mp4a-40-%d", objType)
	}
	return profile, codecID, info, sbrExplicitNo
}

func parseESDSBitrates(entry []byte) (uint32, uint32, bool) {
	payload, ok := findMP4ChildBox(entry, mp4AudioSampleEntryHeaderSize, "esds")
	if !ok {
		payload, ok = findMP4ChildBox(entry, mp4AudioSampleEntryHeaderAlt, "esds")
	}
	if !ok {
		payload, ok = findMP4BoxByName(entry, "esds")
	}
	if !ok || len(payload) <= 4 {
		return 0, 0, false
	}
	buf := payload[4:]
	for i := 0; i < len(buf); i++ {
		if buf[i] != 0x04 {
			continue
		}
		length, n := readMP4DescriptorLength(buf[i+1:])
		if n == 0 {
			continue
		}
		start := i + 1 + n
		if start+length > len(buf) {
			continue
		}
		desc := buf[start : start+length]
		// DecoderConfigDescriptor: objectType(1), streamType(1), bufferSizeDB(3), maxBitrate(4), avgBitrate(4)
		if len(desc) < 13 {
			continue
		}
		maxBitrate := binary.BigEndian.Uint32(desc[5:9])
		avgBitrate := binary.BigEndian.Uint32(desc[9:13])
		return avgBitrate, maxBitrate, true
	}
	return 0, 0, false
}

func findMP4BoxByName(buf []byte, name string) ([]byte, bool) {
	for search := 0; search < len(buf); {
		index := bytes.Index(buf[search:], []byte(name))
		if index < 0 {
			return nil, false
		}
		index += search
		if index >= 4 {
			size := int(binary.BigEndian.Uint32(buf[index-4 : index]))
			if size >= 8 && index-4+size <= len(buf) {
				return buf[index+4 : index-4+size], true
			}
		}
		search = index + 1
	}
	return nil, false
}

func mapAACProfile(objType int) string {
	switch objType {
	case 1:
		return "Main"
	case 2:
		return "LC"
	case 3:
		return "SSR"
	case 4:
		return "LTP"
	case 5:
		return "SBR"
	case 29:
		return "HE-AAC v2"
	default:
		return ""
	}
}

func findESDSDecoderSpecificInfo(buf []byte) []byte {
	for i := range buf {
		if buf[i] != 0x05 {
			continue
		}
		length, n := readMP4DescriptorLength(buf[i+1:])
		if n == 0 {
			continue
		}
		start := i + 1 + n
		if start+length > len(buf) {
			continue
		}
		return buf[start : start+length]
	}
	return nil
}

func readMP4DescriptorLength(buf []byte) (int, int) {
	value := 0
	for i := 0; i < 4 && i < len(buf); i++ {
		value = (value << 7) | int(buf[i]&0x7F)
		if buf[i]&0x80 == 0 {
			return value, i + 1
		}
	}
	return 0, 0
}

func findMP4ChildBox(entry []byte, start int, name string) ([]byte, bool) {
	if start < 0 || start+8 > len(entry) {
		return nil, false
	}
	pos := start
	for pos+8 <= len(entry) {
		size := int(binary.BigEndian.Uint32(entry[pos : pos+4]))
		if size < 8 || pos+size > len(entry) {
			return nil, false
		}
		typ := string(entry[pos+4 : pos+8])
		if typ == name {
			return entry[pos+8 : pos+size], true
		}
		pos += size
	}
	return nil, false
}

func parseBtrt(entry []byte, start int) (uint32, uint32, uint32, bool) {
	payload, ok := findMP4ChildBox(entry, start, "btrt")
	if !ok {
		payload, ok = findMP4BoxByName(entry, "btrt")
	}
	if !ok || len(payload) < 12 {
		return 0, 0, 0, false
	}
	bufferSize := binary.BigEndian.Uint32(payload[0:4])
	maxBitrate := binary.BigEndian.Uint32(payload[4:8])
	avgBitrate := binary.BigEndian.Uint32(payload[8:12])
	return bufferSize, maxBitrate, avgBitrate, true
}

func mapVideoFormatInfo(sampleType string) string {
	switch sampleType {
	case "av01":
		return "AOMedia Video 1"
	case "avc1", "avc3":
		return "Advanced Video Codec"
	case "hvc1", "hev1":
		return "High Efficiency Video Coding"
	case "mp4v":
		return "MPEG-4 Visual"
	default:
		return ""
	}
}

func mapVideoCodecIDInfo(sampleType string) string {
	switch sampleType {
	case "av01":
		return "AOMedia Video 1"
	case "avc1", "avc3":
		return "Advanced Video Coding"
	case "hvc1", "hev1":
		return "High Efficiency Video Coding"
	case "mp4v":
		return "MPEG-4 Visual"
	default:
		return ""
	}
}

func mapAudioFormatInfo(sampleType string) string {
	switch sampleType {
	case "mp4a":
		return "Advanced Audio Codec"
	case "ac-3":
		return "Audio Coding 3"
	case "ec-3":
		return "Enhanced AC-3"
	default:
		return ""
	}
}
