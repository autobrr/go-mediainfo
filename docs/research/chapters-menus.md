# Research: chapters and DVD menus (issue #79)

Date: 2026-10-18. Parent map: #65. Rules: ADR 0001 (PR #64) and `AGENTS.md`.

This note answers one question. Which chapter and Menu differences of the 2026-10-17 corpus run come from go-mediainfo bugs, and which root causes produce them? The ticket counts 918 patterns on 77 files. Most come from DVD Video IFO files. upbrr reads nothing from the Menu track, but it pastes the text output into tracker uploads. Each cause gets a verdict: bug, deliberate, decision, or out of scope.

In the pattern lists, "official A, Go B" gives the official v23.04 value first and the go-mediainfo value second. A PGC (program chain) is one playback unit in a DVD IFO file. It has a list of cells with play times, and a program map that marks where each chapter starts.

## Method

- go-mediainfo: `main` at `edbef00`. Line numbers below refer to this commit.
- MediaInfoLib: tag `v23.04`, read through the GitHub API. Line numbers below refer to `v23.04`, except where the text says "v26.05". The local checkout is v26.05 (`MediaInfo_Config.cpp:141`). It stands in for newer MediaInfo.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The JSON command was `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` against `go-mediainfo --output=JSON --language=raw`. The text command was `mediainfo --ParseSpeed=0.5` against `go-mediainfo`. The raw text command added `--Language=raw` to the text command.
- Real files: parity samples `dvd-01` and `dvd-03`. Both are title-set IFO files with one title PGC. No other file from the media library was read.
- Synthetic files: S1 to S6. The table at the end lists them. A small script wrote the IFO files S1 and S2. `ffmpeg` made S3 to S6 from `testsrc` and `sine`.
- Corpus data: the pattern pages and the per-file pages of the corpus archive. A per-file page shows at most four value pairs for each pattern. The witness C-ids come from these pages. Only the contributor can open the files.
- A throwaway Go test fed chapter elements to `parseMatroskaEditionEntry` and `appendMatroskaChapterMenus`. It is not in the repository.

## Summary

| Cause | Patterns | Verdict |
|---|---|---|
| K1 DVD: Go makes one Menu per PTT title and drops short PGCs | G00030 (DVD, 25 files), G00312, G00327, 72 chapter time patterns on C001398, C277379 and C633586 | Decision |
| K2 DVD: v23.04 loses the PGCs that follow a padded program map | G00662 | Deliberate, with a line to add |
| K3 DVD: v23.04 truncates each NTSC cell time before it adds the cells | 796 chapter time patterns on 16 DVD files, between G00663 and G01786 | Decision |
| K4 DVD: the Menu stream lists do not come from the PGC of that Menu | G00324, G00361, G00362, G00478, G00558 to G00561, G00597, G00648, G01517 to G01521 | Bug |
| K5 DVD raw text: the subtitle list labels use the v26 shape | G00212, G00213, G00214, G00216, G00217, G00218 | Decision |
| K6 MP4: `Menus` and `Menu For` do not list every track | G00886, G01455, G01524, G01454, G01523 | Bug |
| K7 Menu text: field names outside the Menu table keep their raw names | G01401, G01406, G01485, G01486, G01378, G01452 | Bug |
| K8 MP4: the chapter Menu has no constant-sample-size `BitRate_Mode` | G01374, G01375, G01504 | Bug |
| K9 MP4: JSON keeps chapters that have an empty name | G01538, G01544, G01545, G01546, G01547 | Bug |
| K10 MPEG-TS text: the Menu fields use the order of the other tracks | G01812 | Bug |
| K11 MPEG-TS raw text: the service fields keep their text labels | G01477 to G01482 | Bug |
| K12 Matroska: the chapter name does not follow the official rule | G01329, G01748, G01348, G01767, G01124, G01125, G01539, G01540 | Bug, with one decision for `ChapLanguageIETF` |

Existing tickets cover these patterns:

- "DVD IFO: stop copying VOB rates and the menu frame rate" (#111) covers the Menu `Delay`, `FrameRate`, `FrameRate_Num`, `FrameRate_Den` and `FrameCount` of an IFO: G00168 to G00172.
- "DVD IFO: show the seconds in the duration text" (#117) covers the Menu duration text: G00309. S1 shows it again: official `15 min 54 s`, Go `15 min`.
- "DVD: decide whether to join the VOB files of a title set" (#116) covers the Menu `Source`: G00388 and G00392. Go writes `Source` on the Menu only when it joins the VOB files (`dvd.go:583-587` and `dvd.go:607-616`). v26.05 writes `Source` on every stream when its IFO opens the `_1.VOB` file (`File_Dvdv.cpp:285-305`, v26.05). The #116 decision settles this field too.
- "MP4: write the chapter track language on the Menu" (#132) covers G01444.
- "MPEG-TS text: show language names for captions and in the Menu list" (#133) covers G01442. G01448 has the same cause. The `List` item shows the code (`AC-3, en`) where official shows the name (`AC-3, English`). S6 shows both lines.
- The Blu-ray playlist part of G00030 (514 files) is out of scope. Go cannot parse `.mpls` files.

## How official v23.04 builds a DVD Menu

`VTS_PGCI` reads the header of the PGC table and the offset of the first PGC only (`File_Dvdv.cpp:851-878`). Then it parses PGCs one after the other until the table ends (lines 879-882). Each title PGC makes one Menu stream (lines 1455-1501). No rule removes a short PGC.

For each program in the program map, the Menu gets one chapter line named `Chapter N`. The chapter time is the sum of the cell times of all earlier programs (lines 1460-1477). `Get_Duration` turns one cell time into milliseconds (lines 1155-1175). The frame part is `FF*1000/fps` in integer math, with fps 30 for NTSC and 25 for PAL (lines 235-236 and 1171).

The stream lists come from the stream control table of the same PGC. `List (Audio)` has one item for each audio entry with the "available" bit set. The four subtitle lists have one item for each subpicture entry with that bit set (lines 1254-1310 and 1481-1500).

v26.05 changes three things. It gives each PGC its own offset (`File_Dvdv.cpp:1298`, v26.05). It adds the cell times as frame counts and rounds once (`File_Dvdv.cpp:1256-1282` and `TimeCode.cpp:846-857`, v26.05). It removes each Menu that is shorter than half of the longest Menu (`File_Dvdv.cpp:307-335`, v26.05, with the default trigger -2 at `MediaInfo_Config.cpp:488`).

## K1: Go makes one Menu per PTT title and drops short PGCs

Patterns: G00030 (Menu missing, 25 DVD files), G00312 (General `MenuCount`, 28 files: official 2 or 3, Go 1), G00327 (`@typeorder` missing, 24 files). Also 72 chapter time patterns that occur only on C001398, C277379 and C633586. On C001398, official Menu #1 has 3 chapters and Go Menu #1 has 19. On C277379, official has 1 more chapter time and Go has 11. Witnesses: C000065, C000066, C001398, C277379.

Official behavior: one Menu for each title PGC, in table order. The section above describes it.

Go behavior: `parseDVDPrograms` reads the PTT table (the table of titles and chapters). It makes one program for each title, from the first PGC of that title (`dvd.go:1364-1403` and `dvd.go:1424`). Then it keeps a program only if its duration is at least half of the longest one (`dvd.go:1405-1414`). This is the v26.05 rule. A PGC that no PTT entry names never becomes a Menu. Go also takes the chapters from the PTT entries (`dvd.go:1468-1483`), but official takes them from the program map.

Evidence: S1 has three title PGCs of 15 min 54 s, 30 s and 11 min 4 s. Official shows three Menus. Go shows two Menus and drops the 30 s PGC. On C001398 and C277379, the official Menu #1 is a short PGC. Go drops it, so its Menu #1 is a different PGC. This explains the chapter count mismatch.

Verdict: decision. Go follows v26.05, but no line in the deliberate list records it. Under ADR 0001 rule 2, the v23.04 behavior applies until the maintainer records a reason. The v26.10 presence list also has no Menu for G00030 ("missing vs v26 too"). C633586 is probably the same cause, but this research did not prove it. The section "Patterns not explained" gives the details.

## K2: v23.04 loses the PGCs that follow a padded program map

Pattern: G00662 (Go has a Menu track that official does not have, 3 files). Witnesses: C000072, C000073, C277675.

Official behavior: `PGC(Offset, true)` gets the offset of the first PGC for every PGC (`File_Dvdv.cpp:881`). The jumps to the program map and the cell table are relative to that offset (lines 1395-1397 and 1413-1415). For the second PGC and later, the jump never happens, and the parser reads the bytes in order. A program map with an odd length has one pad byte. The parser then reads the cell table one byte too early.

Go behavior: Go finds each PGC through its own table entry and its own offsets (`dvd.go:1428-1446`).

Evidence: S2 is S1 with three programs in the second PGC, so its program map has a pad byte. Official shows a wrong second Menu, with chapters at 20.000 and 30.000 s instead of 20.033 and 30.100 s, and a duration of 35 s 133 ms instead of 30 s 100 ms. It shows no third Menu. Go reads all three PGCs correctly. v26.05 fixed the offset (`File_Dvdv.cpp:1298`, v26.05).

Verdict: deliberate. Go corrects a v23.04 parse error. Add a line to the deliberate list in `AGENTS.md` when the K1 decision is made, because the K1 rule decides which of these Menus Go shows.

## K3: v23.04 truncates each NTSC cell time before it adds the cells

Patterns: 796 chapter time patterns on 16 DVD files, between G00663 and G01786. Each file has the same chapter count in both tools. Each Go time is later, and the difference grows from 1 ms to 18 ms over the chapters. Witnesses: C000065, C000066, C001437, C633491, C000552.

Official behavior: `Get_Duration` truncates the frame part of each cell to whole milliseconds (`File_Dvdv.cpp:1171`). At 30 fps, one frame is 33.33 ms, so each cell loses up to 0.67 ms. The chapter time adds these truncated values (lines 1462-1477). At 25 fps, one frame is exactly 40 ms, so PAL files have no difference.

Go behavior: Go adds the cell times as 90 kHz clock ticks (3000 ticks per NTSC frame) and rounds the sum once (`dvd.go:1670-1696` and `dvd.go:1476-1482`). v26.05 does the same with frame counts (`File_Dvdv.cpp:1574-1591` and `TimeCode.cpp:851-852`, v26.05).

Evidence: on `dvd-03`, official shows `00:20:00.466`, `00:30:20.466` and `01:29:43.266`. Go shows `00:20:00.467`, `00:30:20.467` and `01:29:43.267`. On S1, official shows `00:04:54.766`, `00:08:33.799` and `00:12:37.265`. Go shows `.767`, `.800` and `.267`. The exact values from the cell times are `.7667`, `.8000` and `.2667`.

Verdict: decision. Go gives the exact chapter start and matches v26.05. The values change only in the last digits, but they change the text keys and the JSON keys, so every chapter line differs. If the maintainer keeps the Go values, add one line to the deliberate list.

## K4: The DVD Menu stream lists do not come from the PGC of that Menu

Patterns, text only: G00324 (`List (Subtitles 4/3)`, 25 files: official `0 / 0 / 0 / 0 / 0`, Go `0 / 1 / 2 / 3 / 4`), G00361 and G00362 (Letterbox and Wide, 21 files: official `1 / 3 / 5 / 7 / 9` and `0 / 2 / 4 / 6 / 8`, Go all zeros), G00478 (`List (Audio)`, 12 files: official `0 / 1 / 2`, Go `0 / 1 / 2 / 3`), G00559 to G00561 and G00597 (subtitle lists of up to 32 items where official has fewer or none), G00648 (`List (Audio)` where official has none). Patterns in text and raw text: G00558. JSON only: G01517 to G01521. Witnesses: C000021, C000065, C000131, C000162, C001398, C277675.

Official behavior: each Menu lists the available entries of its own PGC. The section on v23.04 above describes it.

Go behavior has two separate errors:

1. The text output ignores the PGC table. `List (Audio)` is `0 / 1 / ...` for each audio attribute of the title set, and `List (Subtitles 4/3)` is `0 / 1 / ...` for each subpicture attribute. The other three subtitle lists are all zeros (`dvd.go:574-582`).
2. The JSON and raw text lists come from the PGC table, but Go computes them once for all Menus. The audio list comes from the first PGC only (`dvd.go:1132-1147`). The subtitle lists take each entry from the first PGC that has it available (`dvd.go:1164-1199`). Every Menu then gets the same lists (`dvd.go:599-602`).

Evidence: S1 PGC 1 has two audio entries and two subtitle entries with Wide numbers 0 and 2 and Letterbox numbers 1 and 3. PGC 3 has one audio entry with stream 1. Official text: Menu #1 `0 / 1`, `0 / 0`, `0 / 2`, `1 / 3`, `0 / 0`, and Menu #3 `1` in each list. Go text for both Menus: `0 / 1`, `0 / 1 / 2`, then three lists of `0 / 0 / 0`. Go JSON for both Menus: the Menu #1 lists. `dvd-03` has the same lists in both tools, because its only PGC lists every stream in order.

Verdict: bug.

## K5: The DVD raw text subtitle list labels use the v26 shape

Patterns, raw text only, 49 files: G00216, G00217, G00218 (official `List (Subtitles Letterbox)`, `List (Subtitles Pan&Scan)` and `List (Subtitles Wide)` missing in Go) and G00212, G00213, G00214 (Go labels `  Letterbox)`, `  Pan&Scan)` and `  Wide)`). Witnesses: C000012, C000013, C000021.

Official v23.04 behavior: the raw text label is the full field name, for example `List (Subtitles Wide)`. S1 and `dvd-03` show it.

Go behavior: `rawTextDVDExtraLabel` returns the short labels on purpose (`field_project_raw_text.go:590-595`). The v26.10 presence list shows that v26.10 writes the same short labels ("v26 emits it" on G00212 to G00214).

Verdict: decision. Go matches v26.10, but no line in the deliberate list records it. A label shape is not more correct in one version, so ADR 0001 rule 2 points to v23.04.

## K6: MP4 `Menus` and `Menu For` do not list every track

Patterns: G00886 (Video `Menus` missing in text, 2 files), G01455 and G01524 (Audio `Menus` missing in text and JSON), G01454 and G01523 (Menu `Menu For`: official `1,2`, Go `2`). Witnesses: C608220, C000220.

Official behavior: a `tref/chap` box in a track adds the chapter track to its `Chapters` list, and adds the track to the `ChaptersFor` list of the chapter track (`File_Mpeg4_Elements.cpp:8465-8466`). Each track with chapters gets `Menus` with every ID (`File_Mpeg4.cpp:680-691`). The chapter track gets `Menu For` with every track ID (lines 724-737). Both fields show in JSON and text.

Go behavior: `markMP4ChapterTracks` stores one source ID, and each later track overwrites it (`mp4.go:745-756`). `Menus` goes only into the JSON `extra` of a Video track, with the first ID only (`analyze.go:883-886`). The text output has no `Menus` line.

Evidence: S4 is H.264 and AAC with a chapter track. `ffmpeg` writes `tref/chap` in both tracks. Official Video and Audio text: `Menus : 3`. Official Menu: `Menu For : 1,2`. Go has no `Menus` line in text, has `Menus` in the Video JSON only, and shows `Menu For : 2`.

Verdict: bug.

## K7: Menu text: field names outside the Menu table keep their raw names

Patterns: G01401 and G01485 (official `Encoded date` and `Tagged date` missing) and G01406 and G01486 (Go `Encoded_Date` and `Tagged_Date`) on an MP4 chapter Menu. G01452 (official `Maximum bit rate` missing) and G01378 (Go `BitRate_Maximum`) on an MPEG-TS Menu. Text only. Witnesses: C000220, C000234.

Official behavior: the text output translates the field name, also for a field that the Menu table does not list.

Go behavior: the Menu builders pass the raw name as the text label: `mp4_probe.go:331` and `mp4_probe.go:335` for the dates, `mpeg_ts.go:3159` for the maximum bit rate.

Evidence: S5 is S4 with `creation_time` set. Official Menu #1: `Encoded date : 2020-01-02 03:04:05 UTC`. Go Menu #1: `Encoded_Date : 2020-01-02 03:04:05 UTC`. The tagged date has the same difference. The TS part has proof from the code and the corpus pages only.

Verdict: bug.

## K8: The MP4 chapter Menu has no constant-sample-size `BitRate_Mode`

Patterns: G01374 and G01375 (text and raw text `Bit rate mode` missing on the Menu), G01504 (JSON `BitRate_Mode` `CBR` missing). Witness: C608220.

Official behavior: if the `stsz` box gives one sample size for all samples and the track has more than one sample, `BitRate_Mode` becomes `CBR`. This rule applies to every track kind, the Menu too (`File_Mpeg4_Elements.cpp:8081-8082`).

Go behavior: `buildMP4ChapterMenu` writes no bit rate mode (`mp4_probe.go:317-363`).

Evidence: code and corpus pages only. In S4, the chapter samples have different sizes, so official writes no bit rate mode there either.

Verdict: bug. The value has little use, but it is part of the official output shape.

## K9: MP4 JSON keeps chapters that have an empty name

Patterns: G01538, G01544, G01545, G01546, G01547 (JSON only: Go has five chapter keys with the value `""`). Witness: C608220.

Official behavior: `moov_udta_chpl` fills each chapter with its name (`File_Mpeg4_Elements.cpp:8694-8724`). An empty value does not show in the output.

Go behavior: the Menu from the `chpl` box adds every chapter to the JSON `extra`, also with an empty title (`analyze.go:914-925`). The text output skips the empty value.

Evidence: S4 has a chapter without a title at 1 s. Official JSON and text have no key for it. Go text has no line for it, but Go JSON has `"_00_00_01_000": ""` in the second Menu.

Verdict: bug.

## K10: MPEG-TS text: the Menu fields use the order of the other tracks

Pattern: G01812 (1 file). Official order: `ID`, `Menu ID`, `Format`, `Duration`, `List`, `Title`, `Language`, `Law rating`. Go puts `List` after `Language`. Witness: C000234.

Official behavior: the text order of a Menu follows the Menu table. `List` comes first, then `Title`, then `Language`, then the service fields (`Menu.csv:68-90`, v26.05).

Go behavior: one text order table serves every track kind except General (`field_order_registry.go:151-157`). It puts `Language` at 47 and `List` at 60 (`field_order_registry.go:92` and `field_order_registry.go:142`).

Evidence: S6 is an MPEG-TS file with MPEG-2 video and English AC-3. Official Menu text has `List` and then `Language`. Go Menu text has `Language` and then `List`. Go raw text has the official order.

Verdict: bug.

## K11: MPEG-TS raw text: the service fields keep their text labels

Patterns: G01480, G01481, G01482 (official `ServiceName`, `ServiceProvider` and `ServiceType` missing) and G01477, G01478, G01479 (Go `Service name`, `Service provider` and `Service type`). Raw text only. Witness: C000313.

Official behavior: raw text uses the field names `ServiceName`, `ServiceProvider` and `ServiceType`.

Go behavior: the TS Menu builder writes the text labels (`mpeg_ts.go:3140-3148`). The raw text writer does not map them back for the Menu.

Evidence: S6 has a service name and a service provider. Official raw text shows `ServiceName`. Go raw text shows `Service name`.

Verdict: bug.

## K12: Matroska: the chapter name does not follow the official rule

Patterns: G01329, G01748, G01348, G01767 (2 files: the official value is a language prefix with no name, for example `en:`, and Go has the default `Chapter NN` form). G01124, G01125, G01539, G01540 (1 file: official joins two chapters at the same time with ` / `, and Go writes two lines with the same key). Witnesses: C044149, C629464, C629652.

Official behavior (`File_Mk.cpp:1374-1410`):

1. Each `ChapterDisplay` gives `lang:` and then `ChapString`. The language comes from `ChapLanguage` through the ISO 639-1 table. `und` gives no prefix.
2. Several displays of one chapter are joined with ` - `.
3. A chapter with no display gets its time as its name.
4. A second chapter at the same time adds its name to the first with ` / `.
5. Nested `ChapterAtom` elements go into the same flat list (`File_Mk.cpp:2046-2048`).
6. v23.04 has no handler for `ChapLanguageIETF`, so it uses `ChapLanguage` only.

Go behavior (`matroska.go:1310-1445`):

1. Go ignores a display with an empty `ChapString`, together with its language (`matroska.go:1395-1400`). The chapter then gets `Chapter N` (`matroska.go:1317-1320` and `matroska.go:1473`).
2. With several displays, Go keeps the language of the first one and the name of the last one.
3. A chapter with no display gets `Chapter N`.
4. Two chapters at the same time make two text lines with the same key. In JSON, the second name replaces the first.
5. Go ignores nested `ChapterAtom` elements.
6. `ChapLanguageIETF` replaces `ChapLanguage` (`matroska.go:1442-1444`).

Evidence: S3 is a Matroska file with a chapter without a title and two chapters at 2 s. Official shows `00:00:01.000 : 00:00:01.000` and `00:00:02.000 : Same A / Same B`. Go shows `00:00:01.000 : Chapter 2`, and two lines at `00:00:02.000`. The throwaway test gave these Go values. An empty `ChapString` with `ChapLanguage` `eng` gives `Chapter 1` (official `en:`). Two displays, `eng` A and `fre` B, give `en:B` (official `en:A - fr:B`). Go loses a nested chapter at 1 s.

Verdict: bug. The IETF part is a decision, because newer MediaInfo reads `ChapLanguageIETF`.

## Patterns not explained

- C633586 (2 chapter time patterns): official Menu #1 has a chapter at `01:02:00.000`, and Go Menu #1 has one at `00:02:00.000`. The file is in G00030 and G00312, so official has more Menus than Go. The probable cause is K1: the two Menus #1 are different PGCs with two chapters each. Only the file can prove it.
- G01120 (C608220, JSON and XML only): official has a member with an empty key, with the value `null` in one output and `{}` in the other. S4 does not make it.
- G01375: official text has two `Bit rate mode` lines on the Menu (`Constant` and `CBR`). K8 explains the field, but not the second line.

## Questions that need a witness file

1. C633586: what are the durations and the chapter counts of the title PGCs, and which PGC does the PTT table name for each title?
2. C608220: which chapter track samples have the same size, and does the official JSON Menu contain a member with an empty key? What is that member?
3. C000072, C000073, C277675: does a program map in these IFO files have an odd length, and which PGC is missing in the official output?

## Side findings

- MP4: official writes `Duration_FirstFrame` on the chapter Menu of S4 (`1500`). Go does not. No corpus pattern has it.
- Matroska: official makes a Menu for each edition, also for an empty one (`File_Mk.cpp:1377-1380`). Go skips an empty edition (`matroska.go:1312`).
- DVD: v23.04 reads 7 bits for the audio stream number in the PGC table (`File_Dvdv.cpp:1263`). Go reads 3 bits, as the DVD format defines them (`dvd.go:1143`). No corpus pattern shows a difference.

## Synthetic files

| ID | How it was made | Content |
|---|---|---|
| S1 | Script, VTS IFO | NTSC 16:9. Two audio and three subpicture attributes. PGCs of 15 min 54 s (4 programs), 30 s 100 ms (2 programs) and 11 min 4 s (2 programs). Each PGC has its own stream control entries. |
| S2 | Script, VTS IFO | S1, but the second PGC has 3 programs, so its program map has a pad byte. |
| S3 | `ffmpeg`, Matroska | H.264 and AAC, 3 s. Chapters at 0 s (with a title), 1 s (no title) and two at 2 s. |
| S4 | `ffmpeg`, MP4 | S3 copied into MP4. `ffmpeg` writes a chapter text track with `tref/chap` in both tracks, and a `chpl` box. |
| S5 | `ffmpeg`, MP4 | S4 with `creation_time` 2020-01-02 03:04:05 UTC. |
| S6 | `ffmpeg`, MPEG-TS | MPEG-2 video and English AC-3, with a service name and a service provider in the SDT. |
