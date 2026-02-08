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
	Kind                StreamKind
	Fields              []Field
	JSON                map[string]string
	JSONRaw             map[string]string
	JSONSkipStreamOrder bool
	JSONSkipComputed    bool
	eac3Dec3            eac3Dec3Info
	nalLengthSize       int
	mkvHeaderStripBytes []byte
	mkvDolbyVision      dolbyVisionConfig
	mkvHasDolbyVision   bool
}

type Report struct {
	Ref     string
	General Stream
	Streams []Stream
}
