# Colour and HDR fields: root causes (issue #67)

Research date: 2026-10-18. Map issue: #65. Corpus run: 2026-10-17.

This file groups the 52 colour and HDR patterns of the corpus run into root causes. Each cause has one of these verdicts:

- Bug. go-mediainfo is wrong and must change.
- Decision. go-mediainfo follows a newer MediaInfo on purpose, but `AGENTS.md` does not record it. ADR 0001 rule 2 requires a recorded reason, or a revert to v23.04.
- Gap. The cause is a listed gap in `AGENTS.md`.
- Out of scope. The pattern is not a colour or HDR field.

The file refers to corpus files by C-id and to patterns by G-id. It names no release, path, or tag value.

## Sources and method

- go-mediainfo: `origin/main` at `6a37300`. Citations are `internal/mediainfo/<file>:<line>` at that commit.
- MediaInfoLib: tag `v23.04` of `~/github/oss/MediaInfoLib`. Citations are `Source/MediaInfo/<file>:<line>` at that tag, unless the text says "master".
- Corpus data: the HTML archive of the 2026-10-17 run, parsed with the local triage scripts. Counts are files that Go can parse.
- Synthetic files: small files that `ffmpeg`, `x264` and `x265` made on the local machine. They contain no library material. The section "Synthetic files" lists them as S1 to S12.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The Go binary is a local `linux/amd64` build of `6a37300` at `/tmp/go-mediainfo-research-67`. The research copied the synthetic files to the host and read no library file. The commands were `mediainfo --ParseSpeed=0.5 <f>`, `mediainfo --ParseSpeed=0.5 --Language=raw <f>`, `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5 <f>`, and the same three outputs from the Go binary. Each command ran under `ionice -c3 nice -n10 timeout 300`.

In this file, "text" is the default text output. "Raw text" is the text output with `--Language=raw`. CSV and HTML use the same labels as text, so they follow text.

## Summary

| Cause | Verdict | Patterns | Files (about) |
|---|---|---|---|
| R1. Colour and HDR facts reach JSON but not text | Bug | G00056, G00057, G00072, G00107, G00538, G00539, G00540, G00079 and G00082 (BDAV part), G01119 | 320 |
| R2. Colour names come from the wrong tables | Bug | G00174, G00177, G00346, G00202, G00203, G01117, G01386, G00587 and G00523 (MP4 part), parts of G00056 and G00057 | 150 |
| R3. MPEG-2 colour comes from the last sequence header, not the first | Bug | G00227, G00228, G00260, G00261, G00262, G00289, G00298, G00301 (MPEG-PS and DVD part), 4 files of G00174 and G00177 | 35 |
| R4. AVC `colour_description_present` without a colour description | Bug | G00227, G00228 (MP4 part) | 15 |
| R5. HDR output shape follows MediaInfo v26, not v23.04 | Decision (one part is a bug) | G00078, G00079, G00080, G00081, G00082, G00083, G00065, G00066, G00101 (Matroska part), G00104 | 300 |
| R6. MPEG-4 Visual and H.263 in MP4 get no picture facts | Bug | G00423, G00536, G00629, G00630, G01498, parts of G00056, G00057, G00072, G00107, G00405, G00416, G00568 | 10 |
| R7. Mastering luminance uses the wrong decimals | Bug | G00649 (2 of 3 files) | 2 |
| R8. Matroska Dolby Vision profile 5 colour is hard-coded | Bug | G00260, G00261, G00262 (Matroska part), G00523, G00603, G00607 (Matroska HEVC part), G00587 (Matroska part), G00405 and G00416 (Matroska HEVC part) | 4 |
| R9. Raw text duplicates `transfer_characteristics_Original` | Bug | G00204 | 54 |
| R10. HDR10+ and Dolby Vision in MP4 HEVC | Gap | G00101 (MP4 part), G01436 | 2 |
| R11. Quantization matrix fields | Out of scope | G00241, G00410, G00535 | 60 |
| Not explained | Needs a witness file | G00883, G00302, G00405 (WebM part), G00523 and G00603 (AV1, VP9 part), G01450, G01451, part of G00538 to G00540, 1 file of G00649 | 35 |

## R1. Colour and HDR facts reach JSON but not text

### What goes wrong

On several parser paths, Go stores the colour facts as structured facts with no text label. JSON and raw text show the values. Text, CSV and HTML do not show the lines `Color range`, `Color primaries`, `Transfer characteristics` and `Matrix coefficients`. On MPEG-TS and BDAV with HEVC, text also loses `HDR format`, `Mastering display color primaries`, `Mastering display luminance`, `Maximum Content Light Level` and `Maximum Frame-Average Light Level`.

upbrr pastes the text output into tracker uploads. These uploads lose all colour and HDR lines for these files.

The corpus shows the effect on text, CSV and HTML only. JSON and raw text match official for most of these files.

| Pattern | Files with a text-only difference | Containers and codecs |
|---|---|---|
| G00056 `Matrix coefficients` | 319 of 324 | MPEG-PS MPEG-2 124, MP4 HEVC 123, MP4 AVC 45, Matroska MPEG-2 12, BDAV AVC 7, BDAV HEVC 7, MP4 VP9 1 |
| G00057 `Color primaries` | 321 of 323 | same set |
| G00072 `Transfer characteristics` | 286 of 288 | same set, MP4 HEVC 90 |
| G00107 `Color range` | 174 of 175 | MP4 HEVC 123, MP4 AVC 43, BDAV HEVC 7, MP4 VP9 1 |
| G00538, G00539, G00540 HDR format and mastering | 7 of 8 | BDAV HEVC |
| G00079, G00082 light levels (text part) | 6 | BDAV HEVC |

Raw text is not affected, because the raw text projection reads the structured map directly (`field_project_raw_text.go:820-828`, `field_project_raw_text.go:871`). Text shows only the entries that have a text label (`field_project_text.go:40-44`).

### The paths

1. HEVC in MP4. `parseMP4HEVCSampleEntryDetails` writes the VUI colour only to the structured facts (`mp4_codec.go:682-701`). The text fields from `parseHEVCConfig` have no colour lines.
2. MP4 `colr` box. `mergeMP4ContainerColorFact` writes the container value to the structured facts only (`mp4_codec.go:627-645`). `applyMP4ContainerColorText` writes a text line only when a conflicting `_Original` value exists (`mp4_codec.go:421-441`). When the colour comes only from the `colr` box, text shows nothing. This explains the 45 MP4 AVC files. They are phone recordings whose VUI probably has no colour description. It also explains why G00072 has 33 fewer MP4 HEVC files than G00057. Those 33 files have a conflict and get a text line.
3. AVC in MPEG-TS and BDAV. `buildH264Fields` adds the three colour lines only when `includeColorDescription` is set (`h264.go:230-240`). The Annex B callers do not set it (`h264.go:761-763`, `mpeg_ts.go:5407-5409`). Only the MP4 `avcC` caller sets it (`h264.go:172-174`). `Color range` does not depend on the option, so G00107 has no BDAV AVC files.
4. HEVC in MPEG-TS and BDAV. The colour and HDR facts go to `streamFacts` only (`mpeg_ts.go:1758-1800`).
5. MPEG-2 video. MPEG-PS (`mpeg_ps.go:1129-1144`) and MPEG-TS (`mpeg_ts.go:2104-2119`) write the sequence display extension to the structured facts only. Matroska calls `replaceCanonicalSeedFill` with an empty display value, so no text entry exists (`matroska_scan.go:2833-2856`).
6. Matroska HLG with a BT.2020 transfer. The block at `matroska_scan.go:2486-2498` sets `transfer_characteristics_Original` with an empty display value, and text loses the line (G01119, 2 files). This path is probable. No synthetic file reproduced it.

Official fills each of these values once, and the same value goes to every output. See `File_Avc.cpp:953-966`, `File_Hevc.cpp:433-447`, `File_Mpegv.cpp:1389-1397` and `File_Mpeg4_Elements.cpp:6916-6928`.

### Proof

Host runs on synthetic files. Official text shows all four colour lines for each file.

| File | Go text shows |
|---|---|
| S1, HEVC in MP4 | `Color space` only |
| S2, AVC in MP4, colour only in `colr` | `Color range` only |
| S3, AVC in M2TS | `Color range` only |
| S4, HDR10 HEVC in M2TS | `Color space` only. All HDR lines are missing. Official shows `HDR format`, both mastering lines and both light level lines. |
| S5, HDR10 HEVC in MP4 | the HDR lines, but no colour lines |
| S6, MPEG-2 in Matroska | no `Matrix coefficients` |
| S7, MPEG-2 in MPEG-PS | no colour lines |

Go JSON for these files has the same colour keys as official JSON.

### Verdict

Bug. One fix point is enough for all paths. Each colour or HDR structured fact needs a text entry with the MediaInfo label. The fix can go in the shared builder, or in one finalization step that adds the missing text entries from the structured keys.

Witness files: C000180, C000255, C000254, C000143, C000060 (BDAV HEVC), C000205 (MP4 HEVC), C521853 (MP4 AVC, `colr` only), C000009 (BDAV AVC), C000025 (MPEG-PS MPEG-2), C000070 (Matroska MPEG-2), C000086 (Matroska HLG).

## R2. Colour names come from the wrong tables

### What goes wrong

Official uses one set of name tables for every codec and container: `Mpegv_colour_primaries`, `Mpegv_transfer_characteristics`, `Mpegv_matrix_coefficients` and `Mpegv_matrix_coefficients_ColorSpace` (`File_Mpegv.cpp:34-110`). The AVC parser (`File_Avc.cpp:960-964`), the HEVC parser (`File_Hevc.cpp:441-445`), the MP4 `colr` parser (`File_Mpeg4_Elements.cpp:6920-6924`) and the Matroska parser (`File_Mk.cpp:4175`) all call these tables.

Go has three sources of names:

1. A Matroska-style table (`matroska.go:3477-3560`). The AVC VUI (`h264.go:464-466`), the HEVC VUI (`hevc_sps.go:502-505`) and the MP4 `colr` box (`mp4_codec.go:571-600`) use it.
2. An MPEG-2 table (`mpeg2_video.go:998-1055`).
3. Patches that rename some values on some paths. They are in `matroska.go:2187-2192`, `matroska_scan.go:2834-2843`, `dvd.go:2144-2151`, `dvd.go:2562-2569`, `field_project_raw_text.go:1583-1592`, and in `mp4ColorPrimariesName` and `mp4MatrixName` for value 6 (`mp4_codec.go:571-600`).

The patches do not cover all paths and values. The table shows examples from the corpus.

| Value | Official | Go | Where |
|---|---|---|---|
| primaries 6 | `BT.601 NTSC` | `SMPTE 170M` | Matroska AVC text (50 files), MP4 AVC (2) |
| matrix 6 | `BT.601` | `SMPTE 170M` | Matroska AVC text (52), MP4 AVC (7) |
| transfer 6 | `BT.601` | `SMPTE 170M` | all outputs, Matroska AVC (2), MP4 AVC (4), MP4 `_Original` (54) |
| primaries 5 | `BT.601 PAL` | `BT.470 BG` or `BT.470BG` | MPEG-PS (9), MP4 AVC (5), Matroska AVC (3) |
| transfer 5 | `BT.470 System B/G` | `Gamma 2.8` or `BT.470BG` | MPEG-PS (9), Matroska AVC (2) |
| matrix 5 | `BT.470 System B/G` | `BT.470 BG` or `BT.470BG` | MPEG-PS (9), Matroska AVC (2), MP4 AVC (1) |
| primaries 4 | `BT.470 System M` | `BT.470 M` | MPEG-PS (1) |
| transfer 13 | `sRGB/sYCC` | `IEC 61966-2-1` | MP4 AVC (1) |
| transfer 14 | `BT.2020 (10-bit)` | `BT.2020 10-bit` | Matroska HEVC text (2) |
| matrix 4 | `FCC 73.682` | missing | MPEG-PS (2: C000681, C277690) |
| matrix 0 | `Identity`, and `ColorSpace` `RGB` | missing, and `ColorSpace` `YUV` | MP4 AVC (1: C610838, G01386) |
| primaries 12 | `Display P3` | missing | MP4 AVC (1: C610646) |

The different names also cause false conflicts in MP4. The VUI matrix 6 is `SMPTE 170M` and the `colr` matrix 6 is `BT.601`. `mergeMP4ContainerColorFact` sees two values and writes a `matrix_coefficients_Original` that official does not have (G01117, G00587 and G00523 MP4 part, `mp4_codec.go:636-643`).

### Proof

- S7 (MPEG-2 PAL in MPEG-PS). Official JSON has `BT.601 PAL`, `BT.470 System B/G`, `BT.470 System B/G`. Go JSON has `BT.470 BG`, `Gamma 2.8`, `BT.470 BG`.
- S10 (AVC NTSC in Matroska). Official text has `BT.601 NTSC`, `BT.601`, `BT.601`. Go text has `SMPTE 170M` three times. Go JSON has `SMPTE 170M` for the transfer.
- S11 (HEVC in MP4, `colr` BT.709, VUI value 6). The official `_Original` values are `BT.601 NTSC`, `BT.601`, `BT.601`. Go has `SMPTE 170M` three times.

### Verdict

Bug. Port the four MediaInfoLib tables once and use them on every path. Then remove the patches. The fix also gives `ColorSpace` `RGB` for matrix 0.

Witness files: C000045 (Matroska AVC), C001256 and C000681 (MPEG-PS), C521942, C528365 and C609131 (MP4 AVC), C518295 and C521913 (MP4 `_Original`), C610838, C610646, C000086.

## R3. MPEG-2 colour comes from the last sequence header

### What goes wrong

Official fills the MPEG-2 colour in `Streams_Fill` (`File_Mpegv.cpp:1389-1397`). The flag starts false one time (`File_Mpegv.cpp:1812`). The values come from the sequence headers that the parser saw before it filled the stream, which is the start of the file.

Go reads the sequence display extension each time and keeps the last one (`mpeg2_video.go:445-468`). Other sequence facts stop at the first bounded-read jump (`sequenceFactsFrozen`, `mpeg2_video.go:172-175`), but the colour code does not check that flag. So the colour can come from the end of the file.

The corpus shows two effects:

1. Go shows a colour description where official shows none. This happens on 23 MPEG-PS files and 8 DVD Video files (G00227, G00228, G00260, G00261, G00262, G00289, G00298, G00301). Example witnesses are C000527 and C000006.
2. Go shows a different value. Examples are PAL against NTSC (C001451, C279247), and `BT.601` against `BT.470 System B/G` (C002287, C002289).

### Proof

- S8 (MPEG-PS, first half without colour, second half PAL). Official shows no colour in any output. Go JSON shows the PAL values.
- S9 (MPEG-PS, first half PAL, second half NTSC). Official shows PAL. Go shows NTSC.

### Verdict

Bug. Apply the colour values only while `sequenceFactsFrozen` is false, as for the other sequence facts.

## R4. AVC `colour_description_present` without a colour description

Official fills `colour_description_present` only when the VUI flag `colour_description_present_flag` is set (`File_Avc.cpp:957-959`). Go sets it when the VUI has only the range flag (`mp4_codec.go:267-270`, `mp4_codec.go:318-321`). The MPEG-TS paths use the same condition (`mpeg_ts.go:1730-1732`, `mpeg_ts.go:1758-1760`, `mpeg_ts.go:1971-1973`), but no MPEG-TS corpus file shows this difference.

The corpus has 15 MP4 AVC files with G00227 and G00228. These files have no other colour difference. The witness is C608415.

Verdict: bug. JSON gets two keys that official does not have.

## R5. HDR output shape follows MediaInfo v26

### What goes wrong

PR #20 (`a8ce7aa`) validated against a MediaInfo v26.05 corpus. Commit `e410715` added the Dolby Vision summary. Several HDR fields have the newer shape, and `AGENTS.md` records none of them.

1. `MaxCLL` and `MaxFALL` are numbers in JSON (`"2279"`). v23.04 puts the unit in the value (`"2279 cd/m2"`, `File__Analyze_Streams.cpp:612-623`). MediaInfoLib master stores a number (`File__Analyze_Streams.cpp:990-1015`, master). Go fills the number in `matroska_video_canonical.go:785`, `mp4_probe.go:233`, `mpeg_ts.go:1795` and `matroska_scan.go:2370`. Patterns: G00078, G00081 (271 files, JSON and XML).
2. Raw text uses the labels `MaxCLL/String` and `MaxFALL/String` (`field_project_raw_text.go:123`, `field_project_raw_text.go:827-828`). v23.04 uses `MaxCLL` and `MaxFALL`. Patterns: G00079, G00082 (raw text part), G00080, G00083. The triage sheet lists G00080 and G00083 as "newer MediaInfo emits it".
3. JSON has `MasteringDisplay_Luminance_Min` and `MasteringDisplay_Luminance_Max` (G00065, G00066, 300 files). v23.04 does not have these fields. v26 has them.
4. The Dolby Vision text summary adds `Profile 8.1` and `no metadata compression` (`dolby_vision.go:212-243`). JSON adds `HDR_Format_Compression` (G00104). v23.04 builds the summary from version, profile, level and settings only (`File__Analyze_Streams_Finish.cpp:823-862`). Its field list has no `HDR_Format_Compression` (`Source/Resource/Text/Stream/Video.csv:34-41` at v23.04). Pattern G00101 covers 179 Matroska files, text only. The JSON `HDR_Format` value is the same.
5. The raw text HDR summary puts a profile name where the version goes. Go writes `SMPTE ST 2094 App 4, Version HDR10+ Profile B, HDR10+ Profile B compatible` and `SMPTE ST 2086, Version HDR10, HDR10 compatible` (`field_project_raw_text.go:1696-1711`, also `mp4_probe.go:196`). Official raw text and Go text both show `Version 1`. A profile name is not a version.

Items 1 to 4 do not change the JSON values that upbrr reads for HDR detection (`HDR_Format`, `transfer_characteristics`, `colour_primaries`, `matrix_coefficients`, `colour_range`). A user that parses the unit from JSON `MaxCLL` gets no unit.

### Verdict

Items 1 to 4 need a decision. The maintainer must either add one line per family to the deliberate list in `AGENTS.md` with the reason (ADR 0001 rule 2), or change the output back to v23.04. A number without a unit is the more correct structured value, and v26 agrees, so a recorded deliberate difference is defensible.

Item 5 is a bug in every version. The raw summary must use the version from the stream, as the text summary does.

Witness files: C000043, C000060, C000075, C000079, C000053, C000393.

## R6. MPEG-4 Visual and H.263 in MP4 get no picture facts

`isVideoSampleEntry` does not list `s263` (`mp4_codec.go:193-199`). H.263 in MP4 skips the visual sample entry parse, so Go never reads the `colr` box. On C528255, official shows the container colour, `colour_range` and `ColorSpace`, and Go shows none.

For `mp4v`, the MP4 path does not call `parseMPEG4Visual`. Only AVI (`avi.go:333`) and Matroska (`matroska_scan.go:3340`) call it. Go loses `ColorSpace`, `Format_Settings_Matrix` (`Default (H.263)`) and the colour description. Also, `parseMPEG4VOL` does not read `video_signal_type`, so the VOL colour is lost in every container. Official fills all of them (`File_Mpeg4v.cpp:444-455`).

Patterns: G00423 (MPEG-4 Visual 8, H.263 1), G00536 (8), G00629, G00630, G01498, and the all-output parts of G00056, G00057, G00072, G00107, G00405, G00416, G00568. Witnesses: C608641, C608340, C528255.

Six AVI files also miss `ColorSpace` (G00423). Official takes it from the codec ID database for RGB codecs (`File_Riff_Elements.cpp:1795-1811`). This research did not trace the Go AVI path. The witness is C608393.

Verdict: bug.

## R7. Mastering luminance uses the wrong decimals

Official writes the minimum with 4 decimals. It writes the maximum with 0 decimals when the value is a whole number, else with 4 (`File__Analyze_Streams.cpp:414-418`). Go picks the decimals from the size of each value: 0 decimals from 100, 1 from 10, 2 from 1, else 4 (`matroska.go:3563-3587`). C000393 and C000415 store the minimum and the maximum in the wrong order. Official writes `min: 1000.0000 cd/m2`, and Go writes `min: 1000 cd/m2`. A maximum such as 4000.5 also differs (official `4000.5000`, Go `4001`). The pattern is G00649.

Verdict: bug.

## R8. Matroska Dolby Vision profile 5 colour is hard-coded

For a Dolby Vision profile 5 track, Go writes a fixed colour block with full range, BT.2020, PQ and `IPT-PQ-C2`, and with `Container` and `_Original_Source` provenance (`matroska_scan.go:2471-2484`). Go does not read these values from the file. Official v23.04 does not show this block, and v26 also does not (the triage sheet marks G00260 to G00262 as "extra in v26 too").

The four corpus files are C000053, C000201, C005004 and C633810. They give G00260, G00261 and G00262 (Matroska part), G00523, G00603 and G00607 (`Stream` against `Container`), G00587 (Matroska part), and the XML-only parts of G00405 and G00416.

Two nearby blocks make text values from the presence of HDR metadata, not from the file (`matroska_scan.go:2502-2516`). They write `Transfer characteristics` `PQ`, `Color range` `Limited`, `Matrix coefficients` `BT.2020 non-constant`, and `Color primaries` with the mastering display primaries. Mastering display primaries and colour primaries are different facts. No corpus pattern shows these blocks, because they fill only empty text lines. After the R1 fix, they are not necessary.

Verdict: bug. One part needs a witness. Official shows `matrix_coefficients_Source` `Stream` on these files (G00523) but no `matrix_coefficients` key (G00261). The witness file must show what official reads.

## R9. Raw text duplicates `transfer_characteristics_Original`

Official raw text cuts the label to 32 characters, `transfer_characteristics_Origina`. Go writes the cut label (`field_project_raw_text.go:824`). Go also writes the full label, because `applyMP4ContainerColorText` adds a text field with the full key (`mp4_codec.go:439`). In the S11 host run, Go writes both lines and official writes only the cut line. The pattern is G00204, on 54 MP4 files. The value difference in G00202 is R2.

Verdict: bug.

## R10. HDR10+ and Dolby Vision in MP4 HEVC (listed gap)

`AGENTS.md` lists "HDR10 and Dolby Vision metadata for HEVC in MP4" as not implemented. Two corpus files fall in that gap:

- C521934. Official detects HDR10+ (`SMPTE ST 2094 App 4`, `HDR_Format_Version` `0`). Go reports only `SMPTE ST 2086`. Patterns: G00101 (MP4 all-output part), G01436.
- C000211. The file has Dolby Vision with HDR10 and HDR10+ in MP4. The text summary differs (G00101 MP4 text part).

Static HDR10 metadata in MP4 HEVC already works. On S5, Go JSON matches official JSON for `HDR_Format`, the mastering display and the light levels. The other differences on S5 are R1 and R5. The gap line in `AGENTS.md` can name HDR10+ and Dolby Vision only.

The MP4 HEVC patterns of R1, R2 and R9 are not in the gap. They are about the colour description, not about HDR metadata.

## R11. Quantization matrix fields (out of scope)

The family filter matches the word "matrix", so it includes MPEG-2 quantization matrix fields. These are G00241 (`Format_Settings_Matrix_Data`, 40 MPEG-PS files), G00410 (16 files) and G00535 (`Custom` against `Default`, 8 files). They are not colour fields and belong with the format and codec naming cluster. G00536 is R6.

A hint for that ticket: Go keeps the first non-default matrix (`mpeg2_video.go:352-356`, `mpeg2_video.go:486-497`). R3 shows that the choice of sequence header matters for MPEG-2 in MPEG-PS.

## Not explained

These patterns need a witness file. The source and the synthetic files do not settle them.

- G00883. `HDR_Format_Compatibility` `HDR10` is missing in Go (C000165, C278286, Matroska HEVC).
- C278286. Go has no `HDR_Format`, mastering display or source fields in any output (G00538, G00539, G00540 Matroska part, G01450, G01451).
- G00649 on C003218. Official shows a mastering maximum of 4000, and Go shows 1000. Official keeps the first mastering metadata that it finds (`File__Analyze_Streams.cpp:400-401`). The file probably has two sources with different values.
- G00302. Go adds `ColorSpace` `YUV` on 18 DVD Video files (C000006), 10 WebM VP9 files (C521817) and 1 MP4 AVC file (C609458). For VP9, official fills `ColorSpace` only from a first frame with colour data (`File_Vp9.cpp:299-313`).
- G00405 WebM part. `matrix_coefficients_Source` is missing in Go on 9 VP9 files.
- G00523, G00603, G00607, G00905 and G01497 on AV1 and VP9 (C000200, C000222, C610827). Official writes `Container` and Go writes `Container / Stream`, or the reverse.

## Proposed bug tickets

1. Colour and HDR lines missing from text output. Which shared step must add a text entry for each colour and HDR structured fact, so that MP4, MPEG-TS, BDAV, MPEG-PS and Matroska MPEG-2 show the same lines as JSON? Witnesses: C000180, C000205, C521853, C000009, C000025.
2. One colour name table for all parsers. Can a port of the MediaInfoLib `Mpegv_*` colour tables replace the Matroska-style table, the MPEG-2 table and the rename patches? Witnesses: C000045, C001256, C521942, C518295, C610838.
3. MPEG-2 colour from the first sequence header. Must the sequence display extension stop updating after `sequenceFactsFrozen`, so that MPEG-PS and DVD match official? Witnesses: C000527, C001451, C002287.
4. AVC `colour_description_present` only with a colour description. Must the MP4 and MPEG-TS AVC paths require the VUI colour description flag, as `File_Avc.cpp:957-959` does? Witness: C608415.
5. Raw HDR summary writes a profile name as the version. Must the raw text HDR summary use the stream version (`Version 1`) instead of the profile name? Witnesses: C000075, C000079.
6. MPEG-4 Visual and H.263 in MP4. Must the MP4 path parse `s263` entries and the MPEG-4 Visual VOL, with `video_signal_type`, for `ColorSpace`, the quantization matrix and the colour? Witnesses: C608641, C528255.
7. Mastering luminance decimals. Must the luminance string use 4 decimals for the minimum and 0 or 4 decimals for the maximum, as `File__Analyze_Streams.cpp:414-418` does? Witnesses: C000393, C000415.
8. Hard-coded Matroska HDR colour. Must the Dolby Vision profile 5 block and the HDR-presence colour blocks in `matroska_scan.go:2471-2516` go, with the values read from the file? Witnesses: C000053, C000201.
9. Duplicate raw `transfer_characteristics_Original`. Must raw text show only the cut label, as official does? Witness: C518295.

R5 items 1 to 4 need a maintainer decision, not a bug ticket.

## Questions for the contributor (witness files)

1. C000053. Which colour keys does official JSON show for this Dolby Vision profile 5 track? G00523 says that official has `matrix_coefficients_Source`, and G00261 says that official has no `matrix_coefficients`.
2. C278286 and C000165. Where is the HDR10 metadata in the file: in the Matroska `Colour` element, in an SEI message, or in both?
3. C003218. Does the file have two mastering display records, one in the container and one in the stream, with maximum 4000 and 1000?
4. C000006 and C521817. Which official field explains the missing `ColorSpace` on DVD Video and WebM VP9?
5. C521853. Does the video VUI of this phone recording set `video_signal_type_present_flag`? R1 path 2 expects no VUI colour data.

## Synthetic files

Each file is 1 to 30 seconds of `testsrc`, made with `ffmpeg`. The local scratch scripts are not in the repository.

| ID | Content |
|---|---|
| S1 | HEVC (x265) in MP4, BT.709 in the VUI and in `colr` |
| S2 | AVC (x264) in MP4, VUI colour set to unspecified, BT.709 only in `colr` |
| S3 | AVC in M2TS, BT.709 |
| S4 | HEVC in M2TS, PQ, BT.2020, mastering display SEI, light level SEI 1000 and 400 |
| S5 | S4 content in MP4 |
| S6 | MPEG-2 in Matroska, BT.709 |
| S7 | MPEG-2 in MPEG-PS, PAL colour (value 5) |
| S8 | MPEG-PS with 30 s without colour, then 30 s of S7 |
| S9 | MPEG-PS with 30 s of S7, then 30 s with NTSC colour (value 6) |
| S10 | AVC in Matroska, NTSC colour (value 6) |
| S11 | HEVC in MP4, VUI value 6, `colr` BT.709 |
| S12 | S4 content in Matroska (local run only) |
