package mediainfo

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"strconv"
	"strings"
)

const (
	maxOggHeaderPacket = 1 << 20
	// Ogg serial numbers are untrusted 32-bit identifiers. A media report cannot
	// usefully expose an arbitrary number of logical streams, so cap retained
	// parser state independently of the packet-size limit.
	maxOggLogicalStreams = 256
)

type oggLogicalStream struct {
	serial         uint32
	format         string
	channels       uint8
	sampleRate     uint32
	width          uint32
	height         uint32
	frameRateNum   uint32
	frameRateDen   uint32
	pixelAspectNum uint32
	pixelAspectDen uint32
	nominalBitRate int64
	derivedBitRate float64
	granuleShift   uint8
	lastGranule    uint64
	payloadBytes   int64
	packetIndex    int
	packet         []byte
	vendor         string
	tags           map[string]string
	firstPage      int
	lastPage       int
	hasPage        bool
	duration       float64
}

// parseOgg scans Ogg page headers and logical-stream headers, retaining exact
// per-stream payload sizes without buffering media packets.
func parseOgg(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, []Field, *canonicalStructuredFacts, *structuredNode, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, nil, nil, nil, false
	}

	logical := make(map[uint32]*oggLogicalStream)
	order := make([]uint32, 0, 2)
	pageOverhead := int64(0)
	pageIndex := 0
	for {
		var header [27]byte
		if _, err := io.ReadFull(file, header[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return ContainerInfo{}, nil, nil, nil, nil, false
		}
		if !bytes.Equal(header[:4], []byte("OggS")) || header[4] != 0 {
			return ContainerInfo{}, nil, nil, nil, nil, false
		}
		segmentCount := int(header[26])
		segments := make([]byte, segmentCount)
		if _, err := io.ReadFull(file, segments); err != nil {
			return ContainerInfo{}, nil, nil, nil, nil, false
		}
		bodySize := 0
		for _, segment := range segments {
			bodySize += int(segment)
		}
		serial := binary.LittleEndian.Uint32(header[14:18])
		stream := logical[serial]
		if stream == nil {
			if len(logical) >= maxOggLogicalStreams {
				if _, err := io.CopyN(io.Discard, file, int64(bodySize)); err != nil {
					return ContainerInfo{}, nil, nil, nil, nil, false
				}
				pageOverhead += int64(len(header) + len(segments) + bodySize)
				pageIndex++
				continue
			}
			stream = &oggLogicalStream{serial: serial, tags: make(map[string]string)}
			logical[serial] = stream
			order = append(order, serial)
		}
		if !stream.hasPage {
			stream.firstPage = pageIndex
			stream.hasPage = true
		}
		stream.lastPage = pageIndex
		pageIndex++
		var body []byte
		if stream.packetIndex < 3 {
			body = make([]byte, bodySize)
			if _, err := io.ReadFull(file, body); err != nil {
				return ContainerInfo{}, nil, nil, nil, nil, false
			}
		} else if _, err := file.Seek(int64(bodySize), io.SeekCurrent); err != nil {
			return ContainerInfo{}, nil, nil, nil, nil, false
		}
		stream.payloadBytes += int64(bodySize)
		pageOverhead += int64(len(header) + len(segments))
		granule := binary.LittleEndian.Uint64(header[6:14])
		if granule != ^uint64(0) {
			stream.lastGranule = granule
		}
		if body != nil {
			consumeOggHeaderSegments(stream, segments, body, header[5]&0x01 != 0)
		}
	}
	for _, serial := range order {
		stream := logical[serial]
		if stream != nil && stream.format == "" {
			pageOverhead += stream.payloadBytes
		}
	}
	derivedAudioBytes := int64(0)
	audioBitRates := make([]int64, 0, len(order))
	var onlyVideo *oggLogicalStream
	videoCount := 0
	for _, serial := range order {
		stream := logical[serial]
		if stream != nil && stream.format == "Theora" {
			videoCount++
			onlyVideo = stream
		}
	}
	for _, serial := range order {
		stream := logical[serial]
		if stream == nil {
			continue
		}
		switch stream.format {
		case "Theora":
			frameCount := oggTheoraFrameCount(stream.lastGranule, stream.granuleShift)
			if frameCount > 0 && stream.frameRateNum > 0 && stream.frameRateDen > 0 {
				stream.duration = float64(frameCount) * float64(stream.frameRateDen) / float64(stream.frameRateNum)
			}
		case "Vorbis", "Opus":
			if stream.lastGranule > 0 && stream.sampleRate > 0 {
				stream.duration = float64(stream.lastGranule) / float64(stream.sampleRate)
			}
			if videoCount > 0 && stream.nominalBitRate > 0 && stream.lastGranule > 0 && stream.sampleRate > 0 {
				duration := stream.duration
				durationMilliseconds := int64(math.Round(duration * 1000))
				stream.payloadBytes = int64(math.Round(float64(stream.nominalBitRate) * float64(durationMilliseconds) / 8000))
			}
			audioBitRates = append(audioBitRates, stream.nominalBitRate)
			derivedAudioBytes += stream.payloadBytes
		}
	}
	maxDuration := oggTimelineDuration(logical, order)
	if videoCount == 1 && onlyVideo != nil {
		if bitRate, ok := estimateOggVideoBitRate(size, maxDuration, audioBitRates); ok {
			onlyVideo.derivedBitRate = bitRate
			frameCount := oggTheoraFrameCount(onlyVideo.lastGranule, onlyVideo.granuleShift)
			if frameCount > 0 && onlyVideo.frameRateNum > 0 && onlyVideo.frameRateDen > 0 {
				duration := float64(frameCount) * float64(onlyVideo.frameRateDen) / float64(onlyVideo.frameRateNum)
				reportedBitRate := math.Round(bitRate)
				reportedDuration := math.Round(duration*1000) / 1000
				onlyVideo.payloadBytes = int64(math.Round(reportedBitRate / 8 * reportedDuration))
			}
		}
	}

	streams := make([]Stream, 0, len(order))
	for _, serial := range order {
		stream := logical[serial]
		if stream == nil || stream.format == "" {
			continue
		}
		switch stream.format {
		case "Theora":
			frameCount := oggTheoraFrameCount(stream.lastGranule, stream.granuleShift)
			duration := 0.0
			if frameCount > 0 && stream.frameRateNum > 0 && stream.frameRateDen > 0 {
				duration = float64(frameCount) * float64(stream.frameRateDen) / float64(stream.frameRateNum)
			}
			streams = append(streams, canonicalOggVideoStream(stream, duration, frameCount))
		case "Vorbis", "Opus":
			duration := 0.0
			if stream.lastGranule > 0 && stream.sampleRate > 0 {
				duration = float64(stream.lastGranule) / float64(stream.sampleRate)
			}
			streams = append(streams, canonicalOggAudioStream(stream, duration, videoCount > 0))
		}
	}
	if len(streams) == 0 {
		return ContainerInfo{}, nil, nil, nil, nil, false
	}

	generalFields, generalFacts, generalExtra := oggGeneralMetadata(logical, order)
	streamOverhead := int64(0)
	if videoCount > 0 {
		streamOverhead = pageOverhead
	}
	if onlyVideo != nil && onlyVideo.payloadBytes > 0 && size >= onlyVideo.payloadBytes+derivedAudioBytes {
		streamOverhead = size - onlyVideo.payloadBytes - derivedAudioBytes
	}
	info := ContainerInfo{DurationSeconds: maxDuration, StreamOverheadBytes: streamOverhead}
	if videoCount > 0 {
		info.BitrateMode = "Variable"
		generalFacts.SetSame("OverallBitRate_Mode", "VBR")
	}
	return info, streams, generalFields, generalFacts, generalExtra, true
}

// oggTimelineDuration sums consecutive chained logical-stream groups while
// retaining the maximum duration of concurrently interleaved streams.
func oggTimelineDuration(logical map[uint32]*oggLogicalStream, order []uint32) float64 {
	total := 0.0
	groupDuration := 0.0
	groupEnd := -1
	for _, serial := range order {
		stream := logical[serial]
		if stream == nil || !stream.hasPage || stream.duration <= 0 {
			continue
		}
		if groupEnd >= 0 && stream.firstPage > groupEnd {
			total += groupDuration
			groupDuration = 0
			groupEnd = -1
		}
		groupDuration = max(groupDuration, stream.duration)
		groupEnd = max(groupEnd, stream.lastPage)
	}
	return total + groupDuration
}

// estimateOggVideoBitRate applies MediaInfoLib's default inter-stream ratios
// when an Ogg container has one video stream and known audio bit rates.
func estimateOggVideoBitRate(size int64, duration float64, audioBitRates []int64) (float64, bool) {
	if size <= 0 || duration < 1 {
		return 0, false
	}
	videoBitRate := float64(size)*8/duration*0.98 - 5000
	for _, audioBitRate := range audioBitRates {
		if audioBitRate <= 0 {
			return 0, false
		}
		videoBitRate -= float64(audioBitRate)/0.98 + 2000
	}
	videoBitRate = videoBitRate*0.98 - 2000
	return videoBitRate, videoBitRate >= 10000
}

func consumeOggHeaderSegments(stream *oggLogicalStream, segments, body []byte, continued bool) {
	if stream == nil {
		return
	}
	if !continued && len(stream.packet) > 0 {
		stream.packet = stream.packet[:0]
	}
	offset := 0
	for _, segment := range segments {
		length := int(segment)
		if offset+length > len(body) {
			return
		}
		if stream.packetIndex < 3 && len(stream.packet)+length <= maxOggHeaderPacket {
			stream.packet = append(stream.packet, body[offset:offset+length]...)
		}
		offset += length
		if segment == 255 {
			continue
		}
		if stream.packetIndex < 3 {
			parseOggHeaderPacket(stream, stream.packet)
		}
		stream.packet = stream.packet[:0]
		stream.packetIndex++
	}
}

func parseOggHeaderPacket(stream *oggLogicalStream, packet []byte) {
	switch {
	case len(packet) >= 30 && packet[0] == 0x01 && bytes.Equal(packet[1:7], []byte("vorbis")):
		stream.format = "Vorbis"
		stream.channels = packet[11]
		stream.sampleRate = binary.LittleEndian.Uint32(packet[12:16])
		stream.nominalBitRate = int64(int32(binary.LittleEndian.Uint32(packet[20:24])))
	case len(packet) >= 42 && packet[0] == 0x80 && bytes.Equal(packet[1:7], []byte("theora")):
		stream.format = "Theora"
		stream.width = readUint24BE(packet[14:17])
		stream.height = readUint24BE(packet[17:20])
		stream.frameRateNum = binary.BigEndian.Uint32(packet[22:26])
		stream.frameRateDen = binary.BigEndian.Uint32(packet[26:30])
		stream.pixelAspectNum = readUint24BE(packet[30:33])
		stream.pixelAspectDen = readUint24BE(packet[33:36])
		stream.nominalBitRate = int64(readUint24BE(packet[37:40]))
		bits := newBitReader(packet[40:])
		_ = bits.readBitsValue(6)
		stream.granuleShift = uint8(bits.readBitsValue(5))
	case len(packet) >= 19 && bytes.HasPrefix(packet, []byte("OpusHead")):
		stream.format = "Opus"
		stream.channels = packet[9]
		stream.sampleRate = 48000
	case len(packet) >= 11 && packet[0] == 0x03 && bytes.Equal(packet[1:7], []byte("vorbis")):
		stream.vendor, stream.tags = parseOggComments(packet[7:])
	case len(packet) >= 11 && packet[0] == 0x81 && bytes.Equal(packet[1:7], []byte("theora")):
		stream.vendor, stream.tags = parseOggComments(packet[7:])
	case bytes.HasPrefix(packet, []byte("OpusTags")):
		stream.vendor, stream.tags = parseOggComments(packet[8:])
	}
}

func parseOggComments(data []byte) (string, map[string]string) {
	vendor, pairs := parseFLACVorbisComment(data)
	tags := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		if pair.Key != "" && pair.Val != "" && tags[pair.Key] == "" {
			tags[pair.Key] = pair.Val
		}
	}
	return vendor, tags
}

func readUint24BE(data []byte) uint32 {
	if len(data) < 3 {
		return 0
	}
	return uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
}

func oggTheoraFrameCount(granule uint64, shift uint8) uint64 {
	if granule == 0 || shift >= 63 {
		return 0
	}
	return (granule >> shift) + (granule & ((uint64(1) << shift) - 1))
}

func canonicalOggVideoStream(stream *oggLogicalStream, duration float64, frameCount uint64) Stream {
	store := &fieldStore{}
	ref := store.Prepare(StreamVideo)
	store.streams[ref].SkipStreamOrder = true
	store.Fill(ref, "ID", strconv.FormatUint(uint64(stream.serial), 10), fillReplace)
	store.Fill(ref, "Format", "Theora", fillReplace)
	if duration > 0 {
		store.Fill(ref, "Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), fillReplace)
	}
	if stream.derivedBitRate > 0 {
		store.Fill(ref, "BitRate", strconv.FormatInt(int64(math.Round(stream.derivedBitRate)), 10), fillReplace)
	} else if duration > 0 && stream.payloadBytes > 0 {
		store.Fill(ref, "BitRate", strconv.FormatInt(int64(math.Round(float64(stream.payloadBytes)*8/duration)), 10), fillReplace)
	}
	if stream.nominalBitRate > 0 {
		store.Fill(ref, "BitRate_Nominal", strconv.FormatInt(stream.nominalBitRate, 10), fillReplace)
	}
	if stream.width > 0 {
		store.Fill(ref, "Width", strconv.FormatUint(uint64(stream.width), 10), fillReplace)
	}
	if stream.height > 0 {
		store.Fill(ref, "Height", strconv.FormatUint(uint64(stream.height), 10), fillReplace)
	}
	pixelAspect := 1.0
	if stream.pixelAspectNum > 0 && stream.pixelAspectDen > 0 {
		pixelAspect = float64(stream.pixelAspectNum) / float64(stream.pixelAspectDen)
	}
	if pixelAspect > 0 {
		store.Fill(ref, "PixelAspectRatio", formatJSONFloat(pixelAspect), fillReplace)
		if stream.width > 0 && stream.height > 0 {
			displayAspect := float64(stream.width) * pixelAspect / float64(stream.height)
			store.Fill(ref, "DisplayAspectRatio", formatJSONFloat(displayAspect), fillReplace)
		}
	}
	if stream.frameRateNum > 0 && stream.frameRateDen > 0 {
		frameRate := float64(stream.frameRateNum) / float64(stream.frameRateDen)
		store.Fill(ref, "FrameRate", formatJSONFloat(frameRate), fillReplace)
		store.Fill(ref, "FrameRate_Num", strconv.FormatUint(uint64(stream.frameRateNum), 10), fillReplace)
		store.Fill(ref, "FrameRate_Den", strconv.FormatUint(uint64(stream.frameRateDen), 10), fillReplace)
	}
	if frameCount > 0 {
		store.Fill(ref, "FrameCount", strconv.FormatUint(frameCount, 10), fillReplace)
	}
	store.Fill(ref, "Compression_Mode", "Lossy", fillReplace)
	if stream.payloadBytes > 0 {
		store.Fill(ref, "StreamSize", strconv.FormatInt(stream.payloadBytes, 10), fillReplace)
	}
	fillOggLibrary(store, ref, "Theora", stream.vendor)
	return canonicalStreamSnapshot(store, ref, canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true})
}

func canonicalOggAudioStream(stream *oggLogicalStream, duration float64, multiplexed bool) Stream {
	store := &fieldStore{}
	ref := store.Prepare(StreamAudio)
	store.streams[ref].SkipStreamOrder = true
	store.Fill(ref, "ID", strconv.FormatUint(uint64(stream.serial), 10), fillReplace)
	store.Fill(ref, "Format", stream.format, fillReplace)
	if stream.format == "Vorbis" {
		store.Fill(ref, "Format_Settings_Floor", "1", fillReplace)
	}
	if duration > 0 {
		store.Fill(ref, "Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), fillReplace)
	}
	if multiplexed && (stream.format == "Vorbis" || stream.format == "Opus") {
		store.Fill(ref, "BitRate_Mode", "Variable", fillReplace)
	}
	if multiplexed && stream.nominalBitRate > 0 {
		store.Fill(ref, "BitRate", strconv.FormatInt(stream.nominalBitRate, 10), fillReplace)
	} else if multiplexed && duration > 0 && stream.payloadBytes > 0 {
		store.Fill(ref, "BitRate", strconv.FormatInt(int64(math.Round(float64(stream.payloadBytes)*8/duration)), 10), fillReplace)
	}
	if stream.channels > 0 {
		channels := strconv.Itoa(int(stream.channels))
		store.Fill(ref, "Channels", channels, fillReplace)
		if !multiplexed {
			if positions := channelPositionsFromCount(channels); positions != "" {
				fillGeneratedStructured(store, ref, "ChannelPositions", positions)
			}
			if layout := channelLayout(uint64(stream.channels)); layout != "" {
				store.Fill(ref, "ChannelLayout", layout, fillReplace)
			}
		}
	}
	if stream.sampleRate > 0 {
		store.Fill(ref, "SamplingRate", strconv.FormatUint(uint64(stream.sampleRate), 10), fillReplace)
	}
	if stream.lastGranule > 0 {
		durationMilliseconds := int64(math.Round(duration * 1000))
		samplingCount := durationMilliseconds * int64(stream.sampleRate) / 1000
		value := strconv.FormatInt(samplingCount, 10)
		fillGeneratedStructured(store, ref, "SamplingCount", value)
	}
	store.Fill(ref, "Compression_Mode", "Lossy", fillReplace)
	if multiplexed && stream.payloadBytes > 0 {
		store.Fill(ref, "StreamSize", strconv.FormatInt(stream.payloadBytes, 10), fillReplace)
	}
	fillOggLibrary(store, ref, stream.format, stream.vendor)
	return canonicalStreamSnapshot(store, ref, canonicalStreamPolicy{SkipStreamOrder: true})
}

func fillOggLibrary(store *fieldStore, ref streamRef, format, vendor string) {
	vendor = strings.TrimSpace(vendor)
	if vendor == "" {
		return
	}
	store.Fill(ref, "Encoded_Library", vendor, fillReplace)
	name, version, date := splitOggLibrary(format, vendor)
	if name != "" {
		store.Fill(ref, "Encoded_Library_Name", name, fillReplace)
	}
	if version != "" {
		store.Fill(ref, "Encoded_Library_Version", version, fillReplace)
	}
	if date != "" {
		store.Fill(ref, "Encoded_Library_Date", date, fillReplace)
	}
}

func splitOggLibrary(format, vendor string) (name, version, date string) {
	parts := strings.Fields(vendor)
	dateIndex := -1
	for index, part := range parts {
		if len(part) == 8 && isAllDigits(part) {
			date = part[:4] + "-" + part[4:6] + "-" + part[6:]
			dateIndex = index
			break
		}
	}
	switch format {
	case "Theora":
		name = "libTheora"
		if dateIndex >= 0 && dateIndex+3 < len(parts) {
			version = strings.Join(parts[dateIndex+1:dateIndex+4], ".")
		}
	case "Vorbis":
		name = "libVorbis"
		switch {
		case strings.Contains(vendor, "20020717"):
			version = "1.0"
		case strings.Contains(vendor, "20070622"):
			version = "1.2"
		}
	case "Opus":
		if strings.Contains(strings.ToLower(vendor), "opus") {
			name = "libopus"
		}
	}
	return name, version, date
}

func formatOggLibraryDisplay(format, vendor string) string {
	name, version, date := splitOggLibrary(format, strings.TrimSpace(vendor))
	if name == "" {
		return strings.TrimSpace(vendor)
	}
	value := name
	if version != "" {
		value += " " + version
	}
	if date != "" {
		value += " (" + date + ")"
	}
	return value
}

func oggGeneralMetadata(logical map[uint32]*oggLogicalStream, order []uint32) ([]Field, *canonicalStructuredFacts, *structuredNode) {
	tags := make(map[string]string)
	for _, serial := range order {
		for key, value := range logical[serial].tags {
			if tags[key] == "" {
				tags[key] = value
			}
		}
	}
	facts := &canonicalStructuredFacts{}
	fields := []Field{}
	if title := tags["TITLE"]; title != "" {
		facts.SetSame("Title", title)
		facts.SetSame("Movie", title)
		fields = append(fields, Field{Name: "Title", Value: title})
	}
	if encoder := tags["ENCODER"]; encoder != "" {
		facts.SetSame("Encoded_Application", encoder)
		fields = append(fields, Field{Name: "Writing application", Value: encoder})
	}
	if license := tags["LICENSE"]; license != "" {
		facts.SetSame("TermsOfUse", license)
		fields = append(fields, Field{Name: "Terms of use", Value: license})
	}
	var extra *structuredNode
	if location := firstNonEmpty(tags["LOCATION"], tags["CONTACT"]); location != "" {
		node := structuredObjectFromKVs([]jsonKV{{Key: "Recorded_Location", Val: location}})
		extra = &node
		fields = append(fields, Field{Name: "Recorded location", Value: location})
	}
	return fields, facts, extra
}
