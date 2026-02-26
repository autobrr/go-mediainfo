// Package mediainfo exposes a public API for media analysis and output rendering
// without importing internal packages.
package mediainfo

import internalmediainfo "github.com/autobrr/go-mediainfo/internal/mediainfo"

type StreamKind = internalmediainfo.StreamKind
type Field = internalmediainfo.Field
type Stream = internalmediainfo.Stream
type Report = internalmediainfo.Report

// AnalyzeOptions controls optional analysis behavior.
//
// A nil option field keeps MediaInfo defaults.
type AnalyzeOptions struct {
	ParseSpeed              *float64
	TestContinuousFileNames *bool
}

func (opts AnalyzeOptions) toInternal() internalmediainfo.AnalyzeOptions {
	internalOpts := internalmediainfo.AnalyzeOptions{}
	if opts.ParseSpeed != nil {
		internalOpts.ParseSpeed = *opts.ParseSpeed
		internalOpts.HasParseSpeed = true
	}
	if opts.TestContinuousFileNames != nil {
		internalOpts.TestContinuousFileNames = *opts.TestContinuousFileNames
		internalOpts.HasTestContinuousFileNames = true
	}
	return internalOpts
}

const (
	AppName = internalmediainfo.AppName
	AppURL  = internalmediainfo.AppURL

	StreamGeneral = internalmediainfo.StreamGeneral
	StreamVideo   = internalmediainfo.StreamVideo
	StreamAudio   = internalmediainfo.StreamAudio
	StreamText    = internalmediainfo.StreamText
	StreamImage   = internalmediainfo.StreamImage
	StreamMenu    = internalmediainfo.StreamMenu
)

// AnalyzeFile analyzes a media file at path and returns a Report and error.
func AnalyzeFile(path string) (Report, error) {
	return internalmediainfo.AnalyzeFile(path)
}

// AnalyzeFileWithOptions analyzes a media file at path with opts and returns a Report and error.
func AnalyzeFileWithOptions(path string, opts AnalyzeOptions) (Report, error) {
	return internalmediainfo.AnalyzeFileWithOptions(path, opts.toInternal())
}

// AnalyzeFiles analyzes multiple media file paths and returns reports, analyzed count, and error.
func AnalyzeFiles(paths []string) ([]Report, int, error) {
	return internalmediainfo.AnalyzeFiles(paths)
}

// AnalyzeFilesWithOptions analyzes multiple media file paths with opts and returns reports, analyzed count, and error.
func AnalyzeFilesWithOptions(paths []string, opts AnalyzeOptions) ([]Report, int, error) {
	return internalmediainfo.AnalyzeFilesWithOptions(paths, opts.toInternal())
}

// DetectFormat detects a media format from header bytes and filename and returns the format name.
func DetectFormat(header []byte, filename string) string {
	return internalmediainfo.DetectFormat(header, filename)
}

// InfoParameters returns supported information parameter documentation as a string.
func InfoParameters() string {
	return internalmediainfo.InfoParameters()
}

// SetAppVersion sets the application version string used in rendered output.
func SetAppVersion(version string) {
	internalmediainfo.SetAppVersion(version)
}

// FormatVersion normalizes a version string for display and returns the formatted value.
func FormatVersion(version string) string {
	return internalmediainfo.FormatVersion(version)
}
