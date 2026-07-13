package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// matroskaAttachmentImageStream converts a JPEG or PNG attachment into the
// Image stream fields emitted by MediaInfo for Matroska cover art.
func matroskaAttachmentImageStream(attachment matroskaAttachment) (Stream, bool) {
	stream := Stream{
		Kind:                StreamImage,
		JSON:                map[string]string{"Type": "Cover", "MuxingMode": "Attachment"},
		JSONRaw:             map[string]string{},
		JSONSkipStreamOrder: true,
		JSONSkipComputed:    true,
	}
	stream.Fields = append(stream.Fields, Field{Name: "Title", Value: attachment.name})
	data := attachment.data
	size := attachment.size
	if size == 0 {
		size = int64(len(data))
	}
	if width, height, depth, subsampling, iccSpace, iccDescription, metadataBytes, ok := parseJPEGAttachment(data); ok {
		stream.Fields = append(stream.Fields, Field{Name: "Format", Value: "JPEG"})
		stream.JSON["Width"] = strconv.Itoa(width)
		stream.JSON["Height"] = strconv.Itoa(height)
		stream.JSON["ColorSpace"] = "YUV"
		stream.JSON["ChromaSubsampling"] = subsampling
		stream.JSON["BitDepth"] = strconv.Itoa(depth)
		stream.JSON["Compression_Mode"] = "Lossy"
		if iccDescription != "" {
			stream.JSON["colour_description_present"] = "Yes"
			stream.JSON["colour_range"] = "Full"
			stream.JSON["colour_primaries"] = "BT.709"
			stream.JSON["transfer_characteristics"] = "sRGB/sYCC"
			stream.JSON["matrix_coefficients"] = "Identity"
			extra := []jsonKV{}
			if iccSpace != "" {
				extra = append(extra, jsonKV{Key: "ColorSpace_ICC", Val: iccSpace})
			}
			extra = append(extra, jsonKV{Key: "colour_primaries_ICC_Description", Val: iccDescription})
			stream.JSONRaw["extra"] = renderJSONObject(extra, false)
		}
		size -= int64(metadataBytes)
	} else if width, height, depth, gamma, ok := parsePNGAttachment(data); ok {
		stream.Fields = append(stream.Fields, Field{Name: "Format", Value: "PNG"})
		stream.JSON["Format_Compression"] = "Deflate"
		stream.JSON["Format_Settings_Packing"] = "Linear"
		stream.JSON["Width"] = strconv.Itoa(width)
		stream.JSON["Height"] = strconv.Itoa(height)
		stream.JSON["PixelAspectRatio"] = "1.000"
		stream.JSON["DisplayAspectRatio"] = formatJSONFloat(float64(width) / float64(height))
		stream.JSON["ColorSpace"] = "RGB"
		stream.JSON["BitDepth"] = strconv.Itoa(depth)
		stream.JSON["Compression_Mode"] = "Lossless"
		if gamma != "" {
			stream.JSONRaw["extra"] = renderJSONObject([]jsonKV{{Key: "Gamma", Val: gamma}}, false)
		}
	} else {
		return Stream{}, false
	}
	stream.JSON["StreamSize"] = strconv.FormatInt(size, 10)
	return stream, true
}

// parseJPEGAttachment reads dimensions, sample depth, chroma sampling, known
// ICC metadata, and removable metadata byte count from a JPEG payload.
func parseJPEGAttachment(data []byte) (width, height, depth int, subsampling, iccSpace, iccDescription string, metadataBytes int, ok bool) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0, 0, "", "", "", 0, false
	}
	for pos := 2; pos+4 <= len(data); {
		if data[pos] != 0xFF {
			pos++
			continue
		}
		for pos < len(data) && data[pos] == 0xFF {
			pos++
		}
		if pos >= len(data) {
			break
		}
		marker := data[pos]
		pos++
		if marker == 0xD9 || marker == 0xDA {
			break
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			continue
		}
		if pos+2 > len(data) {
			break
		}
		length := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if length < 2 || pos+length > len(data) {
			break
		}
		segment := data[pos+2 : pos+length]
		if marker >= 0xE0 && marker <= 0xEF || marker == 0xFE {
			metadataBytes += length + 2
		}
		if marker == 0xE2 && bytes.Contains(segment, []byte("ICC_PROFILE")) {
			iccSpace = "RGB"
			if bytes.Contains(segment, []byte("sRGB IEC61966-2.1")) || bytes.Contains(data, []byte("sRGB IEC61966-2.1")) {
				iccDescription = "sRGB IEC61966-2.1"
			}
		}
		if (marker >= 0xC0 && marker <= 0xC3 || marker >= 0xC5 && marker <= 0xC7 || marker >= 0xC9 && marker <= 0xCB || marker >= 0xCD && marker <= 0xCF) && len(segment) >= 8 {
			depth = int(segment[0])
			height = int(binary.BigEndian.Uint16(segment[1:3]))
			width = int(binary.BigEndian.Uint16(segment[3:5]))
			components := int(segment[5])
			subsampling = "4:4:4"
			if components >= 3 && len(segment) >= 6+components*3 {
				sampling := segment[7]
				switch sampling {
				case 0x22:
					subsampling = "4:2:0"
				case 0x21:
					subsampling = "4:2:2"
				}
			}
			ok = width > 0 && height > 0
		}
		pos += length
	}
	if iccDescription == "" && bytes.Contains(data, []byte("sRGB IEC61966-2.1")) {
		iccSpace = "RGB"
		iccDescription = "sRGB IEC61966-2.1"
	}
	return width, height, depth, subsampling, iccSpace, iccDescription, metadataBytes, ok
}

// parsePNGAttachment reads IHDR dimensions and depth plus optional gAMA from a
// PNG payload.
func parsePNGAttachment(data []byte) (width, height, depth int, gamma string, ok bool) {
	if len(data) < 33 || !bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}) {
		return 0, 0, 0, "", false
	}
	for pos := 8; pos+12 <= len(data); {
		length := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		if length < 0 || pos+12+length > len(data) {
			break
		}
		kind := string(data[pos+4 : pos+8])
		payload := data[pos+8 : pos+8+length]
		switch kind {
		case "IHDR":
			if len(payload) >= 13 {
				width = int(binary.BigEndian.Uint32(payload[0:4]))
				height = int(binary.BigEndian.Uint32(payload[4:8]))
				depth = int(payload[8])
				ok = width > 0 && height > 0
			}
		case "gAMA":
			if len(payload) == 4 {
				value := float64(binary.BigEndian.Uint32(payload)) / 100000
				gamma = strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", value), "0"), ".")
			}
		}
		pos += 12 + length
	}
	return width, height, depth, gamma, ok
}
