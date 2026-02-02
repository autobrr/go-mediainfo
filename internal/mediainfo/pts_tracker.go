package mediainfo

import "math"

type ptsTracker struct {
	first uint64
	last  uint64
	ok    bool
}

func (t *ptsTracker) add(pts uint64) {
	if !t.ok {
		t.first = pts
		t.last = pts
		t.ok = true
		return
	}
	t.last = pts
}

func (t ptsTracker) duration() float64 {
	if !t.ok {
		return 0
	}
	first := t.first
	last := t.last
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
