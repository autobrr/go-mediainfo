package mediainfo

import "encoding/binary"

// dvdSPUProbe accepts complete, structurally bounded DVD subpicture units, not
// decoded bitmaps. Only supplemental discovery enables this per-stream state.
type dvdSPUProbe struct {
	pending  []byte
	accepted bool
	pts      uint64
	hasPTS   bool
}

// consume reassembles a 16-bit-sized SPU across PES payloads. A new timestamp
// abandons an incomplete previous unit; accepted evidence remains valid.
func (p *dvdSPUProbe) consume(data []byte, pts uint64, hasPTS bool) {
	if p.accepted {
		return
	}
	if hasPTS && (!p.hasPTS || pts != p.pts) {
		p.pending = nil
		p.pts = pts
		p.hasPTS = true
	}
	for len(data) > 0 {
		target := 2
		if len(p.pending) >= 2 {
			target = int(binary.BigEndian.Uint16(p.pending))
			if target < 10 {
				p.pending = nil
				return
			}
		}
		n := min(target-len(p.pending), len(data))
		p.pending = append(p.pending, data[:n]...)
		data = data[n:]
		if len(p.pending) < 2 {
			return
		}
		size := int(binary.BigEndian.Uint16(p.pending))
		if size < 10 {
			p.pending = nil
			return
		}
		if len(p.pending) == size {
			p.accepted = validDVDSPU(p.pending)
			p.pending = nil
			if p.accepted {
				return
			}
		}
	}
}

// validDVDSPU checks the unit and control-chain bounds, terminated commands and
// in-range pixel offsets. Pixel run lengths are deliberately not decoded here.
func validDVDSPU(data []byte) bool {
	if len(data) < 10 || int(binary.BigEndian.Uint16(data)) != len(data) {
		return false
	}
	control := int(binary.BigEndian.Uint16(data[2:]))
	if control < 4 || control > len(data)-5 {
		return false
	}
	started, area, pixels := false, false, false
	for current := control; ; {
		if current > len(data)-5 {
			return false
		}
		next := int(binary.BigEndian.Uint16(data[current+2:]))
		if next < current || next > len(data)-5 {
			return false
		}
		limit := len(data)
		if next > current {
			limit = next
		}
		pos := current + 4
		terminated := false
		for pos < limit {
			cmd := data[pos]
			pos++
			count := 0
			switch cmd {
			case 0, 1:
				started = true
			case 2:
			case 3, 4:
				count = 2
			case 5:
				count = 6
			case 6:
				count = 4
			case 7:
				started = true // Color/contrast changes also begin display.
				if pos+2 > limit {
					return false
				}
				count = int(binary.BigEndian.Uint16(data[pos:]))
				if count < 6 {
					return false
				}
			case 0xff:
				terminated = true
			default:
				return false
			}
			if terminated {
				break
			}
			if count > limit-pos {
				return false
			}
			if cmd == 5 {
				x1 := int(data[pos])<<4 | int(data[pos+1]>>4)
				x2 := int(data[pos+1]&15)<<8 | int(data[pos+2])
				y1 := int(data[pos+3])<<4 | int(data[pos+4]>>4)
				y2 := int(data[pos+4]&15)<<8 | int(data[pos+5])
				if x2 < x1 || y2 < y1 {
					return false
				}
				area = true
			}
			if cmd == 6 {
				top := int(binary.BigEndian.Uint16(data[pos:]))
				bottom := int(binary.BigEndian.Uint16(data[pos+2:]))
				if top < 4 || top >= control || bottom < 4 || bottom >= control {
					return false
				}
				pixels = true
			}
			pos += count
		}
		if !terminated {
			return false
		}
		if next == current {
			return started && area && pixels
		}
		current = next
	}
}
