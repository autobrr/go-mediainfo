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

func TestNormalizeMatroskaDisplayAspectRatioUsesPixelAspectRatio(t *testing.T) {
	fields := []jsonKV{
		{Key: "Width", Val: "720"},
		{Key: "Height", Val: "480"},
		{Key: "PixelAspectRatio", Val: "0.889"},
	}

	got := normalizeContainerComputedJSON(StreamVideo, fields, "Matroska")
	if value := jsonFieldValue(got, "DisplayAspectRatio"); value != "1.333" {
		t.Fatalf("DisplayAspectRatio=%q want %q", value, "1.333")
	}
}

func TestNormalizeMatroskaDisplayAspectRatioBranchMatrix(t *testing.T) {
	tests := []struct {
		name   string
		fields []jsonKV
		want   string
	}{
		{
			name: "active format preserves declared ratio",
			fields: []jsonKV{{Key: "Format", Val: "AVC"}, {Key: "Width", Val: "1920"}, {Key: "Height", Val: "1080"},
				{Key: "PixelAspectRatio", Val: "0.999"}, {Key: "DisplayAspectRatio", Val: "2.350"}, {Key: "ActiveFormatDescription", Val: "8"}},
			want: "2.350",
		},
		{
			name: "HEVC container ratio preserved",
			fields: []jsonKV{{Key: "Format", Val: "HEVC"}, {Key: "Width", Val: "1920"}, {Key: "Height", Val: "1080"},
				{Key: "PixelAspectRatio", Val: "1.000"}, {Key: "DisplayAspectRatio", Val: "2.000"}},
			want: "2.000",
		},
		{
			name: "PAL MPEG video special case",
			fields: []jsonKV{{Key: "Format", Val: "MPEG Video"}, {Key: "Width", Val: "720"}, {Key: "Height", Val: "576"},
				{Key: "PixelAspectRatio", Val: "1.067"}},
			want: "1.333",
		},
		{
			name: "near square AVC snaps to square",
			fields: []jsonKV{{Key: "Format", Val: "AVC"}, {Key: "Width", Val: "1920"}, {Key: "Height", Val: "1080"},
				{Key: "PixelAspectRatio", Val: "0.999"}},
			want: "1.778",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeContainerComputedJSON(StreamVideo, append([]jsonKV(nil), tt.fields...), "Matroska")
			if value := jsonFieldValue(got, "DisplayAspectRatio"); value != tt.want {
				t.Fatalf("DisplayAspectRatio=%q want %q", value, tt.want)
			}
		})
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

func TestChannelPositionsFromCountMapsThreeChannelFront(t *testing.T) {
	if got := channelPositionsFromCount("3"); got != "Front: L C R" {
		t.Fatalf("channelPositionsFromCount(3) = %q; want Front: L C R", got)
	}
}

func TestApplyJSONExtrasRawPrecedenceDoesNotDuplicateKeys(t *testing.T) {
	fields := []jsonKV{
		{Key: "Canonical", Val: "unchanged"},
		{Key: "Shared", Val: "canonical"},
	}
	got := applyJSONExtras(
		fields,
		map[string]string{"Added": "json", "Shared": "json"},
		map[string]string{"Added": `{"source":"raw"}`, "Shared": `"raw"`},
	)

	if len(got) != 3 {
		t.Fatalf("field count = %d, want 3: %#v", len(got), got)
	}
	for _, key := range []string{"Added", "Shared"} {
		count := 0
		for _, field := range got {
			if field.Key == key {
				count++
				if !field.Raw {
					t.Errorf("%s Raw = false, want JSONRaw precedence", key)
				}
			}
		}
		if count != 1 {
			t.Errorf("%s count = %d, want 1", key, count)
		}
	}
	if value := jsonFieldValue(got, "Added"); value != `{"source":"raw"}` {
		t.Errorf("Added = %q, want raw value", value)
	}
	if value := jsonFieldValue(got, "Shared"); value != `"raw"` {
		t.Errorf("Shared = %q, want raw value", value)
	}
}
