package mediainfo

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"
)

// ParseMPEGVideo parses an MPEG elementary video stream into canonical
// container and video facts.
func ParseMPEGVideo(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}
	parser := &mpeg2VideoParser{}
	frames, ok := parseMPEGVideoStream(file, parser)
	if !ok {
		return ContainerInfo{}, nil, false
	}
	info := parser.finalize()
	duration := 0.0
	if info.FrameRate > 0 {
		duration = float64(frames) / info.FrameRate
	}

	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Fill("Format", "MPEG Video", "Format", "MPEG Video")
	if info.Version != "" {
		builder.Fill("Format_Version", extractVersionNumber(info.Version), "Format version", info.Version)
	}
	if info.Profile != "" {
		profile, level := splitProfileLevel(info.Profile)
		builder.Fill("Format_Profile", profile, "Format profile", info.Profile)
		builder.Structured("Format_Level", level)
	}
	if info.BVOP != nil {
		value := formatYesNo(*info.BVOP)
		builder.Fill("Format_Settings_BVOP", value, "Format settings, BVOP", value)
	}
	if info.Matrix != "" {
		builder.Fill("Format_Settings_Matrix", info.Matrix, "Format settings, Matrix", info.Matrix)
	}
	if info.GOPLength > 0 {
		value := formatGOPLength(info.GOPLength)
		builder.Fill("Format_Settings_GOP", value, "Format settings, GOP", value)
	}
	structuredFacts := &canonicalStructuredFacts{}
	if duration > 0 {
		durationMilliseconds := int64(math.Round(duration * 1000))
		builder.Fill("Duration", strconv.FormatInt(durationMilliseconds, 10), "Duration", formatDuration(duration))
		builder.Fill("BitRate_Mode", "Variable", "Bit rate mode", "Variable")
		displayBitrate := (float64(size) * 8) / duration
		jsonDuration := math.Round(duration*1000) / 1000
		structuredBitrate := (float64(size) * 8) / jsonDuration
		bitRateRaw := strconv.FormatInt(int64(math.Round(structuredBitrate)), 10)
		kbps := int64((displayBitrate / 1000.0) + 0.5)
		if value := formatBitrateKbps(kbps); value != "" {
			builder.Fill("BitRate", bitRateRaw, "Bit rate", value)
			structuredFacts.SetSame("BitRate", bitRateRaw)
		}
		if info.Width > 0 {
			builder.Fill("Width", strconv.FormatUint(info.Width, 10), "Width", formatPixels(info.Width))
		}
		if info.Height > 0 {
			builder.Fill("Height", strconv.FormatUint(info.Height, 10), "Height", formatPixels(info.Height))
		}
		if info.AspectRatio != "" {
			if value, ok := parseRatioFloat(info.AspectRatio); ok {
				builder.Fill("DisplayAspectRatio", formatJSONFloat(value), "Display aspect ratio", info.AspectRatio)
			}
		}
		if info.FrameRateNumer > 0 && info.FrameRateDenom > 0 {
			display := formatFrameRateRatio(info.FrameRateNumer, info.FrameRateDenom)
			builder.Fill("FrameRate", formatJSONFloat(info.FrameRate), "Frame rate", display)
			builder.Structured("FrameRate_Num", strconv.FormatUint(uint64(info.FrameRateNumer), 10))
			builder.Structured("FrameRate_Den", strconv.FormatUint(uint64(info.FrameRateDenom), 10))
		} else if info.FrameRate > 0 {
			builder.Fill("FrameRate", formatJSONFloat(info.FrameRate), "Frame rate", formatFrameRate(info.FrameRate))
		}
		if frames > 0 {
			frameCount := strconv.Itoa(frames)
			builder.Structured("FrameCount", frameCount)
			structuredFacts.SetSame("FrameCount", frameCount)
		}
		if info.ColorSpace != "" {
			builder.Fill("ColorSpace", info.ColorSpace, "Color space", info.ColorSpace)
		}
		if info.ChromaSubsampling != "" {
			builder.Fill("ChromaSubsampling", info.ChromaSubsampling, "Chroma subsampling", info.ChromaSubsampling)
		}
		if info.BitDepth != "" {
			builder.Fill("BitDepth", extractLeadingNumber(info.BitDepth), "Bit depth", info.BitDepth)
		}
		if info.ScanType != "" {
			builder.Fill("ScanType", info.ScanType, "Scan type", info.ScanType)
		}
		builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
		if info.Width > 0 && info.Height > 0 {
			if bits := formatBitsPerPixelFrame(displayBitrate, info.Width, info.Height, info.FrameRate); bits != "" {
				builder.Text("Bits/(Pixel*Frame)", bits)
			}
		}
		if info.TimeCode != "" {
			builder.Fill("TimeCode_FirstFrame", info.TimeCode, "Time code of first frame", info.TimeCode)
		}
		if info.GOPOpenClosed != "" {
			builder.Fill("Gop_OpenClosed", info.GOPOpenClosed, "GOP, Open/Closed", info.GOPOpenClosed)
		}
		if info.GOPFirstClosed != "" {
			builder.Fill("Gop_OpenClosed_FirstFrame", info.GOPFirstClosed, "GOP, Open/Closed of first frame", info.GOPFirstClosed)
		}
		if streamSize := formatStreamSize(size, size); streamSize != "" {
			value := strconv.FormatInt(size, 10)
			builder.Fill("StreamSize", value, "Stream size", streamSize)
			structuredFacts.SetSame("StreamSize", value)
		}
	}

	if size > 0 && structuredFacts.Projection("StreamSize") == "" {
		value := strconv.FormatInt(size, 10)
		builder.Structured("StreamSize", value)
		structuredFacts.SetSame("StreamSize", value)
	}
	if info.BufferSize > 0 {
		value := strconv.FormatInt(info.BufferSize, 10)
		builder.Structured("BufferSize", value)
		structuredFacts.SetSame("BufferSize", value)
	}
	if info.GOPDropFrame != nil && info.GOPClosed != nil && info.GOPBrokenLink != nil {
		drop := 0
		closed := 0
		broken := 0
		if *info.GOPDropFrame {
			drop = 1
		}
		if *info.GOPClosed {
			closed = 1
		}
		if *info.GOPBrokenLink {
			broken = 1
		}
		structuredFacts.SetSame("Delay", "0.000")
		structuredFacts.SetSame("Delay_Settings", fmt.Sprintf("drop_frame_flag=%d / closed_gop=%d / broken_link=%d", drop, closed, broken))
		if drop == 1 {
			structuredFacts.SetSame("Delay_DropFrame", "Yes")
		} else {
			structuredFacts.SetSame("Delay_DropFrame", "No")
		}
		structuredFacts.SetSame("Delay_Source", "Stream")
	}
	structuredFacts.Apply(builder)
	if info.IntraDCPrecision > 0 {
		node := structuredObjectFromKVs([]jsonKV{{Key: "intra_dc_precision", Val: strconv.Itoa(info.IntraDCPrecision)}})
		builder.StructuredNode("extra", node)
	}

	streams := []Stream{builder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true})}
	container := ContainerInfo{}
	if duration > 0 {
		container.DurationSeconds = duration
		container.BitrateMode = "Variable"
	}
	return container, streams, true
}

func parseMPEGVideoStream(file io.Reader, parser *mpeg2VideoParser) (int, bool) {
	reader := bufio.NewReaderSize(file, 1<<20)
	first := make([]byte, 4)
	if _, err := io.ReadFull(reader, first); err != nil {
		return 0, false
	}
	if first[0] != 0x00 || first[1] != 0x00 || first[2] != 0x01 || first[3] != 0xB3 {
		return 0, false
	}

	parser.consume(first)
	frames := 0
	window := [4]byte{first[0], first[1], first[2], first[3]}
	if window[0] == 0x00 && window[1] == 0x00 && window[2] == 0x01 && window[3] == 0x00 {
		frames++
	}

	buf := make([]byte, 1<<20)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			parser.consume(chunk)
			for _, b := range chunk {
				window[0], window[1], window[2] = window[1], window[2], window[3]
				window[3] = b
				if window[0] == 0x00 && window[1] == 0x00 && window[2] == 0x01 && window[3] == 0x00 {
					frames++
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, false
		}
	}
	return frames, true
}

func formatGOPLength(length int) string {
	if length <= 0 {
		return ""
	}
	return "N=" + itoa(length)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	v := value
	for v > 0 {
		pos--
		buf[pos] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[pos:])
}
