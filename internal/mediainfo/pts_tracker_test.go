package mediainfo

import "testing"

func TestPtsTrackerSegments(t *testing.T) {
	t.Run("small-reorder-does-not-reset", func(t *testing.T) {
		var tracker ptsTracker
		tracker.add(1000)
		tracker.add(2000)
		tracker.add(1500) // reordering (e.g. B-frames)
		tracker.add(2500)

		if tracker.hasResets() {
			t.Fatalf("did not expect reset tracking for small backwards jump")
		}
	})

	t.Run("large-jump-resets", func(t *testing.T) {
		var tracker ptsTracker
		tracker.add(0)
		tracker.add(90_000 * 100) // 100 seconds
		tracker.add(0)            // reset / discontinuity
		tracker.add(90_000 * 10)  // 10 seconds

		if !tracker.hasResets() {
			t.Fatalf("expected reset tracking")
		}

		if got, want := tracker.duration(), float64(90_000*100)/90000.0; got != want {
			t.Fatalf("duration mismatch: got %v want %v", got, want)
		}
		if got, want := tracker.durationTotal(), float64(90_000*100+90_000*10)/90000.0; got != want {
			t.Fatalf("total mismatch: got %v want %v", got, want)
		}
	})
}
