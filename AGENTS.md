# AGENTS

## Goal

go-mediainfo must be accurate. Official MediaInfo is the reference we diff against because it is usually right.

- When official MediaInfo is wrong, fix the value in go-mediainfo. Do not port the bug. "MediaInfoLib does it this way" does not close a correctness issue.
- Prove each deliberate difference with a real file before you ship it. Say why in the commit and pin it with a test.
- Keep the output shape compatible. Field names, JSON schema, text labels, and field order stay MediaInfo-compatible, because tools such as upbrr parse this output. Change values, not the schema.
- Pure Go, no CGO, cross-platform.
- Not implemented yet: ARIB/ISDB metadata and captions in TS, `.mpls` playlists, HDR10 and Dolby Vision metadata for HEVC in MP4, raw `.hevc` elementary streams.

## Workflow

- Loop to done: fix, verify, commit. Do not ask "continue?".
- Write the test first in `internal/mediainfo/*_test.go`. Run `gofmt -w` on touched `.go` files before you commit.
- A new parser gets a fuzz target in `internal/mediainfo/fuzz_parsers_test.go`, wired into the CI fuzz smoke in `.github/workflows/ci.yml` and the scheduled run in `.github/workflows/fuzz.yml`.
- Verify a parser or formatter change against real files on the media host before you push.

## Privacy

Never put release names or media-library paths in anything that lands on GitHub: issues, PRs, commits, this file. Refer to parity samples by ID. The ID to path map is `docs/agents/parity-samples.md`, kept out of git by `.git/info/exclude`.

## Parity harness

- Reference: `mediainfo` (MediaInfoLib v23.04) on the media host. Connect with `ssh -o RemoteCommand=none -T root@media`. The `-o RemoteCommand=none -T` part is required.
- Deploy: `GOOS=linux GOARCH=amd64 go build -o /tmp/go-mediainfo-linux-amd64 ./cmd/mediainfo`, then copy the binary to `/tmp/go-mediainfo` on the host.
- Compare JSON: official `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` against `go-mediainfo --output=JSON --language=raw`. Normalize both with `jq -S 'del(.creatingLibrary)|del(.media.track[]?.File_Created_Date)|del(.media.track[]?.File_Created_Date_Local)'`, then `diff -u`.
- Compare text: official `mediainfo --ParseSpeed=0.5` against `go-mediainfo`, with the `ReportBy` line removed from ours.
- Scripts and sample lists live outside the repo in `~/.local/share/go-mediainfo-parity/scripts/`.
- The per-sample text-diff baseline and the known gaps per sample are in `docs/agents/parity-samples.md`. Update it when a commit moves a number. Do not copy it here.
- Disk politeness on the media host: `ionice -c3 nice -n10`, `timeout 300`, sample a few files per type, no full-tree scans. UHD and BDAV probes have stalled the host in kernel D state before.
- MediaInfoLib source is checked out at `~/github/oss/MediaInfoLib`. It is newer than the v23.04 binary. Read it to settle what official does. It is faster than running the binary and works offline.

## Deliberate differences from official v23.04

Do not change these back to official.

- AAC `Errors: Missing ID_END` on sample `mkv-07`: official reports it, but its SBR parser overruns the fill element. Our walk consumes each frame bit-exactly and finds `ID_END`. Issue #34, PR #35.
- `Encoded_Application_Name` and `Encoded_Application_Version` are split. This is newer MediaInfo behavior, kept on purpose.
- A Matroska cover attachment produces an Image track. Official v23.04 has none.

## Agent skills

### Issue tracker

Issues live as GitHub issues in `autobrr/go-mediainfo`, managed with the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles, using the default label strings. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.
