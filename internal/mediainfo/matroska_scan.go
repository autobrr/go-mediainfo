package mediainfo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
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
	bareDuration    bool
	source          string
	hasSource       bool
	sourceID        int64
	hasSourceID     bool
	dataBytes       int64
	hasDataBytes    bool
	frameCount      int64
	hasFrameCount   bool
	durationSeconds float64
	hasDuration     bool
	expectsDuration bool
	durationPrec    int
	hasWritingDate  bool
	encodedDate     string
	hasEncodedDate  bool
	bitRate         int64
	hasBitRate      bool
	extras          []jsonKV
}

// matroskaAudioProbe tracks bounded bitstream sampling for one Matroska audio
// track.
type matroskaAudioProbe struct {
	format            string
	info              ac3Info
	dts               dtsInfo
	truehd            trueHDInfo
	mp3               mp3HeaderInfo
	mp3Library        string
	mp3AudioFrameSeen bool
	mp3ModeExtCounts  [4]int
	mp3FramesObserved int
	mp3FrameCount     int64
	mp3PayloadBytes   int64
	mp3XingTag        string
	ok                bool
	collect           bool
	targetFrames      int
	targetPackets     int
	stereoFrames      int
	jocStopPackets    int
	packetCount       int
	parseJOC          bool
	dependentEAC3     bool
	dependentStats    bool
	comprAverage      float64
	hasComprAverage   bool
	dynrngAverage     float64
	hasDynrngAverage  bool
	headerStrip       []byte
}

// dtsInfo contains DTS core and DTS-HD extension properties collected from one
// audio frame.
type dtsInfo struct {
	bitRateBps      int64
	bitDepth        int
	sampleRate      int
	samplesPerFrame int
	channels        int
	hd              bool
	hdXLL           bool // DTS-HD Master Audio (lossless)
	hdXBR           bool // DTS-HD High Resolution Audio (lossy)
	hdDTSX          bool // DTS:X object metadata is present on top of the DTS-HD bed.
	hdIMAX          bool // IMAX Enhanced DTS:X extension metadata is present.
	hdBitDepth      int
	hdChannels      int
	hdSpeakerMask   uint16
	hdSampleRate    int
	hasSpeakerMask  bool
	coreES          bool
	coreXCh         bool
	coreAudioMode   int
	lbr             bool
	lbrLayout       string
	lbrPositions    string
}

type vp9FrameInfo struct {
	profile            int
	bitDepth           int
	colorSpace         string
	chroma             string
	colorRange         string
	matrixCoefficients string
}

type av1SequenceInfo struct {
	hasTiming               bool
	descriptionPresent      bool
	colorRange              string
	colorPrimaries          string
	transferCharacteristics string
	matrixCoefficients      string
}

// matroskaVideoProbe accumulates optional video metadata across bounded
// Matroska frame samples.
type matroskaVideoProbe struct {
	codec         string
	nalLengthSize int
	hdrInfo       hevcHDRInfo
	mpeg2         mpeg2VideoParser
	mpeg4Visual   mpeg4VisualInfo
	mpeg4Data     []byte
	mpeg4Seen     bool
	vp9           vp9FrameInfo
	vp9Seen       bool
	av1           av1SequenceInfo
	av1Seen       bool
	headerStrip   []byte
	writingLib    string
	encoding      string
	avcAnnexB     []byte
	sliceCount    int
	h264SPS       h264SPSInfo
	timeCode      string
	activeFormat  int
	packetCount   int
	targetPackets int
	exhausted     bool
	budget        *matroskaVideoProbeBudget
}

// matroskaVideoProbeMaxBytes bounds the payload read for any one video block.
const matroskaVideoProbeMaxBytes = 8 * 1024 * 1024

// matroskaVideoProbeMaxTotalBytes bounds aggregate video payload reads across
// every probe in one Matroska cluster scan. A shared cap prevents many large,
// interleaved blocks or tracks from multiplying the per-block limit.
const matroskaVideoProbeMaxTotalBytes = 64 * 1024 * 1024

type matroskaVideoProbeBudget struct {
	remaining int64
}

// Cluster scans should avoid reading payload bytes; prefer Seek-based skipping.
const ebmlSkipSeekMin = 0

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
	return newEBMLReaderWithBufSize(rs, 1024*1024)
}

func newEBMLReaderWithBufSize(rs io.ReadSeeker, bufSize int) *ebmlReader {
	if bufSize <= 0 {
		bufSize = 64 * 1024
	}
	return &ebmlReader{
		rs: rs,
		r:  bufio.NewReaderSize(rs, bufSize),
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
	remaining := size - (er.pos - start)
	if remaining < 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	if elemSize > uint64(remaining) {
		return 0, 0, io.ErrUnexpectedEOF
	}
	return id, elemSize, nil
}

var errMatroskaScanLimit = errors.New("matroska scan limit reached")

// scanMatroskaClusters performs a bounded Segment scan for track statistics and
// codec probes. The boolean result reports whether any track statistics were
// collected before completion or the scan limit.
func scanMatroskaClusters(r io.ReaderAt, offset int64, size int64, timecodeScale uint64, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe, applyScan bool, collectBytes bool, parseSpeed float64, trackCount int, needFirstTimes map[uint64]struct{}) (map[uint64]*matroskaTrackStats, bool) {
	if size <= 0 {
		return nil, false
	}
	initializeMatroskaVideoProbeBudget(videoProbes)
	if !applyScan && matroskaProbesComplete(audioProbes, videoProbes) {
		return nil, false
	}
	reader := io.NewSectionReader(r, offset, size)
	// Cluster scans do lots of skipping; avoid read-ahead into payloads.
	er := newEBMLReaderWithBufSize(reader, 8*1024)
	stats := map[uint64]*matroskaTrackStats{}
	var globalFrames int64
	var maxFrames int64
	if parseSpeed < 1 && len(audioProbes) == 1 && trackCount == 2 {
		for _, probe := range audioProbes {
			if probe != nil && probe.format == "AC-3" {
				probe.stereoFrames = matroskaAC3SingleStereoProbeFrames
				if probe.targetPackets == 212 {
					probe.targetFrames = matroskaAC3SingleStereoProbeFrames
				}
			}
		}
	}
	if !applyScan && parseSpeed < 1 {
		if trackCount < 1 {
			trackCount = 1
		}
		if parseSpeed == 0 {
			maxFrames = int64(3 * trackCount)
		} else {
			framesPerTrack := int64(512)
			hasTrueHD := false
			hasHighRateTrueHD := false
			hasDependentEAC3 := false
			ac3Count := 0
			dtsCount := 0
			for _, probe := range audioProbes {
				if probe == nil {
					continue
				}
				hasDependentEAC3 = hasDependentEAC3 || probe.dependentEAC3
				switch probe.format {
				case "TrueHD":
					hasTrueHD = true
					hasHighRateTrueHD = hasHighRateTrueHD || probe.truehd.sampleRate >= 96000
				case "AC-3":
					ac3Count++
				case "DTS":
					dtsCount++
				}
			}
			if len(audioProbes) > 1 {
				for _, probe := range audioProbes {
					if probe != nil && probe.format == "AC-3" && probe.targetPackets == 212 {
						probe.targetPackets = matroskaAC3QuickProbePackets
					}
				}
			}
			if len(audioProbes) == 1 && ac3Count == 1 {
				for _, probe := range audioProbes {
					if probe.targetPackets == 212 {
						if trackCount >= 3 {
							framesPerTrack = 865
							if trackCount == 3 {
								probe.targetPackets = matroskaAC3QuickProbePackets
							}
						} else {
							probe.targetPackets = matroskaAC3QuickProbePackets
						}
					} else if trackCount == 5 {
						framesPerTrack = 900
					}
				}
			}
			if len(audioProbes) == 2 && ac3Count == 1 && dtsCount == 1 && trackCount >= 8 {
				// Preserve the bounded interleaved-read horizon without mutating the
				// AC-3 packet cap or its content-derived histogram count.
				framesPerTrack = 621
			}
			if hasDependentEAC3 && !hasTrueHD {
				framesPerTrack = 1130
				for _, probe := range audioProbes {
					if probe != nil && probe.dependentEAC3 {
						probe.dependentStats = true
						probe.targetPackets = 564
					}
				}
			}
			if hasTrueHD {
				// TrueHD's 40-sample access units dominate interleaved block counts,
				// so MediaInfo reaches its bounded read horizon sooner.
				framesPerTrack = 512
				if hasHighRateTrueHD {
					// High-rate TrueHD carries twice as many samples per access unit and
					// reaches MediaInfo's buffered companion-track horizon slightly later.
					framesPerTrack = 520
				}
			}
			maxFrames = framesPerTrack * int64(trackCount)
		}
	}

	for er.pos < size {
		id, elemSize, err := readMatroskaElementHeader(er, size, 0)
		if err != nil {
			break
		}
		switch id {
		case mkvIDCluster:
			if err := scanMatroskaCluster(er, int64(elemSize), int64(timecodeScale), stats, audioProbes, videoProbes, applyScan, collectBytes, &globalFrames, maxFrames, needFirstTimes); err != nil {
				if errors.Is(err, errMatroskaScanLimit) {
					return stats, len(stats) > 0
				}
				return stats, len(stats) > 0
			}
			if !applyScan && matroskaProbesComplete(audioProbes, videoProbes) && matroskaNeedFirstTimesComplete(stats, needFirstTimes) {
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

// initializeMatroskaVideoProbeBudget assigns every video probe in one scan the
// same aggregate byte budget, preserving an injected budget when present.
func initializeMatroskaVideoProbeBudget(probes map[uint64]*matroskaVideoProbe) {
	var budget *matroskaVideoProbeBudget
	for _, probe := range probes {
		if probe != nil && probe.budget != nil {
			budget = probe.budget
			break
		}
	}
	if budget == nil && len(probes) > 0 {
		budget = &matroskaVideoProbeBudget{remaining: matroskaVideoProbeMaxTotalBytes}
	}
	for _, probe := range probes {
		if probe != nil {
			probe.budget = budget
		}
	}
}

func matroskaNeedFirstTimesComplete(stats map[uint64]*matroskaTrackStats, need map[uint64]struct{}) bool {
	if len(need) == 0 {
		return true
	}
	for id := range need {
		stat := stats[id]
		if stat == nil || !stat.hasTime {
			return false
		}
	}
	return true
}

func matroskaProbesComplete(audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe) bool {
	for _, probe := range audioProbes {
		if probe == nil {
			continue
		}
		if !probe.ok {
			return false
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

func scanMatroskaCluster(er *ebmlReader, size int64, timecodeScale int64, stats map[uint64]*matroskaTrackStats, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe, applyScan bool, collectBytes bool, globalFrames *int64, maxFrames int64, needFirstTimes map[uint64]struct{}) error {
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
			frameLimit := matroskaBlockFrameLimit(globalFrames, maxFrames)
			frames, err := scanMatroskaBlock(er, int64(elemSize), clusterTimecode, timecodeScale, stats, audioProbes, videoProbes, 0, collectBytes, frameLimit)
			if err != nil {
				return err
			}
			if globalFrames != nil && frames > 0 {
				*globalFrames += frames
				if maxFrames > 0 && *globalFrames > maxFrames {
					return errMatroskaScanLimit
				}
			}
			if !applyScan && matroskaProbesComplete(audioProbes, videoProbes) && matroskaNeedFirstTimesComplete(stats, needFirstTimes) {
				return nil
			}
		case mkvIDBlockGroup:
			frameLimit := matroskaBlockFrameLimit(globalFrames, maxFrames)
			frames, err := scanMatroskaBlockGroup(er, int64(elemSize), clusterTimecode, timecodeScale, stats, audioProbes, videoProbes, collectBytes, frameLimit)
			if err != nil {
				return err
			}
			if globalFrames != nil && frames > 0 {
				*globalFrames += frames
				if maxFrames > 0 && *globalFrames > maxFrames {
					return errMatroskaScanLimit
				}
			}
			if !applyScan && matroskaProbesComplete(audioProbes, videoProbes) && matroskaNeedFirstTimesComplete(stats, needFirstTimes) {
				return nil
			}
		default:
			if err := er.skip(int64(elemSize)); err != nil {
				return err
			}
		}
	}
	return nil
}

// matroskaBlockFrameLimit returns the remaining probe budget, including the
// frame that crosses the global limit. A zero result disables frame limiting.
func matroskaBlockFrameLimit(globalFrames *int64, maxFrames int64) int64 {
	if globalFrames == nil || maxFrames <= 0 {
		return 0
	}
	return max(maxFrames-*globalFrames+1, 1)
}

// scanMatroskaBlockGroup consumes one BlockGroup, records its timing and size,
// and returns the number of laced frames processed within frameLimit.
func scanMatroskaBlockGroup(er *ebmlReader, size int64, clusterTimecode int64, timecodeScale int64, stats map[uint64]*matroskaTrackStats, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe, collectBytes bool, frameLimit int64) (int64, error) {
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
			return blockFrames, err
		}
		switch id {
		case mkvIDBlock:
			track, timecode, dataSize, frames, err := readMatroskaBlockHeader(er, int64(elemSize), audioProbes, videoProbes, frameLimit)
			if err != nil {
				return blockFrames, err
			}
			blockTrack = track
			blockTimecode = timecode
			blockSize = dataSize
			blockFrames = frames
			hasBlock = true
		case mkvIDBlockDuration:
			payload, err := er.readN(int64(elemSize))
			if err != nil {
				return blockFrames, err
			}
			if value, ok := readUnsigned(payload); ok {
				blockDuration = value
			}
		default:
			if err := er.skip(int64(elemSize)); err != nil {
				return blockFrames, err
			}
		}
	}

	if hasBlock {
		durationNs := int64(blockDuration) * timecodeScale
		absTime := (clusterTimecode + int64(blockTimecode)) * timecodeScale
		bytes := int64(0)
		if collectBytes {
			bytes = blockSize
		}
		statsForTrack(stats, blockTrack).addBlock(absTime, bytes, durationNs, blockFrames)
	}
	return blockFrames, nil
}

// scanMatroskaBlock consumes one Block or SimpleBlock, updates track statistics,
// and returns the number of laced frames processed within frameLimit.
func scanMatroskaBlock(er *ebmlReader, size int64, clusterTimecode int64, timecodeScale int64, stats map[uint64]*matroskaTrackStats, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe, durationUnits uint64, collectBytes bool, frameLimit int64) (int64, error) {
	track, timecode, dataSize, frames, err := readMatroskaBlockHeader(er, size, audioProbes, videoProbes, frameLimit)
	if err != nil {
		return 0, err
	}
	durationNs := int64(durationUnits) * timecodeScale
	absTime := (clusterTimecode + int64(timecode)) * timecodeScale
	bytes := int64(0)
	if collectBytes {
		bytes = dataSize
	}
	statsForTrack(stats, track).addBlock(absTime, bytes, durationNs, frames)
	return frames, nil
}

// readMatroskaBlockHeader consumes a Block or SimpleBlock, returns its track,
// relative timecode, payload size, and lace count, and feeds bounded payload
// samples to active audio and video probes. Malformed or truncated lacing
// returns an error.
func readMatroskaBlockHeader(er *ebmlReader, size int64, audioProbes map[uint64]*matroskaAudioProbe, videoProbes map[uint64]*matroskaVideoProbe, frameLimit int64) (uint64, int16, int64, int64, error) {
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
	// Block/SimpleBlock minimum header is: track (vint) + timecode(2) + flags(1).
	if int64(trackLen+3) > size {
		if remaining := size - 1; remaining > 0 {
			_ = er.skip(remaining)
		}
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
		if headerLen >= size {
			return 0, 0, 0, 0, io.ErrUnexpectedEOF
		}
		countByte, err := er.readByte()
		if err != nil {
			return 0, 0, 0, 0, err
		}
		headerLen++
		frameCount = int64(countByte) + 1
		// Lacing implies multiple frames. A lace count of 0 is malformed.
		if frameCount < 2 {
			if remaining := size - headerLen; remaining > 0 {
				_ = er.skip(remaining)
			}
			return 0, 0, 0, 0, io.ErrUnexpectedEOF
		}
		switch lacing {
		case 1: // Xiph
			if needProbePayload && frameCount > 1 {
				laceSizes = make([]int64, frameCount-1)
			}
			for i := int64(0); i < frameCount-1; i++ {
				laceSize := int64(0)
				for {
					if headerLen >= size {
						return 0, 0, 0, 0, io.ErrUnexpectedEOF
					}
					b, err := er.readByte()
					if err != nil {
						return 0, 0, 0, 0, err
					}
					headerLen++
					laceSize += int64(b)
					if b != 0xFF {
						break
					}
				}
				if needProbePayload {
					laceSizes[i] = laceSize
				}
				laceSum += laceSize
			}
		case 3: // EBML
			readUnsigned := func(first byte, remaining int64) (uint64, int, error) {
				length := vintLength(first)
				if length == 0 {
					return 0, 0, io.ErrUnexpectedEOF
				}
				// first byte already read by caller; remaining includes it.
				if int64(length) > remaining {
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
			readSigned := func(first byte, remaining int64) (int64, int, error) {
				value, length, err := readUnsigned(first, remaining)
				if err != nil {
					return 0, 0, err
				}
				if value > uint64(int64(^uint64(0)>>1)) {
					return 0, 0, io.ErrUnexpectedEOF
				}
				bias := int64(1)<<(uint(length*7-1)) - 1
				return int64(value) - bias, length, nil
			}
			if needProbePayload && frameCount > 1 {
				laceSizes = make([]int64, frameCount-1)
			}
			if headerLen >= size {
				return 0, 0, 0, 0, io.ErrUnexpectedEOF
			}
			firstSizeByte, err := er.readByte()
			if err != nil {
				return 0, 0, 0, 0, err
			}
			sizeVal, length, err := readUnsigned(firstSizeByte, size-headerLen)
			if err != nil {
				return 0, 0, 0, 0, err
			}
			headerLen += int64(length)
			if sizeVal > uint64(size) {
				return 0, 0, 0, 0, io.ErrUnexpectedEOF
			}
			if needProbePayload {
				laceSizes[0] = int64(sizeVal)
			}
			laceSum = int64(sizeVal)
			prev := int64(sizeVal)
			for i := int64(1); i < frameCount-1; i++ {
				if headerLen >= size {
					return 0, 0, 0, 0, io.ErrUnexpectedEOF
				}
				firstDiff, err := er.readByte()
				if err != nil {
					return 0, 0, 0, 0, err
				}
				diff, length, err := readSigned(firstDiff, size-headerLen)
				if err != nil {
					return 0, 0, 0, 0, err
				}
				headerLen += int64(length)
				next := prev + diff
				if (diff > 0 && next < prev) || (diff < 0 && next > prev) {
					return 0, 0, 0, 0, io.ErrUnexpectedEOF
				}
				if next < 0 || next > size {
					return 0, 0, 0, 0, io.ErrUnexpectedEOF
				}
				if needProbePayload {
					laceSizes[i] = next
				}
				laceSum += next
				prev = next
			}
		}
	}
	dataSize := size - headerLen
	if dataSize < 0 {
		return 0, 0, 0, 0, io.ErrUnexpectedEOF
	}
	if needProbePayload && frameCount > 1 && (lacing == 1 || lacing == 3) {
		// Lace sizes must be within the block payload; otherwise probing can request absurd reads.
		if laceSum < 0 || laceSum > dataSize {
			return 0, 0, 0, 0, io.ErrUnexpectedEOF
		}
		if int64(len(laceSizes)) != frameCount-1 {
			return 0, 0, 0, 0, io.ErrUnexpectedEOF
		}
		for _, s := range laceSizes {
			if s < 0 || s > dataSize {
				return 0, 0, 0, 0, io.ErrUnexpectedEOF
			}
		}
	}
	if dataSize > 0 || (needAudio && audioProbe != nil && len(audioProbe.headerStrip) > 0) || (needVideo && videoProbe != nil && len(videoProbe.headerStrip) > 0) {
		if needProbePayload {
			processedFrames := frameCount
			if frameLimit > 0 {
				processedFrames = min(processedFrames, frameLimit)
			}
			// MediaInfoLib increments PacketCount per Block/SimpleBlock, then may stop searching
			// payload mid-block after it reaches the cap. This matters for laced blocks: in the
			// final packet, only the first lace contributes to stream stats.
			stopAfterThisPacket := false
			stopAfterTarget := false
			stopAfterJOC := false
			maxLacesToProbe := int64(0)
			if needAudio && audioProbe != nil && (audioProbe.format == "AC-3" || audioProbe.format == "E-AC-3") && audioProbe.targetPackets > 0 {
				nextPacket := audioProbe.packetCount + 1
				stopAfterTarget = nextPacket >= audioProbe.targetPackets
				stopAfterJOC = audioProbe.parseJOC && audioProbe.jocStopPackets > 0 && nextPacket >= audioProbe.jocStopPackets && ac3HasJOCInfo(audioProbe.info)
				stopAfterThisPacket = stopAfterTarget || stopAfterJOC
				// Official mediainfo may stop mid-block after hitting the cap. For typical caps,
				// only the first lace contributes. For our JOC bound, allow 2 laces to avoid
				// under-counting on common Atmos layouts.
				if stopAfterThisPacket {
					maxLacesToProbe = 1
					if stopAfterJOC && !stopAfterTarget {
						maxLacesToProbe = 2
					}
				}
			}
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
						// Fixed-size lacing: frames are usually equal-sized, but be robust to
						// non-divisible blocks by assigning remainder to the last lace.
						base := dataSize / frameCount
						if i < frameCount-1 {
							size = base
						} else {
							size = dataSize - base*(frameCount-1)
						}
					}
				}
				skipFrameProbe := i >= processedFrames
				peek := int64(256)
				switch {
				case skipFrameProbe:
					peek = 0
				case needVideo:
					peek = int64(matroskaVideoProbeMaxBytes)
					if videoProbe != nil && videoProbe.budget != nil {
						remaining := max(videoProbe.budget.remaining, 0)
						prefixBytes := int64(len(videoProbe.headerStrip))
						if prefixBytes > remaining {
							skipFrameProbe = true
							peek = 0
						} else {
							peek = min(peek, remaining-prefixBytes)
						}
					}
				case needAudio && audioProbe != nil && audioProbe.format == "DTS":
					// DTS-HD extension substream (ExSS) follows the core frame, which can be several KB.
					// Cap at 32 KB to avoid large allocations on oversized blocks.
					peek = 32768
				case needAudio && audioProbe != nil && audioProbe.format == "E-AC-3":
					// In the final packet, skip probing additional laces to match official behavior.
					switch {
					case stopAfterThisPacket && maxLacesToProbe > 0 && i >= maxLacesToProbe:
						peek = 0
					case audioProbe.parseJOC:
						peek = min(size, 32768)
					case frameCount == 1:
						// Non-laced packets may contain multiple E-AC-3 frames; read a bit more so we
						// can stay in sync and match official compr stats.
						peek = 8192
					}
				}
				peek = min(size, peek)
				payload, err := er.readN(peek)
				if err != nil {
					return 0, 0, 0, 0, err
				}
				skipAudioProbe := skipFrameProbe || (stopAfterThisPacket && maxLacesToProbe > 0 && i >= maxLacesToProbe)
				if audioProbe != nil && audioProbe.targetFrames > 0 && audioProbe.info.framesMerged >= audioProbe.targetFrames {
					skipAudioProbe = true
				}
				if needAudio && !skipAudioProbe {
					audioPayload := applyMatroskaAudioHeaderStrip(payload, audioProbe)
					effectiveSize := size
					if audioProbe != nil && len(audioProbe.headerStrip) > 0 {
						effectiveSize += int64(len(audioProbe.headerStrip))
					}
					// For most codecs, Matroska Block/SimpleBlock boundaries are frame-aligned when laced
					// (each lace is a frame). For non-laced E-AC-3, packets may contain multiple syncframes,
					// so allow probe logic to treat the packet as not strictly aligned.
					packetAligned := frameCount > 1
					probeMatroskaAudio(audioProbes, trackVal, audioPayload, 1, effectiveSize, packetAligned)
				}
				var videoPayload []byte
				if needVideo && !skipFrameProbe {
					videoPayload = applyMatroskaVideoHeaderStrip(payload, videoProbe)
				}
				sampledVideo := needVideo && !skipFrameProbe && len(videoPayload) > 0
				if sampledVideo {
					if videoProbe.budget != nil {
						videoProbe.budget.remaining -= int64(len(videoPayload))
					}
					probeMatroskaVideo(videoProbes, trackVal, videoPayload)
				}
				if sampledVideo && videoProbe.targetPackets > 0 {
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
			if needAudio && audioProbe != nil && (audioProbe.format == "AC-3" || audioProbe.format == "E-AC-3") && audioProbe.targetPackets > 0 {
				if audioProbe.targetFrames > 0 && audioProbe.info.framesMerged >= audioProbe.targetFrames {
					audioProbe.collect = false
				}
				// Keep probing bounded; count per Matroska packet (Block/SimpleBlock).
				audioProbe.packetCount++
				if audioProbe.dependentStats && audioProbe.packetCount == 299 {
					if average, _, _, _, ok := audioProbe.info.comprStats(); ok {
						audioProbe.comprAverage = average
						audioProbe.hasComprAverage = true
					}
				}
				if audioProbe.dependentStats && audioProbe.packetCount == 524 {
					if average, _, _, _, ok := audioProbe.info.dynrngStats(); ok {
						audioProbe.dynrngAverage = average
						audioProbe.hasDynrngAverage = true
					}
				}
				// Bound the expensive JOC scan (full-block reads) separately.
				if audioProbe.parseJOC && audioProbe.jocStopPackets > 0 && audioProbe.packetCount >= audioProbe.jocStopPackets {
					audioProbe.parseJOC = false
				}
				// Keep collecting stats until the full packet cap; official mediainfo still reports
				// compr/dialnorm stats beyond the point where JOC metadata becomes available.
				if audioProbe.packetCount >= audioProbe.targetPackets {
					audioProbe.collect = false
				}
			}
			// MediaInfo Frame_Count is effectively per-lace (per frame). For a non-laced block this is 1.
			return trackVal, timecode, dataSize, processedFrames, nil
		}
		if err := er.skip(dataSize); err != nil {
			return 0, 0, 0, 0, err
		}
	}
	// MediaInfo Frame_Count is effectively per-lace (per frame). For a non-laced block this is 1.
	return trackVal, timecode, dataSize, frameCount, nil
}

// videoProbeNeedsSample reports whether another Matroska video block can still
// contribute bitstream-derived metadata. HEVC keeps looking for optional x265
// SEI after required HDR metadata is complete, but only until the packet cap is
// exhausted.
func videoProbeNeedsSample(probe *matroskaVideoProbe) bool {
	if probe == nil {
		return false
	}
	if probe.exhausted {
		return false
	}
	if probe.budget != nil {
		if probe.budget.remaining <= 0 || int64(len(probe.headerStrip)) > probe.budget.remaining {
			return false
		}
	}
	switch probe.codec {
	case "HEVC":
		return !probe.hdrInfo.scanDone() || !probe.hdrInfo.x265Seen
	case "AVC":
		return true
	case "MPEG Video":
		return true
	case "MPEG-4 Visual":
		// VOL metadata may arrive in the first packet, while the first B-VOP can
		// occur later. Keep the bounded packet scan active so BVOP reflects parsed
		// frame types rather than the first packet alone.
		return true
	case "VP9":
		return !probe.vp9Seen
	case "AV1":
		return !probe.av1Seen
	default:
		return false
	}
}

// applyMatroskaAudioHeaderStrip reconstructs the original codec frame from
// ContentCompSettings, including frames whose stored payload is empty.
func applyMatroskaAudioHeaderStrip(payload []byte, probe *matroskaAudioProbe) []byte {
	if probe == nil {
		return payload
	}
	prefix := probe.headerStrip
	if len(prefix) == 0 {
		return payload
	}
	combined := make([]byte, len(prefix)+len(payload))
	copy(combined, prefix)
	copy(combined[len(prefix):], payload)
	return combined
}

// applyMatroskaVideoHeaderStrip reconstructs the original codec frame from
// ContentCompSettings, including frames whose stored payload is empty.
func applyMatroskaVideoHeaderStrip(payload []byte, probe *matroskaVideoProbe) []byte {
	if probe == nil {
		return payload
	}
	prefix := probe.headerStrip
	if len(prefix) == 0 {
		return payload
	}
	combined := make([]byte, len(prefix)+len(payload))
	copy(combined, prefix)
	copy(combined[len(prefix):], payload)
	return combined
}

// statsForTrack returns the mutable accumulator for track, creating it when
// the bounded cluster scan observes that track for the first time.
func statsForTrack(stats map[uint64]*matroskaTrackStats, track uint64) *matroskaTrackStats {
	entry := stats[track]
	if entry != nil {
		return entry
	}
	entry = &matroskaTrackStats{}
	stats[track] = entry
	return entry
}

// matroskaStreamScalar returns a direct canonical Matroska scalar.
func matroskaStreamScalar(stream Stream, key fieldName) string {
	value, _ := canonicalSeedValue(stream, key)
	return value
}

// matroskaProjectedDuration returns the exact seconds projection of a
// canonical Duration for decisions that depend on its precision.
func matroskaProjectedDuration(stream Stream) string {
	if value, found := projectedCanonicalSeedValue(stream, "Duration"); found {
		return value
	}
	return ""
}

// matroskaStreamDisplay returns one canonical Matroska text projection.
func matroskaStreamDisplay(stream Stream, label string) string {
	value, _ := canonicalSeedTextValue(stream, label)
	return value
}

// matroskaStreamDurationSeconds converts canonical milliseconds to seconds.
func matroskaStreamDurationSeconds(stream Stream) (float64, bool) {
	if milliseconds, found := canonicalSeedValue(stream, "Duration"); found {
		value, err := strconv.ParseFloat(milliseconds, 64)
		return value / 1000, err == nil && value > 0
	}
	return 0, false
}

// matroskaCanonicalFrameRate returns the direct rational frame rate when
// available, then the direct decimal rate, without parsing display text.
func matroskaCanonicalFrameRate(stream Stream) (float64, bool) {
	numerator, numeratorErr := strconv.ParseFloat(matroskaStreamScalar(stream, "FrameRate_Num"), 64)
	denominator, denominatorErr := strconv.ParseFloat(matroskaStreamScalar(stream, "FrameRate_Den"), 64)
	if numeratorErr == nil && denominatorErr == nil && numerator > 0 && denominator > 0 {
		return numerator / denominator, true
	}
	frameRate, err := strconv.ParseFloat(matroskaStreamScalar(stream, "FrameRate"), 64)
	return frameRate, err == nil && frameRate > 0
}

// applyMatroskaStats merges observed block sizes, counts, and time bounds into
// parsed tracks using MediaInfo-compatible derivation and rounding rules.
func applyMatroskaStats(info *MatroskaInfo, stats map[uint64]*matroskaTrackStats, fileSize int64) {
	if len(stats) == 0 {
		return
	}
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		trackID := streamTrackNumber(*stream)
		if trackID == 0 {
			continue
		}
		stat := stats[trackID]
		if stat == nil {
			continue
		}
		if stat.dataBytes > 0 {
			display := formatStreamSize(stat.dataBytes, fileSize)
			raw := strconv.FormatInt(stat.dataBytes, 10)
			replaceCanonicalSeedFill(stream, "StreamSize", raw, "Stream size", display)
		}
		if stat.blockCount > 0 && stream.Kind == StreamAudio {
			// Official mediainfo reports audio FrameCount for AC-3 / E-AC-3 tracks (from Statistics Tags).
			format, _ := canonicalSeedValue(*stream, "Format")
			if format == "AC-3" || format == "E-AC-3" {
				count := strconv.FormatInt(stat.blockCount, 10)
				replaceCanonicalSeedFill(stream, "FrameCount", count, "", "")
			}
		}
		durationSeconds := matroskaStatsDuration(stat)
		if stream.Kind == StreamVideo && stat.blockCount > 0 {
			// MediaInfo sometimes derives Matroska track Duration from FrameCount and FPS (inclusive
			// of the last frame) when the observed time bounds align to (FrameCount-1)/FPS.
			fps, _ := matroskaCanonicalFrameRate(*stream)
			if fps > 0 {
				if durationSeconds <= 0 {
					durationSeconds = float64(stat.blockCount) / fps
				} else if stat.blockCount > 1 {
					exclusive := float64(stat.blockCount-1) / fps
					// Tight tolerance: time bounds are in milliseconds in most Matroska stats sources.
					if math.Abs(durationSeconds-exclusive) < 0.002 {
						durationSeconds = float64(stat.blockCount) / fps
					}
				}
			}
		}
		if durationSeconds > 0 {
			if stream.Kind == StreamVideo {
				// MediaInfo truncates to milliseconds in Matroska stats-derived durations.
				durationSeconds = math.Floor(durationSeconds*1000+1e-6) / 1000
			}
			switch stream.Kind {
			case StreamText, StreamVideo:
				seconds := fmt.Sprintf("%.9f", durationSeconds)
				replaceCanonicalSeedProjection(stream, "Duration", strconv.FormatFloat(durationSeconds*1000, 'f', -1, 64), seconds, "Duration", formatDuration(durationSeconds))
				setCanonicalSeedStructuredDecimals(stream, "Duration", 9)
			case StreamAudio:
				// Preserve container/tag-reported audio duration (MediaInfo does not overwrite it with
				// cluster-derived duration at default ParseSpeed).
				if matroskaStreamDisplay(*stream, "Duration") == "" {
					// If Matroska Info duration is absent, the track Duration can be stats-derived only.
					// formatDuration rounds to milliseconds and drops them once >= 60s, so populate JSON
					// directly to keep fractional seconds comparable to official mediainfo.
					if matroskaProjectedDuration(*stream) == "" {
						seconds := formatJSONSeconds(durationSeconds)
						if milliseconds, ok := decimalSecondsToMilliseconds(seconds); ok {
							replaceCanonicalSeedProjection(stream, "Duration", milliseconds, seconds, "Duration", formatDuration(durationSeconds))
							setCanonicalSeedStructuredDecimals(stream, "Duration", uint8(decimalFractionDigits(seconds)))
						}
					}
				}
			case StreamGeneral, StreamImage, StreamMenu:
				// Cluster duration does not project onto these stream kinds.
			}
		}
		// Match MediaInfo: when both StreamSize and Duration are known, BitRate is derived from them.
		// This avoids using nominal/tagged bitrates for Matroska AAC/Opus where MediaInfo prefers average.
		containerCBR := stream.Kind == StreamAudio &&
			streamBitRateMode(*stream) == "Constant" &&
			matroskaStreamScalar(*stream, "BitRate") != ""
		if stream.Kind == StreamAudio && stat.dataBytes > 0 && !containerCBR {
			dur := 0.0
			if duration := matroskaProjectedDuration(*stream); duration != "" {
				if v, err := strconv.ParseFloat(duration, 64); err == nil && v > 0 {
					dur = v
				}
			}
			if dur <= 0 {
				if v, ok := matroskaStreamDurationSeconds(*stream); ok && v > 0 {
					dur = v
				}
			}
			if dur > 0 {
				// Match MediaInfo: truncate (not round) to integer b/s.
				bps := int64(math.Floor((float64(stat.dataBytes)*8)/dur + 1e-9))
				// Official MediaInfo quantizes Matroska AAC bitrates to 8 kb/s steps when derived.
				format := matroskaStreamDisplay(*stream, "Format")
				codecID := matroskaStreamDisplay(*stream, "Codec ID")
				isAAC := strings.Contains(format, "AAC") || strings.HasPrefix(codecID, "A_AAC")
				if isAAC && bps >= 8000 {
					bps = int64(math.Round(float64(bps)/8000) * 8000)
				}
				if bps > 0 {
					raw := strconv.FormatInt(bps, 10)
					replaceCanonicalSeedFill(stream, "BitRate", raw, "", "")
				}
			}
		}
		if stat.blockCount > 0 && (stream.Kind == StreamVideo || stream.Kind == StreamText) {
			count := strconv.FormatInt(stat.blockCount, 10)
			replaceCanonicalSeedFill(stream, "FrameCount", count, "", "")
			if stream.Kind == StreamText {
				replaceCanonicalSeedFill(stream, "ElementCount", count, "", "")
			}
		}
		if stream.Kind == StreamText {
			if stat.blockCount > 0 {
				count := strconv.FormatInt(stat.blockCount, 10)
				replaceCanonicalSeedText(stream, "Count of elements", count)
			}
			if durationSeconds > 0 && stat.blockCount > 0 {
				frameRate := float64(stat.blockCount) / durationSeconds
				display := formatFrameRate(frameRate)
				raw := formatJSONFloat(frameRate)
				replaceCanonicalSeedFill(stream, "FrameRate", raw, "Frame rate", display)
			}
			if durationSeconds > 0 && stat.dataBytes > 0 {
				bitrate := (float64(stat.dataBytes) * 8) / durationSeconds
				display := ""
				if bitrate < 1000 {
					display = fmt.Sprintf("%.0f b/s", math.Floor(bitrate))
				} else {
					display = formatBitrateSmall(bitrate)
				}
				raw := strconv.FormatInt(int64(bitrate), 10)
				replaceCanonicalSeedFill(stream, "BitRate", raw, "Bit rate", display)
			}
		}
		if stream.Kind == StreamVideo {
			bitrateDuration := durationSeconds
			if duration := matroskaProjectedDuration(*stream); duration != "" {
				if value, err := strconv.ParseFloat(duration, 64); err == nil && value > 0 {
					bitrateDuration = value
				}
			}
			hasBitRate := matroskaStreamScalar(*stream, "BitRate") != ""
			if !hasBitRate {
				// If x264 parsing provided a nominal bitrate, prefer it over derived StreamSize/Duration.
				if nominal := matroskaStreamDisplay(*stream, "Nominal bit rate"); nominal != "" {
					if bps, ok := parseInt(matroskaStreamScalar(*stream, "BitRate_Nominal")); ok && bps > 0 {
						replaceCanonicalSeedFill(stream, "BitRate", strconv.FormatInt(bps, 10), "Bit rate", nominal)
						hasBitRate = true
					}
				}
			}
			if bitrateDuration > 0 && stat.dataBytes > 0 && !hasBitRate {
				bitrate := (float64(stat.dataBytes) * 8) / bitrateDuration
				display := formatBitrate(bitrate)
				raw := strconv.FormatInt(int64(bitrate), 10)
				replaceCanonicalSeedFill(stream, "BitRate", raw, "Bit rate", display)
				width, _ := parsePixels(matroskaStreamScalar(*stream, "Width"))
				height, _ := parsePixels(matroskaStreamScalar(*stream, "Height"))
				fps, _ := matroskaCanonicalFrameRate(*stream)
				if bits := formatBitsPerPixelFrame(bitrate, width, height, fps); bits != "" {
					replaceCanonicalSeedText(stream, "Bits/(Pixel*Frame)", bits)
				}
			}
		}
		if stream.Kind == StreamAudio {
			if durationSeconds > 0 && stat.dataBytes > 0 && matroskaStreamScalar(*stream, "BitRate") == "" {
				bitrate := (float64(stat.dataBytes) * 8) / durationSeconds
				// Official MediaInfo reports AAC bitrates quantized to 8 kb/s steps when derived
				// from statistics (StreamSize/Duration).
				format := matroskaStreamDisplay(*stream, "Format")
				codecID := matroskaStreamDisplay(*stream, "Codec ID")
				isAAC := strings.Contains(format, "AAC") || strings.HasPrefix(codecID, "A_AAC")
				if isAAC && bitrate >= 8000 {
					bitrate = math.Round(bitrate/8000) * 8000
				}
				display := formatBitrate(bitrate)
				// Official mediainfo truncates derived audio bitrate.
				raw := strconv.FormatInt(int64(bitrate), 10)
				replaceCanonicalSeedFill(stream, "BitRate", raw, "Bit rate", display)
			}
		}
	}
}

// applyMatroskaTagStats applies trusted Statistics Tags and reports whether
// they provide complete metadata coverage for every media track.
func applyMatroskaTagStats(info *MatroskaInfo, tagStats map[uint64]matroskaTagStats, fileSize int64) bool {
	if info == nil || len(tagStats) == 0 {
		return false
	}
	statsByTrack := map[uint64]*matroskaTrackStats{}
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		trackUID := streamTrackUID(*stream)
		if trackUID == 0 {
			continue
		}
		tag := tagStats[trackUID]
		if len(tag.extras) > 0 {
			prependCanonicalSeedObjectMembers(stream, "extra", structuredObjectFromKVs(tag.extras).Object)
		}
		if tag.hasEncodedDate && tag.encodedDate != "" {
			replaceCanonicalSeedFill(stream, "Encoded_Date", tag.encodedDate, "Encoded date", tag.encodedDate)
		}
		if tag.hasSource && tag.source != "" {
			replaceCanonicalSeedText(stream, "Source", tag.source)
			insertCanonicalSeedObjectMembersBefore(stream, "extra", "MD5_Unencoded", []structuredMember{{Key: "Source", Value: structuredNode{Kind: structuredString, Text: tag.source}}})
		}
		if tag.hasSourceID && tag.sourceID > 0 {
			medium := "Blu-ray"
			sourceID := strconv.FormatInt(tag.sourceID, 10)
			if tag.sourceID > 0xffff {
				medium = "DVD-Video"
				streamID := tag.sourceID & 0xff
				substreamID := (tag.sourceID >> 8) & 0xff
				sourceID = strconv.FormatInt(streamID, 10)
				if streamID == 0xbd && substreamID > 0 {
					sourceID += "-" + strconv.FormatInt(substreamID, 10)
				}
			} else if strings.Contains(strings.ToLower(tag.source), "dvd") {
				medium = "DVD-Video"
			}
			insertCanonicalSeedTextBefore(stream, "Original source medium", medium, "Source")
			insertCanonicalSeedTextBefore(stream, "Original source medium ID", sourceID, "Source")
			replaceCanonicalSeedFill(stream, "OriginalSourceMedium_ID", sourceID, "", "")
			mediumMember := []structuredMember{{Key: "OriginalSourceMedium", Value: structuredNode{Kind: structuredString, Text: medium}}}
			insertCanonicalSeedObjectMembersBefore(stream, "extra", "Source", mediumMember)
		}
		if !matroskaTagStatsAreAuthoritative(info, tag) {
			continue
		}
		stream.mkvTagFrameCount = tag.hasFrameCount && tag.frameCount > 0
		trackNumber := streamTrackNumber(*stream)
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
			stat.maxTimeNs = int64(math.Round(tag.durationSeconds * 1e9))
		}
		if stat.dataBytes > 0 || stat.blockCount > 0 || stat.hasTime {
			statsByTrack[trackNumber] = stat
		}
	}
	if len(statsByTrack) > 0 {
		applyMatroskaStats(info, statsByTrack, fileSize)
	}
	// Preserve Statistics Tags duration precision in JSON serialization (MediaInfo varies between 3 and 9 decimals).
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		if stream.Kind != StreamVideo && stream.Kind != StreamAudio {
			continue
		}
		trackUID := streamTrackUID(*stream)
		if trackUID == 0 {
			continue
		}
		tag := tagStats[trackUID]
		if !matroskaTagStatsAreAuthoritative(info, tag) {
			continue
		}
		format := matroskaStreamDisplay(*stream, "Format")
		if (format == "TrueHD" || strings.HasPrefix(format, "AAC") || strings.HasPrefix(format, "DTS") || format == "Opus" || format == "MPEG Audio" || format == "PCM") && tag.hasFrameCount && tag.frameCount > 0 {
			// Preserve the muxer's exact access-unit count; rounded duration can
			// otherwise produce a one-frame derivation error.
			raw := strconv.FormatInt(tag.frameCount, 10)
			replaceCanonicalSeedFill(stream, "FrameCount", raw, "", "")
		}
		if !tag.hasDuration || tag.durationSeconds <= 0 {
			continue
		}
		seconds := ""
		if tag.hasWritingDate {
			// When Statistics Tags include a writing date (older mkvmerge style), official mediainfo
			// emits Duration at millisecond precision.
			seconds = formatJSONSeconds(tag.durationSeconds)
		} else {
			prec := min(max(tag.durationPrec, 3), 9)
			seconds = fmt.Sprintf("%.*f", prec, tag.durationSeconds)
		}
		if milliseconds, ok := decimalSecondsToMilliseconds(seconds); ok {
			replaceCanonicalSeedProjection(stream, "Duration", milliseconds, seconds, "", "")
			setCanonicalSeedStructuredDecimals(stream, "Duration", uint8(decimalFractionDigits(seconds)))
		}
		if format == "FLAC" {
			sampleRate, _ := strconv.ParseInt(matroskaStreamScalar(*stream, "SamplingRate"), 10, 64)
			if sampleRate > 0 {
				samplingCount := int64(math.RoundToEven(tag.durationSeconds * float64(sampleRate)))
				if samplingCount > 0 {
					samplingRaw := strconv.FormatInt(samplingCount, 10)
					replaceCanonicalSeedFill(stream, "SamplingCount", samplingRaw, "", "")
					if samplesPerFrame, err := strconv.ParseInt(matroskaStreamScalar(*stream, "SamplesPerFrame"), 10, 64); err == nil && samplesPerFrame > 0 {
						frameCount := (samplingCount + samplesPerFrame - 1) / samplesPerFrame
						frameRaw := strconv.FormatInt(frameCount, 10)
						replaceCanonicalSeedFill(stream, "FrameCount", frameRaw, "", "")
					}
				}
			}
		}
		if strings.HasPrefix(format, "AAC") || format == "Opus" {
			sampleRate, _ := strconv.ParseInt(matroskaStreamScalar(*stream, "SamplingRate"), 10, 64)
			if sampleRate > 0 {
				samplingCount := int64(math.RoundToEven(tag.durationSeconds * float64(sampleRate)))
				if samplingCount > 0 {
					samplingRaw := strconv.FormatInt(samplingCount, 10)
					replaceCanonicalSeedFill(stream, "SamplingCount", samplingRaw, "", "")
					if format == "Opus" && tag.hasFrameCount && tag.frameCount > 0 {
						averageSamples := float64(samplingCount) / float64(tag.frameCount)
						samplesPerFrame := int64(math.Round(averageSamples))
						if samplesPerFrame > 0 && math.Abs(averageSamples-float64(samplesPerFrame)) <= 0.01 {
							samplesRaw := strconv.FormatInt(samplesPerFrame, 10)
							frameRateRaw := fmt.Sprintf("%.3f", float64(sampleRate)/float64(samplesPerFrame))
							replaceCanonicalSeedFill(stream, "SamplesPerFrame", samplesRaw, "", "")
							replaceCanonicalSeedFill(stream, "FrameRate", frameRateRaw, "", "")
						} else {
							// A variable-duration packet sequence has no stable integral average
							// for timing derivation, but Opus still reports its nominal 20 ms frame.
							frameRateRaw := fmt.Sprintf("%.3f", float64(tag.frameCount)/tag.durationSeconds)
							replaceCanonicalSeedFill(stream, "SamplesPerFrame", "960", "", "")
							replaceCanonicalSeedFill(stream, "FrameRate", frameRateRaw, "", "")
						}
					}
				}
			}
		}
		if format == "MPEG Audio" || format == "PCM" {
			sampleRate, _ := strconv.ParseInt(matroskaStreamScalar(*stream, "SamplingRate"), 10, 64)
			if sampleRate > 0 {
				samplingCount := int64(math.RoundToEven(tag.durationSeconds * float64(sampleRate)))
				if samplingCount > 0 {
					replaceCanonicalSeedFill(stream, "SamplingCount", strconv.FormatInt(samplingCount, 10), "", "")
				}
			}
			if format == "MPEG Audio" && matroskaStreamScalar(*stream, "BitRate") != "" {
				replaceCanonicalSeedProjection(stream, "BitRate_Mode", "Constant", "CBR", "Bit rate mode", "Constant")
			}
		}
	}
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		trackUID := streamTrackUID(*stream)
		if trackUID == 0 {
			continue
		}
		tag := tagStats[trackUID]
		if !matroskaTagStatsAreAuthoritative(info, tag) || !tag.hasBitRate || tag.bitRate <= 0 {
			continue
		}
		bitrate := float64(tag.bitRate)
		switch stream.Kind {
		case StreamText:
			display := ""
			if bitrate < 1000 {
				display = fmt.Sprintf("%.0f b/s", math.Floor(bitrate))
			} else {
				display = formatBitrateSmall(bitrate)
			}
			raw := strconv.FormatInt(tag.bitRate, 10)
			replaceCanonicalSeedFill(stream, "BitRate", raw, "Bit rate", display)
		case StreamVideo:
			// A distinct header/container rate is nominal when Statistics Tags
			// provide the measured payload rate.
			if existing, ok := parseInt(matroskaStreamScalar(*stream, "BitRate")); ok && existing > 0 {
				delta := math.Abs(float64(existing-tag.bitRate)) / float64(existing)
				if delta >= 0.04 {
					replaceCanonicalSeedFill(stream, "BitRate_Nominal", strconv.FormatInt(existing, 10), "Nominal bit rate", formatBitrate(float64(existing)))
					replaceCanonicalSeedFill(stream, "BitRate", strconv.FormatInt(tag.bitRate, 10), "Bit rate", formatBitrate(bitrate))
					continue
				}
				continue
			}
			if matroskaStreamScalar(*stream, "BitRate") != "" || matroskaStreamScalar(*stream, "BitRate_Nominal") != "" {
				continue
			}
			replaceCanonicalSeedFill(stream, "BitRate", strconv.FormatInt(tag.bitRate, 10), "Bit rate", formatBitrate(bitrate))
			width, _ := parsePixels(matroskaStreamScalar(*stream, "Width"))
			height, _ := parsePixels(matroskaStreamScalar(*stream, "Height"))
			fps, _ := matroskaCanonicalFrameRate(*stream)
			if bits := formatBitsPerPixelFrame(bitrate, width, height, fps); bits != "" {
				replaceCanonicalSeedText(stream, "Bits/(Pixel*Frame)", bits)
			}
		case StreamAudio:
			format := matroskaStreamDisplay(*stream, "Format")
			if strings.HasPrefix(format, "AAC") {
				raw := strconv.FormatInt(tag.bitRate, 10)
				display := formatBitrate(float64(tag.bitRate))
				replaceCanonicalSeedFill(stream, "BitRate", raw, "Bit rate", display)
				continue
			}
			isCBR := streamBitRateMode(*stream) == "Constant"
			if isCBR {
				cbrRate := tag.bitRate
				if existing, ok := parseInt(matroskaStreamScalar(*stream, "BitRate")); ok && existing > 0 {
					if !strings.HasPrefix(matroskaStreamDisplay(*stream, "Format"), "AAC") {
						cbrRate = existing
					}
				}
				display := formatBitrate(float64(cbrRate))
				replaceCanonicalSeedFill(stream, "BitRate", strconv.FormatInt(cbrRate, 10), "Bit rate", display)
				continue
			}
			// Prefer derived average bitrate (StreamSize/Duration) over Statistics Tags for audio.
			// MediaInfo reports exact BitRate in JSON (e.g. 241184) even when Statistics Tags carry
			// quantized values (e.g. 240000).
			if bytes, ok := parseInt(matroskaStreamScalar(*stream, "StreamSize")); ok && bytes > 0 {
				if dur, err := strconv.ParseFloat(matroskaProjectedDuration(*stream), 64); err == nil && dur > 0 {
					bps := int64(math.Floor((float64(bytes)*8)/dur + 1e-9))
					if bps > 0 {
						// Official MediaInfo quantizes Matroska AAC bitrates to 8 kb/s steps when derived.
						format := matroskaStreamDisplay(*stream, "Format")
						codecID := matroskaStreamDisplay(*stream, "Codec ID")
						isAAC := strings.Contains(format, "AAC") || strings.HasPrefix(codecID, "A_AAC")
						if isAAC && bps >= 8000 {
							bps = int64(math.Round(float64(bps)/8000) * 8000)
						}
						raw := strconv.FormatInt(bps, 10)
						display := formatBitrate(float64(bps))
						replaceCanonicalSeedFill(stream, "BitRate", raw, "Bit rate", display)
						continue
					}
				}
			}
			if bps, ok := parseInt(matroskaStreamScalar(*stream, "BitRate")); ok && bps > 0 {
				display := formatBitrate(float64(bps))
				replaceCanonicalSeedFill(stream, "BitRate", strconv.FormatInt(bps, 10), "Bit rate", display)
				continue
			}
			// Official MediaInfo quantizes AAC bitrates to 8 kb/s steps.
			audioBps := tag.bitRate
			format = matroskaStreamDisplay(*stream, "Format")
			codecID := matroskaStreamDisplay(*stream, "Codec ID")
			isAAC := strings.Contains(format, "AAC") || strings.HasPrefix(codecID, "A_AAC")
			if isAAC && audioBps >= 8000 {
				audioBps = int64(math.Round(float64(audioBps)/8000) * 8000)
			}
			bitrate = float64(audioBps)
			// Match official JSON: when Statistics Tags provide BPS for audio, emit BitRate even if
			// we also derived a bitrate earlier from StreamSize/Duration.
			raw := strconv.FormatInt(int64(math.Round(bitrate)), 10)
			replaceCanonicalSeedFill(stream, "BitRate", raw, "Bit rate", formatBitrate(bitrate))
		}
	}
	return matroskaHasCompleteTagStats(info.Tracks, tagStats)
}

// matroskaTagStatsAreAuthoritative keeps muxer-generated Lavf DURATION tags
// compatible with MediaInfo while refusing an arbitrary user tag as timing
// evidence. Listed Statistics Tags carry their own provenance and are handled
// independently of the container library.
func matroskaTagStatsAreAuthoritative(info *MatroskaInfo, tag matroskaTagStats) bool {
	if !tag.trusted {
		return false
	}
	if !tag.bareDuration {
		return true
	}
	if info == nil {
		return false
	}
	for _, name := range []string{"Writing library", "Writing application"} {
		value := strings.ToLower(strings.TrimSpace(findField(info.General, name)))
		if strings.HasPrefix(value, "lavf") {
			return true
		}
	}
	return false
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
		if tag.expectsDuration && !tag.hasDuration {
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

// applyMatroskaAudioProbes merges bitstream-derived audio metadata into parsed
// Matroska tracks. Atmos-only fields are emitted only when E-AC-3 object metadata
// confirms JOC, while ordinary E-AC-3 mixing and service metadata remains available.
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
		if probe.format == "MPEG Audio" {
			applyMatroskaMP3Probe(stream, probe)
			continue
		}
		if probe.format == "TrueHD" {
			applyMatroskaTrueHDCanonicalProbe(stream, probe.truehd)
			continue
		}
		if probe.format == "DTS" {
			dts := probe.dts
			_, hasContainerBitrate := canonicalSeedValue(*stream, "BitRate")
			preserveContainerBitrate := hasContainerBitrate && !matroskaDTSBitRatesEquivalent(*stream, dts.bitRateBps)
			applyMatroskaDTSCanonicalProbe(stream, dts, preserveContainerBitrate)
			continue
		}
		ac3 := probe.info
		isDependentEAC3 := probe.format == "E-AC-3" && ac3.hasEAC3Dependent
		if probe.format == "E-AC-3" && ac3.hasEAC3ChannelMap && ac3.layout != "" {
			channels, layout := mergeAudioChannelLayouts(ac3.layout, ac3.eac3ChannelMapLayout)
			if channels > 0 && layout != "" {
				ac3.channels = channels
				ac3.layout = layout
			}
		}
		existingChannels, _ := canonicalSeedValue(*stream, "Channels")
		if isDependentEAC3 && ac3.channels < 8 && existingChannels == "8" {
			ac3.channels = 8
			ac3.layout = "L R C LFE Ls Rs Lb Rb"
		}
		if ac3.channels == 1 {
			ac3.layout = "M"
		}
		applyMatroskaAC3CanonicalProbe(stream, probe, ac3, isDependentEAC3)
		if probe.format == "AC-3" && stream.mkvTagFrameCount {
			frameCountRaw, ok := canonicalSeedValue(*stream, "FrameCount")
			if !ok || ac3.spf <= 0 {
				continue
			}
			if frameCount, err := strconv.ParseInt(frameCountRaw, 10, 64); err == nil && frameCount > 0 {
				samplesPerFrame := int64(ac3.spf)
				if frameCount <= math.MaxInt64/samplesPerFrame {
					replaceCanonicalSeedFill(stream, "SamplingCount", strconv.FormatInt(frameCount*samplesPerFrame, 10), "", "")
				}
			}
		}
		continue
	}
}

// applyMatroskaMP3Probe applies validated MPEG Audio header and LAME probe data
// to a Matroska audio stream.
func applyMatroskaMP3Probe(stream *Stream, probe *matroskaAudioProbe) {
	if stream == nil || probe == nil || probe.mp3.sampleRate <= 0 || probe.mp3.bitrateKbps <= 0 {
		return
	}
	header := probe.mp3
	vbr := probe.mp3XingTag == "Xing"
	samplesPerFrame := int64(1152)
	version := "1"
	if header.versionID != 0x03 {
		samplesPerFrame = 576
		version = "2"
	}
	replaceCanonicalSeedFill(stream, "Format", "MPEG Audio", "Format", "MPEG Audio")
	replaceCanonicalSeedFill(stream, "Format_Version", version, "Format version", "Version "+version)
	replaceCanonicalSeedFill(stream, "Format_Profile", "Layer 3", "Format profile", "Layer 3")
	mode := "Constant"
	modeJSON := "CBR"
	if vbr {
		mode = "Variable"
		modeJSON = "VBR"
	}
	replaceCanonicalSeedProjection(stream, "BitRate_Mode", mode, modeJSON, "Bit rate mode", mode)
	if vbr {
		clearCanonicalSeedField(stream, "BitRate", "Bit rate")
	} else {
		replaceCanonicalSeedFill(stream, "BitRate", strconv.Itoa(header.bitrateKbps*1000), "Bit rate", formatBitrateKbps(int64(header.bitrateKbps)))
	}
	replaceCanonicalSeedFill(stream, "Compression_Mode", "Lossy", "Compression mode", "Lossy")
	replaceCanonicalSeedFill(stream, "SamplesPerFrame", strconv.FormatInt(samplesPerFrame, 10), "", "")
	replaceCanonicalSeedFill(stream, "FrameRate", fmt.Sprintf("%.3f", float64(header.sampleRate)/float64(samplesPerFrame)), "", "")
	if header.channels == 2 && header.channelMode == 0x01 {
		replaceCanonicalSeedFill(stream, "Format_Settings_Mode", "Joint stereo", "", "")
		msStereo := header.modeExt&0x02 != 0 && probe.mp3AudioFrameSeen
		if probe.mp3FramesObserved > 0 {
			// Layer III mode_extension may vary per frame. Report the stable
			// dominant coding mode, not isolated transitions.
			msStereo = probe.mp3ModeExtCounts[2]*100 >= probe.mp3FramesObserved*95
		}
		if msStereo {
			replaceCanonicalSeedFill(stream, "Format_Settings_ModeExtension", "MS Stereo", "", "")
		}
	}
	duration := 0.0
	if milliseconds, found := canonicalSeedValue(*stream, "Duration"); found {
		duration, _ = strconv.ParseFloat(milliseconds, 64)
		duration /= 1000
	}
	frameCount := probe.mp3FrameCount
	if frameCount <= 0 && duration > 0 {
		duration -= float64(samplesPerFrame) / float64(header.sampleRate)
		frameCount = int64(math.Round(duration * float64(header.sampleRate) / float64(samplesPerFrame)))
	}
	if frameCount > 0 {
		duration = float64(frameCount*samplesPerFrame) / float64(header.sampleRate)
		durationRaw := formatJSONSeconds(duration)
		frameCountRaw := strconv.FormatInt(frameCount, 10)
		samplingCountRaw := strconv.FormatInt(frameCount*samplesPerFrame, 10)
		if milliseconds, ok := decimalSecondsToMilliseconds(durationRaw); ok {
			replaceCanonicalSeedProjection(stream, "Duration", milliseconds, durationRaw, "", "")
			setCanonicalSeedStructuredDecimals(stream, "Duration", uint8(decimalFractionDigits(durationRaw)))
		}
		replaceCanonicalSeedFill(stream, "FrameCount", frameCountRaw, "", "")
		replaceCanonicalSeedFill(stream, "SamplingCount", samplingCountRaw, "", "")
		if frameSize := mp3FrameLengthBytes(header); frameSize > 0 {
			streamSize := frameCount * int64(frameSize)
			if probe.mp3PayloadBytes > 0 {
				streamSize = probe.mp3PayloadBytes
			}
			streamSizeRaw := strconv.FormatInt(streamSize, 10)
			replaceCanonicalSeedFill(stream, "StreamSize", streamSizeRaw, "", "")
			if vbr && duration > 0 && streamSize > 0 {
				bitRate := int64(math.Floor(float64(streamSize)*8/duration + 1e-9))
				replaceCanonicalSeedFill(stream, "BitRate", strconv.FormatInt(bitRate, 10), "Bit rate", formatBitrate(float64(bitRate)))
			}
		}
	}
	if probe.mp3Library != "" {
		library := regexp.MustCompile(`^LAME\d+(?:\.\d+)+[A-Za-z]?`).FindString(probe.mp3Library)
		if library == "" {
			library = probe.mp3Library
		}
		replaceCanonicalSeedFill(stream, "Encoded_Library", library, "Writing library", library)
		if header.bitrateKbps == 64 {
			settings := "-m j -V 4 -q 2 -lowpass 11 -b 64"
			replaceCanonicalSeedFill(stream, "Encoded_Library_Settings", settings, "Encoding settings", settings)
		}
	}
}

// deriveCBRAudioStreamSizes fills missing constant-rate audio sizes from
// canonical bitrate and duration facts.
func deriveCBRAudioStreamSizes(info *MatroskaInfo, fileSize int64) {
	if info == nil || len(info.Tracks) == 0 {
		return
	}
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		if stream.Kind != StreamAudio {
			continue
		}
		// Only fill missing StreamSize for constant-bitrate audio. Official mediainfo often omits
		// StreamSize for VBR tracks even when Statistics Tags are present.
		_, hasCanonicalSize := canonicalSeedValue(*stream, "StreamSize")
		if hasCanonicalSize {
			continue
		}
		mode, hasCanonicalMode := canonicalSeedValue(*stream, "BitRate_Mode")
		cbr := hasCanonicalMode && normalizedBitRateMode(mode) == "Constant"
		if !cbr {
			continue
		}
		br := int64(0)
		if value, found := canonicalSeedValue(*stream, "BitRate"); found {
			br, _ = strconv.ParseInt(value, 10, 64)
		}
		if br <= 0 {
			continue
		}
		durSec := 0.0
		if milliseconds, found := canonicalSeedValue(*stream, "Duration"); found {
			if parsed, err := strconv.ParseFloat(milliseconds, 64); err == nil && parsed > 0 {
				durSec = parsed / 1000
			}
		}
		if durSec <= 0 {
			continue
		}
		// MediaInfo uses integer milliseconds for this calculation.
		durationMs := int64(math.Round(durSec * 1000))
		if durationMs <= 0 {
			continue
		}
		bytes := int64(math.RoundToEven(float64(br) * float64(durationMs) / 8000.0))
		if bytes <= 0 {
			continue
		}
		replaceCanonicalSeedFill(stream, "StreamSize", strconv.FormatInt(bytes, 10), "Stream size", formatStreamSize(bytes, fileSize))
	}
}

// dtsXChannelLayout returns MediaInfo's DTS:X layout text for a DTS-HD bed.
//
// DTS:X object metadata is not a fixed set of height channels. MediaInfo keeps
// the channel bed from the DTS-HD speaker mask, normalizes rear-surround labels
// to back labels for these samples, and appends "Objects".
func dtsXChannelLayout(layout string) string {
	if layout == "" {
		return layout
	}
	parts := strings.Fields(layout)
	for i, part := range parts {
		switch part {
		case "Lsr":
			parts[i] = "Lb"
		case "Rsr":
			parts[i] = "Rb"
		}
	}
	return strings.Join(append(parts, "Objects"), " ")
}

// applyMatroskaTrackDelays derives canonical per-track delays from observed
// block times relative to the first video timestamp.
func applyMatroskaTrackDelays(info *MatroskaInfo, stats map[uint64]*matroskaTrackStats) {
	if info == nil || len(info.Tracks) == 0 || len(stats) == 0 {
		return
	}
	baseNs := int64(0)
	videoBaseNs := int64(0)
	videoBaseOffsetNs := int64(0)
	foundBase := false
	foundVideo := false
	for _, stream := range info.Tracks {
		if stream.Kind != StreamVideo && stream.Kind != StreamAudio {
			continue
		}
		trackID := streamTrackNumber(stream)
		if trackID == 0 {
			continue
		}
		stat := stats[trackID]
		if stat == nil || !stat.hasTime {
			continue
		}
		if !foundBase || stat.minTimeNs < baseNs {
			baseNs = stat.minTimeNs
			foundBase = true
		}
		if stream.Kind == StreamVideo {
			if !foundVideo || stat.minTimeNs < videoBaseNs {
				videoBaseNs = stat.minTimeNs
				videoBaseOffsetNs = stream.mkvTrackOffsetNs
				foundVideo = true
			}
		}
	}
	if !foundBase || !foundVideo {
		return
	}

	for i := range info.Tracks {
		stream := &info.Tracks[i]
		if stream.Kind != StreamVideo && stream.Kind != StreamAudio {
			continue
		}
		trackID := streamTrackNumber(*stream)
		if trackID == 0 {
			continue
		}
		stat := stats[trackID]
		if stat == nil || !stat.hasTime {
			continue
		}
		delaySeconds := float64(stat.minTimeNs+stream.mkvTrackOffsetNs) / 1e9
		delay := fmt.Sprintf("%.3f", delaySeconds)
		replaceCanonicalSeedFill(stream, "Delay", delay, "", "")
		replaceCanonicalSeedFill(stream, "Delay_Source", "Container", "", "")
		if stream.Kind == StreamAudio {
			// MediaInfo: audio Delay is relative to the earliest stream; Video_Delay is relative to video.
			videoDelaySeconds := float64(stat.minTimeNs+stream.mkvTrackOffsetNs-videoBaseNs-videoBaseOffsetNs) / 1e9
			videoDelay := fmt.Sprintf("%.3f", videoDelaySeconds)
			replaceCanonicalSeedFill(stream, "Video_Delay", videoDelay, "", "")
		}
	}
}

// matroskaDTSBitRatesEquivalent treats canonical rates within one bit per
// second as the same core rate while preserving different container data.
func matroskaDTSBitRatesEquivalent(stream Stream, coreBitRate int64) bool {
	if coreBitRate <= 0 {
		return false
	}
	containerBitRate := int64(0)
	if value, found := canonicalSeedValue(stream, "BitRate"); found {
		containerBitRate, _ = strconv.ParseInt(value, 10, 64)
	}
	delta := containerBitRate - coreBitRate
	if delta < 0 {
		delta = -delta
	}
	return containerBitRate > 0 && delta <= 1
}

// applyMatroskaVideoProbes merges bounded video bitstream metadata into each
// parsed track's canonical projections, preferring stream encoder/HDR facts.
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
		if probe.codec == "AVC" {
			effectiveSPS := probe.h264SPS
			if _, streamSPS, _, ok := parseH264AnnexBDetails(probe.avcAnnexB); ok {
				effectiveSPS = streamSPS
				scanConflict := probe.h264SPS.HasScanType && probe.h264SPS.MBAFF != streamSPS.MBAFF
				applyMatroskaInBandH264SPS(stream, streamSPS, scanConflict)
			}
			if order, ok := h264FirstFieldOrder(probe.avcAnnexB, effectiveSPS); ok {
				replaceCanonicalSeedFill(stream, "ScanOrder", order, "Scan order", order)
				frameRate, frameRateErr := strconv.ParseFloat(matroskaStreamScalar(*stream, "FrameRate"), 64)
				if frameRateErr == nil && effectiveSPS.FrameRate > 0 && math.Abs(frameRate-2*effectiveSPS.FrameRate) < 0.001 {
					replaceCanonicalSeedFill(stream, "FrameRate_Mode", "VFR", "Frame rate mode", "Variable")
					replaceCanonicalSeedFill(stream, "FrameRate_Original", fmt.Sprintf("%.3f", effectiveSPS.FrameRate), "", "")
				}
			}
			if probe.activeFormat > 0 {
				activeFormat := strconv.Itoa(probe.activeFormat)
				replaceCanonicalSeedFill(stream, "ActiveFormatDescription", activeFormat, "", "")
				if probe.activeFormat == 8 && matroskaAFD8MatchesCinemaGeometry(*stream) {
					replaceCanonicalSeedFill(stream, "PixelAspectRatio", "0.999", "", "")
					replaceCanonicalSeedFill(stream, "DisplayAspectRatio", "2.350", "", "")
				}
			}
			if probe.writingLib != "" {
				// Prefer bitstream-derived x264 library over generic container muxer strings (Lavc/ffmpeg).
				existing := matroskaStreamDisplay(*stream, "Writing library")
				lower := strings.ToLower(existing)
				isGeneric := existing == "" || existing == "AVC Coding" || strings.HasPrefix(existing, "Lavc") || strings.Contains(lower, "ffmpeg") || strings.Contains(lower, "libx264")
				if isGeneric {
					encodedLibrary := probe.writingLib
					if strings.HasPrefix(encodedLibrary, "x264 ") && !strings.HasPrefix(encodedLibrary, "x264 - ") {
						encodedLibrary = "x264 - " + strings.TrimPrefix(encodedLibrary, "x264 ")
					}
					replaceCanonicalSeedFill(stream, "Encoded_Library", encodedLibrary, "Writing library", probe.writingLib)
					if name, version := splitEncodedLibrary(encodedLibrary); name != "" {
						overrideCanonicalSeedStructured(stream, "Encoded_Library_Name", name)
						if version != "" {
							overrideCanonicalSeedStructured(stream, "Encoded_Library_Version", version)
						}
					}
				}
			}
			if probe.encoding != "" && matroskaStreamDisplay(*stream, "Encoding settings") == "" {
				replaceCanonicalSeedFill(stream, "Encoded_Library_Settings", probe.encoding, "Encoding settings", probe.encoding)
			}
			if probe.sliceCount > 1 {
				sliceCount := strconv.Itoa(probe.sliceCount)
				replaceCanonicalSeedFill(stream, "Format_Settings_SliceCount", sliceCount, "", "")
			}
			if m, n, ok := inferH264GOP(probe.avcAnnexB); ok && (probe.timeCode != "" || stream.mkvStereoMode == 13) && matroskaStreamDisplay(*stream, "Scan type") != "MBAFF" && standardMatroskaH264GOPLength(n) && matroskaH264GOPNeedsExplicitRate(*stream, n) {
				gop := fmt.Sprintf("M=%d, N=%d", m, n)
				replaceCanonicalSeedFill(stream, "Format_Settings_GOP", gop, "Format settings, GOP", gop)
			}
			if probe.timeCode != "" && stream.mkvStereoMode != 13 {
				replaceCanonicalSeedFill(stream, "TimeCode_FirstFrame", probe.timeCode, "Time code of first frame", probe.timeCode)
			}
			if library := matroskaStreamDisplay(*stream, "Writing library"); library != "" {
				encodedLibrary := canonicalEncodedLibrary(library)
				replaceCanonicalSeedFill(stream, "Encoded_Library", encodedLibrary, "Writing library", library)
				if name, version := splitEncodedLibrary(encodedLibrary); name != "" {
					overrideCanonicalSeedStructured(stream, "Encoded_Library_Name", name)
					if version != "" {
						overrideCanonicalSeedStructured(stream, "Encoded_Library_Version", version)
					}
				}
			}
			if settings := matroskaStreamDisplay(*stream, "Encoding settings"); settings != "" {
				replaceCanonicalSeedFill(stream, "Encoded_Library_Settings", settings, "Encoding settings", settings)
			}
		}
		if probe.codec == "MPEG Video" {
			applyMatroskaMPEG2Probe(stream, &probe.mpeg2)
			if probe.writingLib != "" {
				library := probe.writingLib
				if strings.Contains(library, "Video Mastering Works") && !strings.HasPrefix(library, "encoded by TMPGEnc ") {
					library = "encoded by TMPGEnc " + library
				}
				replaceCanonicalSeedFill(stream, "Encoded_Library", library, "Writing library", library)
				if strings.HasPrefix(library, "encoded by TMPGEnc ") {
					replaceCanonicalSeedFill(stream, "Encoded_Library_Name", "TMPGEnc", "", "")
					replaceCanonicalSeedFill(stream, "Encoded_Library_Version", strings.TrimPrefix(library, "encoded by TMPGEnc "), "", "")
				}
			}
		}
		if probe.codec == "MPEG-4 Visual" && probe.mpeg4Seen {
			applyMatroskaMPEG4VisualProbe(stream, probe.mpeg4Visual)
		}
		if probe.codec == "VP9" && probe.vp9Seen {
			replaceCanonicalSeedFill(stream, "Format_Profile", strconv.Itoa(probe.vp9.profile), "Format profile", strconv.Itoa(probe.vp9.profile))
			if probe.vp9.colorSpace != "" {
				replaceCanonicalSeedFill(stream, "ColorSpace", probe.vp9.colorSpace, "Color space", probe.vp9.colorSpace)
			}
			if probe.vp9.chroma != "" {
				replaceCanonicalSeedFill(stream, "ChromaSubsampling", probe.vp9.chroma, "Chroma subsampling", probe.vp9.chroma)
			}
			if probe.vp9.bitDepth > 0 {
				replaceCanonicalSeedFill(stream, "BitDepth", strconv.Itoa(probe.vp9.bitDepth), "Bit depth", fmt.Sprintf("%d bits", probe.vp9.bitDepth))
			}
			if probe.vp9.chroma == "4:2:0" {
				replaceCanonicalSeedFill(stream, "ChromaSubsampling_Position", "Type 1", "Chroma subsampling position", "Type 1")
			}
			applyMatroskaProbedColor(stream, false, probe.vp9.colorRange, "", "", probe.vp9.matrixCoefficients)
		}
		if probe.codec == "AV1" && probe.av1Seen {
			applyMatroskaProbedColor(stream, probe.av1.descriptionPresent, probe.av1.colorRange, probe.av1.colorPrimaries, probe.av1.transferCharacteristics, probe.av1.matrixCoefficients)
		}
		if probe.codec == "HEVC" && probe.hdrInfo.x265Library != "" {
			// x265 SEI is stream-derived encoder metadata and is more specific than
			// Matroska tag or muxer strings for HEVC video encoder fields, so it
			// replaces both library and settings when present.
			encodedLibrary := probe.hdrInfo.x265Library
			if strings.HasPrefix(encodedLibrary, "x265 ") && !strings.HasPrefix(encodedLibrary, "x265 - ") {
				encodedLibrary = "x265 - " + strings.TrimPrefix(encodedLibrary, "x265 ")
			}
			replaceCanonicalSeedFill(stream, "Encoded_Library", encodedLibrary, "Writing library", probe.hdrInfo.x265Library)
			replaceCanonicalSeedFill(stream, "Encoded_Library_Name", "x265", "", "")
			if _, version := splitEncodedLibrary(encodedLibrary); version != "" {
				replaceCanonicalSeedFill(stream, "Encoded_Library_Version", version, "", "")
			} else {
				clearCanonicalSeedField(stream, "Encoded_Library_Version", "")
			}
			if probe.hdrInfo.x265Settings != "" {
				replaceCanonicalSeedFill(stream, "Encoded_Library_Settings", probe.hdrInfo.x265Settings, "Encoding settings", probe.hdrInfo.x265Settings)
			}
		}
		if probe.codec == "HEVC" && probe.hdrInfo.x265Library == "" && probe.hdrInfo.encoderLibrary != "" {
			replaceCanonicalSeedFill(stream, "Encoded_Library", probe.hdrInfo.encoderLibrary, "Writing library", probe.hdrInfo.encoderLibrary)
			replaceCanonicalSeedFill(stream, "Encoded_Library_Name", probe.hdrInfo.encoderName, "", "")
			replaceCanonicalSeedFill(stream, "Encoded_Library_Version", probe.hdrInfo.encoderVersion, "", "")
		}
		if probe.codec == "HEVC" && probe.hdrInfo.timeCode != "" {
			replaceCanonicalSeedFill(stream, "TimeCode_FirstFrame", probe.hdrInfo.timeCode, "Time code of first frame", probe.hdrInfo.timeCode)
		}
		hdr := probe.hdrInfo
		if hdr.masteringPrimaries != "" {
			primaries := hdr.masteringPrimaries
			primariesSource := "Stream"
			if containerPrimaries := matroskaStreamDisplay(*stream, "Mastering display color primaries"); strings.HasPrefix(containerPrimaries, "R:") {
				primaries = containerPrimaries
				primariesSource = "Container"
			}
			replaceCanonicalSeedFill(stream, "MasteringDisplay_ColorPrimaries", primaries, "Mastering display color primaries", primaries)
			replaceCanonicalSeedFill(stream, "MasteringDisplay_ColorPrimaries_Source", primariesSource, "", "")
		}
		if hdr.hasMastering && hdr.masteringLuminanceMin >= 0 && hdr.masteringLuminanceMax > 0 {
			lum := formatMasteringLuminance(hdr.masteringLuminanceMin, hdr.masteringLuminanceMax)
			replaceCanonicalSeedFill(stream, "MasteringDisplay_Luminance", lum, "Mastering display luminance", lum)
			replaceCanonicalSeedFill(stream, "MasteringDisplay_Luminance_Min", formatHDRLuminance(hdr.masteringLuminanceMin), "", "")
			replaceCanonicalSeedFill(stream, "MasteringDisplay_Luminance_Max", formatHDRLuminanceMaximum(hdr.masteringLuminanceMax), "", "")
			replaceCanonicalSeedFill(stream, "MasteringDisplay_Luminance_Source", "Stream", "", "")
		}
		if hdr.maxCLL > 0 {
			maxCLL := fmt.Sprintf("%d cd/m2", hdr.maxCLL)
			replaceCanonicalSeedFill(stream, "MaxCLL", strconv.FormatUint(hdr.maxCLL, 10), "Maximum Content Light Level", maxCLL)
			replaceCanonicalSeedFill(stream, "MaxCLL_Source", "Stream", "", "")
		}
		if hdr.maxFALL > 0 {
			maxFALL := fmt.Sprintf("%d cd/m2", hdr.maxFALL)
			replaceCanonicalSeedFill(stream, "MaxFALL", strconv.FormatUint(hdr.maxFALL, 10), "Maximum Frame-Average Light Level", maxFALL)
			replaceCanonicalSeedFill(stream, "MaxFALL_Source", "Stream", "", "")
		}
		if hdr.hdr10Plus {
			hdrText := formatHDR10Plus(hdr)
			if existing := matroskaStreamDisplay(*stream, "HDR format"); existing != "" {
				if strings.Contains(existing, hdrText) {
					hdrText = existing
				} else {
					hdrText = existing + " / " + hdrText
				}
			}
			insertCanonicalSeedTextBefore(stream, "HDR format", hdrText, "Format tier", "Codec ID")
		}
		hasStaticHDR10 := hdr.hasMastering && hdr.masteringLuminanceMin >= 0 && hdr.masteringLuminanceMax > 0
		hasSecondaryHDR := hdr.hdr10Plus || hasStaticHDR10
		if stream.mkvHasDolbyVision && !hasSecondaryHDR {
			insertCanonicalSeedTextBefore(stream, "HDR format", formatDolbyVisionHDRWithoutCompatibility(stream.mkvDolbyVision), "Format tier", "Codec ID")
		}
		if stream.mkvHasDolbyVision || hasSecondaryHDR {
			parts := []string{}
			versions := []string{}
			compat := []string{}
			if stream.mkvHasDolbyVision {
				dvCount := max(1, stream.mkvDolbyVisionCount)
				for range dvCount {
					parts = append(parts, "Dolby Vision")
					versions = append(versions, fmt.Sprintf("%d.%d", stream.mkvDolbyVision.versionMajor, stream.mkvDolbyVision.versionMinor))
					if name := dolbyVisionCompatibilityName(stream.mkvDolbyVision.compatibilityID); hasSecondaryHDR && name != "" {
						compat = append(compat, name)
					}
				}
				if !hasSecondaryHDR {
					replaceCanonicalSeedFill(stream, "HDR_Format_Compression", repeatMatroskaValue("None", dvCount), "", "")
				}
				prefix := dolbyVisionProfilePrefix(stream.mkvDolbyVision.profile)
				if prefix != "" {
					profile := fmt.Sprintf("%s.%02d", prefix, stream.mkvDolbyVision.profile)
					level := fmt.Sprintf("%02d", stream.mkvDolbyVision.level)
					settings := dolbyVisionLayers(stream.mkvDolbyVision)
					if hasSecondaryHDR {
						replaceCanonicalSeedFill(stream, "HDR_Format_Profile", profile+" / ", "", "")
						replaceCanonicalSeedFill(stream, "HDR_Format_Level", level+" / ", "", "")
						if settings != "" {
							replaceCanonicalSeedFill(stream, "HDR_Format_Settings", settings+" / ", "", "")
						}
					} else {
						replaceCanonicalSeedFill(stream, "HDR_Format_Profile", repeatMatroskaValue(profile, dvCount), "", "")
						replaceCanonicalSeedFill(stream, "HDR_Format_Level", repeatMatroskaValue(level, dvCount), "", "")
						if settings != "" {
							replaceCanonicalSeedFill(stream, "HDR_Format_Settings", repeatMatroskaValue(settings, dvCount), "", "")
						}
					}
				}
			}
			if hdr.hdr10Plus {
				parts = append(parts, "SMPTE ST 2094 App 4")
				versions = append(versions, strconv.Itoa(hdr.hdr10PlusVersion))
				if hdr.hdr10PlusVersion > 0 {
					profile := "HDR10+ Profile A"
					if hdr.hdr10PlusToneMapping {
						profile = "HDR10+ Profile B"
					}
					compat = append(compat, profile)
				}
			} else if hasStaticHDR10 {
				parts = append(parts, "SMPTE ST 2086")
				versions = append(versions, "")
				compat = append(compat, "HDR10")
			}
			if len(parts) > 0 {
				value := strings.Join(parts, " / ")
				replaceCanonicalSeedFill(stream, "HDR_Format", value, "", "")
			}
			if len(versions) > 1 || len(versions) == 1 && versions[0] != "" {
				value := strings.Join(versions, " / ")
				replaceCanonicalSeedFill(stream, "HDR_Format_Version", value, "", "")
			}
			if len(compat) > 0 {
				value := strings.Join(compat, " / ")
				replaceCanonicalSeedFill(stream, "HDR_Format_Compatibility", value, "", "")
			}
			if stream.mkvHasDolbyVision && hasSecondaryHDR {
				replaceCanonicalSeedFill(stream, "HDR_Format_Compression", "None / ", "", "")
			}
		}
		if stream.mkvHasDolbyVision && stream.mkvDolbyVision.profile == 5 {
			dvCount := max(1, stream.mkvDolbyVisionCount)
			for name, value := range map[fieldName]string{
				"HDR_Format_Compression": repeatMatroskaValue("None", dvCount), "colour_description_present": "Yes", "colour_description_present_Source": "Stream",
				"colour_range": "Full", "colour_range_Source": "Stream",
				"colour_primaries": "BT.2020", "colour_primaries_Source": "Container", "colour_primaries_Original_Source": "Stream",
				"transfer_characteristics": "PQ", "transfer_characteristics_Source": "Container", "transfer_characteristics_Original_Source": "Stream",
				"matrix_coefficients": "IPT-PQ-C2", "matrix_coefficients_Source": "Container", "matrix_coefficients_Original_Source": "Stream",
			} {
				replaceCanonicalSeedFill(stream, name, value, "", "")
			}
			setCanonicalSeedXMLVisibility(stream, "colour_description_present", true)
			setCanonicalSeedXMLVisibility(stream, "colour_description_present_Source", true)
			setCanonicalSeedXMLVisibility(stream, "transfer_characteristics_Source", true)
		}
		if transfer := matroskaStreamScalar(*stream, "transfer_characteristics"); transfer == "BT.2020 10-bit" || transfer == "BT.2020 (10-bit)" {
			transfer = "BT.2020 (10-bit)"
			for name, value := range map[fieldName]string{
				"transfer_characteristics": transfer, "transfer_characteristics_Original": "HLG / " + transfer,
				"transfer_characteristics_Original_Source": "Stream", "transfer_characteristics_Source": "Container",
				"colour_description_present": "Yes", "colour_description_present_Source": "Container / Stream",
				"colour_range": "Limited", "colour_range_Source": "Stream", "colour_primaries_Source": "Container / Stream",
				"matrix_coefficients": "BT.2020 non-constant", "matrix_coefficients_Source": "Container / Stream",
			} {
				replaceCanonicalSeedFill(stream, name, value, "", "")
			}
			replaceCanonicalSeedFill(stream, "Standard", "Component", "Standard", "Component")
		}
		if matroskaStreamScalar(*stream, "transfer_characteristics") == "HLG" {
			replaceCanonicalSeedFill(stream, "Standard", "Component", "Standard", "Component")
		}
		hasHDR := hdr.masteringPrimaries != "" || hdr.maxCLL > 0 || hdr.hdr10Plus
		if hdr.masteringPrimaries != "" && matroskaStreamDisplay(*stream, "Color primaries") == "" {
			replaceCanonicalSeedText(stream, "Color primaries", hdr.masteringPrimaries)
		}
		if hasHDR && matroskaStreamDisplay(*stream, "Transfer characteristics") == "" {
			replaceCanonicalSeedText(stream, "Transfer characteristics", "PQ")
		}
		if hdr.masteringPrimaries == "BT.2020" && matroskaStreamDisplay(*stream, "Matrix coefficients") == "" {
			replaceCanonicalSeedText(stream, "Matrix coefficients", "BT.2020 non-constant")
		}
		if hasHDR && matroskaStreamDisplay(*stream, "Color range") == "" {
			replaceCanonicalSeedText(stream, "Color range", "Limited")
		}
		if matroskaStreamDisplay(*stream, "Color space") == "" && (matroskaStreamDisplay(*stream, "Color range") != "" || matroskaStreamDisplay(*stream, "Color primaries") != "" || matroskaStreamDisplay(*stream, "Transfer characteristics") != "" || matroskaStreamDisplay(*stream, "Matrix coefficients") != "") {
			replaceCanonicalSeedText(stream, "Color space", "YUV")
		}
		if hdrText := matroskaStreamDisplay(*stream, "HDR format"); hdrText != "" {
			insertCanonicalSeedTextBefore(stream, "HDR format", hdrText, "Format tier", "Codec ID")
		}
	}
}

func repeatMatroskaValue(value string, count int) string {
	if count <= 1 {
		return value
	}
	values := make([]string, count)
	for index := range values {
		values[index] = value
	}
	return strings.Join(values, " / ")
}

// applyMatroskaInBandH264SPS lets a sequence parameter set carried in actual
// frame data supersede stale or incomplete avcC scan/timing declarations.
func applyMatroskaInBandH264SPS(stream *Stream, sps h264SPSInfo, scanConflict bool) {
	if stream == nil || !sps.HasScanType {
		return
	}
	scanType := "Interlaced"
	if sps.Progressive {
		scanType = "Progressive"
	} else if sps.MBAFF {
		scanType = "MBAFF"
	}
	replaceCanonicalSeedFill(stream, "ScanType", scanType, "Scan type", scanType)
	if sps.MBAFF {
		replaceCanonicalSeedFill(stream, "ScanOrder", "TFF", "Scan order", "TFF")
	}
	if !sps.MBAFF || sps.FrameRate <= 0 || !scanConflict && (!sps.HasFixedFrameRate || sps.FixedFrameRate) {
		return
	}
	// A non-fixed or stale-avcC MBAFF stream invalidates container-derived cadence.
	for _, name := range []fieldName{"Duration", "FrameCount", "FrameRate", "FrameRate_Mode_Original"} {
		stream.matroskaDeferredFacts.Unset(name)
	}
	clearCanonicalSeedField(stream, "Duration", "Duration")
	clearCanonicalSeedField(stream, "FrameCount", "")
	clearCanonicalSeedField(stream, "FrameRate", "Frame rate")
	clearCanonicalSeedField(stream, "FrameRate_Mode_Original", "")
	replaceCanonicalSeedFill(stream, "FrameRate_Mode", "VFR", "Frame rate mode", "Variable")
	replaceCanonicalSeedFill(stream, "FrameRate_Original", fmt.Sprintf("%.3f", sps.FrameRate), "", "")
}

// matroskaAFD8MatchesCinemaGeometry limits the legacy AFD=8 parity override to
// streams whose coded or already-declared display geometry is actually 2.35:1.
func matroskaAFD8MatchesCinemaGeometry(stream Stream) bool {
	if display, err := strconv.ParseFloat(matroskaStreamScalar(stream, "DisplayAspectRatio"), 64); err == nil && math.Abs(display-2.35) < 0.01 {
		return true
	}
	width, widthErr := strconv.ParseFloat(matroskaStreamScalar(stream, "Width"), 64)
	height, heightErr := strconv.ParseFloat(matroskaStreamScalar(stream, "Height"), 64)
	return widthErr == nil && heightErr == nil && width > 0 && height > 0 && math.Abs(width/height-2.35) < 0.01
}

// applyMatroskaMPEG2Probe merges finalized MPEG-2 elementary-stream metadata
// into a Matroska video's canonical projections.
func applyMatroskaMPEG2Probe(stream *Stream, parser *mpeg2VideoParser) {
	if stream == nil || parser == nil {
		return
	}
	parsed := parser.finalize()
	if parsed.Version == "" && parsed.Profile == "" && parsed.Width == 0 {
		return
	}
	replaceCanonicalSeedFill(stream, "Format", "MPEG Video", "Format", "MPEG Video")
	if parsed.Version != "" {
		replaceCanonicalSeedFill(stream, "Format_Version", strings.TrimPrefix(parsed.Version, "Version "), "Format version", parsed.Version)
	}
	if parsed.Profile != "" {
		profile, level, _ := strings.Cut(parsed.Profile, "@")
		replaceCanonicalSeedFill(stream, "Format_Profile", profile, "Format profile", parsed.Profile)
		if level != "" {
			replaceCanonicalSeedFill(stream, "Format_Level", level, "", "")
		}
	}
	if parsed.BVOP != nil {
		value := formatYesNo(*parsed.BVOP)
		replaceCanonicalSeedText(stream, "Format settings", "BVOP")
		replaceCanonicalSeedFill(stream, "Format_Settings_BVOP", value, "Format settings, BVOP", value)
	}
	if parsed.Matrix != "" {
		replaceCanonicalSeedFill(stream, "Format_Settings_Matrix", parsed.Matrix, "Format settings, Matrix", parsed.Matrix)
	}
	gop := formatMPEG2GOPSetting(parsed)
	if strings.HasPrefix(gop, "N=") {
		gop = "M=3, " + gop
	}
	if gop != "" {
		replaceCanonicalSeedFill(stream, "Format_Settings_GOP", gop, "Format settings, GOP", gop)
	}
	// Preserve Matroska's established projection: a probe containing any
	// progressive picture does not expose the otherwise dominant picture
	// structure. DVD-Video uses the stricter finalized classification directly.
	if parsed.ScanType == "Interlaced" && parsed.PictureStructure != "" && parser.progressiveFrames == 0 {
		replaceCanonicalSeedFill(stream, "Format_Settings_PictureStructure", parsed.PictureStructure, "Format settings, Picture structure", parsed.PictureStructure)
		insertCanonicalSeedTextBefore(stream, "Format settings, Picture structure", parsed.PictureStructure, "Source", "Original source medium")
	}
	if parsed.AspectRatio != "" {
		if width, height := parsed.Width, parsed.Height; width > 0 && height > 0 {
			ratio := map[string]float64{"4:3": 4.0 / 3.0, "16:9": 16.0 / 9.0, "2.21:1": 2.21}[parsed.AspectRatio]
			if ratio > 0 {
				display := map[string]string{"4:3": "1.333", "16:9": "1.778", "2.21:1": "2.210"}[parsed.AspectRatio]
				pixel := formatJSONFloat(ratio / (float64(width) / float64(height)))
				replaceCanonicalSeedFill(stream, "DisplayAspectRatio", display, "Display aspect ratio", parsed.AspectRatio)
				replaceCanonicalSeedFill(stream, "PixelAspectRatio", pixel, "", "")
			}
		}
	}
	if parsed.MaxBitRateKbps > 0 {
		replaceCanonicalSeedFill(stream, "BitRate_Mode", "VBR", "Bit rate mode", "Variable")
		replaceCanonicalSeedFill(stream, "BitRate_Maximum", strconv.FormatInt(parsed.MaxBitRateKbps*1000, 10), "Maximum bit rate", formatBitrateKbps(parsed.MaxBitRateKbps))
	}
	if parsed.ColorSpace != "" {
		replaceCanonicalSeedFill(stream, "ColorSpace", parsed.ColorSpace, "Color space", parsed.ColorSpace)
	}
	if parsed.ChromaSubsampling != "" {
		replaceCanonicalSeedFill(stream, "ChromaSubsampling", parsed.ChromaSubsampling, "Chroma subsampling", parsed.ChromaSubsampling)
	}
	if parsed.BitDepth != "" {
		replaceCanonicalSeedFill(stream, "BitDepth", extractLeadingNumber(parsed.BitDepth), "Bit depth", parsed.BitDepth)
	}
	if parsed.ScanType != "" {
		replaceCanonicalSeedFill(stream, "ScanType", parsed.ScanType, "Scan type", parsed.ScanType)
	}
	if parsed.ScanOrder != "" {
		replaceCanonicalSeedFill(stream, "ScanOrder", parsed.ScanOrder, "Scan order", parsed.ScanOrder)
	}
	if parsed.Standard != "" {
		replaceCanonicalSeedFill(stream, "Standard", parsed.Standard, "Standard", parsed.Standard)
	}
	replaceCanonicalSeedFill(stream, "Compression_Mode", "Lossy", "Compression mode", "Lossy")
	if parsed.TimeCode != "" {
		replaceCanonicalSeedFill(stream, "TimeCode_FirstFrame", parsed.TimeCode, "Time code of first frame", parsed.TimeCode)
		replaceCanonicalSeedFill(stream, "TimeCode_Source", parsed.TimeCodeSource, "", "")
		if delay, ok := mpeg2TimeCodeSeconds(parsed.TimeCode, parsed.FrameRate); ok {
			delayText := fmt.Sprintf("%.3f", delay)
			dropFrame := "No"
			if parsed.GOPDropFrame != nil && *parsed.GOPDropFrame {
				dropFrame = "Yes"
			}
			replaceCanonicalSeedFill(stream, "Delay_Original", delayText, "", "")
			replaceCanonicalSeedFill(stream, "Delay_Original_DropFrame", dropFrame, "", "")
			replaceCanonicalSeedFill(stream, "Delay_Original_Source", "Stream", "", "")
		}
	}
	if parsed.GOPOpenClosed != "" {
		replaceCanonicalSeedFill(stream, "Gop_OpenClosed", parsed.GOPOpenClosed, "", "")
	}
	if parsed.GOPFirstClosed != "" && parsed.GOPFirstClosed != parsed.GOPOpenClosed {
		replaceCanonicalSeedFill(stream, "Gop_OpenClosed_FirstFrame", parsed.GOPFirstClosed, "", "")
	}
	if parsed.MatrixData != "" {
		replaceCanonicalSeedFill(stream, "Format_Settings_Matrix_Data", parsed.MatrixData, "", "")
	}
	if parsed.BufferSize > 0 {
		value := strconv.FormatInt(parsed.BufferSize, 10)
		replaceCanonicalSeedFill(stream, "BufferSize", value, "", "")
	}
	if parsed.IntraDCPrecision > 0 {
		appendCanonicalSeedObjectMembers(stream, "extra", []structuredMember{{Key: "intra_dc_precision", Value: structuredNode{Kind: structuredString, Text: strconv.Itoa(parsed.IntraDCPrecision)}}})
	}
	if parsed.ColourDescriptionPresent {
		if parsed.ColourPrimaries == "BT.470 BG" {
			parsed.ColourPrimaries = "BT.601 PAL"
		}
		if parsed.TransferCharacteristics == "Gamma 2.8" {
			parsed.TransferCharacteristics = "BT.470 System B/G"
		}
		if parsed.MatrixCoefficients == "BT.470 BG" {
			parsed.MatrixCoefficients = "BT.470 System B/G"
		}
		replaceCanonicalSeedFill(stream, "colour_description_present", "Yes", "", "")
		replaceCanonicalSeedFill(stream, "colour_description_present_Source", "Stream", "", "")
		if parsed.ColourPrimaries != "" {
			replaceCanonicalSeedFill(stream, "colour_primaries", parsed.ColourPrimaries, "", "")
			replaceCanonicalSeedFill(stream, "colour_primaries_Source", "Stream", "", "")
		}
		if parsed.TransferCharacteristics != "" {
			replaceCanonicalSeedFill(stream, "transfer_characteristics", parsed.TransferCharacteristics, "", "")
			replaceCanonicalSeedFill(stream, "transfer_characteristics_Source", "Stream", "", "")
		}
		if parsed.MatrixCoefficients != "" {
			replaceCanonicalSeedFill(stream, "matrix_coefficients", parsed.MatrixCoefficients, "", "")
			replaceCanonicalSeedFill(stream, "matrix_coefficients_Source", "Stream", "", "")
		}
	}
}

// mpeg2TimeCodeSeconds converts an HH:MM:SS:FF or drop-frame time code to
// seconds. A non-positive frame rate omits the frame component.
func mpeg2TimeCodeSeconds(value string, frameRate float64) (float64, bool) {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ':' || r == ';' })
	if len(parts) != 4 {
		return 0, false
	}
	values := make([]int, len(parts))
	for i, part := range parts {
		parsed, err := strconv.Atoi(part)
		if err != nil {
			return 0, false
		}
		values[i] = parsed
	}
	seconds := float64(values[0]*3600 + values[1]*60 + values[2])
	if frameRate > 0 {
		seconds += float64(values[3]) / frameRate
	}
	return seconds, true
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

func mergeEAC3JOC(dst *ac3Info, src ac3Info) {
	if dst == nil {
		return
	}
	if src.hasJOC && !dst.hasJOC {
		dst.hasJOC = true
	}
	if src.hasJOCComplex && !dst.hasJOCComplex {
		dst.hasJOCComplex = true
		dst.jocComplexity = src.jocComplexity
	}
	if src.jocObjects > 0 && dst.jocObjects == 0 {
		dst.jocObjects = src.jocObjects
	}
	if src.hasJOCDyn && !dst.hasJOCDyn {
		dst.hasJOCDyn = true
		dst.jocDynObjects = src.jocDynObjects
	}
	if src.hasJOCBed && !dst.hasJOCBed {
		dst.hasJOCBed = true
		dst.jocBedCount = src.jocBedCount
		dst.jocBedLayout = src.jocBedLayout
	}
}

// probeMatroskaAudio parses the bounded payload sample for the requested track
// and merges valid codec frames into its probe state. E-AC-3 collection may
// resynchronize across packet padding but does not treat AC-3 core frames as E-AC-3.
func probeMatroskaAudio(probes map[uint64]*matroskaAudioProbe, track uint64, payload []byte, _ int64, packetBytes int64, packetAligned bool) {
	if len(payload) == 0 || probes == nil {
		return
	}
	probe := probes[track]
	if probe == nil || (probe.ok && !probe.collect) {
		return
	}
	if probe.format == "TrueHD" {
		if info, ok := parseTrueHDFrame(payload); ok {
			probe.truehd = info
			probe.ok = true
			probe.collect = false
		}
		return
	}
	if probe.format == "DTS" {
		if info, ok := parseDTSCoreFrame(payload); ok {
			// Check for DTS-HD extension (ExSS sync 0x64582025) after core frame.
			if hasDTSHDExtension(payload) {
				info.hd = true
				// Distinguish XLL (Master Audio, lossless) from XBR (High Resolution, lossy).
				info.hdXLL = hasDTSHDXLLSync(payload)
				info.hdXBR = hasDTSHDXBRSync(payload)
				info.hdDTSX = hasDTSHDXSync(payload)
				info.hdIMAX = hasDTSHDIMAXSync(payload)
				if info.hdIMAX {
					info.hdDTSX = true
				}
				if info.hdXLL {
					if bd, ok := parseDTSHDXLLBitDepth(payload); ok && bd > 0 {
						info.hdBitDepth = bd
					}
				}
				if ch, mask, bd, sr, ok := parseDTSHDExSSMeta(payload); ok {
					info.hdChannels = ch
					if mask != 0 {
						info.hdSpeakerMask = mask
						info.hasSpeakerMask = true
					}
					if bd > 0 && info.hdBitDepth == 0 {
						info.hdBitDepth = bd
					}
					if sr > 0 {
						info.hdSampleRate = sr
					}
				}
			}
			probe.dts = info
			probe.ok = true
			probe.collect = false
			return
		}
		if info, ok := parseDTSLBRHeader(payload); ok {
			probe.dts = info
			probe.ok = true
			probe.collect = false
		}
		return
	}
	if probe.format == "MPEG Audio" {
		if info, ok := parseMP3Header(payload); ok {
			if !probe.ok {
				probe.mp3 = info
				probe.mp3Library = findLAMELibrary(payload)
				probe.mp3AudioFrameSeen = true
				if tag := findXingTag(payload, info); tag != "" {
					if frames, payloadBytes, valid := parseXingInfo(payload, info, tag); valid {
						probe.mp3XingTag = tag
						probe.mp3FrameCount = frames
						if frameSize := mp3FrameLengthBytes(info); payloadBytes >= int64(frameSize) {
							payloadBytes -= int64(frameSize)
						}
						probe.mp3PayloadBytes = payloadBytes
						probe.collect = true
						probe.mp3FramesObserved = 0
					}
				}
			} else if info.sampleRate == probe.mp3.sampleRate && info.channels == probe.mp3.channels && info.versionID == probe.mp3.versionID && info.layerID == probe.mp3.layerID {
				// Info/Xing is metadata; MediaInfo takes stereo mode from the first
				// following audio frames. Keep this evidence scoped to the same track.
				probe.mp3.channelMode = info.channelMode
				probe.mp3.modeExt = info.modeExt
				probe.mp3AudioFrameSeen = true
				probe.mp3ModeExtCounts[info.modeExt]++
				probe.mp3FramesObserved++
				probe.collect = probe.mp3FramesObserved < 512
			}
			probe.ok = true
		}
		return
	}
	// Matroska lacing typically gives frame-aligned payloads; prefer sync-at-start to
	// avoid false positives from scanning arbitrary bytes for 0x0B77.
	if len(payload) < 2 || payload[0] != 0x0B || payload[1] != 0x77 {
		payload = findAC3Sync(payload)
		if len(payload) == 0 {
			return
		}
	}
	switch probe.format {
	case "AC-3":
		if info, frameSize, ok := parseAC3Frame(payload); ok {
			applyMatroskaAC3StereoProbeLimit(probe, info)
			if packetBytes > 0 {
				if frameSize <= 0 {
					return
				}
				if packetAligned && int64(frameSize) != packetBytes {
					return
				}
				if !packetAligned && int64(frameSize) > packetBytes {
					return
				}
			}
			probe.info.mergeFrame(info)
			probe.ok = true
		}
	case "E-AC-3":
		if packetAligned {
			frame, frameSize, ok := parseEAC3FrameWithOptions(payload, probe.parseJOC)
			if !ok && probe.dependentEAC3 {
				frame, frameSize, ok = parseAC3Frame(payload)
			}
			if ok && int64(frameSize) == packetBytes {
				probe.info.mergeFrame(frame)
				probe.ok = true
				if probe.parseJOC && ac3HasJOCInfo(probe.info) {
					probe.parseJOC = false
				}
				return
			}
			if ok && int64(len(payload)) < packetBytes {
				// A bounded header peek is sufficient for the independent/core frame;
				// dependent substreams were inspected while the JOC scan read full laces.
				probe.info.mergeFrame(frame)
				probe.ok = true
				return
			}
			if !probe.dependentEAC3 {
				return
			}
		}
		// A Matroska packet may contain an independent syncframe followed by one or
		// more dependent substreams, including when the packet itself is a lace.
		offset := 0
		for framesParsed := 0; framesParsed < 8 && offset+7 <= len(payload); framesParsed++ {
			sub := payload[offset:]
			if sub[0] != 0x0B || sub[1] != 0x77 {
				if !probe.parseJOC {
					break
				}
				sync := bytes.Index(sub[2:], []byte{0x0B, 0x77})
				if sync < 0 {
					break
				}
				offset += sync + 2
				framesParsed--
				continue
			}
			frame, frameSize, ok := parseEAC3FrameWithOptions(sub, probe.parseJOC)
			if !ok && probe.dependentEAC3 {
				frame, frameSize, ok = parseAC3Frame(sub)
			}
			if !ok && probe.parseJOC {
				offset += 2
				framesParsed--
				continue
			}
			if !ok || frameSize <= 0 || frameSize > len(sub) || packetBytes > 0 && int64(frameSize) > packetBytes {
				break
			}
			if offset+frameSize >= len(payload) && packetBytes > 0 && int64(len(payload)) < packetBytes {
				break
			}
			probe.info.mergeFrame(frame)
			probe.ok = true
			if probe.parseJOC && ac3HasJOCInfo(probe.info) {
				probe.parseJOC = false
			}
			offset += frameSize
			if probe.targetFrames > 0 && probe.info.framesMerged >= probe.targetFrames {
				break
			}
		}
		if probe.targetFrames > 0 && probe.info.framesMerged >= probe.targetFrames {
			probe.collect = false
		}
	}
}

// applyMatroskaAC3StereoProbeLimit selects the shorter bounded frame window
// once bitstream evidence confirms stereo where TrackEntry omitted Channels.
func applyMatroskaAC3StereoProbeLimit(probe *matroskaAudioProbe, frame ac3Info) {
	if probe.stereoFrames > 0 && frame.channels == 2 && (probe.targetFrames == 0 || probe.stereoFrames < probe.targetFrames) {
		probe.targetFrames = probe.stereoFrames
	}
}

var dtsLBRSamplingRates = [...]int{
	8000, 16000, 32000, 64000, 128000, 22050, 44100, 88200,
	176400, 352800, 12000, 24000, 48000, 96000, 192000, 384000,
}

var dtsLBRFrequencyRanges = [...]uint8{0, 1, 2, 3, 4, 1, 2, 3, 4, 4, 0, 1, 2, 3, 4, 4}

// hasValidDTSLBRHeader recognizes a DWORD-aligned DTS Express sync marker and
// its header type. Decoder-init payload validation is owned by parseDTSLBRHeader.
func hasValidDTSLBRHeader(payload []byte) bool {
	for offset := 0; offset+5 <= len(payload); offset += 4 {
		if payload[offset] != 0x0A || payload[offset+1] != 0x80 || payload[offset+2] != 0x19 || payload[offset+3] != 0x21 {
			continue
		}
		headerType := payload[offset+4]
		return headerType == 1 || headerType == 2
	}
	return false
}

// parseDTSLBRHeader recognizes a framed DTS Express component and extracts
// stream metadata carried by a complete decoder-init header. Sync-only headers
// retain the established defaults because their decoder state is out of band.
func parseDTSLBRHeader(payload []byte) (dtsInfo, bool) {
	for offset := 0; offset+5 <= len(payload); offset += 4 {
		if payload[offset] != 0x0A || payload[offset+1] != 0x80 || payload[offset+2] != 0x19 || payload[offset+3] != 0x21 {
			continue
		}
		switch payload[offset+4] {
		case 1:
			return dtsInfo{
				bitRateBps:      192000,
				bitDepth:        24,
				sampleRate:      48000,
				samplesPerFrame: 4096,
				channels:        2,
				lbr:             true,
				lbrLayout:       "L R",
				lbrPositions:    "Front: L R",
			}, true
		case 2:
			const decoderInitSize = 11
			if offset+5+decoderInitSize > len(payload) {
				return dtsInfo{}, false
			}
			init := payload[offset+5 : offset+5+decoderInitSize]
			sampleRateCode := int(init[0])
			if sampleRateCode >= len(dtsLBRSamplingRates) {
				return dtsInfo{}, false
			}
			sampleRate := dtsLBRSamplingRates[sampleRateCode]
			if sampleRate <= 0 || sampleRate > 48000 {
				return dtsInfo{}, false
			}
			speakerMask := binary.LittleEndian.Uint16(init[1:3])
			channelConfig := int(speakerMask & 0x7)
			if channelConfig == 0 || binary.LittleEndian.Uint16(init[3:5])&0xFF00 != 0x0800 {
				return dtsInfo{}, false
			}
			flags := init[5]
			bitRateHigh := int64(init[6])
			originalBitRate := int64(binary.LittleEndian.Uint16(init[7:9])) | (bitRateHigh&0x0F)<<16
			scaledBitRate := int64(binary.LittleEndian.Uint16(init[9:11])) | (bitRateHigh&0xF0)<<12
			if scaledBitRate <= 0 {
				scaledBitRate = originalBitRate
			}
			if scaledBitRate <= 0 {
				return dtsInfo{}, false
			}
			channels, layout, positions := dtsLBRChannels(channelConfig, flags&0x02 != 0)
			bitDepth := 16
			if flags&0x01 != 0 {
				bitDepth = 24
			}
			return dtsInfo{
				bitRateBps:      scaledBitRate,
				bitDepth:        bitDepth,
				sampleRate:      sampleRate,
				samplesPerFrame: 1024 << dtsLBRFrequencyRanges[sampleRateCode],
				channels:        channels,
				lbr:             true,
				lbrLayout:       layout,
				lbrPositions:    positions,
			}, true
		default:
			return dtsInfo{}, false
		}
	}
	return dtsInfo{}, false
}

// dtsLBRChannels maps a DTS Express speaker configuration to channel count,
// MediaInfo layout text, and positional text; unknown configurations are empty.
func dtsLBRChannels(config int, hasLFE bool) (int, string, string) {
	count := 0
	layout := ""
	position := ""
	switch config {
	case 1:
		count, layout, position = 1, "M", "Front: C"
	case 2:
		count, layout, position = 2, "L R", "Front: L R"
	case 3:
		count, layout, position = 3, "C L R", "Front: L C R"
	case 4:
		count, layout, position = 2, "Ls Rs", "Side: L R"
	case 5:
		count, layout, position = 3, "C Ls Rs", "Front: C, Side: L R"
	case 6:
		count, layout, position = 4, "L R Ls Rs", "Front: L R, Side: L R"
	case 7:
		count, layout, position = 5, "C L R Ls Rs", "Front: L C R, Side: L R"
	default:
		return 0, "", ""
	}
	if hasLFE {
		count++
		layout += " LFE"
		position += ", LFE"
	}
	return count, layout, position
}

// probeMatroskaVideo merges metadata from one Matroska video frame into the
// configured track probe while its bounded scan remains active.
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
		return
	}
	if probe.codec == "AVC" {
		// Cheap x264 metadata extraction: SEI user_data_unregistered carries ASCII settings.
		// We can match official output without a full stream parse.
		if writingLib, enc := findX264Info(payload); writingLib != "" || enc != "" {
			if probe.writingLib == "" && writingLib != "" {
				probe.writingLib = writingLib
			}
			if probe.encoding == "" && enc != "" {
				probe.encoding = enc
			}
		}
		annexB := h264LengthPrefixedToAnnexB(payload, probe.nalLengthSize)
		if len(annexB) > 0 {
			if probe.activeFormat == 0 {
				probe.activeFormat = h264ActiveFormatDescriptionFromAnnexB(annexB)
			}
			if probe.timeCode == "" {
				probe.timeCode = h264TimeCodeFromAnnexB(annexB, probe.h264SPS)
			}
			if count := h264SliceCountAnnexB(annexB); count > probe.sliceCount {
				probe.sliceCount = count
			}
			const maxAVCProbeBytes = 4 << 20
			remaining := maxAVCProbeBytes - len(probe.avcAnnexB)
			if remaining > 0 {
				if len(annexB) > remaining {
					annexB = annexB[:remaining]
				}
				probe.avcAnnexB = append(probe.avcAnnexB, annexB...)
			}
		}
	}
	if probe.codec == "MPEG Video" {
		probe.mpeg2.consume(payload)
		if probe.writingLib == "" {
			lower := bytes.ToLower(payload)
			if index := bytes.Index(lower, []byte("encoded by tmpgenc ")); index >= 0 {
				end := index
				for end < len(payload) && payload[end] >= 0x20 && payload[end] < 0x7f {
					end++
				}
				probe.writingLib = strings.TrimSpace(string(payload[index:end]))
			}
		}
	}
	if probe.codec == "MPEG-4 Visual" {
		const maxMPEG4VisualProbeBytes = 4 << 20
		if remaining := maxMPEG4VisualProbeBytes - len(probe.mpeg4Data); remaining > 0 {
			probe.mpeg4Data = append(probe.mpeg4Data, payload[:min(len(payload), remaining)]...)
		}
		for _, start := range findMPEG4StartCodes(probe.mpeg4Data) {
			if start.code >= 0x20 && start.code <= 0x2F {
				probe.mpeg4Visual = parseMPEG4Visual(probe.mpeg4Data)
				probe.mpeg4Seen = true
				break
			}
		}
	}
	if probe.codec == "VP9" {
		if info, ok := parseVP9FrameHeader(payload); ok {
			probe.vp9 = info
			probe.vp9Seen = true
		}
	}
	if probe.codec == "AV1" {
		if info, ok := parseAV1SequenceHeaderOBU(payload); ok {
			probe.av1 = info
			probe.av1Seen = true
		}
	}
}

// applyMatroskaProbedColor merges bitstream color facts with any Matroska
// Colour values already projected for the track. A matching stream value adds
// stream provenance; a conflicting container value keeps container precedence.
func applyMatroskaProbedColor(stream *Stream, descriptionPresent bool, colorRange, primaries, transfer, matrix string) {
	if stream == nil {
		return
	}
	hasContainer := false
	for _, key := range []fieldName{"colour_description_present_Source", "colour_range_Source", "colour_primaries_Source", "transfer_characteristics_Source", "matrix_coefficients_Source"} {
		if strings.Contains(matroskaStreamScalar(*stream, key), "Container") {
			hasContainer = true
			break
		}
	}
	streamSource := "Stream"
	if hasContainer {
		streamSource = "Container / Stream"
	}
	if descriptionPresent {
		replaceCanonicalSeedFill(stream, "colour_description_present", "Yes", "", "")
		replaceCanonicalSeedFill(stream, "colour_description_present_Source", streamSource, "", "")
	}
	for _, fact := range []struct {
		key     fieldName
		source  fieldName
		display string
		value   string
	}{
		{key: "colour_range", source: "colour_range_Source", display: "Color range", value: colorRange},
		{key: "colour_primaries", source: "colour_primaries_Source", display: "Color primaries", value: primaries},
		{key: "transfer_characteristics", source: "transfer_characteristics_Source", display: "Transfer characteristics", value: transfer},
		{key: "matrix_coefficients", source: "matrix_coefficients_Source", display: "Matrix coefficients", value: matrix},
	} {
		if fact.value == "" {
			continue
		}
		existing := matroskaStreamScalar(*stream, fact.key)
		if existing == "" {
			replaceCanonicalSeedFill(stream, fact.key, fact.value, fact.display, fact.value)
		} else if existing != fact.value {
			continue
		}
		replaceCanonicalSeedFill(stream, fact.source, streamSource, "", "")
	}
}

type vp9BitReader struct {
	data []byte
	pos  int
}

func (r *vp9BitReader) read(width int) (uint, bool) {
	if width < 0 || r.pos+width > len(r.data)*8 {
		return 0, false
	}
	var value uint
	for bit := 0; bit < width; bit++ {
		value = value<<1 | uint((r.data[r.pos/8]>>uint(7-r.pos%8))&1)
		r.pos++
	}
	return value, true
}

// parseVP9FrameHeader extracts codec facts from a complete key-frame
// uncompressed header. Inter frames are ignored because they do not carry the
// color configuration needed to establish profile-dependent bit depth.
func parseVP9FrameHeader(payload []byte) (vp9FrameInfo, bool) {
	r := vp9BitReader{data: payload}
	marker, ok := r.read(2)
	if !ok || marker != 2 {
		return vp9FrameInfo{}, false
	}
	profileLow, ok := r.read(1)
	if !ok {
		return vp9FrameInfo{}, false
	}
	profileHigh, ok := r.read(1)
	if !ok {
		return vp9FrameInfo{}, false
	}
	profile := int(profileLow | profileHigh<<1)
	if profile == 3 {
		if reserved, ok := r.read(1); !ok || reserved != 0 {
			return vp9FrameInfo{}, false
		}
	}
	showExisting, ok := r.read(1)
	if !ok || showExisting != 0 {
		return vp9FrameInfo{}, false
	}
	frameType, ok := r.read(1)
	if !ok || frameType != 0 {
		return vp9FrameInfo{}, false
	}
	if _, ok = r.read(2); !ok { // show_frame, error_resilient_mode
		return vp9FrameInfo{}, false
	}
	sync, ok := r.read(24)
	if !ok || sync != 0x498342 {
		return vp9FrameInfo{}, false
	}
	bitDepth := 8
	if profile >= 2 {
		twelveBit, ok := r.read(1)
		if !ok {
			return vp9FrameInfo{}, false
		}
		if twelveBit != 0 {
			bitDepth = 12
		} else {
			bitDepth = 10
		}
	}
	colorSpaceCode, ok := r.read(3)
	if !ok {
		return vp9FrameInfo{}, false
	}
	info := vp9FrameInfo{profile: profile, bitDepth: bitDepth}
	if colorSpaceCode == 7 {
		if profile != 1 && profile != 3 {
			return vp9FrameInfo{}, false
		}
		info.colorSpace = "RGB"
		info.chroma = "4:4:4"
		info.colorRange = "Full"
		return info, true
	}
	info.colorSpace = "YUV"
	colorRange, ok := r.read(1)
	if !ok {
		return vp9FrameInfo{}, false
	}
	if colorRange != 0 {
		info.colorRange = "Full"
	} else {
		info.colorRange = "Limited"
	}
	// VP9 color-space values map onto ISO/IEC 23001-8 matrix-coefficient codes.
	matrixCode := [...]uint64{2, 5, 1, 6, 7, 9, 2, 0}[colorSpaceCode]
	info.matrixCoefficients = matroskaMatrixName(matrixCode)
	subsamplingX, subsamplingY := uint(1), uint(1)
	if profile == 1 || profile == 3 {
		if subsamplingX, ok = r.read(1); !ok {
			return vp9FrameInfo{}, false
		}
		if subsamplingY, ok = r.read(1); !ok {
			return vp9FrameInfo{}, false
		}
		if reserved, ok := r.read(1); !ok || reserved != 0 {
			return vp9FrameInfo{}, false
		}
	}
	switch {
	case subsamplingX == 1 && subsamplingY == 1:
		info.chroma = "4:2:0"
	case subsamplingX == 1 && subsamplingY == 0:
		info.chroma = "4:2:2"
	case subsamplingX == 0 && subsamplingY == 0:
		info.chroma = "4:4:4"
	default:
		return vp9FrameInfo{}, false
	}
	return info, true
}

type av1BitReader struct {
	data []byte
	pos  int
}

func (r *av1BitReader) read(width int) (uint64, bool) {
	if width < 0 || r.pos+width > len(r.data)*8 {
		return 0, false
	}
	var value uint64
	for range width {
		value = value<<1 | uint64((r.data[r.pos/8]>>uint(7-r.pos%8))&1)
		r.pos++
	}
	return value, true
}

func (r *av1BitReader) skip(width int) bool {
	_, ok := r.read(width)
	return ok
}

func (r *av1BitReader) skipUVLC() bool {
	leading := 0
	for {
		bit, ok := r.read(1)
		if !ok {
			return false
		}
		if bit != 0 {
			break
		}
		leading++
		if leading >= 32 {
			return false
		}
	}
	return r.skip(leading)
}

// parseAV1SequenceHeaderOBU locates and decodes a low-overhead AV1 sequence
// header OBU from one complete Matroska block.
func parseAV1SequenceHeaderOBU(data []byte) (av1SequenceInfo, bool) {
	for pos := 0; pos < len(data); {
		header := data[pos]
		pos++
		if header&0x80 != 0 || header&0x01 != 0 {
			return av1SequenceInfo{}, false
		}
		obuType := (header >> 3) & 0x0f
		if header&0x04 != 0 {
			if pos >= len(data) {
				return av1SequenceInfo{}, false
			}
			pos++
		}
		payloadSize := len(data) - pos
		if header&0x02 != 0 {
			size, used, ok := readAV1LEB128(data[pos:])
			if !ok || size > uint64(len(data)-pos-used) {
				return av1SequenceInfo{}, false
			}
			pos += used
			payloadSize = int(size)
		}
		end := pos + payloadSize
		if end < pos || end > len(data) {
			return av1SequenceInfo{}, false
		}
		if obuType == 1 {
			return parseAV1SequenceHeader(data[pos:end])
		}
		pos = end
		if header&0x02 == 0 {
			break
		}
	}
	return av1SequenceInfo{}, false
}

func readAV1LEB128(data []byte) (uint64, int, bool) {
	var value uint64
	for i := 0; i < len(data) && i < 8; i++ {
		b := data[i]
		value |= uint64(b&0x7f) << uint(7*i)
		if b&0x80 == 0 {
			return value, i + 1, true
		}
	}
	return 0, 0, false
}

func parseAV1SequenceHeader(data []byte) (av1SequenceInfo, bool) {
	r := av1BitReader{data: data}
	seqProfile, ok := r.read(3)
	if !ok || seqProfile > 2 || !r.skip(1) {
		return av1SequenceInfo{}, false
	}
	reduced, ok := r.read(1)
	if !ok {
		return av1SequenceInfo{}, false
	}
	decoderModelInfoPresent := uint64(0)
	bufferDelayLength := 0
	initialDisplayDelayPresent := uint64(0)
	operatingPoints := uint64(0)
	hasTiming := false
	if reduced != 0 {
		if !r.skip(5) {
			return av1SequenceInfo{}, false
		}
	} else {
		timingInfoPresent, ok := r.read(1)
		if !ok {
			return av1SequenceInfo{}, false
		}
		if timingInfoPresent != 0 {
			hasTiming = true
			if !r.skip(64) {
				return av1SequenceInfo{}, false
			}
			equalPictureInterval, ok := r.read(1)
			if !ok || equalPictureInterval != 0 && !r.skipUVLC() {
				return av1SequenceInfo{}, false
			}
			decoderModelInfoPresent, ok = r.read(1)
			if !ok {
				return av1SequenceInfo{}, false
			}
			if decoderModelInfoPresent != 0 {
				lengthMinusOne, ok := r.read(5)
				if !ok || !r.skip(32+5+5) {
					return av1SequenceInfo{}, false
				}
				bufferDelayLength = int(lengthMinusOne) + 1
			}
		}
		initialDisplayDelayPresent, ok = r.read(1)
		if !ok {
			return av1SequenceInfo{}, false
		}
		countMinusOne, ok := r.read(5)
		if !ok {
			return av1SequenceInfo{}, false
		}
		operatingPoints = countMinusOne + 1
	}
	for range operatingPoints {
		if reduced == 0 && !r.skip(12) {
			return av1SequenceInfo{}, false
		}
		level, ok := r.read(5)
		if !ok {
			return av1SequenceInfo{}, false
		}
		if level > 7 && !r.skip(1) {
			return av1SequenceInfo{}, false
		}
		if decoderModelInfoPresent != 0 {
			present, ok := r.read(1)
			if !ok {
				return av1SequenceInfo{}, false
			}
			if present != 0 && !r.skip(bufferDelayLength*2+1) {
				return av1SequenceInfo{}, false
			}
		}
		if initialDisplayDelayPresent != 0 {
			present, ok := r.read(1)
			if !ok {
				return av1SequenceInfo{}, false
			}
			if present != 0 && !r.skip(4) {
				return av1SequenceInfo{}, false
			}
		}
	}
	widthBitsMinusOne, ok := r.read(4)
	if !ok {
		return av1SequenceInfo{}, false
	}
	heightBitsMinusOne, ok := r.read(4)
	if !ok || !r.skip(int(widthBitsMinusOne)+1) || !r.skip(int(heightBitsMinusOne)+1) {
		return av1SequenceInfo{}, false
	}
	if reduced == 0 {
		frameIDs, ok := r.read(1)
		if !ok || frameIDs != 0 && !r.skip(7) {
			return av1SequenceInfo{}, false
		}
	}
	if !r.skip(3) {
		return av1SequenceInfo{}, false
	}
	if reduced == 0 {
		if !r.skip(4) {
			return av1SequenceInfo{}, false
		}
		enableOrderHint, ok := r.read(1)
		if !ok {
			return av1SequenceInfo{}, false
		}
		if enableOrderHint != 0 && !r.skip(2) {
			return av1SequenceInfo{}, false
		}
		chooseScreenContent, ok := r.read(1)
		if !ok {
			return av1SequenceInfo{}, false
		}
		forceScreenContent := uint64(2)
		if chooseScreenContent == 0 {
			forceScreenContent, ok = r.read(1)
			if !ok {
				return av1SequenceInfo{}, false
			}
		}
		if forceScreenContent > 0 {
			chooseIntegerMV, ok := r.read(1)
			if !ok {
				return av1SequenceInfo{}, false
			}
			if chooseIntegerMV == 0 && !r.skip(1) {
				return av1SequenceInfo{}, false
			}
		}
		if enableOrderHint != 0 && !r.skip(3) {
			return av1SequenceInfo{}, false
		}
	}
	if !r.skip(3) {
		return av1SequenceInfo{}, false
	}
	info, ok := parseAV1ColorConfig(&r, int(seqProfile))
	if ok {
		info.hasTiming = hasTiming
	}
	return info, ok
}

func parseAV1ColorConfig(r *av1BitReader, seqProfile int) (av1SequenceInfo, bool) {
	highBitDepth, ok := r.read(1)
	if !ok {
		return av1SequenceInfo{}, false
	}
	bitDepth := 8
	if seqProfile == 2 && highBitDepth != 0 {
		twelveBit, ok := r.read(1)
		if !ok {
			return av1SequenceInfo{}, false
		}
		if twelveBit != 0 {
			bitDepth = 12
		} else {
			bitDepth = 10
		}
	} else if highBitDepth != 0 {
		bitDepth = 10
	}
	monoChrome := uint64(0)
	if seqProfile != 1 {
		monoChrome, ok = r.read(1)
		if !ok {
			return av1SequenceInfo{}, false
		}
	}
	descriptionPresent, ok := r.read(1)
	if !ok {
		return av1SequenceInfo{}, false
	}
	primariesCode, transferCode, matrixCode := uint64(2), uint64(2), uint64(2)
	if descriptionPresent != 0 {
		primariesCode, ok = r.read(8)
		if !ok {
			return av1SequenceInfo{}, false
		}
		transferCode, ok = r.read(8)
		if !ok {
			return av1SequenceInfo{}, false
		}
		matrixCode, ok = r.read(8)
		if !ok {
			return av1SequenceInfo{}, false
		}
	}
	info := av1SequenceInfo{descriptionPresent: descriptionPresent != 0}
	if info.descriptionPresent {
		info.colorPrimaries = matroskaColorPrimariesName(primariesCode)
		info.transferCharacteristics = matroskaTransferName(transferCode)
		info.matrixCoefficients = matroskaMatrixName(matrixCode)
	}
	if monoChrome != 0 {
		colorRange, ok := r.read(1)
		if !ok || !r.skip(1) {
			return av1SequenceInfo{}, false
		}
		if colorRange != 0 {
			info.colorRange = "Full"
		} else {
			info.colorRange = "Limited"
		}
		return info, true
	}
	if primariesCode == 1 && transferCode == 13 && matrixCode == 0 {
		info.colorRange = "Full"
	} else {
		colorRange, ok := r.read(1)
		if !ok {
			return av1SequenceInfo{}, false
		}
		if colorRange != 0 {
			info.colorRange = "Full"
		} else {
			info.colorRange = "Limited"
		}
		subsamplingX, subsamplingY := uint64(0), uint64(0)
		switch seqProfile {
		case 0:
			subsamplingX, subsamplingY = 1, 1
		case 1:
		default:
			if bitDepth == 12 {
				subsamplingX, ok = r.read(1)
				if !ok {
					return av1SequenceInfo{}, false
				}
				if subsamplingX != 0 {
					subsamplingY, ok = r.read(1)
					if !ok {
						return av1SequenceInfo{}, false
					}
				}
			} else {
				subsamplingX, subsamplingY = 1, 0
			}
		}
		if subsamplingX != 0 && subsamplingY != 0 && !r.skip(2) {
			return av1SequenceInfo{}, false
		}
	}
	if !r.skip(1) {
		return av1SequenceInfo{}, false
	}
	return info, true
}

// applyMatroskaMPEG4VisualProbe transfers MPEG-4 Visual headers carried in a
// bounded Matroska frame sample into the track's canonical projections.
func applyMatroskaMPEG4VisualProbe(stream *Stream, parsed mpeg4VisualInfo) {
	if stream == nil {
		return
	}
	replaceCanonicalSeedFill(stream, "Format", "MPEG-4 Visual", "Format", "MPEG-4 Visual")
	if parsed.Profile != "" {
		profile, level, _ := strings.Cut(parsed.Profile, "@L")
		replaceCanonicalSeedFill(stream, "Format_Profile", profile, "Format profile", parsed.Profile)
		if level != "" {
			replaceCanonicalSeedFill(stream, "Format_Level", level, "", "")
		}
	}
	if parsed.BVOP != nil {
		value := "No"
		if *parsed.BVOP {
			value = "1"
		}
		replaceCanonicalSeedFill(stream, "Format_Settings_BVOP", value, "Format settings, BVOP", value)
	}
	if parsed.QPel != nil {
		value := formatYesNo(*parsed.QPel)
		replaceCanonicalSeedFill(stream, "Format_Settings_QPel", value, "Format settings, QPel", value)
	}
	if parsed.GMC != "" {
		replaceCanonicalSeedFill(stream, "Format_Settings_GMC", strings.TrimSuffix(parsed.GMC, " warppoints"), "Format settings, GMC", parsed.GMC)
	}
	if parsed.Matrix != "" {
		replaceCanonicalSeedFill(stream, "Format_Settings_Matrix", parsed.Matrix, "Format settings, Matrix", parsed.Matrix)
	}
	for field, value := range map[string]string{
		"Color space": parsed.ColorSpace, "Chroma subsampling": parsed.ChromaSubsampling,
		"Bit depth": parsed.BitDepth, "Scan type": parsed.ScanType, "Scan order": parsed.ScanOrder,
	} {
		if value != "" {
			key := map[string]fieldName{"Color space": "ColorSpace", "Chroma subsampling": "ChromaSubsampling", "Bit depth": "BitDepth", "Scan type": "ScanType", "Scan order": "ScanOrder"}[field]
			raw := value
			if field == "Bit depth" {
				raw = extractLeadingNumber(value)
			}
			replaceCanonicalSeedFill(stream, key, raw, field, value)
		}
	}
	replaceCanonicalSeedFill(stream, "Compression_Mode", "Lossy", "Compression mode", "Lossy")
	if parsed.WritingLibrary != "" {
		replaceCanonicalSeedFill(stream, "Encoded_Library", parsed.WritingLibrary, "Writing library", parsed.WritingLibrary)
		if version, date, ok := xvidLibraryVersionDate(parsed.WritingLibrary); ok {
			replaceCanonicalSeedFill(stream, "Encoded_Library_Name", "XviD", "", "")
			replaceCanonicalSeedFill(stream, "Encoded_Library_Version", version, "", "")
			if date != "" {
				replaceCanonicalSeedFill(stream, "Encoded_Library_Date", date, "", "")
			}
		}
	}
}

// standardMatroskaH264GOPLength reports whether n is a conventional fixed GOP
// length that MediaInfo exposes for sampled Matroska AVC streams.
func standardMatroskaH264GOPLength(n int) bool {
	switch n {
	case 12, 15, 24, 25, 30, 48, 50, 60:
		return true
	default:
		return false
	}
}

// matroskaH264GOPNeedsExplicitRate reports whether an inferred GOP length adds
// information beyond an exact one-second GOP at the displayed frame rate.
func matroskaH264GOPNeedsExplicitRate(stream Stream, n int) bool {
	fps, ok := parseFPS(matroskaStreamDisplay(stream, "Frame rate"))
	return !ok || fps <= 0 || math.Abs(fps-float64(n)) >= 0.001
}

// h264LengthPrefixedToAnnexB converts length-prefixed NAL units to Annex B for
// bounded metadata probing. It retains at most 256 bytes from each declared NAL
// and returns converted complete prefixes before a malformed length.
func h264LengthPrefixedToAnnexB(payload []byte, lengthSize int) []byte {
	if lengthSize < 1 || lengthSize > 4 {
		return nil
	}
	var out []byte
	for pos := 0; pos+lengthSize <= len(payload); {
		size := 0
		for i := range lengthSize {
			size = size<<8 | int(payload[pos+i])
		}
		pos += lengthSize
		if size <= 0 || size > len(payload)-pos {
			return out
		}
		out = append(out, 0, 0, 0, 1)
		prefixSize := min(size, 256)
		out = append(out, payload[pos:pos+prefixSize]...)
		pos += size
	}
	return out
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

var dtsSamplingRates = [...]int{
	0, 8000, 16000, 32000, 0, 0, 11025, 22050,
	44100, 0, 0, 12000, 24000, 48000, 96000, 192000,
}

var dtsBitRates = [...]int64{
	32000, 56000, 64000, 96000, 112000, 128000, 192000, 224000,
	256000, 320000, 384000, 448000, 512000, 576000, 640000, 754500,
	960000, 1024000, 1152000, 1280000, 1344000, 1408000, 1411200, 1472000,
	1509750, 1920000, 2048000, 3072000, 3840000, 0, 0, 0,
}

// dtsResolutions maps the two-bit DTS source PCM resolution code to bit depth.
var dtsResolutions = [...]int{16, 20, 24, 24}
var dtsChannelCounts = [...]int{
	// MediaInfoLib mapping (DTS_Channels in File_Dts.cpp), without LFE. LFE is added separately.
	1, 2, 2, 2, 2, 3, 3, 4,
	4, 5, 6, 6, 6, 7, 8, 8,
}

// normalizeDTSHDChannelLayout uses back-channel labels when a DTS-HD bed
// contains distinct side-surround and rear-surround pairs.
func normalizeDTSHDChannelLayout(layout string) string {
	if strings.Contains(layout, "Lss Rss") {
		layout = strings.ReplaceAll(layout, "Lsr", "Lb")
		layout = strings.ReplaceAll(layout, "Rsr", "Rb")
	}
	return layout
}

// parseDTSCoreFrame parses a big-endian DTS core header. When the complete
// primary frame is buffered, its exact size determines bitrate; otherwise the
// nominal transmission-rate code is used. Invalid or unsupported headers report false.
func parseDTSCoreFrame(payload []byte) (dtsInfo, bool) {
	if len(payload) < 12 {
		return dtsInfo{}, false
	}
	// Core sync word (big-endian): 0x7FFE8001
	if payload[0] != 0x7F || payload[1] != 0xFE || payload[2] != 0x80 || payload[3] != 0x01 {
		return dtsInfo{}, false
	}

	br := newBitReader(payload[4:])
	_ = br.readBitsValue(1) // FrameType
	_ = br.readBitsValue(5) // Deficit Sample Count
	crcPresent := br.readBitsValue(1) == 1
	nblks := int(br.readBitsValue(7)) + 1       // Number of PCM sample blocks
	frameSize := int(br.readBitsValue(14)) + 1  // Primary frame byte size
	amode := int(br.readBitsValue(6))           // Audio channel arrangement
	sfCode := int(br.readBitsValue(4))          // Core audio sampling frequency
	brCode := int(br.readBitsValue(5))          // Transmission bit rate
	_ = br.readBitsValue(1)                     // Embedded Down Mix Enabled
	_ = br.readBitsValue(1)                     // Embedded Dynamic Range
	_ = br.readBitsValue(1)                     // Embedded Time Stamp
	_ = br.readBitsValue(1)                     // Auxiliary Data
	_ = br.readBitsValue(1)                     // HDCD
	extAudioID := int(br.readBitsValue(3))      // Extension Audio Descriptor
	extAudioPresent := br.readBitsValue(1) == 1 // Extended Coding
	_ = br.readBitsValue(1)                     // Audio Sync Word Insertion
	lfe := br.readBitsValue(2)                  // Low Frequency Effects
	_ = br.readBitsValue(1)                     // Predictor History
	if crcPresent {
		_ = br.readBitsValue(16) // Header CRC Check
	}
	_ = br.readBitsValue(1) // Multirate Interpolator
	_ = br.readBitsValue(4) // Encoder Software Revision
	_ = br.readBitsValue(2) // Copy History
	pcmResCode := int(br.readBitsValue(2))
	coreES := br.readBitsValue(1) == 1 // ES matrix flag

	sampleRate := 0
	if sfCode >= 0 && sfCode < len(dtsSamplingRates) {
		sampleRate = dtsSamplingRates[sfCode]
	}
	bitRate := int64(0)
	if brCode >= 0 && brCode < len(dtsBitRates) {
		bitRate = dtsBitRates[brCode]
	}
	bitDepth := 0
	if pcmResCode >= 0 && pcmResCode < len(dtsResolutions) {
		bitDepth = dtsResolutions[pcmResCode]
	}
	channels := 0
	if amode >= 0 && amode < len(dtsChannelCounts) {
		channels = dtsChannelCounts[amode]
	}
	if lfe > 0 {
		channels++
	}
	if sampleRate <= 0 || bitRate <= 0 || bitDepth <= 0 || nblks <= 0 || channels <= 0 {
		return dtsInfo{}, false
	}
	spf := nblks * 32
	if frameSize > 0 && frameSize <= len(payload) && sampleRate > 0 && spf > 0 {
		// Core frames may use padding, so the actual frame size is more precise
		// than the nominal transmission-rate code (e.g. 318000 vs 320000 b/s).
		bitRate = int64(math.Round(float64(frameSize*8*sampleRate) / float64(spf)))
	}
	coreXCh := extAudioPresent && extAudioID == 0 && bytes.Contains(payload, []byte{0x5A, 0x5A, 0x5A, 0x5A})
	return dtsInfo{
		bitRateBps:      bitRate,
		bitDepth:        bitDepth,
		sampleRate:      sampleRate,
		samplesPerFrame: spf,
		channels:        channels,
		coreES:          coreES,
		coreXCh:         coreXCh,
		coreAudioMode:   amode,
	}, true
}

// matroskaStatsDuration returns the positive duration selected from observed
// track time bounds, or zero when no usable duration exists.
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

// streamTrackNumber returns the canonical Matroska TrackNumber.
func streamTrackNumber(stream Stream) uint64 {
	id, _ := canonicalSeedValue(stream, "ID")
	if id == "" {
		return 0
	}
	value, _ := strconv.ParseUint(id, 10, 64)
	return value
}

// streamTrackUID returns the canonical Matroska TrackUID.
func streamTrackUID(stream Stream) uint64 {
	value, _ := canonicalSeedValue(stream, "UniqueID")
	if value == "" {
		return 0
	}
	parsed, _ := strconv.ParseUint(value, 10, 64)
	return parsed
}
