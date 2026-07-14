package mediainfo

type StreamKind string

const (
	StreamGeneral StreamKind = "General"
	StreamVideo   StreamKind = "Video"
	StreamAudio   StreamKind = "Audio"
	StreamText    StreamKind = "Text"
	StreamImage   StreamKind = "Image"
	StreamMenu    StreamKind = "Menu"
)

type Field struct {
	Name  string
	Value string
}

type Stream struct {
	Kind                   StreamKind
	Fields                 []Field
	JSON                   map[string]string
	JSONRaw                map[string]string
	JSONSkipStreamOrder    bool
	JSONSkipComputed       bool
	JSONSkipFrameRateRatio bool
	eac3Dec3               eac3Dec3Info
	nalLengthSize          int
	// mkvH264SPS retains CodecPrivate timing metadata needed to decode frame SEI.
	mkvH264SPS          h264SPSInfo
	mkvHeaderStripBytes []byte
	mkvDolbyVision      dolbyVisionConfig
	mkvHasDolbyVision   bool
	// x265 writing library / encoding settings extracted from the HEVC
	// DecoderConfigurationRecord (hvcC) SEI, when the muxer placed the x265
	// user-data SEI in CodecPrivate rather than in frame data.
	mkvHEVCX265Library  string
	mkvHEVCX265Settings string
	mkvTrackOffsetNs    int64
	mkvStereoMode       uint64
	// mkvGoJSON retains intentional Go-only JSON extensions until all
	// MediaInfo-compatible report calculations have completed.
	mkvGoJSON    map[string]string
	mkvGoJSONRaw map[string]string
}

type Report struct {
	Ref     string
	General Stream
	Streams []Stream
}
