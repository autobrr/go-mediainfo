package mediainfo

import (
	"bufio"
	"bytes"
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

type matroskaTagStats struct {
	trusted         bool
	dataBytes       int64
	hasDataBytes    bool
	frameCount      int64
	hasFrameCount   bool
	durationSeconds float64
	hasDuration     bool
	bitRate         int64
	hasBitRate      bool
}

type matroskaAudioProbe struct {
	format        string
	info          ac3Info
	ok            bool
	collect       bool
	targetFrames  int
	targetPackets int
	packetCount   int
	parseJOC      bool
	headerStrip   []byte
}

type matroskaVideoProbe struct {
	codec         string
	nalLengthSize int
	hdrInfo       hevcHDRInfo
	headerStrip   []byte
	packetCount   int
	targetPackets int
	exhausted     bool
}

const matroskaVideoProbeMaxBytes = 256 * 1024
const ebmlSkipSeekMin = 4 * 1024

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
	rs  io.ReadSeeker
	pos int64
	tmp []byte
}

func newEBMLReader(rs io.ReadSeeker) *ebmlReader {
	return &ebmlReader{
		rs: rs,
		r:  bufio.NewReaderSize(rs, 1024*1024),
	}
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
	var buf []byte
	if n <= 4096 {
		need := int(n)
		if cap(er.tmp) < need {
			er.tmp = make([]byte, need)
		}
		buf = er.tmp[:need]
	} else {
		buf = make([]byte, n)
	}
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
	if er.rs != nil {
		// Never read bytes just to drop them: consume what's already buffered, seek the rest.
		if buffered := er.r.Buffered(); buffered > 0 {
			toDiscard := int64(buffered)
			if toDiscard > n {
				toDiscard = n
			}
			discarded, err := er.r.Discard(int(toDiscard))
			er.pos += int64(discarded)
			n -= int64(discarded)
			if err != nil && err != bufio.ErrBufferFull {
				return err
			}
			if n <= 0 {
				return nil
			}
		}
		if n >= ebmlSkipSeekMin {
			if _, err := er.rs.Seek(er.pos+n, io.SeekStart); err == nil {
				er.pos += n
				er.r.Reset(er.rs)
				return nil
			}
		}
	}
	for n > 0 {
		chunk := n
		if chunk > int64(int(^uint(0)>>1)) {
			chunk = int64(int(^uint(0) >> 1))
		}
		discarded, err := er.r.Discard(int(chunk))
		er.pos += int64(discarded)
		n -= int64(discarded)
		if err != nil {
			if err == bufio.ErrBufferFull {
				continue
			}
			return err
		}
	}
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

func scanMatroskaClusters(r io.ReaderAt, offset int64, size int64, timecodeScale uint64, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe, applyStats bool) (map[uint64]*matroskaTrackStats, bool) {
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
			if !applyStats && matroskaProbesComplete(audioProbes, videoProbes) {
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

func matroskaProbesComplete(audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe) bool {
	for _, probe := range audioProbes {
		if probe == nil {
			continue
		}
		if probe.collect {
			return false
		}
	}
	for _, probe := range videoProbes {
		if videoProbeNeedsSample(probe) {
			return false
		}
	}
	return true
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

	for er.pos-start < size {
		id, elemSize, err := readMatroskaElementHeader(er, size, start)
		if err != nil {
			return err
		}
		switch id {
		case mkvIDBlock:
			track, timecode, dataSize, frames, err := readMatroskaBlockHeader(er, int64(elemSize), audioProbes, videoProbes)
			if err != nil {
				return err
			}
			blockTrack = track
			blockTimecode = timecode
			blockSize = dataSize
			blockFrames = frames
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
	}
	return nil
}

func scanMatroskaBlock(er *ebmlReader, size int64, clusterTimecode int64, timecodeScale int64, stats map[uint64]*matroskaTrackStats, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe, durationUnits uint64) error {
	track, timecode, dataSize, frames, err := readMatroskaBlockHeader(er, size, audioProbes, videoProbes)
	if err != nil {
		return err
	}
	durationNs := int64(durationUnits) * timecodeScale
	absTime := (clusterTimecode + int64(timecode)) * timecodeScale
	statsForTrack(stats, track).addBlock(absTime, dataSize, durationNs, frames)
	return nil
}

func readMatroskaBlockHeader(er *ebmlReader, size int64, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe) (uint64, int16, int64, int64, error) {
	if size < 4 {
		if err := er.skip(size); err != nil {
			return 0, 0, 0, 0, err
		}
		return 0, 0, 0, 0, io.ErrUnexpectedEOF
	}
	first, err := er.readByte()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	trackLen := vintLength(first)
	if trackLen == 0 {
		return 0, 0, 0, 0, io.ErrUnexpectedEOF
	}
	trackVal := uint64(first & byte(0xFF>>trackLen))
	for i := 1; i < trackLen; i++ {
		b, err := er.readByte()
		if err != nil {
			return 0, 0, 0, 0, err
		}
		trackVal = (trackVal << 8) | uint64(b)
	}
	tb1, err := er.readByte()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	tb2, err := er.readByte()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	timecode := int16(uint16(tb1)<<8 | uint16(tb2))
	flags, err := er.readByte()
	if err != nil { // flags
		return 0, 0, 0, 0, err
	}
	headerLen := int64(trackLen + 3)
	frameCount := int64(1)
	lacing := (flags >> 1) & 0x03
	audioProbe := audioProbes[trackVal]
	videoProbe := videoProbes[trackVal]
	needAudio := audioProbe != nil && (!audioProbe.ok || audioProbe.collect)
	needVideo := videoProbeNeedsSample(videoProbe)
	needProbePayload := needAudio || needVideo
	var laceSizes []int64
	var laceSum int64
	if lacing != 0 {
		countByte, err := er.readByte()
		if err != nil {
			return 0, 0, 0, 0, err
		}
		headerLen++
		frameCount = int64(countByte) + 1
		switch lacing {
		case 1: // Xiph
			if needProbePayload && frameCount > 1 {
				laceSizes = make([]int64, frameCount-1)
			}
			for i := int64(0); i < frameCount-1; i++ {
				size := int64(0)
				for {
					b, err := er.readByte()
					if err != nil {
						return 0, 0, 0, 0, err
					}
					headerLen++
					size += int64(b)
					if b != 0xFF {
						break
					}
				}
				if needProbePayload {
					laceSizes[i] = size
				}
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
			if needProbePayload && frameCount > 1 {
				laceSizes = make([]int64, frameCount-1)
			}
			firstSizeByte, err := er.readByte()
			if err != nil {
				return 0, 0, 0, 0, err
			}
			sizeVal, length, err := readUnsigned(firstSizeByte)
			if err != nil {
				return 0, 0, 0, 0, err
			}
			headerLen += int64(length)
			if needProbePayload {
				laceSizes[0] = int64(sizeVal)
			}
			laceSum = int64(sizeVal)
			prev := int64(sizeVal)
			for i := int64(1); i < frameCount-1; i++ {
				firstDiff, err := er.readByte()
				if err != nil {
					return 0, 0, 0, 0, err
				}
				diff, length, err := readSigned(firstDiff)
				if err != nil {
					return 0, 0, 0, 0, err
				}
				headerLen += int64(length)
				size := prev + diff
				if needProbePayload {
					laceSizes[i] = size
				}
				laceSum += size
				prev = size
			}
		}
	}
	dataSize := size - headerLen
	if dataSize > 0 {
		if needProbePayload {
			for i := int64(0); i < frameCount; i++ {
				size := dataSize
				if frameCount > 1 {
					switch lacing {
					case 1, 3:
						if i < frameCount-1 {
							size = laceSizes[i]
						} else {
							size = max(dataSize-laceSum, 0)
						}
					case 2:
						size = dataSize / frameCount
					}
				}
				peek := int64(256)
				if needVideo {
					peek = int64(matroskaVideoProbeMaxBytes)
				} else if needAudio && audioProbe != nil && audioProbe.format == "E-AC-3" {
					if audioProbe.parseJOC {
						peek = size
					}
				}
				peek = min(size, peek)
				payload, err := er.readN(peek)
				if err != nil {
					return 0, 0, 0, 0, err
				}
				if needAudio {
					audioPayload := applyMatroskaAudioHeaderStrip(payload, audioProbe)
					probeMatroskaAudio(audioProbes, trackVal, audioPayload, 1)
				}
				if needVideo {
					videoPayload := applyMatroskaVideoHeaderStrip(payload, videoProbe)
					probeMatroskaVideo(videoProbes, trackVal, videoPayload)
				}
				if needVideo && videoProbe != nil && videoProbe.targetPackets > 0 {
					videoProbe.packetCount++
					if videoProbe.packetCount >= videoProbe.targetPackets {
						videoProbe.exhausted = true
					}
				}
				if size > peek {
					if err := er.skip(size - peek); err != nil {
						return 0, 0, 0, 0, err
					}
				}
			}
			if needAudio && audioProbe != nil && audioProbe.format == "E-AC-3" && audioProbe.targetPackets > 0 {
				audioProbe.packetCount++
				if audioProbe.packetCount >= audioProbe.targetPackets {
					audioProbe.collect = false
				}
			}
			return trackVal, timecode, dataSize, frameCount, nil
		}
		if err := er.skip(dataSize); err != nil {
			return 0, 0, 0, 0, err
		}
	}
	return trackVal, timecode, dataSize, frameCount, nil
}

func videoProbeNeedsSample(probe *matroskaVideoProbe) bool {
	if probe == nil {
		return false
	}
	if probe.exhausted {
		return false
	}
	return !probe.hdrInfo.complete()
}

func applyMatroskaAudioHeaderStrip(payload []byte, probe *matroskaAudioProbe) []byte {
	if probe == nil {
		return payload
	}
	prefix := probe.headerStrip
	if len(prefix) == 0 || len(payload) == 0 {
		return payload
	}
	combined := make([]byte, len(prefix)+len(payload))
	copy(combined, prefix)
	copy(combined[len(prefix):], payload)
	return combined
}

func applyMatroskaVideoHeaderStrip(payload []byte, probe *matroskaVideoProbe) []byte {
	if probe == nil {
		return payload
	}
	prefix := probe.headerStrip
	if len(prefix) == 0 || len(payload) == 0 {
		return payload
	}
	combined := make([]byte, len(prefix)+len(payload))
	copy(combined, prefix)
	copy(combined[len(prefix):], payload)
	return combined
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

func applyMatroskaTagStats(info *MatroskaInfo, tagStats map[uint64]matroskaTagStats, fileSize int64) bool {
	if info == nil || len(tagStats) == 0 {
		return false
	}
	statsByTrack := map[uint64]*matroskaTrackStats{}
	for _, stream := range info.Tracks {
		trackUID := streamTrackUID(stream)
		if trackUID == 0 {
			continue
		}
		tag := tagStats[trackUID]
		if !tag.trusted {
			continue
		}
		trackNumber := streamTrackNumber(stream)
		if trackNumber == 0 {
			continue
		}
		stat := &matroskaTrackStats{}
		if tag.hasDataBytes && tag.dataBytes > 0 {
			stat.dataBytes = tag.dataBytes
		}
		if tag.hasFrameCount && tag.frameCount > 0 {
			stat.blockCount = tag.frameCount
		}
		if tag.hasDuration && tag.durationSeconds > 0 {
			stat.hasTime = true
			stat.minTimeNs = 0
			stat.maxTimeNs = int64(tag.durationSeconds * 1e9)
		}
		if stat.dataBytes > 0 || stat.blockCount > 0 || stat.hasTime {
			statsByTrack[trackNumber] = stat
		}
	}
	if len(statsByTrack) > 0 {
		applyMatroskaStats(info, statsByTrack, fileSize)
	}
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		trackUID := streamTrackUID(*stream)
		if trackUID == 0 {
			continue
		}
		tag := tagStats[trackUID]
		if !tag.trusted || !tag.hasBitRate || tag.bitRate <= 0 {
			continue
		}
		bitrate := float64(tag.bitRate)
		switch stream.Kind {
		case StreamText:
			if bitrate < 1000 {
				stream.Fields = setFieldValue(stream.Fields, "Bit rate", fmt.Sprintf("%.0f b/s", math.Floor(bitrate)))
			} else {
				stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrateSmall(bitrate))
			}
			if stream.JSON == nil {
				stream.JSON = map[string]string{}
			}
			stream.JSON["BitRate"] = strconv.FormatInt(tag.bitRate, 10)
		case StreamVideo:
			stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(bitrate))
			if stream.JSON == nil {
				stream.JSON = map[string]string{}
			}
			stream.JSON["BitRate"] = strconv.FormatInt(tag.bitRate, 10)
			width, _ := parsePixels(findField(stream.Fields, "Width"))
			height, _ := parsePixels(findField(stream.Fields, "Height"))
			fps, _ := parseFPS(findField(stream.Fields, "Frame rate"))
			if bits := formatBitsPerPixelFrame(bitrate, width, height, fps); bits != "" {
				stream.Fields = setFieldValue(stream.Fields, "Bits/(Pixel*Frame)", bits)
			}
		case StreamAudio:
			if findField(stream.Fields, "Bit rate") == "" {
				stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(bitrate))
			}
		}
	}
	return matroskaHasCompleteTagStats(info.Tracks, tagStats)
}

func matroskaHasCompleteTagStats(streams []Stream, tagStats map[uint64]matroskaTagStats) bool {
	hasMediaTrack := false
	for _, stream := range streams {
		if stream.Kind != StreamVideo && stream.Kind != StreamAudio && stream.Kind != StreamText {
			continue
		}
		hasMediaTrack = true
		trackUID := streamTrackUID(stream)
		if trackUID == 0 {
			return false
		}
		tag := tagStats[trackUID]
		if !tag.trusted || !tag.hasDataBytes || tag.dataBytes <= 0 {
			return false
		}
		switch stream.Kind {
		case StreamAudio:
			if !(tag.hasDuration || tag.hasBitRate) {
				return false
			}
		case StreamVideo, StreamText:
			if !(tag.hasDuration || tag.hasFrameCount) {
				return false
			}
		}
	}
	return hasMediaTrack
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
		if ac3.acmod > 0 {
			extraFields = append(extraFields, jsonKV{Key: "acmod", Val: strconv.Itoa(ac3.acmod)})
		}
		if ac3.lfeon >= 0 {
			extraFields = append(extraFields, jsonKV{Key: "lfeon", Val: strconv.Itoa(ac3.lfeon)})
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
		if info, frameSize, ok := parseEAC3FrameWithOptions(payload, probe.parseJOC); ok {
			if probe.parseJOC && !ac3HasJOCInfo(info) {
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
					subInfo, subSize, ok := parseEAC3FrameWithOptions(sub, true)
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
			if probe.parseJOC && ac3HasJOCInfo(probe.info) {
				probe.parseJOC = false
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

func streamTrackUID(stream Stream) uint64 {
	if stream.JSON == nil {
		return 0
	}
	value := stream.JSON["UniqueID"]
	if value == "" {
		return 0
	}
	parsed, _ := strconv.ParseUint(value, 10, 64)
	return parsed
}
