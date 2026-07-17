package mediainfo

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

type flacTagKV struct {
	Key string
	Val string
}

// flacStreamInfo contains the lossless stream properties encoded by a FLAC
// STREAMINFO metadata block.
type flacStreamInfo struct {
	minBlockSize  uint16
	maxBlockSize  uint16
	sampleRate    uint32
	channels      uint8
	bitsPerSample uint8
	totalSamples  uint64
	md5           string
}

// parseFLAC parses FLAC metadata into canonical stream and General facts.
// Oversized or malformed comments and pictures are skipped without preventing
// valid audio parsing. Invalid signatures and unreadable inputs return false.
func parseFLAC(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, *canonicalStructuredFacts, *structuredNode, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, nil, nil, false
	}

	var header [4]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return ContainerInfo{}, nil, nil, nil, false
	}
	if header[0] != 'f' || header[1] != 'L' || header[2] != 'a' || header[3] != 'C' {
		return ContainerInfo{}, nil, nil, nil, false
	}

	var sampleRate uint32
	var channels uint8
	var bitsPerSample uint8
	var totalSamples uint64
	var md5Hex string
	var audioStart int64
	var encoder string
	tags := map[string]string{}
	var coverMIME string
	var coverType string
	assetBudget := &embeddedAssetBudget{}

	for {
		var blockHeader [4]byte
		if _, err := io.ReadFull(file, blockHeader[:]); err != nil {
			break
		}
		isLast := (blockHeader[0] & 0x80) != 0
		blockType := blockHeader[0] & 0x7F
		blockLen := int(blockHeader[1])<<16 | int(blockHeader[2])<<8 | int(blockHeader[3])
		blockStart, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			break
		}
		blockEnd, reason := checkedEmbeddedRange(blockStart, uint64(blockLen), size)
		if reason != embeddedAssetAccepted {
			break
		}
		blockReadOK := true
		switch blockType {
		case 0:
			if blockLen >= 34 {
				var streamInfo [34]byte
				if _, err := io.ReadFull(file, streamInfo[:]); err != nil {
					blockReadOK = false
				} else {
					sampleRate, channels, bitsPerSample, totalSamples, md5Hex = parseFLACStreamInfo(streamInfo[:])
				}
			}
		case 4:
			// VorbisComment. Primary source for tags like ENCODER, TITLE, ALBUM, etc.
			if assetBudget.reserveString(uint64(blockLen), embeddedAssetMaxStringBytes) == embeddedAssetAccepted {
				buf := make([]byte, blockLen)
				if _, err := io.ReadFull(file, buf); err != nil {
					blockReadOK = false
				} else {
					vendor, pairs := parseFLACVorbisComment(buf)
					if encoder == "" {
						encoder = vendor
					}
					for _, kv := range pairs {
						if kv.Key == "" || kv.Val == "" {
							continue
						}
						if tags[kv.Key] == "" {
							tags[kv.Key] = kv.Val
							continue
						}
						if tags[kv.Key] != kv.Val {
							tags[kv.Key] = tags[kv.Key] + " / " + kv.Val
						}
					}
				}
			}
		case 6:
			// METADATA_BLOCK_PICTURE (cover art). Only its bounded type/MIME prefix is needed.
			if assetBudget.reserveItem() == embeddedAssetAccepted && coverMIME == "" {
				mime, typ, ok, err := readFLACPictureHeader(file, int64(blockLen), assetBudget)
				if err != nil {
					blockReadOK = false
				} else if ok {
					coverMIME = mime
					coverType = typ
				}
			}
		}
		if _, err := file.Seek(blockEnd, io.SeekStart); err != nil || !blockReadOK {
			break
		}
		if isLast {
			// The file cursor is now positioned at the start of the audio frames.
			audioStart = blockEnd
			break
		}
	}

	if sampleRate == 0 || channels == 0 {
		return ContainerInfo{}, nil, nil, nil, false
	}

	duration := 0.0
	if totalSamples > 0 {
		duration = float64(totalSamples) / float64(sampleRate)
	}
	// Match MediaInfo: FLAC duration is treated at millisecond precision.
	if duration > 0 {
		duration = math.Round(duration*1000) / 1000
	}

	bitrate := 0.0
	if duration > 0 {
		bitrate = (float64(size) * 8) / duration
	}

	info := ContainerInfo{
		DurationSeconds: duration,
		BitrateMode:     "Variable",
		StreamOverheadBytes: func() int64 {
			if audioStart <= 0 {
				return 0
			}
			return audioStart
		}(),
	}

	rawBitrate := ""
	streamSize := int64(0)
	if audioStart > 0 && audioStart < size {
		streamSize = size - audioStart
		if totalSamples > 0 && sampleRate > 0 {
			// MediaInfo's FLAC bitrates use Duration in integer milliseconds.
			durationMs := int64((totalSamples*1000 + uint64(sampleRate)/2) / uint64(sampleRate))
			if durationMs > 0 {
				// Round to nearest b/s (MediaInfo output is exact integer).
				br := (streamSize*8000 + durationMs/2) / durationMs
				if br > 0 {
					rawBitrate = strconv.FormatInt(br, 10)
				}
			}
		}
	}
	encodedLibraryName := ""
	encodedLibraryVersion := ""
	encodedLibraryDate := ""
	if encoder != "" {
		// Match MediaInfo naming: ENCODER becomes Encoded_Application (General) and Encoded_Library (Audio).
		if name, version, date := splitFLACEncodedLibrary(encoder); name != "" {
			encodedLibraryName = name
			encodedLibraryVersion = version
			encodedLibraryDate = date
		}
	}

	generalFacts, generalExtra := flacTagsToGeneralFacts(tags, encoder)
	if coverMIME != "" && (len(generalFacts.values) > 0 || generalExtra != nil) {
		generalFacts.SetSame("Cover", "Yes")
		generalFacts.SetSame("Cover_Mime", coverMIME)
		if coverType != "" {
			generalFacts.SetSame("Cover_Type", coverType)
		}
	}

	audioStream := canonicalFLACAudioStream(channels, sampleRate, bitsPerSample, totalSamples, duration, bitrate, rawBitrate, streamSize, encoder, encodedLibraryName, encodedLibraryVersion, encodedLibraryDate, md5Hex)
	return info, []Stream{audioStream}, generalFacts, generalExtra, true
}

// canonicalFLACAudioStream records FLAC audio facts in canonical units before
// publishing the public compatibility snapshot.
func canonicalFLACAudioStream(channels uint8, sampleRate uint32, bitsPerSample uint8, totalSamples uint64, duration, displayBitrate float64, rawBitrate string, streamSize int64, encoder, encodedLibraryName, encodedLibraryVersion, encodedLibraryDate, md5Hex string) Stream {
	store := &fieldStore{}
	ref := store.Prepare(StreamAudio)
	store.streams[ref].SkipStreamOrder = true
	store.Fill(ref, "Format", "FLAC", fillReplace)
	if duration > 0 {
		store.Fill(ref, "Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), fillReplace)
	}
	store.Fill(ref, "BitRate_Mode", "Variable", fillReplace)
	if rawBitrate != "" {
		store.Fill(ref, "BitRate", rawBitrate, fillReplace)
		if displayBitrate > 0 {
			store.Fill(ref, "BitRate/String", formatBitrate(displayBitrate), fillReplace)
		}
	} else if displayBitrate > 0 {
		store.Fill(ref, "BitRate", strconv.FormatInt(int64(math.Round(displayBitrate)), 10), fillReplace)
	}
	if channels > 0 {
		channelText := strconv.Itoa(int(channels))
		store.Fill(ref, "Channels", channelText, fillReplace)
		if positions := channelPositionsFromCount(channelText); positions != "" {
			fillGeneratedStructured(store, ref, "ChannelPositions", positions)
		}
		if layout := channelLayout(uint64(channels)); layout != "" {
			store.Fill(ref, "ChannelLayout", layout, fillReplace)
		}
	}
	if sampleRate > 0 {
		store.Fill(ref, "SamplingRate", strconv.FormatUint(uint64(sampleRate), 10), fillReplace)
	}
	if bitsPerSample > 0 {
		store.Fill(ref, "BitDepth", strconv.Itoa(int(bitsPerSample)), fillReplace)
	}

	overrides := []jsonKV{{Key: "Compression_Mode", Val: "Lossless"}}
	if duration > 0 {
		overrides = append(overrides, jsonKV{Key: "Duration", Val: formatJSONSeconds(duration)})
	}
	if totalSamples > 0 {
		overrides = append(overrides, jsonKV{Key: "SamplingCount", Val: strconv.FormatUint(totalSamples, 10)})
	}
	if rawBitrate != "" {
		overrides = append(overrides, jsonKV{Key: "BitRate", Val: rawBitrate})
	}
	if streamSize > 0 {
		overrides = append(overrides, jsonKV{Key: "StreamSize", Val: strconv.FormatInt(streamSize, 10)})
	}
	if encoder != "" {
		overrides = append(overrides, jsonKV{Key: "Encoded_Library", Val: encoder})
	}
	if encodedLibraryName != "" {
		overrides = append(overrides, jsonKV{Key: "Encoded_Library_Name", Val: encodedLibraryName})
	}
	if encodedLibraryVersion != "" {
		overrides = append(overrides, jsonKV{Key: "Encoded_Library_Version", Val: encodedLibraryVersion})
	}
	if encodedLibraryDate != "" {
		overrides = append(overrides, jsonKV{Key: "Encoded_Library_Date", Val: encodedLibraryDate})
	}
	sort.Slice(overrides, func(left, right int) bool { return overrides[left].Key < overrides[right].Key })
	for _, override := range overrides {
		name := fieldName(override.Key)
		if _, ok := store.Get(ref, name); !ok {
			fillGeneratedStructured(store, ref, name, override.Val)
		}
	}
	if md5Hex != "" {
		node := structuredObjectFromKVs([]jsonKV{{Key: "MD5_Unencoded", Val: md5Hex}})
		raw := structuredNodeText(node)
		_, known := structuredFieldSpec(StreamAudio, "extra")
		store.appendEntry(store.stream(ref), fieldEntry{
			Name:          "extra",
			Value:         fieldValue{Text: raw},
			Dynamic:       !known,
			Options:       fieldOptions{ShowStructured: true, ShowXML: true, ValueType: fieldValueNode},
			StructuredKey: "extra",
			Node:          &node,
		})
	}
	return canonicalStreamSnapshot(store, ref, canonicalStreamPolicy{SkipStreamOrder: true})
}

func parseFLACPicture(data []byte) (mime string, typ string, ok bool) {
	// https://xiph.org/flac/format.html#metadata_block_picture
	// picture_type(32), mime_length(32), mime, desc_length(32), desc, width(32), height(32),
	// depth(32), colors(32), data_length(32), data
	if len(data) < 32 {
		return "", "", false
	}
	picType := binary.BigEndian.Uint32(data[0:4])
	pos := uint64(4)
	mimeLen := uint64(binary.BigEndian.Uint32(data[pos : pos+4]))
	pos += 4
	if mimeLen > uint64(len(data))-pos {
		return "", "", false
	}
	mime = string(data[int(pos):int(pos+mimeLen)])
	pos += mimeLen
	if uint64(len(data))-pos < 4 {
		return "", "", false
	}
	descLen := uint64(binary.BigEndian.Uint32(data[int(pos) : int(pos)+4]))
	pos += 4 + descLen
	if pos > uint64(len(data)) || uint64(len(data))-pos < 20 {
		return "", "", false
	}
	// Skip width/height/depth/colors.
	pos += 16
	_ = binary.BigEndian.Uint32(data[int(pos) : int(pos)+4]) // data_length
	switch picType {
	case 3:
		typ = "Cover (front)"
	case 4:
		typ = "Cover (back)"
	default:
		typ = ""
	}
	return mime, typ, true
}

// readFLACPictureHeader reads only the bounded picture type and MIME prefix.
// The caller remains responsible for seeking to the validated block end.
func readFLACPictureHeader(file io.ReadSeeker, blockLen int64, assetBudget *embeddedAssetBudget) (mime string, typ string, ok bool, err error) {
	if blockLen < 8 || assetBudget == nil {
		return "", "", false, nil
	}
	var header [8]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return "", "", false, err
	}
	picType := binary.BigEndian.Uint32(header[0:4])
	mimeLen := uint64(binary.BigEndian.Uint32(header[4:8]))
	if mimeLen > uint64(blockLen-8) || assetBudget.reserveString(mimeLen, embeddedAssetMaxMIMEBytes) != embeddedAssetAccepted {
		return "", "", false, nil
	}
	mimeBytes := make([]byte, int(mimeLen))
	if _, err := io.ReadFull(file, mimeBytes); err != nil {
		return "", "", false, err
	}
	var descriptionSize [4]byte
	if _, err := io.ReadFull(file, descriptionSize[:]); err != nil {
		return "", "", false, err
	}
	descLen := uint64(binary.BigEndian.Uint32(descriptionSize[:]))
	remaining := uint64(blockLen-8) - mimeLen
	if remaining < 24 || descLen > remaining-24 || assetBudget.reserveString(descLen, embeddedAssetMaxNameBytes) != embeddedAssetAccepted {
		return "", "", false, nil
	}
	if _, err := file.Seek(int64(descLen), io.SeekCurrent); err != nil {
		return "", "", false, err
	}
	var imageHeader [20]byte
	if _, err := io.ReadFull(file, imageHeader[:]); err != nil {
		return "", "", false, err
	}
	dataLen := uint64(binary.BigEndian.Uint32(imageHeader[16:20]))
	if dataLen > remaining-24-descLen {
		return "", "", false, nil
	}
	switch picType {
	case 3:
		typ = "Cover (front)"
	case 4:
		typ = "Cover (back)"
	}
	return string(mimeBytes), typ, true, nil
}

func parseFLACStreamInfo(data []byte) (uint32, uint8, uint8, uint64, string) {
	info, ok := parseFLACStreamInfoDetails(data)
	if !ok {
		return 0, 0, 0, 0, ""
	}
	return info.sampleRate, info.channels, info.bitsPerSample, info.totalSamples, info.md5
}

// parseFLACStreamInfoDetails decodes a 34-byte STREAMINFO payload and reports
// false when the payload is truncated or lacks usable audio properties.
func parseFLACStreamInfoDetails(data []byte) (flacStreamInfo, bool) {
	if len(data) < 34 {
		return flacStreamInfo{}, false
	}
	sampleRate := uint32(data[10])<<12 | uint32(data[11])<<4 | uint32(data[12])>>4
	channels := ((data[12] & 0x0E) >> 1) + 1
	bitsPerSample := ((data[12] & 0x01) << 4) | (data[13] >> 4)
	bitsPerSample++

	totalSamples := uint64(data[13]&0x0F)<<32 | uint64(binary.BigEndian.Uint32(data[14:18]))
	md5 := data[18:34]
	allZero := true
	for _, b := range md5 {
		if b != 0 {
			allZero = false
			break
		}
	}
	md5Hex := ""
	if !allZero {
		md5Hex = strings.ToUpper(hex.EncodeToString(md5))
	}
	return flacStreamInfo{
		minBlockSize:  binary.BigEndian.Uint16(data[0:2]),
		maxBlockSize:  binary.BigEndian.Uint16(data[2:4]),
		sampleRate:    sampleRate,
		channels:      channels,
		bitsPerSample: bitsPerSample,
		totalSamples:  totalSamples,
		md5:           md5Hex,
	}, sampleRate > 0 && channels > 0
}

// parseMatroskaFLACPrivate accepts either a bare STREAMINFO payload or a full
// fLaC metadata sequence and returns its STREAMINFO plus Vorbis vendor string.
func parseMatroskaFLACPrivate(data []byte) (flacStreamInfo, string, bool) {
	if len(data) == 34 {
		info, ok := parseFLACStreamInfoDetails(data)
		return info, "", ok
	}
	if len(data) < 8 || !bytes.Equal(data[:4], []byte("fLaC")) {
		return flacStreamInfo{}, "", false
	}

	var info flacStreamInfo
	var vendor string
	hasStreamInfo := false
	pos := 4
	for pos+4 <= len(data) {
		header := data[pos]
		blockType := header & 0x7F
		blockSize := int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
		pos += 4
		if blockSize < 0 || pos+blockSize > len(data) {
			return flacStreamInfo{}, "", false
		}
		block := data[pos : pos+blockSize]
		switch blockType {
		case 0:
			parsed, ok := parseFLACStreamInfoDetails(block)
			if !ok {
				return flacStreamInfo{}, "", false
			}
			info = parsed
			hasStreamInfo = true
		case 4:
			parsedVendor, _ := parseFLACVorbisComment(block)
			if parsedVendor != "" {
				vendor = parsedVendor
			}
		}
		pos += blockSize
		if header&0x80 != 0 {
			return info, vendor, hasStreamInfo
		}
	}
	return flacStreamInfo{}, "", false
}

func parseFLACVorbisComment(buf []byte) (string, []flacTagKV) {
	out := []flacTagKV{}
	if len(buf) < 8 {
		return "", out
	}
	rd := buf
	vendorLen := int(binary.LittleEndian.Uint32(rd[0:4]))
	rd = rd[4:]
	if vendorLen < 0 || vendorLen > len(rd) {
		return "", out
	}
	vendor := string(rd[:vendorLen])
	rd = rd[vendorLen:]
	if len(rd) < 4 {
		return vendor, out
	}
	n := int(binary.LittleEndian.Uint32(rd[0:4]))
	rd = rd[4:]
	for i := 0; i < n; i++ {
		if len(rd) < 4 {
			break
		}
		l := int(binary.LittleEndian.Uint32(rd[0:4]))
		rd = rd[4:]
		if l < 0 || l > len(rd) {
			break
		}
		s := string(rd[:l])
		rd = rd[l:]
		if eq := strings.IndexByte(s, '='); eq > 0 {
			k := strings.ToUpper(strings.TrimSpace(s[:eq]))
			v := strings.TrimSpace(s[eq+1:])
			if k != "" {
				out = append(out, flacTagKV{Key: k, Val: v})
			}
		}
	}
	return vendor, out
}

func splitFLACEncodedLibrary(value string) (name, version, date string) {
	// Example: "reference libFLAC 1.5.0 20250211"
	// MediaInfo emits: Encoded_Library_Name=libFLAC, Encoded_Library_Version=1.5.0, Encoded_Library_Date=2025-02-11
	if !strings.Contains(value, "libFLAC") {
		return "", "", ""
	}
	parts := strings.Fields(value)
	for i := 0; i < len(parts); i++ {
		if parts[i] != "libFLAC" {
			continue
		}
		name = "libFLAC"
		if i+1 < len(parts) {
			version = parts[i+1]
		}
		for j := i + 1; j < len(parts); j++ {
			p := parts[j]
			if len(p) == 8 && isAllDigits(p) {
				date = fmt.Sprintf("%s-%s-%s", p[0:4], p[4:6], p[6:8])
				break
			}
		}
		return name, version, date
	}
	return "", "", ""
}

// flacDerivedLayoutIsOmitted reports whether MediaInfo omits synthesized FLAC
// channel positions for the supplied libFLAC vendor. Unknown versions,
// libFLAC 1.3, and libFLAC 1.5 or later use the omission behavior observed in
// MediaInfo; 1.2 and 1.4 retain their derived layouts.
func flacDerivedLayoutIsOmitted(vendor string) bool {
	if strings.HasPrefix(vendor, "Lavf") {
		return false
	}
	if strings.TrimSpace(vendor) == "" {
		return true
	}
	name, version, _ := splitFLACEncodedLibrary(vendor)
	if name == "" {
		return false
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return true
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return true
	}
	return major > 1 || major == 1 && (minor == 3 || version == "1.4.2" || minor >= 5)
}

// flacTagsToGeneralFacts maps Vorbis comments to canonical General scalars and
// retains unrecognized comments as one ordered extra object.
func flacTagsToGeneralFacts(tags map[string]string, encoder string) (*canonicalStructuredFacts, *structuredNode) {
	general := &canonicalStructuredFacts{}

	mapped := map[string]bool{}
	set := func(key, val string) {
		if val == "" {
			return
		}
		general.SetSame(fieldName(key), val)
	}

	if strings.HasPrefix(encoder, "Lavf") {
		set("Encoded_Application", encoder)
	}

	if v := tags["ALBUM"]; v != "" {
		set("Album", v)
		mapped["ALBUM"] = true
	}
	if v := firstNonEmpty(tags["ALBUMARTIST"], tags["ALBUM_ARTIST"], tags["ALBUM ARTIST"]); v != "" {
		set("Album_Performer", v)
		mapped["ALBUMARTIST"] = tags["ALBUMARTIST"] != ""
		mapped["ALBUM_ARTIST"] = tags["ALBUM_ARTIST"] != ""
		mapped["ALBUM ARTIST"] = tags["ALBUM ARTIST"] != ""
	}
	if v := tags["ARTIST"]; v != "" {
		set("Performer", v)
		mapped["ARTIST"] = true
	}
	if v := tags["GENRE"]; v != "" {
		set("Genre", v)
		mapped["GENRE"] = true
	}
	if v := tags["COMPOSER"]; v != "" {
		set("Composer", v)
		mapped["COMPOSER"] = true
	}
	if v := tags["COPYRIGHT"]; v != "" {
		set("Copyright", v)
		mapped["COPYRIGHT"] = true
	}
	if v := tags["ISRC"]; v != "" {
		set("ISRC", v)
		mapped["ISRC"] = true
	}
	if v := tags["LABEL"]; v != "" {
		set("Label", v)
		mapped["LABEL"] = true
	}
	if v := tags["TITLE"]; v != "" {
		set("Title", v)
		set("Track", v)
		mapped["TITLE"] = true
	}
	if v := tags["TRACKNUMBER"]; v != "" {
		set("Track_Position", v)
		mapped["TRACKNUMBER"] = true
	}
	if v := firstNonEmpty(tags["TOTALTRACKS"], tags["TRACKTOTAL"]); v != "" {
		set("Track_Position_Total", v)
		mapped["TOTALTRACKS"] = tags["TOTALTRACKS"] != ""
		mapped["TRACKTOTAL"] = tags["TRACKTOTAL"] != ""
	}
	if v := tags["DISCNUMBER"]; v != "" {
		set("Part", v)
		mapped["DISCNUMBER"] = true
	}
	if v := firstNonEmpty(tags["TOTALDISCS"], tags["DISCTOTAL"]); v != "" {
		set("Part_Position_Total", v)
		mapped["TOTALDISCS"] = tags["TOTALDISCS"] != ""
		mapped["DISCTOTAL"] = tags["DISCTOTAL"] != ""
	}
	if v := tags["DATE"]; v != "" {
		// MediaInfo often exposes both date and year for audio tags.
		if len(v) >= 4 && isAllDigits(v[0:4]) && strings.Contains(v, "-") {
			set("Recorded_Date", v+" / "+v[0:4])
		} else {
			set("Recorded_Date", v)
		}
		mapped["DATE"] = true
	}
	if v := tags["YEAR"]; v != "" {
		if general.Projection("Recorded_Date") == "" {
			set("Recorded_Date", v)
		}
		mapped["YEAR"] = true
	}

	// Remaining tags go under General.extra (raw JSON object).
	extraFields := make([]jsonKV, 0, len(tags))
	for k, v := range tags {
		if v == "" || mapped[k] || k == "ENCODER" {
			continue
		}
		extraFields = append(extraFields, jsonKV{Key: k, Val: v})
	}
	var extra *structuredNode
	if len(extraFields) > 0 {
		node := structuredObjectFromKVs(extraFields)
		extra = &node
	}
	return general, extra
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
