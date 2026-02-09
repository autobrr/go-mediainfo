package mediainfo

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"strconv"
	"strings"
)

func ParseOgg(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, []Field, map[string]string, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, nil, nil, false
	}

	var (
		sampleRate  uint32
		channels    uint8
		lastGranule uint64
		format      string
		serial      uint32
		tagVendor   string
		tagEncoder  string
	)
	var segTable [255]byte
	bytesRead := int64(0)

	for pages := 0; pages < 64 && bytesRead < 1024*1024; pages++ {
		var header [27]byte
		if _, err := io.ReadFull(file, header[:]); err != nil {
			break
		}
		if header[0] != 'O' || header[1] != 'g' || header[2] != 'g' || header[3] != 'S' {
			return ContainerInfo{}, nil, nil, nil, false
		}
		bytesRead += int64(len(header))
		granule := binary.LittleEndian.Uint64(header[6:14])
		if serial == 0 {
			serial = binary.LittleEndian.Uint32(header[14:18])
		}
		segCount := int(header[26])
		if segCount > len(segTable) {
			return ContainerInfo{}, nil, nil, nil, false
		}
		if _, err := io.ReadFull(file, segTable[:segCount]); err != nil {
			return ContainerInfo{}, nil, nil, nil, false
		}
		bytesRead += int64(segCount)
		dataLen := 0
		for _, seg := range segTable[:segCount] {
			dataLen += int(seg)
		}
		if dataLen > 0 {
			peek := min(dataLen, 4096)
			data := make([]byte, peek)
			if _, err := io.ReadFull(file, data); err != nil {
				return ContainerInfo{}, nil, nil, nil, false
			}
			bytesRead += int64(peek)
			if sampleRate == 0 {
				if sr, ch, fmt := parseOggIdentification(data); sr > 0 {
					sampleRate = sr
					channels = ch
					format = fmt
				}
			}
			if tagVendor == "" && bytes.HasPrefix(data, []byte("OpusTags")) {
				v, e, ok := parseOpusTags(data)
				if ok {
					tagVendor = v
					tagEncoder = e
				}
			}
			if dataLen > peek {
				if _, err := file.Seek(int64(dataLen-peek), io.SeekCurrent); err != nil {
					return ContainerInfo{}, nil, nil, nil, false
				}
				bytesRead += int64(dataLen - peek)
			}
		}
		if granule != ^uint64(0) && granule > lastGranule {
			lastGranule = granule
		}
	}

	if sampleRate == 0 {
		return ContainerInfo{}, nil, nil, nil, false
	}

	// Prefer tail scan for last granule to avoid reading whole files.
	if size > 0 {
		if g, ok := findOggLastGranule(file, size); ok && g > 0 {
			lastGranule = g
		}
	}

	duration := 0.0
	if lastGranule > 0 {
		duration = float64(lastGranule) / float64(sampleRate)
	}

	info := ContainerInfo{
		DurationSeconds: duration,
		BitrateMode:     "",
	}

	if format == "" {
		format = "Unknown"
	}

	streamFields := []Field{
		{Name: "Format", Value: format},
	}
	if serial != 0 {
		streamFields = append(streamFields, Field{Name: "ID", Value: strconv.FormatUint(uint64(serial), 10)})
	}
	streamFields = appendChannelFields(streamFields, uint64(channels))
	streamFields = append(streamFields, Field{Name: "Compression mode", Value: "Lossy"})
	if sampleRate > 0 {
		streamFields = append(streamFields, Field{Name: "Sampling rate", Value: formatSampleRate(float64(sampleRate))})
	}
	streamFields = addStreamDuration(streamFields, duration)
	if tagVendor != "" {
		// Match official: audio Encoded_Library comes from OpusTags vendor (e.g. Lavf...).
		streamFields = append(streamFields, Field{Name: "Writing library", Value: strings.TrimSpace(tagVendor)})
	}

	streamJSON := map[string]string{}
	if sampleRate > 0 && duration > 0 {
		// Match official: SamplingCount is derived from integer milliseconds duration.
		durationMs := int64(math.Round(duration * 1000))
		if durationMs > 0 {
			streamJSON["SamplingCount"] = strconv.FormatInt(durationMs*int64(sampleRate)/1000, 10)
		}
	}

	generalFields := []Field{}
	if tagEncoder != "" {
		// Match official: General Encoded_Application comes from OpusTags ENCODER (Lavc... libopus).
		generalFields = append(generalFields, Field{Name: "Writing application", Value: strings.TrimSpace(tagEncoder)})
	}
	generalJSON := map[string]string{}

	streams := []Stream{{
		Kind:                StreamAudio,
		Fields:              streamFields,
		JSON:                streamJSON,
		JSONSkipStreamOrder: true,
	}}
	return info, streams, generalFields, generalJSON, true
}

func parseOggIdentification(data []byte) (uint32, uint8, string) {
	if len(data) < 16 {
		return 0, 0, ""
	}
	if data[0] == 0x01 && bytes.Equal(data[1:7], []byte("vorbis")) {
		channels := data[11]
		sampleRate := binary.LittleEndian.Uint32(data[12:16])
		return sampleRate, channels, "Vorbis"
	}
	if bytes.HasPrefix(data, []byte("OpusHead")) && len(data) >= 19 {
		channels := data[9]
		return 48000, channels, "Opus"
	}
	return 0, 0, ""
}

func parseOpusTags(data []byte) (vendor string, encoder string, ok bool) {
	if !bytes.HasPrefix(data, []byte("OpusTags")) || len(data) < 16 {
		return "", "", false
	}
	off := 8
	if off+4 > len(data) {
		return "", "", false
	}
	vlen := int(binary.LittleEndian.Uint32(data[off : off+4]))
	off += 4
	if vlen < 0 || off+vlen > len(data) {
		return "", "", false
	}
	vendor = string(data[off : off+vlen])
	off += vlen
	if off+4 > len(data) {
		return vendor, "", true
	}
	count := int(binary.LittleEndian.Uint32(data[off : off+4]))
	off += 4
	for i := 0; i < count && off+4 <= len(data); i++ {
		clen := int(binary.LittleEndian.Uint32(data[off : off+4]))
		off += 4
		if clen < 0 || off+clen > len(data) {
			break
		}
		comment := string(data[off : off+clen])
		off += clen
		if encoder == "" {
			upper := strings.ToUpper(comment)
			if strings.HasPrefix(upper, "ENCODER=") {
				encoder = comment[len("ENCODER="):]
			}
		}
	}
	return vendor, strings.TrimSpace(encoder), true
}

func findOggLastGranule(file io.ReadSeeker, size int64) (uint64, bool) {
	if size <= 0 {
		return 0, false
	}
	const maxWindow = 4 * 1024 * 1024
	window := int64(64 * 1024)
	if window > size {
		window = size
	}
	buf := make([]byte, window)
	for window <= maxWindow && window <= size {
		start := size - window
		if _, err := file.Seek(start, io.SeekStart); err != nil {
			return 0, false
		}
		if _, err := io.ReadFull(file, buf[:window]); err != nil {
			return 0, false
		}
		// Scan backwards for a full page contained in the buffer.
		for i := len(buf) - 4; i >= 0; i-- {
			if buf[i] != 'O' || i+27 > len(buf) {
				continue
			}
			if buf[i+1] != 'g' || buf[i+2] != 'g' || buf[i+3] != 'S' {
				continue
			}
			segCount := int(buf[i+26])
			if i+27+segCount > len(buf) {
				continue
			}
			dataLen := 0
			for _, seg := range buf[i+27 : i+27+segCount] {
				dataLen += int(seg)
			}
			pageLen := 27 + segCount + dataLen
			if i+pageLen > len(buf) {
				continue
			}
			granule := binary.LittleEndian.Uint64(buf[i+6 : i+14])
			if granule != ^uint64(0) {
				return granule, true
			}
		}
		if window == size {
			break
		}
		window *= 2
		if window > size {
			window = size
		}
		buf = make([]byte, window)
	}
	return 0, false
}
