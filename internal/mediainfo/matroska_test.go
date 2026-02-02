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
	trackEntry = buildMatroskaElement(mkvIDTrackEntry, trackEntry)
	return buildMatroskaElement(mkvIDTracks, trackEntry)
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
