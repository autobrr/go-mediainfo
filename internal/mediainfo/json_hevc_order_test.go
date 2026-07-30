package mediainfo

import "testing"

// indexOfKey returns the position of key in the rendered jsonKV slice, or -1.
func indexOfKey(kvs []jsonKV, key string) int {
	for i, kv := range kvs {
		if kv.Key == key {
			return i
		}
	}
	return -1
}

// TestHEVCVideoJSONFieldOrder pins the JSON key order for the fields the HEVC MP4
// path newly emits, against the canonical MediaInfo Video order:
//
//	Format_Profile, Format_Level, Format_Tier
//	... Height, Stored_Width, Stored_Height ...
//	... ChromaSubsampling, ChromaSubsampling_Position, BitDepth ...
//
// and the invariant that "extra" is always the final key. Reference order verified
// against MediaInfoLib (mediainfo --Output=JSON --Language=raw) on an HEVC stream.
func TestHEVCVideoJSONFieldOrder(t *testing.T) {
	s := Stream{
		Kind: StreamVideo,
		Fields: []Field{
			{Name: "Format", Value: "HEVC"},
			{Name: "Format profile", Value: "Main@L4"},
			{Name: "Format tier", Value: "Main"},
			{Name: "Width", Value: "1 920 pixels"},
			{Name: "Height", Value: "1 080 pixels"},
			{Name: "Color space", Value: "YUV"},
			{Name: "Chroma subsampling", Value: "4:2:0"},
			{Name: "Chroma subsampling position", Value: "Type 2"},
			{Name: "Bit depth", Value: "8 bits"},
			{Name: "Scan type", Value: "Progressive"},
			// Produces a nested "extra" object (extra.CodecConfigurationBox), so the
			// "nothing sorts after extra" assertions below are live, not dead.
			{Name: "Codec configuration box", Value: "hvcC"},
		},
		JSON: map[string]string{
			"Stored_Width":  "1920",
			"Stored_Height": "1088",
			"colour_range":  "Limited",
		},
	}
	kvs := buildJSONStreamFields(s, 0, 0, "MPEG-4")

	idx := func(k string) int { return indexOfKey(kvs, k) }
	mustBefore := func(a, b string) {
		ia, ib := idx(a), idx(b)
		if ia < 0 {
			t.Fatalf("%s missing from rendered keys", a)
		}
		if ib < 0 {
			t.Fatalf("%s missing from rendered keys", b)
		}
		if ia >= ib {
			t.Errorf("%s (idx %d) must come before %s (idx %d)", a, ia, b, ib)
		}
	}

	// Format_Tier sits between Format_Level and the codec/dimension block.
	mustBefore("Format_Profile", "Format_Level")
	mustBefore("Format_Level", "Format_Tier")
	mustBefore("Format_Tier", "Width")
	// Stored_Width before Stored_Height, both after Height.
	mustBefore("Height", "Stored_Width")
	mustBefore("Stored_Width", "Stored_Height")
	// ChromaSubsampling_Position directly follows ChromaSubsampling, before BitDepth.
	mustBefore("ChromaSubsampling", "ChromaSubsampling_Position")
	mustBefore("ChromaSubsampling_Position", "BitDepth")
	// "extra" must be the final key; nothing may sort after it. Guard that the input
	// actually produces an "extra" object so these checks never silently go dead.
	if idx("extra") < 0 {
		t.Fatalf("test input did not produce an 'extra' key; cannot verify the post-extra invariant")
	}
	if last := len(kvs) - 1; idx("extra") != last {
		t.Errorf("extra must be last key (at %d), found at %d; keys after it: %v",
			last, idx("extra"), keysAfter(kvs, idx("extra")))
	}
	for _, k := range []string{"Format_Tier", "ChromaSubsampling_Position", "Stored_Width"} {
		if idx(k) > idx("extra") {
			t.Errorf("%s sorts after extra (idx %d > %d) — breaks byte-identical parity", k, idx(k), idx("extra"))
		}
	}
}

// TestHEVCTextFieldOrder pins the text/CSV field order for the HEVC-specific fields:
// "Format tier" follows "Format profile"; "Chroma subsampling position" follows
// "Chroma subsampling" (rather than sorting to the trailing unmapped block).
func TestHEVCTextFieldOrder(t *testing.T) {
	// Input in the order the parser appends them (parseHEVCConfig +
	// parseMP4HEVCSampleEntry). Without the textStreamFieldOrder entries these unmapped
	// names would sort to the trailing block after "Scan type".
	fields := []Field{
		{Name: "Format profile", Value: "Main@L4"},
		{Name: "Format tier", Value: "Main"},
		{Name: "Chroma subsampling", Value: "4:2:0"},
		{Name: "Chroma subsampling position", Value: "Type 2"},
		{Name: "Bit depth", Value: "8 bits"},
		{Name: "Scan type", Value: "Progressive"},
	}
	sortFields(StreamVideo, fields)
	pos := map[string]int{}
	for i, f := range fields {
		pos[f.Name] = i
	}
	// Format tier sits in the format-profile region: after Format profile but before
	// the chroma/bit-depth block (not dumped into the trailing unmapped block).
	if !(pos["Format profile"] < pos["Format tier"] && pos["Format tier"] < pos["Chroma subsampling"]) {
		t.Errorf("Format tier (%d) must be between Format profile (%d) and Chroma subsampling (%d)",
			pos["Format tier"], pos["Format profile"], pos["Chroma subsampling"])
	}
	// Chroma subsampling position directly follows Chroma subsampling and precedes Bit depth.
	if !(pos["Chroma subsampling"] < pos["Chroma subsampling position"] && pos["Chroma subsampling position"] < pos["Bit depth"]) {
		t.Errorf("Chroma subsampling position (%d) must be between Chroma subsampling (%d) and Bit depth (%d)",
			pos["Chroma subsampling position"], pos["Chroma subsampling"], pos["Bit depth"])
	}
}

func keysAfter(kvs []jsonKV, i int) []string {
	var out []string
	for j := i + 1; j < len(kvs); j++ {
		out = append(out, kvs[j].Key)
	}
	return out
}
