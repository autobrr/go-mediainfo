package mediainfo

import "testing"

func TestID3COMMNormalizesNewlinesLikeMediaInfo(t *testing.T) {
	// Encoding: UTF-8, language: "eng", desc: "", comment: lines with blank line.
	data := []byte{0x03, 'e', 'n', 'g', 0x00}
	data = append(data, []byte("a\r\n\r\nb")...)
	got, ok := parseID3COMM(data)
	if !ok {
		t.Fatalf("parseID3COMM ok=false")
	}
	want := "a /  / b"
	if got != want {
		t.Fatalf("parseID3COMM=%q want %q", got, want)
	}
}

func TestID3WXXXAllowsEmptyDescription(t *testing.T) {
	// Encoding: UTF-8, desc: "", url: "http://x".
	data := []byte{0x03, 0x00}
	data = append(data, []byte("http://x")...)
	desc, url, ok := parseID3WXXX(data)
	if !ok {
		t.Fatalf("parseID3WXXX ok=false")
	}
	if desc != "URL" {
		t.Fatalf("desc=%q want %q", desc, "URL")
	}
	if url != "http://x" {
		t.Fatalf("url=%q want %q", url, "http://x")
	}
}

