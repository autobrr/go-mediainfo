package mediainfo

import "testing"

func TestNewPSStreamParserQuickAC3(t *testing.T) {
	if !newPSStreamParser(mpegPSOptions{parseSpeed: 0.5}).quickAC3 {
		t.Fatalf("expected quickAC3 at low parse speed")
	}
	if newPSStreamParser(mpegPSOptions{parseSpeed: 1}).quickAC3 {
		t.Fatalf("did not expect quickAC3 at full parse speed")
	}
	if newPSStreamParser(mpegPSOptions{parseSpeed: 0.5, dvdExtras: true}).quickAC3 {
		t.Fatalf("did not expect quickAC3 with dvd extras")
	}
}

func TestPSConsumePayloadQuickAC3SkipsDecode(t *testing.T) {
	parser := &psStreamParser{quickAC3: true, quickAC3Max: 4}
	entry := &psStream{
		kind:        StreamAudio,
		format:      "AC-3",
		hasAC3:      true,
		audioFrames: 7,
	}
	payload := []byte{0x0B, 0x77, 0x00, 0x00, 0x00, 0x00}
	parser.consumePayload(entry, 0, 0, 0, false, payload)

	if entry.bytes != uint64(len(payload)) {
		t.Fatalf("unexpected bytes: got %d want %d", entry.bytes, len(payload))
	}
	if entry.audioFrames != 7 {
		t.Fatalf("unexpected audio frame decode: got %d want 7", entry.audioFrames)
	}
}
