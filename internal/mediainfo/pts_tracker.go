package mediainfo

import "math"

type ptsTracker struct {
	min uint64
	max uint64
	ok  bool
}

func (t *ptsTracker) add(pts uint64) {
	if !t.ok {
		t.min = pts
		t.max = pts
		t.ok = true
		return
	}
	if pts < t.min {
		t.min = pts
	}
	if pts > t.max {
		t.max = pts
	}
}

func (t ptsTracker) duration() float64 {
	if !t.ok {
		return 0
	}
	first := t.min
	last := t.max
	if last < first {
		last += 1 << 33
	}
	return float64(last-first) / 90000.0
}

func (t ptsTracker) has() bool {
	return t.ok
}

func safeRate(count uint64, duration float64) float64 {
	if duration <= 0 {
		return 0
	}
	rate := float64(count) / duration
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0
	}
	return rate
}
