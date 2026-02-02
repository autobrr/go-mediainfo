package mediainfo

type ContainerInfo struct {
	DurationSeconds float64
}

func (c ContainerInfo) HasDuration() bool {
	return c.DurationSeconds > 0
}
