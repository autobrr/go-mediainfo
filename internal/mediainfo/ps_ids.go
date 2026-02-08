package mediainfo

type psStream struct {
	id                   byte
	subID                byte
	kind                 StreamKind
	format               string
	bytes                uint64
	pts                  ptsTracker
	audioProfile         string
	audioObject          int
	audioMPEGVersion     int
	audioRate            float64
	audioChannels        uint64
	hasAudioInfo         bool
	audioFrames          uint64
	audioBuffer          []byte
	ac3Info              ac3Info
	hasAC3               bool
	videoHeaderBytes     uint64
	videoSeqExtBytes     uint64
	videoGOPBytes        uint64
	videoHeaderCarry     []byte
	videoFrameBytes      uint64
	videoFrameBytesCount int
	videoFrameCarry      []byte
	videoFramePos        int64
	videoFrameStart      int64
	videoFrameStartSet   bool
	videoStartZeroRun    int
	videoExtraZeros      uint64
	videoTotalBytes      uint64
	videoLastStartPos    int64
	videoNoPTSPackets    uint64
	videoFields          []Field
	hasVideoFields       bool
	videoWidth           uint64
	videoHeight          uint64
	videoFrameRate       float64
	videoSliceCount      int
	videoSliceProbed     bool
	videoIsH264          bool
	videoIsMPEG2         bool
	videoBuffer          []byte
	videoCCCarry         []byte
	videoFrameCount      int
	derivedDuration      float64
	ccFound              bool
	ccOdd                ccTrack
	ccEven               ccTrack
	firstPacketOrder     int
	packetCount          int
}

type ccTrack struct {
	found             bool
	firstFrame        int
	lastFrame         int
	firstPTS          uint64
	lastPTS           uint64
	firstCommandPTS   uint64
	firstCommandFrame int
	firstDisplayPTS   uint64
	firstType         string
}
