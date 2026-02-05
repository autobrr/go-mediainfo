package mediainfo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

type matroskaTrackStats struct {
	dataBytes  int64
	blockCount int64
	minTimeNs  int64
	maxTimeNs  int64
	maxEndNs   int64
	hasTime    bool
	hasEnd     bool
}

type matroskaAudioProbe struct {
	format       string
	info         ac3Info
	ok           bool
	collect      bool
	targetFrames int
}

type matroskaVideoProbe struct {
	codec         string
	nalLengthSize int
	hdrInfo       hevcHDRInfo
}

const matroskaVideoProbeMaxBytes = 256 * 1024

func (s *matroskaTrackStats) addBlock(timeNs int64, dataBytes int64, durationNs int64, frames int64) {
	if dataBytes > 0 {
		s.dataBytes += dataBytes
	}
	if frames < 1 {
		frames = 1
	}
	s.blockCount += frames
	if !s.hasTime || timeNs < s.minTimeNs {
		s.minTimeNs = timeNs
	}
	if !s.hasTime || timeNs > s.maxTimeNs {
		s.maxTimeNs = timeNs
	}
	s.hasTime = true
	if durationNs > 0 {
		end := timeNs + durationNs
		if !s.hasEnd || end > s.maxEndNs {
			s.maxEndNs = end
		}
		s.hasEnd = true
	}
}

type ebmlReader struct {
	r   *bufio.Reader
	pos int64
}

func newEBMLReader(r io.Reader) *ebmlReader {
	return &ebmlReader{r: bufio.NewReaderSize(r, 256*1024)}
}

func (er *ebmlReader) readByte() (byte, error) {
	b, err := er.r.ReadByte()
	if err != nil {
		return 0, err
	}
	er.pos++
	return b, nil
}

func (er *ebmlReader) readN(n int64) ([]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(er.r, buf); err != nil {
		return nil, err
	}
	er.pos += n
	return buf, nil
}

func (er *ebmlReader) skip(n int64) error {
	if n <= 0 {
		return nil
	}
	if _, err := io.CopyN(io.Discard, er.r, n); err != nil {
		return err
	}
	er.pos += n
	return nil
}

func (er *ebmlReader) readVintID() (uint64, int, error) {
	first, length, err := er.readVintHeader()
	if err != nil {
		return 0, 0, err
	}
	value := uint64(first)
	value, err = er.readVintTail(value, length)
	return value, length, err
}

func (er *ebmlReader) readVintSize() (uint64, int, error) {
	first, length, err := er.readVintHeader()
	if err != nil {
		return 0, 0, err
	}
	mask := byte(0xFF >> length)
	value := uint64(first & mask)
	value, err = er.readVintTail(value, length)
	if err != nil {
		return 0, 0, err
	}
	if value == (uint64(1)<<(uint(length*7)))-1 {
		return unknownVintSize, length, nil
	}
	return value, length, nil
}

func (er *ebmlReader) readVintHeader() (byte, int, error) {
	first, err := er.readByte()
	if err != nil {
		return 0, 0, err
	}
	length := vintLength(first)
	if length == 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	return first, length, nil
}

func (er *ebmlReader) readVintTail(value uint64, length int) (uint64, error) {
	for i := 1; i < length; i++ {
		b, err := er.readByte()
		if err != nil {
			return 0, err
		}
		value = (value << 8) | uint64(b)
	}
	return value, nil
}

func readMatroskaElementHeader(er *ebmlReader, size int64, start int64) (uint64, uint64, error) {
	id, _, err := er.readVintID()
	if err != nil {
		return 0, 0, err
	}
	elemSize, _, err := er.readVintSize()
	if err != nil {
		return 0, 0, err
	}
	if elemSize == unknownVintSize {
		elemSize = uint64(size - (er.pos - start))
	}
	return id, elemSize, nil
}

func scanMatroskaClusters(r io.ReaderAt, offset int64, size int64, timecodeScale uint64, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe) (map[uint64]*matroskaTrackStats, bool) {
	if size <= 0 {
		return nil, false
	}
	reader := io.NewSectionReader(r, offset, size)
	er := newEBMLReader(reader)
	stats := map[uint64]*matroskaTrackStats{}

	for er.pos < size {
		id, elemSize, err := readMatroskaElementHeader(er, size, 0)
		if err != nil {
			break
		}
		switch id {
		case mkvIDCluster:
			if err := scanMatroskaCluster(er, int64(elemSize), int64(timecodeScale), stats, audioProbes, videoProbes); err != nil {
				return stats, len(stats) > 0
			}
		default:
			if err := er.skip(int64(elemSize)); err != nil {
				return stats, len(stats) > 0
			}
		}
	}
	return stats, len(stats) > 0
}

func scanMatroskaCluster(er *ebmlReader, size int64, timecodeScale int64, stats map[uint64]*matroskaTrackStats, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe) error {
	start := er.pos
	var clusterTimecode int64
	for er.pos-start < size {
		id, elemSize, err := readMatroskaElementHeader(er, size, start)
		if err != nil {
			return err
		}
		switch id {
		case mkvIDTimecode:
			payload, err := er.readN(int64(elemSize))
			if err != nil {
				return err
			}
			if value, ok := readUnsigned(payload); ok {
				clusterTimecode = int64(value)
			}
		case mkvIDSimpleBlock:
			if err := scanMatroskaBlock(er, int64(elemSize), clusterTimecode, timecodeScale, stats, audioProbes, videoProbes, 0); err != nil {
				return err
			}
		case mkvIDBlockGroup:
			if err := scanMatroskaBlockGroup(er, int64(elemSize), clusterTimecode, timecodeScale, stats, audioProbes, videoProbes); err != nil {
				return err
			}
		default:
			if err := er.skip(int64(elemSize)); err != nil {
				return err
			}
		}
	}
	return nil
}

func scanMatroskaBlockGroup(er *ebmlReader, size int64, clusterTimecode int64, timecodeScale int64, stats map[uint64]*matroskaTrackStats, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe) error {
	start := er.pos
	var blockTrack uint64
	var blockTimecode int16
	var blockSize int64
	var blockFrames int64
	var hasBlock bool
	var blockDuration uint64
	var payloadSamples [][]byte

	for er.pos-start < size {
		id, elemSize, err := readMatroskaElementHeader(er, size, start)
		if err != nil {
			return err
		}
		switch id {
		case mkvIDBlock:
			track, timecode, dataSize, frames, samples, err := readMatroskaBlockHeader(er, int64(elemSize), audioProbes, videoProbes)
			if err != nil {
				return err
			}
			blockTrack = track
			blockTimecode = timecode
			blockSize = dataSize
			blockFrames = frames
			payloadSamples = samples
			hasBlock = true
		case mkvIDBlockDuration:
			payload, err := er.readN(int64(elemSize))
			if err != nil {
				return err
			}
			if value, ok := readUnsigned(payload); ok {
				blockDuration = value
			}
		default:
			if err := er.skip(int64(elemSize)); err != nil {
				return err
			}
		}
	}

	if hasBlock {
		durationNs := int64(blockDuration) * timecodeScale
		absTime := (clusterTimecode + int64(blockTimecode)) * timecodeScale
		statsForTrack(stats, blockTrack).addBlock(absTime, blockSize, durationNs, blockFrames)
		for _, sample := range payloadSamples {
			probeMatroskaAudio(audioProbes, blockTrack, sample, 1)
			probeMatroskaVideo(videoProbes, blockTrack, sample)
		}
	}
	return nil
}

func scanMatroskaBlock(er *ebmlReader, size int64, clusterTimecode int64, timecodeScale int64, stats map[uint64]*matroskaTrackStats, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe, durationUnits uint64) error {
	track, timecode, dataSize, frames, samples, err := readMatroskaBlockHeader(er, size, audioProbes, videoProbes)
	if err != nil {
		return err
	}
	durationNs := int64(durationUnits) * timecodeScale
	absTime := (clusterTimecode + int64(timecode)) * timecodeScale
	statsForTrack(stats, track).addBlock(absTime, dataSize, durationNs, frames)
	for _, sample := range samples {
		probeMatroskaAudio(audioProbes, track, sample, 1)
		probeMatroskaVideo(videoProbes, track, sample)
	}
	return nil
}

func readMatroskaBlockHeader(er *ebmlReader, size int64, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe) (uint64, int16, int64, int64, [][]byte, error) {
	if size < 4 {
		if err := er.skip(size); err != nil {
			return 0, 0, 0, 0, nil, err
		}
		return 0, 0, 0, 0, nil, io.ErrUnexpectedEOF
	}
	first, err := er.readByte()
	if err != nil {
		return 0, 0, 0, 0, nil, err
	}
	trackLen := vintLength(first)
	if trackLen == 0 {
		return 0, 0, 0, 0, nil, io.ErrUnexpectedEOF
	}
	trackVal := uint64(first & byte(0xFF>>trackLen))
	for i := 1; i < trackLen; i++ {
		b, err := er.readByte()
		if err != nil {
			return 0, 0, 0, 0, nil, err
		}
		trackVal = (trackVal << 8) | uint64(b)
	}
	timeBytes, err := er.readN(2)
	if err != nil {
		return 0, 0, 0, 0, nil, err
	}
	timecode := int16(binary.BigEndian.Uint16(timeBytes))
	flags, err := er.readByte()
	if err != nil { // flags
		return 0, 0, 0, 0, nil, err
	}
	headerLen := int64(trackLen + 3)
	frameCount := int64(1)
	lacing := (flags >> 1) & 0x03
	var laceSizes []int64
	var laceSum int64
	if lacing != 0 {
		countByte, err := er.readByte()
		if err != nil {
			return 0, 0, 0, 0, nil, err
		}
		headerLen++
		frameCount = int64(countByte) + 1
		switch lacing {
		case 1: // Xiph
			laceSizes = make([]int64, frameCount-1)
			for i := int64(0); i < frameCount-1; i++ {
				size := int64(0)
				for {
					b, err := er.readByte()
					if err != nil {
						return 0, 0, 0, 0, nil, err
					}
					headerLen++
					size += int64(b)
					if b != 0xFF {
						break
					}
				}
				laceSizes[i] = size
				laceSum += size
			}
		case 3: // EBML
			readUnsigned := func(first byte) (uint64, int, error) {
				length := vintLength(first)
				if length == 0 {
					return 0, 0, io.ErrUnexpectedEOF
				}
				mask := byte(0xFF >> length)
				value := uint64(first & mask)
				for i := 1; i < length; i++ {
					b, err := er.readByte()
					if err != nil {
						return 0, 0, err
					}
					value = (value << 8) | uint64(b)
				}
				return value, length, nil
			}
			readSigned := func(first byte) (int64, int, error) {
				value, length, err := readUnsigned(first)
				if err != nil {
					return 0, 0, err
				}
				bias := int64(1)<<(uint(length*7-1)) - 1
				return int64(value) - bias, length, nil
			}
			laceSizes = make([]int64, frameCount-1)
			firstSizeByte, err := er.readByte()
			if err != nil {
				return 0, 0, 0, 0, nil, err
			}
			sizeVal, length, err := readUnsigned(firstSizeByte)
			if err != nil {
				return 0, 0, 0, 0, nil, err
			}
			headerLen += int64(length)
			laceSizes[0] = int64(sizeVal)
			laceSum = int64(sizeVal)
			prev := int64(sizeVal)
			for i := int64(1); i < frameCount-1; i++ {
				firstDiff, err := er.readByte()
				if err != nil {
					return 0, 0, 0, 0, nil, err
				}
				diff, length, err := readSigned(firstDiff)
				if err != nil {
					return 0, 0, 0, 0, nil, err
				}
				headerLen += int64(length)
				size := prev + diff
				laceSizes[i] = size
				laceSum += size
				prev = size
			}
		}
	}
	dataSize := size - headerLen
	frameSizes := []int64{dataSize}
	if frameCount > 1 {
		frameSizes = make([]int64, frameCount)
		switch lacing {
		case 1, 3:
			copy(frameSizes, laceSizes)
			last := max(dataSize-laceSum, 0)
			frameSizes[frameCount-1] = last
		case 2:
			if frameCount > 0 {
				frameSize := dataSize / frameCount
				for i := int64(0); i < frameCount; i++ {
					frameSizes[i] = frameSize
				}
			}
		}
	}
	var samples [][]byte
	if dataSize > 0 {
		audioProbe := audioProbes[trackVal]
		videoProbe := videoProbes[trackVal]
		needAudio := audioProbe != nil && (!audioProbe.ok || audioProbe.collect)
		needVideo := videoProbeNeedsSample(videoProbe)
		if needAudio || needVideo {
			samples = make([][]byte, 0, frameCount)
			for i := int64(0); i < frameCount; i++ {
				size := frameSizes[i]
				peek := int64(256)
				if needVideo {
					peek = int64(matroskaVideoProbeMaxBytes)
				} else if needAudio && audioProbe != nil && audioProbe.format == "E-AC-3" {
					peek = size
				}
				peek = min(size, peek)
				payload, err := er.readN(peek)
				if err != nil {
					return 0, 0, 0, 0, nil, err
				}
				samples = append(samples, payload)
				if size > peek {
					if err := er.skip(size - peek); err != nil {
						return 0, 0, 0, 0, nil, err
					}
				}
			}
			return trackVal, timecode, dataSize, frameCount, samples, nil
		}
		if err := er.skip(dataSize); err != nil {
			return 0, 0, 0, 0, nil, err
		}
	}
	return trackVal, timecode, dataSize, frameCount, samples, nil
}

func videoProbeNeedsSample(probe *matroskaVideoProbe) bool {
	if probe == nil {
		return false
	}
	return !probe.hdrInfo.complete()
}

func statsForTrack(stats map[uint64]*matroskaTrackStats, track uint64) *matroskaTrackStats {
	entry := stats[track]
	if entry != nil {
		return entry
	}
	entry = &matroskaTrackStats{}
	stats[track] = entry
	return entry
}

func applyMatroskaStats(info *MatroskaInfo, stats map[uint64]*matroskaTrackStats, fileSize int64) {
	if len(stats) == 0 {
		return
	}
	for i := range info.Tracks {
		trackID := streamTrackNumber(info.Tracks[i])
		if trackID == 0 {
			continue
		}
		stat := stats[trackID]
		if stat == nil {
			continue
		}
		if stat.dataBytes > 0 {
			info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Stream size", formatStreamSize(stat.dataBytes, fileSize))
			if info.Tracks[i].JSON == nil {
				info.Tracks[i].JSON = map[string]string{}
			}
			info.Tracks[i].JSON["StreamSize"] = strconv.FormatInt(stat.dataBytes, 10)
		}
		durationSeconds := matroskaStatsDuration(stat)
		if info.Tracks[i].Kind == StreamVideo && stat.blockCount > 0 {
			if fps, ok := parseFPS(findField(info.Tracks[i].Fields, "Frame rate")); ok && fps > 0 {
				durationSeconds = float64(stat.blockCount) / fps
			}
		}
		if durationSeconds > 0 {
			if info.Tracks[i].Kind == StreamVideo {
				durationSeconds = math.Ceil(durationSeconds*1000) / 1000
			}
			info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Duration", formatDuration(durationSeconds))
			if info.Tracks[i].Kind == StreamText || info.Tracks[i].Kind == StreamVideo {
				if info.Tracks[i].JSON == nil {
					info.Tracks[i].JSON = map[string]string{}
				}
				info.Tracks[i].JSON["Duration"] = fmt.Sprintf("%.9f", durationSeconds)
			}
		}
		if stat.blockCount > 0 {
			if info.Tracks[i].JSON == nil {
				info.Tracks[i].JSON = map[string]string{}
			}
			info.Tracks[i].JSON["FrameCount"] = strconv.FormatInt(stat.blockCount, 10)
			if info.Tracks[i].Kind == StreamText {
				info.Tracks[i].JSON["ElementCount"] = strconv.FormatInt(stat.blockCount, 10)
			}
		}
		if info.Tracks[i].Kind == StreamText {
			if stat.blockCount > 0 {
				info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Count of elements", strconv.FormatInt(stat.blockCount, 10))
			}
			if durationSeconds > 0 && stat.blockCount > 0 {
				frameRate := float64(stat.blockCount) / durationSeconds
				info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Frame rate", formatFrameRate(frameRate))
			}
			if durationSeconds > 0 && stat.dataBytes > 0 {
				bitrate := (float64(stat.dataBytes) * 8) / durationSeconds
				if bitrate < 1000 {
					info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Bit rate", fmt.Sprintf("%.0f b/s", math.Floor(bitrate)))
				} else {
					info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Bit rate", formatBitrateSmall(bitrate))
				}
				if info.Tracks[i].JSON == nil {
					info.Tracks[i].JSON = map[string]string{}
				}
				info.Tracks[i].JSON["BitRate"] = strconv.FormatInt(int64(bitrate), 10)
			}
		}
		if info.Tracks[i].Kind == StreamVideo {
			bitrateDuration := durationSeconds
			if info.Tracks[i].JSON != nil {
				if value, err := strconv.ParseFloat(info.Tracks[i].JSON["Duration"], 64); err == nil && value > 0 {
					bitrateDuration = value
				}
			}
			if bitrateDuration > 0 && stat.dataBytes > 0 {
				bitrate := (float64(stat.dataBytes) * 8) / bitrateDuration
				info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Bit rate", formatBitrate(bitrate))
				if info.Tracks[i].JSON == nil {
					info.Tracks[i].JSON = map[string]string{}
				}
				info.Tracks[i].JSON["BitRate"] = strconv.FormatInt(int64(bitrate), 10)
				width, _ := parsePixels(findField(info.Tracks[i].Fields, "Width"))
				height, _ := parsePixels(findField(info.Tracks[i].Fields, "Height"))
				fps, _ := parseFPS(findField(info.Tracks[i].Fields, "Frame rate"))
				if bits := formatBitsPerPixelFrame(bitrate, width, height, fps); bits != "" {
					info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Bits/(Pixel*Frame)", bits)
				}
			}
		}
		if info.Tracks[i].Kind == StreamAudio {
			if durationSeconds > 0 && stat.dataBytes > 0 && findField(info.Tracks[i].Fields, "Bit rate") == "" {
				bitrate := (float64(stat.dataBytes) * 8) / durationSeconds
				info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Bit rate", formatBitrate(bitrate))
			}
		}
	}
}

func applyMatroskaAudioProbes(info *MatroskaInfo, probes map[uint64]*matroskaAudioProbe) {
	if len(probes) == 0 {
		return
	}
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		if stream.Kind != StreamAudio {
			continue
		}
		trackID := streamTrackNumber(*stream)
		probe := probes[trackID]
		if probe == nil || !probe.ok {
			continue
		}
		ac3 := probe.info
		if probe.format == "E-AC-3" && ac3.comprCount > 0 {
			ac3.comprDB = ac3.comprMax
		}
		if ac3.channels > 0 {
			stream.Fields = setFieldValue(stream.Fields, "Channel(s)", formatChannels(ac3.channels))
		}
		if ac3.layout != "" {
			stream.Fields = setFieldValue(stream.Fields, "Channel layout", ac3.layout)
		}
		if ac3.sampleRate > 0 {
			stream.Fields = setFieldValue(stream.Fields, "Sampling rate", formatSampleRate(ac3.sampleRate))
		}
		if ac3.frameRate > 0 && ac3.spf > 0 {
			stream.Fields = setFieldValue(stream.Fields, "Frame rate", formatAudioFrameRate(ac3.frameRate, ac3.spf))
		}
		if ac3.bitRateKbps > 0 {
			stream.Fields = setFieldValue(stream.Fields, "Bit rate mode", "Constant")
			stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrateKbps(ac3.bitRateKbps))
		}
		stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Compression mode", Value: "Lossy"}, "Stream size")
		if probe.format == "E-AC-3" {
			hasJOC := ac3.hasJOC || ac3.hasJOCComplex || ac3.jocObjects > 0 || ac3.hasJOCDyn || ac3.hasJOCBed
			if hasJOC {
				stream.Fields = setFieldValue(stream.Fields, "Format", "E-AC-3 JOC")
				stream.Fields = setFieldValue(stream.Fields, "Format/Info", "Enhanced AC-3 with Joint Object Coding")
				stream.Fields = setFieldValue(stream.Fields, "Commercial name", "Dolby Digital Plus with Dolby Atmos")
			} else {
				stream.Fields = setFieldValue(stream.Fields, "Commercial name", "Dolby Digital Plus")
			}
		}
		if ac3.serviceKind != "" {
			stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Service kind", Value: ac3.serviceKind}, "Default")
		}
		if probe.format == "E-AC-3" {
			before := "Dialog Normalization"
			complexity := -1
			if ac3.hasJOCComplex {
				complexity = ac3.jocComplexity
			} else {
				fallback := ac3.jocObjects
				if ac3.hasJOCDyn && ac3.jocDynObjects > fallback {
					fallback = ac3.jocDynObjects
				}
				if fallback > 0 {
					complexity = fallback + 1
				}
			}
			if complexity >= 0 {
				stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Complexity index", Value: strconv.Itoa(complexity)}, before)
			}
			if ac3.hasJOCDyn {
				stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Number of dynamic objects", Value: strconv.Itoa(ac3.jocDynObjects)}, before)
			}
			if ac3.hasJOCBed {
				stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Bed channel count", Value: formatChannels(ac3.jocBedCount)}, before)
				stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Bed channel configuration", Value: ac3.jocBedLayout}, before)
			}
		}
		if ac3.hasDialnorm {
			stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Dialog Normalization", Value: formatDialnorm(ac3.dialnorm)}, "Default")
		}
		if ac3.hasCompr {
			comprValue := ac3.comprDB
			if probe.format == "E-AC-3" {
				comprValue = ac3ComprDB(0xFF)
			} else if ac3.hasComprField {
				comprValue = ac3.comprFieldDB
			}
			stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "compr", Value: formatCompr(comprValue)}, "Default")
		}
		if avg, minVal, maxVal, ok := ac3.dialnormStats(); ok {
			stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "dialnorm_Average", Value: formatDialnorm(avg)}, "Default")
			stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "dialnorm_Minimum", Value: formatDialnorm(minVal)}, "Default")
			stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "dialnorm_Maximum", Value: formatDialnorm(maxVal)}, "Default")
		}

		if stream.JSON == nil {
			stream.JSON = map[string]string{}
		}
		if stream.JSONRaw == nil {
			stream.JSONRaw = map[string]string{}
		}
		if ac3.spf > 0 {
			stream.JSON["SamplesPerFrame"] = strconv.Itoa(ac3.spf)
		}
		if ac3.sampleRate > 0 {
			if frameCount, ok := parseInt(stream.JSON["FrameCount"]); ok && ac3.spf > 0 {
				stream.JSON["SamplingCount"] = strconv.FormatInt(frameCount*int64(ac3.spf), 10)
			} else if duration, ok := parseDurationSeconds(findField(stream.Fields, "Duration")); ok {
				samplingCount := int64(math.Round(duration * ac3.sampleRate))
				stream.JSON["SamplingCount"] = strconv.FormatInt(samplingCount, 10)
			}
		}
		if probe.format == "E-AC-3" {
			stream.JSON["Format_Settings_Endianness"] = "Big"
		}
		if code := ac3ServiceKindCode(ac3.bsmod); code != "" {
			stream.JSON["ServiceKind"] = code
		}

		extraFields := []jsonKV{}
		if ac3.bsid > 0 {
			extraFields = append(extraFields, jsonKV{Key: "bsid", Val: strconv.Itoa(ac3.bsid)})
		}
		if ac3.hasDialnorm {
			extraFields = append(extraFields, jsonKV{Key: "dialnorm", Val: strconv.Itoa(ac3.dialnorm)})
		}
		if ac3.hasCompr {
			comprValue := ac3.comprDB
			if probe.format == "E-AC-3" {
				comprValue = ac3ComprDB(0xFF)
			} else if ac3.hasComprField {
				comprValue = ac3.comprFieldDB
			}
			extraFields = append(extraFields, jsonKV{Key: "compr", Val: formatComprRaw(comprValue)})
		}
		if ac3.acmod > 0 {
			extraFields = append(extraFields, jsonKV{Key: "acmod", Val: strconv.Itoa(ac3.acmod)})
		}
		if ac3.lfeon >= 0 {
			extraFields = append(extraFields, jsonKV{Key: "lfeon", Val: strconv.Itoa(ac3.lfeon)})
		}
		if avg, minVal, maxVal, ok := ac3.dialnormStats(); ok {
			extraFields = append(extraFields, jsonKV{Key: "dialnorm_Average", Val: strconv.Itoa(avg)})
			extraFields = append(extraFields, jsonKV{Key: "dialnorm_Minimum", Val: strconv.Itoa(minVal)})
			if maxVal != minVal {
				extraFields = append(extraFields, jsonKV{Key: "dialnorm_Maximum", Val: strconv.Itoa(maxVal)})
			}
		}
		if avg, minVal, maxVal, count, ok := ac3.comprStats(); ok {
			extraFields = append(extraFields, jsonKV{Key: "compr_Average", Val: formatComprRaw(avg)})
			extraFields = append(extraFields, jsonKV{Key: "compr_Minimum", Val: formatComprRaw(minVal)})
			extraFields = append(extraFields, jsonKV{Key: "compr_Maximum", Val: formatComprRaw(maxVal)})
			extraFields = append(extraFields, jsonKV{Key: "compr_Count", Val: strconv.Itoa(count)})
		}
		if len(extraFields) > 0 {
			stream.JSONRaw["extra"] = renderJSONObject(extraFields, false)
		}
	}
}

func applyMatroskaVideoProbes(info *MatroskaInfo, probes map[uint64]*matroskaVideoProbe) {
	if len(probes) == 0 {
		return
	}
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		if stream.Kind != StreamVideo {
			continue
		}
		trackID := streamTrackNumber(*stream)
		probe := probes[trackID]
		if probe == nil {
			continue
		}
		hdr := probe.hdrInfo
		if hdr.masteringPrimaries != "" {
			stream.Fields = setFieldValue(stream.Fields, "Mastering display color primaries", hdr.masteringPrimaries)
		}
		if hdr.masteringLuminanceMin > 0 && hdr.masteringLuminanceMax > 0 {
			stream.Fields = setFieldValue(stream.Fields, "Mastering display luminance", formatMasteringLuminance(hdr.masteringLuminanceMin, hdr.masteringLuminanceMax))
		}
		if hdr.maxCLL > 0 {
			stream.Fields = setFieldValue(stream.Fields, "Maximum Content Light Level", fmt.Sprintf("%d cd/m2", hdr.maxCLL))
		}
		if hdr.maxFALL > 0 {
			stream.Fields = setFieldValue(stream.Fields, "Maximum Frame-Average Light Level", fmt.Sprintf("%d cd/m2", hdr.maxFALL))
		}
		if hdr.hdr10Plus {
			stream.Fields = mergeHDRFormatField(stream.Fields, formatHDR10Plus(hdr))
		}
		hasHDR := hdr.masteringPrimaries != "" || hdr.maxCLL > 0 || hdr.hdr10Plus
		if hdr.masteringPrimaries != "" && findField(stream.Fields, "Color primaries") == "" {
			stream.Fields = setFieldValue(stream.Fields, "Color primaries", hdr.masteringPrimaries)
		}
		if hasHDR && findField(stream.Fields, "Transfer characteristics") == "" {
			stream.Fields = setFieldValue(stream.Fields, "Transfer characteristics", "PQ")
		}
		if hdr.masteringPrimaries == "BT.2020" && findField(stream.Fields, "Matrix coefficients") == "" {
			stream.Fields = setFieldValue(stream.Fields, "Matrix coefficients", "BT.2020 non-constant")
		}
		if hasHDR && findField(stream.Fields, "Color range") == "" {
			stream.Fields = setFieldValue(stream.Fields, "Color range", "Limited")
		}
		if findField(stream.Fields, "Color space") == "" && (findField(stream.Fields, "Color range") != "" || findField(stream.Fields, "Color primaries") != "" || findField(stream.Fields, "Transfer characteristics") != "" || findField(stream.Fields, "Matrix coefficients") != "") {
			stream.Fields = setFieldValue(stream.Fields, "Color space", "YUV")
		}
	}
}

func parseInt(value string) (int64, bool) {
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func formatDialnorm(value int) string {
	return strconv.Itoa(value) + " dB"
}

func formatCompr(value float64) string {
	return formatComprRaw(value) + " dB"
}

func formatComprRaw(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func probeMatroskaAudio(probes map[uint64]*matroskaAudioProbe, track uint64, payload []byte, frames int64) {
	if len(payload) == 0 || probes == nil {
		return
	}
	probe := probes[track]
	if probe == nil || (probe.ok && !probe.collect) {
		return
	}
	if frames < 1 {
		frames = 1
	}
	payload = findAC3Sync(payload)
	if len(payload) == 0 {
		return
	}
	switch probe.format {
	case "AC-3":
		if info, _, ok := parseAC3Frame(payload); ok {
			if frames > 1 {
				factor := float64(frames)
				if info.dialnormCount > 0 {
					info.dialnormCount *= int(frames)
					info.dialnormSum *= factor
				}
				if info.comprCount > 0 {
					info.comprCount *= int(frames)
					info.comprSum *= factor
				}
				if info.dynrngCount > 0 {
					info.dynrngCount *= int(frames)
					info.dynrngSum *= factor
				}
			}
			probe.info = info
			probe.ok = true
		}
	case "E-AC-3":
		if info, frameSize, ok := parseEAC3Frame(payload); ok {
			if !ac3HasJOCInfo(info) {
				offset := 2
				if frameSize > 0 && frameSize < len(payload) {
					offset = frameSize
				}
				for offset+1 < len(payload) {
					sync := bytes.Index(payload[offset:], []byte{0x0B, 0x77})
					if sync < 0 {
						break
					}
					offset += sync
					sub := payload[offset:]
					subInfo, subSize, ok := parseEAC3Frame(sub)
					if ok && ac3HasJOCInfo(subInfo) {
						info.mergeFrame(subInfo)
						break
					}
					if ok && subSize > 0 && subSize < len(sub) {
						offset += subSize
					} else {
						offset += 2
					}
				}
			}
			if info.hasCompr && info.comprDB > -0.56 {
				info.comprCount = 0
				info.comprSumDB = 0
			}
			if frames > 1 {
				factor := float64(frames)
				if info.dialnormCount > 0 {
					info.dialnormCount *= int(frames)
					info.dialnormSum *= factor
				}
				if info.comprCount > 0 {
					info.comprCount *= int(frames)
					if info.comprIsDB {
						info.comprSumDB *= factor
					} else {
						info.comprSum *= factor
					}
				}
			}
			if !probe.ok {
				probe.info = info
				probe.ok = true
			} else {
				probe.info.mergeFrame(info)
			}
			if probe.targetFrames > 0 && probe.info.comprCount >= probe.targetFrames {
				probe.collect = false
			}
		}
	}
}

func probeMatroskaVideo(probes map[uint64]*matroskaVideoProbe, track uint64, payload []byte) {
	if len(payload) == 0 || probes == nil {
		return
	}
	probe := probes[track]
	if probe == nil || !videoProbeNeedsSample(probe) {
		return
	}
	if probe.codec == "HEVC" {
		parseHEVCSampleHDR(payload, probe.nalLengthSize, &probe.hdrInfo)
	}
}

func mergeHDRFormatField(fields []Field, addition string) []Field {
	if addition == "" {
		return fields
	}
	existing := findField(fields, "HDR format")
	if existing == "" {
		return insertFieldBefore(fields, Field{Name: "HDR format", Value: addition}, "Codec ID")
	}
	if strings.Contains(existing, addition) {
		return fields
	}
	return setFieldValue(fields, "HDR format", existing+" / "+addition)
}

func findAC3Sync(payload []byte) []byte {
	if len(payload) < 2 {
		return nil
	}
	for i := 0; i+1 < len(payload); i++ {
		if payload[i] == 0x0B && payload[i+1] == 0x77 {
			return payload[i:]
		}
	}
	return nil
}

func matroskaStatsDuration(stat *matroskaTrackStats) float64 {
	if stat == nil || !stat.hasTime {
		return 0
	}
	end := stat.maxTimeNs
	if stat.hasEnd && stat.maxEndNs > end {
		end = stat.maxEndNs
	}
	if end <= stat.minTimeNs {
		return 0
	}
	return float64(end-stat.minTimeNs) / 1e9
}

func streamTrackNumber(stream Stream) uint64 {
	id := findField(stream.Fields, "ID")
	if id == "" {
		return 0
	}
	value, _ := strconv.ParseUint(id, 10, 64)
	return value
}
