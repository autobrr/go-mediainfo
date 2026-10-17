# AGENTS

## Goal

go-mediainfo must be accurate. Official MediaInfo is the reference we diff against because it is usually right.

- A measured fact (a size, count, duration, bit rate, stream, metadata value, or error) must be correct. When official MediaInfo reports a wrong fact, fix the value in go-mediainfo. Do not port the bug. "MediaInfoLib does it this way" does not close a correctness issue.
- A reporting convention (rounding, frame-rate snapping, num/den display, units, the stream that General takes a value from) follows official v23.04. The reason and the rejected options are in `docs/adr/0001-measured-facts-vs-reporting-conventions.md`.
- Prove each deliberate difference with a real file before you ship it. Say why in the commit and pin it with a test.
- Keep the output shape compatible. Field names, JSON schema, text labels, and field order stay MediaInfo-compatible, because tools such as upbrr parse this output. Change values, not the schema.
- Pure Go, no CGO, cross-platform.
- Not implemented yet: ARIB/ISDB metadata and captions in TS, `.mpls` playlists, HDR10 and Dolby Vision metadata for HEVC in MP4, raw `.hevc` elementary streams.

## Workflow

- Loop to done: fix, verify, commit. Do not ask "continue?".
- Write the test first in `internal/mediainfo/*_test.go`. Run `gofmt -w` on touched `.go` files before you commit.
- A new parser gets a fuzz target in `internal/mediainfo/fuzz_parsers_test.go`. `scripts/fuzz.sh` discovers every `Fuzz*` function, so CI needs no other change.
- Verify a parser or formatter change against real files before you push, using either a verified local reference binary or the media host described below.

## Privacy

Never put release names or media-library paths in anything that lands on GitHub: issues, PRs, commits, this file. Refer to parity samples by ID. The ID to path map is `docs/agents/parity-samples.md`, kept out of git by `.git/info/exclude`.

## Parity harness

- Reference: official MediaInfo CLI with MediaInfoLib v23.04. Run the selected binary with `--Version` and verify that it reports `MediaInfoLib - v23.04` before comparing. A matching local binary on Windows, Linux, or macOS satisfies the reference requirement; it need not be on `PATH`. Do not substitute another version without explicitly recording and resolving the baseline difference.
- Local validation: invoke the verified reference binary by its full path and build go-mediainfo for the local platform. Compare both binaries against the same real files with the JSON and text options below. Record the reference version, sample IDs, commands, and results. Successful local validation satisfies the pre-push requirement; SSH access is not additionally required.
- Media host alternative: `root@media` is an environment-specific SSH destination, not a universal hostname or a prerequisite for local validation. When configured, connect with `ssh -o RemoteCommand=none -T root@media`. The `-o RemoteCommand=none -T` part is required for this route.
- Remote deployment only: `GOOS=linux GOARCH=amd64 go build -o /tmp/go-mediainfo-linux-amd64 ./cmd/mediainfo`, then copy the binary to `/tmp/go-mediainfo` on the host.
- Compare JSON: official `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` against `go-mediainfo --output=JSON --language=raw`. Normalize both with `jq -S 'del(.creatingLibrary)|del(.media.track[]?.File_Created_Date)|del(.media.track[]?.File_Created_Date_Local)'`, then `diff -u`.
- Compare text: official `mediainfo --ParseSpeed=0.5` against `go-mediainfo`, with the `ReportBy` line removed from ours.
- Media-host scripts and sample lists live outside the repo in `~/.local/share/go-mediainfo-parity/scripts/`. Local validation may use equivalent commands without this directory.
- The per-sample text-diff baseline and the known gaps per sample are in `docs/agents/parity-samples.md`. Update it when a commit moves a number. Do not copy it here.
- Disk politeness on the media host: `ionice -c3 nice -n10`, `timeout 300`, sample a few files per type, no full-tree scans. UHD and BDAV probes have stalled the host in kernel D state before.
- MediaInfoLib source is checked out at `~/github/oss/MediaInfoLib`. It is newer than the v23.04 binary. Read it to settle what official does. It is faster than running the binary and works offline.

## Deliberate differences from official v23.04

Do not change these back to official. Each one is a measured fact or a newer upstream convention.

- Measured fact: AAC `Errors: Missing ID_END` on sample `mkv-07`: official reports it, but its SBR parser overruns the fill element. Our walk consumes each frame bit-exactly and finds `ID_END`. Issue #34, PR #35.
- Newer upstream convention: `Encoded_Application_Name` and `Encoded_Application_Version` are split, as in newer MediaInfo.
- Measured fact: a Matroska cover attachment produces an Image track. Official v23.04 has none.
- Measured fact: Ogg FLAC General keeps `Writing application` and `Title` from the FLAC Vorbis comments. Official v23.04 drops them for Ogg FLAC only, and keeps them for standalone FLAC and Ogg Vorbis. Issue #55.
- Measured fact: Ogg FLAC General has no `Format/Info`. Official v23.04 shows the FLAC description there, but `Format/Info` describes the General format, which is Ogg, so the FLAC description is a wrong value. Issue #55.
- Measured fact: MP4 audio `SamplingCount` (AAC, AC-3, FLAC) comes from the exact track duration. Official v23.04 rounds the duration to milliseconds first, so a 44.1 kHz track of 132304 samples shows 132300. PR #59.
- Measured fact: Ogg with Theora and FLAC shows the measured video bit rate and stream size. FLAC has no nominal bit rate, and official v23.04 shows no video bit rate. PR #59.

## Agent skills

### Issue tracker

Issues live as GitHub issues in `autobrr/go-mediainfo`, managed with the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The six canonical triage roles, using the default label strings. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.
