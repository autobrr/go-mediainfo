package cli

import (
	"fmt"
	"io"
)

const VersionString = "0.0.0-dev"

func Version(stdout io.Writer) {
	fmt.Fprintf(stdout, "MediaInfo Command line, %s\n", VersionString)
}
