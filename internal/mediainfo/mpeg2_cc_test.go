package mediainfo

import "testing"

func TestParseDVDUserDataField1(t *testing.T) {
	data := []byte{
		'C', 'C', 0x01, 0xF8, 0x82,
		0xFF, 0x14, 0x2F,
		0xFE, 0x80, 0x80,
	}
	hasCC, ccType, hasCommand, hasDisplay := parseDVDUserData(data)
	if !hasCC {
		t.Fatalf("expected CC data")
	}
	if ccType != 0 {
		t.Fatalf("expected CC1 (field 1), got %d", ccType)
	}
	if !hasCommand {
		t.Fatalf("expected command detection")
	}
	if !hasDisplay {
		t.Fatalf("expected display detection")
	}
}

func TestParseDVDUserDataField2(t *testing.T) {
	data := []byte{
		'C', 'C', 0x01, 0xF8, 0x82,
		0xFF, 0x80, 0x80,
		0xFE, 0x14, 0x2F,
	}
	hasCC, ccType, hasCommand, hasDisplay := parseDVDUserData(data)
	if !hasCC {
		t.Fatalf("expected CC data")
	}
	if ccType != 1 {
		t.Fatalf("expected CC3 (field 2), got %d", ccType)
	}
	if !hasCommand {
		t.Fatalf("expected command detection")
	}
	if !hasDisplay {
		t.Fatalf("expected display detection")
	}
}

func TestDVDUserDataEmitsBothActiveCaptionFields(t *testing.T) {
	entry := &psStream{id: 0xE0, firstPacketOrder: -1}
	entry.ccOdd.firstFrame = -1
	entry.ccEven.firstFrame = -1
	payload := []byte{
		0x00, 0x00, 0x01, 0x00,
		0x00, 0x00, 0x01, 0xB2,
		'C', 'C', 0x01, 0xF8, 0x82,
		0xFF, 0x14, 0x2F,
		0xFE, 0x15, 0x2F,
		0x00, 0x00, 0x01, 0xB3,
	}
	consumeMPEG2Captions(entry, payload, 90_000, true)

	streams := buildCCTextStreams(entry, 0, 1, 29.97)
	if len(streams) != 2 {
		t.Fatalf("caption stream count = %d, want CC1 and CC3", len(streams))
	}
	for index, want := range []string{"224-CC1", "224-CC3"} {
		if id, _ := canonicalSeedValue(streams[index], "ID"); id != want {
			t.Fatalf("stream %d ID = %q, want %q", index, id, want)
		}
	}
}
