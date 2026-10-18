# Research: delay fields (issue #74)

Date: 2026-10-18. Parent map: #65. Rules: ADR 0001 and `AGENTS.md`.

This note answers one question. Which delay fields does go-mediainfo get wrong, and which root causes make the 46 delay patterns of the 2026-10-17 corpus run? The fields are `Delay`, `Delay_Source`, `Delay_Settings`, `Delay_DropFrame`, `Delay_Original` and its sub-fields, `Video_Delay` (text label "Delay relative to video"), `Source_Delay`, `Source_Delay_Source` and `FirstDisplay_Delay_Frames`. Each cause gets a verdict: bug, deliberate, decision, or out of scope. When an open ticket already covers a cause, the note names that ticket.

## Method

- go-mediainfo: `main` at `6a37300`. Line numbers below refer to this commit.
- MediaInfoLib: tag `v23.04`, read through the GitHub API. Line numbers below refer to `v23.04`. The field visibility flags come from `Source/Resource/Text/Stream/*.csv` at the same tag.
- ZenLib: `Ztring.cpp` and `Ztring.h` on the default branch. ZenLib has no `v23.04` tag. The rounding function is old and stable.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. JSON: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` against `go-mediainfo --output=JSON --language=raw`, both through the `jq -S` normalization in `AGENTS.md`. Text: `mediainfo --ParseSpeed=0.5` against `go-mediainfo`, with the `ReportBy` line removed. Raw text: `mediainfo --Language=raw --ParseSpeed=0.5` against `go-mediainfo --language=raw`. Each run used `ionice -c3 nice -n10 timeout 300`.
- Real files: one file only. It is the second VOB of a title set of four VOBs, in the same disc folder as sample `dvd-02`. This note calls it `dvd-02/b`. No other library file was read.
- Synthetic files: S1 to S11, listed at the end. `ffmpeg` made each one from `testsrc` and `sine`. They contain no library content.
- Corpus data: the per-file pages of the corpus archive give the value pairs. The first value is official and the second value is Go. The witness C-ids come from these pages. Only the contributor can open the files.
- This note calls a file "joined" when its JSON `FileSize` in Go is larger than in official. In MPEG-PS this is the sign of DVD title-set joining (#116).

## Summary

| Cause | Patterns (main part) | Verdict | Ticket |
|---|---|---|---|
| D1 DVD title-set joining | G00112, G00117, G00118, G00122, G00125, G00226, G00234, G00505, G00506, and the DVD and joined parts of 18 more | Covered | #116 |
| D2 Text shows the video `Delay` and `Original delay` lines | G00063 (text), G00115 (text) | Bug | new |
| D3 Text "Delay relative to video" is built apart from `Video_Delay` | G00067 (text, zero), G00122 (AVI, MPEG-PS), G00173 (BDAV) | Bug | new |
| D4 Raw text shows delay fields twice, or shows hidden fields | G00067 (raw text), G00113, G00114 | Bug | new |
| D5 MPEG-2 `Delay_Original` does not come from the first-frame time code | G00118 (single files), synthetic S8 | Bug | new |
| D6 MPEG-2 time code trust rule | G00063 (JSON), G00247, G00248, G00249 | Covered | #138 |
| D7 DVD menu VOB rule deletes the delay fields | G00240, G00267, G00268, G00269, most single files of G00173, G00183, G00193 | Covered | #141 |
| D8 DVD VOB rules shift or delete delays after the parse | G00112 (single files), G00429, G00430 (MPEG-PS), RLE parts of G00183, G00193 | Bug | new |
| D9 `Video_Delay` rounding | G00117 (single files, TS), G00226 (single files) | Bug | new |
| D10 MP4 `tmcd` time code track is not read | G00551, G00552, G00553, G00554, MP4 parts of G00240, G00429, G00430, G00122 | Bug | new |
| D11 MP4 `Source_Delay` trigger | G00134, G00135, G00282, G00283, G00914, G00915, G01536 | Covered | #124 |
| D12 Matroska audio `Video_Delay` with no video delay | G00067 (Matroska JSON) | Bug | new |
| D13 AVI delay fields without the official conditions | G00115 (AVI JSON), AVI parts of G00367, G00368, G00067 | Bug | new |
| D14 Delay of a stream that official never sees start | BDAV parts of G00221, G00321, G00330, MPEG-PS parts of G00367, G00368, G00067 | Decision | new |
| D15 MPEG-PS caption `Delay` uses the minimum video PTS | G00234 (single files), caption part of G00226 | Bug | new |
| D16 DVD IFO Menu `Delay` | G00168 | Covered | #111 |

v26.10 gives no reason to keep a Go-only field. The patterns with v26.10 data are "extra in v26 too" or "missing vs v26 too". The other patterns have no v26.10 data.

## D1: DVD title-set joining

Patterns. Joined MPEG-PS files: G00112 (143 of 166 files), G00125 (133 of 134), G00117 (141 of 152), G00118 (135 of 147), G00122 (120 of 140), G00226 (42 of 47), G00234 (37 of 42), G00505 and G00506 (9 of 9), G00333 (15 of 24), G00173, G00183, G00193 (20 to 30 files each), G00221 (22), G00321 and G00330 (9 each), G00408 (8), and the values in G00063. DVD Video IFO files: G00115, G00379, G00063 (JSON), G00247, G00248, G00249 (18 each), G00367, G00368, G00067 (17 each), G00221 (6), G00321 (5), G00330 (4), G00408 (2). All 18 IFO files of G00115 are also in G00099, a pattern of #116.

Witnesses. All five files of the ticket: C290077, C000556, C290081, C290076, C290079. Also C290078, and the sheet witnesses C000016, C000027, C000106 (IFO files). The five ticket files have the same Go `FileSize`, 6914662400. Official reports about 1 GiB for each, and 473 MB for C290081. For each of them, Go reports the delays of the first VOB: video `Delay` 0.347366667 and `Delay_Original` 4122.501.

Official behavior. v23.04 reads only the given VOB. The delay is the first PTS of that VOB (`File_MpegPs.cpp:487-497`). The IFO parser fills no delay. `File_Dvdv.cpp` has no `Delay` field.

Go behavior. `analyze.go:199-206` joins the VOBs of the title set. The IFO path reads the VOBs too (`dvd.go:242-254`). The delays come from the first VOB.

Evidence. `dvd-02/b`: official `FileSize` 1073739776, video `Delay` 1854.929677778, `Delay_Original` 1853.000, time code 00:30:53:00, audio `Video_Delay` -0.374. Go `FileSize` 3592194048, video `Delay` 0.280633333, `Delay_Original` 0.000, time code 00:00:00:00, audio `Video_Delay` -0.221.

Verdict: covered by "DVD: decide whether to join the VOB files of a title set" (#116). Add the patterns above to it. Causes D2, D3, D8 and D15 also apply inside the joined files. The counts of those causes move fully only after #116.

## D2: Text shows the video `Delay` and `Original delay` lines

Patterns. G00063 text part: 304 files (MPEG-PS 286, DVD Video 18). G00115 text part: 136 files (MPEG-PS 101, AVI 30, DVD Video 5). Witnesses: C000014, C000527, C000619, C000167, C000168, C000169.

Official behavior. In `Video.csv:233` and `Video.csv:244`, `Delay/String` and `Delay_Original/String` have the flag `N NT`. The first letter `N` hides the field in the text output. So official text never shows a video delay. The current MediaInfoLib CSV has the same flags (`Video.csv:242`, `253`).

Go behavior. Three sites give the video delay a text label:

- `dvd.go:2608` writes `Delay_Original` with the label "Original delay". It runs for every title VOB.
- `dvd.go:2614` and `dvd.go:2665` write `Delay` again with the label "Delay" when a DVD rule changes it (see D8).
- `avi.go:554` writes `Delay` with the label "Delay" and the raw value `0.000` for every AVI video track.

Evidence. S3 is a DVD-style VOB. Official text has no delay line for video. Go text has `Original delay : 59 min 59 s` and `Delay : 467 ms`. S10 (AVI, video only) and S11 (AVI with PCM): Go text and raw text have `Delay : 0.000`. Official has no such line.

Verdict: bug. Users paste the text output into tracker uploads, so they see the extra lines.

## D3: Text "Delay relative to video" is built apart from `Video_Delay`

Patterns. G00067 text part with the value `0 ms`: 53 files (BDAV 16, DVD-path MPEG-PS 32, DVD Video 5). G00122 text part: AVI 6 files and MPEG-PS 3 single files. G00173 text part: BDAV 6 files. Witnesses: C000009, C000011, C000020 (BDAV, zero), C000619, C000634 (DVD, zero), C000167, C000168 (AVI), C000226, C000228 (MPEG-PS), C000010, C000030 (BDAV text tracks).

Official behavior. One rule makes all of these lines. When a `Delay` is filled, `Video_Delay` is the audio or text delay minus the video delay. If the result is 0, the `/String` variants are cleared, so the text line disappears (`File__Analyze_Streams.cpp:1278-1325`). `Video_Delay/String` has the flag `Y NT` (`Audio.csv:184`, `Text.csv:195`), so it shows in text for every container.

Go behavior. Each parser writes the text line with its own rule, apart from the JSON value:

- MPEG-TS and BDAV audio: `mpeg_ts.go:2485-2488` uses the minimum PTS, not the first PTS. It writes the line also for 0 ms.
- MPEG-PS: only the AC-3 branch (`mpeg_ps.go:1209-1213`), the AAC branch (`mpeg_ps.go:1418-1424`) and the Text branch (`mpeg_ps.go:1620-1624`) write the line. MPEG Audio and PCM get no line. `delayRelativeToVideoMs` subtracts 3 frames for H.264 video (`mpeg_ps.go:381-392`). The JSON value at `mpeg_ps.go:1648-1658` does not subtract them.
- DVD rules write `0 ms` (`dvd.go:2688`, `dvd.go:2760`).
- AVI writes `Video_Delay` only as a structured fact (`avi.go:842-844`), so the text line never shows.
- MPEG-TS and BDAV text tracks get the JSON value (`mpeg_ts.go:1658-1668`) but no text line.

Evidence. S7 and S8 are MPEG-PS files with MPEG-2 video and MP2 audio. Official text has `Delay relative to video : -10 ms`. Go text has no line, but Go JSON has `Video_Delay` -0.010 (S8) and -0.011 (S7). In S3, both tools show `-5 ms` for AC-3, so the AC-3 branch works.

Verdict: bug.

## D4: Raw text shows delay fields twice, or shows hidden fields

Patterns. G00067 raw text part: Matroska 182 files, WebM 40, MPEG-TS 4, BDAV 4. G00113 and G00114: 158 MP4 files, raw text only. Witnesses: C000037, C000046 (Matroska), C521132, C608371 (WebM), C000227, C000234 (TS), C000212, C000213, C000214 (MP4).

Official behavior. Raw text shows `Video_Delay/String` once. `File_Mpeg4.cpp:877-878` gives `Source_Delay` and `Source_Delay_Source` the option `N NT`, so they never show in text or raw text.

Go behavior. Two gaps exist:

- Matroska writes the text label "Delay relative to video" (`matroska_scan.go:2158-2160`), and so does MPEG-TS (`mpeg_ts.go:2487`). The raw text step also adds `Video_Delay/String` from the structured value (`field_project_raw_text.go:812-814`). "Delay relative to video" is not in `rawTextLabelAliases`, so `rawTextLabel` keeps the text label (`field_project_raw_text.go:1334-1339`). The raw text then has both lines.
- `rawTextExtraVisible` hides a fixed list of `extra` members (`field_project_raw_text.go:1111-1125`). `Source_Delay` and `Source_Delay_Source` are not in the list.

Evidence. In S1, the Matroska audio starts 9 ms late. Official raw text has `Video_Delay/String : 9ms`. Go raw text has that line and also `Delay relative to video : 9 ms`. S4 is an MP4 with an audio edit list. Go raw text has `Source_Delay : 79` and `Source_Delay_Source : Container`, and official has neither. The JSON is the same in both tools for S1 and S4.

Verdict: bug.

## D5: MPEG-2 `Delay_Original` does not come from the first-frame time code

Patterns. G00118 single files: 12 (C278352, C288507, C633429, C633430, C633431, C633432, C633494, C633495, C633497, C633498, C633581, C635292). Synthetic S8 shows the non-DVD case. The corpus possibly has no file of that case.

Official behavior. The MPEG video parser sets the stream delay from the first time code. It adds the `temporal_reference` of the picture to the GOP time code, and it accepts a lower value in the first 16 frames (`File_Mpegv.cpp:2511-2515`). It then fills `Delay` in milliseconds: the seconds, plus the frames divided by the frame rate (`File_Mpegv.cpp:1426-1437`). The MPEG-PS container moves this value to `Delay_Original` and puts the PTS in `Delay` (`File_MpegPs.cpp:487-497`).

Go behavior. Five sites write `Delay_Original`, and each has its own rule:

- MPEG-PS: always `0.000` (`mpeg_ps.go:1644`).
- MPEG-TS MPEG-2: always `0.000` (`mpeg_ts.go:2142-2147`).
- BDAV AVC: from the time code (`mpeg_ts.go:2203-2207`).
- Matroska MPEG-2: from the time code (`matroska_scan.go:2804-2814`).
- DVD title VOB: `dvd.go:2591-2608` turns the `TimeCode_FirstFrame` text into seconds. It subtracts 1 s for 2:3 pulldown when the frame is 0 and the time is 1 h or more. It rounds a drop-frame value and floors a non-drop value. It does not use `temporal_reference`.

Evidence. S8 is an MPEG-PS file outside DVD mode, with the time code 01:00:00:00. Official `Delay_Original` is 3600.000 and Go is 0.000. The time code fields are equal. Corpus values on DVD-path files: 3600.000 against 3599.000 (C278352, C633429, the pulldown rule), 35999.901 against 35999.900 (C288507, the floor rule), 3599.334 against 3599.000 (C633494), and 4694.434 against 4694.542 (C633430). The last two differ by several frames, which fits the `temporal_reference` rule.

Verdict: bug. #138 decides when the time code is trusted. This cause is about the value of a trusted time code.

## D6: MPEG-2 time code trust rule

Patterns. G00063 JSON part (MPEG-PS 20 files with a Go-only `Delay_Original`), G00247, G00248, G00249 (MPEG-PS 20 files). These 38 files are exactly the 38 files of G00132 (Go-only `TimeCode_FirstFrame`), a #138 pattern. Witnesses: C000638, C001451, C001558, C000555.

Official behavior. The time code is not trusted after two GOP time codes of 00:00:00:00 in a row, or after a time code that goes back. Then the stream fills no delay (`File_Mpegv.cpp:4034-4043`, `2516-2520`, `1427`). `Delay_Original`, `Delay_DropFrame` and their sub-fields stay empty, and the output omits them.

Go behavior. `mpeg2_video.go:621-646` keeps the first GOP time code with no test. `mpeg_ps.go:1635-1647` writes the delay fields for every video with a PTS.

Verdict: covered by "MPEG-2: apply the official time code trust rules and GOP open or closed rules" (#138). Its decision 1 asks whether the delay fields follow the same rule. The corpus says yes, because these patterns contain exactly the files of G00132.

## D7: DVD menu VOB rule deletes the delay fields

Patterns. G00240 (MPEG-PS 33), G00267, G00268, G00269 (33 each). The 33 files are the same set as G00274, a #141 pattern. For the single files of G00173, G00183 and G00193, 23 of 43 are in this set. C000873 and C276248 show the other signs of a menu VOB. Witnesses: C000426, C000681, C000755, C000869.

Official behavior. Official has no menu rule. A menu VOB keeps its delays.

Go behavior. `normalizeDVDMenuVOBStreams` deletes `Delay_DropFrame`, `Delay_Original`, `Delay_Original_DropFrame` and `Delay_Original_Source` (`dvd.go:2285-2299`). For text it calls `clearDVDPayloadTextTiming`, which deletes `Duration`, `Delay`, `Delay_Source` and `Video_Delay` (`dvd.go:2308`, `dvd.go:2711-2715`).

Verdict: covered by "DVD VOB: stop replacing and deleting MPEG-2 video fields after the parse" (#141). Add the text delay deletion and the patterns above to it.

## D8: DVD VOB rules shift or delete delays after the parse

Patterns. G00112 single files (about 15 of 21), G00429 and G00430 (MPEG-PS 7), the RLE parts of G00183 and G00193, and the DVD parts of G00067 and G00122. Witnesses: C000776, C282838, C529393, C633574 (shift), C633433, C633582, C000741 (audio delay deleted), C272450 (subtitle delay deleted).

Official behavior. Official has no DVD rules for delays. `Delay` is the first PTS (`File_MpegPs.cpp:496`).

Go behavior. Five rules run after the parse of a title VOB:

1. If the time code ends in `:59:00`, the video `Delay` drops by 2 frames (`dvd.go:2609-2616`). The audio `Video_Delay` does not change, so it no longer agrees with the two delays.
2. If the audio `Video_Delay` is near minus 2 frames, the video `Delay` drops by 2 frames and the audio `Video_Delay` becomes 0 (`normalizeDVDPayloadDelay`, `dvd.go:2623-2691`).
3. The rule deletes an audio delay of 0. It also deletes the delay of an audio stream that is shorter than 60 s and has a delay of more than 1000 s (`clearDVDShortAudioTiming`, `dvd.go:2694-2709`).
4. Only the first RLE subtitle keeps its delay. A later one keeps it only if it has the same values and the file has no caption (`cleanupDVDTextStreams`, `dvd.go:2319-2350`).
5. The longest audio gets `Video_Delay` 0.000 with the text `0 ms` (`expandDVDPrimaryAudioDuration`, `dvd.go:2760`).

Evidence. S3 is a VOB with the time code 00:59:59:00 at 29.97 fps. Official video `Delay` is 0.533366667 and Go is 0.466633267. The difference is 2 frames. Both tools report the audio `Video_Delay` as -0.005. Corpus values: C000776 0.060 against -0.020 (a negative delay in Go), C529393 0.0848 against 0.0048, C633574 0.2806 against 0.2139. C633433 has an audio `Delay` of 4405.46 s in official and none in Go. C272450 has delays for Text #2 and Text #4 in official and none in Go.

Verdict: bug. The rules are next to the rules of #141, but #141 covers the video fields only.

## D9: `Video_Delay` rounding

Patterns. G00117 single MPEG-PS files (C633430 -715 against -716 ms, C633433 -334 against -333 ms) and MPEG-TS (C000313 -22 against -21 ms). G00226 single files (C633575 6.656 against 6.657, C633577 4.554 against 4.555).

Official behavior. `Video_Delay` is `Delay(audio).To_int64s() - Delay(video).To_int64s()`, with both delays in milliseconds (`File__Analyze_Streams.cpp:1307`, `1321`). `To_int64s` reads the integer part, takes the fraction from a `float32`, and adds 1 only if the fraction is more than 0.5 (ZenLib `Ztring.cpp:2090-2124`, default option `Ztring_Rounded` in `Ztring.h:269`). Above about 4194 s, a `float32` has a step of 0.5 ms, so the fraction is coarse.

Go behavior. MPEG-PS audio rounds each delay with `math.Round` (`mpeg_ps.go:1652-1655`). MPEG-PS text formats the exact difference (`mpeg_ps.go:1662-1668`). MPEG-TS uses `math.RoundToEven` for each delay, and the exact difference for AC-3 (`mpeg_ts.go:1643-1657`).

Evidence. S7 is an MPEG-PS file with the video PTS 396483346 and the audio PTS 396482444. Official `Video_Delay` is -0.010 and Go is -0.011. The exact difference is -10.022 ms.

Verdict: bug. Go matches neither the official rule nor the exact difference.

## D10: MP4 `tmcd` time code track is not read

Patterns. G00551 (video `Delay`), G00552 (audio `Delay_DropFrame`), G00553 (video `Delay_Settings`), G00554 (video `Delay_Source`), and the MP4 parts of G00240, G00429, G00430 and G00122. Each of these has the same 7 files: C608258, C608261, C610725, C610767, C610821, C610922, C610929. C610767 and C610821 are sheet witnesses for the audio delay group.

Official behavior. `File_Mpeg4.cpp:3160-3183` reads the time code track. It fills `Delay`, `Delay_DropFrame` and `Delay_Source` on each track that the time code applies to. It fills `Delay_Settings` (`DropFrame=`, `24HourMax=`, `IsVisual=`) on video. The time code track becomes an Other track with `TimeCode_FirstFrame`, `TimeCode_LastFrame` and `TimeCode_Stripped`. `Source_Delay` then adds the time code delay to the edit list delay (`File_Mpeg4.cpp:869`).

Go behavior. go-mediainfo has no `tmcd` code.

Evidence. S5 is an MP4 with the time code 01:00:00:00. Official has video and audio `Delay` 3600.000, `Delay_DropFrame` No, `Delay_Source` Container, video `Delay_Settings` `DropFrame=No / 24HourMax=No / IsVisual=No`, audio `Video_Delay` 0.000, `Source_Delay` 3599979, and an Other track of the type "Time code". Go has none of these, and it writes `Source_Delay` -21.

Verdict: bug.

## D11: MP4 `Source_Delay` trigger

Patterns. G00134 and G00135 (127 files, Go-only JSON `Source_Delay`), G00282 and G00283 (32 files, video `Source_Delay` only in official), G00914 and G00915 (2 files, official `0`), G01536 (1 file, 2 against 3). Witnesses: C000212, C000213, C000214, C524569, C524575, C525309, C525297.

Official behavior. `Source_Delay` comes from the edit list only for two edit list forms, and only when the track already has `Source_Duration` (`File_Mpeg4.cpp:841-878`). It applies to every track kind. It adds the track `Delay`. It writes the value also when the value rounds to 0 ms.

Go behavior. `analyze.go:780-790` writes `Source_Delay` for audio only, without the `Source_Duration` condition. It skips a value that rounds to 0 ms. `mp4EditSourceDelaySeconds` does not compare the edit duration with the `tkhd` duration (`mp4_timing.go:207-224`).

Verdict: covered by "MP4: match the official trigger for `Source_*` fields and the trimmed stream size" (#124). Add these patterns and the code site `analyze.go:780-790` to it. The raw text part (G00113, G00114) is D4.

## D12: Matroska audio `Video_Delay` with no video delay

Patterns. G00067 Matroska JSON part: C000002, C000003, C000004.

Official behavior. Audio `Video_Delay` is filled only when the first video track has a `Delay` (`File__Analyze_Streams.cpp:1299`).

Go behavior. `matroska.go:2600-2605` and each Matroska audio builder (for example `matroska_aac_canonical.go:148-150`) first set `Video_Delay` to 0.000. `applyMatroskaTrackDelays` returns early when it finds no video timestamps (`matroska_scan.go:2130-2132`), so the 0.000 stays.

Evidence. S6 is an audio-only Matroska file. Official audio has `Delay` and `Delay_Source`. Go also has `Video_Delay` 0.000.

Verdict: bug.

## D13: AVI delay fields without the official conditions

Patterns. G00115 AVI JSON part (6 files), the AVI parts of G00367 and G00368 (C000172), and the AVI part of G00067 (C000172). Witnesses: C000172, C608297, C608393.

Official behavior. Official fills an audio delay only when the audio parser accepted the stream and the file has one video track. The delay includes the `strh` start offset. Only then does the video get `Delay` 0 (`File_Riff.cpp:397-431`).

Go behavior. `avi.go:553-555` writes video `Delay` 0.000 for every video track. `avi.go:839-844` writes audio `Delay`, `Delay_Source` and `Video_Delay` for every audio track. Only MP3 and AC-3 get a computed value. Go does not use the `strh` start offset.

Evidence. S10 is a video-only AVI. Official video has no `Delay`, and Go has `Delay` 0.000. S11 is an AVI with PCM audio. Both tools give the same JSON.

Verdict: bug. No file proves the `strh` start offset part yet.

## D14: Delay of a stream that official never sees start

Patterns. The BDAV parts of G00221, G00321 and G00330 (10 files each), the MPEG-PS parts of G00367 and G00368 (C001401, C001809), and the MPEG-PS JSON part of G00067 (C001809). Witnesses: C000009, C000010, C000060, C001809, C001401.

Official behavior. Official reads the start of the file, then jumps near the end. At the jump it stops the search for start timestamps (`File_MpegPs.cpp:944-946`, `File_MpegTs.cpp:2126`). A stream whose first PES comes later gets no `Delay`.

Go behavior. Go takes the first PTS from any part of the file that it scans (`mpeg_ts.go:1630-1641`, `mpeg_ps.go:1648-1670`).

Evidence. Corpus values only. On the BDAV files, the Go text `Video_Delay` is between 5661 s and 7303 s, so the first subtitle PES is near the end of the file. On C001809, Go audio #3 has `Delay` 38.709622222 and official has none.

Verdict: decision. The Go value can be the true start of the stream. Official omits it only because it did not read that part. ADR 0001 needs a recorded reason to keep a difference from v23.04.

## D15: MPEG-PS caption `Delay` uses the minimum video PTS

Patterns. G00234 single files C633495, C633497, C633498, and the caption part of G00226 in the D1 witness files (Text #3 `Video_Delay` 0.000 against 0.067).

Official behavior. A caption track inside the video gets the video `Delay` (`File_MpegPs.cpp:670`).

Go behavior. `mpeg_ps.go:1782-1786` gives the caption the minimum video PTS. `mpeg_ps_cc_stream.go:147-150` writes `Delay` only when it is more than 0, and always writes `Video_Delay` 0.000. The video `Delay` is the first PTS. After a PTS reset, the minimum PTS and the first PTS differ.

Evidence. Corpus values: C633495 caption `Delay` 313.495344444 against 201.234388889, and C633498 243.249633333 against 0.298566667. Both files show other signs of a PTS reset (#118).

Verdict: bug.

## D16: DVD IFO Menu `Delay`

Pattern G00168 (76 files). "DVD IFO: stop copying VOB rates and the menu frame rate" (#111) covers it and lists G00168.

## Pattern index

| Pattern | Field, track | Causes |
|---|---|---|
| G00063 | `Delay_Original`, Video | D2 (text, 304), D6 (JSON, 20), D1 (IFO JSON, 18) |
| G00067 | `Video_Delay`, Audio | D4 (raw text, 230), D3 (text zero, 53), D1 (IFO, 17), D12 (3), D14 (2), D13 (1), unexplained menu VOB (3), WebM (1) |
| G00112 | `Delay`, Video | D1 (143), D8 (about 15), unexplained menu VOB (6), WebM (2) |
| G00113, G00114 | `Source_Delay`, `Source_Delay_Source`, Audio | D4 |
| G00115 | `Delay`, Video | D2 (text, 136), D1 (IFO, 18), D13 (6) |
| G00117 | "Delay relative to video", Audio | D1 (141), D9 (5), unexplained menu VOB (6), WebM (3) |
| G00118 | `Delay_Original`, Video | D1 (135), D5 (12) |
| G00122 | "Delay relative to video", Audio | D1 (120), D3 (9), D8 (7), D10 (7), WebM (2) |
| G00125 | `Delay`, Audio | D1 (133), WebM (1) |
| G00134, G00135 | `extra/Source_Delay`, Audio | D11 |
| G00168 | `Delay`, Menu | D16 |
| G00173 | "Delay relative to video", Text | D7 and D8 (43), D1 (30), D3 (6), track identity (2) |
| G00183, G00193 | `Delay_Source`, `Delay`, Text | D7 and D8 (43), D1 (20 to 25) |
| G00221 | `Video_Delay`, Text | D1 (28), D14 (10), unexplained menu VOB and PTS reset (11) |
| G00226 | `Video_Delay`, Text | D1 (42), D9 (3), track identity (2) |
| G00234 | `Delay`, Text | D1 (37), D15 (3), track identity (2) |
| G00240 | `Delay_DropFrame`, Video | D7 (33), D10 (7) |
| G00247, G00248, G00249 | `Delay_DropFrame`, `Delay_Original_DropFrame`, `Delay_Original_Source`, Video | D6 (20), D1 (18) |
| G00267, G00268, G00269 | `Delay_Original` and sub-fields, Video | D7 |
| G00282, G00283 | `extra/Source_Delay`, Video | D11 |
| G00321, G00330 | `Delay`, `Delay_Source`, Text | D14 (10), D1 (13 to 14), PTS reset (1) |
| G00333 | `FirstDisplay_Delay_Frames`, Text | D1 (15), unexplained (9) |
| G00367, G00368 | `Delay`, `Delay_Source`, Audio | D1 (17), D14 (2), D13 (1) |
| G00379 | `Delay_Source`, Video | D1 |
| G00408 | `FirstDisplay_Delay_Frames`, Text | D1 (10), unexplained (6) |
| G00429, G00430 | `Delay`, `Delay_Source`, Audio | D10 (7), D8 (7) |
| G00505, G00506 | `Delay_DropFrame`, `Delay_Original_DropFrame`, Video | D1 |
| G00551 to G00554 | Video and audio delay fields | D10 |
| G00914, G00915, G01536 | `extra/Source_Delay`, Audio | D11 (G01536 can also be D10) |

## Questions that need a witness file

1. WebM (C608689, C609234, C611045): what are the timestamps of the first video block and the first audio block, and is one of them negative? Official reports a video `Delay` of 0.020 or 0.003 where Go reports 0.000. Go reports an audio `Delay` of -0.003 where official reports 0.000. S9 (VP9 and Opus) shows no difference.
2. D5 (C633430, C633494): what are the first GOP time code and the `temporal_reference` of the first pictures? The answer shows whether the official start rule explains the difference of several frames.
3. D13 (C000167, C000168): what is the `strh` start value of the audio stream, and how many bytes come before the first audio frame?
4. D14 (C000009, C001809): at which file offset is the first PES of the late stream? The answer proves that it is after the part of the file that official reads first.
5. Menu VOB files (C001259, C001357, C001550, C281654, C282594, C638253): what is the PTS sequence of the video? Go sees one video frame and a video `Delay` 2 frames later than official. These files also show the signs of #118 and #141.
6. `FirstDisplay_Delay_Frames` (C627329, C631276, C633495 differ, and C627328, C633494, C633574 are Go-only): at which video frame does the first caption show? The official counter rule is `File_Eia608.cpp:185-188` and `702-706`. Go uses `track.firstFrame` (`mpeg_ps_cc_stream.go:110`, `152`). Caption identity is in the scope of #77.

## Patterns not explained

- The WebM parts of G00112, G00117, G00122, G00125 and G00067 (3 files). See question 1.
- The single-file parts of G00333 (9) and G00408 (6). See question 6.
- The menu VOB files of question 5, in G00112, G00117, G00221 and G00067.
- C276255 and C631282: video `Delay` 0.280633333 against 0.280633334. A floating-point step in a DVD rule is the probable cause.
- C633576 and C633578 in G00173, G00226 and G00234: the Text tracks are in a different order (RLE against EIA-608), so the values compare different tracks. This is a track identity question for #77.

## Side findings

- MP4 raw text (S4): Go writes `Source_Duration_LastFrame : -11 ms`. Official writes `Source_Duration_LastFrame/String : -11ms`.
- `dvd-02/b`: Go has no Text tracks, and official has two. This is probably an effect of joining (#116).
- go-mediainfo computes `Delay_Original` in five places with five rules (D5), and the text "Delay relative to video" in about eight places (D3). One shared step for each would remove most of D2, D3 and D5.

## Proposed bug tickets

Each ticket names its cause. The decisions are plain questions that any contributor can answer.

1. "Text: hide the video Delay and Original delay lines" (D2).
   - Does the text output step hide `Delay/String` and `Delay_Original/String` for video in all containers, or does each parser stop giving them a label?
   - Do the HTML and raw text outputs take the same fix?
2. "Text: build "Delay relative to video" from `Video_Delay` and hide it at 0 ms" (D3).
   - Does one shared step make the text line from the JSON `Video_Delay` for every container?
   - For H.264 in MPEG-PS, which value is correct: the JSON value, or the text value with the 3-frame shift?
3. "Raw text: show `Video_Delay/String` once and hide `Source_Delay`" (D4).
   - Does the raw text output take its hidden fields from the official CSV flags, or from a list in the code?
4. "MPEG-2: take `Delay_Original` from the first-frame time code in every container" (D5).
   - Does one MPEG-2 parser function give the stream delay for MPEG-PS, MPEG-TS, Matroska and DVD?
   - Does the value use the official start rule, with `temporal_reference` and the lowest time code of the first 16 frames?
   - Must this fix wait for the trust rule of #138?
5. "DVD VOB: stop shifting and deleting the delays after the parse" (D8).
   - Does this fix go in the same change as #141?
   - Does a real file need one of these rules, for example the two-frame correction?
6. "`Video_Delay`: use one rounding rule for all containers" (D9).
   - Does go-mediainfo copy the official rule (each delay rounded to milliseconds by the ZenLib rule), or round the exact difference? If it rounds the exact difference, which line goes in the deliberate-differences list?
   - Does the AC-3 exception in MPEG-TS stay?
7. "MP4: read the `tmcd` time code track" (D10).
   - Does the Other track with the time code fields come in the same change as the delay fields?
   - Does the time code delay go to every track, as official does when no track refers to the time code track?
8. "Matroska: write audio `Video_Delay` only when the video has a delay" (D12).
   - Does the same rule apply to the other parsers that set `Video_Delay` to 0.000 by default, for example MPEG-PS and MPEG-TS captions?
9. "AVI: write the delay fields with the official conditions" (D13).
   - Does go-mediainfo add the `strh` start offset to the audio delay?
   - Which audio formats get a delay: only MP3 and AC-3, or every format that the parser accepts?
10. "MPEG-PS captions: use the video Delay for the caption Delay" (D15).
    - When the video `Delay` is 0, does the caption get `Delay` 0.000, as official does?

Proposed decision ticket:

11. "Delay of a stream that starts after the first part of the file" (D14).
    - Does go-mediainfo keep the true first PTS as a deliberate difference, or omit the delay as v23.04 does?
    - If it keeps the value, is the reason the same for MPEG-PS and BDAV?

## Synthetic files

Each file is 3 to 4 seconds long. The IDs describe the files. They are not library names.

| ID | Container | Content | Used for |
|---|---|---|---|
| S1 | Matroska | MPEG-4 Visual, AAC 9 ms late | D4 |
| S2 | AVI | MPEG-4 Visual, MP3 with a 20 ms input offset that the muxer removed | D13 (no difference) |
| S3 | MPEG-PS with a DVD title VOB name | MPEG-2 at 29.97 fps, time code 00:59:59:00, AC-3 | D2, D8 |
| S3b | MPEG-PS with a DVD title VOB name | As S3, time code 01:00:00:00 | D2 |
| S4 | MP4 | AVC, AAC with an edit list | D4, side finding |
| S5 | MP4 | AVC, AAC, `tmcd` track at 01:00:00:00 | D10 |
| S6 | Matroska | AAC only | D12 |
| S7 | MPEG-PS | MPEG-2, MP2, start PTS near 4405 s | D3, D9 |
| S8 | MPEG-PS | MPEG-2, MP2, time code 01:00:00:00 | D3, D5 |
| S9 | WebM | VP9, Opus | WebM question (no difference) |
| S10 | AVI | MPEG-4 Visual only | D2, D13 |
| S11 | AVI | MPEG-4 Visual, PCM | D2 |
