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

// dynamicFieldScope identifies the JSON object that owns a dynamic field.
type dynamicFieldScope uint8

const (
	dynamicFieldGeneral dynamicFieldScope = iota
	dynamicFieldTrack
)

// dynamicFieldSource identifies the parser metadata source for a dynamic field.
type dynamicFieldSource uint8

const (
	dynamicFieldSourceMatroskaTag dynamicFieldSource = iota
)

// dynamicJSONField retains parser-discovered metadata that has no fixed JSON
// schema slot. RawName preserves the container name, Name is its normalized
// renderer name, JSONName is the escaped output key, Scope and Source retain
// provenance, and Order preserves parser encounter order.
type dynamicJSONField struct {
	RawName  string
	Name     string
	JSONName string
	Value    string
	Scope    dynamicFieldScope
	Source   dynamicFieldSource
	Order    int
}

type Stream struct {
	Kind                   StreamKind
	Fields                 []Field
	JSON                   map[string]string
	JSONRaw                map[string]string
	JSONSkipStreamOrder    bool
	JSONSkipComputed       bool
	JSONSkipFrameRateRatio bool
	// JSONPreserveDisplayAR keeps an explicit Matroska display ratio from being
	// replaced by the shared width, height, and pixel-ratio normalization.
	JSONPreserveDisplayAR bool
	eac3Dec3              eac3Dec3Info
	nalLengthSize         int
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
	// mkvGoJSON and mkvGoJSONRaw stage intentional Go-only Matroska fields
	// without exposing them to shared text, XML, or parity calculations.
	mkvGoJSON    map[string]string
	mkvGoJSONRaw map[string]string
	dynamicJSON  []dynamicJSONField
}

type Report struct {
	Ref     string
	General Stream
	Streams []Stream
}
