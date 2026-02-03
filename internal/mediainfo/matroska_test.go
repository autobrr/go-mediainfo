package mediainfo

import "testing"

func TestParseMatroskaTracks(t *testing.T) {
	buf := buildMatroskaSample()
	info, ok := parseMatroska(buf)
	if !ok {
		t.Fatalf("expected matroska info")
	}
	if !info.Container.HasDuration() {
		t.Fatalf("expected duration")
	}
	if len(info.Tracks) != 1 {
		t.Fatalf("expected 1 track")
	}
	if info.Tracks[0].Kind != StreamVideo {
		t.Fatalf("expected video track")
	}
	if findField(info.Tracks[0].Fields, "Width") == "" {
		t.Fatalf("missing width")
	}
	if findField(info.Tracks[0].Fields, "Frame rate") == "" {
		t.Fatalf("missing frame rate")
	}
	if findField(info.Tracks[0].Fields, "Nominal bit rate") == "" {
		t.Fatalf("missing nominal bit rate")
	}
}

func buildMatroskaSample() []byte {
	segment := append(
		buildMatroskaInfo(),
		buildMatroskaTracks()...,
	)
	segmentElem := append(buildMatroskaID(mkvIDSegment), buildMatroskaSize(uint64(len(segment)))...)
	segmentElem = append(segmentElem, segment...)
	return segmentElem
}

func buildMatroskaInfo() []byte {
	info := []byte{}
	info = append(info, buildMatroskaElement(mkvIDTimecodeScale, []byte{0x0F, 0x42, 0x40})...)
	info = append(info, buildMatroskaElement(mkvIDDuration, []byte{0x41, 0x20, 0x00, 0x00})...)
	return buildMatroskaElement(mkvIDInfo, info)
}

func buildMatroskaTracks() []byte {
	trackEntry := buildMatroskaElement(mkvIDTrackType, []byte{0x01})
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEG4/ISO/AVC"))...)
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(41708333))...)
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDBitRate, encodeMatroskaUint(1000000))...)
	trackEntry = append(trackEntry, buildMatroskaVideoSettings(1920, 1080)...)
	trackEntry = buildMatroskaElement(mkvIDTrackEntry, trackEntry)
	return buildMatroskaElement(mkvIDTracks, trackEntry)
}

func buildMatroskaVideoSettings(width, height uint64) []byte {
	video := []byte{}
	video = append(video, buildMatroskaElement(mkvIDPixelWidth, encodeMatroskaUint(width))...)
	video = append(video, buildMatroskaElement(mkvIDPixelHeight, encodeMatroskaUint(height))...)
	return buildMatroskaElement(mkvIDTrackVideo, video)
}

func encodeMatroskaUint(value uint64) []byte {
	if value <= 0xFF {
		return []byte{byte(value)}
	}
	if value <= 0xFFFF {
		return []byte{byte(value >> 8), byte(value)}
	}
	if value <= 0xFFFFFF {
		return []byte{byte(value >> 16), byte(value >> 8), byte(value)}
	}
	return []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
}

func buildMatroskaElement(id uint64, payload []byte) []byte {
	buf := append(buildMatroskaID(id), buildMatroskaSize(uint64(len(payload)))...)
	buf = append(buf, payload...)
	return buf
}

func buildMatroskaID(id uint64) []byte {
	if id <= 0xFF {
		return []byte{byte(id)}
	}
	if id <= 0xFFFF {
		return []byte{byte(id >> 8), byte(id)}
	}
	if id <= 0xFFFFFF {
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	}
	return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
}

func buildMatroskaSize(size uint64) []byte {
	if size < 0x7F {
		return []byte{byte(0x80 | size)}
	}
	if size < 0x3FFF {
		return []byte{byte(0x40 | (size >> 8)), byte(size)}
	}
	return []byte{byte(0x20 | (size >> 16)), byte(size >> 8), byte(size)}
}
