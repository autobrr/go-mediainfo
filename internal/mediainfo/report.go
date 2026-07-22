package mediainfo

// StreamKind identifies a MediaInfo stream category.
type StreamKind string

const (
	// StreamGeneral identifies container-level metadata.
	StreamGeneral StreamKind = "General"
	// StreamVideo identifies a video stream.
	StreamVideo StreamKind = "Video"
	// StreamAudio identifies an audio stream.
	StreamAudio StreamKind = "Audio"
	// StreamText identifies a subtitle or caption stream.
	StreamText StreamKind = "Text"
	// StreamImage identifies an image stream.
	StreamImage StreamKind = "Image"
	// StreamMenu identifies a chapter or menu stream.
	StreamMenu StreamKind = "Menu"
)

// Field is one display-label/value pair in a legacy text stream snapshot.
type Field struct {
	// Name is the display label used by text-oriented renderers.
	Name string
	// Value is the preformatted display value associated with Name.
	Value string
}

// Stream contains one media stream's public compatibility snapshot. Parsers
// also retain private canonical entries used by the built-in renderers.
type Stream struct {
	// Kind identifies the General, video, audio, text, image, or menu stream.
	Kind StreamKind
	// Fields contains the ordered legacy text projection.
	Fields []Field
	// JSON contains scalar overrides accepted by the public legacy adapter.
	JSON map[string]string
	// JSONRaw contains pre-encoded structured values accepted by the legacy adapter.
	JSONRaw map[string]string
	// JSONSkipStreamOrder suppresses generated StreamOrder in legacy input.
	JSONSkipStreamOrder bool
	// JSONSkipComputed suppresses legacy computed structured fields.
	JSONSkipComputed bool
	// JSONSkipFrameRateRatio suppresses legacy frame-rate ratio derivation.
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
	mkvDolbyVisionCount int
	// x265 writing library / encoding settings extracted from the HEVC
	// DecoderConfigurationRecord (hvcC) SEI, when the muxer placed the x265
	// user-data SEI in CodecPrivate rather than in frame data.
	mkvHEVCX265Library   string
	mkvHEVCX265Settings  string
	mkvTrackOffsetNs     int64
	mkvStereoMode        uint64
	mkvTagFrameCount     bool
	dvdMPEG2IntraDCFirst int
	dvdMPEG2IntraDCLast  int
	dvdMPEG2MaxBitRate   int64
	// matroskaDeferredFacts holds fallback TrackEntry values until all parser
	// refinements have updated the canonical seed.
	matroskaDeferredFacts *matroskaDeferredFacts
	// reportStore is attached only to General to preserve Report's public three-field layout.
	reportStore *fieldStore
	// reportSnapshot detects caller mutation before a renderer reuses reportStore.
	reportSnapshot *legacyReportState
	// canonicalSeed retains parser-direct entries until analysis attaches the report store.
	canonicalSeed []fieldEntry
	// canonicalPolicy retains format-neutral projection policy until adapters publish legacy flags.
	canonicalPolicy canonicalStreamPolicy
}

// Report contains the public compatibility snapshot for one analyzed input.
type Report struct {
	// Ref identifies the analyzed input path.
	Ref string
	// General contains the input's container-level stream.
	General Stream
	// Streams contains non-General streams in display order.
	Streams []Stream
}
