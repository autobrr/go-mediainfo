# Research: MPEG video fields (issue #70)

Date: 2026-10-18. Parent map: #65. Rules: ADR 0001 and `AGENTS.md`.

This note answers one question. Which MPEG video fields does go-mediainfo get wrong, and which root causes produce them? The fields are scan type, scan order, picture structure, GOP settings, time code, GOP open or closed, standard, bit depth and buffer size. Each cause gets a verdict: bug, deliberate, decision, or out of scope.

## Method

- go-mediainfo: `main` at `6a37300`. Line numbers below refer to this commit.
- MediaInfoLib: tag `v23.04`, read through the GitHub API. Line numbers below refer to `v23.04`, except where a line says "v26.05". The local checkout is v26.05.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The JSON command was `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` against `go-mediainfo --output=JSON --language=raw`. The text command was `mediainfo --ParseSpeed=0.5` against `go-mediainfo`, with the `ReportBy` line removed.
- Real files: parity samples `dvd-01` (a title-set IFO), `dvd-02` (a 10 MB title VOB), and `dvd-01-menu`. `dvd-01-menu` is a 6 MB menu VOB in the same disc folder as `dvd-01`. No other file from the media library was read.
- Synthetic files: S1 to S13 and A1 to A3. `ffmpeg` made them from `testsrc2`. A small script changed flag bits in some of them. They contain no library content. The table at the end lists them.
- Corpus data: the pattern list of the 2026-10-17 run. The witness C-ids come from it. Only the contributor can open those files.

## Summary

| Cause | Patterns | Verdict |
|---|---|---|
| M1 Text labels and text values for MPEG video fields | G00094, G00106 (TFF part), G00208 (Matroska), G00230 (Matroska), G00484 (Matroska), G00896, text part of G00091 | Bug |
| M2 MPEG-2 scan type, scan order and picture structure rules | G00091, G00149, G00256, G00263, G00486 (MPEG-PS), G00106 (pulldown part), G01427 | Bug |
| M3 MPEG-2 time code trust and GOP open or closed rules | G00132 (MPEG-PS part), G00251 (MPEG-PS part), G00186, G00163, G00230 (MPEG-PS), G00484 (MPEG-PS), G00514 | Bug |
| M4 MPEG-2 GOP setting (`Format_Settings_GOP`) rule | G00124 (MPEG-PS), G00164 (MPEG-PS), G00255 (MPEG-PS) | Bug |
| M5 MPEG-2 `Standard` rule | G00427, G00497, G00565 | Bug |
| M6 DVD VOB normalizers replace or delete parser values | G00145 (MPEG-PS), G00149 (MPEG-PS), G00274, G00208 (MPEG-PS), part of G00230, G00094, G00124, G00497 | Bug |
| M7 MPEG-2 `BufferSize`: first header, no size extension | G00470 (MPEG-PS) | Bug |
| M8 Matroska `FieldOrder` element is not read | G00486 (Matroska), G00579, G00896, G00149 (Matroska) | Bug |
| M9 DVD title-set joining and IFO stream copy | G00128, G00364, G00365, the DVD parts of G00091, G00132, G00163, G00186, G00251, G00255, G00256, G00263, most of G00514 | Existing ticket #116, with #111 |
| M10 AVC and HEVC time code from SEI | G00132 (Matroska, MPEG-4, BDAV part, about 92 files) | Decision, for #83 |
| M11 MPEG-4 Visual in MP4 has no VOL facts | G00145 (MPEG-4), G00422 (MPEG-4), G00533, G00536 | Existing ticket #98 |

upbrr reads `ScanType` and `Standard` to tell PAL from NTSC. M2, M5 and M6 change these two fields, so they come first.

## M1: Text labels and text values for MPEG video fields

Patterns: G00094 (217 files, MPEG-PS and Matroska), G00106 (177 files, the `TFF` part), G00208 (the 19 Matroska files), G00230 (the 10 Matroska files), G00484 (the 4 Matroska files), G00896 (2 files). Witnesses: C000025, C000070, C000093 (G00094), C000014, C000017, C000082 (G00106), C000070, C000199, C000239 (G00208), C000185, C529429 (G00896).

Official behavior:

- The text label of `Format_Settings_PictureStructure` is `Format settings, picture structure`, with a lowercase `p` (`DefaultLanguage.csv:445`).
- The text value of `ScanOrder` is a translation. `TFF` becomes `Top Field First`, and `BFF` becomes `Bottom Field First` (`File__Analyze_Streams_Finish.cpp:1780-1783`, `DefaultLanguage.csv:700`, `DefaultLanguage.csv:704`).
- The text value of `ScanType_StoreMethod` is a translation. `SeparatedFields` with 2 fields per block becomes `Separated fields (2 fields per block)` (`File__Analyze_Streams_Finish.cpp:1770-1777`, `DefaultLanguage.csv:1215-1218`).
- `Time code source`, `GOP, Open/Closed` and `GOP, Open/Closed of first frame` show in text when the field is set (`File__Analyze_Streams_Finish.cpp:1823-1827`).

Go behavior:

- Go uses the label `Format settings, Picture structure` (`mpeg_ps.go:708-709`, `matroska_scan.go:2766-2767`, `mpeg_ts.go:1855`, `dvd.go:2289`). The text order table has no entry for this label, so the line prints at the end of the Video section.
- Go writes the raw value `TFF` as the text value (`mpeg_ps.go:1021-1024`, `matroska_scan.go:2796-2798`, `matroska_scan.go:2569`, `matroska_video_canonical.go:325`, `mpeg_ps_canonical.go:96`, `mp4_codec.go:932`). Only two DVD rules write the translated text (`dvd.go:2418`, `dvd.go:2582`).
- Go writes the raw value `SeparatedFields` as the text of `Scan type, store method` (`matroska_scan.go:2213`).
- The Matroska MPEG-2 path writes `TimeCode_Source`, `Gop_OpenClosed` and `Gop_OpenClosed_FirstFrame` with an empty text label (`matroska_scan.go:2805`, `matroska_scan.go:2818`, `matroska_scan.go:2821`). The JSON has the fields. The text does not.

Evidence:

- S1 (MPEG-PS): official text has `Format settings, picture structure : Frame` after the GOP line, and `Scan order : Top Field First`. Go text has `Scan order : TFF`, and `Format settings, Picture structure : Frame` as the last line of the section. The JSON values are equal.
- S1 as Matroska: official text has `Time code source`, `GOP, Open/Closed` and `GOP, Open/Closed of first frame`. Go text has none of the three lines. The JSON values are equal.

Verdict: bug. The text output goes into tracker uploads.

## M2: MPEG-2 scan type, scan order and picture structure rules

Patterns: G00091 (229 files, Go adds the picture structure), G00149 (98 files, Go drops the scan order), G00256 (36 files, Go adds the scan order), G00263 (34 files, Go adds the scan type), G00486 (the 3 MPEG-PS files), G00106 (the `2:3 Pulldown` to `TFF` part), G01427 (1 file). Witnesses: C000006, C000008, C000019 (G00091), C000426, C000520, C000522 (G00149), C000013, C000016 (G00256, G00263), C002292, C290089 (G00486), C278314 (G01427).

Official behavior (`File_Mpegv.cpp:1315-1372`, counters at `File_Mpegv.cpp:3867-3906`):

1. The scan type is `Progressive` when `progressive_sequence` is set, or when every picture has `progressive_frame` set. One missing picture is allowed.
2. If some pictures are progressive and some are not, the stream is mixed. Then official writes no scan type, no scan order and no picture structure.
3. The scan order is `TFF` or `BFF` only if all pictures have the same `top_field_first` value. For a progressive stream, official writes a scan order only when `progressive_sequence` is 0.
4. The picture structure is written only for an interlaced stream, and only if all pictures are frame pictures or all are field pictures.
5. Official reports `2:3 Pulldown` when the display-order flags contain the exact cadence `T2T3B2B3T2T3B2B3`. It then sets the scan type to `Progressive`. This test does not depend on `progressive_sequence`.

Go behavior (`mpeg2_video.go:846-870`, counters at `mpeg2_video.go:555-571`):

1. The scan type is `Progressive` only when `progressive_sequence` is set. DVD and MPEG-TS also accept 95 percent progressive pictures (`mpeg2_video.go:850`). Plain MPEG-PS does not.
2. Go has no mixed case. Go always writes a scan type.
3. Go writes `TFF` when one picture has `top_field_first` set (`mpeg2_video.go:859-861`). Go never writes `BFF`, except through a DVD rule (`dvd.go:2579-2584`).
4. Go takes the most frequent picture structure (`mpeg2_video.go:821-824`).
5. Go tests for pulldown only when the sequence is progressive or 95 percent of the pictures are progressive (`mpeg2_video.go:862-870`). The looser cadence test is already #112.

Evidence (synthetic MPEG-PS on the host):

- S2, interlaced, all pictures bottom field first: official `ScanOrder` `BFF`. Go has no scan order.
- S5, interlaced sequence with all pictures progressive: official `Progressive` and `TFF`, no picture structure. Go `Interlaced`, `TFF`, and picture structure `Frame`.
- S6, every second picture progressive: official has no scan type, no scan order and no picture structure. Go has `Interlaced`, `TFF` and `Frame`.
- S7, top field first on every second picture: official `Interlaced` with no scan order. Go `Interlaced` and `TFF`.
- S13, NTSC with soft 2:3 pulldown flags in display order: official `Progressive`, `2:3 Pulldown`. Go `Interlaced`, `TFF`.

Verdict: bug. The pulldown part shares the fix of #112. The fix must copy the official cadence test and drop the `progressive_sequence` gate. G01427 (Go `Field`, official `Frame`) probably comes from rule 4, but it needs a stream with field pictures. `ffmpeg` cannot make one.

## M3: MPEG-2 time code trust and GOP open or closed rules

Patterns: G00132 (the 20 MPEG-PS files), G00251 (the 20 MPEG-PS files), G00186 (65 files), G00163 (83 files), G00230 (the 35 MPEG-PS files), G00484 (the 7 MPEG-PS files), G00514 (9 files). Witnesses: C000006, C000008, C000013 (G00132, G00186, G00163), C000016 (G00251), C000070, C000093 (G00230), C000093, C001550 (G00484), C000830, C000831, C001401 (G00514).

Official behavior:

- If two GOP headers in a row both have the time code `00:00:00:00`, official marks the time code as not trustable (`File_Mpegv.cpp:4037-4043`). If a time code goes back, official also marks it as not trustable (`File_Mpegv.cpp:2516-2520`).
- `TimeCode_FirstFrame`, `TimeCode_Source`, `Gop_OpenClosed`, `Gop_OpenClosed_FirstFrame` and the stream delay are filled only inside one block. That block runs only for a trustable time code (`File_Mpegv.cpp:1427-1459`).
- `Gop_OpenClosed` needs 4 GOPs or more. The mix must also be coherent: all closed, all open, or only the first one closed (`File_Mpegv.cpp:1439-1453`). Else official writes neither field.
- `TimeCode_Source` is written only when the video is inside a container (`File_Mpegv.cpp:1457-1458`).

Go behavior:

- Go keeps the time code of the first GOP header and always sets `TimeCode_Source` (`mpeg2_video.go:621-631`). Go has no trust rule.
- Go writes `Open` when any GOP is open, and `Closed` for the first GOP when it is closed (`mpeg2_video.go:808-818`). MPEG-PS has no 4-GOP rule and no coherence rule. The Matroska path has both (`matroska_scan.go:2725-2731`).
- MPEG-PS writes the open or closed fields only when `GOPLength > 1` (`mpeg_ps.go:1040-1048`). A GOP that Go calls variable has `GOPLength` 0, so the fields disappear. Official writes them for a variable GOP too.

Evidence:

- S4, S2 with every GOP time code set to `00:00:00:00`: official has no time code, no time code source and no GOP open or closed. Go has all four. The Matroska and MPEG-TS copies of S4 give the same result.
- S12, S1 with the time code going back after GOP 2: official has no time code and no GOP open or closed. Go has `10:00:00:00` and `Open`.
- `dvd-02`: the first two GOP headers have `00:00:00:00`. Official has no time code and no `Gop_OpenClosed`. Go has `00:00:00:00` and `Closed`.

Verdict: bug. G00132 and G00251 also have a DVD IFO part (M9) and an AVC part (M10).

## M4: MPEG-2 GOP setting rule

Patterns: G00124 (the 98 MPEG-PS files), G00164 (the 5 MPEG-PS files), G00255 (the 6 MPEG-PS files). Witnesses: C000067, C000147, C000689 (G00124), C000100, C000873 (G00164).

Official behavior:

- A GOP is the string of picture types from one I picture to the next, in decode order. The second field of a field pair does not count (`File_Mpegv.cpp:2366-2397`).
- With 4 GOPs or more, official takes the most frequent string. If fewer than half of the GOPs and fewer than 4 GOPs differ from it, official writes `M=x, N=y`. N is the length of the string. M is the longest run of B pictures plus 1. Else official writes `Variable` (`File_Mpegv.cpp:1597-1647`, the limit of 4 at `File_Mpegv.cpp:1065`).

Go behavior:

- Go counts pictures between GOP headers, between I pictures, and between anchor pictures, in maps (`mpeg2_video.go:686-724`).
- If the N count or the M count has more than one value, Go sets both M and N to 0 (`mpeg2_video.go:771-774`). A short last GOP is enough. The output then becomes `N=x` without M (`mpeg_ps.go:695-702`).
- The GOP length is fixed when one length covers 50 percent of the GOPs (`mpeg2_video.go:751-755`). MPEG-TS uses 95 percent (`mpeg2_video.go:882-906`).
- A sampled file needs 8 GOPs or more before Go shows the GOP (`mpeg_ps.go:689`). DVD rules force `Variable` after a PTS reset (`mpeg_ps.go:692-694`, `dvd.go:312`), and add `M=3, ` to every `N=` value when B pictures exist (`dvd.go:2586-2590`). The Matroska path always adds `M=3, ` (`matroska_scan.go:2756-2758`).

Evidence:

- S1, S2 and S3 (15-picture GOPs, 2 B pictures, a short last GOP): official `M=3, N=15`. Go `N=15`. S8 and S10: official `M=3, N=12`. Go `N=12`.
- S9 (closed GOPs at irregular key frames): official `Variable`. Go `N=10`.

Verdict: bug. The value `M=3, N=24` where official has `M=3, N=12` probably comes from field pictures, because Go counts each field as a picture. A witness with field pictures is necessary to prove it.

## M5: MPEG-2 Standard rule

Patterns: G00427 (15 files, Go drops `Standard`), G00497 (10 files, the values differ), G00565 (7 files, Go adds `NTSC`). Witnesses: C000022, C000520, C000522, C000523 (G00427), C000555, C000556, C000557, C000869 (G00497), C001434, C001435, C001436, C277376 (G00565).

Official behavior:

- `Standard` is the `video_format` value of the sequence display extension: `Component`, `PAL`, `NTSC`, `SECAM` or `MAC` (`File_Mpegv.cpp:1388`, table at `File_Mpegv.cpp:332-341`). Without that extension, `video_format` is 5, which gives an empty value (`File_Mpegv.cpp:1794`).
- If `Standard` is still empty, a generic rule applies. Width 720 or 704 with height 576 and frame rate `25.000` gives `PAL`. Height 480 or 486 with frame rate `29.970` gives `NTSC`. Width 352 has the same test with heights 288, 576, 240, 243, 480 and 486 (`File__Analyze_Streams_Finish.cpp:768-782`). A 2:3 pulldown stream has the rate `23.976`, so it gets no fallback.

Go behavior:

- Without a display extension, Go uses the frame rate only. Above 50 fps gives `Component`, 29.97 or 30 fps gives `NTSC`, and 25 fps gives `PAL` (`mpeg2_video.go:965-978`). Go does not test the width or the height. Go uses the coded rate, not the rate after pulldown.
- MPEG-PS skips this fallback for a sampled file (`mpeg_ps.go:997-1004`).
- For a DVD VOB, Go sets `Standard` from the height alone. 576 gives `PAL` and 480 gives `NTSC`. This replaces the `video_format` value. For 480 lines at 23.976 fps with `2:3 Pulldown`, Go deletes `Standard` (`dvd.go:2559-2577`), also when the stream has `video_format` `NTSC`.

Evidence:

- S10 (1280x720, 59.94 fps, no display extension): official has no `Standard`. Go `Component`.
- S11 (1920x1080, 29.97 fps, no display extension): official has no `Standard`. Go `NTSC`.
- S13 (720x480 with soft pulldown, no display extension): official has no `Standard`, because the rate becomes 23.976. Go `NTSC`.
- `dvd-02` (one display extension with `video_format` NTSC, 2:3 pulldown): official `NTSC`. Go has no `Standard`.

Verdict: bug. upbrr reads this field to detect PAL and NTSC. The DVD part also needs M6.

## M6: DVD VOB normalizers replace or delete parser values

Patterns: G00145 (94 MPEG-PS files), G00149 (MPEG-PS part), G00274 (33 files), G00208 (the 33 MPEG-PS files), part of G00230, G00094 (17 files), G00124 (the 45 joined-VOB files), G00497 (DVD part). Witnesses: C000426, C000520, C000522, C000523 (G00145), C000426, C000681, C000755, C000869 (G00274).

Official behavior: MediaInfoLib has no DVD-specific rule for these video fields. A VOB goes through `File_MpegPs` and `File_Mpegv` like any program stream. A one-frame menu still keeps its scan type and scan order.

Go behavior. After the parse, Go changes the video fields of every VOB:

- `suppressDVDShortVideoScan` deletes `ScanType` and `ScanOrder` when the video duration is less than 3 s (`dvd.go:2486-2495`). The duration comes from Go. On a VOB with a PTS reset, Go measures only the last segment (#118), so a long VOB can fall below 3 s.
- `normalizeDVDMenuVOBStreams` deletes these fields from every menu VOB: the picture structure, `Delay_Original` and the related delay fields, the time code, the time code source, `Gop_OpenClosed`, `Gop_OpenClosed_FirstFrame` and `BitRate_Maximum` (`dvd.go:2281-2314`).
- `normalizeDVDVideoFields` sets `Standard` from the height and deletes it for pulldown (M5). It also writes `BFF` when an interlaced stream has no scan order (`dvd.go:2579-2584`), and adds `M=3, ` to the GOP (`dvd.go:2586-2590`).
- `alignDVDHeaderAudioStreams` writes `Interlaced`, `TFF` and `Closed` for a one-frame video (`dvd.go:2411-2420`).
- A PTS reset forces the GOP to `Variable` (`mpeg_ps.go:692-694`).

Evidence:

- `dvd-02`: official `ScanType` `Progressive`, `ScanOrder` `2:3 Pulldown`, `Standard` `NTSC`. Go has none of the three. Go video duration is 1.066 s, official 9.713 s.
- `dvd-01-menu` (one still picture, 0.033 s in both tools): official `Progressive` and `TFF`. Go has neither.
- The corpus: all 33 files of G00274 also have G00208, G00240, G00267, G00268 and G00269. These are exactly the fields that `normalizeDVDMenuVOBStreams` deletes.

Verdict: bug. These rules copy the shape of some official outputs, not the official logic. When M2, M3, M4 and M5 follow the official rules, the DVD rules can go. #118 is a separate bug, but it triggers the 3 s rule.

## M7: MPEG-2 BufferSize

Patterns: G00470 (the 4 MPEG-PS files). JSON and XML only. Witnesses: C001014, C001029, C001038.

Official behavior: `BufferSize` is `2 * 1024 * ((vbv_buffer_size_extension << 10) + vbv_buffer_size_value)` (`File_Mpegv.cpp:1471`). The extension comes from the sequence extension (`File_Mpegv.cpp:3747`). Official fills the stream after its first frames, so it uses the header values that are current at that time.

Go behavior: Go keeps the first non-zero `vbv_buffer_size_value` (`mpeg2_video.go:388-390`). Go skips `vbv_buffer_size_extension` (`mpeg2_video.go:434`).

Evidence:

- S8 (1080i, VBV buffer of 20000000 bits): official `2500608`. Go `403456`. Go lost the upper bits.
- `dvd-02`: the first sequence header is a one-frame lead-in with a buffer of 4 units. All later headers have 112 units. Official `229376`. Go `8192`.

Verdict: bug. #95 says that official keeps the colour of the first sequence header. The fixes for #95 and for this cause must use one rule for the "current header at fill time".

## M8: Matroska FieldOrder element is not read

Patterns: G00486 (the 8 Matroska files), G00579 (6 files), G00896 (2 files), G00149 (the Matroska file). Witnesses: C002292, C290089, C633433, C663537 (G00486), C000082, C000100, C000258 (G00579), C000185, C529429 (G00896).

Official behavior: the Matroska `FieldOrder` element sets the container scan type to `Interlaced` for the values 1, 6, 9 and 14. It sets `TFF` or `BFF`. It sets `ScanType_StoreMethod` to `SeparatedFields` (values 1 and 6, 2 fields per block) or `InterleavedFields` (values 9 and 14) (`File_Mk.cpp:4324-4366`). The container values win over the stream values when they differ.

Go behavior: the Matroska parser does not read `FieldOrder`. Go writes a store method only for AVC field pictures (`matroska_scan.go:2212-2213`), and it writes the raw value (M1).

Evidence: S6 as Matroska (`ffmpeg` writes `FieldOrder`). Official `Interlaced`, `TFF`, and `Scan type, store method : Separated fields (2 fields per block)`. Go has no scan type, no scan order and no store method. S1 as Matroska: official has the store method line. Go does not.

Verdict: bug. The same element affects AVC and HEVC in Matroska.

## M9: DVD title-set joining and IFO stream copy

Patterns: G00128 (133 files), G00364 (20 files), G00365 (20 files), and the DVD parts of G00091, G00132, G00163, G00186, G00251, G00255, G00256, G00263, and 7 of the 9 files of G00514. Witnesses: C000522, C000523, C000556, C000557 (G00128), C000006, C000008, C000013, C000016 (DVD IFO).

All 133 files of G00128 also have G00099 or G00100, the patterns of title-set joining. Go reads the time code of the first VOB of the set, so all VOBs of one set show the same Go value. This explains why one Go value repeats with different official values. The DVD IFO files get `BitDepth`, `BufferSize`, GOP, scan and time code fields from the VOB files. Official v23.04 reads only the IFO.

`dvd-01` (IFO) shows it. Official has only `Format_Version`, `Height` and `Standard`. Go adds the GOP, `BitDepth`, `ScanType`, `ScanOrder`, the time code, `Gop_OpenClosed` and `BufferSize`.

Verdict: existing ticket "DVD: decide whether to join the VOB files of a title set" (#116), and "DVD IFO: stop copying VOB rates and the menu frame rate" (#111). No new ticket.

## M10: AVC and HEVC time code from SEI

Patterns: G00132 for Matroska AVC (63 files), Matroska HEVC (15), MPEG-4 AVC (8), BDAV (6). Witnesses: C000011 and other Matroska files of G00132.

Official behavior: v23.04 `File_Avc.cpp` and `File_Hevc.cpp` never fill `TimeCode_FirstFrame`. v26.05 fills it from the picture timing SEI (`File_Avc.cpp:2701-2706` and `File_Hevc.cpp:1496-1501`, v26.05). The corpus marks G00132 as "extra in v26 too", but the pattern mixes MPEG-2 and AVC files, so the v26.10 data must be split by codec.

Go behavior: Go reads the SEI time code for Matroska AVC (`matroska_scan.go:2205`, `matroska_scan.go:2264-2266`), Matroska HEVC (`matroska_scan.go:2348`) and MP4 AVC (`analyze.go:750-756`).

Verdict: decision. If v26.10 shows the field for these files, the value is real data and the line can go to the deliberate differences list. #83 reviews the v26.10 families.

## M11: MPEG-4 Visual in MP4

Patterns: G00145 (the 8 MPEG-4 files), G00422 (the 8 MPEG-4 Visual files), G00533, G00536. The MP4 path does not read the VOL header, so the scan type, bit depth, BVOP and matrix are missing. Verdict: existing ticket "MP4: parse the MPEG-4 Visual VOL header" (#98).

## Patterns that this research does not explain

- AVC GOP in MP4, Matroska and BDAV: G00124 (MPEG-4 31, Matroska 6), G00164 (MPEG-4 60, Matroska 10, BDAV 3), G00255 (BDAV 8, MPEG-4 3). The synthetic AVC files A1, A2 and A3 give the same GOP value in both tools. The cause is in `h264_gop.go` or in the bounded MP4 probe, but a witness is necessary.
- G00470 for AVC (MPEG-4 7, Matroska 1): official `29999616`, Go `30000000 / 30000000`, and the reverse in another file. Official joins the NAL and VCL HRD buffer sizes (`File_Avc.cpp:972-996`). Go writes one value from another source (`mp4_codec.go:900`).
- G00588: Matroska HEVC, official 10 bits, Go 8 bits (5 files). This is not MPEG video.
- G00422 for AVI (RGB, JPEG, AVC, 6 files) and G00145 for AVI (2 files).
- G01427 and the `N=24` value of G00124 need a stream with field pictures (see M2 and M4).

## Side findings

- `modeValue` (`mpeg2_video.go:1143-1156`) iterates over a Go map. When two values have the same count, the result changes from run to run. The GOP length, M, N, the picture structure and `intra_dc_precision` use it.
- MPEG-PS calls `finalize` on the same MPEG-2 parser two times (`mpeg_ps.go:416` and `mpeg_ps.go:511`). The first call changes the parser state, for example the frame rate after pulldown.
- MPEG-TS (S1 and S4 as TS): Go has no `Standard`, shows GOP `Variable` where official shows `M=3, N=15`, and has no `Gop_OpenClosed`. The corpus has only 4 MPEG-TS files.
- Raw MPEG-2 video (S1 as `.m2v`): Go has no `Standard` and no scan order.
- Writer-specific rules delete `Standard` in Matroska for HandBrake and for `Lavf58.45.100` (`analyze.go:2490`, `analyze.go:2507`, `analyze.go:2512`). An MP4 rule changes the time code separator when the handler name starts with `BAMTech ` (`analyze.go:750-753`). These rules follow sample files, not the format.
- `dvd-02`: Go reports a video stream size of 79734137 bytes for a file of 10176512 bytes. This is #116 and #120.

## Synthetic files

Each file is a 6 to 10 s clip from `testsrc2`. `ffmpeg` encoded it with `mpeg2video` and muxed it with `-f vob`. A Python script changed bits in the picture coding extension or the GOP header for S4 to S7, S12 and S13.

| ID | Content |
|---|---|
| S1 | 720x576, 25 fps, interlaced TFF, GOP 15 with 2 B pictures, time code 10:00:00:00, `video_format` PAL |
| S2 | 720x480, 29.97 fps, interlaced BFF, time code from 00:00:00:00, no display extension |
| S3 | 720x480, 29.97 fps, progressive sequence, no display extension |
| S4 | S2 with every GOP time code set to 00:00:00:00 |
| S5 | S1 with `progressive_frame` set on every picture |
| S6 | S1 with `progressive_frame` set on every second picture |
| S7 | S1 with `top_field_first` inverted on every second picture |
| S8 | 1920x1080, 29.97 fps, interlaced, VBV buffer 20000000 bits, `video_format` component, 19.5 MB (sampled by Go) |
| S9 | S1 settings with closed GOPs at irregular key frames |
| S10 | 1280x720, 59.94 fps, progressive, no display extension |
| S11 | 1920x1080, 29.97 fps, progressive, no display extension, time code 00:59:59:00 |
| S12 | S1 with GOP 3 and later set to 00:00:01:00, so the time code goes back |
| S13 | S2 with soft 2:3 pulldown flags in display order and `progressive_frame` set |
| A1 to A3 | `libx264` in MP4: fixed GOP 24, default GOP, and fixed GOP 12 with 3 B pictures |

S1, S4 and S6 were also remuxed to Matroska, and S1 and S4 to MPEG-TS. S1 and S4 also ran as raw `.m2v`.
