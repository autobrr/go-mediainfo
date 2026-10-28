# AGENTS

## Goal

go-mediainfo must be accurate. Official MediaInfo is the reference we diff against because it is usually right.

- A measured fact (a size, count, duration, bit rate, stream presence, metadata value, or error) must be correct. When official MediaInfo reports a wrong fact, fix the value in go-mediainfo. Do not port the bug. "MediaInfoLib does it this way" does not close a correctness issue.
- A reporting convention (rounding, frame-rate snapping, num/den display, units, names, the stream that General takes a value from) follows official v26.10 for DVD output and v23.04 otherwise. See `docs/adr/0001-parity-baseline-and-output-scope.md` and its DVD-specific successor, `docs/adr/0002-dvd-parity-reference.md`.
- Prove each deliberate difference with a real file before you ship it. Say why in the commit and pin it with a test.
- Keep the output shape compatible. Field names, JSON schema, text labels, and field order come from official MediaInfo, because tools such as upbrr parse this output. A field comes from v23.04, or from v26.10 where ADR 0001 or the DVD-specific ADR 0002 allows it. Never invent a field or a label.
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

- Reference: official MediaInfo CLI with MediaInfoLib v26.10 for all DVD output, and v23.04 otherwise. Run the selected binary with `--Version` and verify the applicable version before comparing. Record its official artifact URL, platform, SHA-256, runtime version and applicable source revision as described in ADR 0002. A matching local binary on Windows, Linux, or macOS satisfies the reference requirement; it need not be on `PATH`. Do not substitute another version without explicitly recording and resolving the baseline difference.
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

Do not change these back to official. Each line is a measured fact, a newer upstream convention, or a safety rule, except the specific maintainer-approved DVD gap convention in ADR 0002. Before you add a line, read `docs/adr/0001-parity-baseline-and-output-scope.md`: it sets when v26.10 behavior wins and which outputs count.

DVD uses the reference in `docs/adr/0002-dvd-parity-reference.md`. Keep evidence-backed corrections and safety rules even when that reference disagrees.

- Safety: hostile input gets the limits, omissions and escaping in the "Embedded metadata safety" section of `README.md`. These protect against denial of service and output injection, and `output_safety_test.go` pins them.
- Newer upstream convention: all DVD output uses the v26.10 reference, including title-set joining and IFO reports. v26.10 preserves IFO stream languages after VOB merge. Wrong upstream facts are still corrected with evidence and tests; this is a reference decision, not a claim of completed parity. ADR 0002, issue #248.
- Maintainer-approved convention: for incomplete DVD title sets, continuation retains existing numbered members after a gap. v26.10 sometimes stops before a gap and sometimes selects later members; its sizes are accurate for its selection. Keep go-mediainfo's existing selection rather than reproducing that inconsistent convention. ADR 0002, issue #248.
- Measured fact: MPEG-PS RLE `Duration` is omitted when resets or ambiguous 33-bit PTS steps do not establish a coherent timeline, for DVD and generic MPEG-PS. Verified v23.04 and v26.10 binaries can report unsupported long spans for crafted forward jumps. Known-invalid RLE timing also cannot supply container duration or its global fallback, or infer video duration/frame count without independent timing evidence; overall bitrate follows the remaining supported duration. Keep ordinary spans and continuously observed wraps. PR #249.
- Measured fact: AAC `Errors: Missing ID_END` on sample `mkv-07`: official reports it, but its SBR parser overruns the fill element. Our walk consumes each frame bit-exactly and finds `ID_END`. Issue #34, PR #35.
- Newer upstream convention: `Encoded_Application_Name` and `Encoded_Application_Version` are split, as in newer MediaInfo. The Matroska General `Encoded_Library` (MuxingApp) is split into name and version the same way. Issue #83.
- Measured fact: a Matroska cover attachment produces an Image track, and General shows `Cover_Description`, `Cover_Mime`, `Cover_Type` and `ImageCount`. Official v23.04 has none.
- Measured fact: Ogg FLAC General keeps `Writing application` and `Title` from the FLAC Vorbis comments. Official v23.04 drops them for Ogg FLAC only, and keeps them for standalone FLAC and Ogg Vorbis. Issue #55.
- Measured fact: Ogg FLAC General has no `Format/Info`. Official v23.04 shows the FLAC description there, but `Format/Info` describes the General format, which is Ogg, so the FLAC description is a wrong value. Issue #55.
- Measured fact: MP4 audio `SamplingCount` (AAC, AC-3, FLAC) comes from the exact track duration. Official v23.04 rounds the duration to milliseconds first, so a 44.1 kHz track of 132304 samples shows 132300. PR #59.
- Measured fact: Ogg with Theora and FLAC shows the measured video bit rate and stream size. FLAC has no nominal bit rate, and official v23.04 shows no video bit rate. PR #59.
- Newer upstream convention: HDR fields have the v26 form. JSON `MaxCLL` and `MaxFALL` are numbers with no unit, raw text uses `MaxCLL/String` and `MaxFALL/String`, and Dolby Vision text shows the profile as "8.1". A number is the better JSON value, and "8.1" is the real profile name. Issue #67.
- Measured fact: HDR fields that v23.04 does not fill: `MasteringDisplay_Luminance_Min`, `MasteringDisplay_Luminance_Max`, `HDR_Format_Compression`, and `HDR_Format_Compatibility` on MP4. Issue #67.
- Measured fact: AC-3 metadata (dialnorm, compr, dynrng and the others) shows in text, and JSON has `cmixlev`, `surmixlev`, `mixlevel`, `roomtyp`, `dmixmod`, the `loro` and `ltrt` mix levels, `dialnorm_Count` and `adconvtyp`. v23.04 hides these values, and v26 shows them. Issue #78.
- Newer upstream convention: aspect ratio names `2.39:1` (v23.04 shows `2.40:1` for 2.37 to 2.45) and `9:16`. These are the standard names. Issue #68.
- Newer upstream convention: titles are not joined as "X / X" when the two values are equal, and spaces at the start and the end are trimmed. Issue #73.
- Measured fact: `vp09` in MP4 shows as VP9, and Matroska text shows AV1 film grain. v23.04 shows the raw code and skips the film grain line. Issue #71.
- Measured fact: `ServiceKind` comes from the Matroska hearing impaired, original and commentary flags. v23.04 does not read the flags, and v26.05 does. Issue #75.
- Measured fact: the Matroska tag `ENCODER_OPTIONS` shows as `Encoded_Library_Settings`, and `DATE`, `ARTIST`, `GENRE` and `SYNOPSIS` map to `Recorded_Date`, `Performer`, `Genre` and `Synopsis`. v23.04 shows the raw tag names, and v26 maps them. Issue #83.
- Newer upstream convention: DVD IFO stream IDs use the "189-128" form, which names the private stream and the sub-stream, as v26.05 does. Issue #77.
- Measured fact: DVD menus. go-mediainfo makes one Menu per title and drops a menu shorter than half of the longest one (v26.05 rule). It reads all program chains where v23.04 has a parse error, and it adds NTSC cell times in exact 90 kHz ticks before it rounds. Raw text subtitle labels follow the newer upstream convention, the v26 short form. Issue #79.
- Measured fact: TS captions show `Duration_End` and `Duration_Start2End` from the caption timestamps, as in v26. Issue #83.

## Agent skills

### Issue tracker

Issues live as GitHub issues in `autobrr/go-mediainfo`, managed with the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The six canonical triage roles, using the default label strings. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.
