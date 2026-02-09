package mediainfo

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"strconv"
	"strings"
)

type mp3HeaderInfo struct {
	bitrateKbps int
	sampleRate  int
	channels    int
	channelMode byte
	modeExt     byte
	versionID   byte
	layerID     byte
	hasCRC      bool
	padding     bool
}

func ParseMP3(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, map[string]string, map[string]string, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, nil, nil, false
	}

	id3, ok := parseID3v2(file)
	if !ok {
		return ContainerInfo{}, nil, nil, nil, false
	}
	offset := id3.Offset

	dataSize := size - offset
	if dataSize <= 0 {
		return ContainerInfo{}, nil, nil, nil, false
	}

	hasV1 := hasID3v1(file, size)
	if hasV1 {
		dataSize -= 128
	}
	if dataSize <= 0 {
		return ContainerInfo{}, nil, nil, nil, false
	}

	header, xingTag, headerIndex, probe, ok := findMP3Header(file, offset)
	if !ok {
		return ContainerInfo{}, nil, nil, nil, false
	}
	vbr := xingTag == "Xing"

	samplesPerFrame := 1152.0
	if header.versionID != 0x03 {
		samplesPerFrame = 576.0
	}

	generalDuration := 0.0
	audioDuration := 0.0
	frameCount := int64(0)
	payloadBytes := int64(0)
	if header.sampleRate > 0 {
		if xingTag != "" && headerIndex >= 0 && headerIndex < len(probe) {
			if frames, bytes, ok := parseXingInfo(probe[headerIndex:], header, xingTag); ok && frames > 0 {
				frameLen := mp3FrameLengthBytes(header)
				frameCount = frames
				payloadBytes = bytes
				if frameLen > 0 {
					// MediaInfo's StreamSize excludes the initial Info/Xing header frame.
					if payloadBytes > int64(frameLen) {
						payloadBytes -= int64(frameLen)
					} else if payloadBytes == 0 {
						payloadBytes = frameCount * int64(frameLen)
						if payloadBytes > int64(frameLen) {
							payloadBytes -= int64(frameLen)
						}
					}
				}
				audioDuration = (float64(frameCount) * samplesPerFrame) / float64(header.sampleRate)
				generalDuration = audioDuration
			}
		}
	}
	if generalDuration == 0 && header.bitrateKbps > 0 {
		generalDuration = (float64(dataSize) * 8) / (float64(header.bitrateKbps) * 1000)
	}
	if audioDuration == 0 {
		audioDuration = generalDuration
	}
	if frameCount == 0 && audioDuration > 0 && header.sampleRate > 0 {
		frameCount = int64(math.Round(audioDuration * float64(header.sampleRate) / samplesPerFrame))
	}
	if frameCount > 0 && header.sampleRate > 0 {
		// Match MediaInfo: audio duration snaps to whole-frame duration, General duration may differ.
		audioDuration = (float64(frameCount) * samplesPerFrame) / float64(header.sampleRate)
	}

	mode := "Constant"
	if vbr {
		mode = "Variable"
	}
	modeJSON := "CBR"
	if vbr {
		modeJSON = "VBR"
	}

	encodedLibrary := findLAMELibrary(probe)

	info := ContainerInfo{
		DurationSeconds: generalDuration,
		BitrateMode:     mode,
		StreamOverheadBytes: func() int64 {
			overhead := size - dataSize
			if overhead < 0 {
				return 0
			}
			return overhead
		}(),
	}

	fields := []Field{
		{Name: "Format", Value: "MPEG Audio"},
	}
	if header.channels > 0 {
		fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(uint64(header.channels))})
	}
	if header.sampleRate > 0 {
		fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(float64(header.sampleRate))})
	}
	fields = append(fields, Field{Name: "Bit rate mode", Value: mode})
	fields = addStreamCommon(fields, audioDuration, float64(header.bitrateKbps)*1000)

	streamJSON := map[string]string{}
	streamJSON["BitRate_Mode"] = modeJSON
	streamJSON["Compression_Mode"] = "Lossy"
	streamJSON["Format_Profile"] = "Layer 3"
	if header.versionID == 0x03 {
		streamJSON["Format_Version"] = "1"
	}
	streamJSON["SamplesPerFrame"] = strconv.FormatInt(int64(samplesPerFrame), 10)
	if header.bitrateKbps > 0 {
		streamJSON["BitRate"] = strconv.FormatInt(int64(header.bitrateKbps)*1000, 10)
	}
	if header.sampleRate > 0 {
		streamJSON["SamplingRate"] = strconv.Itoa(header.sampleRate)
	}
	if header.channels > 0 {
		streamJSON["Channels"] = strconv.Itoa(header.channels)
	}
	if payloadBytes > 0 {
		streamJSON["StreamSize"] = strconv.FormatInt(payloadBytes, 10)
	} else if dataSize > 0 {
		streamJSON["StreamSize"] = strconv.FormatInt(dataSize, 10)
	}
	if audioDuration > 0 {
		streamJSON["Duration"] = formatJSONSeconds(audioDuration)
	}
	if frameCount > 0 {
		streamJSON["FrameCount"] = strconv.FormatInt(frameCount, 10)
		streamJSON["SamplingCount"] = strconv.FormatInt(frameCount*int64(samplesPerFrame), 10)
		if audioDuration > 0 {
			streamJSON["FrameRate"] = formatJSONFloat(float64(frameCount) / audioDuration)
		}
	}

	// Format settings mode: MediaInfo appears to take it from the first audio frame after an Info header.
	if header.channels == 2 {
		effective := header.channelMode
		effectiveExt := header.modeExt
		if frameLen := mp3FrameLengthBytes(header); headerIndex >= 0 && frameLen > 0 && headerIndex+frameLen+4 <= len(probe) {
			if next, ok := parseMP3Header(probe[headerIndex+frameLen : headerIndex+frameLen+4]); ok {
				if next.sampleRate == header.sampleRate && next.bitrateKbps == header.bitrateKbps && next.channels == header.channels && next.versionID == header.versionID && next.layerID == header.layerID {
					effective = next.channelMode
					effectiveExt = next.modeExt
				}
			}
		}
		if effective == 0x01 {
			streamJSON["Format_Settings_Mode"] = "Joint stereo"
			// Layer III: mode extension bit 1 indicates MS stereo.
			if (effectiveExt & 0x02) != 0 {
				streamJSON["Format_Settings_ModeExtension"] = "MS Stereo"
			} else if (effectiveExt & 0x01) != 0 {
				streamJSON["Format_Settings_ModeExtension"] = "Intensity Stereo"
			}
		}
	}
	if encodedLibrary != "" {
		streamJSON["Encoded_Library"] = encodedLibrary
	}

	generalJSON := map[string]string{}
	if encodedLibrary != "" {
		generalJSON["Encoded_Library"] = encodedLibrary
	}
	generalJSONRaw := map[string]string{}

	streams := []Stream{{Kind: StreamAudio, Fields: fields, JSON: streamJSON, JSONSkipStreamOrder: true, JSONSkipComputed: true}}
	if len(id3.Pictures) > 0 {
		pic := id3.Pictures[0]
		for i := range id3.Pictures {
			if id3.Pictures[i].Type == 0x03 { // Cover (front)
				pic = id3.Pictures[i]
				break
			}
		}
		mime := strings.ToLower(strings.TrimSpace(pic.MIME))

		imgJSON := map[string]string{
			"StreamSize": strconv.FormatInt(pic.DataSize, 10),
		}
		if mime == "image/png" || bytes.HasPrefix(pic.DataHead, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}) {
			imgJSON["Format"] = "PNG"
			imgJSON["Compression_Mode"] = "Lossless"
			imgJSON["Format_Compression"] = "Deflate"
			if info, ok := parsePNGInfo(pic.DataHead); ok {
				if info.Width > 0 {
					imgJSON["Width"] = strconv.Itoa(info.Width)
				}
				if info.Height > 0 {
					imgJSON["Height"] = strconv.Itoa(info.Height)
				}
				if info.BitDepth > 0 {
					imgJSON["BitDepth"] = strconv.Itoa(info.BitDepth)
				}
				if info.ColorSpace != "" {
					imgJSON["ColorSpace"] = info.ColorSpace
				}
			}
		} else {
			imgJSON["Format"] = "JPEG"
			imgJSON["Compression_Mode"] = "Lossy"
			if info, ok := parseJPEGInfo(pic.DataHead); ok {
				if info.Width > 0 {
					imgJSON["Width"] = strconv.Itoa(info.Width)
				}
				if info.Height > 0 {
					imgJSON["Height"] = strconv.Itoa(info.Height)
				}
				if info.BitDepth > 0 {
					imgJSON["BitDepth"] = strconv.Itoa(info.BitDepth)
				}
				if info.ColorSpace != "" {
					imgJSON["ColorSpace"] = info.ColorSpace
				}
				if info.ChromaSubsample != "" {
					imgJSON["ChromaSubsampling"] = info.ChromaSubsample
				}
			}
		}
		streams = append(streams, Stream{Kind: StreamImage, JSON: imgJSON, JSONSkipStreamOrder: true, JSONSkipComputed: true})

		generalJSON["Cover"] = "Yes"
		if pic.Type == 0x03 {
			generalJSON["Cover_Type"] = "Cover (front)"
		} else if pic.Type == 0x04 {
			generalJSON["Cover_Type"] = "Cover (back)"
		}
		if pic.Description != "" {
			generalJSON["Cover_Description"] = pic.Description
		}
		if mime != "" {
			generalJSON["Cover_Mime"] = mime
		} else if pic.MIME != "" {
			generalJSON["Cover_Mime"] = pic.MIME
		}
	}
	if id3.Text != nil {
		applyID3TextToGeneralJSON(generalJSON, generalJSONRaw, id3.Text)
	}

	// MediaInfo appears to omit General OverallBitRate_Mode when a cover is present.
	if xingTag != "" && generalJSON["Cover"] == "" {
		generalJSON["OverallBitRate_Mode"] = modeJSON
	}

	if len(generalJSON) == 0 {
		generalJSON = nil
	}
	if len(generalJSONRaw) == 0 {
		generalJSONRaw = nil
	}
	return info, streams, generalJSON, generalJSONRaw, true
}

func hasID3v1(file io.ReadSeeker, size int64) bool {
	if size < 128 {
		return false
	}
	if _, err := file.Seek(size-128, io.SeekStart); err != nil {
		return false
	}
	var buf [3]byte
	if _, err := io.ReadFull(file, buf[:]); err != nil {
		return false
	}
	return buf[0] == 'T' && buf[1] == 'A' && buf[2] == 'G'
}

func findMP3Header(file io.ReadSeeker, offset int64) (mp3HeaderInfo, string, int, []byte, bool) {
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return mp3HeaderInfo{}, "", 0, nil, false
	}
	buf := make([]byte, 1<<16)
	n, _ := io.ReadFull(file, buf)
	buf = buf[:n]
	for i := 0; i+4 <= len(buf); i++ {
		info, ok := parseMP3Header(buf[i : i+4])
		if !ok {
			continue
		}
		tag := findXingTag(buf[i:], info)
		return info, tag, i, buf, true
	}
	return mp3HeaderInfo{}, "", 0, buf, false
}

func parseMP3Header(header []byte) (mp3HeaderInfo, bool) {
	if len(header) < 4 {
		return mp3HeaderInfo{}, false
	}
	if header[0] != 0xFF || (header[1]&0xE0) != 0xE0 {
		return mp3HeaderInfo{}, false
	}
	versionID := (header[1] >> 3) & 0x03
	layerID := (header[1] >> 1) & 0x03
	hasCRC := (header[1] & 0x01) == 0
	if versionID == 0x01 || layerID == 0x00 {
		return mp3HeaderInfo{}, false
	}
	bitrateIndex := (header[2] >> 4) & 0x0F
	sampleRateIndex := (header[2] >> 2) & 0x03
	if bitrateIndex == 0x00 || bitrateIndex == 0x0F || sampleRateIndex == 0x03 {
		return mp3HeaderInfo{}, false
	}
	bitrate := mp3Bitrate(versionID, layerID, bitrateIndex)
	sampleRate := mp3SampleRate(versionID, sampleRateIndex)
	if bitrate == 0 || sampleRate == 0 {
		return mp3HeaderInfo{}, false
	}
	channelMode := (header[3] >> 6) & 0x03
	modeExt := (header[3] >> 4) & 0x03
	channels := 2
	if channelMode == 0x03 {
		channels = 1
	}
	padding := ((header[2] >> 1) & 0x01) != 0
	return mp3HeaderInfo{
		bitrateKbps: bitrate,
		sampleRate:  sampleRate,
		channels:    channels,
		channelMode: channelMode,
		modeExt:     modeExt,
		versionID:   versionID,
		layerID:     layerID,
		hasCRC:      hasCRC,
		padding:     padding,
	}, true
}

func mp3Bitrate(versionID, layerID, index byte) int {
	if layerID != 0x01 {
		return 0
	}
	var rates []int
	switch versionID {
	case 0x03:
		rates = []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	case 0x02, 0x00:
		rates = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
	default:
		return 0
	}
	idx := int(index)
	if idx < 0 || idx >= len(rates) {
		return 0
	}
	return rates[idx]
}

func mp3SampleRate(versionID, index byte) int {
	var rates []int
	switch versionID {
	case 0x03: // MPEG1
		rates = []int{44100, 48000, 32000}
	case 0x02: // MPEG2
		rates = []int{22050, 24000, 16000}
	case 0x00: // MPEG2.5
		rates = []int{11025, 12000, 8000}
	default:
		return 0
	}
	idx := int(index)
	if idx < 0 || idx >= len(rates) {
		return 0
	}
	return rates[idx]
}

func findXingTag(buf []byte, info mp3HeaderInfo) string {
	if info.layerID != 0x01 {
		return ""
	}
	sideInfo := 32
	if info.versionID != 0x03 {
		sideInfo = 17
	}
	if info.channels == 1 {
		if info.versionID == 0x03 {
			sideInfo = 17
		} else {
			sideInfo = 9
		}
	}
	crcLen := 0
	if info.hasCRC {
		crcLen = 2
	}
	offset := 4 + crcLen + sideInfo
	if len(buf) < offset+4 {
		return ""
	}
	tag := string(buf[offset : offset+4])
	if tag == "Xing" || tag == "Info" {
		return tag
	}
	return ""
}

func parseXingInfo(buf []byte, info mp3HeaderInfo, tag string) (int64, int64, bool) {
	if info.layerID != 0x01 {
		return 0, 0, false
	}
	sideInfo := 32
	if info.versionID != 0x03 {
		sideInfo = 17
	}
	if info.channels == 1 {
		if info.versionID == 0x03 {
			sideInfo = 17
		} else {
			sideInfo = 9
		}
	}
	crcLen := 0
	if info.hasCRC {
		crcLen = 2
	}
	offset := 4 + crcLen + sideInfo
	if len(buf) < offset+8 {
		return 0, 0, false
	}
	if string(buf[offset:offset+4]) != tag {
		return 0, 0, false
	}
	flags := int64(binary.BigEndian.Uint32(buf[offset+4 : offset+8]))
	pos := offset + 8
	frames := int64(0)
	bytes := int64(0)
	if flags&0x0001 != 0 {
		if len(buf) < pos+4 {
			return 0, 0, false
		}
		frames = int64(binary.BigEndian.Uint32(buf[pos : pos+4]))
		pos += 4
	}
	if flags&0x0002 != 0 {
		if len(buf) < pos+4 {
			return 0, 0, false
		}
		bytes = int64(binary.BigEndian.Uint32(buf[pos : pos+4]))
	}
	if frames > 0 {
		return frames, bytes, true
	}
	return 0, 0, false
}

func mp3FrameLengthBytes(info mp3HeaderInfo) int {
	if info.layerID != 0x01 || info.bitrateKbps <= 0 || info.sampleRate <= 0 {
		return 0
	}
	pad := 0
	if info.padding {
		pad = 1
	}
	if info.versionID == 0x03 {
		return (144000*info.bitrateKbps)/info.sampleRate + pad
	}
	return (72000*info.bitrateKbps)/info.sampleRate + pad
}

func validateMP3FrameCount(file io.ReadSeeker, start int64, guess int64, frameLen int, want mp3HeaderInfo) int64 {
	if guess <= 0 || frameLen <= 0 {
		return 0
	}
	// Trim a few frames from the end until we land on a valid header.
	for tries := 0; tries < 8 && guess > 0; tries++ {
		pos := start + (guess-1)*int64(frameLen)
		if hasMP3HeaderAt(file, pos, want) {
			return guess
		}
		guess--
	}
	if guess <= 0 {
		return 0
	}
	return guess
}

func hasMP3HeaderAt(file io.ReadSeeker, pos int64, want mp3HeaderInfo) bool {
	if _, err := file.Seek(pos, io.SeekStart); err != nil {
		return false
	}
	var hdr [4]byte
	if _, err := io.ReadFull(file, hdr[:]); err != nil {
		return false
	}
	info, ok := parseMP3Header(hdr[:])
	if !ok {
		return false
	}
	// Same stream characteristics.
	return info.bitrateKbps == want.bitrateKbps && info.sampleRate == want.sampleRate && info.channels == want.channels && info.versionID == want.versionID && info.layerID == want.layerID
}

func findLAMELibrary(buf []byte) string {
	// Look for "LAME" then parse a compact version string.
	// We return "LAME3.100" style to match MediaInfo.
	idx := bytes.Index(buf, []byte("LAME"))
	if idx < 0 || idx+8 > len(buf) {
		return ""
	}
	rest := buf[idx+4:]
	out := make([]byte, 0, 16)
	out = append(out, []byte("LAME")...)
	i := 0
	for ; i < len(rest) && len(out) < 12; i++ {
		c := rest[i]
		if (c >= '0' && c <= '9') || c == '.' {
			out = append(out, c)
			continue
		}
		break
	}
	if len(out) <= 4 {
		return ""
	}
	// MediaInfo appends a few bytes from the LAME header (ISO-8859-1), e.g. "UUUÀ".
	tail := make([]byte, 0, 4)
	hitNUL := false
	for j := i; j < len(rest) && len(tail) < 4; j++ {
		if rest[j] == 0x00 {
			hitNUL = true
			break
		}
		tail = append(tail, rest[j])
	}
	// If we didn't see a terminator within the short tail window, don't append anything.
	if !hitNUL && i+len(tail) < len(rest) && rest[i+len(tail)] == 0x00 {
		hitNUL = true
	}
	if len(tail) == 0 || !hitNUL {
		return string(out)
	}
	if !(tail[0] >= '0' && tail[0] <= '9') && !(tail[0] >= 'A' && tail[0] <= 'Z') && !(tail[0] >= 'a' && tail[0] <= 'z') {
		return string(out)
	}
	runes := make([]rune, 0, len(out)+len(tail))
	for _, b := range out {
		runes = append(runes, rune(b))
	}
	for _, b := range tail {
		runes = append(runes, rune(b))
	}
	return string(runes)
}

func applyID3TextToGeneralJSON(dst map[string]string, raw map[string]string, text map[string]string) {
	set := func(k, v string) {
		if v != "" && dst[k] == "" {
			dst[k] = v
		}
	}
	if v := text["TALB"]; v != "" {
		set("Album", v)
	}
	if v := text["TPE1"]; v != "" {
		set("Performer", v)
	}
	if v := text["TPE2"]; v != "" {
		set("Album_Performer", v)
	}
	if v := text["TIT2"]; v != "" {
		set("Title", v)
		set("Track", v)
	}
	if v := text["TRCK"]; v != "" {
		set("Track_Position", v)
	}
	if v := text["TCOM"]; v != "" {
		set("Composer", v)
	}
	if v := text["TPE3"]; v != "" {
		set("Conductor", v)
	}
	if v := text["TENC"]; v != "" {
		set("EncodedBy", v)
	}
	if v := text["TCON"]; v != "" {
		set("Genre", v)
	}
	if v := text["TCOP"]; v != "" {
		set("Copyright", v)
	}
	if v := text["TEXT"]; v != "" {
		set("Lyricist", v)
	}
	if v := text["USLT"]; v != "" {
		set("Lyrics", v)
	}
	if v := text["TOLY"]; v != "" {
		set("Original_Lyricist", v)
	}
	if v := text["TOPE"]; v != "" {
		set("Original_Performer", v)
	}
	if v := text["TPE4"]; v != "" {
		set("RemixedBy", v)
	}
	if v := text["TRSN"]; v != "" {
		set("ServiceName", v)
	}
	if v := text["COMM"]; v != "" {
		set("Comment", v)
	}
	if v := text["TPUB"]; v != "" {
		set("Publisher", v)
	}
	if v := text["TPOS"]; v != "" {
		// "1/2" -> "1"
		if i := strings.IndexByte(v, '/'); i > 0 {
			v = v[:i]
		}
		set("Part_Position", strings.TrimSpace(v))
	}

	// Recorded date: prefer full date when available.
	if v := text["TDRC"]; v != "" {
		// Common formats: "YYYY", "YYYY-MM-DD", "YYYY-MM-DDTHH:MM:SS".
		if i := strings.IndexByte(v, 'T'); i > 0 {
			v = v[:i]
		}
		set("Recorded_Date", strings.TrimSpace(v))
	} else if year := text["TYER"]; year != "" && len(year) >= 4 && isAllDigits(year[:4]) {
		if d := text["TDAT"]; len(d) == 4 && isAllDigits(d) {
			day := d[:2]
			mon := d[2:]
			set("Recorded_Date", year[:4]+"-"+mon+"-"+day)
		} else {
			set("Recorded_Date", year[:4])
		}
	}

	// User-defined text frames.
	if v := text["TXXX:ISRC"]; v != "" {
		set("ISRC", v)
	}
	extras := []jsonKV{}
	for k, v := range text {
		if !strings.HasPrefix(k, "WXXX:") {
			continue
		}
		name := strings.TrimPrefix(k, "WXXX:")
		if name == "" || v == "" {
			continue
		}
		if strings.EqualFold(name, "URL") {
			extras = append(extras, jsonKV{Key: "URL", Val: v})
		}
	}
	for k, v := range text {
		if !strings.HasPrefix(k, "TXXX:") {
			continue
		}
		name := strings.TrimPrefix(k, "TXXX:")
		if name == "" || v == "" || name == "ISRC" {
			continue
		}
		extras = append(extras, jsonKV{Key: name, Val: v})
	}
	if len(extras) > 0 && raw != nil {
		raw["extra"] = renderJSONObject(extras, false)
	}
}
