package mediainfo

type ContainerInfo struct {
	DurationSeconds float64
	BitrateMode     string
}

func (c ContainerInfo) HasDuration() bool {
	return c.DurationSeconds > 0
}
