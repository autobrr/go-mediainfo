package mediainfo

type ptsTracker struct {
	first        uint64
	min          uint64
	max          uint64
	last         uint64
	segmentStart uint64
	segmentTotal uint64
	lastNonZero  uint64
	resets       int
	ok           bool
}

func (t *ptsTracker) add(pts uint64) {
	// PTS can be slightly non-monotonic (e.g. B-frame reordering).
	// Treat only large backwards jumps as discontinuities (segment breaks).
	const reorderMax = 2 * 90000 // 2 seconds

	if !t.ok {
		t.first = pts
		t.min = pts
		t.max = pts
		t.last = pts
		t.segmentStart = pts
		t.ok = true
		return
	}
	if pts < t.last {
		backward := t.last - pts
		if backward > reorderMax {
			segment := ptsDelta(t.segmentStart, t.last)
			t.segmentTotal += segment
			if segment > 0 {
				t.lastNonZero = segment
			}
			t.segmentStart = pts
			t.resets++
			t.last = pts
		}
	} else {
		t.last = pts
	}
	if pts < t.min {
		t.min = pts
	}
	if pts > t.max {
		t.max = pts
	}
}

func (t *ptsTracker) addTextPTS(pts uint64) {
	// Text streams (notably BDAV PGS) can have slightly non-monotonic PTS even in file order.
	// MediaInfo behavior aligns closer to treating the last seen PTS as the track end, rather than
	// keeping the maximum PTS across a small reordering window.
	const reorderMax = 2 * 90000 // 2 seconds

	if !t.ok {
		t.first = pts
		t.min = pts
		t.max = pts
		t.last = pts
		t.segmentStart = pts
		t.ok = true
		return
	}
	if pts < t.last {
		backward := t.last - pts
		if backward > reorderMax {
			segment := ptsDelta(t.segmentStart, t.last)
			t.segmentTotal += segment
			if segment > 0 {
				t.lastNonZero = segment
			}
			t.segmentStart = pts
			t.resets++
		}
		// Accept small reordering as-is for text: last should reflect the last seen PTS.
		t.last = pts
	} else {
		t.last = pts
	}
	if pts < t.min {
		t.min = pts
	}
	if pts > t.max {
		t.max = pts
	}
}

func (t *ptsTracker) breakSegment(start uint64) {
	if !t.ok {
		t.add(start)
		return
	}
	segment := ptsDelta(t.segmentStart, t.last)
	t.segmentTotal += segment
	if segment > 0 {
		t.lastNonZero = segment
	}
	t.segmentStart = start
	t.last = start
	t.resets++
	if start < t.min {
		t.min = start
	}
	if start > t.max {
		t.max = start
	}
}

func (t ptsTracker) duration() float64 {
	if !t.ok {
		return 0
	}
	return float64(ptsDelta(t.min, t.max)) / 90000.0
}

func (t ptsTracker) durationLastSegment() float64 {
	if !t.ok {
		return 0
	}
	segment := ptsDelta(t.segmentStart, t.last)
	if segment == 0 && t.lastNonZero > 0 {
		segment = t.lastNonZero
	}
	return float64(segment) / 90000.0
}

func (t ptsTracker) durationTotal() float64 {
	if !t.ok {
		return 0
	}
	return float64(t.segmentTotal+ptsDelta(t.segmentStart, t.last)) / 90000.0
}

// durationTotalTicks returns the accumulated PTS duration in the track's
// native 90 kHz clock, including segments separated by timestamp resets.
func (t ptsTracker) durationTotalTicks() uint64 {
	if !t.ok {
		return 0
	}
	return t.segmentTotal + ptsDelta(t.segmentStart, t.last)
}

func (t ptsTracker) hasResets() bool {
	return t.resets > 0
}

func (t ptsTracker) has() bool {
	return t.ok
}

func ptsDelta(start, end uint64) uint64 {
	if end < start {
		end += 1 << 33
	}
	return end - start
}
