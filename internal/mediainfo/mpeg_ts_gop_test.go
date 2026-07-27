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

func TestBDAVSemanticProjectionPolicies(t *testing.T) {
	variable := mpeg2VideoInfo{
		GOPVariable: true, GOPMDominant: 3, GOPNDominant: 15,
		GOPOpenClosed: "Open", GOPFirstClosed: "Closed",
	}
	if !preferTSDominantGOP(variable, true) {
		t.Fatal("BDAV should prefer its dominant GOP over bounded-window variability")
	}
	if preferTSDominantGOP(variable, false) {
		t.Fatal("ordinary TS should retain variable GOP")
	}
	bdav := projectTSGOP(variable, true)
	if bdav.Setting != "M=3, N=15" || bdav.OpenClosed != "Open" || bdav.FirstClosed != "Closed" {
		t.Fatalf("BDAV GOP projection=%#v", bdav)
	}
	ordinary := projectTSGOP(variable, false)
	if ordinary.Setting != "Variable" || ordinary.OpenClosed != "" || ordinary.FirstClosed != "" {
		t.Fatalf("ordinary TS GOP projection=%#v", ordinary)
	}
	unstable := projectTSGOP(mpeg2VideoInfo{GOPVariable: true}, true)
	if unstable.Setting == "M=3, N=15" || unstable.OpenClosed != "" || unstable.FirstClosed != "" {
		t.Fatalf("unstable BDAV falsely projected fixed GOP: %#v", unstable)
	}

	avc := &tsStream{format: "AVC", h264SliceCount: 4}
	if got := bdavH264SliceCount(avc, true); got != 4 {
		t.Fatalf("BDAV AVC slice count = %d, want 4", got)
	}
	if got := bdavH264SliceCount(avc, false); got != 0 {
		t.Fatalf("ordinary TS AVC slice count = %d, want suppressed", got)
	}
	avc.h264SliceCount = 1
	if got := bdavH264SliceCount(avc, true); got != 0 {
		t.Fatalf("single-slice BDAV count = %d, want suppressed", got)
	}
}

func TestAlignBDAVHeadBufferEndCompletesStraddlingPacket(t *testing.T) {
	const packetSize = int64(192)
	got := alignBDAVHeadBufferEnd(64<<10, 0, packetSize)
	if got != 98_304 {
		t.Fatalf("aligned end = %d, want 98304", got)
	}
	if got%packetSize != 0 || got < 64<<10 {
		t.Fatalf("aligned end %d does not complete the boundary packet", got)
	}
	if ordinary := boundedTransportHeadScanEnd(64<<10, 0, 188, false); ordinary != 64<<10 {
		t.Fatalf("ordinary TS head end = %d, want unchanged 65536", ordinary)
	}
}

func TestProgressivePictureThresholdIsTSSpecific(t *testing.T) {
	base := mpeg2VideoParser{
		gotSeqExt:         true,
		pictureCount:      100,
		progressiveFrames: 95,
	}
	shared := base
	if got := shared.finalize(); got.ScanType != "Interlaced" {
		t.Fatalf("shared MPEG-2 finalize ScanType = %q, want sequence-derived Interlaced", got.ScanType)
	}
	ts := base
	if got := ts.finalizeTS(); got.ScanType != "Progressive" {
		t.Fatalf("TS MPEG-2 finalize ScanType = %q, want picture-threshold Progressive", got.ScanType)
	}
}
