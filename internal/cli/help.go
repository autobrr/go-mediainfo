package cli

import (
	"fmt"
	"io"
)

func Help(program string, stdout io.Writer) {
	Version(stdout)
	fmt.Fprintf(stdout, "Usage: \"%s [-Options...] FileName1 [Filename2...]\"\n", program)
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "Options:")
	fmt.Fprintln(stdout, "--help, -h")
	fmt.Fprintln(stdout, "                    Display this help and exit")
	fmt.Fprintln(stdout, "--help-output")
	fmt.Fprintln(stdout, "                    Display help for output= option")
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "--full, -f")
	fmt.Fprintln(stdout, "                    Full information display (all internal tags)")
	fmt.Fprintln(stdout, "--output=TEXT|JSON|XML|OLDXML|HTML|CSV|EBUCore|EBUCore_JSON|PBCore|PBCore2|Graph_Svg|Graph_Dot")
	fmt.Fprintln(stdout, "                    Select output format")
	fmt.Fprintln(stdout, "--language=raw")
	fmt.Fprintln(stdout, "                    Display non-translated unique identifiers (internal text)")
	fmt.Fprintln(stdout, "--logfile=...")
	fmt.Fprintln(stdout, "                    Save the output in the specified file")
	fmt.Fprintln(stdout, "--bom")
	fmt.Fprintln(stdout, "                    Byte order mark for UTF-8 output (Windows only)")
	fmt.Fprintln(stdout, "--info-parameters")
	fmt.Fprintln(stdout, "                    Display list of inform= parameters")
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "Commands:")
	fmt.Fprintln(stdout, "version              Print MediaInfo version information")
	fmt.Fprintln(stdout, "update               Update mediainfo to latest version (release builds only)")
}

func HelpNothing(program string, stdout io.Writer) {
	fmt.Fprintf(stdout, "Usage: \"%s [-Options...] FileName1 [Filename2...]\"\n", program)
	fmt.Fprintf(stdout, "\"%s --help\" for displaying more information\n", program)
}

func HelpOutput(program string, stdout io.Writer) {
	fmt.Fprintln(stdout, "--output=...  Specify a template (BETA)")
	fmt.Fprintf(stdout, "Usage: \"%s --output=[xxx;]Text FileName\"\n", program)
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "xxx can be: General, Video, Audio, Text, Chapter, Image, Menu")
	fmt.Fprintln(stdout, "Text can be the template text, or a filename")
	fmt.Fprintln(stdout, "     Filename must be in the form file://filename")
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "See --info-parameters for available parameters in the text")
	fmt.Fprintln(stdout, "(Parameters must be surrounded by \"%\" sign)")
	fmt.Fprintln(stdout, "")
	fmt.Fprintf(stdout, "Usage: \"%s --output=Video;%%AspectRatio%% FileName\"\n", program)
	fmt.Fprintln(stdout, "")
	fmt.Fprintf(stdout, "Usage: \"%s --output=Video;file://Video.txt FileName\"\n", program)
	fmt.Fprintln(stdout, "and Video.txt contains ")
	fmt.Fprintln(stdout, "\"%DisplayAspectRatio%\"        for Video Aspect Ratio.")
	fmt.Fprintln(stdout, "")
	fmt.Fprintf(stdout, "Usage: \"%s --output=file://Text.txt FileName\"\n", program)
	fmt.Fprintln(stdout, "and Text.txt contains")
	fmt.Fprintln(stdout, "\"Video;%DisplayAspectRatio%\"  for Video Aspect Ratio.")
	fmt.Fprintf(stdout, "\"Audio;%%Format%%\"              for Audio Format.\n")
}

func Usage(program string, stdout io.Writer) int {
	HelpNothing(program, stdout)
	return exitError
}
