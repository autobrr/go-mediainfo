package mediainfo

import "testing"

func writeBits(dst []byte, bitPos *int, value uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		bit := (value >> uint(i)) & 1
		bytePos := *bitPos >> 3
		shift := 7 - (*bitPos & 7)
		if bit == 1 {
			dst[bytePos] |= 1 << uint(shift)
		}
		*bitPos++
	}
}

func buildDTSCoreFrame(amode uint32, lfe uint32, brCode uint32) []byte {
	// Minimal DTS core frame matching parseDTSCoreFrame bit layout.
	out := make([]byte, 24)
	out[0] = 0x7F
	out[1] = 0xFE
	out[2] = 0x80
	out[3] = 0x01
	pos := 32
	writeBits(out, &pos, 0, 1)      // FrameType
	writeBits(out, &pos, 0, 5)      // Deficit sample count
	writeBits(out, &pos, 0, 1)      // CRC present
	writeBits(out, &pos, 15, 7)     // nblks (16 blocks -> 512 SPF)
	writeBits(out, &pos, 95, 14)    // primary frame bytes - 1
	writeBits(out, &pos, amode, 6)  // channel arrangement
	writeBits(out, &pos, 13, 4)     // sfCode 48 kHz
	writeBits(out, &pos, brCode, 5) // bit rate code
	writeBits(out, &pos, 0, 1)      // downmix
	writeBits(out, &pos, 0, 1)      // dynrng
	writeBits(out, &pos, 0, 1)      // time stamp
	writeBits(out, &pos, 0, 1)      // aux data
	writeBits(out, &pos, 0, 1)      // HDCD
	writeBits(out, &pos, 0, 3)      // ext audio descriptor
	writeBits(out, &pos, 0, 1)      // ext coding
	writeBits(out, &pos, 0, 1)      // sync insertion
	writeBits(out, &pos, lfe, 2)    // LFE
	writeBits(out, &pos, 0, 1)      // predictor history
	writeBits(out, &pos, 0, 1)      // multirate interpolator
	writeBits(out, &pos, 0, 4)      // encoder software rev
	writeBits(out, &pos, 0, 2)      // copy history
	writeBits(out, &pos, 6, 3)      // source PCM resolution code (24-bit)
	return out
}

func TestInferBDAVStreamDTS(t *testing.T) {
	payload := buildDTSCoreFrame(7, 1, 15)
	kind, format, stype, ok := inferBDAVStream(0x1101, payload)
	if !ok {
		t.Fatalf("inferBDAVStream: ok=false")
	}
	if kind != StreamAudio || format != "DTS" || stype != 0x82 {
		t.Fatalf("inferBDAVStream: got kind=%v format=%q stype=0x%02X", kind, format, stype)
	}
}

func TestParseDTSCoreFrameUsesActualPaddedFrameRate(t *testing.T) {
	frame := make([]byte, 424)
	copy(frame, []byte{0x7F, 0xFE, 0x80, 0x01})
	pos := 32
	writeBits(frame, &pos, 0, 1)    // FrameType
	writeBits(frame, &pos, 0, 5)    // Deficit sample count
	writeBits(frame, &pos, 0, 1)    // CRC present
	writeBits(frame, &pos, 15, 7)   // 512 samples per frame
	writeBits(frame, &pos, 423, 14) // 424-byte primary frame
	writeBits(frame, &pos, 2, 6)    // stereo channel arrangement
	writeBits(frame, &pos, 13, 4)   // 48 kHz
	writeBits(frame, &pos, 9, 5)    // nominal 320 kb/s transmission code
	writeBits(frame, &pos, 0, 13)   // optional core flags through predictor history
	writeBits(frame, &pos, 0, 1)    // multirate interpolator
	writeBits(frame, &pos, 0, 4)    // encoder software revision
	writeBits(frame, &pos, 0, 2)    // copy history
	writeBits(frame, &pos, 6, 3)    // source PCM resolution code (24-bit)

	info, ok := parseDTSCoreFrame(frame)
	if !ok {
		t.Fatal("parseDTSCoreFrame returned ok=false")
	}
	if info.bitRateBps != 318000 {
		t.Fatalf("bitRateBps = %d, want 318000", info.bitRateBps)
	}
}

func TestConsumeDTSCoreAndHDExtension(t *testing.T) {
	entry := tsStream{format: "DTS"}
	core := buildDTSCoreFrame(7, 1, 15)
	consumeDTS(&entry, core)
	if !entry.hasAudioInfo {
		t.Fatalf("expected DTS core to set audio info")
	}
	if entry.dtsHD {
		t.Fatalf("expected core-only payload to keep dtsHD=false")
	}
	// MediaInfoLib channel mapping (DTS_Channels) yields AMODE=7 => 4ch plus LFE => 5ch.
	if entry.audioRate != 48000 || entry.audioSpf != 512 || entry.audioChannels != 5 {
		t.Fatalf("unexpected core parse: rate=%v spf=%d channels=%d", entry.audioRate, entry.audioSpf, entry.audioChannels)
	}
	if entry.audioBitRateMode != "Constant" || entry.audioBitRateKbps != 768 {
		t.Fatalf("unexpected core bitrate mode: mode=%q bitrate=%d", entry.audioBitRateMode, entry.audioBitRateKbps)
	}

	consumeDTS(&entry, []byte{
		0x00, 0x64, 0x58, 0x20, 0x25,
		0x00, 0x41, 0xA2, 0x95, 0x47,
		0x00, 0x02, 0x00, 0x08, 0x50,
		0x00, 0xF1, 0x40, 0x00, 0xD7,
	})
	if !entry.dtsHD {
		t.Fatalf("expected DTS-HD extension sync to set dtsHD=true")
	}
	if !entry.dtsHDXLL {
		t.Fatalf("expected DTS-HD XLL sync to set dtsHDXLL=true")
	}
	if !entry.dtsHDX || !entry.dtsHDIMAX {
		t.Fatalf("expected immersive flags, got dtsHDX=%v dtsHDIMAX=%v", entry.dtsHDX, entry.dtsHDIMAX)
	}
	if entry.audioBitRateMode != "Variable" || entry.audioBitRateKbps != 0 {
		t.Fatalf("expected DTS-HD mode switch, got mode=%q bitrate=%d", entry.audioBitRateMode, entry.audioBitRateKbps)
	}
}

func TestDTSHDFormatLabelsIMAX(t *testing.T) {
	entry := &tsStream{dtsHD: true, dtsHDXLL: true, dtsHDX: true, dtsHDIMAX: true}
	format, commercial, features := dtsHDFormatLabels(entry)
	if format != "DTS XLL X IMAX" || commercial != "DTS-HD MA + IMAX Enhanced" || features != "XLL X IMAX" {
		t.Fatalf("labels = %q, %q, %q", format, commercial, features)
	}
}

func TestUpdateDTSHDExtensionFlagsRejectsDistantImmersiveMarkers(t *testing.T) {
	payload := append([]byte{0x41, 0xA2, 0x95, 0x47}, make([]byte, 512)...)
	payload = append(payload, 0x02, 0x00, 0x08, 0x50, 0xF1, 0x40, 0x00, 0xD7)
	entry := &tsStream{}
	updateDTSHDExtensionFlags(entry, payload)
	if entry.dtsHDX || entry.dtsHDIMAX {
		t.Fatalf("distant markers accepted: dtsHDX=%v dtsHDIMAX=%v", entry.dtsHDX, entry.dtsHDIMAX)
	}
}

func TestUpdateDTSHDExtensionFlagsRecognizesSplitMarkers(t *testing.T) {
	entry := &tsStream{}
	updateDTSHDExtensionFlags(entry, []byte{0x41, 0xA2, 0x95})
	updateDTSHDExtensionFlags(entry, []byte{0x47, 0x00, 0x02, 0x00})
	updateDTSHDExtensionFlags(entry, []byte{0x08, 0x50, 0x00, 0xF1, 0x40})
	updateDTSHDExtensionFlags(entry, []byte{0x00, 0xD7})
	if !entry.dtsHDXLL || !entry.dtsHDX || !entry.dtsHDIMAX {
		t.Fatalf("split markers not recognized: XLL=%v X=%v IMAX=%v", entry.dtsHDXLL, entry.dtsHDX, entry.dtsHDIMAX)
	}
}

func TestDTSHDImmersiveMarkers(t *testing.T) {
	payload := []byte{
		0x00, 0x64, 0x58, 0x20, 0x25,
		0x00, 0x41, 0xA2, 0x95, 0x47,
		0x00, 0x02, 0x00, 0x08, 0x50,
		0x00, 0xF1, 0x40, 0x00, 0xD7,
	}
	if !hasDTSHDExtension(payload) {
		t.Fatal("expected ExSS sync")
	}
	if !hasDTSHDXLLSync(payload) {
		t.Fatal("expected XLL sync")
	}
	if !hasDTSHDXSync(payload) {
		t.Fatal("expected DTS:X sync")
	}
	if !hasDTSHDIMAXSync(payload) {
		t.Fatal("expected IMAX sync")
	}
}

func TestApplyMatroskaAudioProbes_DTSHDXLLIMAX(t *testing.T) {
	info := &MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "ID", Value: "2"},
			{Name: "Format", Value: "DTS"},
			{Name: "Format/Info", Value: "Digital Theater Systems"},
			{Name: "Bit rate", Value: "6 242 kb/s"},
			{Name: "Channel(s)", Value: "8 channels"},
			{Name: "Default", Value: "Yes"},
		},
		JSON:          map[string]string{"BitRate": "6242280"},
		canonicalSeed: matroskaDTSCanonicalSeed(matroskaDTSCanonicalFacts{trackNumber: 2, audioChannels: 8, bitRate: 6242280}),
	}}}
	probes := map[uint64]*matroskaAudioProbe{
		2: {
			format: "DTS",
			dts: dtsInfo{
				sampleRate:      48000,
				samplesPerFrame: 512,
				channels:        8,
				hd:              true,
				hdXLL:           true,
				hdDTSX:          true,
				hdIMAX:          true,
				hdBitDepth:      24,
				hdChannels:      8,
				hdSpeakerMask:   0x084B,
				hasSpeakerMask:  true,
			},
			ok: true,
		},
	}

	applyMatroskaAudioProbes(info, probes)
	refreshCanonicalLegacySnapshot(&info.Tracks[0])
	stream := info.Tracks[0]
	if got := findField(stream.Fields, "Format"); got != "DTS XLL X IMAX" {
		t.Fatalf("Format=%q want DTS XLL X IMAX", got)
	}
	if got := findField(stream.Fields, "Commercial name"); got != "DTS-HD MA + IMAX Enhanced" {
		t.Fatalf("Commercial name=%q", got)
	}
	if got := findField(stream.Fields, "Bit rate"); got != "6 242 kb/s" {
		t.Fatalf("Bit rate=%q", got)
	}
	if got := findField(stream.Fields, "Channel layout"); got != "C L R LFE Lb Rb Lss Rss Objects" {
		t.Fatalf("Channel layout=%q", got)
	}
	if got := stream.JSON["Format"]; got != "DTS" {
		t.Fatalf("JSON Format=%q", got)
	}
	if got := stream.JSON["Format_AdditionalFeatures"]; got != "XLL X IMAX" {
		t.Fatalf("Format_AdditionalFeatures=%q", got)
	}
	if got := stream.JSON["Format_Commercial_IfAny"]; got != "DTS-HD MA + IMAX Enhanced" {
		t.Fatalf("Format_Commercial_IfAny=%q", got)
	}
	if got := stream.JSON["BitRate"]; got != "6242280" {
		t.Fatalf("JSON BitRate=%q", got)
	}
	if got := stream.JSON["ChannelLayout"]; got != "C L R LFE Lb Rb Lss Rss Objects" {
		t.Fatalf("JSON ChannelLayout=%q", got)
	}
	if got := stream.JSON["ChannelPositions"]; got != "Front: L C R, Side: L R, Back: L R, LFE, Objects" {
		t.Fatalf("JSON ChannelPositions=%q", got)
	}
}

func TestApplyMatroskaAudioProbes_DTSHDXLLPlainKeepsExistingBehavior(t *testing.T) {
	info := &MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "ID", Value: "2"},
			{Name: "Format", Value: "DTS"},
			{Name: "Format/Info", Value: "Digital Theater Systems"},
			{Name: "Bit rate", Value: "3 000 kb/s"},
			{Name: "Channel(s)", Value: "6 channels"},
		},
		JSON:          map[string]string{"BitRate": "3000000"},
		canonicalSeed: matroskaDTSCanonicalSeed(matroskaDTSCanonicalFacts{trackNumber: 2, audioChannels: 6, bitRate: 3000000}),
	}}}
	probes := map[uint64]*matroskaAudioProbe{
		2: {
			format: "DTS",
			dts: dtsInfo{
				sampleRate:      48000,
				samplesPerFrame: 512,
				channels:        6,
				hd:              true,
				hdXLL:           true,
				hdBitDepth:      24,
			},
			ok: true,
		},
	}

	applyMatroskaAudioProbes(info, probes)
	refreshCanonicalLegacySnapshot(&info.Tracks[0])
	stream := info.Tracks[0]
	if got := findField(stream.Fields, "Format"); got != "DTS XLL" {
		t.Fatalf("Format=%q want DTS XLL", got)
	}
	if got := findField(stream.Fields, "Commercial name"); got != "DTS-HD Master Audio" {
		t.Fatalf("Commercial name=%q", got)
	}
	if got := findField(stream.Fields, "Bit rate"); got != "" {
		t.Fatalf("Bit rate should be removed for plain DTS-HD, got %q", got)
	}
	if got := stream.JSON["Format_AdditionalFeatures"]; got != "XLL" {
		t.Fatalf("Format_AdditionalFeatures=%q", got)
	}
	if got := stream.JSON["BitRate"]; got != "" {
		t.Fatalf("JSON BitRate should be removed for plain DTS-HD, got %q", got)
	}
}

func TestNormalizeDTSHDChannelLayout(t *testing.T) {
	tests := []struct {
		name   string
		layout string
		want   string
	}{
		{name: "rear and side pairs", layout: "C L R LFE Lsr Rsr Lss Rss", want: "C L R LFE Lb Rb Lss Rss"},
		{name: "rear pair only", layout: "C L R LFE Lsr Rsr", want: "C L R LFE Lsr Rsr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeDTSHDChannelLayout(tt.layout); got != tt.want {
				t.Fatalf("normalizeDTSHDChannelLayout(%q)=%q want %q", tt.layout, got, tt.want)
			}
		})
	}
}
