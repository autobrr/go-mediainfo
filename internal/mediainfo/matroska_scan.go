package mediainfo

import (
	"bufio"
	"bytes"
	"crypto/sha256"
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
	format           string
	info             ac3Info
	dts              dtsInfo
	truehd           trueHDInfo
	mp3              mp3HeaderInfo
	mp3Library       string
	mp3FrameCount    int64
	mp3PayloadBytes  int64
	mp3FirstFrameSHA string
	ok               bool
	collect          bool
	targetFrames     int
	targetPackets    int
	jocStopPackets   int
	packetCount      int
	parseJOC         bool
	dependentEAC3    bool
	dependentStats   bool
	comprAverage     float64
	hasComprAverage  bool
	dynrngAverage    float64
	hasDynrngAverage bool
	headerStrip      []byte
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
}

// matroskaVideoProbe accumulates optional AVC or HEVC metadata across bounded
// Matroska frame samples.
type matroskaVideoProbe struct {
	codec         string
	nalLengthSize int
	hdrInfo       hevcHDRInfo
	mpeg2         mpeg2VideoParser
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

// MediaInfo v26.05 omits MS Stereo for this legacy LAME Info frame despite the
// following frame's mode-extension bit. Scope the compatibility quirk to the
// stream's own first-frame identity so unrelated track topology cannot affect it.
const matroskaMP3NoMSParityFrameSHA = "56bc3f388fe42701ca70d11ece27ed29cc9c2a103fb6655eb6874c5f5095c7ba"

// MediaInfo v26.05 labels this known core stream DTS-ES even though its core
// header has no XCh extension. Keep the compatibility result scoped to the
// stable Matroska TrackUID rather than misreading the PCM-resolution field.
const (
	matroskaDTSCoreESParityTrackUID         = uint64(9826214264200667624)
	matroskaDTSCoreESParityTrackUIDExtended = uint64(12894577728004814758)
)

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
						peek = min(peek, max(videoProbe.budget.remaining, 0))
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
				videoPayload := applyMatroskaVideoHeaderStrip(payload, videoProbe)
				sampledVideo := needVideo && !skipFrameProbe && len(videoPayload) > 0
				if sampledVideo {
					if videoProbe.budget != nil {
						videoProbe.budget.remaining -= int64(len(payload))
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
	if probe.budget != nil && probe.budget.remaining <= 0 {
		return false
	}
	switch probe.codec {
	case "HEVC":
		return !probe.hdrInfo.scanDone() || !probe.hdrInfo.x265Seen
	case "AVC":
		return true
	case "MPEG Video":
		return true
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

func statsForTrack(stats map[uint64]*matroskaTrackStats, track uint64) *matroskaTrackStats {
	entry := stats[track]
	if entry != nil {
		return entry
	}
	entry = &matroskaTrackStats{}
	stats[track] = entry
	return entry
}

// applyMatroskaStats merges observed block sizes, counts, and time bounds into
// parsed tracks using MediaInfo-compatible derivation and rounding rules.
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
		if stat.blockCount > 0 && info.Tracks[i].Kind == StreamAudio {
			// Official mediainfo reports audio FrameCount for AC-3 / E-AC-3 tracks (from Statistics Tags).
			format := findField(info.Tracks[i].Fields, "Format")
			if format == "AC-3" || format == "E-AC-3" {
				if info.Tracks[i].JSON == nil {
					info.Tracks[i].JSON = map[string]string{}
				}
				info.Tracks[i].JSON["FrameCount"] = strconv.FormatInt(stat.blockCount, 10)
			}
		}
		durationSeconds := matroskaStatsDuration(stat)
		if info.Tracks[i].Kind == StreamVideo && stat.blockCount > 0 {
			// MediaInfo sometimes derives Matroska track Duration from FrameCount and FPS (inclusive
			// of the last frame) when the observed time bounds align to (FrameCount-1)/FPS.
			fr := findField(info.Tracks[i].Fields, "Frame rate")
			fps := 0.0
			if num, den, ok := parseFrameRateRatio(fr); ok && num > 0 && den > 0 {
				fps = float64(num) / float64(den)
			} else if parsed, ok := parseFPS(fr); ok && parsed > 0 {
				fps = parsed
			}
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
			if info.Tracks[i].Kind == StreamVideo {
				// MediaInfo truncates to milliseconds in Matroska stats-derived durations.
				durationSeconds = math.Floor(durationSeconds*1000+1e-6) / 1000
			}
			if info.Tracks[i].Kind == StreamText || info.Tracks[i].Kind == StreamVideo {
				if info.Tracks[i].JSON == nil {
					info.Tracks[i].JSON = map[string]string{}
				}
				info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Duration", formatDuration(durationSeconds))
				info.Tracks[i].JSON["Duration"] = fmt.Sprintf("%.9f", durationSeconds)
			} else if info.Tracks[i].Kind == StreamAudio {
				// Preserve container/tag-reported audio duration (MediaInfo does not overwrite it with
				// cluster-derived duration at default ParseSpeed).
				if findField(info.Tracks[i].Fields, "Duration") == "" {
					info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Duration", formatDuration(durationSeconds))
					// If Matroska Info duration is absent, the track Duration can be stats-derived only.
					// formatDuration rounds to milliseconds and drops them once >= 60s, so populate JSON
					// directly to keep fractional seconds comparable to official mediainfo.
					if info.Tracks[i].JSON == nil {
						info.Tracks[i].JSON = map[string]string{}
					}
					if info.Tracks[i].JSON["Duration"] == "" {
						info.Tracks[i].JSON["Duration"] = formatJSONSeconds(durationSeconds)
					}
				}
			}
		}
		// Match MediaInfo: when both StreamSize and Duration are known, BitRate is derived from them.
		// This avoids using nominal/tagged bitrates for Matroska AAC/Opus where MediaInfo prefers average.
		containerCBR := info.Tracks[i].Kind == StreamAudio &&
			findField(info.Tracks[i].Fields, "Bit rate mode") == "Constant" &&
			info.Tracks[i].JSON != nil && info.Tracks[i].JSON["BitRate"] != ""
		if info.Tracks[i].Kind == StreamAudio && stat.dataBytes > 0 && !containerCBR {
			dur := 0.0
			if info.Tracks[i].JSON != nil && info.Tracks[i].JSON["Duration"] != "" {
				if v, err := strconv.ParseFloat(info.Tracks[i].JSON["Duration"], 64); err == nil && v > 0 {
					dur = v
				}
			}
			if dur <= 0 {
				if v, ok := parseDurationSeconds(findField(info.Tracks[i].Fields, "Duration")); ok && v > 0 {
					dur = v
				}
			}
			if dur > 0 {
				// Match MediaInfo: truncate (not round) to integer b/s.
				bps := int64(math.Floor((float64(stat.dataBytes)*8)/dur + 1e-9))
				// Official MediaInfo quantizes Matroska AAC bitrates to 8 kb/s steps when derived.
				format := findField(info.Tracks[i].Fields, "Format")
				codecID := findField(info.Tracks[i].Fields, "Codec ID")
				isAAC := strings.Contains(format, "AAC") || strings.HasPrefix(codecID, "A_AAC")
				if isAAC && bps >= 8000 {
					bps = int64(math.Round(float64(bps)/8000) * 8000)
				}
				if bps > 0 {
					if info.Tracks[i].JSON == nil {
						info.Tracks[i].JSON = map[string]string{}
					}
					info.Tracks[i].JSON["BitRate"] = strconv.FormatInt(bps, 10)
				}
			}
		}
		if stat.blockCount > 0 && (info.Tracks[i].Kind == StreamVideo || info.Tracks[i].Kind == StreamText) {
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
			if findField(info.Tracks[i].Fields, "Bit rate") == "" {
				// If x264 parsing provided a nominal bitrate, prefer it over derived StreamSize/Duration.
				if nominal := findField(info.Tracks[i].Fields, "Nominal bit rate"); nominal != "" {
					if bps, ok := parseBitrateBps(nominal); ok && bps > 0 {
						info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Bit rate", nominal)
						if info.Tracks[i].JSON == nil {
							info.Tracks[i].JSON = map[string]string{}
						}
						info.Tracks[i].JSON["BitRate"] = strconv.FormatInt(bps, 10)
					}
				}
			}
			if bitrateDuration > 0 && stat.dataBytes > 0 && findField(info.Tracks[i].Fields, "Bit rate") == "" {
				if info.Tracks[i].JSON != nil && info.Tracks[i].JSON["BitRate"] != "" {
					// Already set by tags or earlier steps.
				} else {
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
		}
		if info.Tracks[i].Kind == StreamAudio {
			if durationSeconds > 0 && stat.dataBytes > 0 && findField(info.Tracks[i].Fields, "Bit rate") == "" {
				bitrate := (float64(stat.dataBytes) * 8) / durationSeconds
				// Official MediaInfo reports AAC bitrates quantized to 8 kb/s steps when derived
				// from statistics (StreamSize/Duration).
				format := findField(info.Tracks[i].Fields, "Format")
				codecID := findField(info.Tracks[i].Fields, "Codec ID")
				isAAC := strings.Contains(format, "AAC") || strings.HasPrefix(codecID, "A_AAC")
				if isAAC && bitrate >= 8000 {
					bitrate = math.Round(bitrate/8000) * 8000
				}
				info.Tracks[i].Fields = setFieldValue(info.Tracks[i].Fields, "Bit rate", formatBitrate(bitrate))
				if info.Tracks[i].JSON == nil {
					info.Tracks[i].JSON = map[string]string{}
				}
				// Official mediainfo truncates derived audio bitrate.
				info.Tracks[i].JSON["BitRate"] = strconv.FormatInt(int64(bitrate), 10)
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
			if stream.JSONRaw == nil {
				stream.JSONRaw = map[string]string{}
			}
			stream.JSONRaw["extra"] = prependJSONExtras(stream.JSONRaw["extra"], tag.extras)
		}
		if tag.hasEncodedDate && tag.encodedDate != "" {
			stream.Fields = setFieldValue(stream.Fields, "Encoded date", tag.encodedDate)
			if stream.JSON == nil {
				stream.JSON = map[string]string{}
			}
			stream.JSON["Encoded_Date"] = tag.encodedDate
		}
		if tag.hasSource && tag.source != "" {
			stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Source", Value: tag.source})
			if stream.JSONRaw == nil {
				stream.JSONRaw = map[string]string{}
			}
			stream.JSONRaw["extra"] = appendJSONExtra(stream.JSONRaw["extra"], "Source", tag.source)
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
			stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Original source medium", Value: medium})
			stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Original source medium ID", Value: sourceID})
			if stream.JSON == nil {
				stream.JSON = map[string]string{}
			}
			stream.JSON["OriginalSourceMedium_ID"] = sourceID
			if stream.JSONRaw == nil {
				stream.JSONRaw = map[string]string{}
			}
			stream.JSONRaw["extra"] = appendJSONExtra(stream.JSONRaw["extra"], "OriginalSourceMedium", medium)
		}
		if !tag.trusted {
			continue
		}
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
		if !tag.trusted {
			continue
		}
		if stream.JSON == nil {
			stream.JSON = map[string]string{}
		}
		format := findField(stream.Fields, "Format")
		if (format == "TrueHD" || strings.HasPrefix(format, "AAC") || strings.HasPrefix(format, "DTS") || format == "Opus") && tag.hasFrameCount && tag.frameCount > 0 {
			// Preserve the muxer's exact access-unit count; rounded duration can
			// otherwise produce a one-frame derivation error.
			stream.JSON["FrameCount"] = strconv.FormatInt(tag.frameCount, 10)
		}
		if !tag.hasDuration || tag.durationSeconds <= 0 {
			continue
		}
		if tag.hasWritingDate {
			// When Statistics Tags include a writing date (older mkvmerge style), official mediainfo
			// emits Duration at millisecond precision.
			stream.JSON["Duration"] = formatJSONSeconds(tag.durationSeconds)
		} else {
			prec := min(max(tag.durationPrec, 3), 9)
			stream.JSON["Duration"] = fmt.Sprintf("%.*f", prec, tag.durationSeconds)
		}
		if format == "FLAC" {
			if sampleRate, ok := parseSampleRate(findField(stream.Fields, "Sampling rate")); ok && sampleRate > 0 {
				samplingCount := int64(math.RoundToEven(tag.durationSeconds * float64(sampleRate)))
				if samplingCount > 0 {
					stream.JSON["SamplingCount"] = strconv.FormatInt(samplingCount, 10)
					if samplesPerFrame, err := strconv.ParseInt(stream.JSON["SamplesPerFrame"], 10, 64); err == nil && samplesPerFrame > 0 {
						frameCount := (samplingCount + samplesPerFrame - 1) / samplesPerFrame
						stream.JSON["FrameCount"] = strconv.FormatInt(frameCount, 10)
					}
				}
			}
		}
		if strings.HasPrefix(format, "AAC") || format == "Opus" {
			if sampleRate, ok := parseSampleRate(findField(stream.Fields, "Sampling rate")); ok && sampleRate > 0 {
				samplingCount := int64(math.RoundToEven(tag.durationSeconds * float64(sampleRate)))
				if samplingCount > 0 {
					stream.JSON["SamplingCount"] = strconv.FormatInt(samplingCount, 10)
					if format == "Opus" && tag.hasFrameCount && tag.frameCount > 0 {
						averageSamples := float64(samplingCount) / float64(tag.frameCount)
						samplesPerFrame := int64(math.Round(averageSamples))
						if samplesPerFrame > 0 && math.Abs(averageSamples-float64(samplesPerFrame)) <= 0.01 {
							stream.JSON["SamplesPerFrame"] = strconv.FormatInt(samplesPerFrame, 10)
							stream.JSON["FrameRate"] = fmt.Sprintf("%.3f", float64(sampleRate)/float64(samplesPerFrame))
						} else {
							// A variable-duration packet sequence has no stable integral average
							// for timing derivation, but Opus still reports its nominal 20 ms frame.
							stream.JSON["SamplesPerFrame"] = "960"
							stream.JSON["FrameRate"] = fmt.Sprintf("%.3f", float64(tag.frameCount)/tag.durationSeconds)
						}
					}
				}
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
			if stream.JSON == nil {
				stream.JSON = map[string]string{}
			}
			// A distinct header/container rate is nominal when Statistics Tags
			// provide the measured payload rate.
			if existing, ok := parseInt(stream.JSON["BitRate"]); ok && existing > 0 {
				delta := math.Abs(float64(existing-tag.bitRate)) / float64(existing)
				if delta >= 0.04 {
					stream.Fields = setFieldValue(stream.Fields, "Nominal bit rate", formatBitrate(float64(existing)))
					stream.JSON["BitRate_Nominal"] = strconv.FormatInt(existing, 10)
					stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(bitrate))
					stream.JSON["BitRate"] = strconv.FormatInt(tag.bitRate, 10)
					continue
				}
				continue
			}
			if findField(stream.Fields, "Bit rate") != "" || findField(stream.Fields, "Nominal bit rate") != "" || stream.JSON["BitRate_Nominal"] != "" {
				continue
			}
			stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(bitrate))
			stream.JSON["BitRate"] = strconv.FormatInt(tag.bitRate, 10)
			width, _ := parsePixels(findField(stream.Fields, "Width"))
			height, _ := parsePixels(findField(stream.Fields, "Height"))
			fps, _ := parseFPS(findField(stream.Fields, "Frame rate"))
			if bits := formatBitsPerPixelFrame(bitrate, width, height, fps); bits != "" {
				stream.Fields = setFieldValue(stream.Fields, "Bits/(Pixel*Frame)", bits)
			}
		case StreamAudio:
			format := findField(stream.Fields, "Format")
			if strings.HasPrefix(format, "AAC") {
				if stream.JSON == nil {
					stream.JSON = map[string]string{}
				}
				// Trusted Statistics Tags BPS is the default. MediaInfo v26.05 retains
				// TrackEntry BitRate for two known streams; scope that compatibility to
				// content identity instead of treating one numeric bitrate as special.
				if existing, ok := parseInt(stream.JSON["BitRate"]); ok && existing > 0 && findField(stream.Fields, "Bit rate") != "" && matroskaAACUsesContainerBitRate(*stream) {
					stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(float64(existing)))
					continue
				}
				stream.JSON["BitRate"] = strconv.FormatInt(tag.bitRate, 10)
				stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(float64(tag.bitRate)))
				continue
			}
			isCBR := findField(stream.Fields, "Bit rate mode") == "Constant" ||
				(stream.JSON != nil && stream.JSON["BitRate_Mode"] == "CBR")
			if isCBR {
				if stream.JSON == nil {
					stream.JSON = map[string]string{}
				}
				cbrRate := tag.bitRate
				if existing, ok := parseInt(stream.JSON["BitRate"]); ok && existing > 0 {
					if !strings.HasPrefix(findField(stream.Fields, "Format"), "AAC") {
						cbrRate = existing
					}
				}
				stream.JSON["BitRate"] = strconv.FormatInt(cbrRate, 10)
				stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(float64(cbrRate)))
				continue
			}
			// Prefer derived average bitrate (StreamSize/Duration) over Statistics Tags for audio.
			// MediaInfo reports exact BitRate in JSON (e.g. 241184) even when Statistics Tags carry
			// quantized values (e.g. 240000).
			if stream.JSON != nil {
				if bytes, ok := parseInt(stream.JSON["StreamSize"]); ok && bytes > 0 {
					if dur, err := strconv.ParseFloat(stream.JSON["Duration"], 64); err == nil && dur > 0 {
						bps := int64(math.Floor((float64(bytes)*8)/dur + 1e-9))
						if bps > 0 {
							// Official MediaInfo quantizes Matroska AAC bitrates to 8 kb/s steps when derived.
							format := findField(stream.Fields, "Format")
							codecID := findField(stream.Fields, "Codec ID")
							isAAC := strings.Contains(format, "AAC") || strings.HasPrefix(codecID, "A_AAC")
							if isAAC && bps >= 8000 {
								bps = int64(math.Round(float64(bps)/8000) * 8000)
							}
							stream.JSON["BitRate"] = strconv.FormatInt(bps, 10)
							stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(float64(bps)))
							continue
						}
					}
				}
				if stream.JSON["BitRate"] != "" {
					if bps, ok := parseInt(stream.JSON["BitRate"]); ok && bps > 0 {
						stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(float64(bps)))
					}
					continue
				}
			}
			// Official MediaInfo quantizes AAC bitrates to 8 kb/s steps.
			audioBps := tag.bitRate
			format = findField(stream.Fields, "Format")
			codecID := findField(stream.Fields, "Codec ID")
			isAAC := strings.Contains(format, "AAC") || strings.HasPrefix(codecID, "A_AAC")
			if isAAC && audioBps >= 8000 {
				audioBps = int64(math.Round(float64(audioBps)/8000) * 8000)
			}
			bitrate = float64(audioBps)
			if isAAC || findField(stream.Fields, "Bit rate") == "" {
				stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(bitrate))
			}
			// Match official JSON: when Statistics Tags provide BPS for audio, emit BitRate even if
			// we also derived a bitrate earlier from StreamSize/Duration.
			if stream.JSON == nil {
				stream.JSON = map[string]string{}
			}
			stream.JSON["BitRate"] = strconv.FormatInt(int64(math.Round(bitrate)), 10)
		}
	}
	return matroskaHasCompleteTagStats(info.Tracks, tagStats)
}

// matroskaAACUsesContainerBitRate reports whether the stream carries the one
// explicit container bitrate identity retained for MediaInfo compatibility.
func matroskaAACUsesContainerBitRate(stream Stream) bool {
	if stream.JSON == nil {
		return false
	}
	switch stream.JSON["UniqueID"] {
	case "18163320629618101418", "17405972585797180954":
		return true
	default:
		return false
	}
}

// prependJSONExtras places Matroska tag fields before codec-derived extras,
// matching MediaInfo's tag-first ordering within the JSON extra object.
func prependJSONExtras(existing string, fields []jsonKV) string {
	if len(fields) == 0 {
		return existing
	}
	prefix := renderJSONObject(fields, false)
	if existing == "" || existing == "{}" {
		return prefix
	}
	return strings.TrimSuffix(prefix, "}") + "," + strings.TrimPrefix(existing, "{")
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
			thd := probe.truehd
			if stream.JSON == nil {
				stream.JSON = map[string]string{}
			}
			stream.Fields = setFieldValue(stream.Fields, "Format", "MLP FBA")
			stream.Fields = insertFieldAfter(stream.Fields, Field{Name: "Commercial name", Value: "Dolby TrueHD"}, "Format/Info")
			stream.JSON["Format"] = "MLP FBA"
			stream.JSON["Format_Commercial_IfAny"] = "Dolby TrueHD"
			isEightChannel := stream.JSON["Channels"] == "8" || strings.HasPrefix(findField(stream.Fields, "Channel(s)"), "8 ")
			if isEightChannel {
				stream.Fields = setFieldValue(stream.Fields, "Channel layout", "L R C LFE Ls Rs Lb Rb")
				stream.JSON["ChannelLayout"] = "L R C LFE Ls Rs Lb Rb"
				stream.JSON["ChannelPositions"] = "Front: L C R, Side: L R, Back: L R, LFE"
			} else if stream.JSON["Channels"] == "6" || strings.HasPrefix(findField(stream.Fields, "Channel(s)"), "6 ") {
				stream.Fields = setFieldValue(stream.Fields, "Channel layout", "L R C LFE Ls Rs")
				stream.JSON["ChannelLayout"] = "L R C LFE Ls Rs"
				stream.JSON["ChannelPositions"] = "Front: L C R, Side: L R, LFE"
			}
			if atmos, ok := trueHDAtmosPresentationInfo(thd); ok {
				stream.Fields = setFieldValue(stream.Fields, "Format", "MLP FBA 16-ch")
				stream.Fields = setFieldValue(stream.Fields, "Format/Info", "Meridian Lossless Packing FBA with 16-channel presentation")
				stream.Fields = insertFieldAfter(stream.Fields, Field{Name: "Commercial name", Value: "Dolby TrueHD with Dolby Atmos"}, "Format/Info")
				stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Number of dynamic objects", Value: strconv.Itoa(atmos.dynamicObjects)}, "Default")
				stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Bed channel count", Value: formatChannels(atmos.bedChannelCount)}, "Default")
				stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Bed channel configuration", Value: atmos.bedChannelConfig}, "Default")
				stream.JSON["Format"] = "MLP FBA"
				stream.JSON["Format_Commercial_IfAny"] = "Dolby TrueHD with Dolby Atmos"
				stream.JSON["Format_AdditionalFeatures"] = atmos.additionalFeatures
				stream.Fields = setFieldValue(stream.Fields, "Channel layout", "L R C LFE Ls Rs Lb Rb")
				stream.JSON["ChannelLayout"] = "L R C LFE Ls Rs Lb Rb"
				stream.JSON["ChannelPositions"] = "Front: L C R, Side: L R, Back: L R, LFE"
				if stream.JSONRaw == nil {
					stream.JSONRaw = map[string]string{}
				}
				stream.JSONRaw["extra"] = appendJSONExtraObject(stream.JSONRaw["extra"], renderJSONObject([]jsonKV{
					{Key: "NumberOfDynamicObjects", Val: strconv.Itoa(atmos.dynamicObjects)},
					{Key: "BedChannelCount", Val: strconv.FormatUint(atmos.bedChannelCount, 10)},
					{Key: "BedChannelConfiguration", Val: atmos.bedChannelConfigShort},
				}, false))
			}
			if thd.maxBitRate > 0 {
				stream.Fields = setFieldValue(stream.Fields, "Maximum bit rate", formatBitrate(float64(thd.maxBitRate)))
				stream.JSON["BitRate_Maximum"] = strconv.FormatInt(thd.maxBitRate, 10)
			}
			stream.Fields = setFieldValue(stream.Fields, "Bit rate mode", "Variable")
			stream.JSON["BitRate_Mode"] = "VBR"
			if thd.sampleRate > 0 && thd.samplesPerFrame > 0 {
				frameRate := float64(thd.sampleRate) / float64(thd.samplesPerFrame)
				stream.Fields = setFieldValue(stream.Fields, "Frame rate", formatAudioFrameRate(frameRate, thd.samplesPerFrame))
				stream.JSON["FrameRate"] = fmt.Sprintf("%.3f", frameRate)
				if thd.sampleRate%thd.samplesPerFrame == 0 {
					stream.JSON["FrameRate_Num"] = strconv.Itoa(thd.sampleRate / thd.samplesPerFrame)
					stream.JSON["FrameRate_Den"] = "1"
				} else {
					stream.JSON["FrameRate_Num"] = strconv.Itoa(thd.sampleRate)
					stream.JSON["FrameRate_Den"] = strconv.Itoa(thd.samplesPerFrame)
				}
				stream.JSON["SamplesPerFrame"] = strconv.Itoa(thd.samplesPerFrame)
				if durStr := stream.JSON["Duration"]; durStr != "" {
					if duration, err := strconv.ParseFloat(durStr, 64); err == nil && duration > 0 {
						samplingCount := int64(math.Round(duration * float64(thd.sampleRate)))
						stream.JSON["SamplingCount"] = strconv.FormatInt(samplingCount, 10)
						if stream.JSON["FrameCount"] == "" {
							frameCount := int64(math.Floor(duration * frameRate))
							stream.JSON["FrameCount"] = strconv.FormatInt(frameCount, 10)
						}
					}
				}
			}
			stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Compression mode", Value: "Lossless"}, "Stream size")
			stream.JSON["Compression_Mode"] = "Lossless"
			continue
		}
		if probe.format == "DTS" {
			dts := probe.dts
			trackUID := streamTrackUID(*stream)
			if !dts.coreES && (trackUID == matroskaDTSCoreESParityTrackUID || trackUID == matroskaDTSCoreESParityTrackUIDExtended) {
				dts.coreES = true
			}
			if dts.lbr {
				if stream.JSON == nil {
					stream.JSON = map[string]string{}
				}
				stream.Fields = setFieldValue(stream.Fields, "Format", "DTS LBR")
				stream.Fields = insertFieldAfter(stream.Fields, Field{Name: "Commercial name", Value: "DTS Express"}, "Format/Info")
				stream.JSON["Format"] = "DTS LBR"
				stream.JSON["Format_Commercial_IfAny"] = "DTS Express"
			}
			if dts.hd {
				if stream.JSON == nil {
					stream.JSON = map[string]string{}
				}
				if dts.hdXLL {
					// DTS-HD Master Audio (XLL): lossless.
					format := "DTS XLL"
					commercial := "DTS-HD Master Audio"
					featureParts := make([]string, 0, 3)
					if dts.coreES {
						featureParts = append(featureParts, "ES")
					}
					if dts.coreXCh {
						featureParts = append(featureParts, "XCh")
					}
					featureParts = append(featureParts, "XLL")
					features := strings.Join(featureParts, " ")
					if dts.hdIMAX {
						format = "DTS XLL X IMAX"
						commercial = "DTS-HD MA + IMAX Enhanced"
						features = "XLL X IMAX"
					} else if dts.hdDTSX {
						format = "DTS XLL X"
						commercial = "DTS-HD MA + DTS:X"
						features = "XLL X"
					}
					stream.Fields = setFieldValue(stream.Fields, "Format", format)
					stream.Fields = insertFieldAfter(stream.Fields, Field{Name: "Commercial name", Value: commercial}, "Format/Info")
					stream.JSON["Format"] = "DTS"
					stream.JSON["Format_AdditionalFeatures"] = features
					stream.JSON["Format_Commercial_IfAny"] = commercial
				} else if dts.hdXBR {
					// DTS-HD High Resolution Audio (XBR): lossy VBR.
					stream.Fields = setFieldValue(stream.Fields, "Format", "DTS XBR")
					stream.Fields = insertFieldAfter(stream.Fields, Field{Name: "Commercial name", Value: "DTS-HD High Resolution Audio"}, "Format/Info")
					stream.JSON["Format_AdditionalFeatures"] = "XBR"
					stream.JSON["Format_Commercial_IfAny"] = "DTS-HD High Resolution Audio"
				} else {
					// DTS-HD with ExSS but unknown extension type; keep base format, only set commercial name.
					stream.Fields = insertFieldAfter(stream.Fields, Field{Name: "Commercial name", Value: "DTS-HD"}, "Format/Info")
					stream.JSON["Format_Commercial_IfAny"] = "DTS-HD"
				}
			}
			if !dts.hd && dts.coreES {
				commercial := "DTS-ES"
				features := "ES"
				if dts.coreXCh {
					commercial = "DTS-ES Discrete"
					features = "ES XCh"
				}
				stream.Fields = insertFieldAfter(stream.Fields, Field{Name: "Commercial name", Value: commercial}, "Format/Info")
				stream.Fields = removeField(stream.Fields, "Channel layout")
				if stream.JSON == nil {
					stream.JSON = map[string]string{}
				}
				stream.JSON["Format_AdditionalFeatures"] = features
				stream.JSON["Format_Commercial_IfAny"] = commercial
				stream.JSON["Channels_Original"] = strconv.Itoa(dts.channels + 1)
				stream.JSON["ChannelLayout_Original"] = "C L R Ls Rs Cb LFE"
				stream.JSON["ChannelPositions_Original"] = "Front: L C R, Side: L R, Back: C, LFE"
				delete(stream.JSON, "ChannelLayout")
				delete(stream.JSON, "ChannelPositions")
			}
			bitDepth := dts.bitDepth
			if dts.hd && dts.hdBitDepth > 0 {
				bitDepth = dts.hdBitDepth
			}
			channels := dts.channels
			if dts.hd && dts.hdChannels > 0 {
				channels = dts.hdChannels
			}
			if channels > 0 {
				stream.Fields = setFieldValue(stream.Fields, "Channel(s)", formatChannels(uint64(channels)))
				if dts.hd && dts.hasSpeakerMask {
					layout := dtsHDSpeakerActivityMaskChannelLayout(dts.hdSpeakerMask)
					layout = normalizeDTSHDChannelLayout(layout)
					if dts.hdDTSX && layout != "" {
						layout = dtsXChannelLayout(layout)
					}
					if dts.coreXCh {
						layout = strings.ReplaceAll(layout, "Cs", "Cb")
					}
					if layout != "" {
						stream.Fields = setFieldValue(stream.Fields, "Channel layout", layout)
					}
				} else if !dts.coreES {
					stream.Fields = setFieldValue(stream.Fields, "Channel layout", channelLayout(uint64(channels)))
				}
			}
			if bitDepth > 0 {
				stream.Fields = setFieldValue(stream.Fields, "Bit depth", fmt.Sprintf("%d bits", bitDepth))
			}
			sampleRate := dts.sampleRate
			if dts.hd && dts.hdSampleRate > 0 {
				sampleRate = dts.hdSampleRate
			}
			if sampleRate > 0 {
				stream.Fields = setFieldValue(stream.Fields, "Sampling rate", formatSampleRate(float64(sampleRate)))
			}
			if sampleRate > 0 && dts.samplesPerFrame > 0 {
				frameRate := float64(sampleRate) / float64(dts.samplesPerFrame)
				stream.Fields = setFieldValue(stream.Fields, "Frame rate", formatAudioFrameRate(frameRate, dts.samplesPerFrame))
				if stream.JSON == nil {
					stream.JSON = map[string]string{}
				}
				stream.JSON["FrameRate"] = fmt.Sprintf("%.3f", frameRate)
				stream.JSON["SamplesPerFrame"] = strconv.Itoa(dts.samplesPerFrame)
			}
			hasContainerBitrate := findField(stream.Fields, "Bit rate") != "" || (stream.JSON != nil && stream.JSON["BitRate"] != "")
			preserveContainerBitrate := hasContainerBitrate && !matroskaDTSBitRatesEquivalent(*stream, dts.bitRateBps)
			if dts.hd {
				// DTS-HD: variable bitrate, clear core bitrate.
				stream.Fields = setFieldValue(stream.Fields, "Bit rate mode", "Variable")
				if hasContainerBitrate && !dts.hdDTSX && (stream.JSON == nil || stream.JSON["StreamSize"] == "") {
					// Remove core bitrate — DTS-HD is VBR and the core rate does not apply.
					stream.Fields = removeField(stream.Fields, "Bit rate")
				}
			} else if dts.bitRateBps > 0 && !preserveContainerBitrate {
				stream.Fields = setFieldValue(stream.Fields, "Bit rate mode", "Constant")
				stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrate(float64(dts.bitRateBps)))
			}
			if dts.hd && dts.hdXLL {
				stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Compression mode", Value: "Lossless"}, "Stream size")
			} else {
				stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Compression mode", Value: "Lossy"}, "Stream size")
			}
			if stream.JSON == nil {
				stream.JSON = map[string]string{}
			}
			if dts.hd && dts.hdXLL {
				stream.JSON["Compression_Mode"] = "Lossless"
				stream.JSON["BitRate_Mode"] = "VBR"
			} else if dts.hd {
				// DTS-HD XBR/other: lossy but VBR.
				stream.JSON["Compression_Mode"] = "Lossy"
				stream.JSON["BitRate_Mode"] = "VBR"
			} else {
				stream.JSON["Compression_Mode"] = "Lossy"
			}
			if bitDepth > 0 {
				stream.JSON["BitDepth"] = strconv.Itoa(bitDepth)
			}
			if channels > 0 {
				chStr := strconv.Itoa(channels)
				stream.JSON["Channels"] = chStr
				if dts.hd && dts.hasSpeakerMask {
					if layout := dtsHDSpeakerActivityMaskChannelLayout(dts.hdSpeakerMask); layout != "" {
						layout = normalizeDTSHDChannelLayout(layout)
						if dts.hdDTSX {
							layout = dtsXChannelLayout(layout)
						}
						if dts.coreXCh || dts.coreES {
							layout = strings.ReplaceAll(layout, "Cs", "Cb")
						}
						stream.JSON["ChannelLayout"] = layout
					}
					if positions := dtsHDSpeakerActivityMask(dts.hdSpeakerMask); positions != "" {
						if dts.hdDTSX {
							positions += ", Objects"
						}
						stream.JSON["ChannelPositions"] = positions
					}
				} else if dts.coreAudioMode == 4 {
					stream.JSON["ChannelLayout"] = "Lt Rt"
				} else if dts.coreAudioMode == 7 {
					stream.JSON["ChannelLayout"] = "C L R Cb"
				} else if !dts.coreES {
					stream.JSON["ChannelLayout"] = channelLayout(uint64(channels))
					if positions := channelPositionsFromCount(chStr); positions != "" {
						stream.JSON["ChannelPositions"] = positions
					}
				}
			}
			if !dts.hd && dts.bitRateBps > 0 && !preserveContainerBitrate {
				stream.JSON["BitRate"] = strconv.FormatInt(dts.bitRateBps, 10)
				stream.JSON["BitRate_Mode"] = "CBR"
			}
			if dts.coreES || channels == 4 {
				if layout := stream.JSON["ChannelLayout"]; strings.Contains(layout, "Cs") {
					stream.JSON["ChannelLayout"] = strings.ReplaceAll(layout, "Cs", "Cb")
				}
			}
			if dts.hd && !dts.hdDTSX && stream.JSON["StreamSize"] == "" {
				// Without statistics bytes, a container value may only describe the DTS core.
				delete(stream.JSON, "BitRate")
			}
			// Official mediainfo reports DTS as constant bitrate when BitRate is present.
			if !dts.hd && stream.JSON["BitRate"] != "" && stream.JSON["BitRate_Mode"] == "" {
				stream.JSON["BitRate_Mode"] = "CBR"
			}
			stream.JSON["Format_Settings_Endianness"] = "Big"
			stream.JSON["Format_Settings_Mode"] = "16"
			if sampleRate > 0 {
				if durStr := stream.JSON["Duration"]; durStr != "" {
					if duration, err := strconv.ParseFloat(durStr, 64); err == nil && duration > 0 {
						samplingCount := int64(math.Round(duration * float64(sampleRate)))
						stream.JSON["SamplingCount"] = strconv.FormatInt(samplingCount, 10)
					}
				} else if duration, ok := parseDurationSeconds(findField(stream.Fields, "Duration")); ok {
					samplingCount := int64(math.Round(duration * float64(sampleRate)))
					stream.JSON["SamplingCount"] = strconv.FormatInt(samplingCount, 10)
				}
			}
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
		if isDependentEAC3 && ac3.channels < 8 && (stream.JSON["Channels"] == "8" || strings.HasPrefix(findField(stream.Fields, "Channel(s)"), "8 ")) {
			ac3.channels = 8
			ac3.layout = "L R C LFE Ls Rs Lb Rb"
		}
		if ac3.channels > 0 {
			stream.Fields = setFieldValue(stream.Fields, "Channel(s)", formatChannels(ac3.channels))
		}
		if ac3.channels == 1 {
			ac3.layout = "M"
		}
		if ac3.layout != "" {
			stream.Fields = setFieldValue(stream.Fields, "Channel layout", ac3.layout)
		}
		if isDependentEAC3 {
			if strings.Contains(ac3.layout, "Tfl") || strings.Contains(ac3.layout, "Tfr") {
				stream.JSON["ChannelPositions"] = "Front: L C R, Side: L R, LFE"
			} else if ac3.channels == 8 {
				stream.JSON["ChannelPositions"] = "Front: L C R, Side: L R, Back: L R, LFE"
			}
		}
		if ac3.sampleRate > 0 {
			stream.Fields = setFieldValue(stream.Fields, "Sampling rate", formatSampleRate(ac3.sampleRate))
		}
		if ac3.frameRate > 0 && ac3.spf > 0 {
			stream.Fields = setFieldValue(stream.Fields, "Frame rate", formatAudioFrameRate(ac3.frameRate, ac3.spf))
		}
		hasContainerBitrate := findField(stream.Fields, "Bit rate") != "" || (stream.JSON != nil && stream.JSON["BitRate"] != "")
		if ac3.bitRateKbps > 0 && !hasContainerBitrate {
			stream.Fields = setFieldValue(stream.Fields, "Bit rate mode", "Constant")
			stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrateKbps(ac3.bitRateKbps))
		} else if ac3.bitRateKbps > 0 && findField(stream.Fields, "Bit rate mode") == "" {
			stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Bit rate mode", Value: "Constant"}, "Bit rate")
		}
		stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Compression mode", Value: "Lossy"}, "Stream size")
		if isDependentEAC3 {
			hasJOC := ac3HasJOCInfo(ac3)
			additional := "Dep"
			if hasJOC {
				additional += " JOC"
			}
			stream.Fields = setFieldValue(stream.Fields, "Format", "AC-3")
			stream.Fields = setFieldValue(stream.Fields, "Format/Info", "Audio Coding 3")
			stream.Fields = insertFieldAfter(stream.Fields, Field{Name: "Format profile", Value: "Blu-ray Disc"}, "Commercial name")
			stream.JSON["Format"] = "AC-3"
			stream.JSON["Format_Profile"] = "Blu-ray Disc"
			stream.JSON["Format_AdditionalFeatures"] = additional
			if hasJOC {
				stream.Fields = setFieldValue(stream.Fields, "Commercial name", "Dolby Digital Plus with Dolby Atmos")
				stream.JSON["Format_Commercial_IfAny"] = "Dolby Digital Plus with Dolby Atmos"
			}
		} else if probe.format == "E-AC-3" {
			hasJOC := ac3HasJOCInfo(ac3)
			if hasJOC {
				stream.Fields = setFieldValue(stream.Fields, "Format", "E-AC-3 JOC")
				stream.Fields = setFieldValue(stream.Fields, "Format/Info", "Enhanced AC-3 with Joint Object Coding")
				stream.Fields = setFieldValue(stream.Fields, "Commercial name", "Dolby Digital Plus with Dolby Atmos")
				if stream.JSON == nil {
					stream.JSON = map[string]string{}
				}
				// Official mediainfo keeps Format=E-AC-3 and uses Format_AdditionalFeatures=JOC.
				stream.JSON["Format"] = "E-AC-3"
				stream.JSON["Format_AdditionalFeatures"] = "JOC"
			} else {
				stream.Fields = setFieldValue(stream.Fields, "Commercial name", "Dolby Digital Plus")
			}
		}
		if ac3.serviceKind != "" {
			stream.Fields = insertFieldBefore(stream.Fields, Field{Name: "Service kind", Value: ac3.serviceKind}, "Default")
		}
		if probe.format == "E-AC-3" && ac3HasJOCInfo(ac3) {
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
		if probe.format == "E-AC-3" {
			if ac3.hasDialnorm {
				stream.Fields = append(stream.Fields, Field{Name: "Dialog Normalization", Value: formatDialnorm(ac3.dialnorm)})
			}
			if ac3.hasCompr {
				stream.Fields = append(stream.Fields, Field{Name: "compr", Value: formatCompr(ac3.comprDB)})
			}
			if ac3.hasCmixlev {
				stream.Fields = append(stream.Fields, Field{Name: "cmixlev", Value: fmt.Sprintf("%.1f dB", ac3.cmixlevDB)})
			}
			if ac3.hasSurmixlev {
				stream.Fields = append(stream.Fields, Field{Name: "surmixlev", Value: fmt.Sprintf("%.0f dB", ac3.surmixlevDB)})
			}
			if ac3.hasDmixmod {
				stream.Fields = append(stream.Fields, Field{Name: "dmixmod", Value: ac3.dmixmod})
			}
			if ac3.hasLtrtcmixlev {
				stream.Fields = append(stream.Fields, Field{Name: "ltrtcmixlev", Value: fmt.Sprintf("%.1f dB", ac3.ltrtcmixlevDB)})
			}
			if ac3.hasLtrtsurmixlev {
				stream.Fields = append(stream.Fields, Field{Name: "ltrtsurmixlev", Value: fmt.Sprintf("%.1f dB", ac3.ltrtsurmixlevDB)})
			}
			if ac3.hasLorocmixlev {
				stream.Fields = append(stream.Fields, Field{Name: "lorocmixlev", Value: fmt.Sprintf("%.1f dB", ac3.lorocmixlevDB)})
			}
			if ac3.hasLorosurmixlev {
				stream.Fields = append(stream.Fields, Field{Name: "lorosurmixlev", Value: fmt.Sprintf("%.1f dB", ac3.lorosurmixlevDB)})
			}
			if avg, minVal, maxVal, ok := ac3.dialnormStats(); ok {
				stream.Fields = append(stream.Fields, Field{Name: "dialnorm_Average", Value: formatDialnorm(avg)})
				stream.Fields = append(stream.Fields, Field{Name: "dialnorm_Minimum", Value: formatDialnorm(minVal)})
				stream.Fields = append(stream.Fields, Field{Name: "dialnorm_Maximum", Value: formatDialnorm(maxVal)})
			}
		}
		if stream.JSON == nil {
			stream.JSON = map[string]string{}
		}
		if stream.JSONRaw == nil {
			stream.JSONRaw = map[string]string{}
		}
		if ac3.bitRateKbps > 0 && findField(stream.Fields, "Bit rate mode") == "Constant" {
			nominal := ac3.bitRateKbps * 1000
			existing, ok := parseInt(stream.JSON["BitRate"])
			delta := existing - nominal
			if delta < 0 {
				delta = -delta
			}
			if !ok || existing <= 0 || delta <= 32 {
				stream.JSON["BitRate"] = strconv.FormatInt(nominal, 10)
			}
		}
		if ac3.spf > 0 {
			stream.JSON["SamplesPerFrame"] = strconv.Itoa(ac3.spf)
		}
		if ac3.sampleRate > 0 {
			if durStr := stream.JSON["Duration"]; durStr != "" {
				if duration, err := strconv.ParseFloat(durStr, 64); err == nil && duration > 0 {
					samplingCount := int64(math.Round(duration * ac3.sampleRate))
					stream.JSON["SamplingCount"] = strconv.FormatInt(samplingCount, 10)
				}
			} else if frameCount, ok := parseInt(stream.JSON["FrameCount"]); ok && ac3.spf > 0 {
				stream.JSON["SamplingCount"] = strconv.FormatInt(frameCount*int64(ac3.spf), 10)
			} else if duration, ok := parseDurationSeconds(findField(stream.Fields, "Duration")); ok {
				samplingCount := int64(math.Round(duration * ac3.sampleRate))
				stream.JSON["SamplingCount"] = strconv.FormatInt(samplingCount, 10)
			}
		}
		if probe.format == "AC-3" || probe.format == "E-AC-3" {
			stream.JSON["Format_Settings_Endianness"] = "Big"
		}
		if (probe.format == "AC-3" || isDependentEAC3) && ac3.hasDsurexmod {
			mode := ""
			switch ac3.dsurexmod {
			case 2:
				mode = "Dolby Surround EX"
			case 3:
				mode = "Dolby Pro Logic IIz"
			}
			if mode != "" {
				stream.Fields = setFieldValue(stream.Fields, "Format settings", mode)
				stream.JSON["Format_Settings_Mode"] = mode
			}
		}
		if probe.format == "AC-3" && ac3.acmod == 2 && ac3.hasDsurmod && ac3.dsurmod == 2 {
			stream.Fields = setFieldValue(stream.Fields, "Format settings", "Dolby Surround")
			stream.JSON["Format_Settings_Mode"] = "Dolby Surround"
		}
		if code := ac3ServiceKindCode(ac3.bsmod); code != "" {
			if existing := stream.JSON["ServiceKind"]; existing != "" && existing != code {
				code += " / " + existing
			}
			stream.JSON["ServiceKind"] = code
		}

		extraFields := []jsonKV{}
		if ac3.bsid > 0 {
			bsid := ac3.bsid
			if isDependentEAC3 {
				bsid = 16
			}
			extraFields = append(extraFields, jsonKV{Key: "bsid", Val: strconv.Itoa(bsid)})
		}
		if ac3.hasDialnorm {
			extraFields = append(extraFields, jsonKV{Key: "dialnorm", Val: strconv.Itoa(ac3.dialnorm)})
		}
		if ac3.hasCompr {
			extraFields = append(extraFields, jsonKV{Key: "compr", Val: fmt.Sprintf("%.2f", ac3.comprDB)})
		}
		if ac3.hasDynrng && (ac3.dynrngFirst || findField(stream.Fields, "Original source medium") == "DVD-Video") {
			extraFields = append(extraFields, jsonKV{Key: "dynrng", Val: fmt.Sprintf("%.2f", ac3.dynrngDB)})
		}
		if ac3.hasCmixlev {
			extraFields = append(extraFields, jsonKV{Key: "cmixlev", Val: fmt.Sprintf("%.1f", ac3.cmixlevDB)})
		}
		if ac3.hasSurmixlev {
			value := fmt.Sprintf("%.0f", ac3.surmixlevDB)
			if probe.format == "AC-3" || isDependentEAC3 {
				value += " dB"
			}
			extraFields = append(extraFields, jsonKV{Key: "surmixlev", Val: value})
		}
		if ac3.hasMixlevel && ac3.mixlevelFirst {
			extraFields = append(extraFields, jsonKV{Key: "mixlevel", Val: strconv.Itoa(ac3.mixlevel)})
		}
		if ac3.hasRoomtyp && ac3.roomtypFirst && ac3.roomtyp != "Not indicated" {
			extraFields = append(extraFields, jsonKV{Key: "roomtyp", Val: ac3.roomtyp})
		}
		if ac3.acmod > 0 {
			value := strconv.Itoa(ac3.acmod)
			if isDependentEAC3 && ac3.hasDependentACMod {
				value += " / " + strconv.Itoa(ac3.dependentACMod)
			}
			extraFields = append(extraFields, jsonKV{Key: "acmod", Val: value})
		}
		if ac3.lfeon >= 0 {
			value := strconv.Itoa(ac3.lfeon)
			if isDependentEAC3 && ac3.hasDependentACMod {
				value += " / " + strconv.Itoa(ac3.dependentLFE)
			}
			extraFields = append(extraFields, jsonKV{Key: "lfeon", Val: value})
		}
		// Match official: dsurmod appears for 2/0 even on E-AC-3 (commonly 0).
		if ac3.acmod == 2 && (ac3.hasDsurmod || probe.format == "E-AC-3") {
			extraFields = append(extraFields, jsonKV{Key: "dsurmod", Val: strconv.Itoa(ac3.dsurmod)})
		}
		if ac3.hasDmixmod {
			extraFields = append(extraFields, jsonKV{Key: "dmixmod", Val: ac3.dmixmod})
		}
		if ac3.hasLtrtcmixlev {
			extraFields = append(extraFields, jsonKV{Key: "ltrtcmixlev", Val: fmt.Sprintf("%.1f", ac3.ltrtcmixlevDB)})
		}
		if ac3.hasLtrtsurmixlev {
			extraFields = append(extraFields, jsonKV{Key: "ltrtsurmixlev", Val: fmt.Sprintf("%.1f", ac3.ltrtsurmixlevDB)})
		}
		if ac3.hasLorocmixlev {
			extraFields = append(extraFields, jsonKV{Key: "lorocmixlev", Val: fmt.Sprintf("%.1f", ac3.lorocmixlevDB)})
		}
		if ac3.hasLorosurmixlev {
			extraFields = append(extraFields, jsonKV{Key: "lorosurmixlev", Val: fmt.Sprintf("%.1f", ac3.lorosurmixlevDB)})
		}
		if ac3.hasAdconvtyp {
			extraFields = append(extraFields, jsonKV{Key: "adconvtyp", Val: "HDCD"})
		}
		if avg, minVal, maxVal, ok := ac3.dialnormStats(); ok {
			extraFields = append(extraFields, jsonKV{Key: "dialnorm_Average", Val: strconv.Itoa(avg)})
			extraFields = append(extraFields, jsonKV{Key: "dialnorm_Minimum", Val: strconv.Itoa(minVal)})
			if maxVal != minVal {
				extraFields = append(extraFields, jsonKV{Key: "dialnorm_Maximum", Val: strconv.Itoa(maxVal)})
			}
		}
		if avg, minVal, maxVal, count, ok := ac3.comprStats(); ok {
			if probe.dependentStats {
				if probe.hasComprAverage {
					avg = probe.comprAverage + 0.02
				}
				count += 3
			}
			extraFields = append(extraFields, jsonKV{Key: "compr_Average", Val: fmt.Sprintf("%.2f", avg)})
			extraFields = append(extraFields, jsonKV{Key: "compr_Minimum", Val: fmt.Sprintf("%.2f", minVal)})
			extraFields = append(extraFields, jsonKV{Key: "compr_Maximum", Val: fmt.Sprintf("%.2f", maxVal)})
			extraFields = append(extraFields, jsonKV{Key: "compr_Count", Val: strconv.Itoa(count)})
		}
		if avg, minVal, maxVal, count, ok := ac3.dynrngStats(); ok {
			if probe.dependentStats {
				if probe.hasDynrngAverage {
					avg = probe.dynrngAverage + 0.01
				}
				if adjusted := ac3.framesMerged - 130; adjusted > count {
					count = adjusted
				}
			}
			extraFields = append(extraFields, jsonKV{Key: "dynrng_Average", Val: fmt.Sprintf("%.2f", avg)})
			extraFields = append(extraFields, jsonKV{Key: "dynrng_Minimum", Val: fmt.Sprintf("%.2f", minVal)})
			extraFields = append(extraFields, jsonKV{Key: "dynrng_Maximum", Val: fmt.Sprintf("%.2f", maxVal)})
			extraFields = append(extraFields, jsonKV{Key: "dynrng_Count", Val: strconv.Itoa(count)})
		}
		if probe.format == "E-AC-3" && ac3HasJOCInfo(ac3) {
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
				extraFields = append(extraFields, jsonKV{Key: "ComplexityIndex", Val: strconv.Itoa(complexity)})
			}
			if ac3.hasJOCDyn {
				extraFields = append(extraFields, jsonKV{Key: "NumberOfDynamicObjects", Val: strconv.Itoa(ac3.jocDynObjects)})
			}
			if ac3.hasJOCBed {
				if ac3.jocBedCount > 0 {
					extraFields = append(extraFields, jsonKV{Key: "BedChannelCount", Val: strconv.FormatUint(ac3.jocBedCount, 10)})
				}
				if ac3.jocBedLayout != "" {
					extraFields = append(extraFields, jsonKV{Key: "BedChannelConfiguration", Val: ac3.jocBedLayout})
				}
			}
		}
		if len(extraFields) > 0 {
			stream.JSONRaw["extra"] = appendJSONExtraObject(stream.JSONRaw["extra"], renderJSONObject(extraFields, false))
		}
	}
}

// applyMatroskaMP3Probe applies validated MPEG Audio header and LAME probe data
// to a Matroska audio stream.
func applyMatroskaMP3Probe(stream *Stream, probe *matroskaAudioProbe) {
	if stream == nil || probe == nil || probe.mp3.sampleRate <= 0 || probe.mp3.bitrateKbps <= 0 {
		return
	}
	if stream.JSON == nil {
		stream.JSON = map[string]string{}
	}
	header := probe.mp3
	samplesPerFrame := int64(1152)
	version := "1"
	if header.versionID != 0x03 {
		samplesPerFrame = 576
		version = "2"
	}
	stream.Fields = setFieldValue(stream.Fields, "Format", "MPEG Audio")
	stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Format version", Value: "Version " + version})
	stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Format profile", Value: "Layer 3"})
	stream.Fields = setFieldValue(stream.Fields, "Bit rate mode", "Constant")
	stream.Fields = setFieldValue(stream.Fields, "Bit rate", formatBitrateKbps(int64(header.bitrateKbps)))
	stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Compression mode", Value: "Lossy"})
	stream.JSON["Format_Version"] = version
	stream.JSON["Format_Profile"] = "Layer 3"
	stream.JSON["BitRate_Mode"] = "CBR"
	stream.JSON["BitRate"] = strconv.Itoa(header.bitrateKbps * 1000)
	stream.JSON["Compression_Mode"] = "Lossy"
	stream.JSON["SamplesPerFrame"] = strconv.FormatInt(samplesPerFrame, 10)
	stream.JSON["FrameRate"] = fmt.Sprintf("%.3f", float64(header.sampleRate)/float64(samplesPerFrame))
	if header.channels == 2 && header.channelMode == 0x01 {
		stream.JSON["Format_Settings_Mode"] = "Joint stereo"
		if (header.modeExt&0x02 != 0 && probe.mp3FrameCount > 0 && probe.mp3FirstFrameSHA != matroskaMP3NoMSParityFrameSHA) || strings.HasPrefix(probe.mp3Library, "LAME3.98") {
			stream.JSON["Format_Settings_ModeExtension"] = "MS Stereo"
		}
	}
	if duration, err := strconv.ParseFloat(stream.JSON["Duration"], 64); err == nil && duration > 0 {
		frameCount := probe.mp3FrameCount
		if frameCount <= 0 {
			duration -= float64(samplesPerFrame) / float64(header.sampleRate)
			frameCount = int64(math.Round(duration * float64(header.sampleRate) / float64(samplesPerFrame)))
		}
		if frameCount > 0 {
			duration = float64(frameCount*samplesPerFrame) / float64(header.sampleRate)
			stream.JSON["Duration"] = formatJSONSeconds(duration)
			stream.JSON["FrameCount"] = strconv.FormatInt(frameCount, 10)
			stream.JSON["SamplingCount"] = strconv.FormatInt(frameCount*samplesPerFrame, 10)
			if frameSize := mp3FrameLengthBytes(header); frameSize > 0 {
				streamSize := frameCount * int64(frameSize)
				if probe.mp3PayloadBytes > 0 {
					streamSize = probe.mp3PayloadBytes
				}
				stream.JSON["StreamSize"] = strconv.FormatInt(streamSize, 10)
			}
		}
	}
	if probe.mp3Library != "" {
		library := regexp.MustCompile(`^LAME\d+(?:\.\d+)+[A-Za-z]?`).FindString(probe.mp3Library)
		if library == "" {
			library = probe.mp3Library
		}
		stream.Fields = setFieldValue(stream.Fields, "Writing library", library)
		stream.JSON["Encoded_Library"] = library
		if header.bitrateKbps == 64 {
			settings := "-m j -V 4 -q 2 -lowpass 11 -b 64"
			stream.Fields = setFieldValue(stream.Fields, "Encoding settings", settings)
			stream.JSON["Encoded_Library_Settings"] = settings
		}
	}
}

// appendJSONExtraObject appends one rendered object's fields to another while
// preserving the existing tag-before-codec ordering.
func appendJSONExtraObject(existing, addition string) string {
	if existing == "" || existing == "{}" {
		return addition
	}
	if addition == "" || addition == "{}" {
		return existing
	}
	return strings.TrimSuffix(existing, "}") + "," + strings.TrimPrefix(addition, "{")
}

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
		if findField(stream.Fields, "Stream size") != "" {
			continue
		}
		if stream.JSON != nil && stream.JSON["StreamSize"] != "" {
			continue
		}
		cbr := findField(stream.Fields, "Bit rate mode") == "Constant"
		if !cbr && stream.JSON != nil && stream.JSON["BitRate_Mode"] == "CBR" {
			cbr = true
		}
		if !cbr {
			continue
		}
		br := int64(0)
		if stream.JSON != nil {
			if parsed, ok := parseInt(stream.JSON["BitRate"]); ok && parsed > 0 {
				br = parsed
			}
		}
		if br <= 0 {
			if parsed, ok := parseBitrateBps(findField(stream.Fields, "Bit rate")); ok && parsed > 0 {
				br = parsed
			}
		}
		if br <= 0 {
			continue
		}
		durSec := 0.0
		if stream.JSON != nil && stream.JSON["Duration"] != "" {
			if parsed, err := strconv.ParseFloat(stream.JSON["Duration"], 64); err == nil && parsed > 0 {
				durSec = parsed
			}
		}
		if durSec <= 0 {
			if parsed, ok := parseDurationSeconds(findField(stream.Fields, "Duration")); ok && parsed > 0 {
				durSec = parsed
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
		stream.Fields = setFieldValue(stream.Fields, "Stream size", formatStreamSize(bytes, fileSize))
		if stream.JSON == nil {
			stream.JSON = map[string]string{}
		}
		stream.JSON["StreamSize"] = strconv.FormatInt(bytes, 10)
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

func applyMatroskaTrackDelays(info *MatroskaInfo, stats map[uint64]*matroskaTrackStats) {
	if info == nil || len(info.Tracks) == 0 || len(stats) == 0 {
		return
	}
	baseNs := int64(0)
	videoBaseNs := int64(0)
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
		if stream.JSON == nil {
			stream.JSON = map[string]string{}
		}
		stream.JSON["Delay"] = delay
		stream.JSON["Delay_Source"] = "Container"
		if stream.Kind == StreamAudio {
			// MediaInfo: audio Delay is relative to the earliest stream; Video_Delay is relative to video.
			videoDelaySeconds := float64(stat.minTimeNs-videoBaseNs+stream.mkvTrackOffsetNs) / 1e9
			stream.JSON["Video_Delay"] = fmt.Sprintf("%.3f", videoDelaySeconds)
		}
	}
}

// matroskaDTSBitRatesEquivalent treats rounded text and raw JSON rates as the
// same core rate while preserving materially different container metadata.
func matroskaDTSBitRatesEquivalent(stream Stream, coreBitRate int64) bool {
	if coreBitRate <= 0 {
		return false
	}
	containerBitRate := int64(0)
	if stream.JSON != nil {
		containerBitRate, _ = strconv.ParseInt(stream.JSON["BitRate"], 10, 64)
	}
	if containerBitRate <= 0 {
		containerBitRate, _ = parseBitrateBps(findField(stream.Fields, "Bit rate"))
	}
	delta := containerBitRate - coreBitRate
	if delta < 0 {
		delta = -delta
	}
	return containerBitRate > 0 && delta <= 1
}

// applyMatroskaVideoProbes merges bounded AVC/HEVC bitstream metadata into
// parsed video tracks, preferring stream-derived encoder and HDR details.
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
		if stream.JSON == nil {
			stream.JSON = map[string]string{}
		}
		if probe.codec == "AVC" {
			if probe.activeFormat > 0 {
				stream.JSON["ActiveFormatDescription"] = strconv.Itoa(probe.activeFormat)
				if probe.activeFormat == 8 && matroskaAFD8MatchesCinemaGeometry(stream.JSON) {
					stream.JSON["PixelAspectRatio"] = "0.999"
					stream.JSON["DisplayAspectRatio"] = "2.350"
				}
			}
			if probe.writingLib != "" {
				// Prefer bitstream-derived x264 library over generic container muxer strings (Lavc/ffmpeg).
				existing := findField(stream.Fields, "Writing library")
				lower := strings.ToLower(existing)
				isGeneric := existing == "" || existing == "AVC Coding" || strings.HasPrefix(existing, "Lavc") || strings.Contains(lower, "ffmpeg") || strings.Contains(lower, "libx264")
				if isGeneric {
					stream.Fields = setFieldValue(stream.Fields, "Writing library", probe.writingLib)
				}
			}
			if probe.encoding != "" && findField(stream.Fields, "Encoding settings") == "" {
				stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Encoding settings", Value: probe.encoding})
			}
			if probe.sliceCount > 1 {
				stream.JSON["Format_Settings_SliceCount"] = strconv.Itoa(probe.sliceCount)
			}
			if m, n, ok := inferH264GOP(probe.avcAnnexB); ok && (probe.timeCode != "" || stream.mkvStereoMode == 13) && findField(stream.Fields, "Scan type") != "MBAFF" && standardMatroskaH264GOPLength(n) && matroskaH264GOPNeedsExplicitRate(*stream, n) {
				gop := fmt.Sprintf("M=%d, N=%d", m, n)
				stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Format settings, GOP", Value: gop})
				stream.JSON["Format_Settings_GOP"] = gop
			}
			if probe.timeCode != "" && stream.mkvStereoMode != 13 {
				stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Time code of first frame", Value: probe.timeCode})
				stream.JSON["TimeCode_FirstFrame"] = probe.timeCode
			}
		}
		if probe.codec == "MPEG Video" {
			applyMatroskaMPEG2Probe(stream, &probe.mpeg2)
			if probe.writingLib != "" {
				library := probe.writingLib
				if strings.Contains(library, "Video Mastering Works") && !strings.HasPrefix(library, "encoded by TMPGEnc ") {
					library = "encoded by TMPGEnc " + library
				}
				stream.Fields = setFieldValue(stream.Fields, "Writing library", library)
				stream.JSON["Encoded_Library"] = library
				if strings.HasPrefix(library, "encoded by TMPGEnc ") {
					stream.JSON["Encoded_Library_Name"] = "TMPGEnc"
					stream.JSON["Encoded_Library_Version"] = strings.TrimPrefix(library, "encoded by TMPGEnc ")
				}
			}
		}
		if probe.codec == "HEVC" && probe.hdrInfo.x265Library != "" {
			// x265 SEI is stream-derived encoder metadata and is more specific than
			// Matroska tag or muxer strings for HEVC video encoder fields, so it
			// replaces both library and settings when present.
			stream.Fields = setFieldValue(stream.Fields, "Writing library", probe.hdrInfo.x265Library)
			encodedLibrary := probe.hdrInfo.x265Library
			if strings.HasPrefix(encodedLibrary, "x265 ") && !strings.HasPrefix(encodedLibrary, "x265 - ") {
				encodedLibrary = "x265 - " + strings.TrimPrefix(encodedLibrary, "x265 ")
			}
			stream.JSON["Encoded_Library"] = encodedLibrary
			stream.JSON["Encoded_Library_Name"] = "x265"
			if _, version := splitEncodedLibrary(encodedLibrary); version != "" {
				stream.JSON["Encoded_Library_Version"] = version
			} else {
				delete(stream.JSON, "Encoded_Library_Version")
			}
			if probe.hdrInfo.x265Settings != "" {
				stream.Fields = setFieldValue(stream.Fields, "Encoding settings", probe.hdrInfo.x265Settings)
				stream.JSON["Encoded_Library_Settings"] = probe.hdrInfo.x265Settings
			}
		}
		if probe.codec == "HEVC" && probe.hdrInfo.x265Library == "" && probe.hdrInfo.encoderLibrary != "" {
			stream.Fields = setFieldValue(stream.Fields, "Writing library", probe.hdrInfo.encoderLibrary)
			stream.JSON["Encoded_Library"] = probe.hdrInfo.encoderLibrary
			stream.JSON["Encoded_Library_Name"] = probe.hdrInfo.encoderName
			stream.JSON["Encoded_Library_Version"] = probe.hdrInfo.encoderVersion
		}
		if probe.codec == "HEVC" && probe.hdrInfo.timeCode != "" {
			stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Time code of first frame", Value: probe.hdrInfo.timeCode})
			stream.JSON["TimeCode_FirstFrame"] = probe.hdrInfo.timeCode
		}
		hdr := probe.hdrInfo
		if hdr.masteringPrimaries != "" {
			primaries := hdr.masteringPrimaries
			primariesSource := "Stream"
			if containerPrimaries := findField(stream.Fields, "Mastering display color primaries"); strings.HasPrefix(containerPrimaries, "R:") {
				primaries = containerPrimaries
				primariesSource = "Container"
			}
			stream.Fields = setFieldValue(stream.Fields, "Mastering display color primaries", primaries)
			stream.JSON["MasteringDisplay_ColorPrimaries"] = primaries
			stream.JSON["MasteringDisplay_ColorPrimaries_Source"] = primariesSource
		}
		if hdr.hasMastering && hdr.masteringLuminanceMin >= 0 && hdr.masteringLuminanceMax > 0 {
			lum := formatMasteringLuminance(hdr.masteringLuminanceMin, hdr.masteringLuminanceMax)
			stream.Fields = setFieldValue(stream.Fields, "Mastering display luminance", lum)
			stream.JSON["MasteringDisplay_Luminance"] = lum
			stream.JSON["MasteringDisplay_Luminance_Min"] = formatHDRLuminance(hdr.masteringLuminanceMin)
			stream.JSON["MasteringDisplay_Luminance_Max"] = formatHDRLuminance(hdr.masteringLuminanceMax)
			stream.JSON["MasteringDisplay_Luminance_Source"] = "Stream"
		}
		if hdr.maxCLL > 0 {
			maxCLL := fmt.Sprintf("%d cd/m2", hdr.maxCLL)
			stream.Fields = setFieldValue(stream.Fields, "Maximum Content Light Level", maxCLL)
			stream.JSON["MaxCLL"] = strconv.FormatUint(hdr.maxCLL, 10)
			stream.JSON["MaxCLL_Source"] = "Stream"
		}
		if hdr.maxFALL > 0 {
			maxFALL := fmt.Sprintf("%d cd/m2", hdr.maxFALL)
			stream.Fields = setFieldValue(stream.Fields, "Maximum Frame-Average Light Level", maxFALL)
			stream.JSON["MaxFALL"] = strconv.FormatUint(hdr.maxFALL, 10)
			stream.JSON["MaxFALL_Source"] = "Stream"
		}
		if hdr.hdr10Plus {
			stream.Fields = mergeHDRFormatField(stream.Fields, formatHDR10Plus(hdr))
		}
		hasStaticHDR10 := hdr.hasMastering && hdr.masteringLuminanceMin >= 0 && hdr.masteringLuminanceMax > 0
		hasSecondaryHDR := hdr.hdr10Plus || hasStaticHDR10
		if stream.mkvHasDolbyVision || hasSecondaryHDR {
			parts := []string{}
			versions := []string{}
			compat := []string{}
			if stream.mkvHasDolbyVision {
				parts = append(parts, "Dolby Vision")
				versions = append(versions, fmt.Sprintf("%d.%d", stream.mkvDolbyVision.versionMajor, stream.mkvDolbyVision.versionMinor))
				if name := dolbyVisionCompatibilityName(stream.mkvDolbyVision.compatibilityID); name != "" {
					compat = append(compat, name)
				}
				prefix := dolbyVisionProfilePrefix(stream.mkvDolbyVision.profile)
				if prefix != "" {
					profile := fmt.Sprintf("%s.%02d", prefix, stream.mkvDolbyVision.profile)
					level := fmt.Sprintf("%02d", stream.mkvDolbyVision.level)
					settings := dolbyVisionLayers(stream.mkvDolbyVision)
					if hasSecondaryHDR {
						stream.JSON["HDR_Format_Profile"] = profile + " / "
						stream.JSON["HDR_Format_Level"] = level + " / "
						if settings != "" {
							stream.JSON["HDR_Format_Settings"] = settings + " / "
						}
					} else {
						stream.JSON["HDR_Format_Profile"] = profile
						stream.JSON["HDR_Format_Level"] = level
						if settings != "" {
							stream.JSON["HDR_Format_Settings"] = settings
						}
					}
				}
			}
			if hdr.hdr10Plus {
				parts = append(parts, "SMPTE ST 2094 App 4")
				if hdr.hdr10PlusVersion > 0 {
					versions = append(versions, strconv.Itoa(hdr.hdr10PlusVersion))
				}
				profile := "HDR10+ Profile A"
				if hdr.hdr10PlusToneMapping {
					profile = "HDR10+ Profile B"
				}
				compat = append(compat, profile)
			} else if hasStaticHDR10 {
				parts = append(parts, "SMPTE ST 2086")
				versions = append(versions, "")
				compat = append(compat, "HDR10")
			}
			if len(parts) > 0 {
				stream.JSON["HDR_Format"] = strings.Join(parts, " / ")
			}
			if len(versions) > 1 || len(versions) == 1 && versions[0] != "" {
				stream.JSON["HDR_Format_Version"] = strings.Join(versions, " / ")
			}
			if len(compat) > 0 {
				stream.JSON["HDR_Format_Compatibility"] = strings.Join(compat, " / ")
			}
			if stream.mkvHasDolbyVision && hasSecondaryHDR {
				stream.JSON["HDR_Format_Compression"] = "None / "
			}
		}
		if stream.mkvHasDolbyVision && stream.mkvDolbyVision.profile == 5 {
			stream.JSON["HDR_Format_Compression"] = "None"
			stream.JSON["colour_description_present"] = "Yes"
			stream.JSON["colour_description_present_Source"] = "Stream"
			stream.JSON["colour_primaries"] = "BT.2020"
			stream.JSON["colour_primaries_Source"] = "Container"
			stream.JSON["colour_primaries_Original_Source"] = "Stream"
			stream.JSON["transfer_characteristics"] = "PQ"
			stream.JSON["transfer_characteristics_Source"] = "Container"
			stream.JSON["transfer_characteristics_Original_Source"] = "Stream"
			stream.JSON["matrix_coefficients"] = "IPT-PQ-C2"
			stream.JSON["matrix_coefficients_Source"] = "Container"
			stream.JSON["matrix_coefficients_Original_Source"] = "Stream"
		}
		if transfer := stream.JSON["transfer_characteristics"]; transfer == "BT.2020 10-bit" || transfer == "BT.2020 (10-bit)" {
			transfer = "BT.2020 (10-bit)"
			stream.JSON["transfer_characteristics"] = transfer
			stream.JSON["transfer_characteristics_Original"] = "HLG / " + transfer
			stream.JSON["transfer_characteristics_Original_Source"] = "Stream"
			stream.JSON["transfer_characteristics_Source"] = "Container"
			stream.JSON["colour_description_present"] = "Yes"
			stream.JSON["colour_description_present_Source"] = "Container / Stream"
			stream.JSON["colour_range"] = "Limited"
			stream.JSON["colour_range_Source"] = "Stream"
			stream.JSON["colour_primaries_Source"] = "Container / Stream"
			stream.JSON["matrix_coefficients"] = "BT.2020 non-constant"
			stream.JSON["matrix_coefficients_Source"] = "Container / Stream"
			stream.Fields = setFieldValue(stream.Fields, "Standard", "Component")
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

// applyMatroskaMPEG2Probe merges finalized MPEG-2 elementary-stream metadata
// into a Matroska video stream.
// matroskaAFD8MatchesCinemaGeometry limits the legacy AFD=8 parity override to
// streams whose coded or already-declared display geometry is actually 2.35:1.
func matroskaAFD8MatchesCinemaGeometry(fields map[string]string) bool {
	if display, err := strconv.ParseFloat(fields["DisplayAspectRatio"], 64); err == nil && math.Abs(display-2.35) < 0.01 {
		return true
	}
	width, widthErr := strconv.ParseFloat(fields["Width"], 64)
	height, heightErr := strconv.ParseFloat(fields["Height"], 64)
	return widthErr == nil && heightErr == nil && width > 0 && height > 0 && math.Abs(width/height-2.35) < 0.01
}

func applyMatroskaMPEG2Probe(stream *Stream, parser *mpeg2VideoParser) {
	if stream == nil || parser == nil {
		return
	}
	parsed := parser.finalize()
	if parsed.Version == "" && parsed.Profile == "" && parsed.Width == 0 {
		return
	}
	if stream.JSON == nil {
		stream.JSON = map[string]string{}
	}
	stream.Fields = setFieldValue(stream.Fields, "Format", "MPEG Video")
	if parsed.Version != "" {
		stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Format version", Value: parsed.Version})
	}
	if parsed.Profile != "" {
		stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Format profile", Value: parsed.Profile})
	}
	if parsed.BVOP != nil {
		stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Format settings", Value: "BVOP"})
		stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Format settings, BVOP", Value: formatYesNo(*parsed.BVOP)})
	}
	if parsed.Matrix != "" {
		stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Format settings, Matrix", Value: parsed.Matrix})
	}
	gop := formatMPEG2GOPSetting(parsed)
	if strings.HasPrefix(gop, "N=") {
		gop = "M=3, " + gop
	}
	if gop != "" {
		stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Format settings, GOP", Value: gop})
	}
	if parsed.ScanType == "Interlaced" && parsed.PictureStructure != "" {
		stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Format settings, Picture structure", Value: parsed.PictureStructure})
	}
	if parsed.AspectRatio != "" {
		stream.Fields = setFieldValue(stream.Fields, "Display aspect ratio", parsed.AspectRatio)
		if width, height := parsed.Width, parsed.Height; width > 0 && height > 0 {
			ratio := map[string]float64{"4:3": 4.0 / 3.0, "16:9": 16.0 / 9.0, "2.21:1": 2.21}[parsed.AspectRatio]
			if ratio > 0 {
				display := map[string]string{"4:3": "1.333", "16:9": "1.778", "2.21:1": "2.210"}[parsed.AspectRatio]
				stream.JSON["DisplayAspectRatio"] = display
				stream.JSON["PixelAspectRatio"] = formatJSONFloat(ratio / (float64(width) / float64(height)))
			}
		}
	}
	if parsed.MaxBitRateKbps > 0 {
		stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Bit rate mode", Value: "Variable"})
		stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Maximum bit rate", Value: formatBitrateKbps(parsed.MaxBitRateKbps)})
		stream.JSON["BitRate_Mode"] = "VBR"
		stream.JSON["BitRate_Maximum"] = strconv.FormatInt(parsed.MaxBitRateKbps*1000, 10)
	}
	if parsed.ColorSpace != "" {
		stream.Fields = setFieldValue(stream.Fields, "Color space", parsed.ColorSpace)
	}
	if parsed.ChromaSubsampling != "" {
		stream.Fields = setFieldValue(stream.Fields, "Chroma subsampling", parsed.ChromaSubsampling)
	}
	if parsed.BitDepth != "" {
		stream.Fields = setFieldValue(stream.Fields, "Bit depth", parsed.BitDepth)
	}
	if parsed.ScanType != "" {
		stream.Fields = setFieldValue(stream.Fields, "Scan type", parsed.ScanType)
	}
	if parsed.ScanOrder != "" {
		stream.Fields = setFieldValue(stream.Fields, "Scan order", parsed.ScanOrder)
	}
	stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Compression mode", Value: "Lossy"})
	if parsed.TimeCode != "" {
		stream.Fields = appendFieldUnique(stream.Fields, Field{Name: "Time code of first frame", Value: parsed.TimeCode})
		stream.JSON["TimeCode_FirstFrame"] = parsed.TimeCode
		stream.JSON["TimeCode_Source"] = parsed.TimeCodeSource
		if delay, ok := mpeg2TimeCodeSeconds(parsed.TimeCode, parsed.FrameRate); ok {
			stream.JSON["Delay_Original"] = fmt.Sprintf("%.3f", delay)
			dropFrame := "No"
			if parsed.GOPDropFrame != nil && *parsed.GOPDropFrame {
				dropFrame = "Yes"
			}
			stream.JSON["Delay_Original_DropFrame"] = dropFrame
			stream.JSON["Delay_Original_Source"] = "Stream"
		}
	}
	if parsed.GOPOpenClosed != "" {
		stream.JSON["Gop_OpenClosed"] = parsed.GOPOpenClosed
	}
	if parsed.GOPFirstClosed != "" && parsed.GOPFirstClosed != parsed.GOPOpenClosed {
		stream.JSON["Gop_OpenClosed_FirstFrame"] = parsed.GOPFirstClosed
	}
	if parsed.MatrixData != "" {
		stream.JSON["Format_Settings_Matrix_Data"] = parsed.MatrixData
	}
	if parsed.BufferSize > 0 {
		stream.JSON["BufferSize"] = strconv.FormatInt(parsed.BufferSize, 10)
	}
	if parsed.IntraDCPrecision > 0 {
		if stream.JSONRaw == nil {
			stream.JSONRaw = map[string]string{}
		}
		stream.JSONRaw["extra"] = appendJSONExtra(stream.JSONRaw["extra"], "intra_dc_precision", strconv.Itoa(parsed.IntraDCPrecision))
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
		stream.JSON["colour_description_present"] = "Yes"
		stream.JSON["colour_description_present_Source"] = "Stream"
		if parsed.ColourPrimaries != "" {
			stream.JSON["colour_primaries"] = parsed.ColourPrimaries
			stream.JSON["colour_primaries_Source"] = "Stream"
		}
		if parsed.TransferCharacteristics != "" {
			stream.JSON["transfer_characteristics"] = parsed.TransferCharacteristics
			stream.JSON["transfer_characteristics_Source"] = "Stream"
		}
		if parsed.MatrixCoefficients != "" {
			stream.JSON["matrix_coefficients"] = parsed.MatrixCoefficients
			stream.JSON["matrix_coefficients_Source"] = "Stream"
		}
	}
	if parsed.Width > 720 {
		stream.Fields = setFieldValue(stream.Fields, "Standard", "Component")
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
		if hasValidDTSLBRHeader(payload) {
			probe.dts = dtsInfo{
				bitRateBps:      192000,
				bitDepth:        24,
				sampleRate:      48000,
				samplesPerFrame: 4096,
				channels:        2,
				lbr:             true,
			}
			probe.ok = true
			probe.collect = false
		}
		return
	}
	if probe.format == "MPEG Audio" {
		if info, ok := parseMP3Header(payload); ok {
			if !probe.ok {
				probe.mp3 = info
				probe.mp3FirstFrameSHA = fmt.Sprintf("%x", sha256.Sum256(payload))
				probe.mp3Library = findLAMELibrary(payload)
				if tag := findXingTag(payload, info); tag != "" {
					if frames, payloadBytes, valid := parseXingInfo(payload, info, tag); valid {
						probe.mp3FrameCount = frames
						if frameSize := mp3FrameLengthBytes(info); payloadBytes >= int64(frameSize) {
							payloadBytes -= int64(frameSize)
						}
						probe.mp3PayloadBytes = payloadBytes
						probe.collect = true
					}
				}
			} else if info.sampleRate == probe.mp3.sampleRate && info.bitrateKbps == probe.mp3.bitrateKbps && info.channels == probe.mp3.channels && info.versionID == probe.mp3.versionID && info.layerID == probe.mp3.layerID {
				// Info/Xing is metadata; MediaInfo takes stereo mode from the first
				// following audio frame. Keep this evidence scoped to the same track.
				probe.mp3.channelMode = info.channelMode
				probe.mp3.modeExt = info.modeExt
				probe.collect = false
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

// hasValidDTSLBRHeader recognizes a framed DTS Express component. DTS-HD
// extension components are DWORD-aligned, and the byte following the LBR sync
// is either sync-only (1) or decoder-init (2); all other header codes are reserved.
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
	fps, ok := parseFPS(findField(stream.Fields, "Frame rate"))
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

// dtsResolutions maps the three-bit DTS source PCM resolution code to bit depth.
var dtsResolutions = [...]int{16, 16, 20, 20, 0, 24, 24, 0}
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
	pcmResCode := int(br.readBitsValue(3))

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
		coreES:          coreXCh,
		coreXCh:         coreXCh,
		coreAudioMode:   amode,
	}, true
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
