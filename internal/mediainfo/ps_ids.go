package mediainfo

type psStream struct {
	id               byte
	subID            byte
	kind             StreamKind
	format           string
	bytes            uint64
	frames           uint64
	pts              ptsTracker
	audioProfile     string
	audioObject      int
	audioMPEGVersion int
	audioRate        float64
	audioChannels    uint64
	hasAudioInfo     bool
	audioFrames      uint64
	audioBuffer      []byte
	ac3Info          ac3Info
	hasAC3           bool
	videoHeaderBytes uint64
	videoHeaderCarry []byte
}
