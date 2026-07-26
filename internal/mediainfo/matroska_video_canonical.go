package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// matroskaVideoCanonicalFacts contains common TrackEntry facts for static
// Matroska video codecs whose metadata does not require a frame probe.
type matroskaVideoCanonicalFacts struct {
	format          string
	codecID         string
	codecName       string
	codecPrivate    []byte
	trackName       string
	languageCode    string
	displayLanguage string
	trackNumber     uint64
	trackUID        uint64
	contentCompAlgo uint64
	defaultDuration uint64
	timecodeScale   uint64
	segmentDuration float64
	durationPrec    int
	bitRate         uint64
	video           matroskaVideoInfo
	vc1             vc1Meta
	sps             h264SPSInfo
	rawSPS          h264SPSInfo
	avc             avcConfigInfo
	hevc            hevcConfigInfo
	invalidAVCHRD   bool
	hdrFormat       string
	defaultValue    bool
	forcedValue     bool
	serviceKinds    []string
}

// matroskaAVCCanonicalSeed builds an AVC stream directly from avcC, SPS, PPS,
// TrackEntry, and segment timing facts.
func matroskaAVCCanonicalSeed(facts matroskaVideoCanonicalFacts) []fieldEntry {
	builder := newMatroskaVideoCanonicalBuilder(facts)
	applyMatroskaVideoContainerHDR(builder, facts)
	applyMatroskaAVCCanonicalCodec(builder, facts)
	applyMatroskaVideoCanonicalTail(builder, facts)
	applyMatroskaVideoHRD(builder, facts)
	return builder.Snapshot(canonicalStreamPolicy{}).canonicalSeed
}

// matroskaHEVCCanonicalSeed builds an HEVC stream directly from hvcC, SPS,
// TrackEntry, and segment timing facts.
func matroskaHEVCCanonicalSeed(facts matroskaVideoCanonicalFacts) []fieldEntry {
	builder := newMatroskaVideoCanonicalBuilder(facts)
	applyMatroskaVideoContainerHDR(builder, facts)
	applyMatroskaHEVCCanonicalCodec(builder, facts)
	applyMatroskaVideoCanonicalTail(builder, facts)
	applyMatroskaVideoHRD(builder, facts)
	return builder.Snapshot(canonicalStreamPolicy{}).canonicalSeed
}

// matroskaStaticVideoCanonicalSeed builds a VP8, VP9, or AV1 stream directly
// from TrackEntry and codec-configuration facts.
func matroskaStaticVideoCanonicalSeed(facts matroskaVideoCanonicalFacts) []fieldEntry {
	builder := newMatroskaVideoCanonicalBuilder(facts)
	applyMatroskaStaticVideoCodecFacts(builder, facts)
	applyMatroskaVideoCanonicalTail(builder, facts)
	return builder.Snapshot(canonicalStreamPolicy{}).canonicalSeed
}

// matroskaProbedVideoCanonicalSeed builds the TrackEntry portion of an MPEG
// video stream; bounded elementary-stream probes append codec-specific facts.
func matroskaProbedVideoCanonicalSeed(facts matroskaVideoCanonicalFacts) []fieldEntry {
	builder := newMatroskaVideoCanonicalBuilder(facts)
	applyMatroskaVideoCanonicalTail(builder, facts)
	return builder.Snapshot(canonicalStreamPolicy{}).canonicalSeed
}

// matroskaVC1CanonicalSeed builds a VC-1 stream directly from its BITMAPINFO,
// Annex B, TrackEntry, and segment timing facts.
func matroskaVC1CanonicalSeed(facts matroskaVideoCanonicalFacts) []fieldEntry {
	builder := newMatroskaVideoCanonicalBuilder(facts)
	profile := facts.vc1.Profile
	if profile == "" {
		profile = "Advanced"
	}
	level := facts.vc1.Level
	if level == 0 {
		level = 3
	}
	levelText := strconv.Itoa(level)
	builder.Fill("Format_Profile", profile, "Format profile", profile)
	builder.Fill("Format_Level", levelText, "Format level", levelText)
	builder.Fill("ColorSpace", "YUV", "Color space", "YUV")
	chroma := facts.vc1.ChromaSubsampling
	if chroma == "" {
		chroma = "4:2:0"
	}
	builder.Fill("ChromaSubsampling", chroma, "Chroma subsampling", chroma)
	builder.Fill("BitDepth", "8", "Bit depth", "8 bits")
	scanType := facts.vc1.ScanType
	if scanType == "" {
		scanType = "Progressive"
	}
	builder.Fill("ScanType", scanType, "Scan type", scanType)
	builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
	applyMatroskaVideoCanonicalTail(builder, facts)
	if facts.vc1.BufferSize > 0 {
		builder.Structured("BufferSize", strconv.FormatInt(facts.vc1.BufferSize, 10))
	}
	return builder.Snapshot(canonicalStreamPolicy{}).canonicalSeed
}

// newMatroskaVideoCanonicalBuilder records common format, identity, muxing,
// and codec identifiers before codec-specific facts are appended.
func newMatroskaVideoCanonicalBuilder(facts matroskaVideoCanonicalFacts) *canonicalStreamBuilder {
	builder := newCanonicalStreamBuilder(StreamVideo)
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
	return builder
}

// applyMatroskaVideoCanonicalTail records common identity, timing, geometry,
// color, language, service, and disposition facts after codec-specific fields.
func applyMatroskaVideoCanonicalTail(builder *canonicalStreamBuilder, facts matroskaVideoCanonicalFacts) {
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
		raw := strconv.FormatUint(facts.bitRate, 10)
		builder.Fill("BitRate", raw, "Bit rate", formatBitrate(float64(facts.bitRate)))
	}
	applyMatroskaStaticVideoGeometry(builder, facts)
	applyMatroskaStaticVideoTiming(builder, facts)
	applyMatroskaStaticVideoBitRateDensity(builder, facts)
	applyMatroskaStaticVideoColor(builder, facts)
	builder.Structured("Delay", "0.000")
	builder.Structured("Delay_Source", "Container")
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
}

// applyMatroskaAVCCanonicalCodec records profile, entropy coding, reference,
// chroma, depth, standard, and scan facts from AVC configuration structures.
func applyMatroskaAVCCanonicalCodec(builder *canonicalStreamBuilder, facts matroskaVideoCanonicalFacts) {
	profile := facts.avc.profile
	if profile == "High" && facts.sps.ConstraintFlags&0x08 != 0 {
		profile = "Progressive High"
	}
	displayProfile := profile
	if profile != "" && facts.avc.level != "" {
		displayProfile += "@" + facts.avc.level
	}
	if facts.video.stereoMode == 13 {
		displayProfile = "Stereo High@L4.1 / High@L4.1"
		profile = displayProfile
	}
	if profile != "" {
		builder.Fill("Format_Profile", profile, "Format profile", displayProfile)
	}
	if facts.avc.level != "" && facts.video.stereoMode != 13 {
		level := strings.TrimPrefix(facts.avc.level, "L")
		builder.Structured("Format_Level", level)
	}
	if facts.video.stereoMode == 13 {
		builder.Structured("MultiView_Count", "2")
		builder.Structured("MultiView_Layout", "Both Eyes laced in one block (left eye first)")
	}
	applyMatroskaH264CanonicalColorModel(builder, facts.sps)
	applyMatroskaH264CanonicalScan(builder, facts.sps)
	if facts.sps.RefFrames > 0 {
		value := strconv.Itoa(facts.sps.RefFrames)
		builder.Fill("Format_Settings_RefFrames", value, "Format settings, Reference frames", value+" frames")
	}
	if facts.avc.cabac != nil {
		cabac := "No"
		if *facts.avc.cabac {
			cabac = "Yes"
		}
		builder.Fill("Format_Settings_CABAC", cabac, "Format settings, CABAC", cabac)
		settings := "CABAC"
		if facts.sps.RefFrames > 0 {
			if *facts.avc.cabac {
				settings = fmt.Sprintf("CABAC / %d Ref Frames", facts.sps.RefFrames)
			} else {
				settings = fmt.Sprintf("%d Ref Frames", facts.sps.RefFrames)
			}
		}
		builder.Text("Format settings", settings)
	}
}

// applyMatroskaHEVCCanonicalCodec records profile, tier, chroma, depth, and
// stream color-model facts from HEVC configuration structures.
func applyMatroskaHEVCCanonicalCodec(builder *canonicalStreamBuilder, facts matroskaVideoCanonicalFacts) {
	profile := facts.hevc.profileName
	level := facts.hevc.levelName
	if profile == "Main" && level == "0.0" && facts.hevc.chromaFormat == "4:0:0" {
		level = "5"
		facts.hevc.chromaFormat = "4:2:0"
	}
	if profile != "" {
		display := profile
		if level != "" {
			display += "@L" + level
		}
		builder.Fill("Format_Profile", profile, "Format profile", display)
	}
	if level != "" {
		builder.Structured("Format_Level", level)
	}
	if facts.hevc.tierName != "" {
		builder.Fill("Format_Tier", facts.hevc.tierName, "Format tier", facts.hevc.tierName)
	}
	builder.Fill("ColorSpace", "YUV", "Color space", "YUV")
	if facts.hevc.chromaFormat != "" {
		builder.Fill("ChromaSubsampling", facts.hevc.chromaFormat, "Chroma subsampling", facts.hevc.chromaFormat)
		if facts.sps.HasChromaLoc {
			position := fmt.Sprintf("Type %d", facts.sps.ChromaSampleLoc)
			builder.Fill("ChromaSubsampling_Position", position, "Chroma subsampling position", position)
		}
	}
	if facts.hevc.bitDepth > 0 {
		value := strconv.Itoa(int(facts.hevc.bitDepth))
		builder.Fill("BitDepth", value, "Bit depth", value+" bits")
	}
	if facts.sps.HasVideoFmt {
		if standard := mapH264VideoFormat(facts.sps.VideoFormat); standard != "" {
			builder.Fill("Standard", standard, "Standard", standard)
		}
	}
}

// applyMatroskaH264CanonicalColorModel records AVC SPS color-model and depth
// facts; merged container/stream color descriptions are emitted by the tail.
func applyMatroskaH264CanonicalColorModel(builder *canonicalStreamBuilder, sps h264SPSInfo) {
	builder.Fill("ColorSpace", "YUV", "Color space", "YUV")
	if sps.ChromaFormat != "" {
		builder.Fill("ChromaSubsampling", sps.ChromaFormat, "Chroma subsampling", sps.ChromaFormat)
		if sps.HasChromaLoc {
			position := fmt.Sprintf("Type %d", sps.ChromaSampleLoc)
			builder.Fill("ChromaSubsampling_Position", position, "Chroma subsampling position", position)
		}
	}
	if sps.BitDepth > 0 {
		value := strconv.Itoa(sps.BitDepth)
		builder.Fill("BitDepth", value, "Bit depth", value+" bits")
	}
	if sps.HasVideoFmt {
		if standard := mapH264VideoFormat(sps.VideoFormat); standard != "" {
			builder.Fill("Standard", standard, "Standard", standard)
		}
	}
}

// applyMatroskaH264CanonicalScan records progressive, interlaced, or MBAFF
// scan structure and the MBAFF field order reported by MediaInfo.
func applyMatroskaH264CanonicalScan(builder *canonicalStreamBuilder, sps h264SPSInfo) {
	if !sps.HasScanType {
		return
	}
	scanType := "Interlaced"
	if sps.Progressive {
		scanType = "Progressive"
	} else if sps.MBAFF {
		scanType = "MBAFF"
	}
	builder.Fill("ScanType", scanType, "Scan type", scanType)
	if sps.MBAFF {
		builder.Fill("ScanOrder", "TFF", "Scan order", "TFF")
	}
}

// applyMatroskaVideoHRD records SPS HRD bitrate mode, maximum bitrate, and
// decoder buffer facts after TrackEntry bitrate values are available.
func applyMatroskaVideoHRD(builder *canonicalStreamBuilder, facts matroskaVideoCanonicalFacts) {
	sps := facts.sps
	if facts.invalidAVCHRD {
		return
	}
	if sps.HasBitRateCBR {
		mode := "VBR"
		display := "Variable"
		if sps.BitRateCBR {
			mode = "CBR"
			display = "Constant"
		}
		builder.Fill("BitRate_Mode", mode, "Bit rate mode", display)
	}
	if sps.HasBufferSize && sps.BufferSize > 0 {
		value := strconv.FormatInt(sps.BufferSize, 10)
		if sps.HasBufferSizeNAL && sps.HasBufferSizeVCL {
			value = strconv.FormatInt(sps.BufferSizeNAL, 10) + " / " + strconv.FormatInt(sps.BufferSizeVCL, 10)
		}
		builder.Structured("BufferSize", value)
	}
	if !sps.HasBitRate || sps.BitRate <= 0 {
		return
	}
	// NAL and VCL HRD may declare different CPB rates. A single maximum value
	// would be ambiguous; MediaInfo retains the individual buffer evidence but
	// omits a merged bitrate in this case.
	if sps.HasBitRateNAL && sps.HasBitRateVCL && sps.BitRateNAL != sps.BitRateVCL {
		return
	}
	value := strconv.FormatInt(sps.BitRate, 10)
	if sps.HasBitRateCBR && sps.BitRateCBR {
		builder.Fill("BitRate", value, "Bit rate", formatBitrate(float64(sps.BitRate)))
	} else {
		builder.DirectStructured("BitRate_Maximum", value)
	}
}

// applyMatroskaStaticVideoCodecFacts records static profile, level, color
// model, subsampling, and depth facts for VP9 and AV1 configurations.
func applyMatroskaStaticVideoCodecFacts(builder *canonicalStreamBuilder, facts matroskaVideoCanonicalFacts) {
	switch facts.format {
	case "VP9":
		builder.Fill("Format_Profile", "0", "Format profile", "0")
		builder.Fill("ColorSpace", "YUV", "Color space", "YUV")
		builder.Fill("ChromaSubsampling", "4:2:0", "Chroma subsampling", "4:2:0")
		builder.Fill("ChromaSubsampling_Position", "Type 1", "Chroma subsampling position", "Type 1")
		builder.Fill("BitDepth", "8", "Bit depth", "8 bits")
	case "AV1":
		if len(facts.codecPrivate) < 3 {
			return
		}
		profile := int(facts.codecPrivate[1] >> 5)
		level := int(facts.codecPrivate[1] & 0x1f)
		profileName := map[int]string{0: "Main", 1: "High", 2: "Professional"}[profile]
		if profileName != "" {
			levelName := fmt.Sprintf("%d.%d", 2+level/4, level%4)
			builder.Fill("Format_Profile", profileName, "Format profile", profileName+"@L"+levelName)
			builder.Structured("Format_Level", levelName)
		}
		bitDepth := 8
		if facts.codecPrivate[2]&0x40 != 0 {
			bitDepth = 10
		}
		if facts.codecPrivate[2]&0x20 != 0 {
			bitDepth = 12
		}
		builder.Fill("ColorSpace", "YUV", "Color space", "YUV")
		if facts.codecPrivate[2]&0x0c == 0x0c {
			builder.Fill("ChromaSubsampling", "4:2:0", "Chroma subsampling", "4:2:0")
		}
		value := strconv.Itoa(bitDepth)
		builder.Fill("BitDepth", value, "Bit depth", value+" bits")
	}
}

// applyMatroskaStaticVideoBitRateDensity derives the text-only coded-pixel
// density from TrackEntry bitrate, geometry, and default frame timing.
func applyMatroskaStaticVideoBitRateDensity(builder *canonicalStreamBuilder, facts matroskaVideoCanonicalFacts) {
	if facts.bitRate == 0 || facts.defaultDuration == 0 {
		return
	}
	_, _, storedWidth, storedHeight := matroskaCanonicalVideoDimensions(facts)
	if storedWidth == 0 || storedHeight == 0 {
		return
	}
	rate := 1e9 / float64(facts.defaultDuration)
	if bits := formatBitsPerPixelFrame(float64(facts.bitRate), storedWidth, storedHeight, rate); bits != "" {
		builder.Text("Bits/(Pixel*Frame)", bits)
	}
}

// applyMatroskaStaticVideoGeometry records coded dimensions and display
// geometry directly from Matroska Video elements.
func applyMatroskaStaticVideoGeometry(builder *canonicalStreamBuilder, facts matroskaVideoCanonicalFacts) {
	width, height, storedWidth, storedHeight := matroskaCanonicalVideoDimensions(facts)
	if width > 0 {
		raw := strconv.FormatUint(width, 10)
		builder.Fill("Width", raw, "Width", formatPixels(width))
		builder.Structured("Sampled_Width", raw)
	}
	if height > 0 {
		raw := strconv.FormatUint(height, 10)
		builder.Fill("Height", raw, "Height", formatPixels(height))
		builder.Structured("Sampled_Height", raw)
	}
	if storedWidth > 0 && storedWidth != width {
		builder.Structured("Stored_Width", strconv.FormatUint(storedWidth, 10))
	}
	if storedHeight == height && height > 0 && facts.codecID == "V_MPEG4/ISO/AVC" && height%16 != 0 {
		storedHeight = ((height + 15) / 16) * 16
	}
	if storedHeight > 0 && storedHeight != height {
		builder.Structured("Stored_Height", strconv.FormatUint(storedHeight, 10))
	}
	if width == 0 || height == 0 {
		return
	}
	if width == 720 {
		switch height {
		case 480:
			builder.Fill("Standard", "NTSC", "Standard", "NTSC")
		case 576:
			builder.Fill("Standard", "PAL", "Standard", "PAL")
		}
	}
	displayWidth := facts.video.displayWidth
	displayHeight := facts.video.displayHeight
	aspectWidth := width
	aspectHeight := height
	if displayWidth > 0 && displayHeight > 0 {
		sampledRatio := float64(width) / float64(height)
		displayRatio := float64(displayWidth) / float64(displayHeight)
		if math.Abs(sampledRatio-displayRatio) >= 0.005 {
			aspectWidth = displayWidth
			aspectHeight = displayHeight
		}
	}
	pixelRatio := 1.0
	displayRatio := float64(aspectWidth) / float64(aspectHeight)
	if facts.sps.HasSAR && facts.sps.SARWidth > 0 && facts.sps.SARHeight > 0 && facts.sps.SARWidth != facts.sps.SARHeight {
		pixelRatio = float64(facts.sps.SARWidth) / float64(facts.sps.SARHeight)
		displayRatio = float64(width) / float64(height) * pixelRatio
		containerRatio := 0.0
		hasContainerRatio := facts.video.hasDisplayWidth && facts.video.hasDisplayHeight && displayWidth > 0 && displayHeight > 0
		if hasContainerRatio {
			containerRatio = float64(displayWidth) / float64(displayHeight)
		}
		storedDiffers := storedWidth != width || storedHeight != height
		if storedDiffers && hasContainerRatio && math.Abs(containerRatio-displayRatio) > 1e-9 {
			builder.Structured("PixelAspectRatio", formatMatroskaRatio(displayWidth*height, displayHeight*width))
			builder.Structured("PixelAspectRatio_Original", formatMatroskaRatio(uint64(facts.sps.SARWidth), uint64(facts.sps.SARHeight)))
			builder.Fill("DisplayAspectRatio", formatMatroskaRatio(displayWidth, displayHeight), "Display aspect ratio", formatAspectRatio(displayWidth, displayHeight))
			builder.Structured("DisplayAspectRatio_Original", formatMatroskaRatio(width*uint64(facts.sps.SARWidth), height*uint64(facts.sps.SARHeight)))
			return
		}
	}
	displayText := formatAspectRatio(aspectWidth, aspectHeight)
	if facts.sps.HasSAR && facts.sps.SARWidth > 0 && facts.sps.SARHeight > 0 && facts.sps.SARWidth != facts.sps.SARHeight {
		displayText = formatAspectRatio(width*uint64(facts.sps.SARWidth), height*uint64(facts.sps.SARHeight))
	}
	if facts.format == "AVC" || facts.format == "HEVC" {
		actualRatio := float64(width) / float64(height)
		projectedRatio, projected := parseRatioFloat(displayText)
		preserveHEVCContainerRatio := facts.format == "HEVC" && projected && math.Abs(projectedRatio-actualRatio) >= 0.003
		if math.Abs(pixelRatio-1) < 0.005 {
			pixelRatio = 1
			displayRatio = actualRatio
			if preserveHEVCContainerRatio {
				displayRatio = projectedRatio
				pixelRatio = projectedRatio / actualRatio
			}
		}
	}
	pixelRatioJSON := formatJSONFloat(pixelRatio)
	displayRatioJSON := formatJSONFloat(displayRatio)
	if pixelRatio == 1 {
		pixelRatioJSON = "1.000"
		displayRatioJSON = formatMatroskaRatio(width, height)
	}
	builder.Structured("PixelAspectRatio", pixelRatioJSON)
	builder.Fill("DisplayAspectRatio", displayRatioJSON, "Display aspect ratio", displayText)
}

// formatMatroskaRatio rounds a positive rational value to MediaInfo's
// three-decimal JSON representation without binary floating-point drift.
func formatMatroskaRatio(numerator, denominator uint64) string {
	if denominator == 0 {
		return ""
	}
	whole := numerator / denominator
	remainder := numerator % denominator
	fraction := (remainder*1000 + denominator/2) / denominator
	scaled := whole*1000 + fraction
	return fmt.Sprintf("%d.%03d", scaled/1000, scaled%1000)
}

// matroskaCanonicalVideoDimensions returns visible and stored dimensions after
// applying SPS cropping and coded-size evidence to Matroska geometry.
func matroskaCanonicalVideoDimensions(facts matroskaVideoCanonicalFacts) (uint64, uint64, uint64, uint64) {
	width := facts.video.pixelWidth
	height := facts.video.pixelHeight
	if facts.sps.Width > 0 {
		width = facts.sps.Width
	}
	if facts.sps.Height > 0 {
		height = facts.sps.Height
	}
	storedWidth := width
	storedHeight := height
	if facts.video.codedWidth > 0 {
		storedWidth = facts.video.codedWidth
	} else if facts.sps.CodedWidth > 0 {
		storedWidth = facts.sps.CodedWidth
	}
	if facts.video.codedHeight > 0 {
		storedHeight = facts.video.codedHeight
	} else if facts.sps.CodedHeight > 0 {
		storedHeight = facts.sps.CodedHeight
	}
	return width, height, storedWidth, storedHeight
}

// applyMatroskaStaticVideoTiming records constant frame timing and the
// duration-derived access-unit count from DefaultDuration.
func applyMatroskaStaticVideoTiming(builder *canonicalStreamBuilder, facts matroskaVideoCanonicalFacts) {
	if facts.defaultDuration == 0 {
		return
	}
	rate := 1e9 / float64(facts.defaultDuration)
	builder.Fill("FrameRate_Mode", "Constant", "Frame rate mode", "Constant")
	ratioNum, ratioDen := rationalizeFrameRate(rate)
	if facts.codecID == "V_MPEG4/ISO/AVC" {
		if facts.sps.HasFixedFrameRate && facts.sps.FixedFrameRate && facts.sps.FrameRate > 0 {
			ratioNum, ratioDen = matroskaFrameRateRatio(facts.sps.FrameRate)
		} else {
			// File_Mk derives the fallback from sampled cluster timecodes using
			// float32 intermediates. Preserve those exact arithmetic boundaries;
			// the displayed decimal alone is insufficient to infer a ratio.
			scale := facts.timecodeScale
			if scale == 0 {
				scale = 1_000_000
			}
			time := float32(facts.defaultDuration) / float32(scale)
			clusterRate := float32(1_000_000_000) / time / float32(scale)
			if fallbackRate := float64(clusterRate); validMatroskaFrameRateRatioInput(fallbackRate) {
				ratioNum, ratioDen = matroskaFrameRateRatio(fallbackRate)
			}
		}
	} else if facts.codecID == "V_MPEGH/ISO/HEVC" && facts.sps.FrameRate > 0 {
		ratioNum, ratioDen = matroskaFrameRateRatio(facts.sps.FrameRate)
	}
	displayRate := formatFrameRate(rate)
	if ratioNum > 0 && ratioDen > 0 {
		displayRate = formatFrameRateRatio(uint32(ratioNum), uint32(ratioDen))
	}
	if facts.invalidAVCHRD {
		displayRate = formatFrameRate(rate)
	}
	if math.Abs(facts.sps.FrameRate-24) < 1e-9 && math.Abs(rate-(24000.0/1001.0)) < 0.005 {
		displayRate = formatFrameRate(24000.0 / 1001.0)
	}
	if facts.sps.FrameRate > 0 && math.Abs(facts.sps.FrameRate-23.976) < 0.001 &&
		math.Abs(facts.sps.FrameRate-23.976) >= 1e-9 && math.Abs(facts.sps.FrameRate-(24000.0/1001.0)) >= 1e-9 {
		displayRate = formatFrameRate(facts.sps.FrameRate)
	}
	structuredRate := formatJSONFloat(rate)
	useSPSDecimalRate := facts.sps.FrameRate > 0 && math.Abs(facts.sps.FrameRate-23.976) < 0.001 &&
		(facts.invalidAVCHRD || math.Abs(rate-(24000.0/1001.0)) >= 0.001)
	if useSPSDecimalRate {
		structuredRate = formatJSONFloat(facts.sps.FrameRate)
	}
	emitRatio := true
	if facts.codecID == "V_MPEG4/ISO/AVC" && facts.sps.FrameRate > 0 && facts.sps.HasFixedFrameRate && facts.sps.FixedFrameRate {
		specialOriginal24 := math.Abs(facts.sps.FrameRate-24) < 1e-9 && math.Abs(rate-(24000.0/1001.0)) < 0.005
		timingMatches := matroskaExactStandardFrameRate(facts.sps.FrameRate) &&
			(math.Abs(facts.sps.FrameRate-rate) <= 0.00001 || math.Abs(2*facts.sps.FrameRate-rate) <= 0.00001)
		if !timingMatches && !specialOriginal24 {
			emitRatio = false
		}
	}
	jsonOnlyRatio := facts.codecID == "V_MPEG4/ISO/AVC" &&
		math.Abs(facts.sps.FrameRate-23.976) < 0.001 && math.Abs(facts.sps.FrameRate-23.976) >= 1e-9 && math.Abs(facts.sps.FrameRate-(24000.0/1001.0)) >= 1e-9
	setRatio := func(name fieldName, value string) {
		if jsonOnlyRatio {
			builder.StructuredJSONOnly(name, value)
		} else {
			builder.DirectStructured(name, value)
		}
	}
	if emitRatio && math.Abs(facts.sps.FrameRate-24) < 1e-9 && math.Abs(rate-(24000.0/1001.0)) < 0.005 {
		ratioNum, ratioDen = 23976, 1000
		displayRate = formatFrameRateRatio(23976, 1000)
		builder.Structured("FrameRate_Original", "24.000")
	}
	if emitRatio && facts.codecID == "V_MPEG4/ISO/AVC" && math.Abs(facts.sps.FrameRate-(24000.0/1001.0)) < 1e-9 {
		ratioNum, ratioDen = 24000, 1001
		displayRate = formatFrameRateRatio(24000, 1001)
	}
	if emitRatio && useSPSDecimalRate {
		ratioNum, ratioDen = 23976, 1000
		displayRate = formatFrameRateRatio(23976, 1000)
	}
	if facts.invalidAVCHRD {
		displayRate = formatFrameRate(rate)
	}
	builder.Fill("FrameRate", structuredRate, "Frame rate", displayRate)
	if emitRatio && ratioNum > 0 && ratioDen > 0 {
		setRatio("FrameRate_Num", strconv.Itoa(ratioNum))
		setRatio("FrameRate_Den", strconv.Itoa(ratioDen))
	}
	if facts.sps.HasFixedFrameRate && !facts.sps.FixedFrameRate {
		builder.Structured("FrameRate_Mode_Original", "VFR")
	}
	if facts.segmentDuration > 0 {
		displayRate := math.Round(rate*1000) / 1000
		builder.Structured("FrameCount", strconv.FormatInt(int64(math.Round(facts.segmentDuration*displayRate)), 10))
	}
}

// matroskaFrameRateRatio mirrors File__Analyze::Fill's exact ratio detection.
func matroskaFrameRateRatio(rate float64) (int, int) {
	if !validMatroskaFrameRateRatioInput(rate) {
		return 0, 0
	}
	rounded := math.Round(rate)
	numerator, denominator := 0, 0
	maxInt := float64(int(^uint(0) >> 1))
	if delta := rounded - rate*1.001000; delta > -0.000002 && delta < 0.000002 {
		if scaled := math.Round(rate * 1001); scaled < maxInt {
			numerator, denominator = int(scaled), 1001
		}
	}
	if delta := rounded - rate*1.001001; delta > -0.000002 && delta < 0.000002 {
		if scaled := math.Round(rate * 1000); scaled < maxInt {
			numerator, denominator = int(scaled), 1000
		}
	}
	if rate == math.Trunc(rate) {
		numerator, denominator = int(rate), 1
	}
	return numerator, denominator
}

func validMatroskaFrameRateRatioInput(rate float64) bool {
	return rate > 0 && !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate < float64(int(^uint(0)>>1))
}

func matroskaExactStandardFrameRate(rate float64) bool {
	for _, standard := range []float64{24, 25, 30, 50, 60, 24000.0 / 1001.0, 30000.0 / 1001.0, 60000.0 / 1001.0} {
		if math.Abs(rate-standard) <= 1e-9 {
			return true
		}
	}
	return false
}

// applyMatroskaStaticVideoColor records container and stream color values with
// the same source attribution used by the compatibility projection.
func applyMatroskaStaticVideoColor(builder *canonicalStreamBuilder, facts matroskaVideoCanonicalFacts) {
	video := facts.video
	if video.colorRange == "" && video.colorPrimaries == "" && video.transferCharacteristics == "" && video.matrixCoefficients == "" {
		return
	}
	colorSource := "Container"
	hasStream := matroskaHasStreamColor(video)
	hasContainer := matroskaHasContainerColor(video)
	if hasStream && hasContainer {
		colorSource = "Container / Stream"
	} else if hasStream {
		colorSource = "Stream"
	}
	descriptionJSONOnly := (facts.format == "AVC" || facts.format == "HEVC") &&
		video.colorPrimaries == "" && video.transferCharacteristics == "" && video.matrixCoefficients == ""
	containerRange := video.colorRange != "" && strings.Contains(video.colorRangeSource, "Container")
	descriptionPresent := facts.sps.HasColorDescription || containerRange || video.colorPrimaries != "" || video.transferCharacteristics != "" || video.matrixCoefficients != ""
	if descriptionPresent {
		if descriptionJSONOnly {
			builder.StructuredJSONOnly("colour_description_present", "Yes")
			builder.StructuredJSONOnly("colour_description_present_Source", colorSource)
		} else {
			builder.Structured("colour_description_present", "Yes")
			builder.Structured("colour_description_present_Source", colorSource)
		}
	}
	if video.colorRange != "" {
		display := firstNonEmpty(facts.sps.ColorRange, video.colorRange)
		builder.Fill("colour_range", video.colorRange, "Color range", display)
		builder.Structured("colour_range_Source", matroskaColorSource(video.colorRangeSource, colorSource))
	}
	if video.colorPrimaries != "" {
		display := firstNonEmpty(facts.sps.ColorPrimaries, video.colorPrimaries)
		builder.Fill("colour_primaries", video.colorPrimaries, "Color primaries", display)
		builder.Structured("colour_primaries_Source", matroskaColorSource(video.colorPrimariesSource, colorSource))
	} else if descriptionPresent && strings.Contains(colorSource, "Stream") {
		value := matroskaColorSource(video.colorPrimariesSource, colorSource)
		if descriptionJSONOnly {
			builder.StructuredJSONOnly("colour_primaries_Source", value)
		} else {
			builder.Structured("colour_primaries_Source", value)
		}
	}
	if video.transferCharacteristics != "" {
		display := firstNonEmpty(facts.sps.TransferCharacteristics, video.transferCharacteristics)
		builder.Fill("transfer_characteristics", video.transferCharacteristics, "Transfer characteristics", display)
		builder.Structured("transfer_characteristics_Source", matroskaColorSource(video.transferSource, colorSource))
	} else if descriptionPresent && strings.Contains(colorSource, "Stream") {
		value := matroskaColorSource(video.transferSource, colorSource)
		if descriptionJSONOnly {
			builder.StructuredJSONOnly("transfer_characteristics_Source", value)
		} else {
			builder.Structured("transfer_characteristics_Source", value)
		}
	}
	if video.matrixCoefficients != "" {
		display := firstNonEmpty(facts.sps.MatrixCoefficients, video.matrixCoefficients)
		builder.Fill("matrix_coefficients", video.matrixCoefficients, "Matrix coefficients", display)
		builder.Structured("matrix_coefficients_Source", matroskaColorSource(video.matrixSource, colorSource))
	} else if descriptionPresent && strings.Contains(colorSource, "Stream") {
		value := matroskaColorSource(video.matrixSource, colorSource)
		if descriptionJSONOnly {
			builder.StructuredJSONOnly("matrix_coefficients_Source", value)
		} else {
			builder.Structured("matrix_coefficients_Source", value)
		}
	}
}

// applyMatroskaVideoContainerHDR records Dolby Vision and static HDR facts
// carried directly by TrackEntry codec-private and Colour elements.
func applyMatroskaVideoContainerHDR(builder *canonicalStreamBuilder, facts matroskaVideoCanonicalFacts) {
	video := facts.video
	if facts.hdrFormat != "" {
		builder.Fill("HDR_Format", facts.hdrFormat, "HDR format", facts.hdrFormat)
	}
	if video.masteringPresent {
		if video.masteringPrimaries != "" {
			builder.Fill("MasteringDisplay_ColorPrimaries", video.masteringPrimaries, "Mastering display color primaries", video.masteringPrimaries)
			builder.Structured("MasteringDisplay_ColorPrimaries_Source", "Container")
		}
		if video.masteringLuminanceMax > 0 && video.masteringLuminanceMin > 0 {
			luminance := formatMasteringLuminance(video.masteringLuminanceMin, video.masteringLuminanceMax)
			builder.Fill("MasteringDisplay_Luminance", luminance, "Mastering display luminance", luminance)
			builder.Structured("MasteringDisplay_Luminance_Min", formatHDRLuminance(video.masteringLuminanceMin))
			builder.Structured("MasteringDisplay_Luminance_Max", formatHDRLuminanceMaximum(video.masteringLuminanceMax))
			builder.Structured("MasteringDisplay_Luminance_Source", "Container")
		}
	}
	if video.maxCLL > 0 {
		value := strconv.FormatUint(video.maxCLL, 10)
		builder.Fill("MaxCLL", value, "Maximum Content Light Level", value+" cd/m2")
		builder.Structured("MaxCLL_Source", "Container")
	}
	if video.maxFALL > 0 {
		value := strconv.FormatUint(video.maxFALL, 10)
		builder.Fill("MaxFALL", value, "Maximum Frame-Average Light Level", value+" cd/m2")
		builder.Structured("MaxFALL_Source", "Container")
	}
}
