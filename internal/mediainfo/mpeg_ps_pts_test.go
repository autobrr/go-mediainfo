package mediainfo

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestMPEGPSRLEDurationEvidence(t *testing.T) {
	const wrap = uint64(1 << 33)
	for _, tc := range []struct {
		name     string
		pts      []uint64
		gap      bool
		sampled  bool
		noPTS    bool
		generic  bool
		duration string
	}{
		{name: "monotonic", pts: []uint64{90_000, 180_000}, duration: "1000"},
		{name: "small reorder", pts: []uint64{90_000, 180_000, 170_000}, duration: "1000"},
		{name: "continuous wrap", pts: []uint64{wrap - 90_000, 90_000}, duration: "2000"},
		{name: "wrap with forward endpoint", pts: []uint64{90_000, wrap - 90_000, 0, 180_000}},
		{name: "two observed wraps", pts: []uint64{wrap - 90_000, 90_000, wrap - 90_000, 90_000}},
		{name: "forward step over half range", pts: []uint64{90_000, wrap - 90_000}},
		{name: "forward step at half range", pts: []uint64{90_000, 90_000 + wrap/2}},
		{name: "forward half range after reorder", pts: []uint64{180_000, 90_000, 90_000 + wrap/2}},
		{name: "backward half range after reorder", pts: []uint64{wrap/2 + 90_000, wrap / 2, 0}},
		{name: "generic backward reset", pts: []uint64{900_000, 90_000}, generic: true},
		{name: "generic ambiguous forward step", pts: []uint64{90_000, wrap - 90_000, 0, 180_000}, generic: true},
		{name: "wrap within sampled section", pts: []uint64{wrap - 90_000, 90_000}, sampled: true, duration: "2000"},
		{name: "sampled monotonic span", pts: []uint64{90_000, 180_000}, gap: true, sampled: true, duration: "1000"},
		{name: "backward reset", pts: []uint64{900_000, 90_000}},
		{name: "forward endpoint after reset", pts: []uint64{90_000, 900_000, 0, 180_000}},
		{name: "sampled reset endpoints", pts: []uint64{1_211_442, 44_722}, gap: true, sampled: true},
		{name: "wrap across unsampled gap", pts: []uint64{wrap - 90_000, 90_000}, gap: true, sampled: true},
		{name: "reset then wrap", pts: []uint64{900_000, 0, wrap - 90_000, 90_000}},
		{name: "single timestamp", pts: []uint64{90_000}},
		{name: "no timestamps", noPTS: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := newPSStreamParser(mpegPSOptions{})
			if tc.noPTS {
				if !parser.parseReader(bytes.NewReader(testDVDSPUPES(0x21, testDVDSPU()))) {
					t.Fatal("payload did not parse")
				}
			}
			for i, pts := range tc.pts {
				if tc.gap && i == 1 {
					parser.beginSection()
				}
				packet := []byte{0, 0, 1, 0xbd, 0, 0, 0x80, 0x80, 5,
					0x21 | byte(pts>>30&7)<<1, byte(pts >> 22), byte(pts>>15&0x7f)<<1 | 1,
					byte(pts >> 7), byte(pts&0x7f)<<1 | 1, 0x21}
				packet = append(packet, testDVDSPU()...)
				binary.BigEndian.PutUint16(packet[4:6], uint16(len(packet)-6))
				if !parser.parseReader(bytes.NewReader(packet)) {
					t.Fatal("timestamped payload did not parse")
				}
			}
			var videoPTS ptsTracker
			videoPTS.add(0)
			_, streams, ok := finalizeMPEGPS(parser.streams, parser.streamOrder, nil, videoPTS, parser.anyPTS, 4096,
				mpegPSOptions{dvdParsing: !tc.generic, parseSpeed: 0.5, sampled: tc.sampled})
			if !ok || len(streams) != 1 {
				t.Fatalf("finalization lost payload stream: ok=%v streams=%d", ok, len(streams))
			}
			stream := streams[0]
			if id, _ := canonicalSeedValue(stream, "ID"); id != "189-33" {
				t.Fatalf("ID = %q", id)
			}
			if got, present := canonicalSeedValue(stream, "Duration"); got != tc.duration || present != (tc.duration != "") {
				t.Errorf("Duration = %q, present=%v; want %q", got, present, tc.duration)
			}
			if tc.duration == "" && findField(stream.Fields, "Duration") != "" {
				t.Error("unsupported duration leaked into text fields")
			}
			if len(tc.pts) > 0 {
				want := float64(tc.pts[0]) / 90_000
				for _, key := range []fieldName{"Delay", "Video_Delay"} {
					value, present := canonicalSeedValue(stream, key)
					got, valid := parseFloatValue(value)
					if !present || !valid || math.Abs(got-want) > 0.0005 {
						t.Errorf("%s = %q; want preserved first PTS %.9f", key, value, want)
					}
				}
			}
		})
	}
}

func TestPtsDurationPS_Resets(t *testing.T) {
	var tracker ptsTracker
	tracker.add(0)
	tracker.add(90_000 * 100) // 100 seconds
	tracker.add(0)            // discontinuity
	tracker.add(90_000 * 10)  // 10 seconds

	t.Run("parseSpeed<1 uses total", func(t *testing.T) {
		got := ptsDurationPS(tracker, mpegPSOptions{parseSpeed: 0.5})
		if got != 110 {
			t.Fatalf("duration=%v, want 110", got)
		}
	})

	t.Run("parseSpeed>=1 uses minmax span (non-dvd)", func(t *testing.T) {
		got := ptsDurationPS(tracker, mpegPSOptions{parseSpeed: 1})
		if got != 100 {
			t.Fatalf("duration=%v, want 100", got)
		}
	})

	t.Run("dvdParsing uses last segment", func(t *testing.T) {
		got := ptsDurationPS(tracker, mpegPSOptions{parseSpeed: 1, dvdParsing: true})
		if got != 10 {
			t.Fatalf("duration=%v, want 10", got)
		}
	})
}

func TestSampledFrameClockDurationPSUsesResetSegmentStart(t *testing.T) {
	var tracker ptsTracker
	tracker.add(9_000_000)
	tracker.add(9_090_000)
	tracker.add(90_000)
	tracker.add(180_000)
	if !tracker.hasResets() || tracker.segmentStart == tracker.first {
		t.Fatalf("tracker did not establish distinct reset segment: %#v", tracker)
	}
	stream := &psStream{
		pts:             tracker,
		audioFrames:     12,
		sampleSection:   2,
		clockPTS:        270_000,
		clockHasPTS:     true,
		clockAudioStart: 10,
	}
	got, ok := sampledFrameClockDurationPS(stream, mpegPSOptions{sampled: true}, 48_000, 1152, false)
	want := 2.0 + 2.0*1152.0/48_000.0
	if !ok || math.Abs(got-want) > 1e-9 {
		t.Fatalf("sampledFrameClockDurationPS() = %.9f, %v; want %.9f, true", got, ok, want)
	}
}
