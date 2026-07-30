package mediainfo

// normalizeAACBitRate mirrors MediaInfoLib's well-known AAC rate
// normalization intervals.
func normalizeAACBitRate(bitRate int64) int64 {
	for _, known := range [...]struct {
		minimum int64
		nominal int64
		maximum int64
	}{
		{46_000, 48_000, 50_000},
		{64_827, 66_150, 67_473},
		{70_560, 72_000, 73_440},
		{94_080, 96_000, 97_920},
		{129_654, 132_300, 134_946},
		{141_120, 144_000, 146_880},
		{188_160, 192_000, 195_840},
		{259_308, 264_600, 269_892},
		{282_240, 288_000, 293_760},
		{345_744, 352_800, 359_856},
		{376_320, 384_000, 391_680},
		{518_616, 529_200, 539_784},
		{564_480, 576_000, 587_520},
		{648_270, 661_500, 674_730},
	} {
		if bitRate >= known.minimum && bitRate <= known.maximum {
			return known.nominal
		}
	}
	return bitRate
}
