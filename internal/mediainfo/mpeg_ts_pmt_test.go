package mediainfo

import "testing"

func TestParsePMTDVBSubtitleDescriptor(t *testing.T) {
	section := []byte{
		0x02,       // table_id (PMT)
		0xB0, 0x00, // section_length (patched below)
		0x00, 0x01, // program_number
		0xC1,       // version/current_next
		0x00,       // section_number
		0x00,       // last_section_number
		0xE1, 0x00, // PCR PID
		0xF0, 0x00, // program_info_length
		0x06,       // stream_type: PES private data
		0xE1, 0x01, // elementary_pid
		0xF0, 0x0A, // ES_info_length (one DVB subtitle descriptor)
		0x59, 0x08, // descriptor tag/length
		'e', 'n', 'g', // ISO 639 language
		0x10,       // subtitling type
		0x00, 0x01, // composition page id
		0x00, 0x01, // ancillary page id
		0x00, 0x00, 0x00, 0x00, // CRC (not validated by parser)
	}
	sectionLen := len(section) - 3
	section[1] = 0xB0 | byte((sectionLen>>8)&0x0F)
	section[2] = byte(sectionLen)

	payload := append([]byte{0x00}, section...)
	streams, _ := parsePMT(payload, 1)
	if len(streams) != 1 {
		t.Fatalf("streams len=%d, want 1", len(streams))
	}
	st := streams[0]
	if st.kind != StreamText {
		t.Fatalf("kind=%v, want %v", st.kind, StreamText)
	}
	if st.format != "DVB Subtitle" {
		t.Fatalf("format=%q, want DVB Subtitle", st.format)
	}
	if st.language != "eng" {
		t.Fatalf("language=%q, want eng", st.language)
	}
}

func TestParsePMTATSCProgramAndCaptionDescriptors(t *testing.T) {
	section := []byte{
		0x02, 0xB0, 0x6A, 0x00, 0x03, 0xC1, 0x00, 0x00, 0xE0, 0x31, 0xF0, 0x21,
		0x0C, 0x04, 0x80, 0xB4, 0x81, 0x68,
		0x0E, 0x03, 0xC0, 0x9E, 0x37,
		0x05, 0x04, 'G', 'A', '9', '4',
		0x10, 0x06, 0xC0, 0x9E, 0x37, 0xC0, 0x08, 0x00,
		0x05, 0x04, 'C', 'U', 'E', 'I', 0xAA, 0x00,
		0x02, 0xE0, 0x31, 0xF0, 0x19,
		0x02, 0x03, 0x22, 0x85, 0x5F, 0x52, 0x01, 0x04,
		0x0E, 0x03, 0xC0, 0x96, 0x1A, 0x06, 0x01, 0x02,
		0x86, 0x07, 0xE1, 'e', 'n', 'g', 0xC1, 0x3F, 0xFF,
		0x81, 0xE0, 0x34, 0xF0, 0x19,
		0x05, 0x04, 'A', 'C', '-', '3', 0x81, 0x03, 0x08, 0x3C, 0x05,
		0x0A, 0x04, 'e', 'n', 'g', 0x00, 0x52, 0x01, 0x8E,
		0x0E, 0x03, 0xC0, 0x04, 0xEB,
		0x57, 0x2C, 0xB0, 0x8F,
	}

	streams, metadata := parsePMT(append([]byte{0}, section...), 3)
	if !metadata.ATSC {
		t.Fatal("ATSC program registration was not detected")
	}
	if metadata.MaximumBitRate != 16_201_200 {
		t.Fatalf("program maximum bitrate=%d, want 16201200", metadata.MaximumBitRate)
	}
	if len(streams) != 2 {
		t.Fatalf("streams len=%d, want 2", len(streams))
	}
	if service, ok := streams[0].captionServices["1"]; !ok || service.Language != "en" {
		t.Fatalf("caption service=%+v, present=%v", service, ok)
	}
	if streams[1].maximumBitRate != 503_600 {
		t.Fatalf("audio maximum bitrate=%d, want 503600", streams[1].maximumBitRate)
	}
}
