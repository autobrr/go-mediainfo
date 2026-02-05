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

Note: Homebrew’s official MediaInfo install can conflict (`media-info` / `mediainfo`). If you have it installed, uninstall/unlink it first.

- Go install (requires Go toolchain):

```sh
go install github.com/autobrr/go-mediainfo/cmd/mediainfo@latest
```

- Latest release (one-liner, Linux x86_64):
  - Replace `linux_amd64` with `linux_arm64`, `darwin_amd64`, or `darwin_arm64` as needed.

```sh
curl -sL "$(curl -s https://api.github.com/repos/autobrr/go-mediainfo/releases/latest | grep browser_download_url | grep linux_amd64 | cut -d\" -f4)" | tar -xz -C /usr/local/bin
```

## Usage

Recommended (likely what you want):

```sh
mediainfo /path/to/file
```

```sh
mediainfo /path/to/file --Output=JSON
mediainfo /path/to/file --Full
mediainfo /path/to/dir
mediainfo --Info-Parameters
mediainfo --Info-CanHandleUrls
mediainfo --Version
mediainfo update
mediainfo version
```

Path is required (file or directory).

Default output: text.

## Options

- `-f, --Full` (complete report)
- `--Output=...` (TEXT/JSON/XML/HTML/CSV/EBUCore/PBCore/Graph)
- `--Language=...` (output language)
- `--LogFile=...` (write output to a file)
- `--BOM` (write UTF-8 BOM on Windows)
- `--Help`, `--Help-Output`
- `--Info-Parameters`, `--Info-CanHandleUrls`
- `--Version`
- `--self-update` / `--update` (update to latest release; release builds only)

## Commands

- `update` (same as `--self-update`)
- `version`
