package mediainfo

import (
	"encoding/binary"
	"io"
)

const maxMoovSize = int64(16 << 20)

func ParseMP4(r io.ReaderAt, size int64) (ContainerInfo, bool) {
	var offset int64
	for offset+8 <= size {
		boxSize, boxType, headerSize, ok := readMP4BoxHeader(r, offset, size)
		if !ok || boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "moov" {
			moovSize := boxSize - headerSize
			if moovSize > maxMoovSize {
				return ContainerInfo{}, false
			}
			buf := make([]byte, moovSize)
			if _, err := r.ReadAt(buf, dataOffset); err != nil && err != io.EOF {
				return ContainerInfo{}, false
			}
			if info, ok := parseMoov(buf); ok {
				return info, true
			}
		}
		offset += boxSize
	}
	return ContainerInfo{}, false
}

func readMP4BoxHeader(r io.ReaderAt, offset, fileSize int64) (boxSize int64, boxType string, headerSize int64, ok bool) {
	header := make([]byte, 8)
	if _, err := r.ReadAt(header, offset); err != nil {
		return 0, "", 0, false
	}

	size32 := binary.BigEndian.Uint32(header[0:4])
	boxType = string(header[4:8])
	if size32 == 0 {
		return fileSize - offset, boxType, 8, true
	}
	if size32 == 1 {
		larger := make([]byte, 8)
		if _, err := r.ReadAt(larger, offset+8); err != nil {
			return 0, "", 0, false
		}
		size64 := binary.BigEndian.Uint64(larger)
		if size64 < 16 {
			return 0, "", 0, false
		}
		return int64(size64), boxType, 16, true
	}
	if size32 < 8 {
		return 0, "", 0, false
	}
	return int64(size32), boxType, 8, true
}

func parseMoov(buf []byte) (ContainerInfo, bool) {
	var offset int64
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "mvhd" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if duration, ok := parseMvhd(payload); ok {
				return ContainerInfo{DurationSeconds: duration}, true
			}
		}
		offset += boxSize
	}
	return ContainerInfo{}, false
}

func readMP4BoxHeaderFrom(buf []byte, offset int64) (boxSize int64, boxType string, headerSize int64) {
	if offset+8 > int64(len(buf)) {
		return 0, "", 0
	}
	size32 := binary.BigEndian.Uint32(buf[offset : offset+4])
	boxType = string(buf[offset+4 : offset+8])
	if size32 == 0 {
		return int64(len(buf)) - offset, boxType, 8
	}
	if size32 == 1 {
		if offset+16 > int64(len(buf)) {
			return 0, "", 0
		}
		size64 := binary.BigEndian.Uint64(buf[offset+8 : offset+16])
		return int64(size64), boxType, 16
	}
	return int64(size32), boxType, 8
}

func sliceBox(buf []byte, offset, length int64) []byte {
	if offset < 0 || length < 0 {
		return nil
	}
	end := offset + length
	if end > int64(len(buf)) {
		end = int64(len(buf))
	}
	if offset > end {
		return nil
	}
	return buf[offset:end]
}

func parseMvhd(payload []byte) (float64, bool) {
	if len(payload) < 20 {
		return 0, false
	}
	version := payload[0]
	if version == 0 {
		if len(payload) < 20 {
			return 0, false
		}
		timescale := binary.BigEndian.Uint32(payload[12:16])
		duration := binary.BigEndian.Uint32(payload[16:20])
		if timescale == 0 {
			return 0, false
		}
		return float64(duration) / float64(timescale), true
	}
	if version == 1 {
		if len(payload) < 32 {
			return 0, false
		}
		timescale := binary.BigEndian.Uint32(payload[20:24])
		duration := binary.BigEndian.Uint64(payload[24:32])
		if timescale == 0 {
			return 0, false
		}
		return float64(duration) / float64(timescale), true
	}
	return 0, false
}
