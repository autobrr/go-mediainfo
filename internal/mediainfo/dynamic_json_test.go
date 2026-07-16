package mediainfo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeMatroskaDynamicCanonicalExtrasEscapesOrdersAndJoins(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{
		{Key: "Existing", Value: structuredNode{Kind: structuredString, Text: "first"}},
		{Key: "Collision", Value: structuredNode{Kind: structuredString, Text: "base"}},
	}})
	stream := builder.Snapshot(canonicalStreamPolicy{})
	mergeMatroskaDynamicCanonicalExtras(&stream, []structuredMember{
		{Key: "Zeta", Value: structuredNode{Kind: structuredString, Text: "quote \" and\nnewline"}},
		{Key: "Collision", Value: structuredNode{Kind: structuredString, Text: "tag"}},
		{Key: "Alpha", Value: structuredNode{Kind: structuredString, Text: "value"}},
	})
	node := canonicalSeedStructuredNode(&stream, "extra")
	if node == nil {
		t.Fatal("canonical extra is missing")
	}
	got := structuredNodeText(*node)

	var decoded map[string]string
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("dynamic JSON is invalid: %v: %s", err, got)
	}
	if decoded["Existing"] != "first" || decoded["Zeta"] != "quote \" and\nnewline" || decoded["Collision"] != "base / tag" {
		t.Fatalf("dynamic JSON values = %#v", decoded)
	}
	for _, ordered := range []string{`"Existing"`, `"Collision"`, `"Zeta"`, `"Alpha"`} {
		index := strings.Index(got, ordered)
		if index < 0 {
			t.Fatalf("missing %s in %s", ordered, got)
		}
		got = got[index+len(ordered):]
	}
}

func TestMediaInfoJSONName(t *testing.T) {
	for _, test := range []struct {
		name string
		want string
	}{
		{name: "PARENT/CHILD", want: "PARENT_CHILD"},
		{name: "1 database.id", want: "_1_database_id"},
		{name: "hyphen-(value)", want: "hyphenvalue"},
		{name: "©", want: "Unknown"},
	} {
		if got := mediaInfoJSONName(test.name); got != test.want {
			t.Fatalf("mediaInfoJSONName(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestOrderMatroskaJSONExtraPreservesUnknownSlots(t *testing.T) {
	got := orderMatroskaJSONExtra(`{"CustomA":"a","acmod":"2","CustomB":"b","bsid":"8","dialnorm":"-27"}`)
	want := `{"CustomA":"a","bsid":"8","CustomB":"b","dialnorm":"-27","acmod":"2"}`
	if got != want {
		t.Fatalf("ordered extra = %s, want %s", got, want)
	}
}
