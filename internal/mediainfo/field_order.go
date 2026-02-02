package mediainfo

var generalFieldOrder = map[string]int{
	"Complete name":         0,
	"Format":                1,
	"File size":             2,
	"Duration":              3,
	"Overall bit rate mode": 4,
	"Overall bit rate":      5,
}

var streamFieldOrder = map[string]int{
	"ID":             0,
	"Format":         1,
	"Duration":       2,
	"Bit rate mode":  3,
	"Bit rate":       4,
	"Width":          5,
	"Height":         6,
	"Frame rate":     7,
	"Channel(s)":     8,
	"Channel layout": 9,
	"Sampling rate":  10,
}
