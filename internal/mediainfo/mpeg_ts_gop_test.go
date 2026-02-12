package mediainfo

import "testing"

func TestFormatMPEG2GOPSetting(t *testing.T) {
	tests := []struct {
		name string
		info mpeg2VideoInfo
		want string
	}{
		{
			name: "variable-overrides-interlaced-mn",
			info: mpeg2VideoInfo{
				GOPVariable: true,
				ScanType:    "Interlaced",
				GOPM:        3,
				GOPN:        15,
			},
			want: "Variable",
		},
		{
			name: "interlaced-mn",
			info: mpeg2VideoInfo{
				ScanType: "Interlaced",
				GOPM:     3,
				GOPN:     15,
			},
			want: "M=3, N=15",
		},
		{
			name: "gop-length",
			info: mpeg2VideoInfo{
				GOPLength: 13,
			},
			want: formatGOPLength(13),
		},
		{
			name: "empty",
			info: mpeg2VideoInfo{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMPEG2GOPSetting(tt.info)
			if got != tt.want {
				t.Fatalf("formatMPEG2GOPSetting()=%q want %q", got, tt.want)
			}
		})
	}
}
