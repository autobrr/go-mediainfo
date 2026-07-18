package mediainfo

import (
	"math"
	"testing"
)

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
