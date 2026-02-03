package mediainfo

type ContainerInfo struct {
	DurationSeconds   float64
	BitrateMode       string
	OverallBitrateMin float64
	OverallBitrateMax float64
}

func (c ContainerInfo) HasDuration() bool {
	return c.DurationSeconds > 0
}
