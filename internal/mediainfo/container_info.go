package mediainfo

type ContainerInfo struct {
	DurationSeconds   float64
	BitrateMode       string
	OverallBitrateMin float64
	OverallBitrateMax float64
	// StreamOverheadBytes is container-level overhead not attributable to any single stream
	// (e.g. TS headers, BDAV timestamps, adaptation fields).
	StreamOverheadBytes int64
}

func (c ContainerInfo) HasDuration() bool {
	return c.DurationSeconds > 0
}
