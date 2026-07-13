package mediainfo

import "testing"

func TestExtractLeadingNumber(t *testing.T) {
	cases := []struct {
		value string
		want  string
	}{
		{value: "1 920 pixels", want: "1920"},
		{value: "640", want: "640"},
		{value: "  29.970 FPS", want: "29.970"},
		{value: "", want: ""},
	}
	for _, tc := range cases {
		if got := extractLeadingNumber(tc.value); got != tc.want {
			t.Fatalf("extractLeadingNumber(%q)=%q want %q", tc.value, got, tc.want)
		}
	}
}

func TestMapStreamFieldsToJSONDisplayAspectRatioDecimal(t *testing.T) {
	fields := []Field{{Name: "Display aspect ratio", Value: "1.200"}}

	got := mapStreamFieldsToJSON(StreamVideo, fields)

	if value := jsonFieldValue(got, "DisplayAspectRatio"); value != "1.200" {
		t.Fatalf("DisplayAspectRatio=%q want %q", value, "1.200")
	}
}

func TestSortJSONAudioFieldsPlacesTitleBeforeLanguageAndExtra(t *testing.T) {
	fields := sortJSONFields(StreamAudio, []jsonKV{
		{Key: "extra", Val: `{}`},
		{Key: "Language", Val: "en"},
		{Key: "Title", Val: "E-AC-3 5.1"},
	})

	for i, key := range []string{"Title", "Language", "extra"} {
		if fields[i].Key != key {
			t.Fatalf("field %d=%q want %q", i, fields[i].Key, key)
		}
	}
}
