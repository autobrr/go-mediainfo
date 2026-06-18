package mediainfo

import "testing"

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
				stream.Fields = append(stream.Fields, Field{Name: "Writing library", Value: tc.writingLib})
			}
			if tc.settings != "" {
				stream.Fields = append(stream.Fields, Field{Name: "Encoding settings", Value: tc.settings})
			}
			if got := matroskaVideoHasX264Settings(stream); got != tc.want {
				t.Fatalf("matroskaVideoHasX264Settings() = %v, want %v", got, tc.want)
			}
		})
	}
}
