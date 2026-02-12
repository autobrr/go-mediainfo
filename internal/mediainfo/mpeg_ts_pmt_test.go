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
	streams, _, _, _ := parsePMT(payload, 1)
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
