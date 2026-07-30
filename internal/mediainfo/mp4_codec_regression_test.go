package mediainfo

import "testing"

func TestParseMP4VP9SampleEntryMapsVPCChromaSubsampling(t *testing.T) {
	tests := []struct {
		code byte
		want string
	}{
		{code: 0, want: "4:2:0"},
		{code: 1, want: "4:2:0"},
		{code: 2, want: "4:2:2"},
		{code: 3, want: "4:4:4"},
		{code: 4, want: ""},
	}
	for _, tt := range tests {
		t.Run(string(rune('0'+tt.code)), func(t *testing.T) {
			payload := make([]byte, 12)
			payload[4] = 1
			payload[6] = 8<<4 | tt.code<<1
			box := make([]byte, 8+len(payload))
			box[3] = byte(len(box))
			copy(box[4:8], "vpcC")
			copy(box[8:], payload)
			entry := append(make([]byte, mp4VisualSampleEntryHeaderSize), box...)

			facts := &mp4StructuredFacts{}
			var fields []Field
			parseMP4VP9SampleEntry(entry, facts, &fields)
			if got := facts.Get("ChromaSubsampling"); got != tt.want {
				t.Fatalf("structured chroma code %d = %q, want %q", tt.code, got, tt.want)
			}
			if got := findField(fields, "Chroma subsampling"); got != tt.want {
				t.Fatalf("text chroma code %d = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}
