package mediainfo

func addStreamCommon(fields []Field, duration float64, bitrate float64) []Field {
	fields = addStreamDuration(fields, duration)
	if bitrate > 0 {
		if mode := bitrateMode(bitrate); mode != "" {
			fields = appendFieldUnique(fields, Field{Name: "Bit rate mode", Value: mode})
		}
		fields = addStreamBitrate(fields, bitrate)
	}
	return fields
}
