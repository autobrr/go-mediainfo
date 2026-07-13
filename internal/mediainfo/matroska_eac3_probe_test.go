package mediainfo

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func putBits(dst []byte, bitPos *int, value uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		bit := (value >> uint(i)) & 1
		byteIdx := *bitPos >> 3
		shift := 7 - (*bitPos & 7)
		if bit == 1 {
			dst[byteIdx] |= 1 << uint(shift)
		}
		*bitPos++
	}
}

func buildEAC3Frame(frameSize int, dialnorm uint64, comprByte uint64) []byte {
	if frameSize%2 != 0 || frameSize < 8 {
		panic("invalid frameSize")
	}
	frmsiz := uint64(frameSize/2 - 1)
	out := make([]byte, frameSize)
	pos := 0
	putBits(out, &pos, 0x0B77, 16)   // syncword
	putBits(out, &pos, 0, 2)         // strmtyp (independent)
	putBits(out, &pos, 0, 3)         // substreamid
	putBits(out, &pos, frmsiz, 11)   // frmsiz
	putBits(out, &pos, 0, 2)         // fscod (48kHz)
	putBits(out, &pos, 3, 2)         // numblkscod (6 blocks => 1536 samples)
	putBits(out, &pos, 2, 3)         // acmod
	putBits(out, &pos, 0, 1)         // lfeon
	putBits(out, &pos, 16, 5)        // bsid (>=10)
	putBits(out, &pos, dialnorm, 5)  // dialnorm
	putBits(out, &pos, 1, 1)         // compre
	putBits(out, &pos, comprByte, 8) // compr (0xFF would mean "unset")
	return out
}

func TestProbeMatroskaAudio_EAC3MultiFramePacket(t *testing.T) {
	const track = 1
	frame := buildEAC3Frame(20, 1, 0x00)
	payload := append(append([]byte{}, frame...), frame...)

	t.Run("packetAligned=false parses multiple frames", func(t *testing.T) {
		probes := map[uint64]*matroskaAudioProbe{
			track: {format: "E-AC-3", collect: true, parseJOC: false},
		}
		probeMatroskaAudio(probes, track, payload, 1, int64(len(payload)), false)
		p := probes[track]
		if p == nil || !p.ok {
			t.Fatalf("probe ok=%v, want true", p != nil && p.ok)
		}
		if p.info.framesMerged != 2 {
			t.Fatalf("framesMerged=%d, want 2", p.info.framesMerged)
		}
		if p.info.dialnormCount != 2 {
			t.Fatalf("dialnormCount=%d, want 2", p.info.dialnormCount)
		}
	})

	t.Run("packetAligned=true rejects multi-frame packet", func(t *testing.T) {
		probes := map[uint64]*matroskaAudioProbe{
			track: {format: "E-AC-3", collect: true, parseJOC: false},
		}
		probeMatroskaAudio(probes, track, payload, 1, int64(len(payload)), true)
		p := probes[track]
		if p == nil {
			t.Fatal("missing probe")
		}
		if p.ok {
			t.Fatalf("probe ok=true, want false")
		}
	})
}

func TestProbeMatroskaAudio_EAC3JOCResyncsAcrossPacketPadding(t *testing.T) {
	const track = 1
	frame := buildEAC3Frame(20, 1, 0x00)
	jocFrame := buildEAC3Frame(64, 2, 0x00)
	copy(jocFrame[20:], buildEMDFJOCPayload())
	payload := append(append(append([]byte{}, frame...), bytes.Repeat([]byte{0xAA}, 16)...), jocFrame...)

	probes := map[uint64]*matroskaAudioProbe{
		track: {format: "E-AC-3", collect: true, parseJOC: true},
	}
	probeMatroskaAudio(probes, track, payload, 1, int64(len(payload)), false)

	p := probes[track]
	if p == nil || !p.ok {
		t.Fatalf("probe ok=%v, want true", p != nil && p.ok)
	}
	if !p.info.hasJOC {
		t.Fatal("expected JOC metadata from padded second syncframe")
	}
	if p.info.framesMerged != 2 {
		t.Fatalf("framesMerged=%d, want 2", p.info.framesMerged)
	}
}

func TestApplyMatroskaAudioProbes_EAC3JOCTextMetadata(t *testing.T) {
	info := &MatroskaInfo{
		Tracks: []Stream{
			{
				Kind: StreamAudio,
				Fields: []Field{
					{Name: "ID", Value: "2"},
					{Name: "Format", Value: "E-AC-3"},
					{Name: "Format/Info", Value: "Enhanced AC-3"},
					{Name: "Codec ID", Value: "A_EAC3"},
					{Name: "Bit rate mode", Value: "Constant"},
					{Name: "Bit rate", Value: "1 536 kb/s"},
					{Name: "Default", Value: "Yes"},
					{Name: "Forced", Value: "No"},
				},
				JSON: map[string]string{"BitRate": "1536000"},
			},
		},
	}
	probes := map[uint64]*matroskaAudioProbe{
		2: {
			format: "E-AC-3",
			ok:     true,
			info: ac3Info{
				hasJOC:        true,
				hasJOCComplex: true,
				jocComplexity: 16,
				hasJOCDyn:     true,
				jocDynObjects: 15,
				hasJOCBed:     true,
				jocBedCount:   1,
				jocBedLayout:  "LFE",
				channels:      8,
				layout:        "L R C LFE Ls Rs Tfl Tfr",
				sampleRate:    48000,
				spf:           1536,
				frameRate:     31.25,
				bitRateKbps:   640,
				bsid:          6,
				acmod:         7,
				lfeon:         1,
				hasDialnorm:   true,
				dialnorm:      -31,
				dialnormCount: 1,
				dialnormSum:   math.Pow(10.0, -31.0/10.0),
				dialnormMin:   -31,
				dialnormMax:   -31,
				hasCompr:      true,
				comprDB:       -0.28,
				hasCmixlev:    true,
				cmixlevDB:     -3,
				hasSurmixlev:  true,
				surmixlevDB:   -3,
				hasDmixmod:    true,
				dmixmod:       "Lo/Ro",
			},
		},
	}

	applyMatroskaAudioProbes(info, probes)

	stream := info.Tracks[0]
	checks := map[string]string{
		"Format":                    "E-AC-3 JOC",
		"Format/Info":               "Enhanced AC-3 with Joint Object Coding",
		"Commercial name":           "Dolby Digital Plus with Dolby Atmos",
		"Format profile":            "Blu-ray Disc",
		"Bit rate mode":             "Constant",
		"Bit rate":                  "1 536 kb/s",
		"Complexity index":          "16",
		"Number of dynamic objects": "15",
		"Bed channel count":         "1 channel",
		"Bed channel configuration": "LFE",
		"Dialog Normalization":      "-31 dB",
		"compr":                     "-0.28 dB",
		"cmixlev":                   "-3.0 dB",
		"surmixlev":                 "-3 dB",
		"dmixmod":                   "Lo/Ro",
		"dialnorm_Average":          "-31 dB",
		"dialnorm_Minimum":          "-31 dB",
		"dialnorm_Maximum":          "-31 dB",
	}
	for name, want := range checks {
		if got := findField(stream.Fields, name); got != want {
			t.Fatalf("%s=%q, want %q", name, got, want)
		}
	}
	if got := stream.JSON["BitRate"]; got != "1536000" {
		t.Fatalf("JSON BitRate=%q, want 1536000", got)
	}
	if got := stream.JSON["Format"]; got != "E-AC-3" {
		t.Fatalf("JSON Format=%q, want E-AC-3", got)
	}
	if got := stream.JSON["Format_AdditionalFeatures"]; got != "JOC" {
		t.Fatalf("JSON Format_AdditionalFeatures=%q, want JOC", got)
	}
	if got := stream.JSONRaw["extra"]; !strings.Contains(got, `"acmod":"7","lfeon":"1","dmixmod":"Lo/Ro"`) {
		t.Fatalf("extra missing or misordering E-AC-3 mixing metadata: %s", got)
	}
}
