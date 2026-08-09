# AGENTS

Owner: soup

## Scope
- Goal: be better than MediaInfo CLI. Parity is the test method, not the target.
  - Diffing against official MediaInfo is how we find our bugs, because it is usually right.
  - Where official is wrong or gives up, fixing it is the point. "MediaInfoLib does it this way" is not a reason to close an improvement.
  - Prove any deliberate difference with a real file before shipping it, and say why in the commit.
- Pure Go implementation, no CGO
- Cross-platform

## Workflow
- Don't ask for "continue": run parity + perf loops automatically, then commit/push once verified.
- Before committing: run `gofmt -w` on touched `.go` files.
- For `/mnt/storage/...` scans: be polite to disks (sample files; avoid full-tree scans; use `ionice -c3 nice -n 10` when doing bulk comparisons).
- Never put release names or media-library paths in anything that lands on GitHub (issues, PRs, commits, this file). Refer to parity samples by ID; the ID -> path map is `docs/agents/parity-samples.md`, kept local-only via `.git/info/exclude`.

## Status (2026-08-10)
- Follow-up to the 2026-08-09 text-gap pass: fixed almost all remaining pre-existing Matroska default-text gaps.
- Root-cause find: Matroska `DateUTC` must be decoded like MediaInfoLib (unsigned ns since 2001, Unix seconds truncated to 32 bits). mkvmerge `no_variable_data` files wrap to `2010-02-22 21:41:29`; our signed decode gave `1970-01-01`, which made the Statistics Tags date comparison pass and trust stats official rejects.
- Untrusted stats now mirror `File_Mk.cpp`: lexicographic `_STATISTICS_WRITING_DATE_UTC >= Encoded_Date` compare, per-track `Statistics Tags Issue` (`<statsApp> <statsDate> / <writingApp> <encodedDate>`), `FromStats_*` per `_STATISTICS_TAGS` list order, all also emitted in default text.
- Also fixed: `Delay relative to video` text for mkv audio (hidden at 0 ms, like official); generic `Bits/(Pixel*Frame)` pass (BitRate, falling back to BitRate_Nominal); HE-AAC SBR text (`AAC LC SBR`, long Format/Info, `Format settings: Explicit`); DTS ES XCh text (`DTS ES XCh` + `Channel(s)_Original`/`ChannelLayout_Original` lines); `Unique ID` hex without leading zeros (`0 (0x0)`); x/1 frame-rate ratios display plain (`25.000 FPS`, not `25.000 (25/1) FPS`); per-stream text tail order now matches the official schema (Writing library -> Encoding settings -> Language -> Service kind -> Default/Forced -> dates -> extras -> colour); removed mkv AC-3 `ltrt*/loro*` extras official never reports.
- Verified vs official v23.04 on the media host: 10-file real mkv set (samples `mkv-01`..`mkv-10`) all at text diff `0` except `mkv-07` (`Errors: Missing ID_END` only); JSON matches official except pre-existing `Encoded_Application_Name/Version` split (newer-MediaInfo behavior we keep) and env-only `File_Created_Date`. TS/mp4 outputs byte-identical to main.
- Known remaining: `Errors: Missing ID_END` tracked in issue #34; `Count of elements` on tiny synthetic text tracks; TS controls have drifted vs official on main (pre-existing, unrelated to Matroska work; e.g. `ts-01` default-text diff `41` for both main and this branch).

## Status (2026-08-09)
- Default TEXT output (what upbrr posts) was missing Matroska metadata that official shows: General `extra` tags (IMDB/TMDB/TVDB, any dynamic tag), `Attachments`, `Cover`, track-scoped tags (FILENAME/MIMETYPE), and `Movie name`. Reported by a darkpeers moderator (uploads flagged as modified). Never worked in any commit; JSON and `--language=raw` were always complete.
- Fixed on this pass: dynamic tags + Attachments + Cover now emitted as display fields; General `Title` renders as `Movie name` after tag merge; `Movie name`/`Encoded date`/`Tagged date` now order before `Writing application` (official template order); ASS/SSA `Codec ID/Info`; Matroska AAC text frame rate 4 -> 3 decimals (`46.875 FPS`).
- Verified vs official v23.04 on root@media: synthetic tag/attachment mkvs now text-identical (sans our `ReportBy` trailer + our Image track for covers, which official v23.04 lacks); 9 real mkv sweep `improved=9 worse=0`; mp4/ts text and all JSON byte-identical to main.
- Known remaining text gaps (pre-existing, separate work): `Statistics Tags Issue`/`FromStats_*` text lines, fake stats-date handling (`no_variable_data` -> Encoded date), HE-AAC SBR detection on some files, `Delay relative to video`, `Bits/(Pixel*Frame)` rounding on one sample, `Count of elements` emitted for tiny text tracks where official omits, mkv `Nominal bit rate` vs `Bit rate` labeling with stats tags.

## Status (2026-02-16)
- CI: `gofmt` clean; `go test ./...` green.
- Parity target: official `/usr/bin/mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` (MediaInfoLib v23.04).
- TS parity controls: `Nickelodeon - Generic Halloween Promo.ts=0`, `Nickelodeon - Saturday Morning Promo.ts=0`, `Disney Channel - Evermoor Behind The Scenes.ts=0`.
- BDAV/M2TS parity spot checks now `0` on user discs:
  - `Static .../BDMV/STREAM/00004.m2ts`
  - `Intruders .../BDMV/STREAM/00061.m2ts`
  - `Sabrina .../BDMV/STREAM/00003.m2ts`
  - `Network UHD .../BDMV/STREAM/00005.m2ts` (HEVC derived `StreamSize`/`BitRate` + General remainder math)
  - `Network UHD .../BDMV/STREAM/00007.m2ts` (AC-3 dynrng_* suppression for mono)
  - `Excalibur UHD .../BDMV/STREAM/00004.m2ts` (bounded tail window selection + long-duration rounding)
- BDAV bounded ParseSpeed behavior aligned closer to MediaInfoLib:
  - PCR head shrink threshold: ~30s for BDAV (vs ~16.8s for TS captures); tail window uses head-sized end window when shrunk, else uses 16 MiB.
  - Long BDAV video Duration (`>= 3600s`): JSON uses millisecond truncation to match official 0.001s rounding on long clips.
  - AVC `Format_Settings_GOP`: emitted for BDAV only when duration window is stable (`>= 120s`), to avoid false-positive GOP on short clips.

## Status (2026-02-09)
- CI: green on `main` (recent runs: MP4/AVI/MPEG-PS fixes).
- Parity: verified 1:1 JSON raw vs `/usr/bin/mediainfo` (MediaInfoLib v23.04) for:
- Matroska samples (incl. 4K sample)
- MP4 sample
- VOB sample (`VTS_01_2.vob`) after `fix(mpeg-ps): improve VOB parity`

## Status (2026-02-11)
- CI: green locally (`go test ./...`, `gofmt -w` clean).
- PTP helper: `scripts/ptp_scan.go` supports `-min-seeders`; latest conservative scan (`4` requests, `35s` min interval) returned `13` candidates at `seeders>=10` to `.cache/ptp-scan/candidates.jsonl`.
- Focused parity samples (official: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
- TS `Nickelodeon - Generic Halloween Promo.ts`: diff `12` (down from `14` this pass; GOP JSON parity improved).
- TS `Disney Channel - Evermoor Behind The Scenes.ts`: diff `25` (remaining: AC-3 stats/count windows).
- M2TS `01007.m2ts`: diff `8` (remaining: AC-3 dynrng stats + 1-byte StreamSize split).
- M2TS `01099.m2ts`: diff `4` (remaining: 2-byte General/Video StreamSize split).
- Non-TS spot checks: `mp4/mp3/flac/avi` at `0`; one sampled `mkv` at `1`.
- Extended TS sweep (`/mnt/storage/torrents/keepalive/Halloween 2014 Promos and Bumpers Pack`, 12-file sample):
- Diff range `13..64`; recurring gaps are AC-3 `dynrng_*` count windows, MPEG-2 `Format_Settings_Matrix`/`intra_dc_precision`, caption service mapping (extra/misaligned text tracks), and missing General `Title`/`Movie` on some captures.
- GOP experiment note: forcing `Variable` on interlaced TS improves some Cartoon Network captures but regresses Nick captures; reverted for now.
- `ptp-archive` spot checks (ParseSpeed=0.5): `VTS_01_0.IFO=0`, `VTS_01_1.VOB=28`, `KAUTOKEINO 00011.m2ts: 62->27`, `KAUTOKEINO 00012.m2ts: 66->27` after BDAV MPEG-2 field parity expansion.
- Remaining highest-impact work:
- TS/BDAV AC-3 stats parity (`compr_*`/`dynrng_*` counts + averaging windows).
- BDAV/TS tiny General vs Video `StreamSize` byte-allocation parity (1-3 byte deltas).

## Status (2026-02-12)
- New samples checked:
- `.../David Bowie and the Story of Ziggy Stardust ... .ts` diff `107`.
- `.../Now You See Me ... /BDMV/STREAM/00000.m2ts` diff `400`.
- `.../Now You See Me ... /BDMV/PLAYLIST/00100.mpls` diff `27146`.
- `.../Infernal.../STREAM/00000.m2ts` diff `272`; `.../PLAYLIST/00999.mpls` diff `13468`.
- `.../Intruders.../STREAM/50201.m2ts` diff `507`; `.../PLAYLIST/00020.mpls` diff `6699`.
- `.../Network.../STREAM/00007.m2ts` diff `214`; `.../PLAYLIST/00002.mpls` diff `1390`.
- Additional availability sweep (non-cross-seed canonical paths):
- `A.Beautiful.Mind.../STREAM/00068.m2ts` diff `312`; `.../PLAYLIST/00020.mpls` diff `3091`.
- `Now You See Me.../STREAM/00000.m2ts` diff `406`; `.../PLAYLIST/00101.mpls` diff `27074`.
- `Through.the.Looking.Glass.../STREAM/00003.m2ts` diff `164`; `.../PLAYLIST/00001.mpls` diff `1159`.
- Continuous filename parity:
- `detectContinuousFileSet` now includes sparse numeric sets (not just strict contiguous sequence) and picks highest numbered matching file.
- `detectContinuousFileSet` sparse matching retained for explicit `--File_TestContinuousFileNames=1` flows; default remains off.
- TS caption parser parity win:
- Fixed CEA-708 DTVCC packet payload sizing in `internal/mediainfo/mpeg_ts_mpeg2.go` (`packet_size_code` handling): payload bytes are `127` when code `0`, else `(code*2)-1`.
- Validation (official `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
- 71-file TS sweep (`keepalive/Halloween...`): `improved=6 same=65 worse=0`.
- Largest wins: `New Thursdays 169->144`, `Generic Week of Eeek 135->99`, `Regular Show IV Promo 130->94`.
- Control set unchanged: `Nick=41`, `Evermoor=45`, `01007.m2ts=31`, `01099.m2ts=20`, `KAUTOKEINO 00007.m2ts=55`.
- Remaining 708 mismatch in this sweep: `Halloweentown Be Right Back.ts` (official none vs ours service `2`).
- TS GOP parity win:
- `internal/mediainfo/mpeg_ts.go`: GOP field output now prefers `"Variable"` when parser flags variable GOP, before interlaced `M/N`.
- `internal/mediainfo/mpeg2_video.go`: TS GOP variability now uses strict dominant-length threshold (`>=95%`) before collapsing to fixed length, preventing false Variable/Fixed flips on controls.
- Validation sweep (19 files; Halloween TS controls + UHD M2TS samples): `improved=11 same=8 worse=0` vs previous `main` (`f4fda3a`) using official JSON as reference.
- TS GOP cleanup:
- `internal/mediainfo/mpeg_ts.go`: suppress `Gop_OpenClosed` / `Gop_OpenClosed_FirstFrame` when GOP is variable (official omits on variable-GOP samples).
- Validation:
- Halloween control set (13 TS): `improved=11 same=2 worse=0` vs previous commit (`0d1e7f7`), each improved file dropped by 2 keys.
- New user-provided disc sample set (24 files across Excalibur/A.Beautiful.Mind/Ben-Hur/Infernal/Intruders/Network/Sabrina/The.Man.Who.Wasnt.There/Through.the.Looking.Glass/Zombeavers): `improved=0 same=24 worse=0` (no regressions).
- TS caption parity cleanup:
- `internal/mediainfo/mpeg_ts_captions.go`: emit CC3 only when command timing exists (`firstCommandPTS/Frame`) or as no-DTVCC fallback with detected display type.
- Validation (Halloween 71-file TS sweep): `improved=10 same=61 worse=0`; biggest wins from removing extra CC3 tracks on Disney/Adult Swim captures.
- TS caption parity cleanup (follow-up):
- `internal/mediainfo/mpeg_ts_captions.go`: emit CC1 only when command timing exists (matches official `Duration_Start_Command` behavior on sampled broadcasts).
- Validation vs `23ae163` (Halloween 71-file TS sweep): `improved=9 same=62 worse=0`.
- TS caption parity cleanup (outlier guard):
- `internal/mediainfo/mpeg_ts_captions.go`: suppress early service-2-only caption detection when CC1 is not emitted and computed first command start is `<0.5s`.
- Validation vs `684b8f8` (Halloween 71-file TS sweep): `improved=1 same=70 worse=0` (`Halloweentown Be Right Back.ts: 45->10`).
- Targeted non-TS/BDAV sanity sample set (13 files incl. `00007.m2ts`, `50201.m2ts`, `00068.m2ts`, `00003.m2ts`): unchanged (`worse=0`).
- TS XDS metadata guard (same outlier path):
- `internal/mediainfo/mpeg_ts.go`: emit XDS-derived General `Title`/`Movie`/`LawRating` only when a Text stream is actually emitted for that video PID.
- Combined validation vs `684b8f8` (Halloween 71-file TS sweep): `improved=1 same=70 worse=0` (`Halloweentown Be Right Back.ts: 45->7`).
- I/O note: some UHD `mediainfo` probes on `/mnt/storage/torrents` entered kernel `D` state (blocked I/O); reruns exclude those exact paths to avoid runaway disk stalls.

## Status (2026-02-13)
- CI: `gofmt` clean; `go test ./...` green (golden `samples/sample.ts` updated for new TS fields).
- TS parity (official: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
- `Assault in the Ring(2008.1080i.TS).ts`: diff `0` (AVC GOP `Format_Settings_GOP` now matches).
- `David Bowie and the Story of Ziggy Stardust (2012) ... .ts`: diff `0` (DVB subtitle `Duration`/`Language` + region/page extras now match).
- `Reigen Ohatsu - Furueru Iwa (2024).ts`: diff `524` (ISDB/ARIB metadata/text tracks not implemented).
  - Scan note: no other obvious ARIB/ISDB `.ts` samples currently found under `/mnt/storage/torrents` or `/mnt/storage/torrents/ptp-archive`.
- BDAV spot checks:
- `Intruders .../00061.m2ts`: diff `0`.
- `Network ... UHD .../00007.m2ts`: diff `0`.
- `Network ... 1080p AVC LPCM .../00007.m2ts`: diff `4` (remaining: AC-3 `compr_*` stats window/counts).
- `Static .../00004.m2ts`: diff `0`.
- `Sabrina .../00007.m2ts` (video-only clip): diff `0` (BDAV derived `StreamSize`/`BitRate` parity fixed).
- TS/BDAV AC-3 parity:
- Removed the AC-3 stereo `alignToByte()` dynrng hack (it caused BDAV false-positive `dynrng_*`).
- TS output now suppresses `dynrng`/`dynrng_*` for AC-3 stereo (`acmod==2`) to match official behavior on sampled discs.
- TS bounded ParseSpeed stats: dynamic tail contribution (TS vs BDAV + short-tail tweak) to better match `compr_*`/`dynrng_*` counts/averages (Nick Generic now `0`).
- AC-3 dynrng stats: skip `dynrng=0xFF` ("unset") from histogram (matches MediaInfoLib; fixes `dynrng_Minimum=-0.14` on Nick Saturday sample).
- Current focused diffs (same official command):
- `Nickelodeon - Saturday Morning Promo.ts`: diff `4` (remaining: AC-3 `compr_*` stats window/counts).
- `Nickelodeon - Generic Halloween Promo.ts`: diff `0`.
- `Disney Channel - Evermoor Behind The Scenes.ts`: diff `12` (remaining: AC-3 stats window/counts across both audio tracks).

## Status (2026-02-14)
- CI: `gofmt` clean; `go test ./...` green.
- TS parity controls unchanged:
- `Nickelodeon - Saturday Morning Promo.ts`: diff `4` (only `extra.compr_*`).
- `Nickelodeon - Generic Halloween Promo.ts`: diff `0`.
- `Disney Channel - Evermoor Behind The Scenes.ts`: diff `12` (AC-3 stats windows).
- Local dev hygiene:
- `internal/mediainfo/z_debug_*.go` ignored via `.gitignore` (avoid accidental commits).
- AC-3 bit parsing now bounds the bit reader to the syncframe when enough bytes are buffered (correctness guard; no parity change yet).
- ARIB/ISDB sample confirmation:
- `Reigen Ohatsu - Furueru Iwa (2024).ts`: contains `arib_caption` subtitle PID (ffprobe); still diff `524` (ARIB/ISDB metadata + subtitle parsing not implemented).
- MediaInfoLib note (likely relevant to remaining TS stats parity):
- `File_MpegTs.cpp`: when begin+end windows overlap, MediaInfo sets `MpegTs_JumpTo_End=0` (no end jump); current Go implementation still uses bounded head/tail sampling heuristics for stats.

## Status (2026-02-15)
- CI: `gofmt` clean; `go test ./...` green.
- TS bounded ParseSpeed windowing:
- Added DTVCC-aware head-window shrink gating on PCR span (prevents premature lock; fixes caption `Duration_Start_Command` parity on Evermoor controls).
- Added mid-file scan when DTVCC (CEA-708) seen (matches official read pattern).
- Jump resets now freeze MPEG-2 GOP info before clearing parser state (prevents false `Format_Settings_GOP=Variable` regressions).
- Current focused diffs (official: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
- `Nickelodeon - Generic Halloween Promo.ts`: diff `0`.
- `Nickelodeon - Saturday Morning Promo.ts`: diff `6` (AC-3 stats counts/avg).
- `Disney Channel - Evermoor Behind The Scenes.ts`: diff `6` (AC-3 stats counts; captions now match).

## Status (2026-02-15)
- CI/local: `gofmt` clean; `go test ./...` green.
- TS parity controls (official: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
- `Nickelodeon - Generic Halloween Promo.ts`: diff `0`.
- `Nickelodeon - Saturday Morning Promo.ts`: diff `4` (only `extra.compr_Average` + `extra.compr_Count`).
- `Disney Channel - Evermoor Behind The Scenes.ts`: diff `10` (AC-3 stats windows: `compr_*` counts/avg on both audio, and `dynrng_Count` on 5.1).
- Key validation: at `--ParseSpeed=1` on the same TS samples, our AC-3 `compr_*` / `dynrng_*` stats match official exactly; remaining mismatch is bounded ParseSpeed scan/sample window behavior (TS demux selection).

## Status (2026-02-15)
- CI: `gofmt` clean; `go test ./...` green.
- TS parity controls (official: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
- `Nickelodeon - Generic Halloween Promo.ts`: diff `0` (must not regress).
- `Nickelodeon - Saturday Morning Promo.ts`: diff `4` (remaining: mono AC-3 `extra.compr_*` count/avg).
- `Disney Channel - Evermoor Behind The Scenes.ts`: diff `10` (remaining: AC-3 `compr_*` and 5.1 `dynrng_Count` window).
- Latest win: bounded TS/BDAV AC-3 stats now sample tail-end frames and swap in missing tail-window dynrng extrema (fixes Evermoor `dynrng_Maximum` parity).

## Status (2026-02-15)
- CI: `gofmt` clean; `go test ./...` green.
- TS parity controls (official: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
- `Nickelodeon - Generic Halloween Promo.ts`: diff `0` (unchanged).
- `Nickelodeon - Saturday Morning Promo.ts`: diff `4` (still only `extra.compr_*` avg/count).
- `Disney Channel - Evermoor Behind The Scenes.ts`: diff `10` (down from `12`): fixed `compr_Minimum` parity by ensuring tail-window extrema can be represented in the selected bounded tail slice.
- Remaining Evermoor diffs: `compr_Count`/`compr_Average` (both AC-3 tracks) + `dynrng_Count` (track 7102).

## Status (2026-02-16)
- CI/local: `gofmt` clean; `go test ./...` green.
- TS controls (official: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
- `Nickelodeon - Generic Halloween Promo.ts`: diff `0`.
- `Nickelodeon - Saturday Morning Promo.ts`: diff `6` TSV-lines (`3` fields: `extra.compr_Count`, `extra.dynrng_Count`, `extra.dynrng_Average`).
- `Disney Channel - Evermoor Behind The Scenes.ts`: diff `4` TSV-lines (`2` fields on track 7102: `extra.compr_Count`, `extra.dynrng_Count`).
- Parameter sweeps (local-only; reverted):
- `tsHeadBoundPCR`: baseline `453600000` remains best on controls (higher values overcount Nick controls).
- `tsHeadBoundPCRDTVCC`: baseline `225000000` remains best on controls (lower values undercount Evermoor; higher values overcount).
- Tail-start lead and TS AC-3 sampled-mode variants were tested and reverted (no safe net win across control set).
- Debug finding (TS AC-3 stats): on `Nickelodeon - Saturday Morning Promo.ts` PID `0x1917`, current bounded windows yield `headFrames=515`, `tailFrames=545`, `compr_Count=286`, `dynrng_Count=1005`; brute force over head/tail selection with same frame pool cannot reach official (`compr_Count=298`, `dynrng_Count=1017`) -> remaining gap is scan-window/frame-capture timing, not histogram merge math.
- TS bounded jump quantization: align `jumpBytes` to MediaInfo-like 65424-byte effective step (`64 KiB - 112`), with near-boundary pad (`<=8192` remainder -> +10 steps). Control impact: `Generic` stays `0`, `Saturday` stays `6`, `Evermoor` improves `6 -> 4`; `Saturday` values moved closer (`compr_Count 297 vs 298`, `dynrng_Count 1016 vs 1017`); 15-file TS sample slice showed `0` regressions (`improved=0 same=15 worse=0`).
- Pushed: `af65c88` (`fix(ts): quantize bounded jump window to mediainfo step`).
- Matroska fuzz/CI: fixed lacing size validation in `readMatroskaBlockHeader` to prevent oversized `readN()` allocations on malformed blocks; added regression test + corpus seed.
- Scheduled fuzz: `.github/workflows/fuzz.yml` daily (~10m) + manual trigger; CI keeps 3s fuzz smokes.
- Fuzz targets: added container/parser fuzzers for `MP4`, `AVI`, `MPEG-PS`, `FLAC`, `MP3` (available for manual runs; scheduled job currently runs TS/Matroska/AC-3-focused targets).

## Learnings / Decisions
- Command name: mediainfo
- Parity target: MediaInfo-master in this repo
- CLI option parsing: case-insensitive before "=" only; values preserve case
- Unknown --Option[=Value] forwarded to core (default value "1" if missing)
- Help text copied from upstream CLI (Help.cpp)
- Stub core: parsing not implemented yet
- Upstream C++ tree kept untracked via root .gitignore
- Post-parity target: MediaInfo issue #760 (DVD IFO language/runtime regression)
- Issue #760 notes: IFO in VIDEO_TS shows VOB-derived streams but loses audio language; same IFO copied elsewhere shows languages; BUP files show old IFO-style info
- Issue #760: dev snapshots fixed duration, language issue persists when IFO is inside VIDEO_TS; still reported broken in 23.07+
- Issue #760 repro (Ask.Me.to.Dance.2022 DVD): `VIDEO_TS/VTS_02_0.IFO` lacks Language; copy outside VIDEO_TS shows Language=English; `VTS_02_0.BUP` shows Language even inside VIDEO_TS
- DVD IFO/BUP: parse IFO/BUP headers for video/audio attrs, chapters, duration; add Menu stream with chapter list
- DVD JSON: Language uses ISO-639 code (e.g., `en`), text uses full name (e.g., English)
- DVD JSON: Menu order is Duration, Delay, FrameRate, FrameRate_Num, FrameRate_Den, FrameCount, extra
- DVD JSON: SamplingCount present for audio; PixelAspectRatio present for video even when computed fields skipped
- JSON: FileExtension preserves filename case; emit File_Created_Date/Local (CLI includes)
- DVD aggregate (VTS_XX_0.IFO): parse title-set VOBs via streaming MPEG-PS; file size = sum VTS_XX_1..n + IFO (exclude VTS_XX_0.VOB)
- DVD aggregate JSON: add AC-3 *_String extras + dialnorm_Count; add Source in stream extra
- DVD IFO/BUP: audio/subpic counts are 16-bit; subpic attrs are 6 bytes, audio attrs 8 bytes
- DVD IFO/BUP: emit all audio/subpic streams; VMG (VIDEO_TS) shows Audio/Text without IDs
- DVD Menu lists: List (Audio) uses index list; Subtitles 4/3 + Pan&Scan are zero lists; Wide + Letterbox use index list
- DVD Menu JSON extra includes List_Subtitles_4_3/Wide/Letterbox/PanScan
- DVD chapter start times use tick sums (avoid per-cell rounding drift)
- Directory input: expand to sorted file list before analysis (CLI parity)
- DVD CC: detect GA94 user_data in MPEG-2, emit EIA-608 Text stream + JSON fields (MuxingMode_MoreInfo, Duration_* timing, FirstDisplay_*), CC3 for field 2
- DVD CC (DVD-Video): user_data header is "CC" (0x4343), type 0x01, caption_block_size 0xF8; caption blocks carry odd/even field flag for CC1/CC3
- DVD MPEG-PS sampling: when ParseSpeed < 1 and dvdExtras, sample includes middle chunk and min 16 MiB window to capture CC
- DVD CC mapping: DVD user_data odd-field captions map to CC3 (matches MediaInfo)
- DVD CC timing: Start_Command uses -1 CC frame, Start/End use -4 CC frames for PopOn (frame rate 29.97 when video 23.976)
- DVD CC visible duration: Duration_Start2End rounds to 0.5 ms (2000 Hz) before JSON formatting
- Amelie PAL DVD set (Disc 1/2) has no EIA-608 Text tracks (no CC timing to validate)
- Hebe NTSC DVD set has no EIA-608 Text tracks (no CC timing to validate)
- DVD AC-3 text: add numeric+dB duplicate lines + dialnorm/compr counts in aggregate mode
- `--output` without "=" treated as filename (matches upstream)
- `--` alone is a no-op (ignored)
- `--help` prints version line then usage
- Core analyzer implemented (General stream only): format sniff + file size
- Text output aligns with MediaInfo-style label/value columns
- JSON output implemented (minimal MediaInfo-like shape)
- MP4/MOV: parse `moov/mvhd` for duration; compute overall bitrate
- Matroska: parse Segment/Info for duration using TimecodeScale
- MPEG-TS: parse PAT/PMT, map stream types, PTS-based duration
- MPEG-PS: scan PES for stream types + PTS-based duration
- Text output now numbers stream groups (e.g., Audio #1)
- MP4: track detection via `trak/mdia/hdlr` (vide/soun/text)
- Matroska: track detection via Tracks/TrackEntry/TrackType
- MP4: sample entry parsing (`stsd`) for codec format
- Matroska: codec mapping via CodecID
- MP4: parse width/height (video) and channels/sampling rate (audio) from sample entry
- Matroska: parse track video/audio settings (pixel width/height, channels, sampling rate)
- MP4: derive frame rate from mdhd duration + stts sample count
- Matroska: derive frame rate from DefaultDuration
- General/stream fields now sorted to match MediaInfo ordering
- MPEG-TS: estimate frame rate from video PES count vs PTS span
- MPEG-TS/MPEG-PS: add video stream duration + bitrate (best-effort)
- MPEG-PS: per-stream IDs surfaced; JSON multi-file now outputs media list
- MPEG-PS: estimate frame rate from video PTS count vs duration
- Bit rate mode now emitted when bitrate is computed
- TS/PS: per-stream PTS tracking for non-video durations
- Stream common helper added for duration/bitrate fields
- JSON track ordering now stable; MP4/MKV audio durations added
- Channel layout now emitted for common channel counts
- JSON field ordering now matches CLI ordering
- JSON output now uses MediaInfoLib creatingLibrary + 26.01 version, with best-effort key mapping/values (still missing many raw fields)
- JSON output now matches MediaInfo CLI for MP4 (field order + formatting + raw values)
- MP4: top-level box sizes (ftyp/free/mdat/moov) mapped to JSON Header/Data/Footer/StreamSize + IsStreamable
- MP4: edit list media_time captured; JSON extra Source_Delay derived from media_time/timescale
- MP4 audio bitrate estimated from sample sizes + duration
- Matroska BitRate element parsed into stream Bit rate
- XML/HTML output renderers added (basic)
- JSON/XML/CSV outputs are not yet MediaInfo schema-compatible (currently text-like field names/values)
- --info-parameters / --info-canhandleurls now implemented (basic)
- Info-Parameters expanded to include IDs + menu group
- Output format placeholders accepted for CSV/EBUCore/PBCore/Graph
- CSV/EBUCore/PBCore renderers added (basic)
- Graph DOT/SVG outputs added (empty)
- CSV/XML/HTML now include stream numbering titles
- CSV output uses section headers (no header row), no CSV quoting, blank line between sections, 2 trailing blank lines
- AC-3 stats: text/CSV emit dialnorm_Maximum even when equal to Minimum; JSON extra only when distinct
- formatBytes for <1 KiB uses "Bytes" (not "B")
- XML field names sanitized for valid tags (spaces -> _)
- Upstream build needs autoconf/automake/libtool/pkg-config; use PATH "/opt/homebrew/opt/libtool/libexec/gnubin" for glibtoolize
- Built CLI binary at `~/github/oss/mediainfo-build/MediaInfo/Project/GNU/CLI/mediainfo`
- `--info-canhandleurls` prints libcurl-disabled message when built without libcurl
- Fixed file size units (KiB/MiB bug) + added formatBytes tests
- Added pure-Go parsers for MP3/FLAC/WAV/Ogg (duration/bitrate/channels/sample rate/bit depth)
- Audio-only general bitrate mode now set from parse (constant for PCM/CBR)
- MP4: parse `ftyp` for Format profile + Codec ID; `udta/meta/ilst/©too` for Writing application
- MP4: track IDs from `tkhd`, AAC profile from `esds`, bitrate from `btrt`
- MP4 AVC: parse `avcC` SPS/PPS for profile, chroma subsampling, bit depth, scan type, CABAC, ref frames, aspect ratio
- MP4: add stream size, bits/(pixel*frame), compression mode, frame rate mode, AAC frame rate
- MP4: honor edit list (`edts/elst`) for display duration, add Source duration + Source_Duration_LastFrame; stream size scales via sample delta
- Formatting: file size now rounds like MediaInfo; sub-minute durations show `X s Y ms`
- Matroska: parse EBML DocTypeVersion, SegmentUID, Writing/Muxing app; track number mapped to ID; add Format/Info
- Matroska: AVC codec private -> profile/settings/chroma/bit depth/scan; AAC ASC -> AAC LC + Codec ID A_AAC-2
- Matroska: tags ENCODER -> audio Writing library; x264 settings -> Nominal bit rate + Bits/(Pixel*Frame)
- Matroska: track flags Default/Forced; video color range; ErrorDetectionType set when CRC-32 elements present in header/segment (Per level 1)
- Matroska: ParseSpeed <1 skips cluster stats for small files (<=4 MiB) but still runs AC-3 probes; avoids bitrate/duration overrides
- Matroska: color source tracked (Container vs Stream); Color space only when stream color; JSON colour_* sources follow origin
- Matroska: FrameRate_Mode_Original=VFR when H.264 SPS fixed_frame_rate_flag is false (CFR derived from DefaultDuration)
- Matroska: Track BitRate (or H.264 HRD CBR) -> Nominal bit rate + Bit rate mode Constant; HRD VBR -> Maximum bit rate + Variable
- Matroska: Overall bit rate mode emitted only when Variable (matches MediaInfo)
- MPEG-TS: overall duration/bitrate derived from PCR interval average (packets between PCRs) to match CLI
- MPEG-TS: handle PCR PID payload (don't skip PES when PCR PID == video PID)
- MPEG-TS: parse x264 writing library + encoding settings from video PES (Annex B), add Nominal bit rate
- MPEG-TS: audio ADTS parsing needs int-cast shifts + next-sync validation; duration uses integer ms floor
- Text output: label column width 41 + ends with 2 blank lines (Fprintln adds 1) to match CLI
- AVI: parse RIFF hdrl/strl/movi + INFO/ISFT; MPEG-4 Visual VOL/user data for profile/BVOP/QPel/GMC/Matrix; frame rate ratio for stream, simple rate for general
- MPEG-PS/VOB: parse PES + MPEG-2 sequence/extension/GOP/picture headers for profile/Matrix/GOP/timecode/BVOP; duration uses PTS span + 2 frames; stream bitrate uses floored kbps for CLI parity
- MPEG Video (MPG): detect elementary stream; parse MPEG-2 headers for profile/GOP/timecode; add extension warning fields and general Format version
- MPEG-PS: private stream 0xBD substream IDs map AC-3 (0x80-0x87) / RLE subs (0x20-0x3F); AC-3 payload skips 4-byte DVD header; parse AC-3 header for bitrate/channels/sample rate/service kind; AAC ADTS probing for PS audio
- MPEG-PS: pack/system/padding headers (0xBA/0xBB/0xBE) need explicit skip; PTS parsing now checks marker bits; NTSC/PAL standard only when 720x480/576
- MPEG-PS: private stream 2 (0xBF) carries DVD menu/navigation; no PES header, payload is length-only; emit Menu stream with Format "DVD-Video" (no ID)
- ADTS: ID bit drives AAC Format version (MPEG-2 -> Version 2, MPEG-4 -> Version 4); expose Codec ID from AAC audio object in PS
- MPEG-PS: detect AVC in PES via Annex B SPS/PPS, switch format to AVC, parse profile/ref frames/CABAC + width/height/framerate
- H.264 SPS: parse video_format for Standard (PAL/NTSC/SECAM/MAC) + full-range flag for Color range
- AAC in PS: duration from PTS span + 3 AAC frames (1024 samples) to match CLI
- MPEG-PS AVC: slice count from Annex B first_mb_in_slice; probe after buffer grows (avoid early 1-slice false positives)
- MPEG-TS: do not emit slice count (CLI sample output omits it)
- H.264 Annex B: require SPS+PPS, valid profile, width/height; skip NALs with forbidden_zero_bit to avoid false AVC detection
- MPEG-PS AAC delay: adjust by 3 video frames for AVC to match CLI delay output; format delay as signed duration
- Bitrate formatting: use Mb/s when >= 1,000,000 bps in `formatBitrate`
- MPEG-PS AC-3: buffer across PES payloads; use AC-3 frame size table + next-sync validation; parse dialog normalization/compr/cmixlev/surmixlev/mixlevel/roomtyp + dialnorm stats
- AC-3 `compr` dB uses heavy dynamic range scale `pow(2, v) * ((code & 0xF) | 0x10)` with `v = (code>>4) - ((code>>7)<<4) - 4`, then `20*log10(scale)`
- Matroska JSON parity: TrackUID -> `UniqueID`, TrackOffset -> `Delay`/`Video_Delay` (Container); video Duration uses frameCount*DefaultDuration with 9-decimal JSON
- Matroska JSON: video Duration from stats uses framecount/fps with ratio token; ceil to ms; overrides segment Duration
- Matroska: Maximum bit rate text uses Mb/s; JSON `BitRate_Maximum` uses raw BitRate element
- Matroska: Menu chapters now emitted in text fields + JSON extra; chapter time uses raw ns (no TimecodeScale)
- Matroska: General `Overall bit rate mode` now derived from video Bit rate mode
- Matroska Text: sub bitrate floors b/s under 1 kb/s for CLI parity
- Matroska JSON adds `colour_*` fields when Color range present; `ErrorDetectionType` lives under `extra`
- JSON Rotation should be injected for MP4 video only; AAC `FrameCount` only for `mp4a*` codec IDs
- MPEG-TS JSON parity: StreamOrder `0-0`/`0-1`, IDs/MenuID decimal-only; Duration/Delay 9-decimal JSON; precision min/max = overall/9600
- MPEG-TS Menu JSON: `List_StreamKind`/`List_StreamPos`, Service* fields, PMT `pointer_field` + `section_length` in `extra`
- MPEG-PS JSON: FirstPacketOrder ignores Menu streams (video/audio start at 0)
- MPEG-PS: PTS tracker stores first PTS; text (RLE) Delay/Duration use first->last PTS span + MuxingMode=DVD-Video + Video_Delay
- MPEG-PS: video duration uses zero-segment half-frame seed; bitrate uses framecount/fps on reset streams and +1 frame when no resets
- MPEG-PS: JSON stream size uses full bytes (no padding subtraction); GOP-only bitrate mode = Constant
- MPEG-PS: single-file path now honors ParseSpeed sampling (first/middle/last)
- MPEG-PS header-only video (1-2 frames): use frameCount/frameRate duration, avoid syncing to audio PTS, stream size from header bytes (matches MediaInfo tiny StreamSize/BitRate for menu VOBs)
- DVD aggregate: title-set streams use VOB-derived duration; menu uses IFO-derived duration (formatDVDDuration)
- AC-3 text (dvdExtras): include bsid/dsurmod/acmod/lfeon; dynrng only when dynrnge present; surmixlev shows dB line only
- MPEG-PS text (RLE): add Format/Info + Delay relative to video
- MPEG-2 video: Time code emitted even when GOP variable; Max bit rate kept for DVD sizes even if mixed
- AVI JSON: StreamSize uses raw bytes; BitRate uses stream rate/scale duration (not kb/s-rounded); General JSON carries OverallBitRate/FrameCount/StreamSize
- MPEG Video JSON: no StreamOrder; BitRate uses JSON-rounded Duration; Delay_Settings from GOP header; BufferSize + intra_dc_precision in `extra`
- JSON field order: Delay_Settings between Delay and Delay_DropFrame; Format_Settings only for General
- AC-3 JSON: ServiceKind uses short codes (e.g., CM)
- AC-3 stats parity: dialnorm/compr/dynrng averages are intensity-based; skip compr/dynrng code 0xFF; dynrng counts include dynrnge=false frames as 0 dB but stats only emitted when any dynrnge seen
- AC-3 compr average uses linear mean (log10 of average), not dB mean
- ParseSpeed default is 0.5; ParseSpeed>=1 enables full E-AC-3 probe, <1 limits compr stats sample (target 1113 frames)
- Matroska E-AC-3: parse laced frames per-frame for probe stats
- E-AC-3 compr field uses 0xFF mapping (ac3ComprDB(0xFF)) even when stats exclude 0xFF
- CLI: cobra wrapper with ASCII banner; no-args shows cobra help, flags passthrough to internal CLI
- Release/CI: GoReleaser v2 + GitHub Actions CI (gofmt/vet/test/tidy), tag-triggered releases (v*)
- Self-update: cobra `update` command (release builds only), uses go-selfupdate
- UX: docs/help print lowercase flags; parser accepts options case-insensitively
- Repo samples: generated fixtures (ffmpeg testsrc2+sine); see `samples/README.md` and `samples/generate.sh`
- HEVC x265: parse SEI user_data_unregistered (payloadType 5, UUID hi `0x2CA2DE09B51747DB`) for `Encoded_Library`/`_Name`/`_Version`/`_Settings`; mirrors MediaInfoLib `File_Hevc::sei_message_user_data_unregistered_x265` (split on " - "; version truncated at " 8bpp"; options drop `options:`/digit-leading/`fps=`/`bitdepth=` tokens; no derived BitRate_Nominal unlike x264). Wired for Matroska (incl. hvcC CodecPrivate SEI via `parseHEVCConfigSEI`, takes precedence over generic `Lavc`/muxer ENCODER tag) + MPEG-TS (Annex B). Verified 1:1 vs reference on 8/10-bit mkv + ts; x264 mkv/mp4 unchanged.
- HEVC-in-MP4: `mp4_codec.go` `parseMP4HEVCSampleEntry` parses the `hvc1`/`hev1` `hvcC` box (config record + SPS via `parseHEVCConfig`, x265 SEI via `parseHEVCConfigSEI`) and emits `Format_Profile`/`Format_Level`/`Format_Tier`, `BitDepth`, `ChromaSubsampling`(+`_Position` when signaled), `ColorSpace=YUV`, `ScanType=Progressive`, `colour_*` (Source `Stream`, since MP4 has no `colr`), `extra.CodecConfigurationBox=hvcC`, `Stored_Width`/`Stored_Height` (coded vs display), and `Encoded_Library*`/`Settings`. hev1's `hvcC` carries SPS+SEI too, so no mdat scan is needed. Verified 1:1 vs reference on x265 8/10-bit, hev1-tagged, explicit-colour, signaled-chromaloc, and 1080p (`Stored_Height=1088`) MP4s; x264 mp4 + all non-HEVC formats byte-identical.
- HEVC shared fix: `parseHEVCConfig` now always reports `Format_Tier` (Main as well as High) and gates `ChromaSubsampling_Position` on the SPS `chroma_loc_info_present_flag` (value from `chroma_sample_loc_type_top_field`, plumbed through `parseHEVCVUI`) instead of a hardcoded "Type 2" for 4:2:0 — also fixes MKV HEVC (`matroska.go` and `mp4_codec.go` both call `parseHEVCConfig`; TS/BDAV use the Annex B path).
- HEVC-in-MP4 remaining (TODO): HDR10/Dolby Vision metadata (`HDR_Format`, `MasteringDisplay_*`, `MaxCLL`/`MaxFALL`) not yet surfaced for MP4 (`parseHEVCConfigSEI` already collects it); non-even resolutions (e.g. 322x242) still hit the pre-existing general-MP4 `PixelAspectRatio`/`DisplayAspectRatio` rounding gap; raw `.hevc` elementary streams have no format-dispatch case.

## Notes
- Update this file as we learn more about CLI behavior, formats, and edge cases.
- Loop-to-done: no “continue?” prompts. Run parity/bench cycles until 1:1 vs official + tests green.
- IO: avoid full-dataset sweeps on `/mnt/storage/torrents*`; sample a few files per type; stop runaway jobs fast.

- Status (2026-02-12):
- Status (2026-02-16):
- TS bounded-window jump parity tweak: when DTVCC detected, add one `miStep` before mid/tail jumps (kept existing near-boundary non-DTVCC padding).
- Focused parity (`mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
- `Nickelodeon - Generic Halloween Promo.ts`: diff `0` (unchanged).
- `Nickelodeon - Saturday Morning Promo.ts`: diff `6` (unchanged: `compr_Count 297/298`, `dynrng_Count 1016/1017`, `dynrng_Average 0.25/0.26`).
- `Disney Channel - Evermoor Behind The Scenes.ts`: diff `2` (improved from `4`; now only `dynrng_Count 793/796`).
- Additional TS control improved: `Disney XD on Disney Channel 10-25 Saturday Night Promo.ts` diff `5 -> 1`.
- TS 20-file safety sweep (Halloween controls subset): `improved=2 same=18 worse=0` vs `d135591`.
- Tests: `go test ./...` green.
- Perf spot-check (5 runs, ParseSpeed=0.5): no material regression on affected TS controls.
- Fuzz harness added: `internal/mediainfo/fuzz_parsers_test.go` with targets for AC-3/E-AC-3 frame parsing, TS/BDAV packet parsing, Matroska container parsing, Matroska block-header parsing, and Matroska cluster scanning.
- End-to-end smoke coverage now includes a minimal `.m2ts` analyze/render test: `internal/mediainfo/m2ts_smoke_test.go`.
- Note: `.mpls` playlist parsing is still not implemented in this repo (no parser path yet).
- Matroska hardening fixes from fuzz findings:
  - `readMatroskaBlockHeader`: malformed EBML lacing with `frameCount=1` now returns error instead of panicking.
  - `readMatroskaElementHeader`: now rejects element sizes larger than remaining bytes (prevents huge allocations/OOM on malformed inputs).
- Fuzz smoke checks run:
  - `go test ./internal/mediainfo -run=^$ -fuzz=FuzzParseAC3FrameParsers -fuzztime=3s`
  - `go test ./internal/mediainfo -run=^$ -fuzz=FuzzParseMPEGTSPacketizers -fuzztime=3s`
  - `go test ./internal/mediainfo -run=^$ -fuzz=FuzzParseMatroskaContainers -fuzztime=3s`
  - `go test ./internal/mediainfo -run=^$ -fuzz=FuzzReadMatroskaBlockHeader -fuzztime=3s`
  - `go test ./internal/mediainfo -run=^$ -fuzz=FuzzScanMatroskaClusters -fuzztime=3s`
  - all pass.
- CI now runs a bounded fuzz smoke stage (`go fuzz smoke`) after `go test` using the same 5 targets/timeouts.

- Status (2026-02-12):
- Parity snapshot (`mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
- TS: `Nickelodeon - Halloween Bumper 2.ts` diff=0, `Evermoor Behind The Scenes.ts` diff=25 (mostly AC-3 stats).
- BDAV/M2TS: AC-3 bounded stats parity now 1:1 on local control set:
- `Static.2012.../STREAM/00004.m2ts` diff=0
- `A.Bras.Ouverts.../STREAM/00001.m2ts` diff=0
- `Intruders.../STREAM/50258.m2ts` diff=0
- AC-3: fix dual-mono (1+1) BSI parsing alignment (dialnorm2/compr2/langcod2/audprodi2 fields) to match MediaInfoLib.
- AC-3 bounded stats: shift 2 earliest head frames into tail budget when the first head frames lack `compr` metadata (matches MediaInfoLib on BDAV edge cases).
- BDAV TrueHD bridge landed: detect TrueHD sync in 0x83-style streams; emit `Format=MLP FBA`, `MuxingMode=Stream extension`, TrueHD channel layout/positions + max bitrate while keeping AC-3 core extras.
- BDAV DTS fallback landed: infer DTS on 0x11xx audio PIDs when PMT is missing and parse core DTS fields (`channels/sampling/framecount/codec id`) with DTS-HD extension detection for JSON mode/commercial fields.
- BDAV AVC parity tweak landed: `BitRate_Maximum`/`BufferSize` now branch by detected audio family (`TrueHD=38999808 + dual buffer`, `DTS=35000000 + single buffer`, fallback unchanged).
- `ptp-archive` BDAV sweep (40-file sample): `old_avg=24.60 -> new_avg=15.05`, `improved=18 same=22 worse=0`.
- New focused diffs after DTS + AVC tweaks: `00001.m2ts=50` (from 90), `00003.m2ts=31` (from 71), `00206.m2ts=22` (from 44), `00201.m2ts=22` (from 44).
- BDAV stream-order parity tweak landed: normalize output order to `Video -> Audio -> Text` (PID tie-break) for BDAV/M2TS, matching official ordering in PMT-missing cases.
- Focused recheck vs previous commit (`HEAD~1`, line-diff metric): improved `00001.m2ts 149->139`, `00003.m2ts 126->83`, `00201.m2ts 92->80`, `00206.m2ts 92->80`; unchanged `01007.m2ts 29`, `01099.m2ts 18`.
- TS safety sweep (71 files in `keepalive/Halloween 2014 Promos and Bumpers Pack`): `improved=0 same=71 worse=0` vs previous commit.
- Added safe PTP candidate scanner: `scripts/ptp_scan.go` (API headers only; no scrape endpoints), with hard throttles/backoff (`403 halt`, `429/5xx backoff`), request caps, and local state cache.
- PTP scanner output now includes direct `download_url` (`torrents.php?id=<torrent_id>&action=download`) alongside group permalink for exact snatches.
- PTP scanner presets now include `file-mpls` to surface playlist-bearing disc uploads directly.
- BDAV DTS duration guard landed: when DTS duration collapses to sub-second on long BDAV clips, use video/container duration fallback; validated on `A.Bras.Ouverts .../STREAM/00001.m2ts` (`Audio.Duration 0.010 -> 28.417`, official `28.437`) with no regressions on TS/BDAV control set.
- Remaining big diffs: TS AC-3 stats sampling/count window parity; TS ATSC `Title`/`Movie` on some captures; BDAV video max bitrate/size edge cases; DVD/VOB duration/framecount/streamsize/GOP semantics; AVI container/video codec details (DivX/XviD specifics).

- Perf notes:
- Small files: startup dominates; go can be slower than `/usr/bin/mediainfo` (measure with multi-run median).
- Large MKV (2160p sample): go can be faster than official at ParseSpeed=0.5.

- Verification method:
- Compare official `/usr/bin/mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` vs `/tmp/go-mediainfo --output=JSON --language=raw`, normalize with `jq -S` and `del(.creatingLibrary)`, then `diff -u`.

- Status (2026-02-16):
- TS bounded ParseSpeed windowing: align PCR/DTVCC head-stop boundary to the same jump-aligned `MpegTs_JumpTo_Begin` value (fixes AC-3 stats window/count parity when the head scan is shrunk).
  - Avoid double-applying DTVCC step alignment when head size is already locked/aligned.
- Parity controls (official: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5`):
  - `Nickelodeon - Generic Halloween Promo.ts`: diff `0`.
  - `Nickelodeon - Saturday Morning Promo.ts`: diff `0` (AC-3 `compr_*`/`dynrng_*`).
  - `Disney Channel - Evermoor Behind The Scenes.ts`: diff `0` (AC-3 stats on both audio tracks).

## Agent skills

### Issue tracker

Issues live as GitHub issues in `autobrr/go-mediainfo`, managed with the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles, using the default label strings. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.
