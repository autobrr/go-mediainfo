package mediainfo

import "encoding/binary"

const mp4SampleSizeHeadMax = 16
const mp4SampleSizeTailMax = 16

func parseStszWithHead(payload []byte, headN int) (uint64, []uint32, []uint32, bool) {
	if len(payload) < 12 {
		return 0, nil, nil, false
	}
	sampleSize := binary.BigEndian.Uint32(payload[4:8])
	sampleCount := binary.BigEndian.Uint32(payload[8:12])
	if sampleCount == 0 {
		return 0, nil, nil, false
	}
	if headN < 0 {
		headN = 0
	}
	if headN > mp4SampleSizeHeadMax {
		headN = mp4SampleSizeHeadMax
	}
	if sampleSize > 0 {
		head := []uint32(nil)
		tail := []uint32(nil)
		if headN > 0 {
			n := int(sampleCount)
			if n > headN {
				n = headN
			}
			head = make([]uint32, n)
			for i := 0; i < n; i++ {
				head[i] = sampleSize
			}
		}
		nTail := int(sampleCount)
		if nTail > mp4SampleSizeTailMax {
			nTail = mp4SampleSizeTailMax
		}
		tail = make([]uint32, nTail)
		for i := 0; i < nTail; i++ {
			tail[i] = sampleSize
		}
		return uint64(sampleSize) * uint64(sampleCount), head, tail, true
	}
	offset := 12
	var total uint64
	head := []uint32(nil)
	if headN > 0 {
		n := int(sampleCount)
		if n > headN {
			n = headN
		}
		head = make([]uint32, 0, n)
	}
	tailN := int(sampleCount)
	if tailN > mp4SampleSizeTailMax {
		tailN = mp4SampleSizeTailMax
	}
	tailRing := make([]uint32, tailN)
	tailCount := 0
	tailPos := 0
	for i := 0; i < int(sampleCount); i++ {
		if offset+4 > len(payload) {
			break
		}
		size := binary.BigEndian.Uint32(payload[offset : offset+4])
		total += uint64(size)
		if head != nil && len(head) < cap(head) {
			head = append(head, size)
		}
		if tailN > 0 {
			tailRing[tailPos] = size
			tailPos = (tailPos + 1) % tailN
			if tailCount < tailN {
				tailCount++
			}
		}
		offset += 4
	}
	if total == 0 {
		return 0, nil, nil, false
	}
	tail := []uint32(nil)
	if tailCount > 0 {
		tail = make([]uint32, tailCount)
		// tailPos points to the slot where the next write would go.
		start := tailPos - tailCount
		if start < 0 {
			start += tailN
		}
		for i := 0; i < tailCount; i++ {
			tail[i] = tailRing[(start+i)%tailN]
		}
	}
	return total, head, tail, true
}

func parseStsz(payload []byte) (uint64, bool) {
	total, _, _, ok := parseStszWithHead(payload, 0)
	return total, ok
}
