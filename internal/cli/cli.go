package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	mediainfo "github.com/autobrr/go-mediainfo"
)

const (
	exitOK    = 0
	exitError = 1
)

type Options struct {
	Full        bool
	Output      string
	Language    string
	LogFile     string
	Bom         bool
	CoreOptions []CoreOption
}

type CoreOption struct {
	Name  string
	Value string
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return exitError
	}

	program := programName(args[0])
	var opts Options
	var files []string

	for i := 1; i < len(args); i++ {
		original := args[i]
		normalized := normalizeArg(original)

		switch {
		case normalized == "--full" || normalized == "-f":
			opts.Full = true
		case normalized == "--version":
			Version(stdout)
			return exitOK
		case normalized == "--help" || normalized == "-h":
			Help(program, stdout)
			return exitOK
		case strings.HasPrefix(normalized, "--help-"):
			return helpTopic(normalized, program, stdout)
		case normalized == "--info-parameters":
			fmt.Fprintln(stdout, mediainfo.InfoParameters())
			return exitOK
		case strings.HasPrefix(normalized, "--language"):
			if value, ok := valueAfterEqual(original); ok {
				opts.Language = value
			}
		case strings.HasPrefix(normalized, "-lang="):
			if value, ok := valueAfterEqual(original); ok {
				opts.Language = value
			}
		case strings.HasPrefix(normalized, "--output="):
			if value, ok := valueAfterEqual(original); ok {
				opts.Output = value
			} else {
				HelpOutput(program, stdout)
				return exitError
			}
		case strings.HasPrefix(normalized, "--output"):
			files = append(files, original)
		case strings.HasPrefix(normalized, "--logfile"):
			opts.LogFile = valueAfterLogfile(original)
		case normalized == "--bom":
			opts.Bom = true
		case strings.HasPrefix(normalized, "--"):
			if normalized == "--" {
				continue
			}
			name, value := parseCoreOption(normalized, original)
			if name == "" {
				continue
			}
			opts.CoreOptions = append(opts.CoreOptions, CoreOption{Name: name, Value: value})
		default:
			files = append(files, original)
		}
	}

	if len(files) == 0 {
		return Usage(program, stdout)
	}

	if opts.Bom {
		writeBOM(stdout, stderr)
	}

	output, filesCount, err := runCore(opts, files)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}

	if output != "" {
		fmt.Fprintln(stdout, output)
	}

	if opts.LogFile != "" {
		if err := writeLogFile(opts.LogFile, output, opts.Bom); err != nil {
			fmt.Fprintln(stderr, err)
			return exitError
		}
	}

	if filesCount > 0 {
		return exitOK
	}

	return exitError
}

func helpTopic(normalized, program string, stdout io.Writer) int {
	if normalized == "--help-output" || normalized == "--help-inform" {
		HelpOutput(program, stdout)
		return exitOK
	}

	fmt.Fprintln(stdout, "No help available yet")
	return exitOK
}

func programName(arg0 string) string {
	name := filepath.Base(arg0)
	if runtime.GOOS == "windows" {
		ext := filepath.Ext(name)
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

func normalizeArg(arg string) string {
	eq := strings.IndexByte(arg, '=')
	if eq == -1 {
		eq = len(arg)
	}

	lower := strings.ToLower(arg[:eq])
	return lower + arg[eq:]
}

func valueAfterEqual(arg string) (string, bool) {
	_, after, ok := strings.Cut(arg, "=")
	if !ok {
		return "", false
	}
	return after, true
}

func valueAfterLogfile(arg string) string {
	if len(arg) <= 10 {
		return ""
	}
	return arg[10:]
}

func parseCoreOption(normalized, original string) (string, string) {
	eq := strings.IndexByte(normalized, '=')
	if eq == -1 {
		name := strings.TrimPrefix(normalized, "--")
		return name, "1"
	}

	name := strings.TrimPrefix(normalized[:eq], "--")
	return name, original[eq+1:]
}

func writeBOM(stdout, stderr io.Writer) {
	if runtime.GOOS != "windows" {
		return
	}

	bom := []byte{0xEF, 0xBB, 0xBF}
	_, _ = stdout.Write(bom)
	_, _ = stderr.Write(bom)
}

func writeLogFile(path, output string, includeBOM bool) error {
	data := []byte(output)
	if includeBOM && runtime.GOOS == "windows" {
		data = append([]byte{0xEF, 0xBB, 0xBF}, data...)
	}

	return os.WriteFile(path, data, 0o644) //nolint:gosec // user-facing output file
}

func runCore(opts Options, files []string) (string, int, error) {
	outputFormat := mediainfo.OutputText
	if opts.Output != "" {
		if strings.Contains(opts.Output, ";") || strings.HasPrefix(strings.ToLower(opts.Output), "file://") {
			return "", 0, fmt.Errorf("output template not implemented: %s", opts.Output)
		}

		outputFormat = mediainfo.OutputFormat(strings.ToUpper(strings.TrimSpace(opts.Output)))
		switch outputFormat {
		case mediainfo.OutputText, mediainfo.OutputJSON, mediainfo.OutputXML, mediainfo.OutputOldXML, mediainfo.OutputHTML, mediainfo.OutputCSV,
			mediainfo.OutputEBUCore, mediainfo.OutputEBUCoreJSON, mediainfo.OutputPBCore, mediainfo.OutputPBCore2, mediainfo.OutputGraphSVG, mediainfo.OutputGraphDOT:
		default:
			return "", 0, fmt.Errorf("output format not implemented: %s", opts.Output)
		}
	}

	var analyzeOpts []mediainfo.AnalyzeOption
	for _, opt := range opts.CoreOptions {
		if strings.EqualFold(opt.Name, "parsespeed") {
			if value, err := strconv.ParseFloat(strings.TrimSpace(opt.Value), 64); err == nil {
				analyzeOpts = append(analyzeOpts, mediainfo.WithParseSpeed(value))
			}
			continue
		}
		if strings.EqualFold(opt.Name, "file_testcontinuousfilenames") {
			value := strings.TrimSpace(opt.Value)
			analyzeOpts = append(analyzeOpts, mediainfo.WithTestContinuousFileNames(value != "0"))
			continue
		}
	}
	reports, count, err := mediainfo.AnalyzeFilesWithCount(files, analyzeOpts...)
	if err != nil {
		return "", 0, err
	}

	output, err := mediainfo.Render(reports, outputFormat)
	if err != nil {
		return "", 0, err
	}
	return output, count, nil
}
