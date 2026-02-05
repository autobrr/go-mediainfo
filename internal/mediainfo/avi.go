package mediainfo

import (
	"encoding/binary"
	"io"
	"math"
	"strconv"
)

const aviMaxVisualScan = 1 << 20

type aviMainHeader struct {
	microSecPerFrame uint32
	maxBytesPerSec   uint32
	totalFrames      uint32
	streams          uint32
	width            uint32
	height           uint32
}

type aviStream struct {
	index        int
	kind         StreamKind
	handler      string
	compression  string
	scale        uint32
	rate         uint32
	length       uint32
	width        uint32
	height       uint32
	bitCount     uint16
	bytes        uint64
	writingLib   string
	profile      string
	bvop         *bool
	qpel         *bool
	gmc          string
	matrix       string
	colorSpace   string
	chroma       string
	bitDepth     string
	scanType     string
	hasVideoInfo bool
}

type vopScanner struct {
	carry []byte
	bvop  *bool
}

func (s *vopScanner) feed(data []byte) {
	if s.bvop != nil && *s.bvop {
		return
	}
	buf := append(append([]byte{}, s.carry...), data...)
	stop := false
	scanMPEG2StartCodes(buf, 0, func(i int, code byte) bool {
		if code != 0xB6 {
			return true
		}
		if i+4 >= len(buf) {
			s.carry = append([]byte{}, buf[i:]...)
			stop = true
			return false
		}
		vopType := (buf[i+4] >> 6) & 0x03
		if vopType == 2 {
			val := true
			s.bvop = &val
			stop = true
			return false
		}
		return true
	})
	if stop {
		return
	}
	if len(buf) >= 3 {
		s.carry = append([]byte{}, buf[len(buf)-3:]...)
	} else {
		s.carry = append([]byte{}, buf...)
	}
}

func ParseAVI(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, []Field, bool) {
	if size < 12 {
		return ContainerInfo{}, nil, nil, false
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, nil, false
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(file, header); err != nil {
		return ContainerInfo{}, nil, nil, false
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "AVI " {
		return ContainerInfo{}, nil, nil, false
	}

	var main aviMainHeader
	streams := []*aviStream{}
	var writingApp string
	var videoData []byte
	var vopScan vopScanner
	setFormatSettings := false

	offset := int64(12)
	for offset+8 <= size {
		var chunkHeader [8]byte
		if _, err := readAt(file, offset, chunkHeader[:]); err != nil {
			break
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))
		dataStart := offset + 8
		dataEnd := dataStart + chunkSize
		if dataEnd > size {
			break
		}
		if chunkID == "LIST" {
			var listTypeBytes [4]byte
			if _, err := readAt(file, dataStart, listTypeBytes[:]); err != nil {
				break
			}
			listType := string(listTypeBytes[:])
			listDataStart := dataStart + 4
			listDataSize := chunkSize - 4
			switch listType {
			case "hdrl":
				listData := make([]byte, listDataSize)
				if _, err := readAt(file, listDataStart, listData); err != nil {
					break
				}
				parseAVIHDRL(listData, &main, &streams)
				if len(streams) > 0 {
					setFormatSettings = true
				}
			case "INFO":
				listData := make([]byte, listDataSize)
				if _, err := readAt(file, listDataStart, listData); err != nil {
					break
				}
				if app := parseAVIINFO(listData); app != "" {
					writingApp = app
				}
			case "movi":
				parseAVIMovi(file, listDataStart, dataEnd, streams, &videoData, &vopScan)
			}
		}
		pad := chunkSize % 2
		offset = dataEnd + pad
	}

	if len(streams) == 0 {
		return ContainerInfo{}, nil, nil, false
	}

	var videoDuration float64
	var streamsOut []Stream
	for _, st := range streams {
		if st.kind == StreamVideo {
			duration := aviStreamDuration(st, main)
			if duration > 0 {
				videoDuration = duration
			}
		}
	}

	if len(videoData) > 0 {
		info := parseMPEG4Visual(videoData)
		for _, st := range streams {
			if st.kind != StreamVideo {
				continue
			}
			if st.handler == "FMP4" || st.compression == "FMP4" || st.compression == "MP4V" || st.compression == "DIVX" || st.compression == "XVID" {
				if info.Profile != "" {
					st.profile = info.Profile
				}
				if info.WritingLibrary != "" {
					st.writingLib = info.WritingLibrary
				}
				st.qpel = info.QPel
				st.bvop = info.BVOP
				st.gmc = info.GMC
				st.matrix = info.Matrix
				st.colorSpace = info.ColorSpace
				st.chroma = info.ChromaSubsampling
				st.bitDepth = info.BitDepth
				st.scanType = info.ScanType
				st.hasVideoInfo = true
			}
		}
	}
	if vopScan.bvop != nil {
		for _, st := range streams {
			if st.kind == StreamVideo {
				st.bvop = vopScan.bvop
			}
		}
	}

	for _, st := range streams {
		fields := []Field{}
		if st.kind == StreamVideo {
			var jsonExtras map[string]string
			fields = append(fields, Field{Name: "ID", Value: strconv.Itoa(st.index)})
			if format := mapAVICompression(st); format != "" {
				fields = append(fields, Field{Name: "Format", Value: format})
			}
			if st.profile != "" {
				fields = append(fields, Field{Name: "Format profile", Value: st.profile})
			}
			if st.bvop != nil {
				fields = append(fields, Field{Name: "Format settings, BVOP", Value: formatYesNo(*st.bvop)})
			}
			if st.qpel != nil {
				fields = append(fields, Field{Name: "Format settings, QPel", Value: formatYesNo(*st.qpel)})
			}
			if st.gmc != "" {
				fields = append(fields, Field{Name: "Format settings, GMC", Value: st.gmc})
			}
			if st.matrix != "" {
				fields = append(fields, Field{Name: "Format settings, Matrix", Value: st.matrix})
			}
			if st.handler != "" {
				fields = append(fields, Field{Name: "Codec ID", Value: st.handler})
			}
			duration := aviStreamDuration(st, main)
			if duration > 0 {
				fields = addStreamDuration(fields, duration)
			}
			if st.bytes > 0 && duration > 0 {
				bitrate := (float64(st.bytes) * 8) / duration
				fields = addStreamBitrate(fields, bitrate)
				if jsonExtras == nil {
					jsonExtras = map[string]string{}
				}
				jsonExtras["BitRate"] = strconv.FormatInt(int64(math.Round(bitrate)), 10)
				if st.width > 0 && st.height > 0 {
					frameRate := aviFrameRate(st)
					if frameRate > 0 {
						if bits := formatBitsPerPixelFrame(bitrate, uint64(st.width), uint64(st.height), frameRate); bits != "" {
							fields = append(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
						}
					}
				}
			}
			if st.width > 0 {
				fields = append(fields, Field{Name: "Width", Value: formatPixels(uint64(st.width))})
			}
			if st.height > 0 {
				fields = append(fields, Field{Name: "Height", Value: formatPixels(uint64(st.height))})
			}
			if st.width > 0 && st.height > 0 {
				if ar := formatAspectRatio(uint64(st.width), uint64(st.height)); ar != "" {
					fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
				}
			}
			if frameRate := aviFrameRate(st); frameRate > 0 {
				fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRateRatio(st.rate, st.scale)})
			}
			if st.colorSpace != "" {
				fields = append(fields, Field{Name: "Color space", Value: st.colorSpace})
			}
			if st.chroma != "" {
				fields = append(fields, Field{Name: "Chroma subsampling", Value: st.chroma})
			}
			if st.bitDepth != "" {
				fields = append(fields, Field{Name: "Bit depth", Value: st.bitDepth})
			}
			if st.scanType != "" {
				fields = append(fields, Field{Name: "Scan type", Value: st.scanType})
			}
			fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
			if st.bytes > 0 {
				if streamSize := formatStreamSize(int64(st.bytes), size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
				if jsonExtras == nil {
					jsonExtras = map[string]string{}
				}
				jsonExtras["StreamSize"] = strconv.FormatUint(st.bytes, 10)
			}
			if st.writingLib != "" {
				fields = append(fields, Field{Name: "Writing library", Value: st.writingLib})
			}
			streamsOut = append(streamsOut, Stream{Kind: StreamVideo, Fields: fields, JSON: jsonExtras})
		}
	}

	info := ContainerInfo{}
	if videoDuration > 0 {
		info.DurationSeconds = videoDuration
	}

	generalFields := []Field{}
	generalFields = append(generalFields, Field{Name: "Format/Info", Value: "Audio Video Interleave"})
	if setFormatSettings {
		generalFields = append(generalFields, Field{Name: "Format settings", Value: "BitmapInfoHeader"})
	}
	if rate := firstAVIFrameRate(streams); rate > 0 {
		generalFields = append(generalFields, Field{Name: "Frame rate", Value: formatFrameRate(rate)})
	}
	if writingApp != "" {
		generalFields = append(generalFields, Field{Name: "Writing application", Value: writingApp})
	}

	return info, streamsOut, generalFields, true
}

func parseAVIHDRL(data []byte, main *aviMainHeader, streams *[]*aviStream) {
	parseRIFFChunks(data, func(id string, payload []byte) {
		switch id {
		case "avih":
			if len(payload) < 40 {
				return
			}
			main.microSecPerFrame = binary.LittleEndian.Uint32(payload[0:4])
			main.maxBytesPerSec = binary.LittleEndian.Uint32(payload[4:8])
			main.totalFrames = binary.LittleEndian.Uint32(payload[16:20])
			main.streams = binary.LittleEndian.Uint32(payload[24:28])
			main.width = binary.LittleEndian.Uint32(payload[32:36])
			main.height = binary.LittleEndian.Uint32(payload[36:40])
		case "LIST":
			if len(payload) < 4 {
				return
			}
			listType := string(payload[0:4])
			if listType != "strl" {
				return
			}
			stream := parseAVIStrl(payload[4:], len(*streams))
			if stream != nil {
				*streams = append(*streams, stream)
			}
		}
	})
}

func parseAVIStrl(data []byte, index int) *aviStream {
	stream := &aviStream{index: index}
	parseRIFFChunks(data, func(id string, payload []byte) {
		switch id {
		case "strh":
			parseAVIStrh(payload, stream)
		case "strf":
			parseAVIStrf(payload, stream)
		}
	})
	if stream.kind == "" {
		return nil
	}
	return stream
}

func parseAVIStrh(payload []byte, stream *aviStream) {
	if len(payload) < 56 {
		return
	}
	fccType := string(payload[0:4])
	fccHandler := string(payload[4:8])
	stream.handler = fccHandler
	stream.scale = binary.LittleEndian.Uint32(payload[20:24])
	stream.rate = binary.LittleEndian.Uint32(payload[24:28])
	stream.length = binary.LittleEndian.Uint32(payload[32:36])
	switch fccType {
	case "vids":
		stream.kind = StreamVideo
	case "auds":
		stream.kind = StreamAudio
	case "txts":
		stream.kind = StreamText
	}
}

func parseAVIStrf(payload []byte, stream *aviStream) {
	if len(payload) < 16 {
		return
	}
	if stream.kind == StreamVideo {
		if len(payload) < 40 {
			return
		}
		stream.width = binary.LittleEndian.Uint32(payload[4:8])
		stream.height = binary.LittleEndian.Uint32(payload[8:12])
		stream.bitCount = binary.LittleEndian.Uint16(payload[14:16])
		compression := binary.LittleEndian.Uint32(payload[16:20])
		stream.compression = fourCC(compression)
	}
}

func parseAVIINFO(data []byte) string {
	var writingApp string
	parseRIFFChunks(data, func(id string, payload []byte) {
		if id != "ISFT" {
			return
		}
		writingApp = trimNullString(payload)
	})
	return writingApp
}

func parseAVIMovi(file io.ReadSeeker, start, end int64, streams []*aviStream, videoData *[]byte, vopScan *vopScanner) {
	const aviScanChunk = 256 << 10
	scanBuf := make([]byte, aviScanChunk)
	offset := start
	for offset+8 <= end {
		var header [8]byte
		if _, err := readAt(file, offset, header[:]); err != nil {
			break
		}
		chunkID := string(header[0:4])
		chunkSize := int64(binary.LittleEndian.Uint32(header[4:8]))
		dataStart := offset + 8
		dataEnd := dataStart + chunkSize
		if dataEnd > end {
			break
		}
		if len(chunkID) == 4 {
			if index, ok := parseAVIStreamIndex(chunkID); ok {
				if index >= 0 && index < len(streams) {
					stream := streams[index]
					stream.bytes += uint64(chunkSize)
					if stream.kind == StreamVideo && chunkSize > 0 {
						needVOP := vopScan != nil && vopScan.bvop == nil
						needVisual := videoData != nil && len(*videoData) < aviMaxVisualScan
						if needVOP || needVisual {
							remainingVisual := 0
							if needVisual {
								remainingVisual = aviMaxVisualScan - len(*videoData)
							}
							remainingChunk := chunkSize
							readPos := dataStart
							for remainingChunk > 0 && (needVOP || remainingVisual > 0) {
								readLen := int64(len(scanBuf))
								if readLen > remainingChunk {
									readLen = remainingChunk
								}
								buf := scanBuf[:readLen]
								if _, err := readAt(file, readPos, buf); err != nil {
									break
								}
								if needVOP {
									vopScan.feed(buf)
									if vopScan.bvop != nil {
										needVOP = false
									}
								}
								if remainingVisual > 0 {
									take := len(buf)
									if take > remainingVisual {
										take = remainingVisual
									}
									*videoData = append(*videoData, buf[:take]...)
									remainingVisual -= take
								}
								remainingChunk -= readLen
								readPos += readLen
							}
						}
					}
				}
			}
		}
		pad := chunkSize % 2
		offset = dataEnd + pad
	}
}

func aviStreamDuration(stream *aviStream, main aviMainHeader) float64 {
	if stream.rate > 0 && stream.scale > 0 && stream.length > 0 {
		return float64(stream.length) * float64(stream.scale) / float64(stream.rate)
	}
	if main.microSecPerFrame > 0 && main.totalFrames > 0 {
		return float64(main.microSecPerFrame*main.totalFrames) / 1e6
	}
	return 0
}

func aviFrameRate(stream *aviStream) float64 {
	if stream.rate > 0 && stream.scale > 0 {
		return float64(stream.rate) / float64(stream.scale)
	}
	return 0
}

func firstAVIFrameRate(streams []*aviStream) float64 {
	for _, st := range streams {
		if st.kind == StreamVideo {
			if rate := aviFrameRate(st); rate > 0 {
				return rate
			}
		}
	}
	return 0
}

func mapAVICompression(stream *aviStream) string {
	code := stream.handler
	if code == "" {
		code = stream.compression
	}
	switch code {
	case "FMP4", "MP4V", "DIVX", "XVID":
		return "MPEG-4 Visual"
	case "H264", "AVC1":
		return "AVC"
	case "MJPG":
		return "Motion JPEG"
	default:
		return code
	}
}

func parseAVIStreamIndex(id string) (int, bool) {
	if len(id) != 4 {
		return 0, false
	}
	if id[0] < '0' || id[0] > '9' || id[1] < '0' || id[1] > '9' {
		return 0, false
	}
	return int(id[0]-'0')*10 + int(id[1]-'0'), true
}

func parseRIFFChunks(data []byte, fn func(id string, payload []byte)) {
	pos := 0
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		end := start + size
		if end > len(data) {
			return
		}
		fn(id, data[start:end])
		if size%2 == 1 {
			end++
		}
		pos = end
	}
}

func readAt(file io.ReadSeeker, offset int64, buf []byte) (int, error) {
	if readerAt, ok := file.(io.ReaderAt); ok {
		return readAtReaderAt(readerAt, offset, buf)
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	return io.ReadFull(file, buf)
}

func readAtReaderAt(readerAt io.ReaderAt, offset int64, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := readerAt.ReadAt(buf[total:], offset+int64(total))
		total += n
		if err != nil {
			if err == io.EOF && total == len(buf) {
				return total, nil
			}
			return total, err
		}
		if n == 0 {
			return total, io.EOF
		}
	}
	return total, nil
}

func fourCC(value uint32) string {
	b := []byte{byte(value), byte(value >> 8), byte(value >> 16), byte(value >> 24)}
	return string(b)
}

func formatYesNo(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func trimNullString(data []byte) string {
	for i, b := range data {
		if b == 0x00 {
			return string(data[:i])
		}
	}
	return string(data)
}
