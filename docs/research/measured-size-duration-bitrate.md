# Measured size, duration and bit rate

Research for issue #66, a child of the map issue #65. Date: 2026-10-18.

## Question

Which stream size, duration, frame count and bit rate values are wrong in go-mediainfo, and which root causes produce them? The fields that upbrr reads come first: General `Duration`, General `OverallBitRate`, and Video `BitRate`.

## Sources and method

- The corpus run of 2026-10-17. Its Go binary reports `v0.8.1-0.20261016123829-78e84313ab42`. That commit is `main` at `6a37300` plus one frame-rate commit, which does not touch the code in this file. The contributor ran it on Windows.
- The local triage data of that run: the pattern list, the file list with the container and codec summary, and four value pairs for each pattern. It has no file paths. Files are C-ids and patterns are G-ids.
- go-mediainfo at `6a37300`. Citations are `internal/mediainfo/<file>:<line>`.
- MediaInfoLib source at tag `v23.04`. Citations are `MIL23 <file>:<line>`, relative to `Source/MediaInfo/`. `MIL-HEAD` is the newer checkout from 2026-06.
- The media host, with the official MediaInfo CLI `MediaInfoLib - v23.04` and a Linux build of `6a37300`. Commands:
  - `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5 <file>` against `go-mediainfo --output=JSON --language=raw <file>`.
  - `mediainfo --ParseSpeed=0.5 <file>` against `go-mediainfo <file>`.
  - Each command ran under `ionice -c3 nice -n10 timeout 300`.
- Library samples on the host: `fresh-02`, `fresh-12`, `mp4-01`, `dvd-01`, `dvd-02`. Each file ran one time. `dvd-02` also ran through a symbolic link outside `VIDEO_TS`.
- Synthetic files made with ffmpeg on the local machine and copied to the host. They are not library files. The names below exist only in this research:
  - `syn-reset`: two MPEG-2 and AC-3 program streams of 6 s and 4 s, joined with `cat`. The PTS (presentation time stamp) jumps back at the join. The video is CBR at 5 Mb/s. One copy is `VIDEO_TS/VTS_01_1.VOB`, and one copy is outside `VIDEO_TS`.
  - `syn-split`: one 10 s program stream, cut at a pack boundary into `VIDEO_TS/VTS_02_1.VOB` and `VTS_02_2.VOB`. `syn-whole` is the same stream as one file outside `VIDEO_TS`.
  - `syn-vfr`, `syn-vfr-noedts`, `syn-cfr`: H.264 and AAC in MP4, 12 s. The VFR files drop frames to make variable frame durations. `syn-vfr-noedts` has no edit list.
  - `syn-mvhd-long` and `syn-mvhd-longer`: `syn-cfr` with the `mvhd` duration patched to 12.4 s and 13.0 s. The tracks stay at 12.0 s.
  - `syn-webm` and `syn-vp8-mkv`: the same VP8 and Vorbis streams in a WebM file and in a Matroska file. `syn-opus` is H.264 and Opus in Matroska. ffmpeg writes no statistics tags.

ADR 0001 (PR #64) sets the rules for the verdicts. v23.04 is the reference. A v26.10 behavior wins only with a reason in the "Deliberate differences" list of `AGENTS.md`. XML, HTML and CSV differences count only when they share a root cause with a JSON or text difference.

## Summary

The counts are files in the corpus run. One file can have more than one cause.

| ID | Cause | Verdict | Main G-ids | upbrr fields |
|---|---|---|---|---|
| R1 | Go joins the VOB files of a DVD title set into one input | Bug under ADR rule 2, or a decision | G00099, G00100, and about 170 files in each of G00024, G00025, G00045 to G00051, G00054, G00055, G00075 | Duration, OverallBitRate, Video BitRate |
| R2 | DVD IFO durations drop the seconds in text | Bug | G00048, G00046, G00051, G00309 (DVD Video) | Duration (text) |
| R3 | MPEG-PS with a PTS reset: Go takes the last PTS segment | Bug in Go. Official is also wrong | G00046, G00048, G00050, G00024 (MPEG-PS) | Duration, OverallBitRate, Video BitRate |
| R4 | MPEG-PS: the AC-3 frame cap of 128 frames also applies when Go reads the whole file | Bug | G00051, G00075, G00045, G00046, G00048 (MPEG-PS) | Duration |
| R5 | MPEG-2 video: the sequence header bit rate becomes `BitRate` | Bug | G00024, G00243, G00229, G00396 (MPEG-PS, DVD Video) | Video BitRate |
| R6 | MP4: General Duration starts from `mvhd`, not from the longest stream | Bug | G00048, G00050 (MPEG-4) | Duration, OverallBitRate |
| R7 | WebM: Go uses the Matroska overhead ratio for the video estimate | Bug | G00024, G00047, G00025, G00037 (WebM) | Video BitRate |
| R8 | MP4 VFR video: Go divides by the track duration, not by frame count over the rounded frame rate | Defensible correction, or a parity fix | G00024, G00037 (MPEG-4) | Video BitRate (JSON, a few b/s) |
| R9 | MP4 AAC: Go has no 0.5 % tolerance between `avgBitrate` and `maxBitrate` | Bug | G00237, G00285, G00123, G00207 (MPEG-4) | none |
| R10 | MP4: the source-timeline (`Source_*`) trigger and the trimmed stream size differ from official | Bug, needs a witness | G00139, G00140, G00141, G00210, G00211, G00025, G00045, G00047 (MPEG-4) | none |
| R11 | Matroska General `StreamSize` is JSON-only | Out of scope under ADR rule 3 (XML only) | G00013 (Matroska, WebM) | none |
| R12 | Matroska x264 `Maximum bit rate` is JSON-only | Bug (text) | G00071 (Matroska) | none |
| R13 | Matroska Vorbis audio `StreamSize` is JSON-only | Bug (text) | G00148 (Vorbis) | none |
| R14 | DVD IFO: Go adds Text `Duration` and Menu `Delay`, `FrameRate`, `FrameCount` | Bug, or a decision under ADR rule 2 | G00178, G00168 to G00172 | none |
| R15 | MP4 audio `SamplingCount` from the exact track duration | Deliberate (listed in `AGENTS.md`, PR #59) | G00049 (MPEG-4, 106 files) | none |

## R1. DVD title-set joining

### What Go does

- Go analyzes a title VOB (`VTS_nn_m.VOB` inside `VIDEO_TS`) together with all the members of its title set. `FileSize` becomes the total size, and `CompleteName_Last` names the last member: `analyze.go:200-206`, `dvd.go:1832`.
- For a title-set IFO inside `VIDEO_TS`, Go reads the VOB files and puts their stream data into the IFO report: `dvd.go:149-153`, `dvd.go:242-292`. JSON `FileSize` becomes the IFO size plus the VOB size. The text `File size` stays the IFO size.

### What official v23.04 does

- `MIL23 Multiple/File_MpegPs.cpp` never calls `TestContinuousFileNames`. Only `File_MpegTs.cpp:329`, `File_Mpeg4.cpp:383` and some image parsers call it.
- `MIL23 Multiple/File_Dvdv.cpp` reads only the IFO. It has no code that opens a VOB.
- Newer MediaInfoLib joins files, with limits. `MIL-HEAD Multiple/File_Dvdv.cpp:285-305` merges only `VTS_nn_1.VOB` into an IFO. `MIL-HEAD Multiple/File_MpegPs.cpp:271-272` tests continuous names only for a `1.VOB` file of 0x3F000000 to 0x40000000 bytes, which is a VOB of about 1 GiB. Go joins without these limits.

### Proof

- Host, `syn-split`. On `VTS_02_1.VOB`, official reports 724 KiB and 5.038 s. Go reports 1.42 MiB and 10.010 s, which is the two files together.
- Host, `dvd-01` (a short title-set IFO). Official: JSON `FileSize` 18432, no video `BitRate`. Go: JSON `FileSize` 10194944, text `File size` 18.0 KiB, `OverallBitRate` 7828086, video `StreamSize` 9793817. Go JSON and Go text disagree on the file size.
- Corpus: G00100 (`CompleteName_Last` new, 181 MPEG-PS files) has the same files as the MPEG-PS part of G00099 (`FileSize` different). Of the MPEG-PS files in each key pattern, about 170 are in G00099 and G00100. The v26.10 presence list also has no `CompleteName_Last` for these files.

### Verdict

The joining is not v23.04 behavior, and `AGENTS.md` does not list it. Under ADR 0001 rule 2 it is a bug. The maintainer can also decide to keep it. Then the decision needs a line in the deliberate list, and the code must match a real MediaInfo rule. Today it matches neither v23.04 nor MIL-HEAD.

Witnesses: C000014, C000017, C000025 (MPEG-PS VOB), C000006, C000008, C000013 (DVD IFO). The ticket witnesses C000066, C000162 (IFO), C000520, C000556, C001435 (VOB) are all in this group.

## R2. DVD IFO duration text drops the seconds

`formatDVDDuration` (`dvd.go:1729-1742`) prints whole minutes when the duration is one minute or more. For 3439.5 s it prints `57 min`. The IFO code calls it for the General, Video, Audio, Text and Menu durations: `dvd.go:216`, `dvd.go:299-306`, `dvd.go:532`, `dvd.go:558`.

Official v23.04 prints the standard two units, for example `57 min 19 s`. Corpus pairs: G00309 (Menu) `"57 min 19 s" -> "57 min"` and `"1 min 49 s" -> "1 min"`, G00048 (General) `"33 min 55 s" -> "33 min"`. Above one hour, both tools print `1 h 34 min`, so long titles do not show the bug.

Verdict: bug in the text output. The JSON duration is correct. Witnesses: C000005, C000006, C000007.

## R3. MPEG-PS with a PTS reset

When a VOB has a PTS reset (a jump back of more than 2 s), `ptsDurationPS` returns only the last segment in DVD mode: `mpeg_ps.go:174-176`, `pts_tracker.go` (`durationLastSegment`). Outside DVD mode it returns the span from the smallest to the largest PTS. The frame count of the stream does not change the result.

Official v23.04 takes the first and the last time stamp of the stream (`MIL23 Multiple/File_MpegPs.cpp:704-751`). If the end is not after the start, it sets no duration from the PTS. Then it derives the duration from `StreamSize` and `BitRate` (`MIL23 File__Analyze_Streams_Finish.cpp:650-657`). It also clears all durations when the bit rate from the duration is 3 times too high or 20 times too low (`File_MpegPs.cpp:543-557`).

Proof:

- Host, `dvd-02` (a 10 MB title VOB with resets). ffprobe decodes 251 video frames and 40 AC-3 frames.
  - Official: `Duration` 9.713, video `FrameCount` 233. The audio duration is a copy of 9.713, but the audio is only 1.28 s.
  - Go: General `Duration` 1.312, video `Duration` 1.066 with `FrameCount` 252, `OverallBitRate` 62051902, video `StreamSize` 79734137, which is 784 % of the file.
  - The Go audio values (41 frames, 1.312 s) agree with ffprobe. The Go video and General values do not.
- Host, `dvd-02` through a link outside `VIDEO_TS`: Go General `Duration` 9.780, close to official.
- Host, `syn-reset`, 10 s of content. Official: 4.004 s, and video `BitRate` 12.5 Mb/s for a 5 Mb/s stream. Go in DVD mode: General 10.048 s, but video `Duration` 3.971 s with `FrameCount` 300. Go outside DVD mode: General 9.977 s, video `StreamSize` 2447039 (38 %), General `StreamSize` 3779649 (58 % of the file).

The video size above 100 % comes from the Go copy of the MediaInfo video estimate in `analyze.go:1817-1868`. It uses `OverallBitRate` from the wrong duration and has no upper limit at the file size.

Verdict: bug in Go. Official is also wrong when the stream has a reset. A fix can use the sum of the segments or the frame count, and keep a stream size at or below the file size. A result that is better than official needs a line in the deliberate list. Witnesses: C000426, C000527, C000543 (MPEG-PS, not joined).

## R4. AC-3 frame cap in MPEG-PS

`newPSStreamParser` sets `quickAC3` with a limit of 128 frames when `ParseSpeed` is below 1 and the file is not in DVD mode (`mpeg_ps_stream.go:55-56`). After 128 frames, Go stops reading AC-3 data (`mpeg_ps_stream.go:594-596`). Go reads a file of at most two analysis windows whole, and `opts.sampled` is false (`mpeg_ps.go:63-70`). Then the audio duration is the frame count (`mpeg_ps.go:271-273`). The video duration takes the audio duration through the `sync` value (`mpeg_ps.go:433-447`, `mpeg_ps.go:829-840`).

Proof, local and host, `syn-whole` (10 s, 1.4 MB):

- Official: General 10.016 s, video 10.010 s, audio 313 frames.
- Go with the default `--ParseSpeed=0.5`: General 9.943 s, video 4.091 s, audio 128 frames, 4.096 s, audio `StreamSize` 98304.
- Go with `--ParseSpeed=1`: 10.016 s, 10.011 s and 313 frames, the same as official.

Verdict: bug. It affects small MPEG-PS files outside `VIDEO_TS`. The corpus does not give file sizes, so the share of the about 100 MPEG-PS files that are not joined is not known.

## R5. MPEG-2 header bit rate as BitRate

For CBR MPEG-2 video (`vbv_delay` is not 0xFFFF), Go replaces the measured rate with the `bit_rate_value` of the sequence header: `mpeg_ps.go:927-929`. The MediaInfo video estimate in Go then does not replace it, because the mode is CBR: `analyze.go:1857`.

Official v23.04 puts the header value in `BitRate_Maximum` only (`MIL23 Video/File_Mpegv.cpp:1307-1313`). `BitRate` stays empty, so the video estimate from the overall rate fills it (`MIL23 File__Analyze_Streams_Finish.cpp:1432-1541`, with the MPEG-PS ratio 0.99 at lines 1466-1476).

Proof:

- Host, `dvd-02`. The header says 100 kb/s. Official `BitRate` 8086969. Go `BitRate` 100000 in both the DVD path and the plain path.
- Host, `syn-reset`. The header says 5 Mb/s. Official reports `BitRate_Maximum` 5000000. Go reports `BitRate` 5000000 and no maximum.
- Corpus: G00243 (Video `BitRate` new, for example `9000000`), G00229 (`BitRate_Maximum` new, `7500000`), G00396 (`BitRate_Mode` VBR to CBR).

Verdict: bug. A header value of 100 kb/s on a stream of 8 Mb/s shows that the header is not a safe source for `BitRate`. Witnesses: C001105, C001264, C001553 (G00243), C000228 (G00024, not joined).

## R6. MP4 General Duration from mvhd

Go starts General `Duration` from the `mvhd` duration. It uses a track or edit duration only when that one is more than 0.5 s longer (`analyze.go:235-244`). In v23.04 the `mvhd` fill is commented out (`MIL23 Multiple/File_Mpeg4_Elements.cpp:3949-3953`). General `Duration` is the longest stream duration (`MIL23 File__Analyze_Streams_Finish.cpp:1352-1371`). `OverallBitRate` follows from it (lines 1404-1419).

Proof, host: `syn-mvhd-long` (`mvhd` 12.4 s, tracks 12.0 s). Official `Duration` 12.000 and `OverallBitRate` 644581. Go `Duration` 12.400 and `OverallBitRate` 623788. With `mvhd` at 13.0 s, Go reports 13.000.

Verdict: bug. 48 of the 52 MPEG-4 files in G00050 are also in G00048. 30 of them are Android recordings with VFR video. The witness files must show that their `mvhd` is longer than their streams. Witnesses: C518293, C521821, C521823.

## R7. WebM overhead ratio

Go detects WebM as Matroska and estimates the video rate with the Matroska ratio 0.99: `analyze.go:2320-2335`. Official reports the General format `WebM`. Its ratio table has a case for `Matroska` but not for `WebM` (`MIL23 File__Analyze_Streams_Finish.cpp:1488-1498`). So WebM gets the default: 0.98 and 5000 b/s less for General, and 0.98 and 2000 b/s less for each stream (lines 1435-1442).

Proof, host:

- `syn-webm`. Official video `BitRate` 546037, `StreamSize` 819056, General `StreamSize` 55745. Go 568905, 853358 and 21443. The default formula gives 546037 exactly from `OverallBitRate` 711055 and audio 128000.
- `syn-vp8-mkv` (the same streams in Matroska). Official 567102, Go 567101.

Verdict: bug. All 56 WebM files of G00024 are also in G00047 and G00025. The format name (WebM against Matroska) is in the "format and codec naming" cluster. Witnesses: C521817, C608386, C608446.

## R8. MP4 VFR video bit rate

For VFR video, `mp4VideoBitRate` returns 0 (`mp4_timing.go:302`). Then Go divides the sample bytes by the track duration (`analyze.go:386-392`). Official uses `StreamSize*8*1000 / (FrameCount*1000/FrameRate)` (`MIL23 File__Analyze_Streams_Finish.cpp:614-630`), and the VFR frame rate has 3 decimals (`MIL23 Multiple/File_Mpeg4.cpp:881-893`). For CFR video Go already copies the 3-decimal rule.

Proof, host, `syn-vfr`: official `BitRate` 418223, Go 418218. The text is `418 kb/s` in both.

Verdict: Go computes the exact value. The difference is a rounding effect in official of a few parts per million, and it shows only in JSON. Two choices are possible: copy the rule for VFR as Go does for CFR, or add a deliberate line. 244 MPEG-4 files of G00024 have VFR video, and 210 of them are Android recordings. Witnesses: C518293, C518294, C518295.

## R9. MP4 AAC CBR tolerance

`mp4_codec.go:799-808` marks AAC as VBR and adds `BitRate_Maximum` when `maxBitrate` is larger than `avgBitrate` by any amount. Official marks it CBR when `maxBitrate` is at most `avgBitrate*1.005` (`MIL23 Multiple/File_Mpeg4_Descriptors.cpp:794-806`).

Proof, host, `syn-cfr`: official audio `BitRate` 128000, `BitRate_Mode` CBR, no maximum. Go `BitRate` 127547, `BitRate_Mode` VBR, `BitRate_Maximum` 128000. Go then also reports General `OverallBitRate_Mode` VBR, because one stream is VBR (`MIL23 File__Analyze_Streams_Finish.cpp:1588-1611`). That explains the MPEG-4 part of G00207.

Verdict: bug. Witnesses: C608274, C608277, C608319.

## R10. MP4 source timeline

Go fills `Source_Duration`, `Source_FrameCount` and `Source_StreamSize`, and trims `StreamSize` and `FrameCount`, when the edit duration differs from the `mdhd` duration by more than 0.5 ms and the edit media time is positive (`analyze.go:298-319`). Official v23.04 does this when the `stts` duration is outside the `tkhd` duration plus or minus one tick, in float32 arithmetic (`MIL23 Multiple/File_Mpeg4.cpp:762-786`). Official then sums the sizes of the first N samples (lines 790-823). Go scales by a proportion of the duration or of the sample count (`analyze.go:446-492`).

Proof: the corpus has 114 MPEG-4 files with a Go-only `Source_Duration` (G00139), and v26.10 does not emit it either. On `syn-cfr` both tools emit the same `Source_*` values, so the common ffmpeg edit list does not show the difference. The edit-list form in the corpus files is not known.

Verdict: bug. A witness file is necessary to find the edit-list form. Witnesses: C000212, C000213, C000214.

## R11. Matroska General StreamSize is JSON-only

Go stores the Matroska General `StreamSize` as a JSON-only value: `analyze.go:1199`, `analyze.go:2311`, `analyze.go:2611-2622`. The XML output does not get it. Official fills it in all outputs (`MIL23 File__Analyze_Streams_Finish.cpp:1373-1396`, `1565-1586`).

Proof, host, `fresh-02`: JSON `StreamSize` 349102 in both tools. The official XML has `<StreamSize>349102</StreamSize>` in the General track, and the Go XML does not.

Verdict: out of scope for now under ADR 0001 rule 3, because only XML shows it. It is the largest pattern (1278 Matroska and 57 WebM files), so a later XML pass can take it. Witnesses: C000034, C000036, C000037.

## R12. Matroska maximum bit rate missing from text

For x264 video with HRD (hypothetical reference decoder) signaling, Go first stores `BitRate_Maximum` as JSON-only (`analyze.go:1164-1169`). The text fill at `analyze.go:1174-1179` runs only when `BitRate_Maximum` is empty, so it never runs.

Proof, host, `fresh-02`: both JSON outputs have `BitRate_Maximum` 15000000. Official text has `Maximum bit rate : 15.0 Mb/s`, and Go text does not. The parity map already notes the same gap for `fresh-01`.

Verdict: bug in the text that upbrr pastes. It explains the Matroska part (174 files) of G00071. The MPEG-PS, MPEG-4 and BDAV parts are not explained. Witnesses: C000166, C000225, C000250.

## R13. Vorbis audio stream size missing from text

`matroska_vorbis_canonical.go:68-71` stores the derived Vorbis `StreamSize` with `builder.Structured`, so only JSON gets it. Official fills `StreamSize` from `BitRate` and `Duration` for every stream (`MIL23 File__Analyze_Streams_Finish.cpp:659-669`) and prints it in text.

Proof, host, `syn-webm` and `syn-vp8-mkv`: both JSON outputs have audio `StreamSize` 192048. Official text has `Stream size : 188 KiB (18%)`, and Go text does not.

Verdict: bug. It explains 68 Vorbis files of G00148. The 49 DTS files of G00148 are not explained. Witnesses: C000045, C000078, C000097.

## R14. DVD IFO fields that only Go emits

- Text `Duration` on each subpicture stream of an IFO: `dvd.go:530-533`. Official v23.04 and v26.10 do not emit it (G00178, 55 DVD Video files).
- Menu `Delay`, `FrameRate`, `FrameRate_Num`, `FrameRate_Den`, `FrameCount`: `dvd.go:558-598`. v23.04 does not emit them. `MIL-HEAD Multiple/File_Dvdv.cpp:369` and `1596-1599` emit them, with a rate of 30000/1001 for NTSC. Go writes 30/1. The v26.10 presence list marks them as Go-only, which disagrees with MIL-HEAD. A witness run on v26.10 can settle this.

Verdict: the Text duration is a bug. For the Menu fields, the maintainer decides under ADR rule 2. If they stay, the NTSC rate must match MIL-HEAD. Witnesses: C000012, C000013, C000021 (Text), C000005, C000006, C000007 (Menu).

## R15. Deliberate: MP4 SamplingCount

`AGENTS.md` lists this difference (PR #59). The 106 MPEG-4 files of G00049 are this case. Keep it. The MPEG-PS part of G00049 is R1. Example `165096000 -> 165095424`: Go multiplies 107484 AC-3 frames by 1536. Official multiplies 3439.5 s by 48000.

## Patterns that this research cannot explain

- G00048 BDAV (20 files): General `Duration` differs by less than 1 ms, for example `7332.404015333 -> 7332.404739958`. The text is the same. The research did not run BDAV files on the host.
- G00233 (44 Matroska files): Text `Stream size` percent `(0%) -> (1%)` for sizes near 50 MiB. Both tools round the same way in `fresh-12`, and Go uses the same formula (`format_fields.go:93`).
- G00233 also shows `"51.0 Bytes" -> "51 Bytes"`. Official prints one decimal for byte counts from 10 to 99 (`MIL23 File__Analyze_Streams.cpp:3058-3097`). `formatBytes` prints none (`format_bytes.go:7-8`). This is a small text bug.
- G00037 Matroska (76 files): `Bits/(Pixel*Frame)` in text, for example `0.486 -> 0.481`. 62 % of these files also have G00071. The parity map notes the same symptom on `fresh-11`.
- G00051 Matroska (65 files, 18 with Opus): audio `Duration`. `syn-opus` gives the same result in both tools.
- G00071 MPEG-PS, MPEG-4 and BDAV (119 files): Video `Maximum bit rate` missing in all outputs.
- G00148 DTS (49 files): audio `Stream size` missing.
- G00047 and G00025 MPEG-4 on Android files (about 90 files): video `StreamSize` differs. `syn-vfr` does not reproduce it. R10 is a candidate.
- G00223, G00258, G00270, G00272, G00286 (MPEG-PS captions and subtitles): Text durations, for example `1412.394 -> 1.502`. About 60 % of these files are R1 files. The rest needs a witness.
- G00103 (181 MPEG-4 files): `mdhd_Duration` missing from text, and v26.10 emits it. Go has the same condition as official (`mp4ShouldExposeMediaHeaderDuration` in `mp4_timing.go`). The cause is not found.

## Questions that need a witness file

1. R6: in C518293, C521821 and C521823, is the `mvhd` duration longer than the longest track duration? Which track sets it?
2. R10: in C000212, C000213 and C000214, what are the `tkhd`, `mdhd`, `stts` and `elst` values of the audio track?
3. R3 and R4: what are the file sizes, and how many PTS resets do C000426, C000527 and C000543 have?
4. G00233: what are the file size and the Text `StreamSize` of C000056, C000201 and C000249?
5. G00037: what are the video `BitRate`, `FrameRate` and size of C000057, C000059 and C000113?
6. G00051 Opus: do C000247, C000259 and C007382 have statistics tags? What are the tag `DURATION` and the last block time of each Opus track?
7. G00148 DTS: which DTS profile and which tags do C000032, C000051 and C000069 have?
8. R14: does MediaInfo v26.10 emit Menu `FrameRate` and `FrameCount` for C000005?

## Proposed bug tickets

1. DVD title-set joining is not v23.04 behavior (R1). Must go-mediainfo stop joining the VOB files of a title set, or record the joining as deliberate and limit it to the MIL-HEAD rules? If it stays, how do JSON and text agree on `FileSize`?
2. DVD IFO durations drop the seconds in text (R2). Can `formatDVDDuration` go away, so that IFO durations use the standard duration format? Which test pins `57 min 19 s`?
3. MPEG-PS durations after a PTS reset (R3). Which duration must Go report for a VOB with PTS resets: the sum of segments, or frame count over frame rate? How does Go keep a stream size at or below the file size?
4. AC-3 frame cap in a whole-file MPEG-PS read (R4). Can the 128-frame cap apply only when Go samples the file? Does the `sync` duration then match official on `syn-whole`?
5. MPEG-2 header bit rate is reported as BitRate (R5). Can the header value go to `BitRate_Maximum` only, as in `File_Mpegv.cpp:1312-1313`? Does the video estimate then fill `BitRate` for CBR streams?
6. MP4 General Duration comes from mvhd (R6). Can General `Duration` be the longest stream duration, as in v23.04? Which files need the `mvhd` value?
7. WebM uses the Matroska overhead ratio (R7). Can the video estimate use the default ratios when the file is WebM? Does the General format need to become WebM first?
8. MP4 AAC is VBR without the 0.5 % tolerance (R9). Can Go mark AAC as CBR when `maxBitrate` is at most `avgBitrate*1.005`? Which `BitRate` value does official report then?
9. MP4 Source_* trigger differs from v23.04 (R10). Which edit-list forms make Go emit `Source_*` when official does not? Can Go use the `stts` against `tkhd` test in float32?
10. Matroska text loses Maximum bit rate and Vorbis Stream size (R12, R13). Can the x264 `BitRate_Maximum` and the Vorbis `StreamSize` fill the text output too? Which other Matroska facts are JSON-only by mistake?
11. DVD IFO adds Text Duration and Menu frame fields (R14). Must Go drop the Text `Duration` of an IFO? Must it keep the Menu fields as v26 behavior with the 30000/1001 NTSC rate?

A decision for R8 (rounding) and R11 (XML only) can wait for the related family work.
