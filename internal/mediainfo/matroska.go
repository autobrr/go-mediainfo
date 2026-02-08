package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
)

const (
	mkvIDEBML                = 0x1A45DFA3
	mkvIDSegment             = 0x18538067
	mkvIDInfo                = 0x1549A966
	mkvIDCluster             = 0x1F43B675
	mkvIDSeekHead            = 0x114D9B74
	mkvIDSeek                = 0x4DBB
	mkvIDSeekID              = 0x53AB
	mkvIDSeekPosition        = 0x53AC
	mkvIDSegmentUID          = 0x73A4
	mkvIDTimecodeScale       = 0x2AD7B1
	mkvIDDuration            = 0x4489
	mkvIDDateUTC             = 0x4461
	mkvIDMuxingApp           = 0x4D80
	mkvIDWritingApp          = 0x5741
	mkvIDTitle               = 0x7BA9
	mkvIDErrorDetection      = 0x6BAA
	mkvIDTracks              = 0x1654AE6B
	mkvIDTags                = 0x1254C367
	mkvIDChapters            = 0x1043A770
	mkvIDAttachments         = 0x1941A469
	mkvIDAttachedFile        = 0x61A7
	mkvIDFileName            = 0x466E
	mkvIDTag                 = 0x7373
	mkvIDTagTargets          = 0x63C0
	mkvIDSimpleTag           = 0x67C8
	mkvIDTagName             = 0x45A3
	mkvIDTagString           = 0x4487
	mkvIDTagTrackUID         = 0x63C5
	mkvIDEditionEntry        = 0x45B9
	mkvIDChapterAtom         = 0xB6
	mkvIDChapterTimeStart    = 0x91
	mkvIDChapterDisplay      = 0x80
	mkvIDChapString          = 0x85
	mkvIDChapLanguage        = 0x437C
	mkvIDTrackEntry          = 0xAE
	mkvIDTrackNumber         = 0xD7
	mkvIDTrackUID            = 0x73C5
	mkvIDTrackType           = 0x83
	mkvIDTrackName           = 0x536E
	mkvIDTrackLanguage       = 0x22B59C
	mkvIDTrackLanguageIETF   = 0x22B59D
	mkvIDTrackOffset         = 0x537F
	mkvIDCodecID             = 0x86
	mkvIDCodecPrivate        = 0x63A2
	mkvIDCodecName           = 0x258688
	mkvIDContentEncodings    = 0x6D80
	mkvIDContentEncoding     = 0x6240
	mkvIDContentEncodingType = 0x5033
	mkvIDContentCompression  = 0x5034
	mkvIDContentCompAlgo     = 0x4254
	mkvIDContentCompSettings = 0x4255
	mkvIDDefaultDuration     = 0x23E383
	mkvIDTrackTimestampScale = 0x23314F
	mkvIDFlagDefault         = 0x88
	mkvIDFlagForced          = 0x55AA
	mkvIDTrackVideo          = 0xE0
	mkvIDTrackAudio          = 0xE1
	mkvIDBitRate             = 0x6264
	mkvIDPixelWidth          = 0xB0
	mkvIDPixelHeight         = 0xBA
	mkvIDDisplayWidth        = 0x54B0
	mkvIDDisplayHeight       = 0x54BA
	mkvIDDisplayUnit         = 0x54B2
	mkvIDAspectRatioType     = 0x54B3
	mkvIDPixelCropTop        = 0x54AA
	mkvIDPixelCropBottom     = 0x54BB
	mkvIDPixelCropLeft       = 0x54CC
	mkvIDPixelCropRight      = 0x54DD
	mkvIDColour              = 0x55B0
	mkvIDMasteringMetadata   = 0x55D0
	mkvIDMasteringPrimRx     = 0x55D1
	mkvIDMasteringPrimRy     = 0x55D2
	mkvIDMasteringPrimGx     = 0x55D3
	mkvIDMasteringPrimGy     = 0x55D4
	mkvIDMasteringPrimBx     = 0x55D5
	mkvIDMasteringPrimBy     = 0x55D6
	mkvIDMasteringWhiteX     = 0x55D7
	mkvIDMasteringWhiteY     = 0x55D8
	mkvIDMasteringLumMax     = 0x55D9
	mkvIDMasteringLumMin     = 0x55DA
	mkvIDMaxCLL              = 0x55BC
	mkvIDMaxFALL             = 0x55BD
	mkvIDRange               = 0x55B9
	mkvIDColourPrimaries     = 0x55BB
	mkvIDTransferChar        = 0x55BA
	mkvIDMatrixCoeffs        = 0x55B3
	mkvIDSamplingRate        = 0xB5
	mkvIDOutputSamplingRate  = 0x78B5
	mkvIDChannels            = 0x9F
	mkvIDDocType             = 0x4282
	mkvIDDocTypeVersion      = 0x4287
	mkvIDTimecode            = 0xE7
	mkvIDSimpleBlock         = 0xA3
	mkvIDBlockGroup          = 0xA0
	mkvIDBlock               = 0xA1
	mkvIDBlockDuration       = 0x9B
	mkvIDCRC32               = 0xBF
	mkvMaxScan               = int64(4 << 20)
	mkvMaxCountsScan         = int64(32 << 20)
)

const matroskaEAC3QuickProbeFrames = 1113

// MediaInfoLib stops stream parsing after PacketCount>=300 when ParseSpeed<1.
const matroskaEAC3QuickProbePackets = 300

// Bound the expensive JOC scan (full-block reads) separately; stats probing continues to PacketCount.
const matroskaEAC3QuickProbePacketsJOC = 198
const matroskaHEVCQuickProbePackets = 300
const matroskaAVCQuickProbePackets = 8

type MatroskaInfo struct {
	Container     ContainerInfo
	General       []Field
	Tracks        []Stream
	SegmentOffset int64
	SegmentSize   int64
	TimecodeScale uint64
	tagStats      map[uint64]matroskaTagStats
	attachments   []string
}

func ParseMatroska(r io.ReaderAt, size int64) (MatroskaInfo, bool) {
	return ParseMatroskaWithOptions(r, size, defaultAnalyzeOptions())
}

func ParseMatroskaWithOptions(r io.ReaderAt, size int64, opts AnalyzeOptions) (MatroskaInfo, bool) {
	opts = normalizeAnalyzeOptions(opts)
	scanSize := min(size, mkvMaxScan)
	if scanSize <= 0 {
		return MatroskaInfo{}, false
	}

	buf := make([]byte, scanSize)
	if _, err := r.ReadAt(buf, 0); err != nil && err != io.EOF {
		return MatroskaInfo{}, false
	}

	info, ok := parseMatroska(buf)
	if !ok {
		return MatroskaInfo{}, false
	}
	needsDolbyVisionProbe := false
	for i := range info.Tracks {
		if info.Tracks[i].Kind == StreamVideo && findField(info.Tracks[i].Fields, "Format") == "HEVC" {
			needsDolbyVisionProbe = true
			break
		}
	}
	if needsDolbyVisionProbe {
		if hdr := parseDolbyVisionConfigFromPrivate(buf); hdr != "" {
			for i := range info.Tracks {
				if info.Tracks[i].Kind == StreamVideo && findField(info.Tracks[i].Fields, "HDR format") == "" {
					info.Tracks[i].Fields = insertFieldBefore(info.Tracks[i].Fields, Field{Name: "HDR format", Value: hdr}, "Codec ID")
				}
			}
		}
	}
	if info.SegmentSize == 0 && info.SegmentOffset > 0 && size > info.SegmentOffset {
		info.SegmentSize = size - info.SegmentOffset
	}
	if info.SegmentOffset > 0 && info.SegmentSize > 0 && info.TimecodeScale > 0 {
		if size > scanSize {
			// If Attachments were truncated by the initial scan buffer, resolve them via SeekHead and
			// parse lazily (seek-skipping file payloads).
			attachmentsOffset := int64(0)
			if seekPos, ok := findMatroskaSeekPosition(buf, int(info.SegmentOffset), mkvIDAttachments); ok {
				attachmentsOffset = info.SegmentOffset + int64(seekPos)
			} else {
				// Some files omit Attachments from SeekHead. Fall back to a bounded scan in the initial buffer.
				needle := []byte{0x19, 0x41, 0xA4, 0x69}
				start := int(info.SegmentOffset)
				if start < 0 {
					start = 0
				}
				if start < len(buf) {
					// Search only a small prefix past SegmentOffset to avoid false positives in payloads.
					end := start + (8 << 20)
					if end > len(buf) {
						end = len(buf)
					}
					if idx := bytes.Index(buf[start:end], needle); idx >= 0 {
						attachmentsOffset = info.SegmentOffset + int64(idx)
					}
				}
			}
			if attachmentsOffset > 0 {
				if names := scanMatroskaAttachmentsFromFile(r, attachmentsOffset, size); len(names) > 0 {
					seen := map[string]struct{}{}
					for _, n := range info.attachments {
						seen[n] = struct{}{}
					}
					for _, n := range names {
						if _, ok := seen[n]; ok {
							continue
						}
						seen[n] = struct{}{}
						info.attachments = append(info.attachments, n)
					}
				}
			}
		}
		needEncoders := false
		for _, stream := range info.Tracks {
			if stream.Kind != StreamVideo || findField(stream.Fields, "Format") != "AVC" {
				continue
			}
			if findField(stream.Fields, "Writing library") == "" || findField(stream.Fields, "Encoding settings") == "" {
				needEncoders = true
				break
			}
		}
		if (len(info.tagStats) == 0 || needEncoders) && size > scanSize {
			encodedDate := findField(info.General, "Encoded date")
			if seekPos, ok := findMatroskaSeekPosition(buf, int(info.SegmentOffset), mkvIDTags); ok {
				tagsOffset := info.SegmentOffset + int64(seekPos)
				if tagsOffset > 0 && tagsOffset < size {
					tagsSize := min(size-tagsOffset, int64(8<<20))
					if tagsSize > 0 {
						tagsBuf := make([]byte, tagsSize)
						if _, err := r.ReadAt(tagsBuf, tagsOffset); err == nil || err == io.EOF {
							encodersByTrackUID, settingsByTrackUID, stats := parseMatroskaTagsFromBuffer(tagsBuf, encodedDate)
							if len(stats) > 0 {
								if info.tagStats == nil {
									info.tagStats = map[uint64]matroskaTagStats{}
								}
								for uid, st := range stats {
									current := info.tagStats[uid]
									mergeMatroskaTagStats(&current, st)
									info.tagStats[uid] = current
								}
							}
							if (len(encodersByTrackUID) > 0 || len(settingsByTrackUID) > 0) && len(info.Tracks) > 0 {
								applyMatroskaEncoders(info.Tracks, encodersByTrackUID, settingsByTrackUID)
							}
						}
					}
				}
			}
		}
		tagStatsComplete := false
		if len(info.tagStats) > 0 {
			tagStatsComplete = applyMatroskaTagStats(&info, info.tagStats, size)
		}
		audioProbes := map[uint64]*matroskaAudioProbe{}
		videoProbes := map[uint64]*matroskaVideoProbe{}
		for _, stream := range info.Tracks {
			if id := streamTrackNumber(stream); id > 0 {
				switch stream.Kind {
				case StreamAudio:
					format := findField(stream.Fields, "Format")
					if format != "AC-3" && format != "E-AC-3" && format != "DTS" {
						continue
					}
					probe := &matroskaAudioProbe{
						format:      format,
						headerStrip: stream.mkvHeaderStripBytes,
					}
					if format == "E-AC-3" {
						probe.parseJOC = !stream.eac3Dec3.parsed || stream.eac3Dec3.hasJOC || stream.eac3Dec3.hasJOCComplex
						if stream.eac3Dec3.hasJOCComplex {
							probe.info.hasJOCComplex = true
							probe.info.jocComplexity = stream.eac3Dec3.jocComplexity
						}
						if stream.eac3Dec3.hasJOC {
							probe.info.hasJOC = true
						}
						probe.collect = true
						if opts.ParseSpeed < 1 {
							// Keep Matroska probing bounded (ParseSpeed < 1), but still sample enough
							// audio frames to match official JSON stats output (dialnorm/compr/JOC).
							probe.targetPackets = matroskaEAC3QuickProbePackets
							if probe.parseJOC {
								probe.jocStopPackets = matroskaEAC3QuickProbePacketsJOC
							}
							if stream.eac3Dec3.parsed && !stream.eac3Dec3.hasJOC && !stream.eac3Dec3.hasJOCComplex {
								probe.parseJOC = false
							}
						}
					}
					if format == "DTS" {
						// Header-only probe: grab core metadata from the first frame.
						if opts.ParseSpeed < 1 {
							probe.targetPackets = 1
						}
					}
					audioProbes[id] = probe
				case StreamVideo:
					format := findField(stream.Fields, "Format")
					if format == "AVC" {
						probe := &matroskaVideoProbe{
							codec:       format,
							headerStrip: stream.mkvHeaderStripBytes,
						}
						if opts.ParseSpeed < 1 {
							probe.targetPackets = matroskaAVCQuickProbePackets
						}
						videoProbes[id] = probe
						continue
					}
					if format == "HEVC" && stream.nalLengthSize > 0 {
						probe := &matroskaVideoProbe{
							codec:         format,
							nalLengthSize: stream.nalLengthSize,
							headerStrip:   stream.mkvHeaderStripBytes,
						}
						if opts.ParseSpeed < 1 {
							probe.targetPackets = matroskaHEVCQuickProbePackets
						}
						videoProbes[id] = probe
					}
				case StreamGeneral, StreamText, StreamImage, StreamMenu:
					continue
				}
			}
		}
		applyStats := shouldApplyMatroskaClusterStats(opts.ParseSpeed, size, info.tagStats, tagStatsComplete)
		applyCounts := shouldApplyMatroskaClusterCounts(opts.ParseSpeed, size, tagStatsComplete)
		applyScan := applyStats || applyCounts
		needsScan := applyScan || len(audioProbes) > 0 || len(videoProbes) > 0
		if needsScan {
			trackCount := 0
			for _, stream := range info.Tracks {
				if streamTrackNumber(stream) > 0 {
					trackCount++
				}
			}
			if stats, ok := scanMatroskaClusters(r, info.SegmentOffset, info.SegmentSize, info.TimecodeScale, audioProbes, videoProbes, applyScan, applyStats, opts.ParseSpeed, trackCount); ok {
				if applyScan {
					applyMatroskaStats(&info, stats, size)
				}
				applyMatroskaAudioProbes(&info, audioProbes)
				applyMatroskaVideoProbes(&info, videoProbes)
			}
		}
	}
	return info, true
}

func shouldApplyMatroskaClusterStats(parseSpeed float64, size int64, tagStats map[uint64]matroskaTagStats, tagStatsComplete bool) bool {
	// MediaInfo CLI default is metadata-first and very fast; per-track StreamSize/FrameCount
	// are usually sourced from Matroska Statistics Tags (mkvmerge) without a full Cluster pass.
	//
	// A full Cluster scan is extremely expensive on large files, so prefer Tags unless the
	// user asked for full parse speed.
	_ = size
	if tagStatsComplete {
		return false
	}
	if parseSpeed >= 1 {
		return true
	}
	return false
}

func shouldApplyMatroskaClusterCounts(parseSpeed float64, size int64, tagStatsComplete bool) bool {
	if parseSpeed >= 1 {
		return false
	}
	if tagStatsComplete {
		return false
	}
	return size > 0 && size <= mkvMaxCountsScan
}

func findMatroskaSeekPosition(buf []byte, segmentOffset int, targetID uint64) (uint64, bool) {
	if segmentOffset <= 0 || segmentOffset >= len(buf) {
		return 0, false
	}
	pos := segmentOffset
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDSeekHead {
			if seekPos, ok := parseMatroskaSeekHead(buf[dataStart:dataEnd], targetID); ok {
				return seekPos, true
			}
		}
		pos = dataEnd
	}
	return 0, false
}

func parseMatroskaSeekHead(buf []byte, targetID uint64) (uint64, bool) {
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDSeek {
			if seekID, seekPos, ok := parseMatroskaSeekEntry(buf[dataStart:dataEnd]); ok && seekID == targetID {
				return seekPos, true
			}
		}
		pos = dataEnd
	}
	return 0, false
}

func parseMatroskaSeekEntry(buf []byte) (uint64, uint64, bool) {
	var seekID uint64
	var seekPos uint64
	var hasID bool
	var hasPos bool
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDSeekID:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				seekID = value
				hasID = true
			}
		case mkvIDSeekPosition:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				seekPos = value
				hasPos = true
			}
		}
		pos = dataEnd
	}
	return seekID, seekPos, hasID && hasPos
}

func parseMatroska(buf []byte) (MatroskaInfo, bool) {
	pos := 0
	var headerFields []Field
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDEBML {
			headerFields = parseMatroskaHeader(buf[dataStart:dataEnd])
		}
		if id == mkvIDSegment {
			if info, ok := parseMatroskaSegment(buf[dataStart:dataEnd]); ok {
				if len(headerFields) > 0 {
					info.General = append(headerFields, info.General...)
				}
				info.SegmentOffset = int64(dataStart)
				if size != unknownVintSize {
					info.SegmentSize = int64(size)
				}
				return info, true
			}
		}
		pos = dataEnd
	}
	return MatroskaInfo{}, false
}

func parseMatroskaSegment(buf []byte) (MatroskaInfo, bool) {
	info := MatroskaInfo{}
	encodersByTrackUID := map[uint64]string{}
	settingsByTrackUID := map[uint64]string{}
	statsByTrackUID := map[uint64]matroskaTagStats{}
	var segmentFields []Field
	var chaptersPayloads [][]byte
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDInfo {
			if segInfo, ok := parseMatroskaInfo(buf[dataStart:dataEnd]); ok {
				info.Container.DurationSeconds = segInfo.Duration
				info.TimecodeScale = segInfo.TimecodeScale
				info.General = append(info.General, segInfo.Fields...)
			}
		}
		if id == mkvIDErrorDetection {
			if label := matroskaErrorDetectionLabel(buf[dataStart:dataEnd]); label != "" {
				segmentFields = append(segmentFields, Field{Name: "ErrorDetectionType", Value: label})
			}
		}
		if id == mkvIDTracks {
			if tracks, ok := parseMatroskaTracks(buf[dataStart:dataEnd], info.Container.DurationSeconds); ok {
				info.Tracks = append(info.Tracks, tracks...)
			}
		}
		if id == mkvIDTags {
			encodedDate := findField(info.General, "Encoded date")
			tagEncoders, tagSettings, tagStats := parseMatroskaTags(buf[dataStart:dataEnd], encodedDate)
			for uid, enc := range tagEncoders {
				if enc != "" {
					encodersByTrackUID[uid] = enc
				}
			}
			for uid, settings := range tagSettings {
				if settings != "" {
					settingsByTrackUID[uid] = settings
				}
			}
			for trackUID, stat := range tagStats {
				current := statsByTrackUID[trackUID]
				mergeMatroskaTagStats(&current, stat)
				statsByTrackUID[trackUID] = current
			}
		}
		if id == mkvIDAttachments {
			if names := parseMatroskaAttachments(buf[dataStart:dataEnd]); len(names) > 0 {
				info.attachments = append(info.attachments, names...)
			}
		}
		if id == mkvIDChapters {
			chaptersPayloads = append(chaptersPayloads, buf[dataStart:dataEnd])
		}
		pos = dataEnd
	}
	if (len(encodersByTrackUID) > 0 || len(settingsByTrackUID) > 0) && len(info.Tracks) > 0 {
		applyMatroskaEncoders(info.Tracks, encodersByTrackUID, settingsByTrackUID)
	}
	if len(segmentFields) > 0 {
		info.General = append(info.General, segmentFields...)
	}
	if len(statsByTrackUID) > 0 {
		info.tagStats = statsByTrackUID
	}
	if findField(info.General, "ErrorDetectionType") == "" && matroskaHasCRC(buf) {
		info.General = append(info.General, Field{Name: "ErrorDetectionType", Value: "Per level 1"})
	}
	if len(chaptersPayloads) > 0 {
		scale := info.TimecodeScale
		if scale == 0 {
			scale = 1000000
		}
		chapters := make([]matroskaChapter, 0, len(chaptersPayloads))
		for _, payload := range chaptersPayloads {
			chapters = append(chapters, parseMatroskaChapters(payload, scale)...)
		}
		if len(chapters) > 0 {
			menu := Stream{
				Kind:                StreamMenu,
				JSONRaw:             map[string]string{},
				JSONSkipStreamOrder: true,
				JSONSkipComputed:    true,
			}
			for i, chapter := range chapters {
				name := chapter.name
				if name == "" {
					name = fmt.Sprintf("Chapter %d", i+1)
				}
				if chapter.lang != "" {
					name = chapter.lang + ":" + name
				}
				menu.Fields = append(menu.Fields, Field{Name: formatMatroskaChapterTimeMs(chapter.startMs), Value: name})
			}
			menu.JSONRaw["extra"] = renderMatroskaMenuExtra(chapters)
			info.Tracks = append(info.Tracks, menu)
		}
	}
	if info.Container.HasDuration() || len(info.Tracks) > 0 {
		return info, true
	}
	return MatroskaInfo{}, false
}

func parseMatroskaHeader(buf []byte) []Field {
	pos := 0
	var fields []Field
	var docTypeVersion uint64
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDDocTypeVersion {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				docTypeVersion = value
			}
		}
		pos = dataEnd
	}
	if docTypeVersion > 0 {
		fields = append(fields, Field{Name: "Format version", Value: fmt.Sprintf("Version %d", docTypeVersion)})
	}
	return fields
}

type matroskaChapter struct {
	startMs int64
	name    string
	lang    string
}

func parseMatroskaChapters(buf []byte, timecodeScale uint64) []matroskaChapter {
	var chapters []matroskaChapter
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDEditionEntry {
			chapters = append(chapters, parseMatroskaEditionEntry(buf[dataStart:dataEnd], timecodeScale)...)
		}
		pos = dataEnd
	}
	return chapters
}

func parseMatroskaEditionEntry(buf []byte, _ uint64) []matroskaChapter {
	var chapters []matroskaChapter
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDChapterAtom {
			if chapter, ok := parseMatroskaChapterAtom(buf[dataStart:dataEnd]); ok {
				chapters = append(chapters, chapter)
			}
		}
		pos = dataEnd
	}
	return chapters
}

func parseMatroskaChapterAtom(buf []byte) (matroskaChapter, bool) {
	var chapter matroskaChapter
	var hasStart bool
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDChapterTimeStart:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				chapter.startMs = int64(value) / 1_000_000
				hasStart = true
			}
		case mkvIDChapterDisplay:
			if name, lang := parseMatroskaChapterDisplay(buf[dataStart:dataEnd]); name != "" {
				chapter.name = name
				if chapter.lang == "" {
					chapter.lang = lang
				}
			}
		}
		pos = dataEnd
	}
	if hasStart {
		return chapter, true
	}
	return matroskaChapter{}, false
}

func parseMatroskaChapterDisplay(buf []byte) (string, string) {
	pos := 0
	var name string
	var lang string
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDChapString:
			name = strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00")
		case mkvIDChapLanguage:
			lang = normalizeLanguageCode(strings.TrimSpace(string(buf[dataStart:dataEnd])))
		}
		pos = dataEnd
	}
	return name, lang
}

func formatMatroskaChapterTimeMs(msTotal int64) string {
	if msTotal < 0 {
		msTotal = 0
	}
	h := msTotal / (3600 * 1000)
	msTotal -= h * 3600 * 1000
	m := msTotal / (60 * 1000)
	msTotal -= m * 60 * 1000
	s := msTotal / 1000
	ms := msTotal - s*1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

func renderMatroskaMenuExtra(chapters []matroskaChapter) string {
	fields := make([]jsonKV, 0, len(chapters))
	for i, chapter := range chapters {
		name := chapter.name
		if name == "" {
			name = fmt.Sprintf("Chapter %d", i+1)
		}
		if chapter.lang != "" {
			name = chapter.lang + ":" + name
		}
		key := "_" + strings.NewReplacer(":", "_", ".", "_").Replace(formatMatroskaChapterTimeMs(chapter.startMs))
		fields = append(fields, jsonKV{Key: key, Val: name})
	}
	return renderJSONObject(fields, false)
}

func formatSegmentUID(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	value := new(big.Int).SetBytes(payload)
	hex := fmt.Sprintf("%X", payload)
	return fmt.Sprintf("%s (0x%s)", value.String(), hex)
}

type matroskaSegmentInfo struct {
	Duration      float64
	TimecodeScale uint64
	Fields        []Field
}

func parseMatroskaInfo(buf []byte) (matroskaSegmentInfo, bool) {
	timecodeScale := uint64(1000000)
	var durationValue float64
	var hasDuration bool
	var fields []Field

	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		payload := buf[dataStart:dataEnd]
		switch id {
		case mkvIDTimecodeScale:
			if value, ok := readUnsigned(payload); ok {
				timecodeScale = value
			}
		case mkvIDDuration:
			if value, ok := readFloat(payload); ok {
				durationValue = value
				hasDuration = true
			}
		case mkvIDSegmentUID:
			if len(payload) > 0 {
				fields = append(fields, Field{Name: "Unique ID", Value: formatSegmentUID(payload)})
			}
		case mkvIDWritingApp:
			if len(payload) > 0 {
				fields = append(fields, Field{Name: "Writing application", Value: string(payload)})
			}
		case mkvIDMuxingApp:
			if len(payload) > 0 {
				fields = append(fields, Field{Name: "Writing library", Value: string(payload)})
			}
		case mkvIDTitle:
			if len(payload) > 0 {
				fields = append(fields, Field{Name: "Movie name", Value: strings.TrimRight(string(payload), "\x00")})
			}
		case mkvIDDateUTC:
			if value, ok := readSigned(payload); ok {
				fields = append(fields, Field{Name: "Encoded date", Value: formatMatroskaDateUTC(value)})
			}
		case mkvIDErrorDetection:
			if label := matroskaErrorDetectionLabel(payload); label != "" {
				fields = append(fields, Field{Name: "ErrorDetectionType", Value: label})
			}
		}
		pos = dataEnd
	}

	if !hasDuration {
		return matroskaSegmentInfo{}, false
	}
	seconds := durationValue * float64(timecodeScale) / 1e9
	if seconds <= 0 {
		return matroskaSegmentInfo{}, false
	}
	return matroskaSegmentInfo{Duration: seconds, TimecodeScale: timecodeScale, Fields: fields}, true
}

func formatMatroskaDateUTC(deltaNs int64) string {
	base := time.Date(2001, time.January, 1, 0, 0, 0, 0, time.UTC)
	value := base.Add(time.Duration(deltaNs))
	return value.Format("2006-01-02 15:04:05 UTC")
}

func parseMatroskaTracks(buf []byte, segmentDuration float64) ([]Stream, bool) {
	entries := []Stream{}
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDTrackEntry {
			if stream, ok := parseMatroskaTrackEntry(buf[dataStart:dataEnd], segmentDuration); ok {
				entries = append(entries, stream)
			}
		}
		pos = dataEnd
	}
	return entries, len(entries) > 0
}

func parseMatroskaTrackEntry(buf []byte, segmentDuration float64) (Stream, bool) {
	pos := 0
	var trackType uint64
	var trackNumber uint64
	var trackUID uint64
	var trackName string
	var trackLanguage string
	var trackLanguageIETF string
	var trackOffset int64
	var hasTrackOffset bool
	var codecID string
	var codecPrivate []byte
	var codecName string
	var videoInfo matroskaVideoInfo
	var spsInfo h264SPSInfo
	var audioChannels uint64
	var audioSampleRate float64
	var audioBaseSampleRate float64
	var defaultDuration uint64
	var trackTSScale float64
	var hasTrackTSScale bool
	var bitRate uint64
	var flagDefault *bool
	var flagForced *bool
	var nalLengthSize int
	var hdrFormat string
	var dvCfg dolbyVisionConfig
	var hasDV bool
	var contentCompAlgo uint64
	var contentCompSettings []byte
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDTrackType {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				trackType = value
			}
		}
		if id == mkvIDTrackNumber {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				trackNumber = value
			}
		}
		if id == mkvIDTrackUID {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				trackUID = value
			}
		}
		if id == mkvIDTrackName {
			trackName = strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00")
		}
		if id == mkvIDTrackLanguage {
			trackLanguage = strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00")
		}
		if id == mkvIDTrackLanguageIETF {
			trackLanguageIETF = strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00")
		}
		if id == mkvIDCodecID {
			codecID = string(buf[dataStart:dataEnd])
		}
		if id == mkvIDCodecPrivate {
			codecPrivate = buf[dataStart:dataEnd]
		}
		if id == mkvIDCodecName {
			codecName = string(buf[dataStart:dataEnd])
		}
		if id == mkvIDContentEncodings {
			algo, settings, ok := parseMatroskaTrackCompression(buf[dataStart:dataEnd])
			if ok {
				contentCompAlgo = algo
				contentCompSettings = settings
			}
		}
		if id == mkvIDFlagDefault {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				v := value != 0
				flagDefault = &v
			}
		}
		if id == mkvIDFlagForced {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				v := value != 0
				flagForced = &v
			}
		}
		if id == mkvIDTrackOffset {
			if value, ok := readSigned(buf[dataStart:dataEnd]); ok {
				trackOffset = value
				hasTrackOffset = true
			}
		}
		if id == mkvIDBitRate {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				bitRate = value
			} else if value, ok := readFloat(buf[dataStart:dataEnd]); ok {
				bitRate = uint64(math.Round(value))
			}
		}
		if id == mkvIDDefaultDuration {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				defaultDuration = value
			}
		}
		if id == mkvIDTrackTimestampScale {
			if value, ok := readFloat(buf[dataStart:dataEnd]); ok && value > 0 {
				trackTSScale = value
				hasTrackTSScale = true
			}
		}
		if id == mkvIDTrackVideo {
			videoInfo = parseMatroskaVideo(buf[dataStart:dataEnd])
		}
		if id == mkvIDTrackAudio {
			channels, sampleRate, outputSampleRate := parseMatroskaAudio(buf[dataStart:dataEnd])
			if channels > 0 {
				audioChannels = channels
			}
			// For HE-AAC/SBR, Matroska may provide both base and output sample rates.
			// Prefer output for display, but keep base for frame rate/SPF decisions.
			if outputSampleRate > 0 {
				audioSampleRate = outputSampleRate
			} else if sampleRate > 0 {
				audioSampleRate = sampleRate
			}
			if sampleRate > 0 {
				audioBaseSampleRate = sampleRate
			}
		}
		pos = dataEnd
	}
	displayLanguage := trackLanguage
	if displayLanguage == "" {
		displayLanguage = trackLanguageIETF
	}
	kind, format := mapMatroskaCodecID(codecID, trackType)
	if kind == "" {
		return Stream{}, false
	}
	var dec3Info eac3Dec3Info
	if kind == StreamAudio && format == "E-AC-3" && len(codecPrivate) > 0 {
		if info, ok := parseEAC3Dec3(codecPrivate); ok || info.parsed {
			dec3Info = info
		}
	}
	aacProfile := ""
	aacObjType := 0
	aacSBRExplicitNo := false
	if kind == StreamAudio && format == "AAC" && len(codecPrivate) > 0 {
		aacProfile, aacObjType, aacSBRExplicitNo = parseAACProfileFromASC(codecPrivate)
		if aacProfile != "" {
			format = "AAC " + aacProfile
		}
		if codecID == "A_AAC" && aacObjType > 0 {
			codecID = fmt.Sprintf("A_AAC-%d", aacObjType)
		}
	}
	fields := []Field{{Name: "Format", Value: format}}
	if trackNumber > 0 {
		fields = append(fields, Field{Name: "ID", Value: strconv.FormatUint(trackNumber, 10)})
	}
	if codecID != "" {
		fields = append(fields, Field{Name: "Codec ID", Value: codecID})
	}
	if contentCompAlgo == 3 {
		fields = insertFieldBefore(fields, Field{Name: "Muxing mode", Value: "Header stripping"}, "Codec ID")
	}
	if codecID == "S_TEXT/UTF8" {
		fields = append(fields, Field{Name: "Codec ID/Info", Value: "UTF-8 Plain Text"})
	}
	if info := mapMatroskaFormatInfo(format); info != "" {
		fields = append(fields, Field{Name: "Format/Info", Value: info})
	}
	if kind == StreamAudio && format == "E-AC-3" {
		fields = append(fields, Field{Name: "Commercial name", Value: "Dolby Digital Plus"})
	}
	if kind == StreamAudio && format == "AC-3" {
		fields = append(fields, Field{Name: "Commercial name", Value: "Dolby Digital"})
	}
	if kind == StreamAudio && aacProfile == "LC" {
		fields = append(fields, Field{Name: "Format/Info", Value: "Advanced Audio Codec Low Complexity"})
	}
	if kind == StreamVideo && codecID == "V_MPEG4/ISO/AVC" && len(codecPrivate) > 0 {
		_, avcFields, avcInfo := parseAVCConfig(codecPrivate)
		fields = append(fields, avcFields...)
		spsInfo = avcInfo
	}
	if kind == StreamVideo && codecID == "V_MPEGH/ISO/HEVC" && len(codecPrivate) > 0 {
		_, hevcFields, hevcInfo, hevcSPS := parseHEVCConfig(codecPrivate)
		fields = append(fields, hevcFields...)
		nalLengthSize = hevcInfo.nalLengthSize
		if hevcSPS.Width > 0 || hevcSPS.CodedWidth > 0 || hevcSPS.ColorPrimaries != "" {
			spsInfo = hevcSPS
		}
		if dv := parseDolbyVisionConfigFromPrivate(codecPrivate); dv != "" {
			hdrFormat = dv
			if cfg, ok := parseDolbyVisionConfigFromPrivateRaw(codecPrivate); ok {
				dvCfg = cfg
				hasDV = true
			}
		}
		if hdrFormat == "" {
			if dv := parseDolbyVisionConfigFromPrivate(buf); dv != "" {
				hdrFormat = dv
				if cfg, ok := parseDolbyVisionConfigFromPrivateRaw(buf); ok {
					dvCfg = cfg
					hasDV = true
				}
			}
		}
	}
	if spsInfo.CodedWidth > 0 {
		videoInfo.codedWidth = spsInfo.CodedWidth
	}
	if spsInfo.CodedHeight > 0 {
		videoInfo.codedHeight = spsInfo.CodedHeight
	}
	// If container didn't specify DisplayWidth/DisplayHeight, prefer SPS visible dimensions.
	// This matches official mediainfo behavior for streams with coded size (e.g. 1920x1088)
	// and cropping to display (e.g. 1920x1080).
	if videoInfo.displayWidth == 0 && spsInfo.Width > 0 {
		videoInfo.displayWidth = spsInfo.Width
	}
	if videoInfo.displayHeight == 0 && spsInfo.Height > 0 {
		videoInfo.displayHeight = spsInfo.Height
	}
	if videoInfo.colorRange == "" && spsInfo.HasColorRange {
		videoInfo.colorRange = spsInfo.ColorRange
		videoInfo.colorRangeSource = "Stream"
	}
	if videoInfo.colorPrimaries == "" && spsInfo.ColorPrimaries != "" {
		videoInfo.colorPrimaries = spsInfo.ColorPrimaries
		videoInfo.colorPrimariesSource = "Stream"
	}
	if videoInfo.transferCharacteristics == "" && spsInfo.TransferCharacteristics != "" {
		videoInfo.transferCharacteristics = spsInfo.TransferCharacteristics
		videoInfo.transferSource = "Stream"
	}
	if videoInfo.matrixCoefficients == "" && spsInfo.MatrixCoefficients != "" {
		videoInfo.matrixCoefficients = spsInfo.MatrixCoefficients
		videoInfo.matrixSource = "Stream"
	}
	if kind == StreamVideo {
		bitRateNominal := spsInfo.HasBitRateCBR && spsInfo.BitRateCBR
		storedWidth := videoInfo.pixelWidth
		storedHeight := videoInfo.pixelHeight
		if videoInfo.codedWidth > 0 {
			storedWidth = videoInfo.codedWidth
		}
		if videoInfo.codedHeight > 0 {
			storedHeight = videoInfo.codedHeight
		}
		displayWidth := videoInfo.displayWidth
		displayHeight := videoInfo.displayHeight
		if videoInfo.displayUnit != 0 {
			displayWidth = 0
			displayHeight = 0
		}
		outWidth := storedWidth
		outHeight := storedHeight
		if displayWidth > 0 && displayHeight > 0 {
			outWidth = displayWidth
			outHeight = displayHeight
		}
		if outWidth > 0 {
			fields = append(fields, Field{Name: "Width", Value: formatPixels(outWidth)})
		}
		if outHeight > 0 {
			fields = append(fields, Field{Name: "Height", Value: formatPixels(outHeight)})
		}
		aspectW := storedWidth
		aspectH := storedHeight
		if displayWidth > 0 && displayHeight > 0 {
			aspectW = displayWidth
			aspectH = displayHeight
		}
		if ar := formatAspectRatio(aspectW, aspectH); ar != "" {
			fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
		}
		if defaultDuration > 0 {
			rate := 1e9 / float64(defaultDuration)
			fields = append(fields, Field{Name: "Frame rate mode", Value: "Constant"})
			fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRateWithRatio(rate)})
			if segmentDuration > 0 {
				frameDuration := float64(defaultDuration) / 1e9
				frameCount := math.Round(segmentDuration / frameDuration)
				if frameCount > 0 {
					fields = addStreamDuration(fields, frameCount*frameDuration)
				}
			}
		}
		if bitRate > 0 {
			// Treat Matroska TrackEntry BitRate as nominal (matches official JSON behavior).
			fields = append(fields, Field{Name: "Nominal bit rate", Value: formatBitrate(float64(bitRate))})
			// Only emit BitRate_Mode when the stream provides HRD mode signaling.
			// (Some files report BitRate but omit BitRate_Mode in official output.)
			if spsInfo.HasBitRateCBR {
				if bitRateNominal {
					fields = append(fields, Field{Name: "Bit rate mode", Value: "Constant"})
				} else {
					fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
				}
			}
			if defaultDuration > 0 && storedWidth > 0 && storedHeight > 0 {
				rate := 1e9 / float64(defaultDuration)
				if bits := formatBitsPerPixelFrame(float64(bitRate), storedWidth, storedHeight, rate); bits != "" {
					fields = append(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
				}
			}
		}
		if spsInfo.HasBitRateCBR && findField(fields, "Bit rate mode") == "" {
			if bitRateNominal {
				fields = append(fields, Field{Name: "Bit rate mode", Value: "Constant"})
			} else {
				fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
			}
		}
		if videoInfo.colorRange != "" && findField(fields, "Color range") == "" {
			fields = append(fields, Field{Name: "Color range", Value: videoInfo.colorRange})
		}
		if videoInfo.colorPrimaries != "" && findField(fields, "Color primaries") == "" {
			fields = append(fields, Field{Name: "Color primaries", Value: videoInfo.colorPrimaries})
		}
		if videoInfo.transferCharacteristics != "" && findField(fields, "Transfer characteristics") == "" {
			fields = append(fields, Field{Name: "Transfer characteristics", Value: videoInfo.transferCharacteristics})
		}
		if videoInfo.matrixCoefficients != "" && findField(fields, "Matrix coefficients") == "" {
			fields = append(fields, Field{Name: "Matrix coefficients", Value: videoInfo.matrixCoefficients})
		}
		if videoInfo.masteringPresent {
			if videoInfo.masteringPrimaries != "" && findField(fields, "Mastering display color primaries") == "" {
				fields = append(fields, Field{Name: "Mastering display color primaries", Value: videoInfo.masteringPrimaries})
			}
			if videoInfo.masteringLuminanceMax > 0 && videoInfo.masteringLuminanceMin > 0 && findField(fields, "Mastering display luminance") == "" {
				fields = append(fields, Field{Name: "Mastering display luminance", Value: formatMasteringLuminance(videoInfo.masteringLuminanceMin, videoInfo.masteringLuminanceMax)})
			}
		}
		if videoInfo.maxCLL > 0 && findField(fields, "Maximum Content Light Level") == "" {
			fields = append(fields, Field{Name: "Maximum Content Light Level", Value: fmt.Sprintf("%d cd/m2", videoInfo.maxCLL)})
		}
		if videoInfo.maxFALL > 0 && findField(fields, "Maximum Frame-Average Light Level") == "" {
			fields = append(fields, Field{Name: "Maximum Frame-Average Light Level", Value: fmt.Sprintf("%d cd/m2", videoInfo.maxFALL)})
		}
		if hdrFormat != "" && findField(fields, "HDR format") == "" {
			fields = insertFieldBefore(fields, Field{Name: "HDR format", Value: hdrFormat}, "Codec ID")
		}
		if findField(fields, "Color space") == "" {
			if codecID == "V_MPEG4/ISO/AVC" || codecID == "V_MPEGH/ISO/HEVC" {
				fields = append(fields, Field{Name: "Color space", Value: "YUV"})
			} else if videoInfo.colorRange != "" || videoInfo.colorPrimaries != "" || videoInfo.transferCharacteristics != "" || videoInfo.matrixCoefficients != "" {
				if matroskaHasStreamColor(videoInfo) {
					fields = append(fields, Field{Name: "Color space", Value: "YUV"})
				}
			}
		}
	}
	if kind == StreamAudio {
		if audioChannels > 0 {
			fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(audioChannels)})
			if layout := channelLayout(audioChannels); layout != "" {
				fields = append(fields, Field{Name: "Channel layout", Value: layout})
			}
		}
		if audioSampleRate > 0 {
			fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(audioSampleRate)})
			if format == "AAC LC" {
				spf := 1024.0
				// HE-AAC/SBR commonly reports base and output sample rates; MediaInfo uses 2048 SPF at output rate.
				if audioBaseSampleRate > 0 && audioSampleRate > audioBaseSampleRate {
					spf = 2048.0
				}
				frameRate := audioSampleRate / spf
				// Keep enough precision so duration from FrameCount/FrameRate matches official JSON rounding.
				fields = append(fields, Field{Name: "Frame rate", Value: fmt.Sprintf("%.4f FPS (%.0f SPF)", frameRate, spf)})
			}
		}
		if bitRate > 0 {
			fields = append(fields, Field{Name: "Bit rate mode", Value: "Constant"})
			fields = append(fields, Field{Name: "Bit rate", Value: formatBitrate(float64(bitRate))})
		}
		if segmentDuration > 0 {
			fields = addStreamDuration(fields, segmentDuration)
		}
		if format == "AAC LC" {
			fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
		}
		if codecName != "" && strings.Contains(codecName, "Lavc") {
			fields = append(fields, Field{Name: "Writing library", Value: codecName})
		}
	}
	defaultValue := true
	if flagDefault != nil {
		defaultValue = *flagDefault
	}
	if defaultValue {
		fields = append(fields, Field{Name: "Default", Value: "Yes"})
	} else {
		fields = append(fields, Field{Name: "Default", Value: "No"})
	}
	forcedValue := false
	if flagForced != nil {
		forcedValue = *flagForced
	}
	if forcedValue {
		fields = append(fields, Field{Name: "Forced", Value: "Yes"})
	} else {
		fields = append(fields, Field{Name: "Forced", Value: "No"})
	}
	languageCode := normalizeLanguageCode(trackLanguageIETF)
	if languageCode == "" {
		languageCode = normalizeLanguageCode(trackLanguage)
	}
	if language := formatLanguage(displayLanguage); language != "" {
		fields = insertFieldBefore(fields, Field{Name: "Language", Value: language}, "Default")
	}
	if trackName != "" {
		before := "Default"
		if languageCode != "" {
			before = "Language"
		}
		fields = insertFieldBefore(fields, Field{Name: "Title", Value: trackName}, before)
	}
	jsonExtras := map[string]string{}
	if trackUID > 0 {
		jsonExtras["UniqueID"] = strconv.FormatUint(trackUID, 10)
	}
	if bitRate > 0 && kind != StreamVideo {
		// Keep JSON BitRate exact; parsing from the formatted text field can introduce rounding drift.
		jsonExtras["BitRate"] = strconv.FormatUint(bitRate, 10)
	}
	if languageCode != "" {
		jsonExtras["Language"] = languageCode
	}
	if aacSBRExplicitNo {
		// Match official MediaInfo: only emit Format_Settings_SBR when AudioSpecificConfig explicitly signals it.
		jsonExtras["Format_Settings_SBR"] = "No (Explicit)"
	}
	delaySeconds := 0.0
	if hasTrackOffset {
		delaySeconds = float64(trackOffset) / 1e9
	}
	if kind == StreamVideo || kind == StreamAudio {
		delay := fmt.Sprintf("%.3f", delaySeconds)
		jsonExtras["Delay"] = delay
		jsonExtras["Delay_Source"] = "Container"
		if kind == StreamAudio {
			jsonExtras["Video_Delay"] = delay
		}
	}
	if kind == StreamVideo {
		storedWidth := videoInfo.pixelWidth
		storedHeight := videoInfo.pixelHeight
		if videoInfo.codedWidth > 0 {
			storedWidth = videoInfo.codedWidth
		}
		if videoInfo.codedHeight > 0 {
			storedHeight = videoInfo.codedHeight
		}
		displayWidth := videoInfo.displayWidth
		displayHeight := videoInfo.displayHeight
		if displayWidth == 0 && storedWidth > 0 {
			crop := videoInfo.cropLeft + videoInfo.cropRight
			if crop > 0 && crop < storedWidth {
				displayWidth = storedWidth - crop
			} else {
				displayWidth = storedWidth
			}
		}
		if displayHeight == 0 && storedHeight > 0 {
			crop := videoInfo.cropTop + videoInfo.cropBottom
			if crop > 0 && crop < storedHeight {
				displayHeight = storedHeight - crop
			} else {
				displayHeight = storedHeight
			}
		}
		if storedWidth > 0 && displayWidth > 0 && storedWidth != displayWidth {
			jsonExtras["Stored_Width"] = strconv.FormatUint(storedWidth, 10)
		}
		if storedHeight == displayHeight && displayHeight > 0 && codecID == "V_MPEG4/ISO/AVC" {
			if displayHeight%16 != 0 {
				storedHeight = ((displayHeight + 15) / 16) * 16
			}
		}
		if storedHeight > 0 && displayHeight > 0 && storedHeight != displayHeight {
			jsonExtras["Stored_Height"] = strconv.FormatUint(storedHeight, 10)
		}
		if spsInfo.HasFixedFrameRate && !spsInfo.FixedFrameRate {
			if findField(fields, "Frame rate mode") == "Constant" {
				jsonExtras["FrameRate_Mode_Original"] = "VFR"
			}
		}
		if videoInfo.colorRange != "" || videoInfo.colorPrimaries != "" || videoInfo.transferCharacteristics != "" || videoInfo.matrixCoefficients != "" {
			colorSource := "Container"
			hasStream := matroskaHasStreamColor(videoInfo)
			hasContainer := matroskaHasContainerColor(videoInfo)
			if hasStream && hasContainer {
				colorSource = "Container / Stream"
			} else if hasStream {
				colorSource = "Stream"
			}
			jsonExtras["colour_description_present"] = "Yes"
			jsonExtras["colour_description_present_Source"] = colorSource
			if videoInfo.colorRange != "" {
				jsonExtras["colour_range"] = videoInfo.colorRange
				jsonExtras["colour_range_Source"] = matroskaColorSource(videoInfo.colorRangeSource, colorSource)
			}
			if videoInfo.colorPrimaries != "" {
				jsonExtras["colour_primaries"] = videoInfo.colorPrimaries
				jsonExtras["colour_primaries_Source"] = matroskaColorSource(videoInfo.colorPrimariesSource, colorSource)
			}
			if videoInfo.transferCharacteristics != "" {
				jsonExtras["transfer_characteristics"] = videoInfo.transferCharacteristics
				jsonExtras["transfer_characteristics_Source"] = matroskaColorSource(videoInfo.transferSource, colorSource)
			}
			if videoInfo.matrixCoefficients != "" {
				jsonExtras["matrix_coefficients"] = videoInfo.matrixCoefficients
				jsonExtras["matrix_coefficients_Source"] = matroskaColorSource(videoInfo.matrixSource, colorSource)
			}
		}
		if spsInfo.HasBufferSize && spsInfo.BufferSize > 0 {
			jsonExtras["BufferSize"] = strconv.FormatInt(spsInfo.BufferSize, 10)
		}
		// For AVC, SPS HRD bit_rate_value represents the signaled max bitrate (vbv maxrate).
		// Official mediainfo maps this to BitRate_Maximum when present.
		if spsInfo.HasBitRate && spsInfo.BitRate > 0 {
			jsonExtras["BitRate_Maximum"] = strconv.FormatInt(spsInfo.BitRate, 10)
		}
	}
	durationSeconds := 0.0
	if kind == StreamVideo {
		if defaultDuration > 0 && segmentDuration > 0 {
			frameDuration := float64(defaultDuration) / 1e9
			frameCount := math.Round(segmentDuration / frameDuration)
			if frameCount > 0 {
				durationSeconds = frameCount * frameDuration
				jsonExtras["FrameCount"] = strconv.FormatInt(int64(frameCount), 10)
			}
		}
		if durationSeconds == 0 && segmentDuration > 0 {
			durationSeconds = segmentDuration
		}
	}
	if kind == StreamAudio && segmentDuration > 0 {
		durationSeconds = segmentDuration
	}
	if kind == StreamVideo && durationSeconds > 0 && hasTrackTSScale && trackTSScale > 0 {
		durationSeconds *= trackTSScale
	}
	if durationSeconds > 0 {
		if kind == StreamVideo {
			durationSeconds = math.Round(durationSeconds*1000) / 1000
		}
		jsonExtras["Duration"] = fmt.Sprintf("%.9f", durationSeconds)
	}
	headerStrip := []byte(nil)
	if contentCompAlgo == 3 && len(contentCompSettings) > 0 {
		headerStrip = append(headerStrip, contentCompSettings...)
	}
	return Stream{
		Kind:                kind,
		Fields:              fields,
		JSON:                jsonExtras,
		eac3Dec3:            dec3Info,
		nalLengthSize:       nalLengthSize,
		mkvHeaderStripBytes: headerStrip,
		mkvDolbyVision:      dvCfg,
		mkvHasDolbyVision:   hasDV,
	}, true
}

func parseMatroskaTrackCompression(buf []byte) (uint64, []byte, bool) {
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDContentEncoding {
			if algo, settings, ok := parseMatroskaContentEncoding(buf[dataStart:dataEnd]); ok {
				return algo, settings, true
			}
		}
		pos = dataEnd
	}
	return 0, nil, false
}

func parseMatroskaContentEncoding(buf []byte) (uint64, []byte, bool) {
	pos := 0
	var encodingType uint64
	var compAlgo uint64
	var compSettings []byte
	hasCompression := false
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDContentEncodingType:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				encodingType = value
			}
		case mkvIDContentCompression:
			algo, settings, ok := parseMatroskaContentCompression(buf[dataStart:dataEnd])
			if ok {
				compAlgo = algo
				compSettings = settings
				hasCompression = true
			}
		}
		pos = dataEnd
	}
	if encodingType != 0 || !hasCompression {
		return 0, nil, false
	}
	return compAlgo, compSettings, true
}

func parseMatroskaContentCompression(buf []byte) (uint64, []byte, bool) {
	pos := 0
	var compAlgo uint64
	var compSettings []byte
	hasAlgo := false
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDContentCompAlgo:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				compAlgo = value
				hasAlgo = true
			}
		case mkvIDContentCompSettings:
			compSettings = append(compSettings[:0], buf[dataStart:dataEnd]...)
		}
		pos = dataEnd
	}
	if !hasAlgo {
		return 0, nil, false
	}
	return compAlgo, compSettings, true
}

type matroskaVideoInfo struct {
	pixelWidth              uint64
	pixelHeight             uint64
	displayWidth            uint64
	displayHeight           uint64
	displayUnit             uint64
	aspectRatioType         uint64
	codedWidth              uint64
	codedHeight             uint64
	cropTop                 uint64
	cropBottom              uint64
	cropLeft                uint64
	cropRight               uint64
	colorRange              string
	colorRangeSource        string
	colorPrimaries          string
	colorPrimariesSource    string
	transferCharacteristics string
	transferSource          string
	matrixCoefficients      string
	matrixSource            string
	masteringPrimaries      string
	masteringLuminanceMin   float64
	masteringLuminanceMax   float64
	masteringPresent        bool
	maxCLL                  uint64
	maxFALL                 uint64
}

func parseMatroskaVideo(buf []byte) matroskaVideoInfo {
	info := matroskaVideoInfo{}
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDPixelWidth:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.pixelWidth = value
			}
		case mkvIDPixelHeight:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.pixelHeight = value
			}
		case mkvIDDisplayWidth:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.displayWidth = value
			}
		case mkvIDDisplayHeight:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.displayHeight = value
			}
		case mkvIDDisplayUnit:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.displayUnit = value
			}
		case mkvIDAspectRatioType:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.aspectRatioType = value
			}
		case mkvIDPixelCropTop:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.cropTop = value
			}
		case mkvIDPixelCropBottom:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.cropBottom = value
			}
		case mkvIDPixelCropLeft:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.cropLeft = value
			}
		case mkvIDPixelCropRight:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.cropRight = value
			}
		case mkvIDColour:
			colour := parseMatroskaColour(buf[dataStart:dataEnd])
			if colour.rangeValue != "" {
				info.colorRange = colour.rangeValue
				info.colorRangeSource = "Container"
			}
			if colour.primaries != "" {
				info.colorPrimaries = colour.primaries
				info.colorPrimariesSource = "Container"
			}
			if colour.transfer != "" {
				info.transferCharacteristics = colour.transfer
				info.transferSource = "Container"
			}
			if colour.matrix != "" {
				info.matrixCoefficients = colour.matrix
				info.matrixSource = "Container"
			}
			if colour.masteringPresent {
				info.masteringPresent = true
				info.masteringLuminanceMax = colour.masteringLuminanceMax
				info.masteringLuminanceMin = colour.masteringLuminanceMin
				info.masteringPrimaries = colour.masteringPrimaries
			}
			if colour.maxCLL > 0 {
				info.maxCLL = colour.maxCLL
			}
			if colour.maxFALL > 0 {
				info.maxFALL = colour.maxFALL
			}
		}
		pos = dataEnd
	}
	return info
}

func parseMatroskaAudio(buf []byte) (uint64, float64, float64) {
	pos := 0
	var channels uint64
	var sampleRate float64
	var outputSampleRate float64
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDChannels {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				channels = value
			}
		}
		if id == mkvIDSamplingRate {
			if value, ok := readFloat(buf[dataStart:dataEnd]); ok {
				sampleRate = value
			} else if valueInt, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				sampleRate = float64(valueInt)
			}
		}
		if id == mkvIDOutputSamplingRate {
			if value, ok := readFloat(buf[dataStart:dataEnd]); ok {
				outputSampleRate = value
			} else if valueInt, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				outputSampleRate = float64(valueInt)
			}
		}
		pos = dataEnd
	}
	return channels, sampleRate, outputSampleRate
}

type matroskaColourInfo struct {
	rangeValue            string
	primaries             string
	transfer              string
	matrix                string
	masteringPrimaries    string
	masteringLuminanceMin float64
	masteringLuminanceMax float64
	masteringPresent      bool
	maxCLL                uint64
	maxFALL               uint64
}

func parseMatroskaColour(buf []byte) matroskaColourInfo {
	info := matroskaColourInfo{}
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDRange:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				switch value {
				case 1:
					info.rangeValue = "Limited"
				case 2:
					info.rangeValue = "Full"
				}
			}
		case mkvIDColourPrimaries:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.primaries = matroskaColorPrimariesName(value)
			}
		case mkvIDTransferChar:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.transfer = matroskaTransferName(value)
			}
		case mkvIDMatrixCoeffs:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.matrix = matroskaMatrixName(value)
			}
		case mkvIDMasteringMetadata:
			mastering := parseMatroskaMasteringMetadata(buf[dataStart:dataEnd])
			if mastering.present {
				info.masteringPresent = true
				info.masteringLuminanceMax = mastering.luminanceMax
				info.masteringLuminanceMin = mastering.luminanceMin
			}
		case mkvIDMaxCLL:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.maxCLL = value
			}
		case mkvIDMaxFALL:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.maxFALL = value
			}
		}
		pos = dataEnd
	}
	if info.masteringPresent && info.primaries != "" {
		info.masteringPrimaries = info.primaries
	}
	return info
}

type matroskaMasteringInfo struct {
	luminanceMin float64
	luminanceMax float64
	present      bool
}

func parseMatroskaMasteringMetadata(buf []byte) matroskaMasteringInfo {
	info := matroskaMasteringInfo{}
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDMasteringLumMax:
			if value, ok := readFloat(buf[dataStart:dataEnd]); ok {
				info.luminanceMax = value
				info.present = true
			} else if valueInt, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.luminanceMax = float64(valueInt)
				info.present = true
			}
		case mkvIDMasteringLumMin:
			if value, ok := readFloat(buf[dataStart:dataEnd]); ok {
				info.luminanceMin = value
				info.present = true
			} else if valueInt, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.luminanceMin = float64(valueInt)
				info.present = true
			}
		case mkvIDMasteringPrimRx, mkvIDMasteringPrimRy, mkvIDMasteringPrimGx, mkvIDMasteringPrimGy, mkvIDMasteringPrimBx, mkvIDMasteringPrimBy, mkvIDMasteringWhiteX, mkvIDMasteringWhiteY:
			if _, ok := readFloat(buf[dataStart:dataEnd]); ok {
				info.present = true
			} else if _, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.present = true
			}
		}
		pos = dataEnd
	}
	return info
}

func matroskaColorPrimariesName(value uint64) string {
	switch value {
	case 1:
		return "BT.709"
	case 4:
		return "BT.470M"
	case 5:
		return "BT.470BG"
	case 6:
		return "SMPTE 170M"
	case 7:
		return "SMPTE 240M"
	case 8:
		return "Film"
	case 9:
		return "BT.2020"
	case 10:
		return "SMPTE ST 428-1"
	default:
		return ""
	}
}

func matroskaTransferName(value uint64) string {
	switch value {
	case 1:
		return "BT.709"
	case 4:
		return "BT.470M"
	case 5:
		return "BT.470BG"
	case 6:
		return "SMPTE 170M"
	case 7:
		return "SMPTE 240M"
	case 8:
		return "Linear"
	case 9:
		return "Log"
	case 10:
		return "Log Sqrt"
	case 11:
		return "IEC 61966-2-4"
	case 12:
		return "BT.1361"
	case 13:
		return "IEC 61966-2-1"
	case 14:
		return "BT.2020 10-bit"
	case 15:
		return "BT.2020 12-bit"
	case 16:
		return "PQ"
	case 17:
		return "SMPTE ST 428-1"
	case 18:
		return "HLG"
	default:
		return ""
	}
}

func matroskaMatrixName(value uint64) string {
	switch value {
	case 1:
		return "BT.709"
	case 4:
		return "FCC"
	case 5:
		return "BT.470BG"
	case 6:
		return "SMPTE 170M"
	case 7:
		return "SMPTE 240M"
	case 8:
		return "YCgCo"
	case 9:
		return "BT.2020 non-constant"
	case 10:
		return "BT.2020 constant"
	default:
		return ""
	}
}

func formatMasteringLuminance(minVal, maxVal float64) string {
	return fmt.Sprintf("min: %s cd/m2, max: %s cd/m2", formatHDRLuminance(minVal), formatHDRLuminance(maxVal))
}

func formatHDRLuminance(value float64) string {
	switch {
	case value >= 100:
		return fmt.Sprintf("%.0f", value)
	case value >= 10:
		return fmt.Sprintf("%.1f", value)
	case value >= 1:
		return fmt.Sprintf("%.2f", value)
	default:
		return fmt.Sprintf("%.4f", value)
	}
}

func matroskaErrorDetectionType(value uint64) string {
	switch value {
	case 1:
		return "Per level 1"
	case 2:
		return "Per level 2"
	case 3:
		return "Per level 3"
	default:
		return ""
	}
}

func matroskaErrorDetectionLabel(payload []byte) string {
	if value, ok := readUnsigned(payload); ok {
		if label := matroskaErrorDetectionType(value); label != "" {
			return label
		}
	}
	if len(payload) > 0 {
		return string(payload)
	}
	return ""
}

func matroskaHasCRC(buf []byte) bool {
	return matroskaScanForCRC(buf, 0, len(buf))
}

func matroskaScanForCRC(buf []byte, start int, end int) bool {
	pos := start
	for pos < end {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			return false
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			return false
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDCRC32 {
			return true
		}
		if matroskaIsMasterID(id) {
			if matroskaScanForCRC(buf, dataStart, dataEnd) {
				return true
			}
		}
		pos = dataEnd
	}
	return false
}

func matroskaIsMasterID(id uint64) bool {
	switch id {
	case mkvIDSegment, mkvIDInfo, mkvIDTracks, mkvIDTags, mkvIDTag, mkvIDSimpleTag,
		mkvIDTrackEntry, mkvIDTrackVideo, mkvIDTrackAudio, mkvIDColour, mkvIDCluster, mkvIDBlockGroup:
		return true
	default:
		return false
	}
}

func matroskaHasStreamColor(info matroskaVideoInfo) bool {
	return info.colorRangeSource == "Stream" ||
		info.colorPrimariesSource == "Stream" ||
		info.transferSource == "Stream" ||
		info.matrixSource == "Stream"
}

func matroskaHasContainerColor(info matroskaVideoInfo) bool {
	return info.colorRangeSource == "Container" ||
		info.colorPrimariesSource == "Container" ||
		info.transferSource == "Container" ||
		info.matrixSource == "Container"
}

func matroskaColorSource(value string, fallback string) string {
	if strings.Contains(fallback, "/") {
		return fallback
	}
	if value != "" {
		return value
	}
	return fallback
}

func parseMatroskaTags(buf []byte, encodedDate string) (map[uint64]string, map[uint64]string, map[uint64]matroskaTagStats) {
	encodersByTrackUID := map[uint64]string{}
	settingsByTrackUID := map[uint64]string{}
	statsByTrackUID := map[uint64]matroskaTagStats{}
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDTag {
			trackUID, tags := parseMatroskaTag(buf[dataStart:dataEnd])
			if encoder, ok := tags["ENCODER"]; ok && encoder != "" {
				key := trackUID
				if key == 0 {
					key = 0
				}
				if encodersByTrackUID[key] == "" {
					encodersByTrackUID[key] = encoder
				}
			}
			if settings, ok := tags["ENCODER_SETTINGS"]; ok && settings != "" {
				key := trackUID
				if key == 0 {
					key = 0
				}
				if settingsByTrackUID[key] == "" {
					settingsByTrackUID[key] = settings
				}
			}
			if trackUID > 0 {
				if stats, ok := parseMatroskaTagStats(tags, encodedDate); ok {
					current := statsByTrackUID[trackUID]
					mergeMatroskaTagStats(&current, stats)
					statsByTrackUID[trackUID] = current
				}
			}
		}
		pos = dataEnd
	}
	return encodersByTrackUID, settingsByTrackUID, statsByTrackUID
}

func parseMatroskaTagsFromBuffer(buf []byte, encodedDate string) (map[uint64]string, map[uint64]string, map[uint64]matroskaTagStats) {
	encodersByTrackUID := map[uint64]string{}
	settingsByTrackUID := map[uint64]string{}
	statsByTrackUID := map[uint64]matroskaTagStats{}
	pattern := []byte{0x12, 0x54, 0xC3, 0x67}
	searchPos := 0
	for searchPos+len(pattern) <= len(buf) {
		index := bytes.Index(buf[searchPos:], pattern)
		if index < 0 {
			break
		}
		start := searchPos + index
		size, sizeLen, ok := readVintSize(buf, start+len(pattern))
		if !ok || size == unknownVintSize {
			searchPos = start + 1
			continue
		}
		dataStart := start + len(pattern) + sizeLen
		dataEnd := dataStart + int(size)
		if dataStart >= len(buf) || dataEnd > len(buf) {
			searchPos = start + 1
			continue
		}
		tagEncoders, tagSettings, tagStats := parseMatroskaTags(buf[dataStart:dataEnd], encodedDate)
		for uid, enc := range tagEncoders {
			if enc != "" && encodersByTrackUID[uid] == "" {
				encodersByTrackUID[uid] = enc
			}
		}
		for uid, settings := range tagSettings {
			if settings != "" && settingsByTrackUID[uid] == "" {
				settingsByTrackUID[uid] = settings
			}
		}
		for trackUID, stat := range tagStats {
			current := statsByTrackUID[trackUID]
			mergeMatroskaTagStats(&current, stat)
			statsByTrackUID[trackUID] = current
		}
		searchPos = start + len(pattern)
	}
	return encodersByTrackUID, settingsByTrackUID, statsByTrackUID
}

func parseMatroskaTag(buf []byte) (uint64, map[string]string) {
	var trackUID uint64
	tags := map[string]string{}
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDTagTargets:
			if value := parseMatroskaTagTargets(buf[dataStart:dataEnd]); value > 0 {
				trackUID = value
			}
		case mkvIDSimpleTag:
			parseMatroskaSimpleTagTree(buf[dataStart:dataEnd], tags)
		}
		pos = dataEnd
	}
	return trackUID, tags
}

func parseMatroskaTagTargets(buf []byte) uint64 {
	var trackUID uint64
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDTagTrackUID {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				trackUID = value
			}
		}
		pos = dataEnd
	}
	return trackUID
}

func parseMatroskaAttachments(buf []byte) []string {
	var out []string
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDAttachedFile {
			name := parseMatroskaAttachedFile(buf[dataStart:dataEnd])
			if name != "" {
				out = append(out, name)
			}
		}
		pos = dataEnd
	}
	return out
}

func parseMatroskaAttachedFile(buf []byte) string {
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDFileName {
			return strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00")
		}
		pos = dataEnd
	}
	return ""
}

func scanMatroskaAttachmentsFromFile(r io.ReaderAt, offset int64, fileSize int64) []string {
	if r == nil || offset <= 0 || fileSize <= offset {
		return nil
	}
	// Attachments can be large (fonts). Use EBML seek skipping to avoid reading file data payloads.
	sr := io.NewSectionReader(r, offset, fileSize-offset)
	er := newEBMLReaderWithBufSize(sr, 256*1024)

	start := er.pos
	id, _, err := er.readVintID()
	if err != nil || id != mkvIDAttachments {
		return nil
	}
	elemSize, _, err := er.readVintSize()
	if err != nil {
		return nil
	}
	if elemSize == unknownVintSize {
		elemSize = uint64((fileSize - offset) - (er.pos - start))
	}
	end := er.pos + int64(elemSize)

	var out []string
	for er.pos < end {
		childID, _, err := er.readVintID()
		if err != nil {
			break
		}
		childSize, _, err := er.readVintSize()
		if err != nil {
			break
		}
		childStart := er.pos
		childEnd := childStart + int64(childSize)
		if childID != mkvIDAttachedFile {
			_ = er.skip(int64(childSize))
			continue
		}
		var name string
		for er.pos < childEnd {
			fid, _, err := er.readVintID()
			if err != nil {
				break
			}
			fsz, _, err := er.readVintSize()
			if err != nil {
				break
			}
			if fid == mkvIDFileName {
				if b, err := er.readN(int64(fsz)); err == nil {
					name = strings.TrimRight(string(b), "\x00")
				} else {
					break
				}
			} else {
				_ = er.skip(int64(fsz))
			}
		}
		if name != "" {
			out = append(out, name)
		}
		if er.pos < childEnd {
			_ = er.skip(childEnd - er.pos)
		}
	}
	return out
}

func parseMatroskaSimpleTagTree(buf []byte, tags map[string]string) {
	if tags == nil {
		return
	}
	var name string
	var value string
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		payload := buf[dataStart:dataEnd]
		if id == mkvIDSimpleTag {
			parseMatroskaSimpleTagTree(payload, tags)
		}
		if id == mkvIDTagName {
			name = string(payload)
		}
		if id == mkvIDTagString {
			value = string(payload)
		}
		pos = dataEnd
	}
	name = strings.TrimSpace(name)
	if name != "" && value != "" {
		if _, exists := tags[name]; !exists {
			tags[name] = value
		}
	}
}

func parseMatroskaTagStats(tags map[string]string, encodedDate string) (matroskaTagStats, bool) {
	if len(tags) == 0 {
		return matroskaTagStats{}, false
	}
	list := strings.Fields(tags["_STATISTICS_TAGS"])
	if len(list) == 0 {
		return matroskaTagStats{}, false
	}
	statsDateUTC := strings.TrimSpace(tags["_STATISTICS_WRITING_DATE_UTC"])
	if statsDateUTC != "" && !parseMatroskaStatsUTC(statsDateUTC) {
		return matroskaTagStats{}, false
	}
	headerUTC := strings.TrimSpace(strings.TrimSuffix(encodedDate, " UTC"))
	if before, _, ok := strings.Cut(headerUTC, " / "); ok {
		headerUTC = strings.TrimSpace(before)
	}
	statsUTC := strings.TrimSpace(strings.TrimSuffix(statsDateUTC, " UTC"))
	trusted := true
	if headerUTC != "" && statsUTC != "" {
		trusted = statsUTC >= headerUTC
	} else if headerUTC != "" && statsUTC == "" {
		trusted = false
	}
	if !trusted {
		return matroskaTagStats{}, false
	}
	out := matroskaTagStats{trusted: true}
	for _, key := range list {
		value := strings.TrimSpace(tags[key])
		if value == "" {
			continue
		}
		switch key {
		case "BPS":
			if parsed, ok := parseMatroskaTagInt(value); ok && parsed > 0 {
				out.bitRate = parsed
				out.hasBitRate = true
			}
		case "DURATION":
			if seconds, prec, ok := parseMatroskaStatisticsDuration(value); ok && seconds > 0 {
				out.durationSeconds = seconds
				out.durationPrec = prec
				out.hasDuration = true
			}
		case "NUMBER_OF_FRAMES":
			if parsed, ok := parseMatroskaTagInt(value); ok && parsed > 0 {
				out.frameCount = parsed
				out.hasFrameCount = true
			}
		case "NUMBER_OF_BYTES":
			if parsed, ok := parseMatroskaTagInt(value); ok && parsed > 0 {
				out.dataBytes = parsed
				out.hasDataBytes = true
			}
		}
	}
	if !out.hasBitRate && !out.hasDuration && !out.hasFrameCount && !out.hasDataBytes {
		return matroskaTagStats{}, false
	}
	return out, true
}

func parseMatroskaStatisticsDuration(value string) (float64, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 3 {
		return 0, 0, false
	}
	hours, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, 0, false
	}
	minutes, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, 0, false
	}
	secStr := strings.TrimSpace(parts[2])
	prec := 0
	if dot := strings.IndexByte(secStr, '.'); dot >= 0 && dot+1 < len(secStr) {
		prec = len(secStr) - dot - 1
		if prec < 0 {
			prec = 0
		}
		if prec > 9 {
			prec = 9
		}
	}
	seconds, err := strconv.ParseFloat(secStr, 64)
	if err != nil {
		return 0, 0, false
	}
	total := (hours * 60 * 60) + (minutes * 60) + seconds
	if total <= 0 {
		return 0, 0, false
	}
	return total, prec, true
}

func parseMatroskaStatsUTC(value string) bool {
	value = strings.TrimSpace(strings.TrimSuffix(value, " UTC"))
	if value == "" {
		return false
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.000000",
		"2006-01-02 15:04:05.000000000",
		time.RFC3339,
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

func matroskaStatsAppMatches(statsApp string, writingApp string, muxingApp string) bool {
	statsApp = strings.Join(strings.Fields(strings.ToLower(statsApp)), " ")
	if statsApp == "" {
		return false
	}
	if writingApp == "" && muxingApp == "" {
		return true
	}
	for _, candidate := range []string{writingApp, muxingApp} {
		candidate = strings.Join(strings.Fields(strings.ToLower(candidate)), " ")
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, statsApp) || strings.Contains(statsApp, candidate) {
			return true
		}
		statsTokens := strings.Fields(statsApp)
		candidateTokens := strings.Fields(candidate)
		if len(statsTokens) > 0 && len(candidateTokens) > 0 && statsTokens[0] == candidateTokens[0] {
			return true
		}
	}
	return false
}

func parseMatroskaTagInt(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		return parsed, true
	}
	floatValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return int64(math.Round(floatValue)), true
}

func mergeMatroskaTagStats(dst *matroskaTagStats, src matroskaTagStats) {
	if dst == nil {
		return
	}
	if src.trusted {
		dst.trusted = true
	}
	if src.hasBitRate {
		if !dst.hasBitRate || src.bitRate > dst.bitRate {
			dst.bitRate = src.bitRate
			dst.hasBitRate = true
		}
	}
	if src.hasDuration {
		if !dst.hasDuration || src.durationSeconds > dst.durationSeconds {
			dst.durationSeconds = src.durationSeconds
			dst.durationPrec = src.durationPrec
			dst.hasDuration = true
		} else if dst.hasDuration && src.durationSeconds == dst.durationSeconds && src.durationPrec > dst.durationPrec {
			dst.durationPrec = src.durationPrec
		}
	}
	if src.hasFrameCount {
		if !dst.hasFrameCount || src.frameCount > dst.frameCount {
			dst.frameCount = src.frameCount
			dst.hasFrameCount = true
		}
	}
	if src.hasDataBytes {
		if !dst.hasDataBytes || src.dataBytes > dst.dataBytes {
			dst.dataBytes = src.dataBytes
			dst.hasDataBytes = true
		}
	}
}

func applyMatroskaEncoders(streams []Stream, encodersByTrackUID map[uint64]string, settingsByTrackUID map[uint64]string) {
	if len(encodersByTrackUID) == 0 && len(settingsByTrackUID) == 0 {
		return
	}

	encList := make([]string, 0, len(encodersByTrackUID))
	for _, v := range encodersByTrackUID {
		if v != "" {
			encList = append(encList, v)
		}
	}

	for i := range streams {
		uid := streamTrackUID(streams[i])
		enc := encodersByTrackUID[uid]
		if enc == "" {
			enc = encodersByTrackUID[0]
		}
		settings := settingsByTrackUID[uid]
		if settings == "" {
			settings = settingsByTrackUID[0]
		}

		if streams[i].Kind == StreamVideo {
			lowerEnc := strings.ToLower(enc)
			// Avoid tagging muxing apps as a codec encoder; keep this conservative.
			isCodecEncoder := strings.Contains(lowerEnc, "x264") || strings.Contains(lowerEnc, "x265")
			if isCodecEncoder && enc != "" && findField(streams[i].Fields, "Writing library") == "" {
				streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Writing library", Value: enc})
			}
			if isCodecEncoder && settings != "" && findField(streams[i].Fields, "Encoding settings") == "" {
				streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Encoding settings", Value: settings})
			}
		}
	}

	// Audio: use any encoder hint when present (e.g. qaac) and fall back to a global AAC encoder token.
	audioEncoder := selectEncoder(encList, "aac")
	if audioEncoder == "" {
		audioEncoder = selectEncoder(encList, "qaac")
	}
	if audioEncoder != "" {
		for i := range streams {
			if streams[i].Kind != StreamAudio {
				continue
			}
			if findField(streams[i].Fields, "Writing library") != "" {
				continue
			}
			streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Writing library", Value: audioEncoder})
		}
	}
}

func selectEncoder(encoders []string, token string) string {
	token = strings.ToLower(token)
	for _, encoder := range encoders {
		lower := strings.ToLower(encoder)
		if strings.Contains(lower, token) {
			return encoder
		}
	}
	return ""
}

func parseAACProfileFromASC(payload []byte) (string, int, bool) {
	objType, sbrData, sbrPresent, ok := parseAACAudioSpecificConfig(payload)
	if !ok || objType <= 0 {
		return "", 0, false
	}
	return mapAACProfile(objType), objType, sbrData && !sbrPresent
}

func parseAACAudioSpecificConfig(payload []byte) (objType int, sbrData bool, sbrPresent bool, ok bool) {
	if len(payload) == 0 {
		return 0, false, false, false
	}
	br := newBitReader(payload)

	objType, ok = readAACAudioObjectType(br)
	if !ok {
		return 0, false, false, false
	}

	sfIndex := br.readBitsValue(4)
	if sfIndex == ^uint64(0) {
		return 0, false, false, false
	}
	if sfIndex == 0xF {
		if br.readBitsValue(24) == ^uint64(0) {
			return 0, false, false, false
		}
	}
	channelConfig := br.readBitsValue(4)
	if channelConfig == ^uint64(0) {
		return 0, false, false, false
	}

	extensionAudioObjectType := 0
	if objType == 5 || objType == 29 {
		extensionAudioObjectType = 5
		extIndex := br.readBitsValue(4)
		if extIndex == ^uint64(0) {
			return 0, false, false, false
		}
		if extIndex == 0xF {
			if br.readBitsValue(24) == ^uint64(0) {
				return 0, false, false, false
			}
		}
		next, ok := readAACAudioObjectType(br)
		if !ok {
			return 0, false, false, false
		}
		objType = next
		if objType == 22 {
			// extensionChannelConfiguration (4)
			if br.readBitsValue(4) == ^uint64(0) {
				return 0, false, false, false
			}
		}
	}

	if isAACGASpecificObjectType(objType) {
		if !skipAACGASpecificConfig(br, channelConfig, objType) {
			return objType, false, false, true
		}
	}

	// syncExtensionType (11) == 0x2b7 indicates explicit SBR/PS signaling.
	// MediaInfo emits "No (Explicit)" only when this extension is present and sbrPresentFlag == 0.
	if extensionAudioObjectType != 5 {
		savedPos := br.pos
		savedBit := br.bit
		syncExt := br.readBitsValue(11)
		if syncExt == 0x2b7 {
			sbrData = true
			extObjType, ok := readAACAudioObjectType(br)
			if !ok {
				return objType, false, false, true
			}
			switch extObjType {
			case 5:
				v := br.readBitsValue(1)
				if v == ^uint64(0) {
					return objType, false, false, true
				}
				sbrPresent = v == 1
			case 29:
				v := br.readBitsValue(1)
				if v == ^uint64(0) {
					return objType, false, false, true
				}
				sbrPresent = v == 1
				if br.readBitsValue(4) == ^uint64(0) { // extensionChannelConfiguration
					return objType, false, false, true
				}
			default:
				sbrData = false
			}
			return objType, sbrData, sbrPresent, true
		}
		// Not an extension: rewind so we don't consume bits unexpectedly.
		br.pos = savedPos
		br.bit = savedBit
	}

	return objType, false, false, true
}

func readAACAudioObjectType(br *bitReader) (int, bool) {
	v := br.readBitsValue(5)
	if v == ^uint64(0) {
		return 0, false
	}
	objType := int(v)
	if objType == 31 {
		ext := br.readBitsValue(6)
		if ext == ^uint64(0) {
			return 0, false
		}
		objType = 32 + int(ext)
	}
	return objType, true
}

func isAACGASpecificObjectType(objType int) bool {
	switch objType {
	case 1, 2, 3, 4, 6, 7, 17, 19, 20, 21, 22, 23:
		return true
	default:
		return false
	}
}

func skipAACGASpecificConfig(br *bitReader, channelConfig uint64, objType int) bool {
	if br.readBitsValue(1) == ^uint64(0) { // frameLengthFlag
		return false
	}
	depends := br.readBitsValue(1) // dependsOnCoreCoder
	if depends == ^uint64(0) {
		return false
	}
	if depends == 1 {
		if br.readBitsValue(14) == ^uint64(0) { // coreCoderDelay
			return false
		}
	}
	extFlag := br.readBitsValue(1) // extensionFlag
	if extFlag == ^uint64(0) {
		return false
	}
	// channelConfiguration == 0 implies a Program Config Element; not needed for our corpus.
	if channelConfig == 0 {
		return false
	}
	if objType == 6 || objType == 20 {
		if br.readBitsValue(3) == ^uint64(0) { // layerNr
			return false
		}
	}
	if extFlag == 1 {
		// Keep alignment for the most common extension cases.
		if objType == 22 {
			if br.readBitsValue(5) == ^uint64(0) || br.readBitsValue(11) == ^uint64(0) { // numOfSubFrame, layer_length
				return false
			}
		}
		switch objType {
		case 17, 19, 20, 21, 23:
			if br.readBitsValue(1) == ^uint64(0) || br.readBitsValue(1) == ^uint64(0) || br.readBitsValue(1) == ^uint64(0) {
				return false
			}
		}
		if br.readBitsValue(1) == ^uint64(0) { // extensionFlag3
			return false
		}
	}
	return true
}

const unknownVintSize = ^uint64(0)

func readVintID(buf []byte, pos int) (uint64, int, bool) {
	if pos >= len(buf) {
		return 0, 0, false
	}
	first := buf[pos]
	length := vintLength(first)
	if length == 0 || pos+length > len(buf) {
		return 0, 0, false
	}
	var value uint64
	for i := range length {
		value = (value << 8) | uint64(buf[pos+i])
	}
	return value, length, true
}

func readVintSize(buf []byte, pos int) (uint64, int, bool) {
	if pos >= len(buf) {
		return 0, 0, false
	}
	first := buf[pos]
	length := vintLength(first)
	if length == 0 || pos+length > len(buf) {
		return 0, 0, false
	}
	mask := byte(0xFF >> length)
	value := uint64(first & mask)
	for i := 1; i < length; i++ {
		value = (value << 8) | uint64(buf[pos+i])
	}
	if value == (uint64(1)<<(uint(length*7)))-1 {
		return unknownVintSize, length, true
	}
	return value, length, true
}

func vintLength(first byte) int {
	for i := range 8 {
		if first&(1<<(7-uint(i))) != 0 {
			return i + 1
		}
	}
	return 0
}

func readUnsigned(buf []byte) (uint64, bool) {
	if len(buf) == 0 || len(buf) > 8 {
		return 0, false
	}
	var value uint64
	for _, b := range buf {
		value = (value << 8) | uint64(b)
	}
	return value, true
}

func readSigned(buf []byte) (int64, bool) {
	if len(buf) == 0 || len(buf) > 8 {
		return 0, false
	}
	var value int64
	for _, b := range buf {
		value = (value << 8) | int64(b)
	}
	if buf[0]&0x80 != 0 {
		value -= 1 << (uint(len(buf)) * 8)
	}
	return value, true
}

func readFloat(buf []byte) (float64, bool) {
	if len(buf) == 4 {
		bits := binary.BigEndian.Uint32(buf)
		return float64(math.Float32frombits(bits)), true
	}
	if len(buf) == 8 {
		bits := binary.BigEndian.Uint64(buf)
		return math.Float64frombits(bits), true
	}
	return 0, false
}
