package mediainfo

import "testing"

func TestParsePATRejectsTooShortSectionLength(t *testing.T) {
	payload := make([]byte, 9)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("parsePAT panicked: %v", recovered)
		}
	}()

	programs, consumed := parsePAT(payload)
	if programs != nil {
		t.Fatalf("expected nil programs, got %v", programs)
	}
	if consumed != 0 {
		t.Fatalf("expected consumed=0, got %d", consumed)
	}
}

func TestParsePATTransportStreamID(t *testing.T) {
	payload := []byte{
		0x00,
		0x00, 0xB0, 0x0D, 0x0C, 0x0B, 0xC1, 0x00, 0x00,
		0x00, 0x03, 0xE0, 0x30,
		0x00, 0x00, 0x00, 0x00,
	}
	id, ok := parsePATTransportStreamID(payload)
	if !ok || id != 3083 {
		t.Fatalf("transport stream ID=%d, ok=%v; want 3083, true", id, ok)
	}
}

func TestParseSDTRejectsTooShortSectionLength(t *testing.T) {
	payload := make([]byte, 12)
	payload[1] = 0x42

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("parseSDT panicked: %v", recovered)
		}
	}()

	name, provider, serviceType := parseSDT(payload, 0)
	if name != "" || provider != "" || serviceType != "" {
		t.Fatalf("expected empty SDT result, got name=%q provider=%q serviceType=%q", name, provider, serviceType)
	}
}
