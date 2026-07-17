package mediainfo

import (
	"encoding/binary"
	"io"
)

const maxMoovSize = int64(16 << 20)

// MP4Track contains parsed track identity, timing, sample-table, and display
// metadata used to construct one canonical media stream.
type MP4Track struct {
	// ID is the tkhd track identifier.
	ID uint32
	// Kind is the stream kind inferred from the media handler.
	Kind StreamKind
	// Format is the codec format inferred from the sample entry.
	Format string
	// HandlerName is the optional mdia handler name.
	HandlerName string
	// HandlerType is the four-character mdia handler code.
	HandlerType string
	// LanguageCode is the normalized language code from mdhd metadata.
	LanguageCode string
	// CreationTime is the raw MP4 epoch creation timestamp.
	CreationTime uint64
	// ModificationTime is the raw MP4 epoch modification timestamp.
	ModificationTime uint64
	// Fields contains sample-entry display metadata retained for compatibility.
	Fields []Field
	// SampleCount is the number of decoded samples described by the sample tables.
	SampleCount uint64
	// SampleBytes is the total byte count described by sample sizes.
	SampleBytes uint64
	// SampleSizeHead retains leading sample sizes needed for bounded adjustments.
	SampleSizeHead []uint32
	// SampleSizeTail retains trailing sample sizes needed for edit-list trimming.
	SampleSizeTail []uint32
	// SampleDelta is the dominant decode-time delta in track timescale units.
	SampleDelta uint32
	// LastSampleDelta is the final decode-time delta in track timescale units.
	LastSampleDelta uint32
	// VariableDeltas reports whether decode-time deltas vary.
	VariableDeltas bool
	// MinimumSampleDelta is the shortest decode-time delta in track timescale units.
	MinimumSampleDelta uint32
	// MaximumSampleDelta is the longest decode-time delta in track timescale units.
	MaximumSampleDelta uint32
	// FirstChunkOff is the first media chunk's absolute file offset.
	FirstChunkOff uint64
	// DurationSeconds is the media duration before edit-list presentation changes.
	DurationSeconds float64
	// EditDuration is the presentation duration selected by the edit list.
	EditDuration float64
	// EditMediaTime is the edit-list media start in track timescale units.
	EditMediaTime int64
	// Default reports whether the track is selected by default.
	Default bool
	// AlternateGroup identifies mutually exclusive tracks.
	AlternateGroup uint16
	// Timescale is the track media timescale.
	Timescale uint32
	// Width is the parsed display width in pixels.
	Width uint64
	// Height is the parsed display height in pixels.
	Height                 uint64
	canonicalSeed          []fieldEntry
	sampleEntryType        string
	nonEmptySampleCount    uint64
	chunkOffsetsHead       []uint64
	sampleToChunk          []mp4SampleToChunkEntry
	hevcNALLengthSize      int
	avcNALLengthSize       int
	avcSPS                 h264SPSInfo
	avcParameterSets       []byte
	hevcSEI                hevcHDRInfo
	dolbyVision            dolbyVisionConfig
	hasDolbyVision         bool
	hevcContainerMastering bool
	hevcContainerCLL       bool
	chapterTrackRefs       []uint32
	menuForTrackID         uint32
	sampleStartsHead       []uint64
	trackTitle             string
}

// mp4SampleToChunkEntry describes one stsc run for bounded sample reads.
type mp4SampleToChunkEntry struct {
	firstChunk      uint32
	samplesPerChunk uint32
}

// MP4Info contains parsed movie-level metadata and tracks.
type MP4Info struct {
	// Container contains canonical movie duration and container facts.
	Container ContainerInfo
	// General contains movie-level display metadata retained for compatibility.
	General []Field
	// Tracks contains supported parsed media tracks.
	Tracks []MP4Track
	// MovieTimescale is the mvhd timescale.
	MovieTimescale uint32
	// MovieCreation is the raw MP4 epoch creation timestamp.
	MovieCreation uint64
	// MovieModified is the raw MP4 epoch modification timestamp.
	MovieModified uint64
	// Chapters contains decoded chapter starts and titles.
	Chapters     []mp4Chapter
	generalExtra []jsonKV
}

// mp4Chapter stores one decoded chapter start and title.
type mp4Chapter struct {
	startMs int64
	title   string
}

// ParseMP4 parses top-level MP4 boxes and returns canonical movie and track
// metadata for a valid file.
func ParseMP4(r io.ReaderAt, size int64) (MP4Info, bool) {
	info := MP4Info{}
	var offset int64
	for offset+8 <= size {
		boxSize, boxType, headerSize, ok := readMP4BoxHeader(r, offset, size)
		if !ok || boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "ftyp" {
			payload := make([]byte, boxSize-headerSize)
			if _, err := r.ReadAt(payload, dataOffset); err == nil || err == io.EOF {
				if fields := parseFtyp(payload); len(fields) > 0 {
					info.General = append(info.General, fields...)
				}
			}
		}
		if boxType == "moov" {
			moovSize := boxSize - headerSize
			if moovSize > maxMoovSize {
				return MP4Info{}, false
			}
			buf := make([]byte, moovSize)
			if _, err := r.ReadAt(buf, dataOffset); err != nil && err != io.EOF {
				return MP4Info{}, false
			}
			if moovInfo, ok := parseMoov(buf); ok {
				if len(info.General) > 0 {
					moovInfo.General = append(info.General, moovInfo.General...)
				}
				return moovInfo, true
			}
		}
		offset += boxSize
	}
	return MP4Info{}, false
}

// readMP4BoxHeader validates and returns one top-level box header without
// reading beyond fileSize.
func readMP4BoxHeader(r io.ReaderAt, offset, fileSize int64) (boxSize int64, boxType string, headerSize int64, ok bool) {
	if offset < 0 || fileSize-offset < 8 {
		return 0, "", 0, false
	}
	remaining := fileSize - offset
	var header [8]byte
	if _, err := r.ReadAt(header[:], offset); err != nil {
		return 0, "", 0, false
	}

	size32 := binary.BigEndian.Uint32(header[0:4])
	boxType = string(header[4:8])
	if size32 == 0 {
		return remaining, boxType, 8, true
	}
	if size32 == 1 {
		var larger [8]byte
		if _, err := r.ReadAt(larger[:], offset+8); err != nil {
			return 0, "", 0, false
		}
		size64 := binary.BigEndian.Uint64(larger[:])
		if size64 < 16 || size64 > uint64(remaining) {
			return 0, "", 0, false
		}
		return int64(size64), boxType, 16, true
	}
	if size32 < 8 {
		return 0, "", 0, false
	}
	if int64(size32) > remaining {
		return 0, "", 0, false
	}
	return int64(size32), boxType, 8, true
}

// parseMoov parses movie timing, metadata, chapter, and track child boxes.
func parseMoov(buf []byte) (MP4Info, bool) {
	var offset int64
	info := MP4Info{}
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "mvhd" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if duration, timescale, created, modified, ok := parseMvhdMeta(payload); ok {
				info.Container.DurationSeconds = duration
				info.MovieTimescale = timescale
				info.MovieCreation = created
				info.MovieModified = modified
			}
		}
		if boxType == "udta" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			info.General = append(info.General, parseMP4UserMetadata(payload)...)
			if extras := parseMP4UnknownUserMetadata(payload); len(extras) > 0 {
				info.generalExtra = append(info.generalExtra, extras...)
				for _, extra := range extras {
					info.General = append(info.General, Field{Name: extra.Key, Value: extra.Val})
				}
			}
			if app := parseMP4WritingApp(payload); app != "" {
				info.General = append(info.General, Field{Name: "Writing application", Value: app})
			}
			if desc := parseMP4Description(payload); desc != "" {
				info.General = append(info.General, Field{Name: "Description", Value: desc})
			}
			if chapters := parseMP4Chpl(payload); len(chapters) > 0 {
				info.Chapters = append(info.Chapters, chapters...)
			}
		}
		if boxType == "trak" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if track, ok := parseTrak(payload, info.MovieTimescale); ok {
				info.Tracks = append(info.Tracks, track)
			}
		}
		offset += boxSize
	}
	markMP4ChapterTracks(info.Tracks)
	if info.Container.HasDuration() || len(info.Tracks) > 0 {
		return info, true
	}
	return MP4Info{}, false
}

// readMP4BoxHeaderFrom returns one validated in-memory box header, or zero size
// when the header or declared extent is invalid.
func readMP4BoxHeaderFrom(buf []byte, offset int64) (boxSize int64, boxType string, headerSize int64) {
	if offset+8 > int64(len(buf)) {
		return 0, "", 0
	}
	remaining := int64(len(buf)) - offset
	size32 := binary.BigEndian.Uint32(buf[offset : offset+4])
	boxType = string(buf[offset+4 : offset+8])
	if size32 == 0 {
		return remaining, boxType, 8
	}
	if size32 == 1 {
		if offset+16 > int64(len(buf)) {
			return 0, "", 0
		}
		size64 := binary.BigEndian.Uint64(buf[offset+8 : offset+16])
		if size64 < 16 || size64 > uint64(remaining) {
			return 0, "", 0
		}
		return int64(size64), boxType, 16
	}
	if size32 < 8 || int64(size32) > remaining {
		return 0, "", 0
	}
	return int64(size32), boxType, 8
}

// sliceBox returns a bounds-clamped box payload slice, or nil for an invalid
// starting offset.
func sliceBox(buf []byte, offset, length int64) []byte {
	if offset < 0 || length < 0 {
		return nil
	}
	end := min(offset+length, int64(len(buf)))
	if offset > end {
		return nil
	}
	return buf[offset:end]
}

// parseMvhd returns movie duration and timescale from an mvhd payload.
func parseMvhd(payload []byte) (float64, uint32, bool) {
	return parseMP4Duration(payload, 20, 32)
}

// parseTrak combines one track's header, edit-list, media, and sample-table
// facts and reports whether the track is supported.
func parseTrak(buf []byte, movieTimescale uint32) (MP4Track, bool) {
	var offset int64
	var tkhdInfo tkhdInfo
	var hasTkhd bool
	var editDuration float64
	var editMediaTime int64
	var chapterTrackRefs []uint32
	var trackTitle string
	var mediaTrack MP4Track
	var hasMediaTrack bool
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "tkhd" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if info, ok := parseTkhd(payload); ok {
				tkhdInfo = info
				hasTkhd = true
			}
		}
		if boxType == "edts" && movieTimescale > 0 {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if duration, mediaTime := parseEdts(payload, movieTimescale); duration > 0 {
				editDuration = duration
				editMediaTime = mediaTime
			}
		}
		if boxType == "tref" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			chapterTrackRefs = parseMP4ChapterTrackRefs(payload)
		}
		if boxType == "udta" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			trackTitle = parseMP4TrackName(payload)
		}
		if boxType == "mdia" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if track, ok := parseMdia(payload); ok {
				mediaTrack = track
				hasMediaTrack = true
			}
		}
		offset += boxSize
	}
	if !hasMediaTrack {
		return MP4Track{}, false
	}
	if hasTkhd && tkhdInfo.ID > 0 {
		mediaTrack.ID = tkhdInfo.ID
	}
	if editDuration > 0 {
		mediaTrack.EditDuration = editDuration
		mediaTrack.EditMediaTime = editMediaTime
	}
	if hasTkhd {
		mediaTrack.Default = tkhdInfo.Default
		mediaTrack.AlternateGroup = tkhdInfo.AlternateGroup
		mediaTrack.CreationTime = tkhdInfo.CreationTime
		mediaTrack.ModificationTime = tkhdInfo.ModifiedTime
	}
	mediaTrack.chapterTrackRefs = append([]uint32(nil), chapterTrackRefs...)
	mediaTrack.trackTitle = trackTitle
	return mediaTrack, true
}

// parseMdia parses one mdia box into track handler, timing, language, and
// sample-table metadata and reports whether a supported handler was found.
func parseMdia(buf []byte) (MP4Track, bool) {
	var offset int64
	var handler string
	var handlerName string
	var sampleInfo SampleInfo
	var trackDuration float64
	var trackTimescale uint32
	var language string
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "hdlr" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			handler = parseHdlr(payload)
			handlerName = parseHdlrName(payload)
		}
		if boxType == "mdhd" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if duration, timescale, lang, ok := parseMdhdMeta(payload); ok {
				trackDuration = duration
				trackTimescale = timescale
				language = lang
			}
		}
		if boxType == "minf" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if info, ok := parseMinfSample(payload); ok {
				sampleInfo = info
			}
		}
		offset += boxSize
	}
	if handler == "" {
		return MP4Track{}, false
	}
	kind, format := mapHandlerType(handler)
	if kind == "" {
		return MP4Track{}, false
	}
	if sampleInfo.Format != "" {
		format = sampleInfo.Format
	}
	return MP4Track{
		Kind:                   kind,
		Format:                 format,
		HandlerName:            handlerName,
		HandlerType:            handler,
		LanguageCode:           language,
		Fields:                 sampleInfo.Fields,
		SampleCount:            sampleInfo.SampleCount,
		SampleBytes:            sampleInfo.SampleBytes,
		SampleSizeHead:         sampleInfo.SampleSizeHead,
		SampleSizeTail:         sampleInfo.SampleSizeTail,
		SampleDelta:            sampleInfo.SampleDelta,
		LastSampleDelta:        sampleInfo.LastSampleDelta,
		VariableDeltas:         sampleInfo.VariableDeltas,
		MinimumSampleDelta:     sampleInfo.MinimumSampleDelta,
		MaximumSampleDelta:     sampleInfo.MaximumSampleDelta,
		FirstChunkOff:          sampleInfo.FirstChunkOff,
		DurationSeconds:        trackDuration,
		Timescale:              trackTimescale,
		Width:                  sampleInfo.Width,
		Height:                 sampleInfo.Height,
		canonicalSeed:          append([]fieldEntry(nil), sampleInfo.canonicalSeed...),
		sampleEntryType:        sampleInfo.SampleEntryType,
		nonEmptySampleCount:    sampleInfo.NonEmptySampleCount,
		chunkOffsetsHead:       append([]uint64(nil), sampleInfo.chunkOffsetsHead...),
		sampleToChunk:          append([]mp4SampleToChunkEntry(nil), sampleInfo.sampleToChunk...),
		hevcNALLengthSize:      sampleInfo.hevcNALLengthSize,
		avcNALLengthSize:       sampleInfo.avcNALLengthSize,
		avcSPS:                 sampleInfo.avcSPS,
		avcParameterSets:       append([]byte(nil), sampleInfo.avcParameterSets...),
		hevcSEI:                sampleInfo.hevcSEI,
		dolbyVision:            sampleInfo.dolbyVision,
		hasDolbyVision:         sampleInfo.hasDolbyVision,
		hevcContainerMastering: sampleInfo.hevcContainerMastering,
		hevcContainerCLL:       sampleInfo.hevcContainerCLL,
		sampleStartsHead:       append([]uint64(nil), sampleInfo.sampleStartsHead...),
	}, true
}

func parseHdlr(payload []byte) string {
	if len(payload) < 20 {
		return ""
	}
	return string(payload[8:12])
}

type tkhdInfo struct {
	ID             uint32
	Default        bool
	AlternateGroup uint16
	CreationTime   uint64
	ModifiedTime   uint64
}

func parseTkhd(payload []byte) (tkhdInfo, bool) {
	if len(payload) < 20 {
		return tkhdInfo{}, false
	}
	version := payload[0]
	flags := uint32(payload[1])<<16 | uint32(payload[2])<<8 | uint32(payload[3])
	if version == 0 {
		if len(payload) < 36 {
			return tkhdInfo{}, false
		}
		creation := uint64(binary.BigEndian.Uint32(payload[4:8]))
		modified := uint64(binary.BigEndian.Uint32(payload[8:12]))
		id := binary.BigEndian.Uint32(payload[12:16])
		alternateGroup := binary.BigEndian.Uint16(payload[34:36])
		return tkhdInfo{
			ID:             id,
			Default:        flags&0x000001 != 0,
			AlternateGroup: alternateGroup,
			CreationTime:   creation,
			ModifiedTime:   modified,
		}, true
	}
	if version == 1 {
		if len(payload) < 48 {
			return tkhdInfo{}, false
		}
		creation := binary.BigEndian.Uint64(payload[4:12])
		modified := binary.BigEndian.Uint64(payload[12:20])
		id := binary.BigEndian.Uint32(payload[20:24])
		alternateGroup := binary.BigEndian.Uint16(payload[46:48])
		return tkhdInfo{
			ID:             id,
			Default:        flags&0x000001 != 0,
			AlternateGroup: alternateGroup,
			CreationTime:   creation,
			ModifiedTime:   modified,
		}, true
	}
	return tkhdInfo{}, false
}

func parseEdts(payload []byte, movieTimescale uint32) (float64, int64) {
	if movieTimescale == 0 {
		return 0, 0
	}
	var offset int64
	var duration float64
	var mediaTime int64
	for offset+8 <= int64(len(payload)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(payload, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "elst" {
			elstPayload := sliceBox(payload, dataOffset, boxSize-headerSize)
			if parsedDuration, parsedMediaTime := parseElst(elstPayload, movieTimescale); parsedDuration > 0 {
				duration = parsedDuration
				mediaTime = parsedMediaTime
			}
		}
		offset += boxSize
	}
	return duration, mediaTime
}

func parseElst(payload []byte, movieTimescale uint32) (float64, int64) {
	if len(payload) < 8 || movieTimescale == 0 {
		return 0, 0
	}
	version := payload[0]
	entryCount := binary.BigEndian.Uint32(payload[4:8])
	offset := 8
	var total uint64
	var mediaTime int64
	switch version {
	case 0:
		for i := 0; i < int(entryCount); i++ {
			if offset+12 > len(payload) {
				break
			}
			segmentDuration := binary.BigEndian.Uint32(payload[offset : offset+4])
			mediaTimeValue := int32(binary.BigEndian.Uint32(payload[offset+4 : offset+8]))
			if mediaTimeValue >= 0 && segmentDuration > 0 {
				total += uint64(segmentDuration)
				if mediaTime == 0 {
					mediaTime = int64(mediaTimeValue)
				}
			}
			offset += 12
		}
	case 1:
		for i := 0; i < int(entryCount); i++ {
			if offset+20 > len(payload) {
				break
			}
			segmentDuration := binary.BigEndian.Uint64(payload[offset : offset+8])
			mediaTimeValue := int64(binary.BigEndian.Uint64(payload[offset+8 : offset+16]))
			if mediaTimeValue >= 0 && segmentDuration > 0 {
				total += segmentDuration
				if mediaTime == 0 {
					mediaTime = mediaTimeValue
				}
			}
			offset += 20
		}
	default:
		return 0, 0
	}
	if total == 0 {
		return 0, mediaTime
	}
	return float64(total) / float64(movieTimescale), mediaTime
}

func mapHandlerType(handler string) (StreamKind, string) {
	switch handler {
	case "vide":
		return StreamVideo, "Video"
	case "soun":
		return StreamAudio, "Audio"
	case "text", "sbtl", "subt":
		return StreamText, "Text"
	default:
		return "", ""
	}
}

func parseMinfSample(buf []byte) (SampleInfo, bool) {
	var offset int64
	var info SampleInfo
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "stbl" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if parsed, ok := parseStbl(payload); ok {
				info = mergeSampleInfo(info, parsed)
			}
		}
		offset += boxSize
	}
	if info.Format != "" || len(info.Fields) > 0 || info.SampleCount > 0 {
		return info, true
	}
	return SampleInfo{}, false
}

func parseStbl(buf []byte) (SampleInfo, bool) {
	var offset int64
	info := SampleInfo{}
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "stsd" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if parsed, ok := parseStsdForSample(payload); ok {
				info = mergeSampleInfo(info, parsed)
			}
		}
		if boxType == "stts" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if count, sampleDelta, lastDelta, ok, variable := parseStts(payload); ok {
				info.SampleCount = count
				info.SampleDelta = sampleDelta
				info.LastSampleDelta = lastDelta
				info.VariableDeltas = variable
				info.MinimumSampleDelta, info.MaximumSampleDelta = parseSttsDeltaRange(payload)
				info.sampleStartsHead = parseSttsSampleStarts(payload, mp4SampleSizeHeadMax)
			}
		}
		if boxType == "stsz" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if total, head, tail, ok := parseStszWithHead(payload, mp4SampleSizeHeadMax); ok {
				info.SampleBytes = total
				info.SampleSizeHead = head
				info.SampleSizeTail = tail
				info.NonEmptySampleCount = countMP4NonEmptySamples(payload)
			}
		}
		if boxType == "stsc" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			info.sampleToChunk = parseStsc(payload)
		}
		if boxType == "stco" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if offsets := parseStcoHead(payload, mp4ChunkOffsetHeadMax); len(offsets) > 0 {
				info.FirstChunkOff = offsets[0]
				info.chunkOffsetsHead = offsets
			}
		}
		if boxType == "co64" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if offsets := parseCo64Head(payload, mp4ChunkOffsetHeadMax); len(offsets) > 0 {
				info.FirstChunkOff = offsets[0]
				info.chunkOffsetsHead = offsets
			}
		}
		offset += boxSize
	}
	if info.Format != "" || len(info.Fields) > 0 || info.SampleCount > 0 {
		return info, true
	}
	return SampleInfo{}, false
}

// parseMP4ChapterTrackRefs returns chapter-track IDs from a tref/chap box.
func parseMP4ChapterTrackRefs(payload []byte) []uint32 {
	chap, ok := findMP4Box(payload, "chap")
	if !ok || len(chap) < 4 {
		return nil
	}
	result := make([]uint32, 0, len(chap)/4)
	for offset := 0; offset+4 <= len(chap); offset += 4 {
		id := binary.BigEndian.Uint32(chap[offset : offset+4])
		if id > 0 {
			result = append(result, id)
		}
	}
	return result
}

// markMP4ChapterTracks reclassifies tracks referenced by tref/chap as menus.
func markMP4ChapterTracks(tracks []MP4Track) {
	for _, source := range tracks {
		for _, targetID := range source.chapterTrackRefs {
			for index := range tracks {
				if tracks[index].ID == targetID {
					tracks[index].Kind = StreamMenu
					tracks[index].menuForTrackID = source.ID
				}
			}
		}
	}
}

func parseStcoFirst(payload []byte) (uint64, bool) {
	if len(payload) < 8 {
		return 0, false
	}
	count := binary.BigEndian.Uint32(payload[4:8])
	if count == 0 || len(payload) < 12 {
		return 0, false
	}
	return uint64(binary.BigEndian.Uint32(payload[8:12])), true
}

func parseCo64First(payload []byte) (uint64, bool) {
	if len(payload) < 8 {
		return 0, false
	}
	count := binary.BigEndian.Uint32(payload[4:8])
	if count == 0 || len(payload) < 16 {
		return 0, false
	}
	return binary.BigEndian.Uint64(payload[8:16]), true
}

const mp4ChunkOffsetHeadMax = 128

// parseStcoHead returns a bounded prefix of 32-bit chunk offsets.
func parseStcoHead(payload []byte, limit int) []uint64 {
	if len(payload) < 8 || limit <= 0 {
		return nil
	}
	count := int(binary.BigEndian.Uint32(payload[4:8]))
	count = min(count, limit)
	if count <= 0 || len(payload) < 8+count*4 {
		return nil
	}
	result := make([]uint64, count)
	for index := range result {
		result[index] = uint64(binary.BigEndian.Uint32(payload[8+index*4 : 12+index*4]))
	}
	return result
}

// parseCo64Head returns a bounded prefix of 64-bit chunk offsets.
func parseCo64Head(payload []byte, limit int) []uint64 {
	if len(payload) < 8 || limit <= 0 {
		return nil
	}
	count := int(binary.BigEndian.Uint32(payload[4:8]))
	count = min(count, limit)
	if count <= 0 || len(payload) < 8+count*8 {
		return nil
	}
	result := make([]uint64, count)
	for index := range result {
		result[index] = binary.BigEndian.Uint64(payload[8+index*8 : 16+index*8])
	}
	return result
}

// parseStsc decodes sample-to-chunk runs used by bounded codec probes.
func parseStsc(payload []byte) []mp4SampleToChunkEntry {
	if len(payload) < 8 {
		return nil
	}
	count := int(binary.BigEndian.Uint32(payload[4:8]))
	if count <= 0 || len(payload) < 8+count*12 {
		return nil
	}
	result := make([]mp4SampleToChunkEntry, 0, count)
	for index := range count {
		pos := 8 + index*12
		firstChunk := binary.BigEndian.Uint32(payload[pos : pos+4])
		samplesPerChunk := binary.BigEndian.Uint32(payload[pos+4 : pos+8])
		if firstChunk == 0 || samplesPerChunk == 0 {
			continue
		}
		result = append(result, mp4SampleToChunkEntry{firstChunk: firstChunk, samplesPerChunk: samplesPerChunk})
	}
	return result
}
