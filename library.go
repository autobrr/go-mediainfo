package mediainfo

import (
	"fmt"
	"strings"

	core "github.com/autobrr/go-mediainfo/internal/mediainfo"
)

type StreamKind = core.StreamKind

const (
	// StreamGeneral is the container-level stream.
	StreamGeneral StreamKind = core.StreamGeneral
	// StreamVideo is a video stream.
	StreamVideo StreamKind = core.StreamVideo
	// StreamAudio is an audio stream.
	StreamAudio StreamKind = core.StreamAudio
	// StreamText is a subtitle/caption stream.
	StreamText StreamKind = core.StreamText
	// StreamImage is an image stream.
	StreamImage StreamKind = core.StreamImage
	// StreamMenu is a menu/chapters stream.
	StreamMenu StreamKind = core.StreamMenu
)

// Field represents one MediaInfo field/value pair.
type Field = core.Field

// Stream represents one parsed stream (General/Video/Audio/Text/Menu).
type Stream = core.Stream

// Report is the analysis result for one input path.
type Report = core.Report

// AnalyzeOption configures parsing behavior.
type AnalyzeOption func(*core.AnalyzeOptions)

// WithParseSpeed sets ParseSpeed (0..1). If unset, default is 0.5.
func WithParseSpeed(value float64) AnalyzeOption {
	return func(opts *core.AnalyzeOptions) {
		opts.ParseSpeed = value
		opts.HasParseSpeed = true
	}
}

// WithTestContinuousFileNames enables MediaInfo-style continuous file probing.
func WithTestContinuousFileNames(enabled bool) AnalyzeOption {
	return func(opts *core.AnalyzeOptions) {
		opts.TestContinuousFileNames = enabled
		opts.HasTestContinuousFileNames = true
	}
}

// AnalyzeFile analyzes one file or directory path.
func AnalyzeFile(path string, options ...AnalyzeOption) (Report, error) {
	return core.AnalyzeFileWithOptions(path, buildAnalyzeOptions(options))
}

// AnalyzeFiles analyzes multiple paths.
func AnalyzeFiles(paths []string, options ...AnalyzeOption) ([]Report, error) {
	reports, _, err := core.AnalyzeFilesWithOptions(paths, buildAnalyzeOptions(options))
	return reports, err
}

// AnalyzeFilesWithCount analyzes multiple paths and returns report count.
func AnalyzeFilesWithCount(paths []string, options ...AnalyzeOption) ([]Report, int, error) {
	return core.AnalyzeFilesWithOptions(paths, buildAnalyzeOptions(options))
}

// OutputFormat is a renderer output type.
type OutputFormat string

const (
	// OutputText renders text output.
	OutputText OutputFormat = "TEXT"
	// OutputJSON renders JSON output.
	OutputJSON OutputFormat = "JSON"
	// OutputXML renders XML output.
	OutputXML OutputFormat = "XML"
	// OutputOldXML renders old XML output.
	OutputOldXML OutputFormat = "OLDXML"
	// OutputHTML renders HTML output.
	OutputHTML OutputFormat = "HTML"
	// OutputCSV renders CSV output.
	OutputCSV OutputFormat = "CSV"
	// OutputEBUCore renders EBUCore output.
	OutputEBUCore OutputFormat = "EBUCORE"
	// OutputEBUCoreJSON renders EBUCore JSON output.
	OutputEBUCoreJSON OutputFormat = "EBUCORE_JSON"
	// OutputPBCore renders PBCore output.
	OutputPBCore OutputFormat = "PBCORE"
	// OutputPBCore2 renders PBCore2 output.
	OutputPBCore2 OutputFormat = "PBCORE2"
	// OutputGraphSVG renders Graph SVG output.
	OutputGraphSVG OutputFormat = "GRAPH_SVG"
	// OutputGraphDOT renders Graph DOT output.
	OutputGraphDOT OutputFormat = "GRAPH_DOT"
)

// Render renders reports with the selected format.
func Render(reports []Report, format OutputFormat) (string, error) {
	switch normalizeOutputFormat(format) {
	case "", OutputText:
		return core.RenderText(reports), nil
	case OutputJSON:
		return core.RenderJSON(reports), nil
	case OutputXML, OutputOldXML:
		return core.RenderXML(reports), nil
	case OutputHTML:
		return core.RenderHTML(reports), nil
	case OutputCSV:
		return core.RenderCSV(reports), nil
	case OutputEBUCore, OutputEBUCoreJSON:
		return core.RenderEBUCore(reports), nil
	case OutputPBCore, OutputPBCore2:
		return core.RenderPBCore(reports), nil
	case OutputGraphSVG:
		return core.RenderGraphSVG(reports), nil
	case OutputGraphDOT:
		return core.RenderGraphDOT(reports), nil
	default:
		return "", fmt.Errorf("output format not implemented: %s", format)
	}
}

// InfoParameters returns the supported info parameter listing.
func InfoParameters() string {
	return core.InfoParameters()
}

// SetAppVersion sets the version shown in rendered metadata.
func SetAppVersion(version string) {
	core.SetAppVersion(version)
}

// FormatVersion normalizes the version label to MediaInfo style.
func FormatVersion(version string) string {
	return core.FormatVersion(version)
}

func buildAnalyzeOptions(options []AnalyzeOption) core.AnalyzeOptions {
	opts := core.AnalyzeOptions{}
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}
	return opts
}

func normalizeOutputFormat(format OutputFormat) OutputFormat {
	return OutputFormat(strings.ToUpper(strings.TrimSpace(string(format))))
}
