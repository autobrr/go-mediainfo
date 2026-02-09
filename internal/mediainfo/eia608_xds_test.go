package mediainfo

import "testing"

func TestEIA608XDS_ProgramNameAndContentAdvisory(t *testing.T) {
	var x eia608XDS

	// Program name: class=0x01 (Current), type=0x03 (Program Name), payload bytes, end marker 0x0F.
	feed := func(a, b byte) {
		title, rating, ok := x.feed(a, b)
		if ok && rating != "" {
			t.Fatalf("unexpected rating during program name: %q", rating)
		}
		if ok && title != "" {
			if title != "ABCD" {
				t.Fatalf("unexpected title: %q", title)
			}
		}
	}
	feed(0x01, 0x03)
	feed('A', 'B')
	feed('C', 'D')
	title, rating, ok := x.feed(0x0F, 0x00)
	if !ok || rating != "" || title != "ABCD" {
		t.Fatalf("expected title=ABCD, got ok=%v title=%q rating=%q", ok, title, rating)
	}

	// Content advisory: class=0x01 (Current), type=0x05, TV-PG: a1a0=1, g2g1g0=4.
	title, rating, ok = x.feed(0x01, 0x05)
	if ok {
		t.Fatalf("expected incomplete packet")
	}
	_, _, _ = title, rating, ok

	// Use values >= 0x10 so they are treated as payload bytes (not XDS continue markers).
	_, _, _ = x.feed(0x48, 0x44) // a1a0=1, rating code=TV-PG
	title, rating, ok = x.feed(0x0F, 0x00)
	if !ok || title != "" || rating != "TV-PG" {
		t.Fatalf("expected rating=TV-PG, got ok=%v title=%q rating=%q", ok, title, rating)
	}
}
