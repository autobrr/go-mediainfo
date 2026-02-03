package mediainfo

func parsePTS(buf []byte) (uint64, bool) {
	if len(buf) < 5 {
		return 0, false
	}
	if (buf[0]&0xF0) != 0x20 && (buf[0]&0xF0) != 0x30 {
		return 0, false
	}
	if (buf[0]&0x01) == 0 || (buf[2]&0x01) == 0 || (buf[4]&0x01) == 0 {
		return 0, false
	}
	pts := (uint64(buf[0]&0x0E) << 29) |
		(uint64(buf[1]) << 22) |
		(uint64(buf[2]&0xFE) << 14) |
		(uint64(buf[3]) << 7) |
		(uint64(buf[4]) >> 1)
	return pts, true
}
