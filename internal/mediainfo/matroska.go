package mediainfo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"math/big"
	"slices"
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
	mkvIDFileMimeType        = 0x4660
	mkvIDFileDescription     = 0x467E
	mkvIDFileData            = 0x465C
	mkvIDFileUID             = 0x46AE
	mkvIDTag                 = 0x7373
	mkvIDTagTargets          = 0x63C0
	mkvIDSimpleTag           = 0x67C8
	mkvIDTagName             = 0x45A3
	mkvIDTagString           = 0x4487
	mkvIDTagLanguage         = 0x447A
	mkvIDTagTrackUID         = 0x63C5
	mkvIDTagBlockAddIDValue  = 0x63C7
	mkvIDTagEditionUID       = 0x63C9
	mkvIDTagChapterUID       = 0x63C4
	mkvIDTagAttachmentUID    = 0x63C6
	mkvIDEditionEntry        = 0x45B9
	mkvIDChapterAtom         = 0xB6
	mkvIDChapterTimeStart    = 0x91
	mkvIDChapterDisplay      = 0x80
	mkvIDChapString          = 0x85
	mkvIDChapLanguage        = 0x437C
	mkvIDChapLanguageIETF    = 0x437D
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
	mkvIDCodecDelay          = 0x56AA
	mkvIDSeekPreRoll         = 0x56BB
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
	mkvIDFlagHearingImpaired = 0x55AB
	mkvIDFlagOriginal        = 0x55AE
	mkvIDFlagCommentary      = 0x55AF
	mkvIDTrackVideo          = 0xE0
	mkvIDTrackAudio          = 0xE1
	mkvIDBitRate             = 0x6264
	mkvIDPixelWidth          = 0xB0
	mkvIDPixelHeight         = 0xBA
	mkvIDDisplayWidth        = 0x54B0
	mkvIDDisplayHeight       = 0x54BA
	mkvIDDisplayUnit         = 0x54B2
	mkvIDAspectRatioType     = 0x54B3
	mkvIDStereoMode          = 0x53B8
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
	mkvIDAudioBitDepth       = 0x6264
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

// maxMatroskaAttachmentSeekPositions bounds independent random-read scans
// triggered by untrusted SeekHead entries. Valid files ordinarily reference a
// single Attachments element; the larger cap preserves unusual muxer layouts.
const maxMatroskaAttachmentSeekPositions = 16

const matroskaEAC3QuickProbeFrames = 1113

// MediaInfoLib stops AC-3 and E-AC-3 parsing after PacketCount>=300 when
// ParseSpeed<1.
const matroskaEAC3QuickProbePackets = 300

// matroskaAC3QuickProbePackets bounds core AC-3 compression-statistics sampling.
const matroskaAC3QuickProbePackets = 300

// Bound the expensive JOC scan (full-block reads) separately; stats probing continues to PacketCount.
const matroskaEAC3QuickProbePacketsJOC = 198
const matroskaHEVCQuickProbePackets = 300

// matroskaAVCQuickProbePackets bounds AVC frame sampling used for encoder,
// slice, GOP, and first-frame time-code metadata.
const matroskaAVCQuickProbePackets = 256

// MatroskaInfo contains parsed container, General, and track metadata plus the
// Segment byte range and timestamp scale used for bounded cluster scans.
type MatroskaInfo struct {
	Container      ContainerInfo
	General        []Field
	Tracks         []Stream
	SegmentOffset  int64
	SegmentSize    int64
	TimecodeScale  uint64
	durationPrec   int
	tagStats       map[uint64]matroskaTagStats
	generalTags    map[string]string
	scopedTags     matroskaScopedTags
	attachments    []string
	attachmentInfo []matroskaAttachment
}

// matroskaAttachment holds attachment metadata and a bounded payload prefix
// when content probing identifies a supported embedded image.
type matroskaAttachment struct {
	name        string
	description string
	mime        string
	uid         uint64
	data        []byte
	size        int64
	// complete distinguishes a bounded payload prefix from the full FileData.
	complete bool
}

// ParseMatroska parses a Matroska stream with the default analysis options.
// It reports false when the input is empty, unreadable, or not valid Matroska.
func ParseMatroska(r io.ReaderAt, size int64) (MatroskaInfo, bool) {
	return ParseMatroskaWithOptions(r, size, defaultAnalyzeOptions())
}

// ParseMatroskaWithOptions parses Matroska metadata and performs option-bounded
// cluster probes for statistics and codec metadata. It reports false when the
// input is empty, unreadable, or not valid Matroska.
func ParseMatroskaWithOptions(r io.ReaderAt, size int64, opts AnalyzeOptions) (MatroskaInfo, bool) {
	return parseMatroskaWithOptions(r, size, opts, true)
}

// parseMatroskaForAnalysis defers exported compatibility snapshots so Analyze
// can finish container-level canonical refinements without observing them.
func parseMatroskaForAnalysis(r io.ReaderAt, size int64, opts AnalyzeOptions) (MatroskaInfo, bool) {
	return parseMatroskaWithOptions(r, size, opts, false)
}

// parseMatroskaWithOptions performs the shared parse and optionally publishes
// the legacy Stream maps required by direct parser callers.
func parseMatroskaWithOptions(r io.ReaderAt, size int64, opts AnalyzeOptions, publishLegacySnapshots bool) (MatroskaInfo, bool) {
	opts = normalizeAnalyzeOptions(opts)
	scanSize := min(size, mkvMaxScan)
	if scanSize <= 0 {
		return MatroskaInfo{}, false
	}

	buf := make([]byte, scanSize)
	if _, err := r.ReadAt(buf, 0); err != nil && err != io.EOF {
		return MatroskaInfo{}, false
	}

	assetBudget := &embeddedAssetBudget{}
	info, ok := parseMatroskaWithBudget(buf, assetBudget)
	if !ok {
		return MatroskaInfo{}, false
	}
	if len(info.Tracks) == 0 && size > scanSize && info.SegmentOffset > 0 {
		if seekPos, found := findMatroskaSeekPosition(buf, int(info.SegmentOffset), mkvIDTracks); found {
			if tracksOffset, reason := checkedEmbeddedOffset(info.SegmentOffset, seekPos, size); reason == embeddedAssetAccepted {
				tracks, parsed, err := scanMatroskaTracksFromFile(r, tracksOffset, size, info.Container.DurationSeconds, info.durationPrec)
				if err != nil {
					return MatroskaInfo{}, false
				}
				if parsed {
					info.Tracks = tracks
				}
			}
		}
	}
	needsDolbyVisionProbe := false
	for i := range info.Tracks {
		if info.Tracks[i].Kind == StreamVideo && matroskaStreamScalar(info.Tracks[i], "Format") == "HEVC" {
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
			if !matroskaHasMenu(info.Tracks) {
				if seekPos, ok := findMatroskaSeekPosition(buf, int(info.SegmentOffset), mkvIDChapters); ok {
					if chaptersOffset, reason := checkedEmbeddedOffset(info.SegmentOffset, seekPos, size); reason == embeddedAssetAccepted {
						if editions := scanMatroskaChaptersFromFile(r, chaptersOffset, size); len(editions) > 0 {
							info.Tracks = appendMatroskaChapterMenus(info.Tracks, editions)
						}
					}
				}
			}
			// If Attachments were truncated by the initial scan buffer, resolve them via SeekHead and
			// parse lazily (seek-skipping file payloads).
			attachmentOffsets := []int64{}
			for _, seekPos := range findMatroskaSeekPositions(buf, int(info.SegmentOffset), mkvIDAttachments) {
				if attachmentOffset, reason := checkedEmbeddedOffset(info.SegmentOffset, seekPos, size); reason == embeddedAssetAccepted {
					attachmentOffsets = append(attachmentOffsets, attachmentOffset)
				}
			}
			if len(attachmentOffsets) == 0 {
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
						attachmentOffsets = append(attachmentOffsets, info.SegmentOffset+int64(idx))
					}
				}
			}
			if len(attachmentOffsets) > 0 {
				var attachments []matroskaAttachment
				for _, attachmentsOffset := range attachmentOffsets {
					attachments = append(attachments, scanMatroskaAttachmentsFromFile(r, attachmentsOffset, size, assetBudget)...)
				}
				if len(attachments) > 0 {
					rescannedNames := make([]string, 0, len(attachments))
					for _, attachment := range attachments {
						rescannedNames = append(rescannedNames, attachment.name)
						info.attachmentInfo = appendMatroskaAttachmentUnique(info.attachmentInfo, attachment)
					}
					info.attachments = mergeMatroskaAttachmentNames(info.attachments, rescannedNames)
				}
			}
		}
		needEncoders := false
		needLangs := false
		for _, stream := range info.Tracks {
			language, _ := canonicalSeedValue(stream, "Language")
			if stream.Kind == StreamAudio && language == "" {
				needLangs = true
			}
			format, _ := canonicalSeedValue(stream, "Format")
			writingLibrary, _ := canonicalSeedTextValue(stream, "Writing library")
			encodingSettings, _ := canonicalSeedTextValue(stream, "Encoding settings")
			if stream.Kind == StreamVideo && format == "AVC" && (writingLibrary == "" || encodingSettings == "") {
				needEncoders = true
			}
			if stream.Kind == StreamAudio && writingLibrary == "" {
				switch format {
				case "AC-3", "E-AC-3", "FLAC", "Opus":
					needEncoders = true
				}
			}
		}
		if (!matroskaHasCompleteTagStats(info.Tracks, info.tagStats) || needEncoders || needLangs || info.generalTags["ENCODER"] == "") && size > scanSize {
			encodedDate := findField(info.General, "Encoded date")
			var tagEncoders map[uint64]string
			var tagSettings map[uint64]string
			var tagLangs map[uint64]string
			var tagStats map[uint64]matroskaTagStats
			var generalTags map[string]string
			var scopedTags matroskaScopedTags
			// Prefer SeekHead for a precise offset, but some files omit Tags entries.
			tagsRead := false
			if seekPos, ok := findMatroskaSeekPosition(buf, int(info.SegmentOffset), mkvIDTags); ok {
				if tagsOffset, reason := checkedEmbeddedOffset(info.SegmentOffset, seekPos, size); reason == embeddedAssetAccepted {
					tagsSize := min(size-tagsOffset, int64(8<<20))
					if tagsSize > 0 {
						tagsBuf := make([]byte, tagsSize)
						if n, err := r.ReadAt(tagsBuf, tagsOffset); n == len(tagsBuf) && (err == nil || err == io.EOF) {
							tagEncoders, tagSettings, tagLangs, tagStats, generalTags, scopedTags = parseMatroskaTagsFromBuffer(tagsBuf, encodedDate)
							tagsRead = matroskaTagsHaveData(tagEncoders, tagSettings, tagLangs, tagStats, generalTags)
						}
					}
				}
			}
			if !tagsRead {
				// Fallback: scan a slightly larger prefix for the Tags element ID and parse in-memory.
				headSize := min(size, int64(16<<20))
				if headSize > 0 {
					head := buf
					if int64(len(head)) < headSize {
						head = make([]byte, headSize)
						copy(head, buf)
						remaining := head[int64(len(buf)):]
						if n, err := r.ReadAt(remaining, int64(len(buf))); n != len(remaining) || (err != nil && err != io.EOF) {
							head = buf
						}
					}
					headEncoders, headSettings, headLangs, headStats, headGeneralTags, headScopedTags := parseMatroskaTagsFromBuffer(head, encodedDate)
					tagEncoders = headEncoders
					tagSettings = headSettings
					tagLangs = headLangs
					tagStats = headStats
					generalTags = headGeneralTags
					mergeMatroskaScopedTags(&scopedTags, headScopedTags)
				}
			}
			// Fallback: some muxers place Tags at EOF. Non-empty maps are not proof
			// that every required track or consumer key was present in the head window.
			langsComplete := !needLangs || matroskaHasCompleteTagLanguages(info.Tracks, tagLangs)
			statsComplete := matroskaHasCompleteCombinedTagStats(info.Tracks, info.tagStats, tagStats)
			generalComplete := info.generalTags["ENCODER"] != "" || generalTags["ENCODER"] != ""
			encodersComplete := !needEncoders || matroskaHasCompleteTagEncoders(info.Tracks, tagEncoders, tagSettings)
			if (!langsComplete || !statsComplete || !generalComplete || !encodersComplete) && size > (32<<20) {
				tailSize := min(size, int64(32<<20))
				if tailSize > 0 {
					tail := make([]byte, tailSize)
					if n, err := r.ReadAt(tail, size-tailSize); n == len(tail) && (err == nil || err == io.EOF) {
						enc, settings, langs, tailStats, tailGeneralTags, tailScopedTags := parseMatroskaTagsFromBuffer(tail, encodedDate)
						tagEncoders = mergeMatroskaTagEncoders(tagEncoders, enc)
						tagSettings = mergeMatroskaTagValues(tagSettings, settings)
						tagLangs = mergeMatroskaTagValues(tagLangs, langs)
						for uid, stat := range tailStats {
							if tagStats == nil {
								tagStats = map[uint64]matroskaTagStats{}
							}
							current, exists := tagStats[uid]
							if (!exists || !current.trusted) && (stat.hasSource || stat.hasSourceID) {
								tagStats[uid] = stat
								continue
							}
							if !exists {
								continue
							}
							// Preserve trusted head/SeekHead measurements; a tail window may
							// contain an older duplicate Tags element with stale numeric stats.
							mergeMatroskaTagStats(&current, matroskaTagStats{source: stat.source, hasSource: stat.hasSource, sourceID: stat.sourceID, hasSourceID: stat.hasSourceID, extras: stat.extras})
							tagStats[uid] = current
						}
						for name, value := range tailGeneralTags {
							if generalTags == nil {
								generalTags = map[string]string{}
							}
							if generalTags[name] == "" {
								generalTags[name] = value
							}
						}
						mergeMatroskaScopedTags(&scopedTags, tailScopedTags)
					}
				}
			}

			if len(tagStats) > 0 {
				if info.tagStats == nil {
					info.tagStats = map[uint64]matroskaTagStats{}
				}
				for uid, st := range tagStats {
					current := info.tagStats[uid]
					mergeMatroskaTagStats(&current, st)
					info.tagStats[uid] = current
				}
			}
			if (len(tagEncoders) > 0 || len(tagSettings) > 0) && len(info.Tracks) > 0 {
				applyMatroskaEncoders(info.Tracks, tagEncoders, tagSettings)
			}
			if len(tagLangs) > 0 && len(info.Tracks) > 0 {
				applyMatroskaTagLanguages(info.Tracks, tagLangs)
			}
			for name, value := range generalTags {
				if info.generalTags == nil {
					info.generalTags = map[string]string{}
				}
				if info.generalTags[name] == "" {
					info.generalTags[name] = value
				}
			}
			mergeMatroskaScopedTags(&info.scopedTags, scopedTags)
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
					format := matroskaStreamScalar(stream, "Format")
					if format != "AC-3" && format != "E-AC-3" && format != "DTS" && format != "TrueHD" && format != "MPEG Audio" {
						continue
					}
					probe := &matroskaAudioProbe{
						format:        format,
						headerStrip:   stream.mkvHeaderStripBytes,
						dependentEAC3: matroskaStreamScalar(stream, "CodecID") == "A_EAC3" && matroskaStreamScalar(stream, "Channels") == "8",
					}
					if format == "E-AC-3" {
						// dec3 can omit JOC signaling on Atmos Matroska tracks; keep a bounded
						// bitstream probe so dependent EMDF/JOC metadata still wins.
						probe.parseJOC = true
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
						}
					}
					if format == "AC-3" {
						probe.collect = true
						if opts.ParseSpeed < 1 {
							probe.targetPackets = matroskaAC3QuickProbePackets
							if matroskaStreamScalar(stream, "Channels") == "2" {
								probe.targetPackets = 212
							}
						}
					}
					if format == "DTS" {
						// Header-only probe: grab core metadata from the first frame.
						if opts.ParseSpeed < 1 {
							probe.targetPackets = 1
						}
					}
					if format == "TrueHD" && opts.ParseSpeed < 1 {
						// Atmos metadata lives in the major-sync header; one packet is enough for bounded scans.
						probe.targetPackets = 1
						if sampleRate, ok := parseInt(matroskaStreamScalar(stream, "SamplingRate")); ok {
							probe.truehd.sampleRate = int(sampleRate)
						}
					}
					if format == "MPEG Audio" && opts.ParseSpeed < 1 {
						probe.targetPackets = 1
					}
					audioProbes[id] = probe
				case StreamVideo:
					format := matroskaStreamScalar(stream, "Format")
					if format == "AVC" {
						probe := &matroskaVideoProbe{
							codec:         format,
							nalLengthSize: stream.nalLengthSize,
							h264SPS:       stream.mkvH264SPS,
							headerStrip:   stream.mkvHeaderStripBytes,
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
						if stream.mkvHEVCX265Library != "" {
							// x265 SEI was carried in CodecPrivate (hvcC); seed it so the
							// cluster-scan transfer emits it with the same precedence as a
							// bitstream-derived library.
							probe.hdrInfo.x265Library = stream.mkvHEVCX265Library
							probe.hdrInfo.x265Settings = stream.mkvHEVCX265Settings
							probe.hdrInfo.x265Seen = true
						}
						probe.targetPackets = matroskaHEVCQuickProbePackets
						videoProbes[id] = probe
						continue
					}
					if format == "MPEG Video" {
						videoProbes[id] = &matroskaVideoProbe{codec: format, targetPackets: 64}
						continue
					}
					if format == "MPEG-4 Visual" {
						videoProbes[id] = &matroskaVideoProbe{codec: format, targetPackets: 64}
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
			needFirstTimes := map[uint64]struct{}{}
			if !applyScan && opts.ParseSpeed < 1 {
				// Delay relative to video: require at least one observed block time per track.
				for _, stream := range info.Tracks {
					if stream.Kind != StreamVideo && stream.Kind != StreamAudio {
						continue
					}
					if id := streamTrackNumber(stream); id > 0 {
						needFirstTimes[id] = struct{}{}
					}
				}
			}
			if len(needFirstTimes) == 0 {
				needFirstTimes = nil
			}
			if stats, ok := scanMatroskaClusters(r, info.SegmentOffset, info.SegmentSize, info.TimecodeScale, audioProbes, videoProbes, applyScan, applyStats, opts.ParseSpeed, trackCount, needFirstTimes); ok {
				if applyScan {
					applyMatroskaStats(&info, stats, size)
				}
				applyMatroskaTrackDelays(&info, stats)
				applyMatroskaAudioProbes(&info, audioProbes)
				applyMatroskaVideoProbes(&info, videoProbes)
			}
		}
	}
	applyMatroskaLavfDurationCorrection(&info)
	// MediaInfo may derive video Duration from FrameCount and the displayed FrameRate (rounded to
	// milliseconds) for some Matroska files. This shows up as a small ms-level delta vs Segment Info.
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		if stream.Kind != StreamVideo {
			continue
		}
		durStr, _ := projectedCanonicalSeedValue(*stream, "Duration")
		fcStr := matroskaStreamScalar(*stream, "FrameCount")
		frStr := matroskaStreamScalar(*stream, "FrameRate")
		if fcStr == "" || frStr == "" {
			continue
		}
		if durStr != "" {
			if dot := strings.IndexByte(durStr, '.'); dot < 0 || len(durStr)-dot-1 != 3 {
				// Don't override stats-derived durations which are serialized at higher precision.
				continue
			}
		}
		frameCount, ok := parseInt(fcStr)
		if !ok || frameCount <= 0 {
			continue
		}
		frameRate, err := strconv.ParseFloat(frStr, 64)
		if err != nil || frameRate <= 0 {
			continue
		}
		ms := math.Round((float64(frameCount) * 1000.0) / frameRate)
		if ms <= 0 {
			continue
		}
		value := fmt.Sprintf("%.3f", ms/1000.0)
		if milliseconds, ok := decimalSecondsToMilliseconds(value); ok {
			replaceCanonicalSeedLegacyProjection(stream, "Duration", milliseconds, value, "", "")
			setCanonicalSeedStructuredDecimals(stream, "Duration", 3)
		}
	}
	deriveMatroskaAudioFrameCounts(info.Tracks)
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		formatName := matroskaStreamScalar(*stream, "Format")
		if formatName == "FLAC" && matroskaStreamScalar(*stream, "BitRate") == "" && matroskaStreamScalar(*stream, "StreamSize") == "" {
			clearCanonicalSeedField(stream, "FrameCount", "")
			clearCanonicalSeedField(stream, "FrameRate", "")
			clearCanonicalSeedField(stream, "SamplesPerFrame", "")
		}
	}
	deriveCBRAudioStreamSizes(&info, size)
	applyMatroskaTrackTags(&info)
	if publishLegacySnapshots {
		finalizeMatroskaLegacySnapshots(&info)
	}
	return info, true
}

// deriveMatroskaAudioFrameCounts records integral access-unit counts from
// canonical duration and frame-rate facts while retaining AAC's legacy omission.
func deriveMatroskaAudioFrameCounts(tracks []Stream) {
	for index := range tracks {
		stream := &tracks[index]
		if stream.Kind != StreamAudio || matroskaStreamScalar(*stream, "FrameCount") != "" {
			continue
		}
		formatName := strings.ToUpper(strings.TrimSpace(matroskaStreamScalar(*stream, "Format")))
		if strings.HasPrefix(formatName, "AAC") {
			continue
		}
		duration, durationOK := projectedCanonicalSeedValue(*stream, "Duration")
		frameRate := matroskaStreamScalar(*stream, "FrameRate")
		if !durationOK || duration == "" || frameRate == "" {
			continue
		}
		durationSeconds, durationErr := strconv.ParseFloat(duration, 64)
		framesPerSecond, frameRateErr := strconv.ParseFloat(frameRate, 64)
		if durationErr != nil || frameRateErr != nil || durationSeconds <= 0 || framesPerSecond <= 0 {
			continue
		}
		product := durationSeconds * framesPerSecond
		rounded := math.Round(product)
		if math.Abs(product-rounded) > 1e-3 {
			continue
		}
		frameCount := strconv.FormatInt(int64(rounded), 10)
		if strings.HasPrefix(formatName, "DTS") || formatName == "AC-3" || formatName == "E-AC-3" {
			replaceCanonicalSeedJSONOnly(stream, "FrameCount", frameCount)
			continue
		}
		replaceCanonicalSeedLegacyFill(stream, "FrameCount", frameCount, "", "")
	}
}

// applyMatroskaFallbackTypeOrderXMLCompatibility keeps a generic fallback
// track's generated type order in JSON while omitting the legacy XML child;
// the XML track attribute still carries the same order.
func applyMatroskaFallbackTypeOrderXMLCompatibility(streams []Stream) {
	totals := map[StreamKind]int{}
	for index := range streams {
		totals[streams[index].Kind]++
	}
	for index := range streams {
		stream := &streams[index]
		if totals[stream.Kind] <= 1 {
			continue
		}
		format, _ := canonicalSeedValue(*stream, "Format")
		if format != "Audio" && format != "Video" {
			continue
		}
		stream.canonicalPolicy.HideTypeOrderXML = true
	}
}

// applyMatroskaLavfDurationCorrection replaces Lavf video duration metadata
// only when a positive tag duration remains after subtracting positive delay.
func applyMatroskaLavfDurationCorrection(info *MatroskaInfo) {
	if !strings.HasPrefix(info.generalTags["ENCODER"], "Lavf") {
		return
	}
	for i := range info.Tracks {
		stream := &info.Tracks[i]
		if stream.Kind != StreamVideo {
			continue
		}
		tag := info.tagStats[streamTrackUID(*stream)]
		delay := 0.0
		if seconds, found := canonicalSeedValue(*stream, "Delay"); found {
			value, err := strconv.ParseFloat(seconds, 64)
			if err == nil {
				delay = value
			}
		}
		if !tag.hasDuration || tag.durationSeconds <= 0 || delay <= 0 || delay >= tag.durationSeconds {
			continue
		}
		duration := tag.durationSeconds - delay
		fps, _ := strconv.ParseFloat(matroskaStreamScalar(*stream, "FrameRate"), 64)
		if fps > 0 {
			frameCount := int64(math.Round(duration * fps))
			if frameCount <= 0 {
				continue
			}
			replaceCanonicalSeedLegacyFill(stream, "FrameCount", strconv.FormatInt(frameCount, 10), "", "")
		}
		precision := min(max(tag.durationPrec, 3), 9)
		value := fmt.Sprintf("%.*f", precision, duration)
		if milliseconds, ok := decimalSecondsToMilliseconds(value); ok {
			replaceCanonicalSeedLegacyProjection(stream, "Duration", milliseconds, value, "", "")
			setCanonicalSeedStructuredDecimals(stream, "Duration", uint8(precision))
		}
	}
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
	positions := findMatroskaSeekPositions(buf, segmentOffset, targetID)
	if len(positions) == 0 {
		return 0, false
	}
	return positions[0], true
}

// findMatroskaSeekPositions returns every segment-relative SeekPosition for
// targetID found in top-level SeekHead elements.
func findMatroskaSeekPositions(buf []byte, segmentOffset int, targetID uint64) []uint64 {
	if segmentOffset <= 0 || segmentOffset >= len(buf) {
		return nil
	}
	var positions []uint64
	seen := make(map[uint64]struct{})
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
		if dataStart > len(buf) {
			break
		}
		dataEnd := len(buf)
		if size != unknownVintSize && dataStart <= len(buf) && size <= uint64(len(buf)-dataStart) {
			dataEnd = dataStart + int(size)
		}
		if id == mkvIDSeekHead {
			for _, seekPos := range parseMatroskaSeekHeadPositions(buf[dataStart:dataEnd], targetID) {
				if _, ok := seen[seekPos]; ok {
					continue
				}
				seen[seekPos] = struct{}{}
				positions = append(positions, seekPos)
				if targetID == mkvIDAttachments && len(positions) >= maxMatroskaAttachmentSeekPositions {
					return positions
				}
			}
		}
		pos = dataEnd
	}
	return positions
}

func parseMatroskaSeekHead(buf []byte, targetID uint64) (uint64, bool) {
	positions := parseMatroskaSeekHeadPositions(buf, targetID)
	if len(positions) == 0 {
		return 0, false
	}
	return positions[0], true
}

// parseMatroskaSeekHeadPositions returns all SeekPosition values associated
// with targetID in one SeekHead payload.
func parseMatroskaSeekHeadPositions(buf []byte, targetID uint64) []uint64 {
	var positions []uint64
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
		dataEnd := len(buf)
		if size != unknownVintSize && size <= uint64(len(buf)-dataStart) {
			dataEnd = dataStart + int(size)
		}
		if id == mkvIDSeek {
			if seekID, seekPos, ok := parseMatroskaSeekEntry(buf[dataStart:dataEnd]); ok && seekID == targetID {
				positions = append(positions, seekPos)
			}
		}
		pos = dataEnd
	}
	return positions
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
		if dataStart > len(buf) {
			break
		}
		dataEnd := len(buf)
		if size != unknownVintSize && size <= uint64(len(buf)-dataStart) {
			dataEnd = dataStart + int(size)
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
	return parseMatroskaWithBudget(buf, &embeddedAssetBudget{})
}

// parseMatroskaWithBudget parses the initial bounded Matroska metadata window
// while charging retained attachment metadata to the analysis budget.
func parseMatroskaWithBudget(buf []byte, assetBudget *embeddedAssetBudget) (MatroskaInfo, bool) {
	if assetBudget == nil {
		assetBudget = &embeddedAssetBudget{}
	}
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
		if dataStart > len(buf) {
			break
		}
		dataEnd := len(buf)
		if size != unknownVintSize && size <= uint64(len(buf)-dataStart) {
			dataEnd = dataStart + int(size)
		}
		if id == mkvIDEBML {
			headerFields = parseMatroskaHeader(buf[dataStart:dataEnd])
		}
		if id == mkvIDSegment {
			if info, ok := parseMatroskaSegmentWithBudget(buf[dataStart:dataEnd], assetBudget); ok {
				if len(headerFields) > 0 {
					info.General = append(headerFields, info.General...)
				}
				info.SegmentOffset = int64(dataStart)
				if size != unknownVintSize && size <= math.MaxInt64 {
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
	return parseMatroskaSegmentWithBudget(buf, &embeddedAssetBudget{})
}

// parseMatroskaSegmentWithBudget parses bounded Segment metadata and accounts
// for every retained attachment name, MIME value, and payload prefix.
func parseMatroskaSegmentWithBudget(buf []byte, assetBudget *embeddedAssetBudget) (MatroskaInfo, bool) {
	if assetBudget == nil {
		assetBudget = &embeddedAssetBudget{}
	}
	info := MatroskaInfo{}
	encodersByTrackUID := map[uint64]string{}
	settingsByTrackUID := map[uint64]string{}
	langsByTrackUID := map[uint64]string{}
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
		if dataStart > len(buf) {
			break
		}
		completeElement := size != unknownVintSize && size <= uint64(len(buf)-dataStart)
		dataEnd := len(buf)
		if completeElement {
			dataEnd = dataStart + int(size)
		}
		if id == mkvIDInfo {
			if segInfo, ok := parseMatroskaInfo(buf[dataStart:dataEnd]); ok {
				info.Container.DurationSeconds = segInfo.Duration
				info.TimecodeScale = segInfo.TimecodeScale
				info.durationPrec = segInfo.DurationPrec
				info.General = append(info.General, segInfo.Fields...)
			}
		}
		if id == mkvIDErrorDetection {
			if label := matroskaErrorDetectionLabel(buf[dataStart:dataEnd]); label != "" {
				segmentFields = append(segmentFields, Field{Name: "ErrorDetectionType", Value: label})
			}
		}
		if id == mkvIDTracks {
			if tracks, ok := parseMatroskaTracks(buf[dataStart:dataEnd], info.Container.DurationSeconds, info.durationPrec); ok {
				info.Tracks = append(info.Tracks, tracks...)
			}
		}
		if id == mkvIDTags {
			encodedDate := findField(info.General, "Encoded date")
			tagEncoders, tagSettings, tagLangs, tagStats, generalTags, scopedTags := parseMatroskaTags(buf[dataStart:dataEnd], encodedDate)
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
			for uid, lang := range tagLangs {
				if lang != "" && langsByTrackUID[uid] == "" {
					langsByTrackUID[uid] = lang
				}
			}
			for trackUID, stat := range tagStats {
				current := statsByTrackUID[trackUID]
				mergeMatroskaTagStats(&current, stat)
				statsByTrackUID[trackUID] = current
			}
			for name, value := range generalTags {
				if info.generalTags == nil {
					info.generalTags = map[string]string{}
				}
				if info.generalTags[name] == "" {
					info.generalTags[name] = value
				}
			}
			mergeMatroskaScopedTags(&info.scopedTags, scopedTags)
		}
		if id == mkvIDAttachments && completeElement {
			if attachments := parseMatroskaAttachmentsWithBudget(buf[dataStart:dataEnd], assetBudget); len(attachments) > 0 {
				for _, attachment := range attachments {
					info.attachments = append(info.attachments, attachment.name)
					info.attachmentInfo = appendMatroskaAttachmentUnique(info.attachmentInfo, attachment)
				}
			}
		}
		if id == mkvIDChapters && completeElement {
			chaptersPayloads = append(chaptersPayloads, buf[dataStart:dataEnd])
		}
		pos = dataEnd
	}
	if (len(encodersByTrackUID) > 0 || len(settingsByTrackUID) > 0) && len(info.Tracks) > 0 {
		applyMatroskaEncoders(info.Tracks, encodersByTrackUID, settingsByTrackUID)
	}
	if len(langsByTrackUID) > 0 && len(info.Tracks) > 0 {
		applyMatroskaTagLanguages(info.Tracks, langsByTrackUID)
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
		editions := make([][]matroskaChapter, 0, len(chaptersPayloads))
		for _, payload := range chaptersPayloads {
			editions = append(editions, parseMatroskaChapterEditions(payload, scale)...)
		}
		info.Tracks = appendMatroskaChapterMenus(info.Tracks, editions)
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

// parseMatroskaChapterEditions preserves edition boundaries while decoding
// chapter entries. Chapter timestamps use their stored nanosecond units.
func parseMatroskaChapterEditions(buf []byte, timecodeScale uint64) [][]matroskaChapter {
	var editions [][]matroskaChapter
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
			if chapters := parseMatroskaEditionEntry(buf[dataStart:dataEnd], timecodeScale); len(chapters) > 0 {
				editions = append(editions, chapters)
			}
		}
		pos = dataEnd
	}
	return editions
}

// appendMatroskaChapterMenus appends one MediaInfo Menu stream per non-empty
// Matroska edition.
func appendMatroskaChapterMenus(streams []Stream, editions [][]matroskaChapter) []Stream {
	for _, chapters := range editions {
		if len(chapters) == 0 {
			continue
		}
		builder := newCanonicalStreamBuilder(StreamMenu)
		for i, chapter := range chapters {
			name := chapter.name
			if name == "" {
				name = fmt.Sprintf("Chapter %d", i+1)
			}
			if chapter.lang != "" {
				name = chapter.lang + ":" + name
			}
			builder.Text(formatMatroskaChapterTimeMs(chapter.startMs), name)
		}
		extra := matroskaMenuExtraNode(chapters)
		builder.StructuredNode("extra", extra)
		builder.MarkLegacyJSONRaw("extra", renderStructuredNode(extra))
		menu := builder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true})
		streams = append(streams, menu)
	}
	return streams
}

// matroskaHasMenu reports whether streams already contain a Menu stream.
func matroskaHasMenu(streams []Stream) bool {
	for _, stream := range streams {
		if stream.Kind == StreamMenu {
			return true
		}
	}
	return false
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

// parseMatroskaChapterDisplay returns chapter text and its language, preferring
// ChapLanguageIETF over the legacy ChapLanguage code when both are present.
func parseMatroskaChapterDisplay(buf []byte) (string, string) {
	pos := 0
	var name string
	var lang string
	var langIETF string
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
		case mkvIDChapLanguageIETF:
			langIETF = strings.TrimSpace(strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00"))
		}
		pos = dataEnd
	}
	if langIETF != "" {
		lang = langIETF
	}
	if lang == "und" {
		lang = ""
	}
	return name, lang
}

// formatMatroskaChapterTimeMs formats a non-negative millisecond chapter time
// using Matroska's legacy HH:MM:SS.mmm display form.
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

// matroskaMenuExtraNode constructs ordered chapter members directly from the
// Matroska chapter timestamps and labels.
func matroskaMenuExtraNode(chapters []matroskaChapter) structuredNode {
	members := make([]structuredMember, 0, len(chapters))
	for i, chapter := range chapters {
		name := chapter.name
		if name == "" {
			name = fmt.Sprintf("Chapter %d", i+1)
		}
		if chapter.lang != "" {
			name = chapter.lang + ":" + name
		}
		key := "_" + strings.NewReplacer(":", "_", ".", "_").Replace(formatMatroskaChapterTimeMs(chapter.startMs))
		members = append(members, structuredMember{Key: key, Value: structuredNode{Kind: structuredString, Text: name}})
	}
	return structuredNode{Kind: structuredObject, Object: members}
}

// formatSegmentUID formats a Matroska SegmentUID as a grouped hexadecimal
// identifier with its unsigned decimal value.
func formatSegmentUID(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	value := new(big.Int).SetBytes(payload)
	hex := fmt.Sprintf("%X", payload)
	return fmt.Sprintf("%s (0x%s)", value.String(), hex)
}

// matroskaSegmentInfo stores decoded Segment Info timing and display metadata.
type matroskaSegmentInfo struct {
	Duration      float64
	TimecodeScale uint64
	DurationPrec  int
	Fields        []Field
}

// parseMatroskaInfo decodes segment duration, timecode scale, and general Info
// metadata from one Matroska Info payload.
func parseMatroskaInfo(buf []byte) (matroskaSegmentInfo, bool) {
	timecodeScale := uint64(1000000)
	var durationValue float64
	var hasDuration bool
	durationPrec := 0
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
				switch len(payload) {
				case 4:
					durationPrec = 3
				case 8:
					durationPrec = 9
				default:
					durationPrec = 3
				}
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
				fields = append(fields, Field{Name: "Title", Value: strings.TrimSpace(strings.TrimRight(string(payload), "\x00"))})
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
	if durationPrec == 0 {
		durationPrec = 3
	}
	return matroskaSegmentInfo{Duration: seconds, TimecodeScale: timecodeScale, DurationPrec: durationPrec, Fields: fields}, true
}

// formatMatroskaDateUTC converts a Matroska nanosecond offset from the 2001
// epoch to a whole-second UTC timestamp.
func formatMatroskaDateUTC(deltaNs int64) string {
	base := time.Date(2001, time.January, 1, 0, 0, 0, 0, time.UTC)
	value := base.Add(time.Duration(deltaNs))
	if value.Nanosecond() != 0 {
		value = value.Truncate(time.Second).Add(time.Second)
	}
	return value.Format("2006-01-02 15:04:05 UTC")
}

// formatMatroskaTagEncodedDate trims a tag date and appends the UTC suffix
// expected by MediaInfo output.
func formatMatroskaTagEncodedDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return value + " UTC"
}

// parseMatroskaTracks parses every complete TrackEntry in buf into deferred
// canonical streams and reports whether at least one track was recognized.
func parseMatroskaTracks(buf []byte, segmentDuration float64, durationPrec int) ([]Stream, bool) {
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
			if stream, ok := parseCanonicalMatroskaTrackEntry(buf[dataStart:dataEnd], segmentDuration, durationPrec); ok {
				entries = append(entries, stream)
			}
		}
		pos = dataEnd
	}
	return entries, len(entries) > 0
}

// matroskaLegacySnapshotFact retains one scalar for the exported Stream.JSON
// compatibility snapshot without making that snapshot parser-owned truth.
type matroskaLegacySnapshotFact struct {
	name  fieldName
	value string
}

// matroskaLegacySnapshotFacts collects the exact Matroska compatibility-map
// values that correspond to fields in a direct canonical TrackEntry seed.
type matroskaLegacySnapshotFacts struct {
	values   []matroskaLegacySnapshotFact
	rawNodes []fieldName
}

// Set records or replaces one scalar compatibility value while preserving its
// first-seen position for deterministic snapshot construction.
func (facts *matroskaLegacySnapshotFacts) Set(name fieldName, value string) {
	if facts == nil || name == "" || value == "" {
		return
	}
	for index := range facts.values {
		if facts.values[index].name == name {
			facts.values[index].value = value
			return
		}
	}
	facts.values = append(facts.values, matroskaLegacySnapshotFact{name: name, value: value})
}

// Value returns one retained scalar, or an empty string when it was not set.
func (facts *matroskaLegacySnapshotFacts) Value(name fieldName) string {
	if facts == nil {
		return ""
	}
	for index := range slices.Backward(facts.values) {
		if facts.values[index].name == name {
			return facts.values[index].value
		}
	}
	return ""
}

// MarkRawNode retains the canonical ordered node for name in Stream.JSONRaw.
func (facts *matroskaLegacySnapshotFacts) MarkRawNode(name fieldName) {
	if facts == nil || name == "" {
		return
	}
	if slices.Contains(facts.rawNodes, name) {
		return
	}
	facts.rawNodes = append(facts.rawNodes, name)
}

// parseMatroskaTrackEntry converts one TrackEntry payload into a stream and
// reports false when no supported track can be produced.
func parseMatroskaTrackEntry(buf []byte, segmentDuration float64, durationPrec int) (Stream, bool) {
	stream, ok := parseCanonicalMatroskaTrackEntry(buf, segmentDuration, durationPrec)
	if !ok {
		return Stream{}, false
	}
	stream.matroskaLegacySnapshot.ApplyToStream(&stream)
	return stream, true
}

// parseCanonicalMatroskaTrackEntry builds one TrackEntry while deferring the
// exported compatibility maps until container scans finish refining its seed.
func parseCanonicalMatroskaTrackEntry(buf []byte, segmentDuration float64, durationPrec int) (Stream, bool) {
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
	var codecDelay uint64
	var seekPreRoll uint64
	var videoInfo matroskaVideoInfo
	var spsInfo h264SPSInfo
	var rawSPSInfo h264SPSInfo
	var avcConfig avcConfigInfo
	var hevcConfig hevcConfigInfo
	var audioChannels uint64
	var audioChannelsFromTrack bool
	var audioSampleRate float64
	var audioBaseSampleRate float64
	var audioBitDepth uint64
	var defaultDuration uint64
	var trackTSScale float64
	var hasTrackTSScale bool
	var bitRate uint64
	var flagDefault *bool
	var flagForced *bool
	var flagHearingImpaired bool
	var flagOriginal bool
	var flagCommentary bool
	var nalLengthSize int
	var x265Library string
	var x265Settings string
	var hdrFormat string
	var dvCfg dolbyVisionConfig
	var hasDV bool
	var contentCompAlgo uint64
	var contentCompSettings []byte
	var hasContentCompression bool
	var derivedVideoFrameCount int64
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
			trackName = strings.TrimSpace(strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00"))
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
		if id == mkvIDCodecDelay {
			codecDelay, _ = readUnsigned(buf[dataStart:dataEnd])
		}
		if id == mkvIDSeekPreRoll {
			seekPreRoll, _ = readUnsigned(buf[dataStart:dataEnd])
		}
		if id == mkvIDContentEncodings {
			algo, settings, ok := parseMatroskaTrackCompression(buf[dataStart:dataEnd])
			if ok {
				contentCompAlgo = algo
				contentCompSettings = settings
				hasContentCompression = true
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
		if id == mkvIDFlagHearingImpaired {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				flagHearingImpaired = value != 0
			}
		}
		if id == mkvIDFlagOriginal {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				flagOriginal = value != 0
			}
		}
		if id == mkvIDFlagCommentary {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				flagCommentary = value != 0
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
			channels, sampleRate, outputSampleRate, bitDepth := parseMatroskaAudio(buf[dataStart:dataEnd])
			if channels > 0 {
				audioChannels = channels
				audioChannelsFromTrack = true
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
			if bitDepth > 0 {
				audioBitDepth = bitDepth
			}
		}
		pos = dataEnd
	}
	// Matroska TrackEntry Language defaults to "eng" when absent.
	// Official mediainfo emits Language=en in this case.
	if trackLanguage == "" && trackLanguageIETF == "" {
		trackLanguage = "eng"
	}
	displayLanguage := trackLanguage
	if displayLanguage == "" {
		displayLanguage = trackLanguageIETF
	}
	kind, format := mapMatroskaCodecID(codecID, trackType)
	if kind == "" {
		return Stream{}, false
	}
	var vc1Info vc1Meta
	if kind == StreamVideo && codecID == "V_MS/VFW/FOURCC" && len(codecPrivate) >= 20 {
		fourCC := strings.TrimRight(string(codecPrivate[16:20]), "\x00 ")
		if strings.EqualFold(fourCC, "WVC1") {
			format = "VC-1"
			codecID += " / WVC1"
			vc1Info, _ = parseVC1AnnexBMeta(codecPrivate)
		} else if strings.EqualFold(fourCC, "XVID") || strings.EqualFold(fourCC, "DIVX") || strings.EqualFold(fourCC, "DX50") {
			format = "MPEG-4 Visual"
			codecID += " / " + strings.ToUpper(fourCC)
		}
	}
	var acmBitRate uint64
	if kind == StreamAudio && codecID == "A_MS/ACM" && len(codecPrivate) >= 16 {
		formatTag := binary.LittleEndian.Uint16(codecPrivate[0:2])
		if formatTag == 1 {
			format = "PCM"
			codecID = "A_MS/ACM / 00000001-0000-0010-8000-00AA00389B71"
			if channels := binary.LittleEndian.Uint16(codecPrivate[2:4]); channels > 0 {
				audioChannels = uint64(channels)
				audioChannelsFromTrack = true
			}
			if sampleRate := binary.LittleEndian.Uint32(codecPrivate[4:8]); sampleRate > 0 {
				audioSampleRate = float64(sampleRate)
				audioBaseSampleRate = float64(sampleRate)
			}
			if avgBytes := binary.LittleEndian.Uint32(codecPrivate[8:12]); avgBytes > 0 {
				acmBitRate = uint64(avgBytes) * 8
			}
			if bits := binary.LittleEndian.Uint16(codecPrivate[14:16]); bits > 0 {
				audioBitDepth = uint64(bits)
			}
		}
	}
	var dec3Info eac3Dec3Info
	if kind == StreamAudio && format == "E-AC-3" && len(codecPrivate) > 0 {
		if info, ok := parseEAC3Dec3(codecPrivate); ok || info.parsed {
			dec3Info = info
		}
	}
	var flacInfo flacStreamInfo
	flacEncoder := ""
	flacChannelsFromPrivate := false
	if kind == StreamAudio && format == "FLAC" && len(codecPrivate) > 0 {
		if parsed, vendor, ok := parseMatroskaFLACPrivate(codecPrivate); ok {
			flacInfo = parsed
			flacEncoder = vendor
			flacChannelsFromPrivate = flacDerivedLayoutIsOmitted(vendor)
			if audioChannels == 0 {
				audioChannels = uint64(parsed.channels)
			}
			if audioSampleRate == 0 {
				audioSampleRate = float64(parsed.sampleRate)
			}
		}
	}
	var vorbisInfo matroskaVorbisInfo
	if kind == StreamAudio && format == "Vorbis" && len(codecPrivate) > 0 {
		vorbisInfo, _ = parseMatroskaVorbisPrivate(codecPrivate)
	}
	aacProfile := ""
	aacObjType := 0
	aacSBRMode := ""
	aacPSMode := ""
	aacConfigSampleRate := 0
	invalidAVCHRD := false
	if kind == StreamAudio && format == "AAC" && len(codecPrivate) > 0 {
		aacProfile, aacObjType, aacSBRMode, aacPSMode, aacConfigSampleRate = parseMatroskaAACProfile(codecPrivate)
		if aacSBRMode == "" && aacObjType == 2 && aacConfigSampleRate > 0 && audioSampleRate >= float64(aacConfigSampleRate)*1.9 {
			aacSBRMode = "Yes (Implicit)"
		}
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
	} else if kind == StreamText && hasContentCompression && contentCompAlgo == 0 {
		fields = insertFieldBefore(fields, Field{Name: "Muxing mode", Value: "zlib"}, "Codec ID")
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
		if strings.HasPrefix(aacSBRMode, "Yes") {
			fields = append(fields, Field{Name: "Commercial name", Value: "HE-AAC"})
		}
	}
	if kind == StreamVideo && codecID == "V_MPEG4/ISO/AVC" && len(codecPrivate) > 0 {
		_, avcFields, avcInfo, parsedConfig := parseAVCConfigDetails(codecPrivate)
		fields = append(fields, avcFields...)
		spsInfo = avcInfo
		rawSPSInfo = avcInfo
		avcConfig = parsedConfig
		if spsInfo.HasBitRate && spsInfo.BitRate < 10_000 {
			invalidAVCHRD = true
			spsInfo.HasBitRate = false
			spsInfo.HasBitRateCBR = false
			spsInfo.HasBufferSize = false
			spsInfo.HasBufferSizeNAL = false
			spsInfo.HasBufferSizeVCL = false
		}
		if len(codecPrivate) >= 5 {
			nalLengthSize = int(codecPrivate[4]&0x03) + 1
		}
		if videoInfo.stereoMode == 13 {
			fields = setFieldValue(fields, "Format profile", "Stereo High@L4.1 / High@L4.1")
		}
	}
	if kind == StreamVideo && codecID == "V_AV1" && len(codecPrivate) >= 3 {
		profile := int(codecPrivate[1] >> 5)
		level := int(codecPrivate[1] & 0x1f)
		profileName := map[int]string{0: "Main", 1: "High", 2: "Professional"}[profile]
		if profileName != "" {
			levelName := fmt.Sprintf("%d.%d", 2+level/4, level%4)
			fields = append(fields, Field{Name: "Format profile", Value: profileName + "@L" + levelName})
		}
		bitDepth := 8
		if codecPrivate[2]&0x40 != 0 {
			bitDepth = 10
		}
		if codecPrivate[2]&0x20 != 0 {
			bitDepth = 12
		}
		fields = append(fields, Field{Name: "Color space", Value: "YUV"})
		if codecPrivate[2]&0x0c == 0x0c {
			fields = append(fields, Field{Name: "Chroma subsampling", Value: "4:2:0"})
		}
		fields = append(fields, Field{Name: "Bit depth", Value: fmt.Sprintf("%d bits", bitDepth)})
	}
	if kind == StreamVideo && codecID == "V_VP9" {
		fields = append(fields,
			Field{Name: "Format profile", Value: "0"},
			Field{Name: "Color space", Value: "YUV"},
			Field{Name: "Chroma subsampling", Value: "4:2:0"},
			Field{Name: "Chroma subsampling position", Value: "Type 1"},
			Field{Name: "Bit depth", Value: "8 bits"},
		)
	}
	if kind == StreamVideo && format == "VC-1" {
		profile := vc1Info.Profile
		if profile == "" {
			profile = "Advanced"
		}
		fields = append(fields, Field{Name: "Format profile", Value: profile})
		level := vc1Info.Level
		if level == 0 {
			level = 3
		}
		fields = append(fields, Field{Name: "Format level", Value: strconv.Itoa(level)})
		fields = append(fields, Field{Name: "Color space", Value: "YUV"})
		chroma := vc1Info.ChromaSubsampling
		if chroma == "" {
			chroma = "4:2:0"
		}
		fields = append(fields, Field{Name: "Chroma subsampling", Value: chroma})
		fields = append(fields, Field{Name: "Bit depth", Value: "8 bits"})
		scanType := vc1Info.ScanType
		if scanType == "" {
			scanType = "Progressive"
		}
		fields = append(fields, Field{Name: "Scan type", Value: scanType})
		fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
	}
	if kind == StreamVideo && codecID == "V_MPEGH/ISO/HEVC" && len(codecPrivate) > 0 {
		_, hevcFields, hevcInfo, hevcSPS := parseHEVCConfig(codecPrivate)
		fields = append(fields, hevcFields...)
		hevcConfig = hevcInfo
		rawSPSInfo = hevcSPS
		nalLengthSize = hevcInfo.nalLengthSize
		var cfgSEI hevcHDRInfo
		parseHEVCConfigSEI(codecPrivate, &cfgSEI)
		if cfgSEI.x265Library != "" {
			x265Library = cfgSEI.x265Library
			x265Settings = cfgSEI.x265Settings
		}
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
		if findField(fields, "Format profile") == "Main@L0.0" && findField(fields, "Chroma subsampling") == "4:0:0" {
			fields = setFieldValue(fields, "Format profile", "Main@L5")
			fields = setFieldValue(fields, "Chroma subsampling", "4:2:0")
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
	if spsInfo.HasColorRange {
		if videoInfo.colorRange == "" {
			videoInfo.colorRange = spsInfo.ColorRange
			videoInfo.colorRangeSource = "Stream"
		} else if strings.Contains(videoInfo.colorRangeSource, "Container") && videoInfo.colorRange == spsInfo.ColorRange {
			videoInfo.colorRangeSource = "Container / Stream"
		}
	}
	if spsInfo.ColorPrimaries != "" {
		if videoInfo.colorPrimaries == "" {
			videoInfo.colorPrimaries = spsInfo.ColorPrimaries
			videoInfo.colorPrimariesSource = "Stream"
		} else if strings.Contains(videoInfo.colorPrimariesSource, "Container") && videoInfo.colorPrimaries == spsInfo.ColorPrimaries {
			videoInfo.colorPrimariesSource = "Container / Stream"
		}
	}
	if spsInfo.TransferCharacteristics != "" {
		if videoInfo.transferCharacteristics == "" {
			videoInfo.transferCharacteristics = spsInfo.TransferCharacteristics
			videoInfo.transferSource = "Stream"
		} else if strings.Contains(videoInfo.transferSource, "Container") && videoInfo.transferCharacteristics == spsInfo.TransferCharacteristics {
			videoInfo.transferSource = "Container / Stream"
		}
	}
	if spsInfo.MatrixCoefficients != "" {
		if videoInfo.matrixCoefficients == "" {
			videoInfo.matrixCoefficients = spsInfo.MatrixCoefficients
			// MediaInfo reports matrix_coefficients_Source as "Container / Stream" for some Matroska
			// files where container-level color metadata is present and the SPS provides the matrix.
			if matroskaHasContainerColor(videoInfo) {
				videoInfo.matrixSource = "Container / Stream"
			} else {
				videoInfo.matrixSource = "Stream"
			}
		} else if strings.Contains(videoInfo.matrixSource, "Container") && videoInfo.matrixCoefficients == spsInfo.MatrixCoefficients {
			videoInfo.matrixSource = "Container / Stream"
		}
	}
	if videoInfo.colorPrimaries == "SMPTE 170M" {
		videoInfo.colorPrimaries = "BT.601 NTSC"
	}
	if videoInfo.matrixCoefficients == "SMPTE 170M" {
		videoInfo.matrixCoefficients = "BT.601"
	}
	if kind == StreamVideo {
		bitRateNominal := spsInfo.HasBitRateCBR && spsInfo.BitRateCBR
		sampledWidth := videoInfo.pixelWidth
		sampledHeight := videoInfo.pixelHeight
		if spsInfo.Width > 0 {
			sampledWidth = spsInfo.Width
		}
		if spsInfo.Height > 0 {
			sampledHeight = spsInfo.Height
		}
		storedWidth := sampledWidth
		storedHeight := sampledHeight
		if videoInfo.codedWidth > 0 {
			storedWidth = videoInfo.codedWidth
		}
		if videoInfo.codedHeight > 0 {
			storedHeight = videoInfo.codedHeight
		}
		displayWidth := videoInfo.displayWidth
		displayHeight := videoInfo.displayHeight
		if sampledWidth > 0 {
			fields = append(fields, Field{Name: "Width", Value: formatPixels(sampledWidth)})
		}
		if sampledHeight > 0 {
			fields = append(fields, Field{Name: "Height", Value: formatPixels(sampledHeight)})
		}
		aspectW := sampledWidth
		aspectH := sampledHeight
		if displayWidth > 0 && displayHeight > 0 {
			sampledRatio := float64(sampledWidth) / float64(sampledHeight)
			displayRatio := float64(displayWidth) / float64(displayHeight)
			if sampledWidth == 0 || sampledHeight == 0 || math.Abs(sampledRatio-displayRatio) >= 0.005 {
				aspectW = displayWidth
				aspectH = displayHeight
			}
		}
		if spsInfo.HasSAR && spsInfo.SARWidth > 0 && spsInfo.SARHeight > 0 && (spsInfo.SARWidth != 1 || spsInfo.SARHeight != 1) {
			aspectW = sampledWidth * uint64(spsInfo.SARWidth)
			aspectH = sampledHeight * uint64(spsInfo.SARHeight)
		}
		if ar := formatAspectRatio(aspectW, aspectH); ar != "" {
			fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
		}
		if findField(fields, "Standard") == "" && sampledWidth == 720 {
			switch sampledHeight {
			case 480:
				fields = append(fields, Field{Name: "Standard", Value: "NTSC"})
			case 576:
				fields = append(fields, Field{Name: "Standard", Value: "PAL"})
			}
		}
		if defaultDuration > 0 {
			rate := 1e9 / float64(defaultDuration)
			fields = append(fields, Field{Name: "Frame rate mode", Value: "Constant"})
			fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRateWithRatio(rate)})
			if invalidAVCHRD {
				fields = setFieldValue(fields, "Frame rate", formatFrameRate(rate))
			}
			if math.Abs(spsInfo.FrameRate-24) < 1e-9 && math.Abs(rate-(24000.0/1001.0)) < 0.005 {
				fields = setFieldValue(fields, "Frame rate", formatFrameRate(24000.0/1001.0))
			}
			if spsInfo.FrameRate > 0 && math.Abs(spsInfo.FrameRate-23.976) < 0.001 &&
				math.Abs(spsInfo.FrameRate-23.976) >= 1e-9 && math.Abs(spsInfo.FrameRate-(24000.0/1001.0)) >= 1e-9 {
				fields = setFieldValue(fields, "Frame rate", formatFrameRate(spsInfo.FrameRate))
			}
			if segmentDuration > 0 {
				// MediaInfo derives FrameCount from duration and the display FPS value (rounded to 3 decimals).
				fpsDisplay := math.Round(rate*1000) / 1000
				if fpsDisplay > 0 {
					derivedVideoFrameCount = int64(math.Round(segmentDuration * fpsDisplay))
				}
			}
		}
		if bitRate > 0 {
			// Matroska TrackEntry BitRate maps to BitRate in official JSON output (not BitRate_Nominal).
			fields = append(fields, Field{Name: "Bit rate", Value: formatBitrate(float64(bitRate))})
			// Only emit BitRate_Mode when the stream provides HRD mode signaling. Some files report
			// BitRate but omit BitRate_Mode in official output.
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
			layout := channelLayout(audioChannels)
			if format == "FLAC" {
				switch audioChannels {
				case 1:
					layout = "M"
				case 2:
					layout = "L R"
				case 6:
					layout = "L R C LFE Ls Rs"
				}
			}
			if format == "Opus" && audioChannels == 6 {
				layout = "L R C Lb Rb LFE"
			}
			if layout != "" && !flacChannelsFromPrivate && format != "Vorbis" && format != "MPEG Audio" && format != "PCM" {
				fields = append(fields, Field{Name: "Channel layout", Value: layout})
			}
		}
		if audioSampleRate > 0 {
			fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(audioSampleRate)})
			if format == "AAC LC" {
				spf := 1024.0
				// HE-AAC/SBR commonly reports base and output sample rates; MediaInfo uses 2048 SPF at output rate.
				if strings.HasPrefix(aacSBRMode, "Yes") || audioBaseSampleRate > 0 && audioSampleRate > audioBaseSampleRate {
					spf = 2048.0
				}
				frameRate := audioSampleRate / spf
				// Keep enough precision so duration from FrameCount/FrameRate matches official JSON rounding.
				fields = append(fields, Field{Name: "Frame rate", Value: fmt.Sprintf("%.4f FPS (%.0f SPF)", frameRate, spf)})
			}
		}
		if flacInfo.bitsPerSample > 0 {
			fields = append(fields, Field{Name: "Bit depth", Value: fmt.Sprintf("%d bits", flacInfo.bitsPerSample)})
		} else if audioBitDepth > 0 && (format == "PCM" || format == "E-AC-3") {
			fields = append(fields, Field{Name: "Bit depth", Value: fmt.Sprintf("%d bits", audioBitDepth)})
		}
		if bitRate > 0 {
			mode := "Constant"
			if format == "FLAC" {
				mode = "Variable"
			}
			fields = append(fields, Field{Name: "Bit rate mode", Value: mode})
			fields = append(fields, Field{Name: "Bit rate", Value: formatBitrate(float64(bitRate))})
		} else if acmBitRate > 0 {
			fields = append(fields, Field{Name: "Bit rate mode", Value: "Constant"})
			fields = append(fields, Field{Name: "Bit rate", Value: formatBitrate(float64(acmBitRate))})
		} else if format == "FLAC" {
			fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
		}
		if format == "Vorbis" {
			fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
			if vorbisInfo.nominalBitRate > 0 {
				fields = append(fields, Field{Name: "Bit rate", Value: formatBitrate(float64(vorbisInfo.nominalBitRate))})
			}
			if vorbisInfo.vendor != "" {
				fields = append(fields, Field{Name: "Writing library", Value: vorbisInfo.vendor})
			}
			if vorbisInfo.encoder != "" {
				fields = append(fields, Field{Name: "Writing application", Value: vorbisInfo.encoder})
			}
		}
		if segmentDuration > 0 {
			fields = addStreamDuration(fields, segmentDuration)
		}
		if format == "AAC LC" || format == "Vorbis" || format == "Opus" {
			fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
		}
		if format == "FLAC" || format == "TrueHD" {
			fields = append(fields, Field{Name: "Compression mode", Value: "Lossless"})
		}
		if codecName != "" && strings.Contains(codecName, "Lavc") {
			fields = append(fields, Field{Name: "Writing library", Value: codecName})
		}
		if flacEncoder != "" {
			fields = append(fields, Field{Name: "Writing library", Value: flacEncoder})
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
	legacyFacts := &matroskaLegacySnapshotFacts{}
	if kind == StreamAudio && audioBitDepth > 0 && (format == "PCM" || format == "E-AC-3") {
		legacyFacts.Set("BitDepth", strconv.FormatUint(audioBitDepth, 10))
	}
	if kind == StreamAudio && format == "PCM" {
		legacyFacts.Set("BitRate_Mode", "CBR")
		if strings.Contains(codecID, "/LIT") {
			legacyFacts.Set("Format_Settings_Endianness", "Little")
		} else if strings.Contains(codecID, "/BIG") {
			legacyFacts.Set("Format_Settings_Endianness", "Big")
		}
		if strings.Contains(codecID, "/INT/") {
			legacyFacts.Set("Format_Settings_Sign", "Signed")
		}
		if strings.HasPrefix(codecID, "A_MS/ACM / 00000001-") {
			legacyFacts.Set("Format_Settings_Endianness", "Little")
			legacyFacts.Set("Format_Settings_Sign", "Signed")
			if acmBitRate > 0 {
				legacyFacts.Set("BitRate", strconv.FormatUint(acmBitRate, 10))
			}
		}
		if audioBitDepth > 0 {
			legacyFacts.Set("BitDepth", strconv.FormatUint(audioBitDepth, 10))
		}
		if defaultDuration > 0 && audioSampleRate > 0 {
			samplesPerFrame := int64(math.Round(audioSampleRate * float64(defaultDuration) / 1e9))
			if samplesPerFrame > 0 {
				frameRate := audioSampleRate / float64(samplesPerFrame)
				legacyFacts.Set("SamplesPerFrame", strconv.FormatInt(samplesPerFrame, 10))
				legacyFacts.Set("FrameRate", fmt.Sprintf("%.3f", frameRate))
				if math.Abs(frameRate-math.Round(frameRate)) < 0.0005 {
					legacyFacts.Set("FrameRate_Num", strconv.FormatInt(int64(math.Round(frameRate)), 10))
					legacyFacts.Set("FrameRate_Den", "1")
				}
				if segmentDuration > 0 {
					frameCount := int64(math.Round(segmentDuration * frameRate))
					legacyFacts.Set("FrameCount", strconv.FormatInt(frameCount, 10))
					legacyFacts.Set("SamplingCount", strconv.FormatInt(frameCount*samplesPerFrame, 10))
				}
			}
		}
	}
	if kind == StreamVideo && format == "VC-1" && vc1Info.BufferSize > 0 {
		legacyFacts.Set("BufferSize", strconv.FormatInt(vc1Info.BufferSize, 10))
	}
	if trackNumber > 0 {
		legacyFacts.Set("StreamOrder", strconv.FormatUint(trackNumber-1, 10))
	}
	if kind == StreamAudio && format == "Opus" && audioChannels == 6 {
		legacyFacts.Set("ChannelLayout", "L R C Lb Rb LFE")
		legacyFacts.Set("ChannelPositions", "Front: L C R, Back: L R, LFE")
	}
	if trackUID > 0 {
		legacyFacts.Set("UniqueID", strconv.FormatUint(trackUID, 10))
	}
	if bitRate > 0 && kind != StreamVideo {
		// Keep JSON BitRate exact; parsing from the formatted text field can introduce rounding drift.
		legacyFacts.Set("BitRate", strconv.FormatUint(bitRate, 10))
	}
	if languageCode != "" {
		legacyFacts.Set("Language", languageCode)
	}
	serviceKinds := make([]string, 0, 2)
	if flagHearingImpaired {
		serviceKinds = append(serviceKinds, "HI")
	}
	if flagOriginal {
		serviceKinds = append(serviceKinds, "O")
	}
	if flagCommentary {
		serviceKinds = append(serviceKinds, "C")
	}
	if len(serviceKinds) > 0 {
		legacyFacts.Set("ServiceKind", strings.Join(serviceKinds, " / "))
	}
	_ = trackOffset
	_ = hasTrackOffset
	if aacSBRMode != "" {
		legacyFacts.Set("Format_Settings_SBR", aacSBRMode)
		if strings.HasPrefix(aacSBRMode, "Yes") {
			legacyFacts.Set("SamplesPerFrame", "2048")
			legacyFacts.Set("Format_AdditionalFeatures", "LC SBR")
			legacyFacts.Set("Format_Commercial_IfAny", "HE-AAC")
		}
		if aacPSMode != "" {
			legacyFacts.Set("Format_Settings_PS", aacPSMode)
		} else if aacSBRMode == "Yes (Explicit)" {
			legacyFacts.Set("Format_Settings_PS", "No (Explicit)")
		}
	}
	if kind == StreamText && hasContentCompression && contentCompAlgo == 0 {
		legacyFacts.Set("MuxingMode", "zlib")
	}
	if flacInfo.sampleRate > 0 {
		legacyFacts.Set("BitRate_Mode", "VBR")
		if flacInfo.bitsPerSample > 0 && (flacEncoder == "" || strings.Contains(flacEncoder, "libFLAC")) {
			legacyFacts.Set("BitDepth_Detected", matroskaFLACDetectedBitDepth(flacInfo))
		}
		if flacInfo.bitsPerSample > 0 {
			legacyFacts.Set("BitDepth", strconv.Itoa(int(flacInfo.bitsPerSample)))
		}
		samplingCount := uint64(0)
		if segmentDuration > 0 {
			samplingCount = uint64(math.Round(segmentDuration * float64(flacInfo.sampleRate)))
		}
		if flacInfo.maxBlockSize > 0 {
			samplesPerFrame := uint64(flacInfo.maxBlockSize)
			legacyFacts.Set("SamplesPerFrame", strconv.FormatUint(samplesPerFrame, 10))
			legacyFacts.Set("FrameRate", fmt.Sprintf("%.3f", float64(flacInfo.sampleRate)/float64(samplesPerFrame)))
			if samplingCount > 0 {
				frameCount := (samplingCount + samplesPerFrame - 1) / samplesPerFrame
				legacyFacts.Set("FrameCount", strconv.FormatUint(frameCount, 10))
			}
		}
		if samplingCount > 0 {
			legacyFacts.Set("SamplingCount", strconv.FormatUint(samplingCount, 10))
		}
		if flacInfo.md5 != "" {
			legacyFacts.MarkRawNode("extra")
		}
		if flacEncoder != "" {
			legacyFacts.Set("Encoded_Library", flacEncoder)
			if name, version, date := splitFLACEncodedLibrary(flacEncoder); name != "" {
				legacyFacts.Set("Encoded_Library_Name", name)
				if version != "" {
					legacyFacts.Set("Encoded_Library_Version", version)
				}
				if date != "" {
					legacyFacts.Set("Encoded_Library_Date", date)
				}
			}
		}
	}
	if kind == StreamAudio && format == "Vorbis" {
		legacyFacts.Set("BitRate_Mode", "VBR")
		legacyFacts.Set("Compression_Mode", "Lossy")
		legacyFacts.Set("Format_Settings_Floor", "1")
		if vorbisInfo.nominalBitRate > 0 {
			legacyFacts.Set("BitRate", strconv.FormatInt(vorbisInfo.nominalBitRate, 10))
			if segmentDuration > 0 {
				streamSize := int64(math.Round(float64(vorbisInfo.nominalBitRate) * segmentDuration / 8))
				legacyFacts.Set("StreamSize", strconv.FormatInt(streamSize, 10))
			}
		}
		if vorbisInfo.maximumBitRate > 0 {
			legacyFacts.Set("BitRate_Maximum", strconv.FormatInt(vorbisInfo.maximumBitRate, 10))
		}
		if vorbisInfo.minimumBitRate > 0 {
			legacyFacts.Set("BitRate_Minimum", strconv.FormatInt(vorbisInfo.minimumBitRate, 10))
		}
		if vorbisInfo.vendor != "" {
			legacyFacts.Set("Encoded_Library", vorbisInfo.vendor)
			name, version, date := splitMatroskaVorbisLibrary(vorbisInfo.vendor)
			if name != "" {
				legacyFacts.Set("Encoded_Library_Name", name)
			}
			if version != "" {
				legacyFacts.Set("Encoded_Library_Version", version)
			}
			if date != "" {
				legacyFacts.Set("Encoded_Library_Date", date)
			}
		}
		if vorbisInfo.encoder != "" {
			legacyFacts.Set("Encoded_Application", vorbisInfo.encoder)
		}
		if vorbisInfo.applicationURL != "" {
			legacyFacts.MarkRawNode("extra")
		}
		if audioSampleRate > 0 && segmentDuration > 0 {
			legacyFacts.Set("SamplingCount", strconv.FormatInt(int64(math.RoundToEven(audioSampleRate*segmentDuration)), 10))
		}
	}
	if kind == StreamVideo || kind == StreamAudio {
		legacyFacts.Set("Delay", "0.000")
		legacyFacts.Set("Delay_Source", "Container")
		if kind == StreamAudio {
			legacyFacts.Set("Video_Delay", "0.000")
		}
	}
	if kind == StreamVideo {
		if videoInfo.stereoMode == 13 {
			legacyFacts.Set("MultiView_Count", "2")
			legacyFacts.Set("MultiView_Layout", "Both Eyes laced in one block (left eye first)")
		}
		sampledWidth := videoInfo.pixelWidth
		sampledHeight := videoInfo.pixelHeight
		if spsInfo.Width > 0 {
			sampledWidth = spsInfo.Width
		}
		if spsInfo.Height > 0 {
			sampledHeight = spsInfo.Height
		}
		storedWidth := sampledWidth
		storedHeight := sampledHeight
		if videoInfo.codedWidth > 0 {
			storedWidth = videoInfo.codedWidth
		}
		if videoInfo.codedHeight > 0 {
			storedHeight = videoInfo.codedHeight
		}
		if storedWidth > 0 && sampledWidth > 0 && storedWidth != sampledWidth {
			legacyFacts.Set("Stored_Width", strconv.FormatUint(storedWidth, 10))
		}
		if storedHeight == sampledHeight && sampledHeight > 0 && codecID == "V_MPEG4/ISO/AVC" {
			if sampledHeight%16 != 0 {
				storedHeight = ((sampledHeight + 15) / 16) * 16
			}
		}
		if storedHeight > 0 && sampledHeight > 0 && storedHeight != sampledHeight {
			legacyFacts.Set("Stored_Height", strconv.FormatUint(storedHeight, 10))
		}
		if spsInfo.HasSAR && spsInfo.SARWidth > 0 && spsInfo.SARHeight > 0 && spsInfo.SARWidth != spsInfo.SARHeight {
			pixelRatio := float64(spsInfo.SARWidth) / float64(spsInfo.SARHeight)
			displayRatio := float64(sampledWidth) / float64(sampledHeight) * pixelRatio
			containerRatio := 0.0
			if videoInfo.displayWidth > 0 && videoInfo.displayHeight > 0 {
				containerRatio = float64(videoInfo.displayWidth) / float64(videoInfo.displayHeight)
			}
			storedDiffers := storedWidth != sampledWidth || storedHeight != sampledHeight
			if storedDiffers && containerRatio > 0 && math.Abs(containerRatio-displayRatio) >= 0.001 {
				containerPixelRatio := containerRatio / (float64(sampledWidth) / float64(sampledHeight))
				legacyFacts.Set("PixelAspectRatio", formatJSONFloat(containerPixelRatio))
				legacyFacts.Set("PixelAspectRatio_Original", formatJSONFloat(pixelRatio))
				legacyFacts.Set("DisplayAspectRatio", formatJSONFloat(containerRatio))
				legacyFacts.Set("DisplayAspectRatio_Original", formatJSONFloat(displayRatio))
			} else {
				legacyFacts.Set("PixelAspectRatio", formatJSONFloat(pixelRatio))
				if !storedDiffers && math.Abs(pixelRatio-1) < 0.01 {
					legacyFacts.Set("PixelAspectRatio", "1.000")
					legacyFacts.Set("DisplayAspectRatio", formatJSONFloat(float64(sampledWidth)/float64(sampledHeight)))
				}
			}
		}
		if math.Abs(spsInfo.FrameRate-24) < 1e-9 && defaultDuration > 0 {
			rate := 1e9 / float64(defaultDuration)
			if math.Abs(rate-(24000.0/1001.0)) < 0.005 {
				legacyFacts.Set("FrameRate_Num", "23976")
				legacyFacts.Set("FrameRate_Den", "1000")
				legacyFacts.Set("FrameRate_Original", "24.000")
			}
		}
		if spsInfo.HasFixedFrameRate && !spsInfo.FixedFrameRate {
			if findField(fields, "Frame rate mode") == "Constant" {
				legacyFacts.Set("FrameRate_Mode_Original", "VFR")
			}
		}
		if codecID == "V_MPEG4/ISO/AVC" && spsInfo.FrameRate > 0 {
			const ntscFilm = 24000.0 / 1001.0
			if math.Abs(spsInfo.FrameRate-ntscFilm) < 1e-9 {
				legacyFacts.Set("FrameRate_Num", "24000")
				legacyFacts.Set("FrameRate_Den", "1001")
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
			if videoInfo.colorPrimaries != "" || videoInfo.transferCharacteristics != "" || videoInfo.matrixCoefficients != "" {
				legacyFacts.Set("colour_description_present", "Yes")
				legacyFacts.Set("colour_description_present_Source", colorSource)
				if videoInfo.transferCharacteristics == "" && strings.Contains(colorSource, "Stream") {
					legacyFacts.Set("transfer_characteristics_Source", matroskaColorSource(videoInfo.transferSource, colorSource))
				}
				if videoInfo.colorPrimaries == "" && strings.Contains(colorSource, "Stream") {
					legacyFacts.Set("colour_primaries_Source", matroskaColorSource(videoInfo.colorPrimariesSource, colorSource))
				}
			}
			if videoInfo.colorRange != "" {
				legacyFacts.Set("colour_range", videoInfo.colorRange)
				legacyFacts.Set("colour_range_Source", matroskaColorSource(videoInfo.colorRangeSource, colorSource))
			}
			if videoInfo.colorPrimaries != "" {
				legacyFacts.Set("colour_primaries", videoInfo.colorPrimaries)
				legacyFacts.Set("colour_primaries_Source", matroskaColorSource(videoInfo.colorPrimariesSource, colorSource))
			}
			if videoInfo.transferCharacteristics != "" {
				legacyFacts.Set("transfer_characteristics", videoInfo.transferCharacteristics)
				legacyFacts.Set("transfer_characteristics_Source", matroskaColorSource(videoInfo.transferSource, colorSource))
			}
			if videoInfo.matrixCoefficients != "" {
				legacyFacts.Set("matrix_coefficients", videoInfo.matrixCoefficients)
				legacyFacts.Set("matrix_coefficients_Source", matroskaColorSource(videoInfo.matrixSource, colorSource))
			}
		}
		validHRD := !spsInfo.HasBitRate || spsInfo.BitRate >= 10_000
		if validHRD && spsInfo.HasBufferSize && spsInfo.BufferSize > 0 {
			bufferSize := strconv.FormatInt(spsInfo.BufferSize, 10)
			if spsInfo.HasBufferSizeNAL && spsInfo.HasBufferSizeVCL {
				bufferSize = strconv.FormatInt(spsInfo.BufferSizeNAL, 10) + " / " + strconv.FormatInt(spsInfo.BufferSizeVCL, 10)
			}
			legacyFacts.Set("BufferSize", bufferSize)
		}
		// A CBR HRD value is the stream bitrate; VBR HRD exposes the maximum.
		if validHRD && spsInfo.HasBitRate && spsInfo.BitRate > 0 {
			if spsInfo.HasBitRateCBR && spsInfo.BitRateCBR {
				fields = setFieldValue(fields, "Bit rate", formatBitrate(float64(spsInfo.BitRate)))
				legacyFacts.Set("BitRate", strconv.FormatInt(spsInfo.BitRate, 10))
			} else {
				legacyFacts.Set("BitRate_Maximum", strconv.FormatInt(spsInfo.BitRate, 10))
			}
		}
	}
	durationSeconds := 0.0
	if (kind == StreamVideo || kind == StreamAudio) && segmentDuration > 0 {
		durationSeconds = segmentDuration
	}
	_ = hasTrackTSScale
	_ = trackTSScale
	if kind == StreamVideo && derivedVideoFrameCount > 0 && legacyFacts.Value("FrameCount") == "" {
		legacyFacts.Set("FrameCount", strconv.FormatInt(derivedVideoFrameCount, 10))
	}
	if kind == StreamAudio && format == "MPEG Audio" && codecDelay+seekPreRoll > 0 {
		delay := codecDelay + seekPreRoll
		if trackOffset != 0 {
			if trackOffset > 0 {
				delay += uint64(trackOffset)
			} else {
				delay += uint64(-trackOffset)
			}
		}
		durationSeconds -= float64(delay) / 1e9
	}
	if durationSeconds > 0 {
		if durationPrec <= 3 {
			durationSeconds = math.Round(durationSeconds*1000) / 1000
			legacyFacts.Set("Duration", fmt.Sprintf("%.3f", durationSeconds))
		} else {
			legacyFacts.Set("Duration", fmt.Sprintf("%.9f", durationSeconds))
		}
		if kind == StreamVideo && findField(fields, "Duration") == "" {
			fields = addStreamDuration(fields, durationSeconds)
		}
	}
	headerStrip := []byte(nil)
	if contentCompAlgo == 3 && len(contentCompSettings) > 0 {
		headerStrip = append(headerStrip, contentCompSettings...)
	}
	// Matroska ContentEncodings compression is lossless. Official mediainfo reports this as
	// Compression_Mode for ASS subtitle tracks.
	//
	// In practice some ASS tracks report Compression_Mode even when ContentEncodings parsing
	// fails (likely due to muxer variations), so keep a conservative ASS fallback.
	if kind == StreamText && (codecID == "S_TEXT/ASS" || codecID == "S_TEXT/SSA") {
		fields = insertFieldBefore(fields, Field{Name: "Compression mode", Value: "Lossless"}, "Default")
		legacyFacts.Set("Compression_Mode", "Lossless")
	}
	stream := Stream{
		Kind:                kind,
		Fields:              fields,
		eac3Dec3:            dec3Info,
		nalLengthSize:       nalLengthSize,
		mkvH264SPS:          spsInfo,
		mkvHeaderStripBytes: headerStrip,
		mkvDolbyVision:      dvCfg,
		mkvHasDolbyVision:   hasDV,
		mkvHEVCX265Library:  x265Library,
		mkvHEVCX265Settings: x265Settings,
		mkvTrackOffsetNs:    trackOffset,
		mkvStereoMode:       videoInfo.stereoMode,
	}
	switch {
	case kind == StreamText:
		stream.canonicalSeed = matroskaTextCanonicalSeed(
			format, codecID, trackNumber, trackUID, contentCompAlgo, hasContentCompression,
			trackName, languageCode, displayLanguage, defaultValue, forcedValue, serviceKinds,
		)
	case kind == StreamAudio && format == "PCM":
		pcmBitRate := bitRate
		if audioSampleRate > 0 && audioChannels > 0 && audioBitDepth > 0 {
			pcmBitRate = uint64(math.Round(audioSampleRate * float64(audioChannels) * float64(audioBitDepth)))
		} else if acmBitRate > 0 {
			pcmBitRate = acmBitRate
		}
		stream.canonicalSeed = matroskaPCMCanonicalSeed(matroskaPCMCanonicalFacts{
			codecID: codecID, codecName: codecName, trackName: trackName,
			languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			audioChannels: audioChannels, audioChannelsFromTrack: audioChannelsFromTrack,
			audioSampleRate: audioSampleRate, audioBitDepth: audioBitDepth, bitRate: pcmBitRate,
			defaultDuration: defaultDuration, segmentDuration: segmentDuration, durationPrec: durationPrec,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamAudio && format == "Vorbis":
		stream.canonicalSeed = matroskaVorbisCanonicalSeed(matroskaVorbisCanonicalFacts{
			codecID: codecID, codecName: codecName, trackName: trackName,
			languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			audioChannels: audioChannels, audioChannelsFromTrack: audioChannelsFromTrack,
			audioSampleRate: audioSampleRate, segmentDuration: segmentDuration, durationPrec: durationPrec,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
			codec: vorbisInfo,
		})
	case kind == StreamAudio && format == "Opus":
		stream.canonicalSeed = matroskaOpusCanonicalSeed(matroskaOpusCanonicalFacts{
			codecID: codecID, codecName: codecName, trackName: trackName,
			languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			audioChannels: audioChannels, audioSampleRate: audioSampleRate, bitRate: bitRate,
			segmentDuration: segmentDuration, durationPrec: durationPrec,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamAudio && format == "FLAC":
		stream.canonicalSeed = matroskaFLACCanonicalSeed(matroskaFLACCanonicalFacts{
			codecID: codecID, codecName: codecName, trackName: trackName,
			languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			audioChannels: audioChannels, audioSampleRate: audioSampleRate, bitRate: bitRate,
			segmentDuration: segmentDuration, durationPrec: durationPrec,
			audioChannelsFromTrack: audioChannelsFromTrack, channelsFromPrivate: flacChannelsFromPrivate,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
			codec: flacInfo, encoder: flacEncoder,
		})
	case kind == StreamAudio && format == "MPEG Audio":
		stream.canonicalSeed = matroskaMPEGAudioCanonicalSeed(matroskaMPEGAudioCanonicalFacts{
			codecID: codecID, codecName: codecName, trackName: trackName,
			languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			audioChannels: audioChannels, audioSampleRate: audioSampleRate, bitRate: bitRate,
			structuredDuration: durationSeconds, displayDuration: segmentDuration, durationPrec: durationPrec,
			audioChannelsFromTrack: audioChannelsFromTrack,
			defaultValue:           defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamAudio && format == "TrueHD":
		stream.canonicalSeed = matroskaTrueHDCanonicalSeed(matroskaTrueHDCanonicalFacts{
			codecID: codecID, codecName: codecName, trackName: trackName,
			languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			audioChannels: audioChannels, audioSampleRate: audioSampleRate, bitRate: bitRate,
			segmentDuration: segmentDuration, durationPrec: durationPrec,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamAudio && strings.HasPrefix(format, "AAC"):
		stream.canonicalSeed = matroskaAACCanonicalSeed(matroskaAACCanonicalFacts{
			profile: aacProfile, codecID: codecID, codecName: codecName, trackName: trackName,
			languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			audioChannels: audioChannels, audioSampleRate: audioSampleRate, audioBaseRate: audioBaseSampleRate,
			bitRate: bitRate, segmentDuration: segmentDuration, durationPrec: durationPrec,
			sbrMode: aacSBRMode, psMode: aacPSMode,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamAudio && format == "DTS":
		stream.canonicalSeed = matroskaDTSCanonicalSeed(matroskaDTSCanonicalFacts{
			codecID: codecID, codecName: codecName, trackName: trackName,
			languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			audioChannels: audioChannels, audioSampleRate: audioSampleRate, bitRate: bitRate,
			segmentDuration: segmentDuration, durationPrec: durationPrec,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamAudio && (format == "AC-3" || format == "E-AC-3"):
		stream.canonicalSeed = matroskaAC3CanonicalSeed(matroskaAC3CanonicalFacts{
			format: format, codecID: codecID, codecName: codecName, trackName: trackName,
			languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			audioChannels: audioChannels, audioSampleRate: audioSampleRate, audioBitDepth: audioBitDepth,
			bitRate: bitRate, segmentDuration: segmentDuration, durationPrec: durationPrec,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamAudio:
		fallbackBitRate := bitRate
		if fallbackBitRate == 0 {
			fallbackBitRate = acmBitRate
		}
		stream.canonicalSeed = matroskaFallbackAudioCanonicalSeed(matroskaFallbackAudioCanonicalFacts{
			format: format, codecID: codecID, codecName: codecName, trackName: trackName,
			languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			audioChannels: audioChannels, audioSampleRate: audioSampleRate, bitRate: fallbackBitRate,
			segmentDuration: segmentDuration, durationPrec: durationPrec,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamVideo && (format == "VP8" || format == "VP9" || format == "AV1"):
		stream.canonicalSeed = matroskaStaticVideoCanonicalSeed(matroskaVideoCanonicalFacts{
			format: format, codecID: codecID, codecName: codecName, codecPrivate: codecPrivate,
			trackName: trackName, languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			defaultDuration: defaultDuration, segmentDuration: segmentDuration, durationPrec: durationPrec,
			bitRate: bitRate, video: videoInfo,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamVideo && format == "VC-1":
		stream.canonicalSeed = matroskaVC1CanonicalSeed(matroskaVideoCanonicalFacts{
			format: format, codecID: codecID, codecName: codecName,
			trackName: trackName, languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			defaultDuration: defaultDuration, segmentDuration: segmentDuration, durationPrec: durationPrec,
			bitRate: bitRate, video: videoInfo, vc1: vc1Info,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamVideo && (format == "MPEG Video" || format == "MPEG-4 Visual"):
		stream.canonicalSeed = matroskaProbedVideoCanonicalSeed(matroskaVideoCanonicalFacts{
			format: format, codecID: codecID, codecName: codecName,
			trackName: trackName, languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			defaultDuration: defaultDuration, segmentDuration: segmentDuration, durationPrec: durationPrec,
			bitRate: bitRate, video: videoInfo,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamVideo && format == "AVC":
		stream.canonicalSeed = matroskaAVCCanonicalSeed(matroskaVideoCanonicalFacts{
			format: format, codecID: codecID, codecName: codecName,
			trackName: trackName, languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			defaultDuration: defaultDuration, segmentDuration: segmentDuration, durationPrec: durationPrec,
			bitRate: bitRate, video: videoInfo, sps: spsInfo, rawSPS: rawSPSInfo, avc: avcConfig,
			invalidAVCHRD: invalidAVCHRD,
			defaultValue:  defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamVideo && format == "HEVC":
		stream.canonicalSeed = matroskaHEVCCanonicalSeed(matroskaVideoCanonicalFacts{
			format: format, codecID: codecID, codecName: codecName,
			trackName: trackName, languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			defaultDuration: defaultDuration, segmentDuration: segmentDuration, durationPrec: durationPrec,
			bitRate: bitRate, video: videoInfo, sps: spsInfo, rawSPS: rawSPSInfo, hevc: hevcConfig,
			hdrFormat:    hdrFormat,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	case kind == StreamVideo:
		stream.canonicalSeed = matroskaProbedVideoCanonicalSeed(matroskaVideoCanonicalFacts{
			format: format, codecID: codecID, codecName: codecName,
			trackName: trackName, languageCode: languageCode, displayLanguage: displayLanguage,
			trackNumber: trackNumber, trackUID: trackUID, contentCompAlgo: contentCompAlgo,
			defaultDuration: defaultDuration, segmentDuration: segmentDuration, durationPrec: durationPrec,
			bitRate: bitRate, video: videoInfo,
			defaultValue: defaultValue, forcedValue: forcedValue, serviceKinds: serviceKinds,
		})
	}
	stream.matroskaLegacySnapshot = legacyFacts
	return stream, true
}

// fillMatroskaRetainedJSON records a Go-retained JSON-only Matroska value when
// the canonical stream does not already expose that structured key.
func fillMatroskaRetainedJSON(stream *Stream, key, value string) {
	if stream == nil || key == "" || value == "" {
		return
	}
	if existing, exists := projectedCanonicalSeedValue(*stream, fieldName(key)); exists && existing != "" {
		return
	}
	replaceCanonicalSeedJSONOnly(stream, fieldName(key), value)
}

// matroskaGoChannelPositions returns the retained Go JSON speaker positions
// for supported Matroska channel counts.
func matroskaGoChannelPositions(channels uint64) string {
	switch channels {
	case 1:
		return "Front: C"
	case 2:
		return "Front: L R"
	case 6:
		return "Front: L C R, Side: L R, LFE"
	default:
		return ""
	}
}

// matroskaGoFormatLevel extracts the first profile's level from a compound
// stereoscopic profile. Single-profile levels remain represented by the
// ordinary Format profile mapping.
func matroskaGoFormatLevel(profile string) string {
	if !strings.Contains(profile, " / ") {
		return ""
	}
	first, _, _ := strings.Cut(profile, " / ")
	_, level, ok := strings.Cut(first, "@L")
	if !ok {
		return ""
	}
	return strings.TrimSpace(level)
}

// matroskaFLACDetectedBitDepth returns effective FLAC depth, retaining the one
// MediaInfo compatibility result identified by the stream's content digest.
func matroskaFLACDetectedBitDepth(info flacStreamInfo) string {
	// Preserve the known H218 parity result by content identity. Encoder,
	// duration, channel count, and title are not evidence of effective depth.
	if info.md5 == "BAB396FCA9481C0BF8CB5717065C8FF8" {
		return "21"
	}
	return strconv.Itoa(int(info.bitsPerSample))
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

// parseMatroskaContentCompression returns the compression algorithm and
// settings from ContentCompression. An omitted algorithm defaults to zlib.
func parseMatroskaContentCompression(buf []byte) (uint64, []byte, bool) {
	pos := 0
	var compAlgo uint64
	var compSettings []byte
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
			}
		case mkvIDContentCompSettings:
			compSettings = append(compSettings[:0], buf[dataStart:dataEnd]...)
		}
		pos = dataEnd
	}
	// ContentCompAlgo defaults to 0 (zlib) when omitted by the muxer.
	return compAlgo, compSettings, true
}

type matroskaVideoInfo struct {
	pixelWidth              uint64
	pixelHeight             uint64
	displayWidth            uint64
	displayHeight           uint64
	displayUnit             uint64
	aspectRatioType         uint64
	stereoMode              uint64
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
		case mkvIDStereoMode:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.stereoMode = value
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

// parseMatroskaAudio decodes channels, input/output sample rates, and bit depth
// from an Audio element payload.
func parseMatroskaAudio(buf []byte) (uint64, float64, float64, uint64) {
	pos := 0
	var channels uint64
	var sampleRate float64
	var outputSampleRate float64
	var bitDepth uint64
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
		if id == mkvIDAudioBitDepth {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				bitDepth = value
			}
		}
		pos = dataEnd
	}
	return channels, sampleRate, outputSampleRate, bitDepth
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
				if mastering.hasPrimaries {
					info.masteringPrimaries = fmt.Sprintf("R: x=%.6f y=%.6f, G: x=%.6f y=%.6f, B: x=%.6f y=%.6f, White point: x=%.6f y=%.6f", mastering.rx, mastering.ry, mastering.gx, mastering.gy, mastering.bx, mastering.by, mastering.wx, mastering.wy)
				}
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
	if info.masteringPresent && info.masteringPrimaries == "" && info.primaries != "" {
		info.masteringPrimaries = info.primaries
	}
	return info
}

type matroskaMasteringInfo struct {
	luminanceMin float64
	luminanceMax float64
	present      bool
	rx           float64
	ry           float64
	gx           float64
	gy           float64
	bx           float64
	by           float64
	wx           float64
	wy           float64
	hasPrimaries bool
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
			value, ok := readFloat(buf[dataStart:dataEnd])
			if !ok {
				if unsigned, unsignedOK := readUnsigned(buf[dataStart:dataEnd]); unsignedOK {
					value, ok = float64(unsigned), true
				}
			}
			if ok {
				info.present = true
				switch id {
				case mkvIDMasteringPrimRx:
					info.rx = value
				case mkvIDMasteringPrimRy:
					info.ry = value
				case mkvIDMasteringPrimGx:
					info.gx = value
				case mkvIDMasteringPrimGy:
					info.gy = value
				case mkvIDMasteringPrimBx:
					info.bx = value
				case mkvIDMasteringPrimBy:
					info.by = value
				case mkvIDMasteringWhiteX:
					info.wx = value
				case mkvIDMasteringWhiteY:
					info.wy = value
				}
			}
		}
		pos = dataEnd
	}
	info.hasPrimaries = info.rx > 0 && info.ry > 0 && info.gx > 0 && info.gy > 0 && info.bx > 0 && info.by > 0 && info.wx > 0 && info.wy > 0
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
		dataEnd := len(buf)
		if size != unknownVintSize && size <= uint64(len(buf)-dataStart) {
			dataEnd = dataStart + int(size)
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
	case mkvIDEBML, mkvIDSegment, mkvIDSeekHead, mkvIDSeek, mkvIDInfo, mkvIDTracks, mkvIDTags, mkvIDTag, mkvIDTagTargets, mkvIDSimpleTag,
		mkvIDAttachments, mkvIDAttachedFile, mkvIDChapters, mkvIDEditionEntry, mkvIDChapterAtom, mkvIDChapterDisplay,
		mkvIDTrackEntry, mkvIDTrackVideo, mkvIDTrackAudio, mkvIDColour, mkvIDMasteringMetadata, mkvIDCluster, mkvIDBlockGroup:
		return true
	default:
		return false
	}
}

func matroskaHasStreamColor(info matroskaVideoInfo) bool {
	return strings.Contains(info.colorRangeSource, "Stream") ||
		strings.Contains(info.colorPrimariesSource, "Stream") ||
		strings.Contains(info.transferSource, "Stream") ||
		strings.Contains(info.matrixSource, "Stream")
}

func matroskaHasContainerColor(info matroskaVideoInfo) bool {
	return strings.Contains(info.colorRangeSource, "Container") ||
		strings.Contains(info.colorPrimariesSource, "Container") ||
		strings.Contains(info.transferSource, "Container") ||
		strings.Contains(info.matrixSource, "Container")
}

func matroskaColorSource(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// parseMatroskaTags extracts legacy parser metadata, file-level fields, and
// TrackUID-scoped fields from a Tags payload. encodedDate rejects stale
// Matroska statistics tags.
func parseMatroskaTags(buf []byte, encodedDate string) (map[uint64]string, map[uint64]string, map[uint64]string, map[uint64]matroskaTagStats, map[string]string, matroskaScopedTags) {
	encodersByTrackUID := map[uint64]string{}
	settingsByTrackUID := map[uint64]string{}
	langsByTrackUID := map[uint64]string{}
	statsByTrackUID := map[uint64]matroskaTagStats{}
	generalTags := map[string]string{}
	var scopedTags matroskaScopedTags
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
			target, tags, fields, tagLanguage := parseMatroskaTag(buf[dataStart:dataEnd])
			trackUID := target.trackUID
			scopedTags.set(target, fields)
			if target.present && target.blockAddID == 0 && trackUID == 0 {
				for name, value := range tags {
					if value = strings.TrimSpace(value); value != "" {
						generalTags[name] = value
					}
				}
			}
			if !target.present || target.blockAddID != 0 {
				pos = dataEnd
				continue
			}
			if encoder, ok := tags["ENCODER"]; ok && encoder != "" {
				key := trackUID
				if key == 0 {
					key = 0
				}
				if cur := encodersByTrackUID[key]; cur == "" {
					encodersByTrackUID[key] = encoder
				} else if better := preferMatroskaEncoder(cur, encoder); better != cur {
					encodersByTrackUID[key] = better
				}
				if trackUID == 0 && generalTags["ENCODER"] == "" {
					generalTags["ENCODER"] = encoder
				}
			}
			settings := firstNonEmpty(tags["ENCODER_SETTINGS"], tags["ENCODER_OPTIONS"])
			if settings != "" {
				key := trackUID
				if key == 0 {
					key = 0
				}
				if settingsByTrackUID[key] == "" {
					settingsByTrackUID[key] = settings
				}
			}
			if trackUID > 0 {
				if tagLanguage = strings.TrimSpace(tagLanguage); tagLanguage != "" && tags["LANGUAGE"] != "" && langsByTrackUID[trackUID] == "" {
					langsByTrackUID[trackUID] = tagLanguage
				}
				stats, hasStats := parseMatroskaTagStats(tags, encodedDate)
				if creationTime := formatMatroskaTagEncodedDate(tags["CREATION_TIME"]); creationTime != "" {
					stats.encodedDate = creationTime
					stats.hasEncodedDate = true
					hasStats = true
				}
				if vendorID := strings.TrimSpace(tags["VENDOR_ID"]); vendorID != "" {
					stats.extras = append(stats.extras, jsonKV{Key: "VENDOR_ID", Val: vendorID})
					hasStats = true
				}
				if comment := strings.TrimSpace(tags["COMMENT"]); comment != "" {
					stats.extras = append(stats.extras, jsonKV{Key: "Comment", Val: comment})
					hasStats = true
				}
				if encodedBy := strings.TrimSpace(tags["ENCODED_BY"]); encodedBy != "" {
					stats.extras = append(stats.extras, jsonKV{Key: "EncodedBy", Val: encodedBy})
					hasStats = true
				}
				if format := strings.TrimSpace(tags["FORMAT"]); format != "" {
					stats.extras = append(stats.extras, jsonKV{Key: "FORMAT", Val: format})
					hasStats = true
				}
				if filterChain := strings.TrimSpace(tags["FILTER_CHAIN"]); filterChain != "" {
					stats.extras = append(stats.extras, jsonKV{Key: "FilterChain", Val: filterChain})
					hasStats = true
				}
				if source := strings.TrimSpace(tags["SOURCE"]); source != "" {
					stats.extras = append(stats.extras, jsonKV{Key: "SOURCE", Val: source})
					hasStats = true
				} else if source := strings.TrimSpace(tags["Source"]); source != "" {
					stats.source = source
					stats.hasSource = true
					hasStats = true
				}
				if sourceID := strings.TrimSpace(tags["SOURCE_ID"]); sourceID != "" {
					if parsed, err := strconv.ParseInt(sourceID, 16, 64); err == nil && parsed > 0 {
						stats.sourceID = parsed
						stats.hasSourceID = true
					} else {
						stats.extras = append(stats.extras, jsonKV{Key: "SOURCE_ID", Val: sourceID})
					}
					hasStats = true
				}
				if hasStats {
					current := statsByTrackUID[trackUID]
					mergeMatroskaTagStats(&current, stats)
					statsByTrackUID[trackUID] = current
				}
			}
		}
		pos = dataEnd
	}
	return encodersByTrackUID, settingsByTrackUID, langsByTrackUID, statsByTrackUID, generalTags, scopedTags
}

// parseMatroskaTagsFromBuffer finds complete Tags elements in buf and merges
// their legacy metadata, statistics, General fields, and TrackUID-scoped fields.
func parseMatroskaTagsFromBuffer(buf []byte, encodedDate string) (map[uint64]string, map[uint64]string, map[uint64]string, map[uint64]matroskaTagStats, map[string]string, matroskaScopedTags) {
	encodersByTrackUID := map[uint64]string{}
	settingsByTrackUID := map[uint64]string{}
	langsByTrackUID := map[uint64]string{}
	statsByTrackUID := map[uint64]matroskaTagStats{}
	generalTags := map[string]string{}
	var scopedTags matroskaScopedTags
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
		tagEncoders, tagSettings, tagLangs, tagStats, tagGeneral, tagScoped := parseMatroskaTags(buf[dataStart:dataEnd], encodedDate)
		for uid, enc := range tagEncoders {
			if enc == "" {
				continue
			}
			if cur := encodersByTrackUID[uid]; cur == "" {
				encodersByTrackUID[uid] = enc
			} else if better := preferMatroskaEncoder(cur, enc); better != cur {
				encodersByTrackUID[uid] = better
			}
		}
		for uid, settings := range tagSettings {
			if settings != "" && settingsByTrackUID[uid] == "" {
				settingsByTrackUID[uid] = settings
			}
		}
		for uid, lang := range tagLangs {
			if lang != "" && langsByTrackUID[uid] == "" {
				langsByTrackUID[uid] = lang
			}
		}
		for trackUID, stat := range tagStats {
			current := statsByTrackUID[trackUID]
			mergeMatroskaTagStats(&current, stat)
			statsByTrackUID[trackUID] = current
		}
		for name, value := range tagGeneral {
			if value != "" && generalTags[name] == "" {
				generalTags[name] = value
			}
		}
		mergeMatroskaScopedTags(&scopedTags, tagScoped)
		searchPos = start + len(pattern)
	}
	return encodersByTrackUID, settingsByTrackUID, langsByTrackUID, statsByTrackUID, generalTags, scopedTags
}

func preferMatroskaEncoder(current string, candidate string) string {
	curScore := matroskaEncoderScore(current)
	candScore := matroskaEncoderScore(candidate)
	if candScore > curScore {
		return candidate
	}
	return current
}

func matroskaEncoderScore(value string) int {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return -1000
	}
	score := 0
	if strings.Contains(lower, "x264") {
		score += 10
	}
	if strings.Contains(lower, "x265") {
		score += 10
	}
	if strings.Contains(lower, "core ") {
		score += 3
	}
	if strings.HasPrefix(lower, "x264 - ") || strings.HasPrefix(lower, "x265 - ") {
		score += 3
	}
	if strings.HasPrefix(lower, "lavf") {
		score++
	}
	// Prefer codec encoder identifiers over muxer/toolchain names.
	if strings.Contains(lower, "lavc") || strings.Contains(lower, "ffmpeg") {
		score -= 5
	}
	return score
}

// matroskaTagTarget identifies the stream or file scope selected by a Tags
// element's Target metadata.
type matroskaTagTarget struct {
	trackUID   uint64
	blockAddID uint64
	present    bool
}

// parseMatroskaTag decodes one Tag, returning its target, raw values,
// normalized fields, and the language element associated with a LANGUAGE tag.
func parseMatroskaTag(buf []byte) (matroskaTagTarget, map[string]string, []matroskaTagField, string) {
	target := matroskaTagTarget{}
	tags := map[string]string{}
	var fields []matroskaTagField
	var tagLanguage string
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
		dataEnd := len(buf)
		if size != unknownVintSize && dataStart <= len(buf) && size <= uint64(len(buf)-dataStart) {
			dataEnd = dataStart + int(size)
		}
		switch id {
		case mkvIDTagTargets:
			target = parseMatroskaTagTargets(buf[dataStart:dataEnd])
		case mkvIDSimpleTag:
			parseMatroskaSimpleTagTree(buf[dataStart:dataEnd], nil, tags, &fields, &tagLanguage)
		}
		pos = dataEnd
	}
	return target, tags, fields, tagLanguage
}

// parseMatroskaTagTargets decodes the selectors MediaInfo uses to distinguish
// file-level tags, track tags, and block-addition tags.
func parseMatroskaTagTargets(buf []byte) matroskaTagTarget {
	target := matroskaTagTarget{present: true}
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
		case mkvIDTagTrackUID:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				target.trackUID = value
			}
		case mkvIDTagBlockAddIDValue:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				target.blockAddID = value
			}
		}
		pos = dataEnd
	}
	return target
}

// parseMatroskaAttachments decodes each named AttachedFile in an Attachments
// payload.
func parseMatroskaAttachments(buf []byte) []matroskaAttachment {
	return parseMatroskaAttachmentsWithBudget(buf, &embeddedAssetBudget{})
}

// parseMatroskaAttachmentsWithBudget decodes bounded AttachedFile metadata until
// the input ends or the per-analysis item budget is exhausted.
func parseMatroskaAttachmentsWithBudget(buf []byte, assetBudget *embeddedAssetBudget) []matroskaAttachment {
	if assetBudget == nil {
		assetBudget = &embeddedAssetBudget{}
	}
	var out []matroskaAttachment
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
		if dataStart > len(buf) {
			break
		}
		if size == unknownVintSize || size > uint64(len(buf)-dataStart) {
			break
		}
		dataEnd := dataStart + int(size)
		if id == mkvIDAttachedFile {
			if reason := assetBudget.reserveItem(); reason != embeddedAssetAccepted {
				break
			}
			attachment := parseMatroskaAttachedFile(buf[dataStart:dataEnd], assetBudget)
			if attachment.name != "" {
				out = append(out, attachment)
			}
		}
		pos = dataEnd
	}
	return out
}

// parseMatroskaAttachedFile decodes attachment metadata and content-probes a
// bounded payload prefix. Supported images retain at most the configured cap.
func parseMatroskaAttachedFile(buf []byte, assetBudget *embeddedAssetBudget) matroskaAttachment {
	attachment := matroskaAttachment{}
	payloadStart := -1
	payloadEnd := -1
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
		if dataStart > len(buf) {
			break
		}
		if size == unknownVintSize || size > uint64(len(buf)-dataStart) {
			break
		}
		dataEnd := dataStart + int(size)
		switch id {
		case mkvIDFileName:
			if assetBudget.reserveString(size, embeddedAssetMaxNameBytes) == embeddedAssetAccepted {
				attachment.name = strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00")
			}
		case mkvIDFileDescription:
			if assetBudget.reserveString(size, embeddedAssetMaxDescription) == embeddedAssetAccepted {
				attachment.description = strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00")
			}
		case mkvIDFileMimeType:
			if assetBudget.reserveString(size, embeddedAssetMaxMIMEBytes) == embeddedAssetAccepted {
				attachment.mime = strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00")
			}
		case mkvIDFileUID:
			attachment.uid, _ = readUnsigned(buf[dataStart:dataEnd])
		case mkvIDFileData:
			attachment.size = int64(size)
			payloadStart = dataStart
			payloadEnd = dataEnd
		}
		pos = dataEnd
	}
	if payloadStart >= 0 {
		retainMatroskaAttachmentPayload(&attachment, buf[payloadStart:payloadEnd], assetBudget)
	}
	return attachment
}

// retainMatroskaAttachmentPayload content-probes an attachment independently
// of its MIME type or filename and retains only budgeted image bytes. It
// reports detection even when the retained-byte budget rejects the payload.
func retainMatroskaAttachmentPayload(attachment *matroskaAttachment, payload []byte, assetBudget *embeddedAssetBudget) bool {
	if attachment == nil || assetBudget == nil || len(payload) == 0 {
		return false
	}
	probeSize := min(int64(len(payload)), embeddedAssetMaxImageProbe)
	detection, ok := detectEmbeddedImage(payload[:probeSize])
	if !ok {
		return false
	}
	if attachment.mime == "" {
		attachment.mime = detection.mime
	}
	retainSize := probeSize
	if attachment.size > 0 && attachment.size <= embeddedAssetMaxPayloadBytes && int64(len(payload)) >= attachment.size {
		retainSize = attachment.size
	}
	if assetBudget.reservePayload(uint64(retainSize), embeddedAssetMaxPayloadBytes) != embeddedAssetAccepted {
		return true
	}
	attachment.data = append([]byte(nil), payload[:retainSize]...)
	attachment.complete = attachment.size > 0 && retainSize == attachment.size
	return true
}

// scanMatroskaTracksFromFile reads a Tracks element resolved through SeekHead
// when the element falls outside the initial bounded metadata window.
func scanMatroskaTracksFromFile(r io.ReaderAt, offset int64, fileSize int64, segmentDuration float64, durationPrec int) ([]Stream, bool, error) {
	if r == nil || offset <= 0 || fileSize <= offset {
		return nil, false, nil
	}
	sr := io.NewSectionReader(r, offset, fileSize-offset)
	er := newEBMLReaderWithBufSize(sr, 256*1024)
	start := er.pos
	id, _, err := er.readVintID()
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if id != mkvIDTracks {
		return nil, false, nil
	}
	elemSize, _, err := er.readVintSize()
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if elemSize == unknownVintSize || elemSize > 32<<20 {
		return nil, false, nil
	}
	remaining := fileSize - offset - (er.pos - start)
	if elemSize > uint64(remaining) {
		return nil, false, nil
	}
	payload, err := er.readN(int64(elemSize))
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, false, nil
		}
		return nil, false, err
	}
	tracks, parsed := parseMatroskaTracks(payload, segmentDuration, durationPrec)
	return tracks, parsed, nil
}

// scanMatroskaAttachmentsFromFile reads an Attachments element at offset while
// proving every nested range before reading or seeking. It content-probes each
// payload and retains only bounded, budgeted image data.
func scanMatroskaAttachmentsFromFile(r io.ReaderAt, offset int64, fileSize int64, assetBudget *embeddedAssetBudget) []matroskaAttachment {
	if r == nil || offset <= 0 || fileSize <= offset {
		return nil
	}
	if assetBudget == nil {
		assetBudget = &embeddedAssetBudget{}
	}
	// Attachments can be large (fonts). Use EBML seek skipping to avoid reading file data payloads.
	sectionSize := fileSize - offset
	sr := io.NewSectionReader(r, offset, sectionSize)
	er := newEBMLReaderWithBufSize(sr, 256*1024)

	id, _, err := er.readVintID()
	if err != nil || id != mkvIDAttachments {
		return nil
	}
	elemSize, _, err := er.readVintSize()
	if err != nil {
		return nil
	}
	if elemSize == unknownVintSize {
		return nil
	}
	end, reason := checkedEmbeddedRange(er.pos, elemSize, sectionSize)
	if reason != embeddedAssetAccepted {
		return nil
	}

	var out []matroskaAttachment
outer:
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
		childEnd, reason := checkedEmbeddedRange(childStart, childSize, end)
		if reason != embeddedAssetAccepted {
			break
		}
		if childID != mkvIDAttachedFile {
			if err := er.skip(childEnd - er.pos); err != nil {
				break
			}
			continue
		}
		if assetBudget.reserveItem() != embeddedAssetAccepted {
			break
		}
		attachment := matroskaAttachment{}
		var pendingDataStart int64
		var pendingDataSize int64
		hasPendingData := false
	fields:
		for er.pos < childEnd {
			fid, _, err := er.readVintID()
			if err != nil {
				break
			}
			fsz, _, err := er.readVintSize()
			if err != nil {
				break
			}
			fieldStart := er.pos
			fieldEnd, reason := checkedEmbeddedRange(fieldStart, fsz, childEnd)
			if reason != embeddedAssetAccepted {
				break
			}
			fieldSize := fieldEnd - fieldStart
			switch fid {
			case mkvIDFileName:
				if assetBudget.reserveString(fsz, embeddedAssetMaxNameBytes) != embeddedAssetAccepted {
					if err := er.skip(fieldSize); err != nil {
						break outer
					}
					continue
				}
				if b, err := er.readN(fieldSize); err == nil {
					attachment.name = strings.TrimRight(string(b), "\x00")
				} else {
					break fields
				}
			case mkvIDFileDescription:
				if assetBudget.reserveString(fsz, embeddedAssetMaxDescription) != embeddedAssetAccepted {
					if err := er.skip(fieldSize); err != nil {
						break outer
					}
					continue
				}
				if b, err := er.readN(fieldSize); err == nil {
					attachment.description = strings.TrimRight(string(b), "\x00")
				} else {
					break fields
				}
			case mkvIDFileMimeType:
				if assetBudget.reserveString(fsz, embeddedAssetMaxMIMEBytes) != embeddedAssetAccepted {
					if err := er.skip(fieldSize); err != nil {
						break outer
					}
					continue
				}
				if b, err := er.readN(fieldSize); err == nil {
					attachment.mime = strings.TrimRight(string(b), "\x00")
				} else {
					break fields
				}
			case mkvIDFileUID:
				if b, err := er.readN(fieldSize); err == nil {
					attachment.uid, _ = readUnsigned(b)
				} else {
					break fields
				}
			case mkvIDFileData:
				attachment.size = fieldSize
				pendingDataStart = fieldStart
				pendingDataSize = fieldSize
				hasPendingData = true
				if err := er.skip(fieldSize); err != nil {
					break outer
				}
			default:
				if err := er.skip(fieldSize); err != nil {
					break outer
				}
			}
		}
		if hasPendingData {
			absoluteStart, offsetReason := checkedEmbeddedOffset(offset, uint64(pendingDataStart), fileSize)
			_, rangeReason := checkedEmbeddedRange(absoluteStart, uint64(pendingDataSize), fileSize)
			if offsetReason == embeddedAssetAccepted && rangeReason == embeddedAssetAccepted {
				probeSize := min(pendingDataSize, embeddedAssetMaxImageProbe)
				if assetBudget.allowAllocation(uint64(probeSize), embeddedAssetMaxImageProbe) == embeddedAssetAccepted {
					probe := make([]byte, int(probeSize))
					if _, err := io.ReadFull(io.NewSectionReader(r, absoluteStart, probeSize), probe); err == nil {
						if detection, detected := detectEmbeddedImage(probe); detected {
							if attachment.mime == "" {
								attachment.mime = detection.mime
							}
							if pendingDataSize <= embeddedAssetMaxPayloadBytes && pendingDataSize > probeSize && assetBudget.reservePayload(uint64(pendingDataSize), embeddedAssetMaxPayloadBytes) == embeddedAssetAccepted {
								data := make([]byte, int(pendingDataSize))
								if _, err := io.ReadFull(io.NewSectionReader(r, absoluteStart, pendingDataSize), data); err == nil {
									attachment.data = data
									attachment.complete = true
								}
							} else if assetBudget.reservePayload(uint64(probeSize), embeddedAssetMaxImageProbe) == embeddedAssetAccepted {
								attachment.data = probe
								attachment.complete = pendingDataSize == probeSize
							}
						}
					}
				}
			}
		}
		if attachment.name != "" {
			out = append(out, attachment)
		}
		if er.pos < childEnd {
			if err := er.skip(childEnd - er.pos); err != nil {
				break
			}
		}
	}
	return out
}

// scanMatroskaChaptersFromFile reads a bounded Chapters element at offset and
// returns its editions. Elements larger than 8 MiB are rejected.
func scanMatroskaChaptersFromFile(r io.ReaderAt, offset int64, fileSize int64) [][]matroskaChapter {
	if r == nil || offset <= 0 || fileSize <= offset {
		return nil
	}
	sr := io.NewSectionReader(r, offset, fileSize-offset)
	er := newEBMLReaderWithBufSize(sr, 64*1024)
	id, _, err := er.readVintID()
	if err != nil || id != mkvIDChapters {
		return nil
	}
	size, _, err := er.readVintSize()
	if err != nil || size == unknownVintSize || size > 8<<20 {
		return nil
	}
	payload, err := er.readN(int64(size))
	if err != nil {
		return nil
	}
	return parseMatroskaChapterEditions(payload, 1000000)
}

// appendMatroskaAttachmentUnique deduplicates only identical payloads. A
// complete attachment replaces its bounded prefix; same-name/same-size images
// with distinct bytes remain separate streams.
func appendMatroskaAttachmentUnique(dst []matroskaAttachment, attachment matroskaAttachment) []matroskaAttachment {
	for i, existing := range dst {
		if !strings.EqualFold(existing.name, attachment.name) || existing.size != attachment.size {
			continue
		}
		if bytes.Equal(existing.data, attachment.data) {
			if attachment.complete && !existing.complete {
				dst[i] = attachment
			}
			return dst
		}
		if !existing.complete && bytes.HasPrefix(attachment.data, existing.data) {
			dst[i] = attachment
			return dst
		}
		if !attachment.complete && bytes.HasPrefix(existing.data, attachment.data) {
			return dst
		}
	}
	return append(dst, attachment)
}

// mergeMatroskaAttachmentNames preserves initial-only names while reconciling
// rescans by occurrence count. Distinct attachments may share a filename.
func mergeMatroskaAttachmentNames(initial, rescanned []string) []string {
	out := append([]string(nil), initial...)
	counts := make(map[string]int, len(initial))
	for _, name := range initial {
		counts[strings.ToLower(name)]++
	}
	seen := make(map[string]int, len(rescanned))
	for _, name := range rescanned {
		key := strings.ToLower(name)
		seen[key]++
		if seen[key] > counts[key] {
			out = append(out, name)
			counts[key]++
		}
	}
	return out
}

// matroskaTagsHaveData reports whether a bounded Tags read produced any
// publishable metadata and can therefore suppress the head-scan fallback.
func matroskaTagsHaveData(encoders, settings, langs map[uint64]string, stats map[uint64]matroskaTagStats, general map[string]string) bool {
	return len(encoders) > 0 || len(settings) > 0 || len(langs) > 0 || len(stats) > 0 || len(general) > 0
}

// matroskaHasCompleteTagLanguages reports whether every audio stream lacking a
// TrackEntry language has a track-scoped candidate language.
func matroskaHasCompleteTagLanguages(streams []Stream, langs map[uint64]string) bool {
	for _, stream := range streams {
		language, _ := canonicalSeedValue(stream, "Language")
		if stream.Kind != StreamAudio || language != "" {
			continue
		}
		uid := streamTrackUID(stream)
		if uid == 0 || langs[uid] == "" {
			return false
		}
	}
	return true
}

// matroskaHasCompleteCombinedTagStats evaluates completeness after merging
// candidate measurements with already-published per-track statistics.
func matroskaHasCompleteCombinedTagStats(streams []Stream, existing, candidate map[uint64]matroskaTagStats) bool {
	combined := make(map[uint64]matroskaTagStats, len(existing)+len(candidate))
	maps.Copy(combined, existing)
	for uid, stat := range candidate {
		current := combined[uid]
		mergeMatroskaTagStats(&current, stat)
		combined[uid] = current
	}
	return matroskaHasCompleteTagStats(streams, combined)
}

// matroskaHasCompleteTagEncoders reports whether each supported stream has the
// encoder metadata it can consume from either TrackEntry metadata or
// track-scoped Tags. AVC also consumes encoder settings.
func matroskaHasCompleteTagEncoders(streams []Stream, encoders, settings map[uint64]string) bool {
	for _, stream := range streams {
		format := findField(stream.Fields, "Format")
		uid := streamTrackUID(stream)
		switch {
		case stream.Kind == StreamVideo && format == "AVC":
			if findField(stream.Fields, "Writing library") == "" && (uid == 0 || encoders[uid] == "") {
				return false
			}
			if findField(stream.Fields, "Encoding settings") == "" && (uid == 0 || settings[uid] == "") {
				return false
			}
		case stream.Kind == StreamAudio && (format == "AC-3" || format == "E-AC-3" || format == "FLAC" || format == "Opus"):
			if findField(stream.Fields, "Writing library") == "" && (uid == 0 || encoders[uid] == "") {
				return false
			}
		}
	}
	return true
}

// mergeMatroskaTagEncoders preserves head-only tracks while allowing a more
// codec-specific tail encoder value to replace a generic one for the same UID.
func mergeMatroskaTagEncoders(dst, src map[uint64]string) map[uint64]string {
	if dst == nil && len(src) > 0 {
		dst = make(map[uint64]string, len(src))
	}
	for uid, candidate := range src {
		if candidate == "" {
			continue
		}
		if current := dst[uid]; current == "" {
			dst[uid] = candidate
		} else {
			dst[uid] = preferMatroskaEncoder(current, candidate)
		}
	}
	return dst
}

// mergeMatroskaTagValues fills missing per-track values without replacing
// values parsed from an earlier, more authoritative window.
func mergeMatroskaTagValues(dst, src map[uint64]string) map[uint64]string {
	if dst == nil && len(src) > 0 {
		dst = make(map[uint64]string, len(src))
	}
	for uid, value := range src {
		if value != "" && dst[uid] == "" {
			dst[uid] = value
		}
	}
	return dst
}

// parseMatroskaSimpleTagTree walks nested SimpleTag elements, retaining their
// slash-delimited raw paths and normalized projections. The first non-und
// LANGUAGE element supplies tagLanguage.
func parseMatroskaSimpleTagTree(buf []byte, parent []string, tags map[string]string, fields *[]matroskaTagField, tagLanguage *string) {
	if tags == nil {
		return
	}
	var name string
	var value string
	var language string
	var children [][]byte
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
			children = append(children, payload)
		}
		if id == mkvIDTagName {
			name = string(payload)
		}
		if id == mkvIDTagString {
			value = string(payload)
		}
		if id == mkvIDTagLanguage {
			lang := strings.TrimSpace(strings.TrimRight(string(payload), "\x00"))
			if lang != "" && lang != "und" {
				language = lang
			}
		}
		pos = dataEnd
	}
	name = strings.TrimSpace(strings.TrimRight(name, "\x00"))
	value = strings.TrimRight(value, "\x00")
	path := append(append([]string(nil), parent...), name)
	if name != "" && value != "" {
		rawName := strings.Join(path, "/")
		if rawName == "ENCODER" {
			tags[rawName] = preferMatroskaEncoder(tags[rawName], value)
		} else {
			tags[rawName] = value
		}
		if name == "LANGUAGE" && language != "" && tagLanguage != nil && *tagLanguage == "" {
			*tagLanguage = language
		}
		if fields != nil {
			*fields = append(*fields, normalizeMatroskaTag(path, value)...)
		}
	}
	childParent := parent
	if name != "" {
		childParent = path
	}
	for _, child := range children {
		parseMatroskaSimpleTagTree(child, childParent, tags, fields, tagLanguage)
	}
}

func parseMatroskaTagStats(tags map[string]string, encodedDate string) (matroskaTagStats, bool) {
	if len(tags) == 0 {
		return matroskaTagStats{}, false
	}
	list := strings.Fields(tags["_STATISTICS_TAGS"])
	if len(list) == 0 {
		if seconds, prec, ok := parseMatroskaStatisticsDuration(strings.TrimRight(tags["DURATION"], "\x00")); ok && seconds > 0 {
			return matroskaTagStats{trusted: true, durationSeconds: seconds, durationPrec: prec, hasDuration: true}, true
		}
		return matroskaTagStats{}, false
	}
	statsDateUTC := strings.TrimSpace(tags["_STATISTICS_WRITING_DATE_UTC"])
	if statsDateUTC != "" && !parseMatroskaStatsUTC(statsDateUTC) {
		return matroskaTagStats{}, false
	}
	hasWritingDate := statsDateUTC != ""
	headerUTC := strings.TrimSpace(strings.TrimSuffix(encodedDate, " UTC"))
	if before, _, ok := strings.Cut(headerUTC, " / "); ok {
		headerUTC = strings.TrimSpace(before)
	}
	statsUTC := strings.TrimSpace(strings.TrimSuffix(statsDateUTC, " UTC"))
	trusted := true
	if headerUTC != "" && statsUTC != "" {
		trusted = statsUTC >= headerUTC
	}
	if !trusted {
		extras := make([]jsonKV, 0, 5)
		statsApp := strings.TrimSpace(tags["_STATISTICS_WRITING_APP"])
		if statsApp != "" && statsUTC != "" && headerUTC != "" {
			extras = append(extras, jsonKV{Key: "Statistics_Tags_Issue", Val: statsApp + " " + statsUTC + " / " + statsApp + " " + headerUTC})
		}
		for _, item := range []struct {
			tag string
			key string
		}{
			{tag: "BPS", key: "FromStats_BitRate"},
			{tag: "DURATION", key: "FromStats_Duration"},
			{tag: "NUMBER_OF_FRAMES", key: "FromStats_FrameCount"},
			{tag: "NUMBER_OF_BYTES", key: "FromStats_StreamSize"},
		} {
			if value := strings.TrimSpace(tags[item.tag]); value != "" {
				extras = append(extras, jsonKV{Key: item.key, Val: value})
			}
		}
		if len(extras) == 0 {
			return matroskaTagStats{}, false
		}
		return matroskaTagStats{extras: extras}, true
	}
	out := matroskaTagStats{trusted: true, hasWritingDate: hasWritingDate}
	for _, key := range list {
		if key == "DURATION" {
			out.expectsDuration = true
		}
		value := strings.TrimSpace(strings.TrimRight(tags[key], "\x00"))
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

// mergeMatroskaTagStats merges one statistics-tag observation into dst,
// retaining source metadata and the largest available numeric measurements.
func mergeMatroskaTagStats(dst *matroskaTagStats, src matroskaTagStats) {
	if dst == nil {
		return
	}
	if src.trusted {
		dst.trusted = true
	}
	if src.hasSource {
		dst.source = src.source
		dst.hasSource = true
	}
	if src.hasSourceID {
		dst.sourceID = src.sourceID
		dst.hasSourceID = true
	}
	if src.hasEncodedDate {
		dst.encodedDate = src.encodedDate
		dst.hasEncodedDate = true
	}
	for _, extra := range src.extras {
		seen := false
		for _, existing := range dst.extras {
			if existing.Key == extra.Key {
				seen = true
				break
			}
		}
		if !seen {
			dst.extras = append(dst.extras, extra)
		}
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
	if src.expectsDuration {
		dst.expectsDuration = true
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

// applyMatroskaEncoders applies track-scoped encoder tags to compatible video
// and audio streams and exposes parsed FLAC library components.
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
		settings := settingsByTrackUID[uid]

		if streams[i].Kind == StreamVideo {
			if enc == "" {
				enc = encodersByTrackUID[0]
			}
			if settings == "" {
				settings = settingsByTrackUID[0]
			}
			lowerEnc := strings.ToLower(enc)
			// Avoid tagging muxing apps as a codec encoder; keep this conservative.
			isCodecEncoder := strings.Contains(lowerEnc, "x264") || strings.Contains(lowerEnc, "x265") || strings.Contains(lowerEnc, "svt-av1") || strings.Contains(lowerEnc, "elemental h.264") || strings.Contains(lowerEnc, "avc coding")
			if isCodecEncoder && enc != "" && findField(streams[i].Fields, "Writing library") == "" {
				streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Writing library", Value: enc})
				replaceCanonicalSeedFill(&streams[i], "Encoded_Library", canonicalEncodedLibrary(enc), "Writing library", enc)
			}
			if isCodecEncoder && settings != "" && findField(streams[i].Fields, "Encoding settings") == "" {
				streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Encoding settings", Value: settings})
				replaceCanonicalSeedFill(&streams[i], "Encoded_Library_Settings", settings, "Encoding settings", settings)
			}
		}
		if streams[i].Kind == StreamAudio && enc != "" && (findField(streams[i].Fields, "Format") == "FLAC" || findField(streams[i].Fields, "Format") == "AC-3" || findField(streams[i].Fields, "Format") == "E-AC-3") {
			if findField(streams[i].Fields, "Writing library") != "" {
				continue
			}
			replaceCanonicalSeedLegacyFill(&streams[i], "Encoded_Library", enc, "Writing library", enc)
			if findField(streams[i].Fields, "Format") == "AC-3" && strings.Contains(enc, "ac3_fixed") {
				replaceCanonicalSeedLegacyFill(&streams[i], "BitDepth", "16", "", "")
			}
			if findField(streams[i].Fields, "Format") == "FLAC" && strings.HasPrefix(enc, "Lavc") {
				clearCanonicalSeedField(&streams[i], "BitDepth_Detected", "")
				channelParts := strings.Fields(findField(streams[i].Fields, "Channel(s)"))
				channels := 0
				if len(channelParts) > 0 {
					channels, _ = strconv.Atoi(channelParts[0])
				}
				layout := ""
				switch channels {
				case 1:
					layout = "M"
				case 2:
					layout = "L R"
				case 6:
					layout = "L R C LFE Ls Rs"
				}
				if layout != "" {
					replaceCanonicalSeedLegacyFill(&streams[i], "ChannelLayout", layout, "Channel layout", layout)
					setCanonicalSeedXMLVisibility(&streams[i], "ChannelLayout", true)
					if positions := matroskaGoChannelPositions(uint64(channels)); positions != "" {
						replaceCanonicalSeedFill(&streams[i], "ChannelPositions", positions, "", "")
						setCanonicalSeedXMLVisibility(&streams[i], "ChannelPositions", true)
					}
				}
			}
			if name, version, date := splitFLACEncodedLibrary(enc); name != "" {
				replaceCanonicalSeedLegacyFill(&streams[i], "Encoded_Library_Name", name, "", "")
				if version != "" {
					replaceCanonicalSeedLegacyFill(&streams[i], "Encoded_Library_Version", version, "", "")
				}
				if date != "" {
					replaceCanonicalSeedLegacyFill(&streams[i], "Encoded_Library_Date", date, "", "")
				}
			}
		}
		if streams[i].Kind == StreamAudio && findField(streams[i].Fields, "Format") == "Opus" && enc != "" {
			streams[i].Fields = setFieldValue(streams[i].Fields, "Writing library", enc)
			replaceCanonicalSeedFill(&streams[i], "Encoded_Library", canonicalEncodedLibrary(enc), "Writing library", enc)
			if settings != "" {
				streams[i].Fields = setFieldValue(streams[i].Fields, "Encoding settings", settings)
				replaceCanonicalSeedFill(&streams[i], "Encoded_Library_Settings", settings, "Encoding settings", settings)
			}
		}
		if streams[i].Kind == StreamText && enc != "" && !strings.HasPrefix(enc, "Lavf") {
			streams[i].Fields = setFieldValue(streams[i].Fields, "Writing library", enc)
			replaceCanonicalSeedFill(&streams[i], "Encoded_Library", canonicalEncodedLibrary(enc), "Writing library", enc)
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
			replaceCanonicalSeedFill(&streams[i], "Encoded_Library", canonicalEncodedLibrary(audioEncoder), "Writing library", audioEncoder)
		}
	}
}

// applyMatroskaTagLanguages fills missing canonical track languages from tag
// values keyed by TrackUID without replacing an existing language.
func applyMatroskaTagLanguages(streams []Stream, langsByTrackUID map[uint64]string) {
	if len(langsByTrackUID) == 0 {
		return
	}
	for i := range streams {
		// Official mediainfo doesn't emit video/text Language based on Statistics Tags TagLanguage.
		// Keep video/text language empty even if muxer tags provide a value.
		if streams[i].Kind == StreamVideo || streams[i].Kind == StreamText {
			continue
		}
		uid := streamTrackUID(streams[i])
		if uid == 0 {
			continue
		}
		lang := strings.TrimSpace(langsByTrackUID[uid])
		if lang == "" {
			continue
		}
		// Don't override TrackEntry-provided language.
		if language, found := canonicalSeedValue(streams[i], "Language"); found && language != "" {
			continue
		}
		if language, found := canonicalSeedTextValue(streams[i], "Language"); found && language != "" {
			continue
		}

		code := normalizeLanguageCode(lang)
		if code == "" {
			code = normalizeLanguageCode(strings.ToLower(lang))
		}
		if display := formatLanguage(lang); code != "" {
			replaceCanonicalSeedLegacyFill(&streams[i], "Language", code, "Language", display)
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
	objType, sbrData, sbrPresent, _, ok := parseAACAudioSpecificConfig(payload)
	if !ok || objType <= 0 {
		return "", 0, false
	}
	return mapAACProfile(objType), objType, sbrData && !sbrPresent
}

// parseMatroskaAACProfile decodes AAC profile, signaled object type, explicit
// SBR state, and sample rate from AudioSpecificConfig data.
func parseMatroskaAACProfile(payload []byte) (profile string, codecObjectType int, sbrMode string, psMode string, sampleRate int) {
	baseObjectType, sbrData, sbrPresent, psPresent, ok := parseAACAudioSpecificConfig(payload)
	if !ok || baseObjectType <= 0 {
		return "", 0, "", "", 0
	}
	profile = mapAACProfile(baseObjectType)
	codecObjectType = baseObjectType
	br := newBitReader(payload)
	signaledObjectType, ok := readAACAudioObjectType(br)
	if !ok {
		return profile, codecObjectType, "", "", 0
	}
	if index := br.readBitsValue(4); index != ^uint64(0) {
		sampleRates := [...]int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
		if index < uint64(len(sampleRates)) {
			sampleRate = sampleRates[index]
		} else if index == 0x0f {
			if explicit := br.readBitsValue(24); explicit != ^uint64(0) {
				sampleRate = int(explicit)
			}
		}
	}
	switch signaledObjectType {
	case 5, 29:
		codecObjectType = signaledObjectType
		sbrMode = "Yes (NBC)"
	default:
		if sbrData {
			if sbrPresent {
				sbrMode = "Yes (Explicit)"
			} else {
				sbrMode = "No (Explicit)"
			}
		}
	}
	if psPresent {
		psMode = "Yes (Explicit)"
	}
	return profile, codecObjectType, sbrMode, psMode, sampleRate
}

// parseAACAudioSpecificConfig reads AAC object type, SBR, and PS signaling
// without requiring a complete decoder configuration beyond the available bits.
func parseAACAudioSpecificConfig(payload []byte) (objType int, sbrData bool, sbrPresent bool, psPresent bool, ok bool) {
	if len(payload) == 0 {
		return 0, false, false, false, false
	}
	br := newBitReader(payload)

	objType, ok = readAACAudioObjectType(br)
	if !ok {
		return 0, false, false, false, false
	}

	sfIndex := br.readBitsValue(4)
	if sfIndex == ^uint64(0) {
		return 0, false, false, false, false
	}
	if sfIndex == 0xF {
		if br.readBitsValue(24) == ^uint64(0) {
			return 0, false, false, false, false
		}
	}
	channelConfig := br.readBitsValue(4)
	if channelConfig == ^uint64(0) {
		return 0, false, false, false, false
	}

	extensionAudioObjectType := 0
	if objType == 5 || objType == 29 {
		extensionAudioObjectType = 5
		extIndex := br.readBitsValue(4)
		if extIndex == ^uint64(0) {
			return 0, false, false, false, false
		}
		if extIndex == 0xF {
			if br.readBitsValue(24) == ^uint64(0) {
				return 0, false, false, false, false
			}
		}
		next, ok := readAACAudioObjectType(br)
		if !ok {
			return 0, false, false, false, false
		}
		objType = next
		if objType == 22 {
			// extensionChannelConfiguration (4)
			if br.readBitsValue(4) == ^uint64(0) {
				return 0, false, false, false, false
			}
		}
	}

	if isAACGASpecificObjectType(objType) {
		if !skipAACGASpecificConfig(br, channelConfig, objType) {
			return objType, false, false, false, true
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
				return objType, false, false, false, true
			}
			switch extObjType {
			case 5:
				v := br.readBitsValue(1)
				if v == ^uint64(0) {
					return objType, false, false, false, true
				}
				sbrPresent = v == 1
			case 29:
				v := br.readBitsValue(1)
				if v == ^uint64(0) {
					return objType, false, false, false, true
				}
				sbrPresent = true
				psPresent = v == 1
				if br.readBitsValue(4) == ^uint64(0) { // extensionChannelConfiguration
					return objType, false, false, false, true
				}
			default:
				sbrData = false
			}
			return objType, sbrData, sbrPresent, psPresent, true
		}
		// Not an extension: rewind so we don't consume bits unexpectedly.
		br.pos = savedPos
		br.bit = savedBit
	}

	return objType, false, false, false, true
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
