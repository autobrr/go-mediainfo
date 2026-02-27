```
███╗   ███╗███████╗██████╗ ██╗ █████╗ ██╗███╗   ██╗███████╗ ██████╗
████╗ ████║██╔════╝██╔══██╗██║██╔══██╗██║████╗  ██║██╔════╝██╔═══██╗
██╔████╔██║█████╗  ██║  ██║██║███████║██║██╔██╗ ██║█████╗  ██║   ██║
██║╚██╔╝██║██╔══╝  ██║  ██║██║██╔══██║██║██║╚██╗██║██╔══╝  ██║   ██║
██║ ╚═╝ ██║███████╗██████╔╝██║██║  ██║██║██║ ╚████║██║     ╚██████╔╝
╚═╝     ╚═╝╚══════╝╚═════╝ ╚═╝╚═╝  ╚═╝╚═╝╚═╝  ╚═══╝╚═╝      ╚═════╝
```

Go rewrite of MediaInfo CLI.

## Installation

- Homebrew (macOS):

```sh
brew tap autobrr/go-mediainfo https://github.com/autobrr/go-mediainfo
brew install --cask autobrr/go-mediainfo/mediainfo
```

Note: Homebrew’s official MediaInfo install can conflict (`media-info` / `mediainfo`). This cask also links `go-mediainfo` so you can run both:

```sh
go-mediainfo version
```

- Go install (requires Go toolchain):

```sh
go install github.com/autobrr/go-mediainfo/cmd/mediainfo@latest
```

- Latest release (one-liner, Linux x86_64):
  - Replace `linux_amd64` with `linux_arm64`, `darwin_amd64`, or `darwin_arm64` as needed.

```sh
curl -sL "$(curl -s https://api.github.com/repos/autobrr/go-mediainfo/releases/latest | grep browser_download_url | grep linux_amd64 | cut -d\" -f4)" | tar -xz -C /usr/local/bin
```

## Library Usage (Go)

```sh
go get github.com/autobrr/go-mediainfo@latest
```

```go
import (
	"fmt"

	mediainfo "github.com/autobrr/go-mediainfo"
)

func main() {
	reports, err := mediainfo.AnalyzeFiles(
		[]string{"movie.mkv"},
		mediainfo.WithParseSpeed(0.5),
	)
	if err != nil {
		panic(err)
	}

	out, err := mediainfo.Render(reports, mediainfo.OutputJSON)
	if err != nil {
		panic(err)
	}
	fmt.Println(out)
}
```

## Usage

Recommended (likely what you want):

```sh
mediainfo /path/to/file
```

```sh
mediainfo /path/to/file --Output=JSON --Language=raw
mediainfo /path/to/file --Output=JSON --Language=raw --ParseSpeed=0.5
mediainfo /path/to/dir
mediainfo --Info-Parameters
mediainfo --Version
mediainfo update
mediainfo version
```

Path is required (file or directory).

Default output: text.
Footer includes `ReportBy : go-mediainfo - vX.Y.Z`.

## Options

- `--Output=...` (TEXT/JSON/XML/OLDXML/HTML/CSV/EBUCore/EBUCore_JSON/PBCore/PBCore2/Graph_Svg/Graph_Dot)
- `--Language=raw` (non-translated unique identifiers; recommended for parity)
- `-lang=raw` (alias for `--Language=raw`)
- `--LogFile=...` (write output to a file)
- `--BOM` (write UTF-8 BOM on Windows)
- `--ParseSpeed=0..1` (speed/accuracy tradeoff; default `0.5`)
- `--File_TestContinuousFileNames=0|1` (MediaInfo-style continuous filename probing; default `0`)
- `--Help`, `--Help-Output`
- `--Info-Parameters`
- `-f, --Full` (reserved; currently no-op)

## Commands

- `update` (self-update this binary; release builds only)
- `version` (print go-mediainfo version)

## License

GPL-2.0-or-later. See [LICENSE](LICENSE).
