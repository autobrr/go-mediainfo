package mediainfo

import "testing"

func TestFindH264WritingLibraryZencoder(t *testing.T) {
	data := []byte("\x00\x00\x01\x06Zencoder Video Encoding System\x00")
	if got := findH264WritingLibrary(data); got != "Zencoder Video Encoding System" {
		t.Fatalf("findH264WritingLibrary()=%q", got)
	}
}

func TestMatroskaVideoHasX264Settings(t *testing.T) {
	x264Settings := "cabac=1 / ref=4 / deblock=1:0:0 / analyse=0x3:0x113 / me=hex / subme=7 / bitrate=5000 / vbv_maxrate=6000 / vbv_bufsize=12000"
	tests := []struct {
		name       string
		writingLib string
		settings   string
		want       bool
	}{
		{
			name:     "settings only x264",
			settings: x264Settings,
			want:     true,
		},
		{
			name:       "x264 library",
			writingLib: "x264 164",
			settings:   "bitrate=5000",
			want:       true,
		},
		{
			name:       "libx264 library",
			writingLib: "libx264",
			settings:   "bitrate=5000",
			want:       true,
		},
		{
			name:       "x264 token boundary",
			writingLib: "x264 - core 164",
			settings:   "bitrate=5000",
			want:       true,
		},
		{
			name:       "x264 adjacent name",
			writingLib: "x264foo",
			settings:   "bitrate=5000",
			want:       false,
		},
		{
			name:       "libx264 adjacent name",
			writingLib: "libx264foo",
			settings:   "bitrate=5000",
			want:       false,
		},
		{
			name:       "x265 library",
			writingLib: "x265 3.5",
			settings:   x264Settings,
			want:       false,
		},
		{
			name:     "x265 settings only",
			settings: "wpp / me=0 / bitrate=5000",
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stream := Stream{Kind: StreamVideo}
			if tc.writingLib != "" {
				replaceCanonicalSeedLegacyFill(&stream, "Encoded_Library", tc.writingLib, "Writing library", tc.writingLib)
			}
			if tc.settings != "" {
				replaceCanonicalSeedLegacyFill(&stream, "Encoded_Library_Settings", tc.settings, "Encoding settings", tc.settings)
			}
			if got := matroskaVideoHasX264Settings(stream); got != tc.want {
				t.Fatalf("matroskaVideoHasX264Settings() = %v, want %v", got, tc.want)
			}
		})
	}
}
