package mediainfo

// embeddedImageKind identifies a supported content-detected image format.
type embeddedImageKind uint8

const (
	embeddedImageUnknown embeddedImageKind = iota
	embeddedImageJPEG
	embeddedImagePNG
	embeddedImageGIF
)

// embeddedImageDetection records the detected image kind and canonical MIME
// type derived from payload bytes.
type embeddedImageDetection struct {
	kind embeddedImageKind
	mime string
}

// embeddedImageDetector identifies an image solely from bounded payload bytes.
type embeddedImageDetector func([]byte) (embeddedImageDetection, bool)

// embeddedImageDetectors defines deterministic content-probe precedence.
var embeddedImageDetectors = []embeddedImageDetector{
	detectEmbeddedJPEG,
	detectEmbeddedPNG,
	detectEmbeddedGIF,
}

// detectEmbeddedImage tries registered content parsers in order. Container MIME
// types and filenames are intentionally not inputs and cannot gate probing.
func detectEmbeddedImage(data []byte) (embeddedImageDetection, bool) {
	for _, detector := range embeddedImageDetectors {
		if detection, ok := detector(data); ok {
			return detection, true
		}
	}
	return embeddedImageDetection{}, false
}

// detectEmbeddedJPEG validates JPEG structure and returns its canonical type.
func detectEmbeddedJPEG(data []byte) (embeddedImageDetection, bool) {
	_, _, _, _, _, _, _, ok := parseJPEGAttachment(data)
	return embeddedImageDetection{kind: embeddedImageJPEG, mime: "image/jpeg"}, ok
}

// detectEmbeddedPNG validates PNG structure and returns its canonical type.
func detectEmbeddedPNG(data []byte) (embeddedImageDetection, bool) {
	_, _, _, _, ok := parsePNGAttachment(data)
	return embeddedImageDetection{kind: embeddedImagePNG, mime: "image/png"}, ok
}

// detectEmbeddedGIF validates GIF structure and returns its canonical type.
func detectEmbeddedGIF(data []byte) (embeddedImageDetection, bool) {
	_, _, _, ok := parseGIFAttachment(data)
	return embeddedImageDetection{kind: embeddedImageGIF, mime: "image/gif"}, ok
}
