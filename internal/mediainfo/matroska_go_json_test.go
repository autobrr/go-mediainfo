package mediainfo

import "testing"

func TestParseMatroskaTrackRetainsGoOnlyPCMJSON(t *testing.T) {
	audio := buildMatroskaElement(mkvIDTrackAudio,
		buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(2)),
	)
	entry := append(
		buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2)),
		buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))...,
	)
	entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("A_PCM/INT/LIT"))...)
	entry = append(entry, audio...)

	stream, ok := parseMatroskaTrackEntry(entry, 1, 3)
	if !ok {
		t.Fatal("expected parsed PCM stream")
	}
	for key, want := range map[string]string{
		"ChannelLayout":    "L R",
		"ChannelPositions": "Front: L R",
		"Compression_Mode": "Lossless",
	} {
		if got := stream.mkvGoJSON[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
		if got := stream.JSON[key]; got != "" {
			t.Errorf("%s leaked into parity JSON before final restoration: %q", key, got)
		}
	}
	if got := findField(stream.Fields, "Channel layout"); got != "" {
		t.Fatalf("Go-only channel layout leaked into text fields: %q", got)
	}
	if got := findField(stream.Fields, "Compression mode"); got != "" {
		t.Fatalf("Go-only compression mode leaked into text fields: %q", got)
	}
}

func TestRestoreMatroskaGoJSONFieldsIsOutputOnly(t *testing.T) {
	general := Stream{Kind: StreamGeneral, JSON: map[string]string{}}
	streams := []Stream{
		{
			Kind:      StreamVideo,
			JSON:      map[string]string{"BitRate_Maximum": "official"},
			mkvGoJSON: map[string]string{"BitRate_Maximum": "legacy", "BitRate_Mode": "VBR", "FrameRate_Num": "24000"},
		},
		{
			Kind:      StreamAudio,
			JSON:      map[string]string{},
			mkvGoJSON: map[string]string{"ChannelLayout": "L R", "ChannelPositions": "Front: L R"},
		},
		{
			Kind:    StreamMenu,
			JSONRaw: map[string]string{"extra": `{"chapter":"one"}`},
		},
		{
			Kind:    StreamMenu,
			JSONRaw: map[string]string{"extra": `{"edition":"two"}`},
		},
	}

	restoreMatroskaGoJSONFields(&general, streams, "1234")
	renderedGeneral := withMatroskaGoJSON(general)
	renderedVideo := withMatroskaGoJSON(streams[0])
	renderedAudio := withMatroskaGoJSON(streams[1])
	renderedMenu := withMatroskaGoJSON(streams[2])

	if got := renderedVideo.JSON["BitRate_Maximum"]; got != "official" {
		t.Fatalf("CLI-backed field overridden: %q", got)
	}
	if got := renderedVideo.JSON["BitRate_Mode"]; got != "VBR" {
		t.Fatalf("BitRate_Mode = %q, want VBR", got)
	}
	if got := renderedVideo.JSON["FrameRate_Num"]; got != "24000" {
		t.Fatalf("FrameRate_Num = %q, want 24000", got)
	}
	if got := renderedAudio.JSON["ChannelLayout"]; got != "L R" {
		t.Fatalf("ChannelLayout = %q, want L R", got)
	}
	if got := renderedGeneral.JSON["StreamSize"]; got != "1234" {
		t.Fatalf("General StreamSize = %q, want 1234", got)
	}
	if got := renderedGeneral.JSON["OverallBitRate_Mode"]; got != "VBR" {
		t.Fatalf("General OverallBitRate_Mode = %q, want VBR", got)
	}
	if got := renderedMenu.JSONRaw["extra"]; got != `{"chapter":"one","edition":"two"}` {
		t.Fatalf("merged Menu.extra = %s", got)
	}
	if streams[0].JSON["BitRate_Mode"] != "" || streams[1].JSON["ChannelLayout"] != "" || general.JSON["StreamSize"] != "" {
		t.Fatal("Go-only fields leaked into the shared report")
	}
	if got := streams[2].JSONRaw["extra"]; got != `{"chapter":"one"}` {
		t.Fatalf("Go-only Menu.extra mutated the shared report: %s", got)
	}
	if len(streams[0].Fields) != 0 || len(streams[1].Fields) != 0 {
		t.Fatal("restoration changed text fields")
	}
}

func TestMatroskaGoFormatLevelUsesFirstStereoProfile(t *testing.T) {
	if got := matroskaGoFormatLevel("Stereo High@L4.1 / High@L4.1"); got != "4.1" {
		t.Fatalf("level = %q, want 4.1", got)
	}
	if got := matroskaGoFormatLevel("High@L4.1"); got != "" {
		t.Fatalf("single profile produced redundant retained level %q", got)
	}
}
