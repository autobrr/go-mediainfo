package cli

import (
	"fmt"
	"io"

	"github.com/autobrr/go-mediainfo/internal/mediainfo"
)

func Version(stdout io.Writer) {
	fmt.Fprintf(stdout, "MediaInfo Command line, %s\n", mediainfo.MediaInfoLibVersion)
}
