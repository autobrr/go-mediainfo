package mediainfo

// Minimal EIA-608 XDS decoder.
//
// Goal: parity with MediaInfo's General.Title/Movie and General.LawRating extraction
// from CEA-608-in-708 (GA94) data. Ref: MediaInfoLib Text/File_Eia608.cpp.
type eia608XDS struct {
	// Active packet index in data, or -1 if none/invalid.
	level int
	data  [][]byte
}

func (x *eia608XDS) init() {
	if x.level == 0 && x.data == nil {
		x.level = -1
	}
}

// feed consumes one (cc_data_1, cc_data_2) pair and returns extracted Title or LawRating
// when a full XDS packet completes.
func (x *eia608XDS) feed(cc1, cc2 byte) (title string, lawRating string, ok bool) {
	x.init()

	// XDS protocol:
	// - Start: cc1 in 0x01..0x0E (odd) identifies packet class/type.
	// - Continue: cc1 in 0x02..0x0F (even) selects which packet continues; payload is not part of data.
	// - Data: other byte pairs are appended to the active packet.
	// - End: cc1 == 0x0F finalizes.
	if cc1 != 0 && cc1 < 0x10 && cc1%2 == 0 {
		// Continue: decrement cc1 and find existing entry by first 2 bytes.
		cc1--
		idx := -1
		for i := range x.data {
			if len(x.data[i]) >= 2 && x.data[i][0] == cc1 && x.data[i][1] == cc2 {
				idx = i
				break
			}
		}
		if idx < 0 {
			x.level = -1
			return "", "", false
		}
		x.level = idx
		return "", "", false
	}
	if cc1 != 0 && cc1 < 0x0F {
		// Start: locate or create new entry.
		idx := -1
		for i := range x.data {
			if len(x.data[i]) >= 2 && x.data[i][0] == cc1 && x.data[i][1] == cc2 {
				idx = i
				break
			}
		}
		if idx < 0 {
			x.data = append(x.data, nil)
			idx = len(x.data) - 1
		} else {
			// Restart existing entry.
			x.data[idx] = x.data[idx][:0]
		}
		x.level = idx
	}

	// MediaInfoLib only treats basic characters (cc1>=0x20) as XDS payload when a packet is active.
	// Control codes (0x10..0x1F) interleave on the same channel and must not corrupt XDS payload.
	if x.level >= 0 {
		if !((cc1 != 0 && cc1 < 0x10) || cc1 >= 0x20) {
			return "", "", false
		}
	}

	if x.level < 0 || x.level >= len(x.data) {
		return "", "", false
	}

	x.data[x.level] = append(x.data[x.level], cc1, cc2)
	if len(x.data[x.level]) >= 36 {
		// Security bound: drop oversized packets.
		x.data[x.level] = x.data[x.level][:0]
		x.level = -1
		return "", "", false
	}

	// End marker.
	if cc1 == 0x0F {
		return x.decodeAndClear()
	}
	return "", "", false
}

func (x *eia608XDS) decodeAndClear() (title string, lawRating string, ok bool) {
	if x.level < 0 || x.level >= len(x.data) {
		return "", "", false
	}
	buf := x.data[x.level]
	// Always clear entry to avoid repeated triggers.
	x.data[x.level] = x.data[x.level][:0]
	x.level = -1

	if len(buf) < 4 {
		return "", "", false
	}

	// XDS: first two bytes are class + type.
	class := buf[0]
	typ := buf[1]

	// Current class.
	if class != 0x01 {
		return "", "", false
	}

	switch typ {
	case 0x03:
		// Program Name: bytes 2..len-2; no checksum validation (matches MediaInfoLib behavior).
		if len(buf) <= 4 {
			return "", "", false
		}
		raw := buf[2 : len(buf)-2]
		out := make([]byte, 0, len(raw))
		for _, b := range raw {
			if b == 0 {
				continue
			}
			out = append(out, b)
		}
		if len(out) == 0 {
			return "", "", false
		}
		return string(out), "", true

	case 0x05:
		// Content advisory: 6 bytes total.
		if len(buf) != 6 {
			return "", "", false
		}
		r2 := buf[2]
		r3 := buf[3]
		a1a0 := (r2 >> 3) & 0x03
		rating := ""
		descriptors := ""

		switch a1a0 {
		case 0, 2:
			// MPAA style.
			switch r2 & 0x07 {
			case 0:
				rating = "N/A"
			case 1:
				rating = "G"
			case 2:
				rating = "PG"
			case 3:
				rating = "PG-13"
			case 4:
				rating = "R"
			case 5:
				rating = "NC-17"
			case 6:
				rating = "C"
			}
		case 1:
			// US TV Parental Guidelines.
			switch r3 & 0x07 {
			case 0, 7:
				rating = "None"
			case 1:
				rating = "TV-Y"
			case 2:
				rating = "TV-Y7"
			case 3:
				rating = "TV-G"
			case 4:
				rating = "TV-PG"
			case 5:
				rating = "TV-14"
			case 6:
				rating = "TV-MA"
			}
			if r2&0x20 != 0 {
				descriptors += "D"
			}
			if r3&0x08 != 0 {
				descriptors += "L"
			}
			if r3&0x10 != 0 {
				descriptors += "S"
			}
			if r3&0x20 != 0 {
				if (r3 & 0x07) == 2 {
					descriptors += "FV"
				} else {
					descriptors += "V"
				}
			}
		case 3:
			// Canada.
			if r3&0x08 != 0 {
				rating = "(Reserved)"
			} else if r2&0x20 != 0 {
				switch r3 & 0x07 {
				case 0:
					rating = "E"
				case 1:
					rating = "G"
				case 2:
					rating = "8+"
				case 3:
					rating = "13+"
				case 4:
					rating = "16+"
				case 5:
					rating = "18+"
				}
			} else {
				switch r3 & 0x07 {
				case 0:
					rating = "E"
				case 1:
					rating = "C"
				case 2:
					rating = "C8+"
				case 3:
					rating = "G"
				case 4:
					rating = "PG"
				case 5:
					rating = "14+"
				case 6:
					rating = "18+"
				}
			}
		}

		if rating == "" {
			return "", "", false
		}
		if descriptors != "" {
			rating = rating + " (" + descriptors + ")"
		}
		return "", rating, true
	}

	return "", "", false
}
