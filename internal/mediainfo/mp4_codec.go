package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type SampleInfo struct {
	Format          string
	Fields          []Field
	SampleCount     uint64
	SampleBytes     uint64
	SampleDelta     uint32
	LastSampleDelta uint32
	Width           uint64
	Height          uint64
}

type sampleEntryResult struct {
	Fields []Field
	Format string
	Width  uint64
	Height uint64
}

func parseStsdForSample(buf []byte) (SampleInfo, bool) {
	if len(buf) < 16 {
		return SampleInfo{}, false
	}
	count := binary.BigEndian.Uint32(buf[4:8])
	offset := 8
	for i := 0; i < int(count); i++ {
		if offset+8 > len(buf) {
			return SampleInfo{}, false
		}
		size := int(binary.BigEndian.Uint32(buf[offset : offset+4]))
		if size < 8 || offset+size > len(buf) {
			return SampleInfo{}, false
		}
		entry := buf[offset : offset+size]
		typ := string(entry[4:8])
		format := mapMP4SampleEntry(typ)
		info := SampleInfo{Format: format}
		if isVideoSampleEntry(typ) {
			result := parseVisualSampleEntry(entry, typ)
			info.Fields = append(info.Fields, result.Fields...)
			if result.Format != "" {
				info.Format = result.Format
			}
			if result.Width > 0 {
				info.Width = result.Width
			}
			if result.Height > 0 {
				info.Height = result.Height
			}
		}
		if isAudioSampleEntry(typ) {
			result := parseAudioSampleEntry(entry, typ)
			info.Fields = append(info.Fields, result.Fields...)
			if result.Format != "" {
				info.Format = result.Format
			}
		}
		if info.Format != "" || len(info.Fields) > 0 {
			return info, true
		}
		offset += size
	}
	return SampleInfo{}, false
}

func mapMP4SampleEntry(sample string) string {
	switch sample {
	case "avc1", "avc3":
		return "AVC"
	case "hvc1", "hev1":
		return "HEVC"
	case "mp4v":
		return "MPEG-4 Visual"
	case "mp4a":
		return "AAC"
	case "ac-3", "ac-4":
		return "AC-3"
	case "ec-3":
		return "E-AC-3"
	case "alac":
		return "ALAC"
	case "flac":
		return "FLAC"
	case "Opus", "opus":
		return "Opus"
	case "mp4s":
		return "MPEG-4 Systems"
	case "tx3g":
		return "Text"
	case "wvtt":
		return "WebVTT"
	default:
		return ""
	}
}

func isVideoSampleEntry(sample string) bool {
	switch sample {
	case "avc1", "avc3", "hvc1", "hev1", "mp4v":
		return true
	default:
		return false
	}
}

func isAudioSampleEntry(sample string) bool {
	switch sample {
	case "mp4a", "ac-3", "ec-3", "alac", "flac", "opus":
		return true
	default:
		return false
	}
}

func parseVisualSampleEntry(entry []byte, sampleType string) sampleEntryResult {
	if len(entry) < 36 {
		return sampleEntryResult{}
	}
	width := binary.BigEndian.Uint16(entry[32:34])
	height := binary.BigEndian.Uint16(entry[34:36])
	fields := []Field{
		{Name: "Codec ID", Value: sampleType},
	}
	if formatInfo := mapVideoFormatInfo(sampleType); formatInfo != "" {
		fields = append(fields, Field{Name: "Format/Info", Value: formatInfo})
	}
	if width > 0 {
		fields = append(fields, Field{Name: "Width", Value: formatPixels(uint64(width))})
	}
	if height > 0 {
		fields = append(fields, Field{Name: "Height", Value: formatPixels(uint64(height))})
	}
	if width > 0 && height > 0 {
		if ar := formatAspectRatio(uint64(width), uint64(height)); ar != "" {
			fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
		}
	}
	if sampleType == "avc1" || sampleType == "avc3" {
		if payload, ok := findMP4ChildBox(entry, mp4VisualSampleEntryHeaderSize, "avcC"); ok {
			_, avcFields, _ := parseAVCConfig(payload)
			fields = append(fields, avcFields...)
			fields = append(fields, Field{Name: "Codec configuration box", Value: "avcC"})
		} else if payload, ok := findMP4BoxByName(entry, "avcC"); ok {
			_, avcFields, _ := parseAVCConfig(payload)
			fields = append(fields, avcFields...)
			fields = append(fields, Field{Name: "Codec configuration box", Value: "avcC"})
		}
	}
	if info := mapVideoCodecIDInfo(sampleType); info != "" {
		fields = append(fields, Field{Name: "Codec ID/Info", Value: info})
	}
	if _, max, avg, ok := parseBtrt(entry, mp4VisualSampleEntryHeaderSize); ok {
		if max > 0 {
			fields = append(fields, Field{Name: "Nominal bit rate", Value: formatBitrate(float64(max))})
			fields = append(fields, Field{Name: "Maximum bit rate", Value: formatBitrate(float64(max))})
		} else if avg > 0 {
			fields = append(fields, Field{Name: "Nominal bit rate", Value: formatBitrate(float64(avg))})
		}
	}
	return sampleEntryResult{Fields: fields, Width: uint64(width), Height: uint64(height)}
}

func parseAudioSampleEntry(entry []byte, sampleType string) sampleEntryResult {
	if len(entry) < 36 {
		return sampleEntryResult{}
	}
	channels := binary.BigEndian.Uint16(entry[24:26])
	sampleRate := binary.BigEndian.Uint32(entry[32:36])
	codecID := sampleType
	fields := []Field{}
	if channels > 0 {
		fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(uint64(channels))})
		if layout := channelLayout(uint64(channels)); layout != "" {
			fields = append(fields, Field{Name: "Channel layout", Value: layout})
		}
	}
	if sampleRate > 0 {
		rate := float64(sampleRate) / 65536
		fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(rate)})
		if sampleType == "mp4a" {
			frameRate := rate / 1024.0
			fields = append(fields, Field{Name: "Frame rate", Value: fmt.Sprintf("%.3f FPS (1024 SPF)", frameRate)})
		}
	}
	format := ""
	if sampleType == "mp4a" {
		if profile, codecIDValue, info := parseESDSProfile(entry); profile != "" {
			if info != "" {
				fields = append(fields, Field{Name: "Format/Info", Value: info})
			}
			if codecIDValue != "" {
				codecID = codecIDValue
			}
			format = "AAC " + profile
		} else if info := mapAudioFormatInfo(sampleType); info != "" {
			fields = append(fields, Field{Name: "Format/Info", Value: info})
		}
	} else if info := mapAudioFormatInfo(sampleType); info != "" {
		fields = append(fields, Field{Name: "Format/Info", Value: info})
	}
	if sampleType == "mp4a" {
		fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
	}
	if _, max, avg, ok := parseBtrt(entry, mp4AudioSampleEntryHeaderSize); ok {
		if max > 0 {
			fields = append(fields, Field{Name: "Maximum bit rate", Value: formatBitrate(float64(max))})
		} else if avg > 0 {
			fields = append(fields, Field{Name: "Maximum bit rate", Value: formatBitrate(float64(avg))})
		}
	}
	fields = append(fields, Field{Name: "Codec ID", Value: codecID})
	return sampleEntryResult{Fields: fields, Format: format}
}

const (
	mp4VisualSampleEntryHeaderSize = 78
	mp4AudioSampleEntryHeaderSize  = 36
	mp4AudioSampleEntryHeaderAlt   = 28
)

func mapAVCProfile(profileID byte) string {
	switch profileID {
	case 66:
		return "Baseline"
	case 77:
		return "Main"
	case 88:
		return "Extended"
	case 100:
		return "High"
	case 110:
		return "High 10"
	case 122:
		return "High 4:2:2"
	case 244:
		return "High 4:4:4 Predictive"
	default:
		return ""
	}
}

func formatAVCLevel(levelID byte) string {
	if levelID == 0 {
		return ""
	}
	major := int(levelID) / 10
	minor := int(levelID) % 10
	if minor == 0 {
		return fmt.Sprintf("L%d", major)
	}
	return fmt.Sprintf("L%d.%d", major, minor)
}

func parseESDSProfile(entry []byte) (string, string, string) {
	payload, ok := findMP4ChildBox(entry, mp4AudioSampleEntryHeaderSize, "esds")
	if !ok {
		payload, ok = findMP4ChildBox(entry, mp4AudioSampleEntryHeaderAlt, "esds")
	}
	if !ok {
		payload, ok = findMP4BoxByName(entry, "esds")
	}
	if !ok || len(payload) <= 4 {
		return "", "", ""
	}
	decoder := findESDSDecoderSpecificInfo(payload[4:])
	if len(decoder) == 0 {
		return "", "", ""
	}
	objType := int(decoder[0] >> 3)
	if objType == 31 && len(decoder) > 1 {
		objType = 32 + int((decoder[0]&0x07)<<3|decoder[1]>>5)
	}
	profile := mapAACProfile(objType)
	info := "Advanced Audio Codec"
	if profile == "LC" {
		info = "Advanced Audio Codec Low Complexity"
	}
	codecID := ""
	if objType > 0 {
		codecID = fmt.Sprintf("mp4a-40-%d", objType)
	}
	return profile, codecID, info
}

func findMP4BoxByName(buf []byte, name string) ([]byte, bool) {
	idx := bytes.Index(buf, []byte(name))
	if idx < 4 {
		return nil, false
	}
	size := int(binary.BigEndian.Uint32(buf[idx-4 : idx]))
	if size < 8 || idx-4+size > len(buf) {
		return nil, false
	}
	return buf[idx+4 : idx-4+size], true
}

func mapAACProfile(objType int) string {
	switch objType {
	case 1:
		return "Main"
	case 2:
		return "LC"
	case 3:
		return "SSR"
	case 4:
		return "LTP"
	case 5:
		return "SBR"
	case 29:
		return "HE-AAC v2"
	default:
		return ""
	}
}

func findESDSDecoderSpecificInfo(buf []byte) []byte {
	for i := range buf {
		if buf[i] != 0x05 {
			continue
		}
		length, n := readMP4DescriptorLength(buf[i+1:])
		if n == 0 {
			continue
		}
		start := i + 1 + n
		if start+length > len(buf) {
			continue
		}
		return buf[start : start+length]
	}
	return nil
}

func readMP4DescriptorLength(buf []byte) (int, int) {
	value := 0
	for i := 0; i < 4 && i < len(buf); i++ {
		value = (value << 7) | int(buf[i]&0x7F)
		if buf[i]&0x80 == 0 {
			return value, i + 1
		}
	}
	return 0, 0
}

func findMP4ChildBox(entry []byte, start int, name string) ([]byte, bool) {
	if start < 0 || start+8 > len(entry) {
		return nil, false
	}
	pos := start
	for pos+8 <= len(entry) {
		size := int(binary.BigEndian.Uint32(entry[pos : pos+4]))
		if size < 8 || pos+size > len(entry) {
			return nil, false
		}
		typ := string(entry[pos+4 : pos+8])
		if typ == name {
			return entry[pos+8 : pos+size], true
		}
		pos += size
	}
	return nil, false
}

func parseBtrt(entry []byte, start int) (uint32, uint32, uint32, bool) {
	payload, ok := findMP4ChildBox(entry, start, "btrt")
	if !ok {
		payload, ok = findMP4BoxByName(entry, "btrt")
	}
	if !ok || len(payload) < 12 {
		return 0, 0, 0, false
	}
	bufferSize := binary.BigEndian.Uint32(payload[0:4])
	maxBitrate := binary.BigEndian.Uint32(payload[4:8])
	avgBitrate := binary.BigEndian.Uint32(payload[8:12])
	return bufferSize, maxBitrate, avgBitrate, true
}

func mapVideoFormatInfo(sampleType string) string {
	switch sampleType {
	case "avc1", "avc3":
		return "Advanced Video Codec"
	case "hvc1", "hev1":
		return "High Efficiency Video Coding"
	case "mp4v":
		return "MPEG-4 Visual"
	default:
		return ""
	}
}

func mapVideoCodecIDInfo(sampleType string) string {
	switch sampleType {
	case "avc1", "avc3":
		return "Advanced Video Coding"
	case "hvc1", "hev1":
		return "High Efficiency Video Coding"
	case "mp4v":
		return "MPEG-4 Visual"
	default:
		return ""
	}
}

func mapAudioFormatInfo(sampleType string) string {
	switch sampleType {
	case "mp4a":
		return "Advanced Audio Codec"
	case "ac-3":
		return "Audio Coding 3"
	case "ec-3":
		return "Enhanced AC-3"
	default:
		return ""
	}
}
