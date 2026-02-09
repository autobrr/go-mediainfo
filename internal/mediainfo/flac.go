package mediainfo

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

type flacTagKV struct {
	Key string
	Val string
}

func ParseFLAC(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, map[string]string, map[string]string, bool) {
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

	for {
		var blockHeader [4]byte
		if _, err := io.ReadFull(file, blockHeader[:]); err != nil {
			break
		}
		isLast := (blockHeader[0] & 0x80) != 0
		blockType := blockHeader[0] & 0x7F
		blockLen := int(blockHeader[1])<<16 | int(blockHeader[2])<<8 | int(blockHeader[3])
		if blockLen <= 0 {
			if isLast {
				break
			}
			continue
		}
		if blockType == 0 {
			if blockLen < 34 {
				if _, err := file.Seek(int64(blockLen), io.SeekCurrent); err != nil {
					break
				}
			} else {
				var streamInfo [34]byte
				if _, err := io.ReadFull(file, streamInfo[:]); err != nil {
					break
				}
				sampleRate, channels, bitsPerSample, totalSamples, md5Hex = parseFLACStreamInfo(streamInfo[:])
				if blockLen > 34 {
					if _, err := file.Seek(int64(blockLen-34), io.SeekCurrent); err != nil {
						break
					}
				}
			}
		} else if blockType == 4 {
			// VorbisComment. Primary source for tags like ENCODER, TITLE, ALBUM, etc.
			buf := make([]byte, blockLen)
			if _, err := io.ReadFull(file, buf); err != nil {
				break
			}
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
		} else {
			if blockType == 6 {
				// METADATA_BLOCK_PICTURE (cover art). We only need the mime/type to match MediaInfo.
				buf := make([]byte, blockLen)
				if _, err := io.ReadFull(file, buf); err != nil {
					break
				}
				if coverMIME == "" {
					if mime, typ, ok := parseFLACPicture(buf); ok {
						coverMIME = mime
						coverType = typ
					}
				}
			} else {
				if _, err := file.Seek(int64(blockLen), io.SeekCurrent); err != nil {
					break
				}
			}
		}
		if isLast {
			// The file cursor is now positioned at the start of the audio frames.
			if pos, err := file.Seek(0, io.SeekCurrent); err == nil {
				audioStart = pos
			}
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

	fields := []Field{
		{Name: "Format", Value: "FLAC"},
	}
	fields = appendChannelFields(fields, uint64(channels))
	fields = appendSampleRateField(fields, float64(sampleRate))
	if bitsPerSample > 0 {
		fields = append(fields, Field{Name: "Bit depth", Value: formatBitDepth(bitsPerSample)})
	}
	fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
	fields = addStreamCommon(fields, duration, bitrate)

	streamJSON := map[string]string{}
	streamJSONRaw := map[string]string{}
	if duration > 0 {
		streamJSON["Duration"] = formatJSONSeconds(duration)
	}
	if totalSamples > 0 {
		streamJSON["SamplingCount"] = strconv.FormatUint(totalSamples, 10)
	}
	streamJSON["Compression_Mode"] = "Lossless"
	if audioStart > 0 && audioStart < size {
		payload := size - audioStart
		streamJSON["StreamSize"] = strconv.FormatInt(payload, 10)
		if totalSamples > 0 && sampleRate > 0 {
			// MediaInfo's FLAC bitrates use Duration in integer milliseconds.
			durationMs := int64((totalSamples*1000 + uint64(sampleRate)/2) / uint64(sampleRate))
			if durationMs > 0 {
				// Round to nearest b/s (MediaInfo output is exact integer).
				br := (payload*8000 + durationMs/2) / durationMs
				if br > 0 {
					streamJSON["BitRate"] = strconv.FormatInt(br, 10)
				}
			}
		}
	}
	if encoder != "" {
		// Match MediaInfo naming: ENCODER becomes Encoded_Application (General) and Encoded_Library (Audio).
		streamJSON["Encoded_Library"] = encoder
		if name, version, date := splitFLACEncodedLibrary(encoder); name != "" {
			streamJSON["Encoded_Library_Name"] = name
			if version != "" {
				streamJSON["Encoded_Library_Version"] = version
			}
			if date != "" {
				streamJSON["Encoded_Library_Date"] = date
			}
		}
	}
	if md5Hex != "" {
		streamJSONRaw["extra"] = renderJSONObject([]jsonKV{{Key: "MD5_Unencoded", Val: md5Hex}}, false)
	}

	generalJSON, generalJSONRaw := flacTagsToGeneralJSON(tags, encoder)
	if coverMIME != "" && generalJSON != nil {
		generalJSON["Cover"] = "Yes"
		generalJSON["Cover_Mime"] = coverMIME
		if coverType != "" {
			generalJSON["Cover_Type"] = coverType
		}
	}

	return info, []Stream{{Kind: StreamAudio, Fields: fields, JSON: streamJSON, JSONRaw: streamJSONRaw, JSONSkipStreamOrder: true}}, generalJSON, generalJSONRaw, true
}

func parseFLACPicture(data []byte) (mime string, typ string, ok bool) {
	// https://xiph.org/flac/format.html#metadata_block_picture
	// picture_type(32), mime_length(32), mime, desc_length(32), desc, width(32), height(32),
	// depth(32), colors(32), data_length(32), data
	if len(data) < 32 {
		return "", "", false
	}
	picType := binary.BigEndian.Uint32(data[0:4])
	pos := 4
	mimeLen := int(binary.BigEndian.Uint32(data[pos : pos+4]))
	pos += 4
	if mimeLen < 0 || pos+mimeLen > len(data) {
		return "", "", false
	}
	mime = string(data[pos : pos+mimeLen])
	pos += mimeLen
	if pos+4 > len(data) {
		return "", "", false
	}
	descLen := int(binary.BigEndian.Uint32(data[pos : pos+4]))
	pos += 4 + descLen
	if pos+20 > len(data) {
		return "", "", false
	}
	// Skip width/height/depth/colors.
	pos += 16
	if pos+4 > len(data) {
		return "", "", false
	}
	_ = binary.BigEndian.Uint32(data[pos : pos+4]) // data_length
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

func parseFLACStreamInfo(data []byte) (uint32, uint8, uint8, uint64, string) {
	if len(data) < 34 {
		return 0, 0, 0, 0, ""
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
	return sampleRate, channels, bitsPerSample, totalSamples, md5Hex
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

func flacTagsToGeneralJSON(tags map[string]string, encoder string) (map[string]string, map[string]string) {
	if len(tags) == 0 && encoder == "" {
		return nil, nil
	}
	general := map[string]string{}
	raw := map[string]string{}

	mapped := map[string]bool{}
	set := func(key, val string) {
		if val == "" {
			return
		}
		general[key] = val
	}

	if strings.HasPrefix(encoder, "Lavf") {
		set("Encoded_Application", encoder)
	}

	if v := tags["ALBUM"]; v != "" {
		set("Album", v)
		mapped["ALBUM"] = true
	}
	if v := tags["ALBUMARTIST"]; v != "" {
		// MediaInfo only emits Album_Performer when it differs from Performer.
		if tags["ARTIST"] == "" || tags["ARTIST"] != v {
			set("Album_Performer", v)
		}
		mapped["ALBUMARTIST"] = true
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
		if general["Recorded_Date"] == "" {
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
	if len(extraFields) > 0 {
		raw["extra"] = renderJSONObject(extraFields, false)
	}
	if len(general) == 0 {
		general = nil
	}
	if len(raw) == 0 {
		raw = nil
	}
	return general, raw
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
