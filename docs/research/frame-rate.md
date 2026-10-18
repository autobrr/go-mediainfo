# Research: frame rate fields (issue #69)

Date: 2026-10-18. Parent map: #65. Rules: ADR 0001 and `AGENTS.md`.

This note answers one question. Which frame rate fields does go-mediainfo get wrong, and which root causes produce the 30 frame rate patterns of the 2026-10-17 corpus run? Each cause gets a verdict: bug, deliberate, or out of scope.

## Method

- go-mediainfo: `main` at `6a37300`. Line numbers below refer to this commit.
- Open PR #62 (`fix/video-frame-rate-normalization`, `8a847cf`): built and run on the same synthetic files, to find which patterns it already covers.
- MediaInfoLib: tag `v23.04`, read through the GitHub API. Line numbers below refer to `v23.04`.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The JSON command was `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` against `go-mediainfo --output=JSON --language=raw`. The text command was `mediainfo --ParseSpeed=0.5` against `go-mediainfo`, with the `ReportBy` line removed.
- Real files: parity samples `dvd-01` and `fresh-07`. No other file from the media library was read.
- Synthetic files: S1 to S17. Each is a 3 to 4 second clip that `ffmpeg` made from `testsrc` and `sine`. A small script changed one box or element in some of them. They contain no library content. The table at the end lists them.
- The witness C-ids come from the corpus data. Only the contributor can open them.

## Summary

| Cause | Patterns | Verdict |
|---|---|---|
| C1 MP4 minimum and maximum frame rate use a raw text label | G00076, G00077, G00087, G00088 | Bug |
| C2 MP4 VFR detection does not follow the official `stts` rules | G00341 (MP4), G00619, 14 files of G00076/G00077, part of G00110/G00111/G00120 | Bug |
| C3 FrameRate_Num/Den come from local guesses, not from the official rule | G00110, G00111, G00152, G00165, G00175, G00176, part of G00120 | Bug |
| C4 No snap to standard cadences on `main` | Part of G00120, G00152, G00165, G00300 (WebM) | Bug, open PR #62 |
| C5 No general check of container rate against stream rate | G00160, G00400, G01434 | Bug |
| C6 MP4 `mdta` metadata keys are not read | G00137 | Bug |
| C7 Audio frame rate is structured only, so the text line is missing | G00136 | Bug |
| C8 Audio frame rate is nominal, official is measured | G00513 | Deliberate candidate |
| C9 Audio frame rate text has no thousands separator | G00189 | Bug |
| C10 MPEG-TS AVC with `fixed_frame_rate_flag=0` | G00557 (TS), G00881, G00882, G00110 (TS) | Bug |
| C11 Matroska video without DefaultDuration gets no rate and no mode | G00577, G00557 (WebM, Matroska), G00028 (supported files), G01433 | Bug |
| C12 DVD Video IFO output carries VOB values and an invented menu rate | G00170, G00171, G00172, G00376 (DVD), G00120/G00152/G00165/G00300 (DVD) | Bug |
| C13 MPEG-2 2:3 pulldown detection is looser than official | G00120, G00152, G00300 (MPEG-PS) | Bug, provisional |
| C14 MP4 `mp4a` entries are always treated as AAC | G00376 (MP4) | Bug |
| C15 Ogg Theora frame rate text has no unit | G00300 (Ogg), G00120 (Ogg), G00110/G00111 (Ogg) | Bug |

v26.10 gives no reason to keep any of these patterns. For each pattern with v26.10 data, v26.10 agrees with v23.04 ("missing vs v26 too" or "extra in v26 too").

## C1: MP4 minimum and maximum frame rate use a raw text label

Patterns: G00076, G00077 (271 files), G00087, G00088 (257 files). All are MPEG-4. Witnesses: C000222, C518293, C518294.

Official behavior. For a VFR track, `File_Mpeg4.cpp:885-893` fills `Video_FrameRate_Minimum` and `Video_FrameRate_Maximum`. The text output shows them as `Minimum frame rate` and `Maximum frame rate`, with the unit `FPS`.

Go behavior. `analyze.go:580-589` writes the numeric fields with `builder.Structured`. It writes the text with `builder.Text("FrameRate_Minimum/String", "%.3f fps")`. The raw key becomes the text label, and the unit is lowercase. `field_schema.go` has no entry for `FrameRate_Minimum` or `FrameRate_Maximum`.

Evidence. S1 is a VFR MP4. Official text has `Minimum frame rate : 15.000 FPS` and `Maximum frame rate : 30.000 FPS`. Go text has `FrameRate_Minimum/String : 15.000 fps` and `FrameRate_Maximum/String : 30.000 fps`. The JSON values are equal.

Verdict: bug. The text output goes into tracker uploads, so users see the wrong label. The other 14 files of G00076 and G00077 come from C2.

## C2: MP4 VFR detection does not follow the official `stts` rules

Patterns: G00341 (MPEG-4 part, 10 files) and G00619 (4 files). These 14 files are exactly the files of G00076 that are not in G00087. Value effects also show in G00110, G00111, G00120 and G00300 for MP4. Witnesses: C608278, C608279, C608318 (G00341), C610698, C610855, C610882 (G00619).

Official behavior. Three rules apply:

1. In a `stts` table with 2 or 3 entries, a single first sample or a single last sample with a different duration does not count for the minimum and the maximum (`File_Mpeg4_Elements.cpp:8284-8298`).
2. The track is VFR only if the maximum rate minus the minimum rate is 0.001 or more (`File_Mpeg4.cpp:885`). Else the rate is `timescale / stts_Max` and the mode is CFR (`File_Mpeg4.cpp:897-898`).
3. A track that has a timecode reference gets no rate and no mode from `stts` (`File_Mpeg4.cpp:881`).

Go behavior. `parseStts` sets `variable` when any entry has a different delta (`mp4_timing.go:118-149`, line 138). The MP4 block then reports VFR, the average rate, and the minimum and the maximum (`analyze.go:551-590`). Go has none of the three rules.

Evidence. S2b has the table `119 x 1001, 1 x 500` at timescale 30000. Official reports CFR, `29.970 (30000/1001) FPS`, and no minimum or maximum. Go reports VFR, `30.096 FPS`, `FrameRate_Num` 30, `FrameRate_Den` 1, a minimum of 29.970 and a maximum of 60.000. S10 is a plain `ffmpeg -c copy` remux, and `ffmpeg` wrote the table `71 x 512, 1 x 533` by itself. Official reports CFR 25.000. Go reports VFR 24.986. So the pattern is common in real files.

Verdict: bug. Rule 3 is the probable cause of G00619. It needs a witness.

## C3: FrameRate_Num/Den come from local guesses, not from the official rule

Patterns: G00110, G00111 (171 files, Go adds the fields), G00175, G00176 (74 files, Go drops the fields), G00152, G00165 (92 and 78 files, the values differ), and the ratio part of G00120. Witnesses: C518293, C521714, C521720 (MP4), C521132, C608371, C608446 (WebM), C608393 (AVI), C000099, C000105, C000185 (G00175).

Official behavior. `File__Analyze::Fill` for a float frame rate clears Num/Den. It fills them again only if the exact value is an integer, or a precise or a rounded 1.001 cadence within 0.000002 (`File__Analyze_Streams.cpp:1598-1631`). Other Num/Den values come only from a merge with the stream parser (`File__Analyze_Streams.cpp:2316-2326`). A rate such as 26.866 gets no Num/Den.

Go behavior. Each container guesses on its own:

- `rationalizeFrameRate` snaps to 8 cadences within 0.0005. Else it returns `round(rate)/1` (`json_fields.go:600-624`, line 623). MP4 VFR (`mp4_timing.go:276-277`), MPEG-TS (`mpeg_ts_canonical.go:264`) and Matroska DefaultDuration (`matroska_video_canonical.go:562`) call it.
- A writer rule adds `round(rate)/1` for every `Lavf` Matroska file with an integer rate (`analyze.go:2510-2516`).
- AVI always writes a ratio, `rate/scale` if no cadence matches (`avi.go:1362-1383`).
- DVD writes a fixed `29970/1000` or `25/1` (`dvd.go:424-428`).
- Matroska AVC drops the ratio when the SPS timing does not match (`matroska_video_canonical.go:603-645`).

Evidence:

- S1 (VFR MP4, 26.866 fps): official has no Num/Den. Go has `27/1`. This value is false, because 27 is not 26.866. PR #62 keeps `27/1`.
- S14 (VFR MP4, 29.9698 fps): official shows `29.970 FPS` with no Num/Den. Go and PR #62 show `29.970 (30000/1001) FPS`.
- S3 (VP8 WebM from `ffmpeg`, 30 fps): official has no Num/Den, because `1e9/33333333` in float32 is not an exact integer. Go and PR #62 have `30/1` from the `Lavf` rule.

Verdict: bug. One shared function can copy `File__Analyze_Streams.cpp:1598-1631`, run on the unrounded value, and replace the per-container guesses. G00175 (Go drops `24/1` in Matroska) needs a witness to find the exact branch.

## C4: No snap to standard cadences on `main`

Patterns: the `29970/1000` part of G00120 (MP4, WebM, Matroska), part of G00152 and G00165, and G00300 for WebM (12 files, the General track copies the video rate). Witnesses: C521828, C521835, C521840 (MP4), C521132, C608883, C609113 (WebM), C000057, C000080, C000109 (Matroska).

Official behavior. Each container fill of a video rate goes through `Video_FrameRate_Rounding` (`File__Analyze_Streams.cpp:1567-1576`, inside `if (!IsSub)` at line 1162). The table in `Video_FrameRate_Rounded` (`File__Analyze_Streams.cpp:2479-2503`) snaps rates near 10, 12, 15, 23.976, 24, 25, 29.970, 30 and their doubles. The snap occurs only if the change is 0.001 or more. The new fill then makes Num/Den by the C3 rule, so a snapped 29.970 shows as `29.970 (29970/1000)`.

Go behavior. `main` has no snap. PR #62 adds the table, rebuilds Num/Den, and copies the first video rate to General.

Evidence. S13 is a VFR MP4 at 29.955 fps. Official shows `29.970 (29970/1000) FPS`. `main` shows `29.955 FPS` with `30/1`. PR #62 shows `29.970` with `29970/1000` and matches official.

Verdict: bug. The snap is a reporting convention, and PR #62 follows official, as the #62 discussion and ADR 0001 decided. PR #62 does not fix C3. S1, S3 and S14 keep their wrong Num/Den on the PR #62 build.

## C5: No general check of container rate against stream rate

Patterns: G00160 (84 files, `Original frame rate` missing), G00400 (17 MP4 files, Go adds `FrameRate_Mode_Original`), G01434 (1 AVI file, Go adds `FrameRate_Original`). Witnesses: C521720, C521721, C521723 (MP4), C000057, C000185, C000258 (Matroska), C000171 (AVI), C608221, C608350, C608351 (G00400), C609729 (G01434).

Official behavior. When the container rate string differs from the stream parser rate, `Merge` keeps the container rate and stores the parser rate as `FrameRate_Original` with its Num/Den (`File__Analyze_Streams.cpp:2316-2326`). A later fill clears the original when the two values become equal (`File__Analyze_Streams.cpp:1574-1575`). `Merge` sets `FrameRate_Mode_Original` only if the parser mode differs from the container mode (`File__Analyze_Streams.cpp:2327-2331`). For AVC, the parser mode is VFR when `fixed_frame_rate_flag` is 0 (`File_Avc.cpp:944-945`).

Go behavior. Go has no general comparison. `FrameRate_Original` comes only from special cases: a `XviD0050` writer string in AVI (`avi.go:527-529`), and Matroska SPS rules (`matroska_scan.go:2536`, `matroska_scan.go:2583`, `matroska_video_canonical.go:624`). Go sets `FrameRate_Mode_Original=VFR` whenever the AVC SPS says that the rate is not fixed, also when the container mode is VFR already (`mp4_codec.go:372-376`, `matroska_video_canonical.go:646-648`).

Evidence:

- S10 (24 fps H.264 in an MP4 container at 25 fps): official shows `Original frame rate : 24.000 FPS`. Go shows no original rate.
- S1 (VFR container, AVC rate not fixed): official has no `FrameRate_Mode_Original`. Go has `VFR`.

Verdict: bug. G01434 probably comes from the `XviD0050` rule. It needs the witness.

## C6: MP4 `mdta` metadata keys are not read

Pattern: G00137 (117 MP4 files, `FrameRate_Real` missing). All 117 files are also in G00076, so they are VFR recordings. Witnesses: C518295, C521913, C521914.

Official behavior. The `moov/udta/meta` key `com.android.capture.fps` goes to `FrameRate_Real` unchanged (`File_Mpeg4_Elements.cpp:3785-3786`, `File_Mpeg4.cpp:489-492`).

Go behavior. Go has no handler for `mdta` keys. A search for `capture.fps`, `mdta` and `com.apple.quicktime` in `internal/mediainfo` finds nothing.

Evidence. S11 is S2 with the key set to `60.000000`. Official shows `Real frame rate : 60.000000 FPS` and JSON `FrameRate_Real`. Go shows neither.

Verdict: bug. The same parser gap can make other General fields from `mdta` keys go missing. That is outside this ticket.

## C7: Audio frame rate is structured only, so the text line is missing

Pattern: G00136 (122 files: Matroska 104, AVI 18). By codec in the files: FLAC 63, Opus 34, MPEG Audio 14, PCM 6. Witnesses: C000044, C000048, C000099 (FLAC), C000182, C000188, C000195 (Opus), C000168, C000170, C000173 (MPEG Audio), C000117, C000239, C000282 (PCM).

Official behavior. Every `FrameRate` gets its display string at finish. For audio, the string adds `(N SPF)` (`File__Analyze_Streams.cpp:3257-3317`). Matroska fills a rate for every track kind from the Statistics tags, frame count divided by duration (`File_Mk.cpp:948-990`).

Go behavior. Several audio paths write the rate as structured only. These are FLAC (`matroska_flac_canonical.go:96`), PCM with DefaultDuration (`matroska_audio_canonical.go:199`), Opus and PCM from Statistics tags with an empty text label (`matroska_scan.go:1612`, `1618`, `1627`), and AVI MP3, AAC and AC-3 (`avi.go:771-803`). The JSON has the value, but the text has no `Frame rate` line.

Evidence:

- `fresh-07` (Matroska with FLAC): the JSON is equal (`10.767`, `4096` SPF). Go text has no line `Frame rate : 10.767 FPS (4096 SPF)`.
- S8 (AC-3 in AVI): Go JSON has `31.250` and `1536`. Official text has `Frame rate : 31.250 FPS (1536 SPF)`. Go text has no such line.

Verdict: bug. One shared projection from the structured `FrameRate` and `SamplesPerFrame` to the text line covers all these paths.

Some files of G00136 also miss the JSON value. Two Go conditions are narrower than official. Go writes a FLAC rate only for a fixed block size, and an AVI MP3 rate only with a LAME tag (`avi.go:775`). Official uses the Statistics tags (`File_Mk.cpp:948-990`) and the Xing frame count (`File_Mpega.cpp:546-558`). This part needs witnesses.

## C8: Audio frame rate is nominal, official is measured

Pattern: G00513 (9 files). Example: official `11.723 fps (4095 SPF)`, Go `11.719 fps (4096 SPF)`. Witnesses: C005258, C048143, C286607.

Official behavior. The rate is measured, frame count divided by duration (Matroska Statistics tags, `File_Mk.cpp:948-990`). The samples per frame then come from the sampling rate divided by that rate (`File__Analyze_Streams_Finish.cpp:1188-1214`). A short last block lowers the average, so the value becomes 4095 SPF.

Go behavior. Go reports the sampling rate divided by the fixed block size (`matroska_flac_canonical.go:93-96`).

Verdict: deliberate candidate for FLAC. A FLAC stream with a fixed block size of 4096 has 4096 samples in each frame, so the Go value is correct. Under `AGENTS.md`, this needs a real-file proof and a line in the deliberate differences list before it counts as deliberate. The files in MP4 (AAC), MPEG-PS (AC-3) and Matroska (MPEG Audio) need witnesses, because the corpus examples show only FLAC values.

## C9: Audio frame rate text has no thousands separator

Pattern: G00189 (64 Matroska files, text only, whitespace only). Example: official `1 200.000 FPS (40 SPF)`, Go `1200.000 FPS (40 SPF)`. The files have TrueHD (MLP FBA). Witnesses: C000079, C000081, C000103.

Official behavior. `Language_Get` puts the thousands separator, a space, in front of the last three digits of the integer part (`MediaInfo_Config.cpp:2501-2509`).

Go behavior. `formatAudioFrameRate` prints `%.3f` with no grouping (`mpeg_ps.go:2151-2156`). `formatThousands` in `format_duration.go:74` already does the grouping.

Evidence. S7 is TrueHD in Matroska. Official shows `1 200.000 FPS (40 SPF)`. Go shows `1200.000 FPS (40 SPF)`.

Verdict: bug. The text is a reporting convention. The video `formatFrameRate` (`format_rates.go:5-10`) has the same gap for rates of 1000 or more.

## C10: MPEG-TS AVC with `fixed_frame_rate_flag=0`

Patterns: G00557 (MPEG-TS part, 2 files, `FrameRate_Mode` missing), G00882 (2 files, Go adds the video rate), G00881 (2 files, Go adds the General rate), and G00110/G00111 for MPEG-TS. All are the same two files. Witnesses: C000313, C610417.

Official behavior. When the AVC VUI has timing but `fixed_frame_rate_flag` is 0, the AVC parser sets the mode to VFR and fills no rate (`File_Avc.cpp:942-948`). MPEG-TS has no container rate, so the output has `FrameRate_Mode: VFR` and no `FrameRate`.

Go behavior. MPEG-TS fills the rate from the VUI and from `rationalizeFrameRate` (`mpeg_ts_canonical.go:262-266`, `301-304`). No MPEG-TS file reads `FixedFrameRate`.

Evidence. S12 is `x264` output in MPEG-TS, and `x264` writes the flag as 0. Official has video `FrameRate_Mode: VFR`, no video rate, and no General rate. Go has `29.970` with `30000/1001` on the video track, `29.970` on General, and no mode.

Verdict: bug. When the flag is 0, the VUI tick rate does not set the frame rate, so the Go value is a guess. Follow official.

## C11: Matroska video without DefaultDuration gets no rate and no mode

Patterns: G00577 (6 files, video rate missing), G00557 (WebM 4 and Matroska 1, mode missing), G00028 on supported containers (WebM 4, Matroska 1, DVD Video 1, AVI 1, the General rate missing), and G01433 (1 file). Witnesses: C609477, C609537, C609592 (WebM), C639567 (Matroska, also G01433), C534799.

Official behavior. Matroska measures the rate from block timecodes (`File_Mk.cpp:1067-1144`). It uses DefaultDuration as a fallback (`File_Mk.cpp:1147-1156`). It always sets the mode, VFR or CFR (`File_Mk.cpp:1158`).

Go behavior. `applyMatroskaStaticVideoTiming` returns at once when there is no DefaultDuration (`matroska_video_canonical.go:557-559`). The mode is `Constant` whenever DefaultDuration exists (`matroska_video_canonical.go:561`).

Evidence. S16 is S3 with both DefaultDuration elements changed to EBML Void. Official shows video `30.068` and `CFR`, and General `30.068`. Go shows no rate and no mode on either track.

Verdict: bug for G00577, G00557 and G00028 on WebM and Matroska. The official `0.000` values in G00577 and the G00028 files on DVD Video and AVI need witnesses. G00341 for Matroska (13 files, CFR and VFR in both directions, witnesses C000185, C529429, C529865) probably has the same root, because Go does not measure the mode from block timecodes. It also needs witnesses.

## C12: DVD Video IFO output carries VOB values and an invented menu rate

Patterns: G00170, G00171, G00172 (76 files, Menu `FrameRate`, Num, Den), G00376 for DVD Video (15 files, audio rate), and G00120, G00152, G00165, G00300 for DVD Video (10, 10, 10 and 1 files, video 23.976 against 29.970). All these files are in G00170. Witnesses: C000005, C000006, C000007 (menu), C000006, C000008, C000016 (video and audio).

Official behavior. For an IFO file, the video rate comes from the IFO table (`File_Dvdv.cpp:597`). The menu stream gets the chapters and a duration only (`File_Dvdv.cpp:1457-1479`). The audio stream has no frame rate.

Go behavior. The IFO path writes a menu rate of 30 or 25 with Num/Den and a frame count (`dvd.go:557-597`). It also reports the AC-3 rate and a pulldown rate from the VOB analysis.

Evidence. `dvd-01` is an IFO file. Official: General and video `29.970`, video `29970/1000`, frame count 312, no audio rate, no menu rate. Go: General and video `23.976`, video `24000/1001`, frame count 249, audio `31.250 FPS (1536 SPF)`, menu `30.000 FPS` with `30/1`.

Verdict: bug. v26.10 also has no menu rate ("extra in v26 too"). The menu `Delay` and `FrameCount` patterns of cluster 37 (G00168, G00169) come from the same lines.

## C13: MPEG-2 2:3 pulldown detection is looser than official

Patterns: G00120, G00152 and G00300 for MPEG-PS (12 files each, the same files). Official `29.970`, Go `23.976`. Witnesses: C000556, C000557, C001103.

Official behavior. The MPEG-2 parser reports 2:3 pulldown and 23.976 only if the analyzed pictures contain the exact cadence `T2T3B2B3T2T3B2B3` or its mirror (`File_Mpegv.cpp:1347-1363`).

Go behavior. One picture with `repeat_first_field` in a progressive stream at 29.97 is enough (`mpeg2_video.go:862-869`).

Verdict: provisional bug. The source explains the direction of the difference. A witness must show that the cadence in these files is not a regular 2:3 cadence.

## C14: MP4 `mp4a` entries are always treated as AAC

Pattern: G00376 for MPEG-4 (4 files, Go adds an audio rate). Witnesses: C608715, C608936, C609458.

Go behavior. Every `mp4a` sample entry gets `rate / 1024` and `1024 SPF` (`mp4_codec.go:1007-1011`). `mp4_codec.go:734` has the same assumption.

Evidence. S6 is MP3 in MP4. Official: `Format: MPEG Audio`, `Layer 3`, no frame rate. Go: `Format: AAC`, `FrameRate 46.875`, `SamplesPerFrame 1024`.

Verdict: bug. The Format value is also wrong, so other clusters can show the same files.

## C15: Ogg Theora frame rate text has no unit

Patterns: G00300 for Ogg (General text `30.000 FPS` against `30.000`), and G00120, G00110 and G00111 for Ogg. It is one file. Witness: C000232.

Go behavior. `ogg.go:414-416` fills `FrameRate` with the numeric string as both the value and the text.

Evidence. S17 is Theora in Ogg. Official text shows `30.000 FPS` on General and video. Go shows `30.000` on both. PR #62 fixes General only, and the video text stays `30.000`. The JSON is equal.

Verdict: bug.

## Patterns that this note cannot explain

- G00120 for MP4. In 71 of 82 files, official shows `29.970 (29970/1000)` and Go shows `29.970 (30000/1001)`. C4 explains the official value when the measured rate snaps. But on `main`, Go shows `30000/1001` only when its rate is within 0.0005 of 29.97003 (C3). A witness must show the measured rates of the two tools.
- G00175 and G00176. Go drops `24/1` and similar values in 73 Matroska files, most with AVC or HEVC at 24.000. The branch in Go that drops them is not known.
- G00577. The official value `0.000 FPS` on WebM and Matroska.
- G00028 on DVD Video (1 file) and AVI (1 file, C609329).
- G00136 files where the JSON value is also missing (C7, second part).

## Questions for the contributor (need a witness file)

1. C610698, C610855, C610882 (G00619): does each MP4 video track have a timecode track reference (`tref/tmcd`)?
2. C521828, C521835, C521840 (G00120 for MP4): what are the `stts` tables, the timescale, and the VUI timing of the video track?
3. C000099, C000105, C000185 (G00175): what are the writing application, the DefaultDuration, the Statistics tags, and the SPS timing?
4. C000556, C000557, C001103 (G00120 for MPEG-PS): what is the sequence of `top_field_first` and `repeat_first_field` flags in the first GOPs?
5. C609477, C639567, C534799 (G00577): does the video track have a DefaultDuration, and how many video blocks does it have?
6. C000168, C000170, C000173 (G00136 for AVI MP3): does the first MP3 frame have a Xing, Info or VBRI header, and does it have a LAME tag?
7. C005258, C048143, C286607 (G00513): what is the audio codec, and is the FLAC block size fixed?
8. C609729 (G01434): what is the writing library string of the AVI video stream?

## Side findings outside this ticket

- Go prints `Format settings, Reference frames : 1 frames` where official prints `1 frame` (S1, S2, S5, S6, S10). This is a text plural rule.
- Go reports MP3 in MP4 as AAC (C14). Look for it in the audio format clusters.

## Synthetic files

All files are short `ffmpeg` clips (`testsrc`, `sine`). They contain no library content.

| ID | Content |
|---|---|
| S1 | H.264 in MP4, VFR: 60 frames at 30 fps, then frames at 24 fps |
| S2 | H.264 in MP4, CFR 30000/1001, AAC |
| S2b | S2 with the video `stts` changed to `119 x 1001, 1 x 500` |
| S3 | VP8 and Opus in WebM, 30 fps |
| S4 | MPEG-4 Visual and CBR MP3 in AVI |
| S5 | H.264 and FLAC in Matroska, 24 fps |
| S6 | H.264 and MP3 in MP4 |
| S7 | H.264 and TrueHD in Matroska |
| S8 | MPEG-4 Visual and AC-3 in AVI |
| S9 | MPEG-4 Visual and VBR MP3 in AVI |
| S10 | Raw H.264 at 24 fps, remuxed into MP4 at 25 fps |
| S11 | S2 with the `mdta` key `com.android.capture.fps` set to `60.000000` |
| S12 | H.264 in MPEG-TS, 30000/1001 |
| S13 | S2 with the video `stts` changed to `100 x 1001, 1 x 1061, 19 x 1001` (29.955 fps) |
| S14 | S2 with the video `stts` changed to `100 x 1001, 1 x 1002, 19 x 1001` (29.9698 fps) |
| S16 | S3 with both DefaultDuration elements changed to EBML Void |
| S17 | Theora and Vorbis in Ogg, 30 fps |
