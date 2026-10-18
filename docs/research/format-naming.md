# Format, codec and container naming: root causes (issue #71)

Research date: 2026-10-18. Map issue: #65. Corpus run: 2026-10-17.

This file groups the format, codec, profile, format settings and container naming patterns of the corpus run into root causes. Each cause has one of these verdicts:

- Bug. go-mediainfo is wrong and must change.
- Existing ticket. An open bug ticket already has the cause.
- Decision. go-mediainfo follows newer MediaInfo, or it can be more correct than v23.04. The maintainer must decide. ADR 0001 rule 2 requires a recorded reason, or a revert to v23.04.
- Out of scope. The pattern is not a naming difference, or it is in a container that Go does not parse.

The file refers to corpus files by C-id and to patterns by G-id. It names no release, path, or tag value.

## Sources and method

- go-mediainfo: `origin/main` at `6a37300`. Citations are `internal/mediainfo/<file>:<line>` at that commit.
- MediaInfoLib: tag `v23.04`, fetched from the MediaArea/MediaInfoLib repository on GitHub. Citations are `Source/MediaInfo/<file>:<line>` at that tag, unless the text says "master". "master" is the local checkout, which is newer than v23.04.
- Corpus data: the HTML archive of the 2026-10-17 run, parsed with the local triage scripts. The counts are files that Go can parse.
- Synthetic files: small files that `ffmpeg`, `x264` and `x265` made on the local machine. They contain no library material. The section "Synthetic files" lists them as S1 to S28.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The Go binary is a local `linux/amd64` build of `6a37300` at `/tmp/go-mediainfo-research-71`. The commands were `mediainfo --ParseSpeed=0.5 <f>`, `mediainfo --ParseSpeed=0.5 --Language=raw <f>`, `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5 <f>`, and the same three outputs from the Go binary. Each command ran under `ionice -c3 nice -n10 timeout 300`. One real file was read: sample `dvd-01`.

In this file, "text" is the default text output. "Raw text" is the text output with `--Language=raw`. CSV and HTML use the same labels as text, so they follow text.

### Scope

The file covers 80 patterns. 77 are the candidate patterns that the triage scripts put in the family "format and codec naming" (Audio 31, Video 33, General 10, Text 3). The other three are the quantization matrix patterns G00241, G00410 and G00535 from #67.

## Summary

| Cause | Verdict | Main patterns | Files (about) |
|---|---|---|---|
| F1. Text `Format/Info` and the text Format name do not follow the official Format table | Bug | G00108, G00144, G00335, G00594, TrueHD part of G00311, BDAV part of G00355, BDAV part of G00253 | 330 |
| F2. Text has no `Codec ID/Info` and no `Codec ID/Hint` | Bug | G00070, G00328, G00215, G00246 | 400 |
| F3. Format facts reach JSON but not text | Bug | G00129 (most), G00179, G00253, G00259, G00317, BDAV part of G00164 | 230 |
| F4. The raw text `Format_Settings` row has invented values | Bug | raw parts of G00250 and G00225, Matroska part of G00555 | 35 |
| F5. Wrong text labels and values | Bug | G00196 (text), MPEG-PS part of G00129, text parts of G00091 and G00094, G01424, G00873 | 260 |
| F6. HEVC tier and VC-1 level are separate text lines | Bug | G00121 (most), G00131, Matroska part of G00323 | 140 |
| F7. Profile, level and tier come from `hvcC` and `avcC`, not from the SPS | Bug | G00371, G00476, parts of G00121 | 20 |
| F8. "1 frames" | Bug | G00119 | 147 |
| F9. MPEG-2 GOP, BVOP and picture structure rules | Bug | MPEG-PS parts of G00124, G00164, G00255, JSON parts of G00091 and G00094, G01427, G00576, Matroska part of G00225 | 180 |
| F10. MPEG-2 quantization matrix rules | Bug | G00241, G00535, MPEG-PS part of G00410, matrix parts of G00225 | 60 |
| F11. AVC GOP rules | Bug | AVC parts of G00124, G00164, G00255 | 120 |
| F12. MP3 mode extension rule | Bug | G00434, G00435, G01425 | 15 |
| F13. AC-3 surround mode taken from the first frame | Bug (needs a witness) | Matroska parts of G00196, G00555, G00645, one file of G00129 | 3 |
| F14. MP4 AAC uses a reduced AudioSpecificConfig reader | Bug | G00556, G00593, G00617, G00644, G01426, MP4 AAC parts of G00129, G00311, G00355, G00504 | 10 |
| F15. MP4 PCM sample entries are not recognized | Bug | G00865, G00877, G00878, MP4 PCM parts of G00129 and G00355 | 2 |
| F16. MP4 `ftyp` handling | Bug | MPEG-4 part of G00023, G00611, G00612, G00866, MP4 part of G00356 | 8 |
| F17. WebM DocType is not read | Bug | WebM part of G00023 | 71 |
| F18. Matroska CodecID keeps the trailing zero byte | Bug | WebM parts of G00504, G00458, G00532, G00179, G00355 | 1 |
| F19. AVI codec table and payload coverage | Bug | AVI parts of G00532, G00458, G00328, G00355, G00317, G00510, G00511, and G00876, G01421, G01423, G01428, G01432 | 10 |
| F20. DVD IFO output copies VOB format facts | Existing ticket #111 | DVD parts of G00255, G00250, G00323, G00410, G00091, G00356, G00108, G00253, G00196, G00555, and G00409, G00433, G01420, G01422 | 20 |
| F21. HEIF image files | Decision | heic part of G00023, G00482, G00483 | 11 |
| F22. `vp09` in MP4 shows as VP9 | Decision | one file of G00532 | 1 |
| F23. AV1 film grain in Matroska text | Decision | Matroska part of G00250 | 4 |
| F24. E-AC-3 JOC in Matroska | Decision (needs a witness) | G00874, G00875, E-AC-3 parts of G00311 and G00355 | 2 |
| F25. MPEG-PS subpicture track missing | Out of scope | G00509, G00512 | 9 |
| MPEG-4 Visual in MP4 | Existing ticket #98 | G00458 (MP4), G00510, G00511, G00529, G00533, G00534, G00537, MP4 part of G00317 | 8 |
| H.263 in MP4 | Existing ticket #92 | G01385, H.263 parts of G00532, G00510, G00511 | 1 |
| MP3 and Vorbis in an `mp4a` entry | Existing ticket #113 | MP4 parts of G00253, G00259, G00355, G00504, G00645, G00129, G00108 | 5 |
| Not explained | Needs a witness file | G01429, one file of G00355, the AAC part of G00504, the doubled N in G00124, one file of G00335 | 10 |

### Patterns in unsupported containers

G00023 has 652 files in the corpus. 566 of them are the files where Go reports General Format `Unknown`. These are the Blu-ray metadata files (out of scope in #65), and the Flash Video, Windows Media, SubRip, ISO 9660, ZIP and raw H.263 files ("Not yet specified" in #65). This file does not count them. The 86 files that remain are WebM (F17), heic (F21) and QuickTime (F16).

No other pattern in this family has a file in an unsupported container.

## F1. Text `Format/Info` and the text Format name do not follow the official Format table

### What goes wrong

Text shows `Format/Info` lines that official does not show, shows the wrong text, or misses the line. On BDAV, the text Format name of TrueHD misses the core and the 16-channel feature.

| Pattern | Effect | Files |
|---|---|---|
| G00108 | Audio `Format/Info` for Vorbis, Opus, MPEG Audio, PCM, Linear PCM | 172 |
| G00144 | Video `Format/Info` for VP8, VP9, MPEG Video, VC-1, MPEG-4 Visual | 108 |
| G00311 (TrueHD part) | "Dolby TrueHD" where official has "Meridian Lossless Packing FBA" | 19 |
| G00594 | BDAV TrueHD has no `Format/Info` | 5 |
| G00335 | General `Format/Info` missing: BDAV 20, FLAC 3, AVI 1 | 24 |
| G00355 (BDAV part) | Text Format "MLP FBA" where official has "MLP FBA AC-3 16-ch" | 3 |
| G00253 (BDAV part) | AC-3 text `Format profile` "Blu-ray Disc" missing | 1 |

### Root cause

Official has one rule. When a parser fills `Format`, MediaInfoLib fills `Format/Info` from the Info column of the Format table (`File__Analyze_Streams.cpp:1175-1178`). For MPEG Video, MPEG-4 Visual, VC-1, VP8, MPEG Audio, Opus and Vorbis this column is empty (`MediaInfo_Config_Automatic.cpp:1446`, `1447`, `1449`, `1451`, `1487`, `1501`, `1505`). PCM and VP9 have no row. BDAV has "Blu-ray Video" (`MediaInfo_Config_Automatic.cpp:1394`) and FLAC has "Free Lossless Audio Codec" (`MediaInfo_Config_Automatic.cpp:1475`).

For AAC, AC-3, E-AC-3 and MLP, the function `HighestFormat` makes the text values from `Format`, `Format_Profile` and `Format_AdditionalFeatures`. It makes the Format name with the additional features (`MediaInfo_Internal.cpp:600-640`), the "Blu-ray Disc" profile (`MediaInfo_Internal.cpp:740-750`), and "Meridian Lossless Packing FBA" with "with 16-channel presentation" (`MediaInfo_Internal.cpp:758-800`).

Go has no Format table. Each parser writes its own `Format/Info` string:

- `mapMatroskaFormatInfo` returns a value for VC-1, VP9, VP8, Opus, MPEG Audio, PCM, Vorbis and MPEG Video, and "Dolby TrueHD" for TrueHD (`matroska_codec.go:68-115`). Matroska (`matroska_video_canonical.go:131`), MPEG-PS (`mpeg_ps.go:556-591`) and other paths call it.
- Hard-coded copies are in `matroska_vorbis_canonical.go:47`, `matroska_opus_canonical.go:47`, `matroska_mpeg_audio_canonical.go:55`, `matroska_audio_canonical.go:152`, `matroska_truehd_canonical.go:47` and `121`, and `dvd.go:454` and `707`.
- MP4 `mp4v` gets "MPEG-4 Visual" (`mp4_codec.go:1260-1273`).
- Raw text has a separate, partial copy of the official rule, with BDAV and MLP FBA (`field_project_raw_text.go:1911-1941`). Text does not use it.

### Proof

- S1 (Matroska): Go text adds `Format/Info` "MPEG Video", "PCM", "MPEG Audio" and "Vorbis". Official has none.
- S2 (WebM): Go adds "Google VP9" and "Vorbis".
- S6, S9 and S24 (MPEG-PS): Go adds "MPEG Audio" to MP2.
- S10 (Matroska TrueHD): official "Meridian Lossless Packing FBA", Go "Dolby TrueHD".
- S4 (MP4 MPEG-4 Visual): Go adds "MPEG-4 Visual".

### Witness files

C608371, C000051, C000182, C000070, C000096, C000235, C000060, C000143, C000009, C000001, C000137

## F2. Text has no `Codec ID/Info` and no `Codec ID/Hint`

### What goes wrong

| Pattern | Effect | Files |
|---|---|---|
| G00070 | Matroska PGS, VobSub and DVB subtitle tracks lose `Codec ID/Info` | 293 |
| G00328 | Matroska MPEG-2 and DivX, and AVI RGB, lose `Codec ID/Info` | 24 |
| G00215 | MP3 and MP2 lose `Codec ID/Hint` (Matroska 26, AVI 23) | 49 |
| G00246 | XviD, DivX and VC-1 lose `Codec ID/Hint` (AVI 30, Matroska 8) | 38 |

All four are text only. JSON does not have these fields.

### Root cause

Official looks up the CodecID tables and fills `CodecID/Info` and `CodecID/Hint` (`File__Analyze_Streams.cpp:3341`). Both are visible in text (`MediaInfo_Config_Automatic.cpp:4728-4729`, `5102-5103`, `5381-5382`). Examples of the table rows are `S_HDMV/PGS` (`MediaInfo_Config_Automatic.cpp:3021`), `S_VOBSUB` (`3020`), `S_DVBSUB` (`3010`), `V_MPEG2` (`1654`), `XVID` (`2577`), `DX50` (`2094`), `DIVX` (`2043`), `A_MPEG/L3` (`2636`) and RIFF `55` (`2813`).

Go fills `Codec ID/Info` in text only for three Matroska text codecs (`matroska_text_canonical.go:33-40`) and for some MP4 entries (`mp4_codec.go:394-396`). Raw text derives both fields from short hand lists (`field_project_raw_text.go:972-996` and `1127-1149`). The comment at `field_project_raw_text.go:1127` says that the "friendly projection historically omitted the description row". MediaInfo v23.04 does not omit it.

### Proof

- S1: official text has `Codec ID/Info` "MPEG 1 or 2 Video" and the MP3 `Codec ID/Hint` "MP3". Go text has neither.
- S17 and S18 (AVI): official has `Codec ID/Hint` "XviD", "DivX 5" and "MP3". Go text has none. Go raw text has the XviD hint.
- S25 (Matroska MPEG-2): same as S1.

### Witness files

C000034, C000035, C000070, C000258, C000167, C000168, C000096, C608393

## F3. Format facts reach JSON but not text

### What goes wrong

Several parser paths store a format fact as a structured fact with no text label. JSON and raw text show it. Text, CSV and HTML do not.

| Pattern | Lost text line | Paths |
|---|---|---|
| G00129 | `Format settings`, for example "Joint stereo / MS Stereo", "Little / Signed", "Big / Signed", "Dolby Surround" | Matroska MP3 22, AVI MP3 19, PCM in Matroska 8, AVI 4, BDAV 5, MPEG-PS 1, BDAV AC-3 2, MPEG-PS MP3 1 |
| G00253, G00259 | MPEG Audio `Format version` and `Format profile` | AVI 23, MPEG-PS 8 |
| G00179 | `Format settings, Floor` | Matroska 34, WebM 36 |
| G00317 | MPEG-4 Visual `Format settings` "BVOP1", "BVOP2", "Custom Matrix" | AVI 21, Matroska 1 |
| G00164 (BDAV part) | `Format settings, GOP` | BDAV AVC 3 |

### Root cause

Official parsers fill `Format_Settings` directly, and each `Fill` appends with " / ". MP3 does it in `File_Mpega.cpp:362-377`, PCM in `File_Pcm.cpp:205-217`, and Vorbis in `File_Vorbis.cpp:229`. `Format_Settings` and `Format_Settings_Floor` are visible in text (`MediaInfo_Config_Automatic.cpp:5080`, `5088`).

These Go paths store the fact without a text label:

- AVI MP3: `avi.go:770-789` (`structuredFacts.SetSame`).
- MPEG-PS MPEG Audio: `mpeg_ps.go:1311-1312` and `1351-1358` (`facts.Set`).
- Matroska MP3: `matroska_scan.go:1964-1975` (empty text label).
- Matroska Vorbis: `matroska_vorbis_canonical.go:92` (`builder.Structured`). The value is also hard-coded to "1". Official reads the floor types from the setup header.
- BDAV AC-3: `mpeg_ts.go:2507` (`streamFacts.Set`).

The colour ticket #93 has the same kind of cause for colour and HDR facts.

### Proof

- S1 and S27 (Matroska MP3): official text has `Format settings` "Joint stereo / MS Stereo". Go text does not. JSON is the same.
- S17 (AVI): Go text loses the MP3 version, profile, settings and hint, and the video `Format settings` "BVOP1". Go JSON has the MP3 facts.
- S6, S13 and S24 (MPEG-PS): Go text loses the MP2 version and profile. Go JSON has them.
- S1, S7 and S25 (PCM in Matroska and AVI): Go text loses "Little / Signed".
- S2 (WebM): Go JSON has `Format_Settings_Floor` "1". Go text has no Floor line.

### Witness files

C000051, C000094, C000167, C000168, C000117, C000010, C000017, C608371, C000045, C000011

## F4. The raw text `Format_Settings` row has invented values

### What goes wrong

Raw text shows `Format_Settings` values that official never writes. These are "BVOPNo" (AVI 8, MPEG-PS 5, DVD 1, Matroska 1 in G00250), "CustomMatrix / BVOP" where official has "CustomMatrix" (MPEG-PS 17 in G00225), and "PNS / PNS" (Matroska 2 in G00555).

### Root cause

`formatRawTextSettings` composes the row from structured facts (`field_project_raw_text.go:1000-1049`):

- It writes "BVOP" plus the value of `Format_Settings_BVOP`, so "No" gives "BVOPNo" (`field_project_raw_text.go:1042-1047`).
- It adds "BVOP" after "CustomMatrix" also when BVOP is "No" (`field_project_raw_text.go:1039-1041`).
- It writes "PNS / PNS" for AAC when SBR is "No (Explicit)" and the track has no bit rate and no title (`field_project_raw_text.go:1017-1019`).

Official writes "BVOP" only when the stream has B-frames (`File_Mpegv.cpp:1461-1468`).

### Proof

- S1 and S24: Go raw text has "BVOPNo". Official has no such row.
- S26 (Matroska AAC LC): Go raw text has "PNS / PNS". Official has no such row.

### Witness files

C000170, C000231, C000189, C000873, C000120

## F5. Wrong text labels and values

### What goes wrong

| Go text | Official text | Patterns | Files |
|---|---|---|---|
| `Format settings, Mode : Dolby Surround` (MPEG-PS AC-3) | `Format settings : Dolby Surround` | G00196 (text), MPEG-PS part of G00129 | 57 |
| `Format settings, Picture structure` | `Format settings, picture structure` | text parts of G00091 and G00094 | 197 |
| `Format_Settings_Floor` (Ogg) | `Format settings, Floor` | G01424, Ogg part of G00179 | 1 |
| `Format settings, GMC : No` (Matroska MPEG-4 Visual) | `Format settings, GMC : No warppoints` | G00873 | 2 |

### Root cause

- MPEG-PS AC-3 uses the label "Format settings, Mode" (`mpeg_ps.go:1427-1430`). Official fills both `Format_Settings` and `Format_Settings_Mode` (`File_Ac3.cpp:1144-1158`), and `Format_Settings_Mode` is hidden in text (`MediaInfo_Config_Automatic.cpp:5085`). The Matroska path is correct (`matroska_ac3_canonical.go:221`).
- The picture structure label has a capital "P" in `mpeg_ps.go:708-709`, `mpeg_ts.go:1855`, `matroska_scan.go:2766-2767`, `dvd.go:2289`, `json_fields.go:263`, `field_project_raw_text.go:65` and `mpeg_ts_canonical.go:197`, `439`. The official label is "Format settings, picture structure" (`MediaInfo_Config_Automatic.cpp:474`).
- Ogg fills `Format_Settings_Floor` with no text label (`ogg.go:435-437`), so text prints the raw name. The official label is "Format settings, Floor" (`MediaInfo_Config_Automatic.cpp:462`).
- `normalizeMatroskaMPEG4VisualSettings` replaces the GMC text with "No" (`analyze.go:2568-2578`).

### Proof

- S6 (MPEG-PS AC-3 with Dolby Surround): official "Format settings : Dolby Surround", Go "Format settings, Mode : Dolby Surround".
- S13 (MPEG-PS) and S25 (Matroska), both interlaced MPEG-2: official "picture structure", Go "Picture structure".
- S19 (Ogg Vorbis): official "Format settings, Floor", Go "Format_Settings_Floor".

### Witness files

C000067, C000520, C000025, C000228, C000070, C000232, C000179, C000189

## F6. HEVC tier and VC-1 level are separate text lines

### What goes wrong

For HEVC in MP4, MPEG-TS and BDAV, Go text shows `Format profile : Main 10@L5.1` and a separate `Format tier : High` line. Official text shows `Format profile : Main 10@L5.1@High` and no tier line. For VC-1 in Matroska, Go text shows `Format profile : Advanced` and a `Format level : 3` line. Official shows `Format profile : Advanced@L3`.

The patterns are G00121 (MP4 HEVC 111, BDAV 7, Matroska VC-1 6), G00131 (MP4 123, BDAV 7) and the Matroska part of G00323 (6).

### Root cause

Official parsers fill `Format_Profile` as "profile@Llevel@tier". JSON and XML split it into `Format_Profile`, `Format_Level` and `Format_Tier` only at output time (`MediaInfo_Inform.cpp:870-897`, and `File__Analyze_Streams.cpp:950-975` when the split option is on).

Go builds the text profile without the tier and adds a tier line in `hevc.go:46-56`, `hevc_annexb.go:10-20` and `mp4_codec.go:907`. The comment "MediaInfo always reports the HEVC tier" is true for JSON only. The Matroska HEVC path is correct (`matroska_video_canonical.go:255-266`). The Matroska VC-1 path writes a `Format level` text line (`matroska_video_canonical.go:93`).

### Proof

- S3 and S28 (MP4 HEVC): official `Main 10@L5.1@High` and `Main@L2@Main`. Go adds the tier line. JSON is the same.
- S28 (the same HEVC stream in Matroska): no difference.
- S21 (MPEG-TS HEVC): the same difference as S3.

### Witness files

C521936, C518295, C000060, C000143, C000205, C000096, C275370

## F7. Profile, level and tier come from `hvcC` and `avcC`, not from the SPS

### What goes wrong

When the decoder configuration header and the SPS disagree, Go shows the header values. Official shows the SPS values.

- G00371 and G00476: MP4 HEVC level "4" becomes "0.5" and tier "Main" becomes "High" (11 files). Matroska HEVC level "5" becomes "0.1" or "0.0" (4 files).
- G00121: the same files in text, for example `Main@L4@Main` becomes `Main 10@L0.5`. Four MP4 AVC files also differ, for example `High@L4.2` becomes `High@L4`.

### Root cause

- HEVC: `parseHEVCConfig` reads the profile from byte 1 and the level from byte 12 of `hvcC` (`hevc.go:22-26`). The Matroska path says "hvcC profile data is authoritative" and uses the SPS only when a value is missing (`matroska_scan.go:2590-2632`). Official reads the `hvcC` header but fills nothing from it (`File_Hevc.cpp:3672-3790`). The profile, level and tier come from the SPS `profile_tier_level` (`File_Hevc.cpp:3317`). Official also detects an old draft layout of the Matroska CodecPrivate and parses it differently (`File_Hevc.cpp:3657-3666`, `3795-3830`). Go reads that layout as the final one. This is the probable cause of the Matroska level "0.1".
- AVC: `parseAVCConfigDetails` uses the `avcC` profile and level bytes first and the SPS only as a fallback (`h264.go:105-145`). Official reads the two bytes but does not fill them (`File_Avc.cpp:4629-4634`). When they differ from the SPS, official writes `MuxingMode` "Container profile=..." (`File_Avc.cpp:4754`).

### Proof

- S15 is an MP4 HEVC file with the `hvcC` level byte changed from 60 to 90. Official keeps `Main@L2@Main` from the SPS. Go shows `Main@L3` and JSON `Format_Level` "3".
- S16 is an MP4 AVC file with the `avcC` level byte changed from 31 to 40. Official keeps `High@L3.1` and adds `Muxing mode : Container profile=High@L4`. Go shows `High@L4` and has no muxing mode.

### Witness files

C518294, C521872, C526889, C007122, C048383, C528380, C608369, C610415, C610723

## F8. "1 frames"

### What goes wrong

G00119: Go text shows `Format settings, Reference frames : 1 frames`. Official shows `1 frame`. MP4 146 files, Matroska 1.

### Root cause

`h264.go:248` and `matroska_video_canonical.go:219` always add " frames". Official uses the measure " frame" with the plural rule of the language file (`MediaInfo_Config_Automatic.cpp:4715`).

### Proof

S14 (MP4 AVC with one reference frame): official "1 frame", Go "1 frames".

### Witness files

C000187, C000220, C518293, C521714

## F9. MPEG-2 GOP, BVOP and picture structure rules

### What goes wrong

- G00124 (MPEG-PS 98, Matroska 1): the GOP differs, for example "Variable" against "M=3, N=12", or "M=2, N=5" against "M=3, N=10".
- G00164 and G00255 (MPEG-PS 11): one side has a GOP and the other side has none.
- JSON parts of G00091 and G00094 (about 50 MPEG-PS files), and G01427: `Format_Settings_PictureStructure` is on one side only, or differs.
- G00576 (MPEG-PS 6): BVOP "No" against "Yes".
- Matroska part of G00225 (16): Go text `Format settings` is "BVOP" where official has "CustomMatrix / BVOP".

### Root cause

Official (`File_Mpegv.cpp:1600-1646`) counts the picture type pattern of each GOP. It needs 4 or more GOPs. It writes "Variable" when the other patterns are half or more of all GOPs. M is the longest run of B pictures plus one, and official writes M only when it is more than 1. Official writes the picture structure only when all parsed pictures are frames or all are fields (`File_Mpegv.cpp:1344-1345`). BVOP is "Yes" when the parser saw a B picture (`File_Mpegv.cpp:1461-1468`).

Go uses other rules:

- `formatMPEG2GOPSetting` writes M only for interlaced streams (`mpeg_ts.go:5344-5355`).
- The GOP and the picture structure use the most frequent value (`mpeg2_video.go:760-780`, `821-823`). A block marked "TS parity" forces M, N and "Frame" on interlaced streams (`mpeg2_video.go:915-929`).
- Matroska adds "M=3, " to every GOP that has no M. It also writes the text "BVOP" when BVOP is known, also when it is "No" (`matroska_scan.go:2747-2757`).

### Proof

- S1 and S11 (Matroska MPEG-2 without B-frames): official GOP "N=12" and no `Format settings` line. Go has "M=3, N=12" and `Format settings : BVOP`.
- S6 (progressive MPEG-PS with B-frames): official "M=3, N=12", Go "N=12".
- S13 (interlaced MPEG-PS with B-frames): official "M=3, N=12", Go "N=12".
- S9 (MPEG-PS with only 3 GOPs): official has no GOP, Go "N=15".

### Witness files

C000067, C000689, C000755, C000770, C001281, C000873, C001357, C000638, C000426, C278314, C282458, C000093

## F10. MPEG-2 quantization matrix rules

### What goes wrong

- G00535 (MPEG-PS 8): `Format_Settings_Matrix` "Custom" against "Default", in both directions.
- G00241 (MPEG-PS 40): `Format_Settings_Matrix_Data` differs. In the corpus values, the official value starts with an intra matrix, and the Go value often starts with a non-intra matrix.
- G00410 (MPEG-PS part, 5): Go has matrix data where official has none.
- G00225 (MPEG-PS, BDAV and TS parts, about 15): "CustomMatrix" is missing from the text `Format settings`, or added to it.

### Root cause

Official (`File_Mpegv.cpp:3604-3633`, `1400-1412`) uses these rules:

- Matrix "Custom" comes from the load flags of the last parsed sequence header. Official does not compare the values with the default matrix.
- The data is the intra matrix, then the non-intra matrix. Each comes from the first sequence header that loads it. The bytes are in bitstream order.

Go (`mpeg2_video.go:20-55`, `308-385`) uses other rules:

- It compares each loaded matrix with the default matrix, and with a "flat" matrix that it also calls default. A loaded matrix that matches is "Default" and gets no data. The comparison uses raster order, but the bitstream sends zigzag order.
- It takes "Custom" or "Default" from the first sequence header.
- The MPEG-TS path writes the text `Format settings` "BVOP" with no matrix part (`mpeg_ts.go:1839`).

### Proof

The proof comes from the source and the shape of the corpus values. S9 has one sequence header and matches, so it does not show the difference between the first and the last header. A synthetic file with two different sequence headers can prove it.

### Witness files

C000520, C000522, C000556, C001014, C001029, C000873, C001102, C000138, C000234

## F11. AVC GOP rules

### What goes wrong

- G00164 (MP4 60, Matroska 10): official has an AVC GOP such as "M=1, N=60". Go has none.
- G00124 (MP4 31, Matroska 6): Go doubles N, for example "M=1, N=16" becomes "M=1, N=32". One MP4 file has "N=1" against "M=0, N=1".
- G00255 (BDAV 8, MP4 3, Matroska 1): Go has a GOP where official has none.

### Root cause

Official `GOP_Detect` (`File_Avc.cpp:4767-4873`) splits the picture types at each I picture. It removes the first GOP when there are more than four, and returns a value only when four or more GOPs are the same. M is the distance from the I picture to the first P picture. For an all-intra stream it writes "N=1".

Go uses other rules: the most frequent value, a value with "confidence", and the earliest GOP (`h264_gop.go:237-420`). It also reads fewer pictures from files with a long GOP.

### Proof

S20 (MP4 AVC, 15 s, GOP 60, no B-frames): official "M=1, N=60", Go has no GOP. The doubled N needs a witness file.

### Witness files

C521853, C521720, C531400, C521886, C000147, C610725, C000009, C521839

## F12. MP3 mode extension rule

### What goes wrong

G00434 (raw text, 14), G00435 (JSON, 14) and G01425 (1): "Joint stereo / MS Stereo" against "Joint stereo", in both directions. Matroska 13, AVI 1, MP4 1.

### Root cause

Official writes the mode extension of the last header that it parsed before it fills the stream (`File_Mpega.cpp:369-373`, `923`). Go Matroska requires MS stereo in 95 % of the observed frames (`matroska_scan.go:1964-1975`). Go AVI tests the prefix of the writing library name (`avi.go:782-788`).

### Proof

From the source only.

### Witness files

C007012, C009491, C009566, C290727, C535532

## F13. AC-3 surround mode taken from the first frame

### What goes wrong

Go has "Dolby Surround EX" where official has none (Matroska parts of G00196 and G00555). The opposite also occurs (Matroska part of G00645, one Matroska file of G00129). About 3 files.

### Root cause

Official parses `dsurexmod` in every frame with the alternate bit stream syntax, so the last parsed frame sets it (`File_Ac3.cpp:2338`, `1149-1158`). Go keeps the first frame that has it (`ac3.go:1339-1345`). MPEG-PS also passes `dependentEAC3=true` for plain AC-3 (`mpeg_ps.go:1428`). The DVD parts of G00196 come from F20.

### Proof

From the source only. A witness file must show which frames carry `dsurexmod`.

### Witness files

C639302, C005054

## F14. MP4 AAC uses a reduced AudioSpecificConfig reader

### What goes wrong

- G00593, G00617 and G00556: four MP4 HE-AAC files lose `Format_Settings_SBR` "Yes (Explicit)", the additional feature "SBR" and `Format_Settings_PS`. Text loses `Format settings : Explicit` (G00129) and the SBR part of `Format/Info` (G00311).
- G00644, G00504 and G00355: three MP4 AAC files have CodecID "mp4a" with no object type, Format "AAC" with no "LC", and no additional features.
- G00556 and G01426 (Matroska, 4): `Format_Settings_PS` "No (Explicit)" is missing or added.

### Root cause

The MP4 path calls `parseESDSProfile`, which uses `parseAACProfileFromASC` (`mp4_codec.go:1088-1112`, `matroska.go:5019-5025`). That function returns only the profile and one SBR flag. The Matroska path has a fuller reader with the explicit SBR and PS modes (`matroska.go:5029-5071`). Official reads the sync extensions and fills SBR and PS for every container (`File_Aac_GeneralAudio.cpp:4261-4284`). Go writes PS "Yes (Explicit)" only when PS is present, so the "No (Explicit)" case is lost.

For the three files with no object type, Go does not read the `esds` box. This needs a witness file.

### Proof

From the source only. The local `ffmpeg` has no HE-AAC encoder.

### Witness files

C608848, C609968, C610021, C610889, C608221, C608348, C610955, C010331, C635355

## F15. MP4 PCM sample entries are not recognized

### What goes wrong

G00865, G00877, G00878, and the MP4 PCM parts of G00129 and G00355: PCM in a QuickTime file gets Format "Audio", no CodecID, and no endianness or sign. Two files.

### Root cause

`isAudioSampleEntry` and `mapMP4SampleEntry` have no `sowt`, `twos`, `lpcm` or other PCM codes (`mp4_codec.go:156-209`). Official maps `lpcm` and `sowt` to PCM (`MediaInfo_Config_Automatic.cpp:2698`, `2718`).

### Proof

S5 (MOV with `sowt` PCM): official Format "PCM", CodecID "sowt", "Little / Signed". Go Format "Audio".

### Witness files

C608641, C610929

## F16. MP4 `ftyp` handling

### What goes wrong

- G00023 (MPEG-4 part, 4): a file with major brand `qt  ` has General Format "QuickTime" in Go and "MPEG-4" in official.
- G00611 and G00612: Go loses the brand version, for example "qt   0000.02 (qt  )" becomes "qt   (qt  )", and has no `CodecID_Version`.
- G00866: Go keeps zero compatible brands, for example "qt  /\0\0\0\0/...".
- JSON `CodecID` is "qt" in Go and "qt  " in official.
- G00356 (MP4 part, 4): General `Format_Profile` such as "3GPP Media Release 5" and "Sony PSP" is missing.

### Root cause

- `DetectFormat` returns "QuickTime" for the `qt  ` brand (`format.go:31-37`). Official takes the Format from the brand table, where `qt  ` is "MPEG-4" with profile "QuickTime" (`File_Mpeg4_Elements.cpp:1641-1643`, `MediaInfo_Config_Automatic.cpp:1617`).
- `parseFtyp` does not read the brand version, keeps zero brands, and maps only five brands to a profile (`mp4_ftyp.go:5-37`). Official adds the version for `qt  ` and skips zero brands (`File_Mpeg4_Elements.cpp:1644-1668`).
- JSON `CodecID` and `CodecID_Compatible` come from a split of the text "Codec ID" value, and the split loses the trailing spaces (`json_fields.go:271-276`).

### Proof

S5 (MOV with brand `qt  ` version 0000.02): official Format "MPEG-4", Codec ID "qt   0000.02 (qt  )", JSON `CodecID` "qt  " and `CodecID_Version` "0000.02". Go has Format "QuickTime", "qt   (qt  )", "qt", and no version.

### Witness files

C608243, C608641, C610725, C610929

## F17. WebM DocType is not read

### What goes wrong

G00023 (WebM part): 71 WebM files show General Format "Matroska".

### Root cause

`parseMatroskaHeader` reads only `DocTypeVersion` (`matroska.go:1242-1270`). Official reads `DocType` and fills "WebM" for "webm" (`File_Mk.cpp:2240-2251`). Many Go functions test `format == "Matroska"`, for example `analyze.go:952`, `2164` and `2203`. The fix must keep the internal container kind separate from the displayed name.

### Proof

S2 (WebM VP9 and Vorbis): official "WebM", Go "Matroska".

### Witness files

C521132, C521817, C608371

## F18. Matroska CodecID keeps the trailing zero byte

### What goes wrong

One WebM file has CodecID "V_VP8\0" and "A_VORBIS\0" in Go. Because of this, the video Format becomes "Video" (G00532), and the Vorbis facts are missing (JSON part of G00179, G00355). The CodecID patterns are G00504 and G00458.

### Root cause

`matroska.go:1789-1791` copies the CodecID bytes as they are. The track name and language strings next to it trim zero bytes (`matroska.go:1781-1787`). EBML (RFC 8794) lets a writer pad a string element with zero bytes.

### Proof

From the source and the corpus values.

### Witness files

C609615

## F19. AVI codec table and payload coverage

### What goes wrong

| Pattern | Effect |
|---|---|
| G00532, G00458, G00328 | RGB shows as Format "DIB" and CodecID "\0\0\0\0". Official has "RGB", "0x00000000" and a Codec ID/Info. MJPG shows as "Motion JPEG", official "JPEG". |
| G01421 | DV has no `Commercial name` "DV". Go Format is the FourCC "DVSD". |
| G00355 (AVI part) | AAC is "AAC", not "AAC LC". |
| G01423, G01428, G01432, G00317 (one file), G00510, G00511 (one file) | AVC in AVI has no profile, level, CABAC, reference frames or `Format/Info`. |
| G00876 | General `Format settings` "BitmapInfoHeader / WaveFormatEx" where official has "BitmapInfoHeader / PcmWaveformat". |

### Root cause

- `mapAVICompression` prefers the `strh` handler, maps only seven codes, and uses the raw code as the Format for all others (`avi.go:1410-1425`). Official uses the RIFF CodecID table. In that table, `MJPG` is "JPEG" (`MediaInfo_Config_Automatic.cpp:2254`), `0x00000000` is "RGB" (`1906`), `dvsd` is "DV" with the hint "Sony" (`2088`), and audio `50` is MPEG Audio version 1 (`2809`).
- The audio path handles only the tags 0x0001, 0x0055, 0x00FF and 0x2000 (`avi.go:610-650`). MP2 (0x0050) gets no Format. AAC gets no profile.
- Go does not parse the AVC payload in AVI.
- The General `Format settings` always says "WaveFormatEx" for audio (`avi.go:405-420`). Official selects WaveFormat, PcmWaveformat, WaveFormatEx or WaveFormatExtensible from the size of `strf` (`File_Riff_Elements.cpp:1302-1303`, `1416-1417`, `1426-1427`, `1577-1578`).

### Proof

- S7 (AVI MJPG and PCM): official "JPEG" and "PcmWaveformat", Go "Motion JPEG" and "WaveFormatEx".
- S8 (AVI RGB): official "RGB" and "0x00000000", Go "\x00\x00\x00\x00".
- S18 (AVI with MP2): official Format "MPEG Audio", Layer 2. Go has no Format.
- S22 (AVI AVC and AAC): official has the AVC profile, CABAC and reference frames, and "AAC LC". Go has none of these, and "AAC".
- S23 (AVI DV): official Format "DV", Commercial name "DV", CodecID "dvsd". Go has "DVSD".

### Witness files

C608393, C608397, C608297, C609329, C000172, C608834, C608624, C608666

## F20. DVD IFO output copies VOB format facts

This is the same root cause as "DVD IFO: stop copying VOB rates and the menu frame rate" (#111).

### What goes wrong

The IFO report has VOB stream facts. For video, these are `Format_Profile`, `Format_Level`, BVOP, GOP, matrix, matrix data and picture structure. For audio, these are `Commercial name`, endianness, muxing mode, `Format/Info` and the MPEG Audio layer. The report also loses the General `Format profile` "Program".

The patterns are the DVD parts of G00255, G00250, G00323, G00410, G00091, G00356, G00108, G00253, G00196 and G00555, and G00409, G00433, G01420, G01422. About 20 files.

### Root cause

For a VTS IFO in a `VIDEO_TS` folder, Go turns on `aggregateMode` (`dvd.go:148-153`). It parses the sibling VOB files with the MPEG-PS parser (`dvd.go:243-263`), and writes "Program" only when aggregate mode is off (`dvd.go:167-170`, `294`). Official fills the IFO streams only from the IFO attributes (`File_Dvdv.cpp:541`, `591-600`, `664-680`).

### Proof

Sample `dvd-01` on the media host. Go text adds `Format profile : Main@Main`, `Format settings : BVOP`, `Format settings, GOP : M=3, N=12`, `Commercial name : Dolby Digital` and `Muxing mode : DVD-Video`, and loses `Format profile : Program`. Go JSON adds `Format_Level`, `Format_Profile`, `Format_Settings_BVOP`, `Format_Settings_GOP`, `Format_Settings_Matrix`, `Format_Commercial_IfAny` and `Format_Settings_Endianness`.

### Witness files

C000006, C000008, C000013, C000016, C000066, C000132, C534799

## F21. HEIF image files (decision)

The patterns are the heic part of G00023 (11), G00482 and G00483. Official v23.04 has no brand table row for `heic`, so it writes the raw brand "heic" as the General Format (`File_Mpeg4_Elements.cpp:1641-1643`). Only `mif1` maps to "HEIF" (`MediaInfo_Config_Automatic.cpp:1596`). Go reports "MPEG-4" and no CodecID. The corpus summaries show image tracks with HEVC tiles and a grid, so these files are still images, not video. The maintainer must decide if HEIF image files are in scope, and if the Format follows v23.04 ("heic") or the table name ("HEIF").

Witness files: C521618, C521625, C523928

## F22. `vp09` in MP4 shows as VP9 (decision)

One file of G00532. Official v23.04 has no CodecID row for `vp09`, so it shows the raw code "vp09". The master table maps `vp09` to VP9 (master `Source/Resource/Text/DataBase/CodecID_Video_Mpeg4.csv:185`). Go shows "VP9" (`mp4_codec.go:156-180`). Go follows newer MediaInfo and is more correct. ADR 0001 rule 2 requires a line in the deliberate differences, or a revert.

Witness file: C000222

## F23. AV1 film grain in Matroska text (decision)

The Matroska part of G00250 (4 files). Go text adds `Format settings : Film Grain Synthesis` (`matroska_scan.go:2315-2318`). Official v23.04 skips the flag (`File_Av1.cpp:415`). Master fills it (master `Source/MediaInfo/Video/File_Av1.cpp:601-602`). The v26.10 presence data also lacks the line, so the master change is newer than v26.10. Go follows master.

Witness files: C000200, C000242, C000247

## F24. E-AC-3 JOC in Matroska (decision, needs a witness)

The patterns are G00874, G00875, and the E-AC-3 parts of G00311 and G00355. In two Matroska files, Go finds JOC (Atmos) and official v23.04 does not. Go can be correct if the JOC data starts after the frames that official parses. A witness file must show where the first JOC frame is.

Witness files: C629543, C671769

## F25. MPEG-PS subpicture track missing (out of scope)

G00509 and G00512 are not naming differences. In these files, official Text #1 is the RLE subpicture stream, and Go Text #1 is a CEA-608 caption stream. Go does not report the subpicture stream, and it also misses one caption stream. These patterns belong with the whole-track text patterns of the map.

Witness files: C001434, C001435, C000066, C000162

## Existing tickets

- "MP4: parse the MPEG-4 Visual VOL header" (#98) explains G00458 (MP4 part), G00510 and G00511 (MP4 parts), G00529, G00533, G00534, G00537, and the MP4 part of G00317. S4 proves the gap. Official has the profile, BVOP, QPel, GMC and matrix, and CodecID "mp4v-20". Go has none of them, and CodecID "mp4v". The CodecID suffix comes from the object type in `esds`, and #98 does not name it yet. When the CodecID is "mp4v-20", official also finds no `Codec ID/Info` row. This explains G00529.
- "MP4: read H.263 sample entries as video" (#92) explains G01385, and the H.263 parts of G00532, G00510 and G00511.
- "MP4: read MP3 in an `mp4a` entry as MPEG Audio" (#113) explains the MP4 parts of G00253, G00259, G00355, G00504, G00645, G00129 and G00108. The same cause explains CodecID "mp4a-DD" for Vorbis (G00504) and the "Ogg" Format of one file (G00355). #113 names only 0x69 and 0x6B, so it must also cover 0xDD.
- "DVD IFO: stop copying VOB rates and the menu frame rate" (#111) explains F20.
- "Text: show the colour and HDR lines that JSON already has" (#93) has the same kind of cause as F3, for colour facts.

## Not explained

These need a witness file:

- G01429: one AVI file has MPEG Audio "Version 2" in official and "Version 1" in Go.
- One Matroska file of G00355: official "DTS ES XCh XLL", Go "DTS XLL".
- The AAC part of G00504 (three MP4 files): Go does not read the `esds` box. See F14.
- The doubled N in the MP4 and Matroska parts of G00124. See F11.
- One AVI file of G00335: the General `Format/Info` "Audio Video Interleave" is missing, although `avi.go:401` writes it on the normal path.
- F7 is the probable cause of the HEVC values in G00371 and G00476. A witness file must show that the `hvcC` header bytes differ from the SPS.

## Side findings

These are outside the corpus patterns of this ticket:

- Go does not parse Matroska `V_MPEG4/ISO/ASP`. Go shows Format "Video" with no profile (S12). Official shows MPEG-4 Visual with all settings.
- HEVC in MPEG-TS has no `Format/Info` "High Efficiency Video Coding" in Go text or raw text (S21).
- AVI MPEG-4 Visual has the text value "MuxingMode_PackedBitstream" where official has "Packed bitstream" (`avi.go:469`, S17).
- AVI MPEG-4 Visual from the `ffmpeg` encoder has level "0" in Go and "1" in official (S18).
- For an AVI file with a lowercase `xvid` FourCC, official v23.04 does not parse the stream and shows Format "xvid". Go parses it as MPEG-4 Visual. Go is more correct here.
- The synthetic runs also show `BitRate_Mode` and `OverallBitRate_Mode` differences. They belong to "Measured size, duration and bit rate" (#66).
- Several Go rules match single observations, not the official rule. Examples are the writing library prefix test in `avi.go:784`, the "PNS / PNS" rule in `field_project_raw_text.go:1017-1019`, the "TS parity" block in `mpeg2_video.go:915-929`, and the forced "M=3" in `matroska_scan.go:2754-2756`.

## Synthetic files

| ID | Content |
|---|---|
| S1 | Matroska: MPEG-2 without B-frames, PCM 16-bit, MP3, Vorbis |
| S2 | WebM: VP9, Vorbis |
| S3 | MP4: HEVC Main 10, level 5.1, High tier |
| S4 | MP4: MPEG-4 Visual with B-frames, AAC |
| S5 | MOV with brand `qt  `: AVC, PCM `sowt` |
| S6 | MPEG-PS (DVD muxer): progressive MPEG-2 with B-frames and GOP 12, AC-3 with Dolby Surround, LPCM, MP2 |
| S7 | AVI: MJPG, PCM |
| S8 | AVI: uncompressed RGB |
| S9 | MPEG-PS (VOB muxer): MPEG-2 with a custom intra matrix, GOP 15, 2 s, MP2 |
| S10 | Matroska: AVC, TrueHD |
| S11 | Matroska: MPEG-2 without B-frames, AC-3 |
| S12 | Matroska: MPEG-4 Visual (`V_MPEG4/ISO/ASP`), AC-3 |
| S13 | MPEG-PS (VOB muxer): interlaced MPEG-2 with B-frames, MP2 |
| S14 | MP4: AVC with one reference frame |
| S15 | MP4: HEVC Main, level 2, with the `hvcC` level byte changed to 90 |
| S16 | MP4: AVC level 3.1, with the `avcC` level byte changed to 40 |
| S17 | AVI: MPEG-4 Visual with FourCC `XVID` and B-frames, MP3 |
| S18 | AVI: MPEG-4 Visual with FourCC `DX50`, MP2 |
| S19 | Ogg: Theora, Vorbis |
| S20 | MP4: AVC, 15 s, GOP 60, no B-frames |
| S21 | MPEG-TS: HEVC Main |
| S22 | AVI: AVC, AAC LC |
| S23 | AVI: DV |
| S24 | MPEG-PS (MPEG-1 system stream): MPEG-1 video, MP2 |
| S25 | Matroska: interlaced MPEG-2 with B-frames, AC-3 |
| S26 | Matroska: AVC, AAC LC |
| S27 | Matroska: AVC, MP3 joint stereo |
| S28 | The same HEVC Main stream in MP4 and in Matroska |
