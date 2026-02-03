package mediainfo

func formatAACVersion(version int) string {
	switch version {
	case 2:
		return "Version 2"
	case 4:
		return "Version 4"
	default:
		return "Version 4"
	}
}

func adtsMPEGVersion(idBit byte) int {
	if idBit == 1 {
		return 2
	}
	return 4
}
