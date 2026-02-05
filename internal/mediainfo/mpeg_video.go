package mediainfo

import (
	"fmt"
	"io"
	"math"
	"strconv"
)

func ParseMPEGVideo(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}
	data, err := io.ReadAll(file)
	if err != nil || len(data) < 4 {
		return ContainerInfo{}, nil, false
	}
	if data[0] != 0x00 || data[1] != 0x00 || data[2] != 0x01 || data[3] != 0xB3 {
		return ContainerInfo{}, nil, false
	}

	parser := &mpeg2VideoParser{}
	parser.consume(data)
	info := parser.finalize()

	frames := countMPEG2Pictures(data)
	duration := 0.0
	if info.FrameRate > 0 {
		duration = float64(frames) / info.FrameRate
	}

	fields := []Field{{Name: "Format", Value: "MPEG Video"}}
	if info.Version != "" {
		fields = append(fields, Field{Name: "Format version", Value: info.Version})
	}
	if info.Profile != "" {
		fields = append(fields, Field{Name: "Format profile", Value: info.Profile})
	}
	if info.BVOP != nil {
		fields = append(fields, Field{Name: "Format settings, BVOP", Value: formatYesNo(*info.BVOP)})
	}
	if info.Matrix != "" {
		fields = append(fields, Field{Name: "Format settings, Matrix", Value: info.Matrix})
	}
	if info.GOPLength > 0 {
		fields = append(fields, Field{Name: "Format settings, GOP", Value: formatGOPLength(info.GOPLength)})
	}
	if duration > 0 {
		fields = addStreamDuration(fields, duration)
		fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
		bitrate := (float64(size) * 8) / duration
		kbps := int64((bitrate / 1000.0) + 0.5)
		if value := formatBitrateKbps(kbps); value != "" {
			fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: value})
		}
		if info.Width > 0 {
			fields = append(fields, Field{Name: "Width", Value: formatPixels(info.Width)})
		}
		if info.Height > 0 {
			fields = append(fields, Field{Name: "Height", Value: formatPixels(info.Height)})
		}
		if info.AspectRatio != "" {
			fields = append(fields, Field{Name: "Display aspect ratio", Value: info.AspectRatio})
		}
		if info.FrameRateNumer > 0 && info.FrameRateDenom > 0 {
			fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRateRatio(info.FrameRateNumer, info.FrameRateDenom)})
		} else if info.FrameRate > 0 {
			fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRate(info.FrameRate)})
		}
		if info.ColorSpace != "" {
			fields = append(fields, Field{Name: "Color space", Value: info.ColorSpace})
		}
		if info.ChromaSubsampling != "" {
			fields = append(fields, Field{Name: "Chroma subsampling", Value: info.ChromaSubsampling})
		}
		if info.BitDepth != "" {
			fields = append(fields, Field{Name: "Bit depth", Value: info.BitDepth})
		}
		if info.ScanType != "" {
			fields = append(fields, Field{Name: "Scan type", Value: info.ScanType})
		}
		fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
		if info.Width > 0 && info.Height > 0 {
			if bits := formatBitsPerPixelFrame(bitrate, info.Width, info.Height, info.FrameRate); bits != "" {
				fields = append(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
			}
		}
		if info.TimeCode != "" {
			fields = append(fields, Field{Name: "Time code of first frame", Value: info.TimeCode})
		}
		if info.GOPOpenClosed != "" {
			fields = append(fields, Field{Name: "GOP, Open/Closed", Value: info.GOPOpenClosed})
		}
		if info.GOPFirstClosed != "" {
			fields = append(fields, Field{Name: "GOP, Open/Closed of first frame", Value: info.GOPFirstClosed})
		}
		if streamSize := formatStreamSize(size, size); streamSize != "" {
			fields = append(fields, Field{Name: "Stream size", Value: streamSize})
		}
	}

	jsonExtras := map[string]string{}
	if duration > 0 {
		jsonDuration := math.Round(duration*1000) / 1000
		if jsonDuration > 0 {
			jsonExtras["BitRate"] = strconv.FormatInt(int64(math.Round((float64(size)*8)/jsonDuration)), 10)
		}
	}
	if size > 0 {
		jsonExtras["StreamSize"] = strconv.FormatInt(size, 10)
	}
	if info.BufferSize > 0 {
		jsonExtras["BufferSize"] = strconv.FormatInt(info.BufferSize, 10)
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
		jsonExtras["Delay"] = "0.000"
		jsonExtras["Delay_Settings"] = fmt.Sprintf("drop_frame_flag=%d / closed_gop=%d / broken_link=%d", drop, closed, broken)
		if drop == 1 {
			jsonExtras["Delay_DropFrame"] = "Yes"
		} else {
			jsonExtras["Delay_DropFrame"] = "No"
		}
		jsonExtras["Delay_Source"] = "Stream"
	}
	jsonRaw := map[string]string{}
	if info.IntraDCPrecision > 0 {
		jsonRaw["extra"] = renderJSONObject([]jsonKV{{Key: "intra_dc_precision", Val: strconv.Itoa(info.IntraDCPrecision)}}, false)
	}

	streams := []Stream{{Kind: StreamVideo, Fields: fields, JSON: jsonExtras, JSONRaw: jsonRaw}}
	container := ContainerInfo{}
	if duration > 0 {
		container.DurationSeconds = duration
		container.BitrateMode = "Variable"
	}
	return container, streams, true
}

func countMPEG2Pictures(data []byte) int {
	count := 0
	for i := 0; i+4 <= len(data); i++ {
		if data[i] == 0x00 && data[i+1] == 0x00 && data[i+2] == 0x01 && data[i+3] == 0x00 {
			count++
		}
	}
	return count
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
