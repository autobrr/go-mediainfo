# Research: MPEG-PS and DVD stream identity, captions and track lists (issue #77)

Date: 2026-10-18. Parent map: #65. Rules: ADR 0001 (PR #64) and `AGENTS.md`.

This note answers one question. Which stream identity, caption and track list differences in MPEG-PS and DVD files come from go-mediainfo bugs, and which root causes produce them? Stream identity means `ID`, `StreamOrder`, `FirstPacketOrder`, `@typeorder`, `MenuID`, the track counts and the presence of whole tracks. The ticket holds 65 patterns on 486 files. Some of them are in Matroska, MP4, AVI and BDAV files, so this note covers those too.

Each cause gets a verdict:

- Bug: go-mediainfo is wrong.
- Deliberate: a listed deliberate difference, or a defensible correction of official.
- Decision: Go matches newer MediaInfo, or Go can be more correct. The maintainer must decide.
- Out of scope.

In the pattern lists, "official A, Go B" gives the official v23.04 value first and the go-mediainfo value second.

## Method

- go-mediainfo: `main` at `6a37300`. Line numbers below refer to this commit.
- MediaInfoLib: tag `v23.04`, read with `git show v23.04:<path>`. Line numbers below refer to `v23.04`, except where the text says "v26.05" or "current source". The local checkout is v26.05 (`MediaInfo_Config.cpp:141`). It stands in for newer MediaInfo.
- FFmpeg: `libavcodec/mpeg12dec.c` at tag `n7.1`, read through the GitHub API, for the DVD caption format.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The JSON command was `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` against `go-mediainfo --output=JSON --language=raw`. The text command was `mediainfo --ParseSpeed=0.5` against `go-mediainfo`.
- Real files. All come from the disc of sample `dvd-01` or from sample `dvd-03`. Each run read one file:
  - H1: the IFO of title set 13 of the `dvd-01` disc. The title set is 16:9 and has four VOB files.
  - H2: the second VOB of title set 13. Go joins the title set for this file.
  - H3: the first 64 MiB of the first VOB of title set 13, copied to a folder that is not `VIDEO_TS`.
  - H4: the first 64 MiB of the second VOB of title set 13, copied the same way.
  - H5: sample `dvd-03`, a 4:3 IFO.
- Synthetic files. `ffmpeg` made each one from `testsrc` and `sine`:
  - S1: MP4, with the audio `trak` before the video `trak`.
  - S2: MPEG-1 program stream, video first.
  - S3: DVD-format program stream (`-f vob`), MPEG-2 video and AC-3, named `s3.vob`.
  - S5: the S3 bytes copied to `VIDEO_TS/VTS_01_1.VOB`.
  - S6: Matroska with a track tag `SOURCE_ID` that is not hex.
  - S7: Matroska with a track tag `SOURCE_ID=001011` and no `_STATISTICS_TAGS`.
- Corpus data: the per-file pages of the corpus archive, and the triage sheet. A diff in the corpus matches tracks by ordinal. The witness C-ids come from these pages. Only the contributor can open the files.

Two patterns sort the corpus files. G00100 (`CompleteName_Last`) marks the 181 VOB files that Go joins with other VOB files of the same title set. G00390 (`extra/Source` on Video) marks the 18 IFO files for which Go also parses the VOB files. The pattern lists below give the number of joined files for each pattern.

## Summary

| Cause | Patterns (main ones) | Verdict |
|---|---|---|
| D1 Go joins title-set VOB files and reads VOB files for an IFO | G00100; joined parts of G00064, G00074, G00281, G00316, G00326, G00334, G00481, G00524, G00574; IFO parts of 16 patterns | Existing ticket #116 |
| D2 The IFO stream ID has the v26.05 form | G00180 (DVD, 71 files), G00150 (DVD, 49 files) | Decision, with one bug inside |
| D3 The IFO Menu count follows the PTT titles, not the PGC table | G00312 (28 files) | Bug |
| D4 The IFO text count follows the attribute table | DVD parts of G00281, G00457 | Existing ticket #134 |
| C1 DVD caption field 1 is CC1 in Go and CC3 in official | MPEG-PS part of G00150, G00252, values of G00222 | Decision |
| C2 Text shows "Caption service name" | G00222 (47 files) | Bug |
| C3 Text has no "Start time (commands)" | G00257 (36 files) | Bug |
| C4 Single-file MPEG-PS does not extend the parse for captions | non-joined parts of G00064, G00509, G00512, G00562, G00494, G00516, G00520 | Bug, probable |
| O1 MPEG-PS StreamOrder comes from fixed rules, not the system header | G00404, G00337, MPEG-PS parts of G00487, G00582 | Bug |
| O2 The shared finalizer writes StreamOrder in kind order | MP4 parts of G00487, G00498, G00626 | Bug |
| O3 DVD VOB tracks are sorted by duration, and the caption is put second | non-joined parts of G00307, G00320 | Bug |
| A1 MPEG-PS drops an audio stream that has no PTS and no frame | MPEG Audio part of G00038, G00857, one file of G00481 | Decision |
| K1 Matroska text writes the wrong label and value for the source medium ID | G00318, G00319, G00401, G00403, G00436, G00438 | Bug |
| K2 Matroska converts or hides `SOURCE_ID` without the statistics-tag test | G00541, G00542, G00543 | Bug |
| M1 MP4 has no Other tracks | G00395 (18 files) | Bug |
| M2 MP4 does not read captions in AVC | MP4 parts of G00064, G00281, G00457 | Bug |
| V1 AVI type-1 DV gives no tracks | G00031, AVI parts of G00038, G00039 | Bug, probable |
| V2 AVI text shows the key `MuxingMode_PackedBitstream` | G00402 (17 files) | Bug |
| B1 BDAV text General ID falls back to the program number | G00373 (20 files) | Bug |
| B2 BDAV audio "Muxing mode" is in JSON only | G00426 (15 files) | Existing ticket #151 |
| X1 Matroska cover attachment gives an Image track | G00188 (65 files) | Deliberate |
| X2 Other existing tickets | G00490 (#167), G00621 (#155), Ogg parts of G00180 and G01438 (#58) | Existing tickets |
| U1 to U3 Not explained | see the end | Needs witness files |

## D1: Go joins title-set VOB files and reads VOB files for an IFO

Existing ticket: "DVD: decide whether to join the VOB files of a title set" (#116). #116 already lists G00100. This research adds the track-list patterns to it.

Patterns in VOB files that Go joins (G00100 is present):

- G00064, whole Text track missing: 55 of 57 MPEG-PS files.
- G00074, `TextCount` missing: 44 of 44 files.
- G00326, Go has an extra Text track: 22 of 23 MPEG-PS files.
- G00281 (`TextCount`), MPEG-PS part: 24 of 26 files.
- G00316 (Audio `FirstPacketOrder`): 26 of 26. G00334 (Text `FirstPacketOrder`): 22 of 24. G00574 (Video `FirstPacketOrder`): 6 of 6.
- G00524 (extra Audio track): 9 of 9. G00481 (`AudioCount`): 10 of 11. G00608 and G01368 (`@typeorder` new): all files.
- 20 of 28 files of G00307 and 19 of 25 files of G00320 (`@typeorder`).

Patterns in IFO files for which Go also parses the VOB files (G00390 is present): G00380 (Video `FirstPacketOrder`), G00387, G00390, G00391, G00496, G00501 (`Source`), G00398, G00575 (`FirstPacketOrder`), G00412, G00598 (`MuxingMode`), G00414, G00582 (`StreamOrder`), G01439 (a caption track ID), and the DVD parts of G00064, G00281, G00326, G00494, G00509, G00512, G00516, G00520.

Witnesses: C000014, C000741, C001434, C001594 (VOB). C000006, C000013, C000066, C000162 (IFO).

Official behavior:

- v23.04 parses one VOB file as one MPEG-PS file. It reads the start and the end of that file only.
- v23.04 does not open a VOB file for an IFO. `File_Dvdv::Streams_Finish` only clears the sector list (`File_Dvdv.cpp:281-286`). v26.05 opens `_1.VOB` and merges it (v26.05 `File_Dvdv.cpp:283-300`).

Go behavior:

- For a title VOB, Go collects all VOB files of the title set (`analyze.go:199-205`). It parses the start of the first VOB and the end of the last VOB (`mpeg_ps_stream.go:784-828`). So Go does not find a subtitle stream that starts in a middle VOB. Go counts `FirstPacketOrder` across the first and the last VOB.
- For an IFO in `VIDEO_TS`, Go parses the VOB files of the title set (`dvd.go:152`, `dvd.go:242-258`). Then it adds the VOB facts to the IFO tracks: `Source`, `StreamOrder`, `FirstPacketOrder`, `MuxingMode` and the caption tracks.

Evidence:

- H2 against H4. H2 is the second VOB in place, so Go joins. Official finds two RLE subtitle tracks (`189-33` and `189-34`). Go finds none, and it has no `TextCount`. H4 has the same first 64 MiB of that VOB, in a folder where Go does not join. Go then finds the same two RLE tracks as official, with the same `FirstPacketOrder`.
- H1. Official writes no `StreamOrder`, no `FirstPacketOrder`, no `MuxingMode` and no `Source` on the IFO tracks. Go writes all four.

Verdict: existing ticket #116. The track-list patterns above go away, or become deliberate, when #116 is decided. Do not file a new ticket.

## D2: The IFO stream ID has the v26.05 form

Patterns:

- G00180, DVD part (71 files): official `128 (0x80)`, Go `189-128`. In text, official `128 (0x80)`, Go `189 (0xBD)-128 (0x80)`.
- G00150, DVD part (49 files): official `32 (0x20)`, Go `189-32`, `189-33`, `189-34` and so on.

Witnesses: C000005, C000012, C000013, C000015, C000024.

Official behavior. v23.04 fills the IFO audio and text IDs from the PGC stream control table:

- Audio: `Get_Hex_ID(ToAdd + Number)`, where `ToAdd` is 0x80 for AC-3, 0x88 for DTS and 0xA0 for `LPCM` (`File_Dvdv.cpp:1270-1285`). The attribute parser fills the format `PCM`, not `LPCM` (`File_Dvdv.cpp:112-122`, `664`). So an LPCM track gets the ID `0 (0x0)`, not 160. MPEG audio also gets the plain stream number.
- Text: `Get_Hex_ID(0x20 + Number_Wide)` (`File_Dvdv.cpp:1311-1318`). Official uses the wide-screen number also on a 4:3 title. A 4:3 title has wide-screen number 0 for all entries, so every Text track gets `32 (0x20)`. The research for #72 found this too.
- v26.05 writes `189-` plus the same number, and `189 (0xBD)-` in the text form (v26.05 `File_Dvdv.cpp:1380-1394`, `1421-1428`). It keeps the `LPCM` test and the wide-screen number.

Go behavior. Go writes `189-` plus the substream number, in the v26.05 form (`dvd.go:446-448`, `505-506`, `516-518`, `540-541`). The audio number comes from `dvdAudioPrivateID` (`dvd.go:1117-1122`). It adds 0x80 for AC-3 only. So a DTS track gets `189-1`, where both MediaInfo versions write 0x88 plus the number. The text number is the 4:3 number on a 4:3 title, and the wide-screen number on a 16:9 title (`dvd.go:1188-1199`).

Evidence:

- H1 (16:9): official audio `128 (0x80)`, text `33 (0x21)`. Go `189-128`, `189-33`.
- H5 (4:3, four subtitle entries): official gives all four Text tracks `32 (0x20)`. Go gives `189-32`, `189-33`, `189-34`, `189-35`.

Verdict:

- The ID form is a decision. Go matches v26.05, and the `189-` form is the same ID that the VOB track has. Under ADR 0001 rule 2, Go can keep it only with a line in the deliberate-differences list. #83 can take this decision.
- The 4:3 text ID is a defensible correction. Official uses the wrong column of the control table on a 4:3 title. If Go keeps it, it needs a line in the deliberate-differences list.
- The missing 0x88 DTS offset is a bug. Both MediaInfo versions add it.
- The LPCM ID is a decision. Official writes the plain number because its `LPCM` test never matches. The DVD substream ID of LPCM is 0xA0 plus the number.

## D3: The IFO Menu count follows the PTT titles, not the PGC table

Patterns: G00312 (28 files). Official `MenuCount` 2 to 12, Go 1 (24 files). Official 2, Go 3 or 4 (3 files). Official 3, Go 2 (1 file).

Witnesses: C000012, C000065, C000072, C001437.

Official behavior. `File_Dvdv::VTS_PGCI` reads the PGCs of the title PGC table one after the other, up to the end address (`File_Dvdv.cpp:854-882`). `File_Dvdv::PGC` makes one Menu track for each title PGC (`File_Dvdv.cpp:1455-1457`).

Go behavior. `parseDVDPrograms` makes one program for each PTT title (`dvd.go:1364-1405`). Then it drops each program that is shorter than half of the longest one (`dvd.go:1408`). The code comment says that MediaInfo does this, but `File_Dvdv.cpp` has no such rule. Go makes one Menu track for each remaining program (`dvd.go:553-605`).

Evidence. Source only. H1 and H5 have one PGC and one title, so both tools show one Menu.

Verdict: bug. The research ticket "Chapters and DVD menus" (#79) covers the content of the DVD Menu tracks. If #79 files a ticket for the same rule, use that ticket.

## D4: The IFO text count follows the attribute table

Patterns: DVD part of G00281: official 32, Go 1 or 2 (C000132, C000162). Official 5, Go 10 (C000022). Official 4, Go 5 (C000066). DVD part of G00457.

Official behavior. For each enabled subpicture control (up to 32), official makes Text tracks up to that position, also when the attribute table has fewer entries (`File_Dvdv.cpp:1311-1318`). So a PGC that enables all 32 controls gives 32 Text tracks, and most of them are empty.

Go behavior. Go makes one Text track for each attribute entry (`dvd.go:513-546`). In aggregate mode, Go also adds the VOB substreams and captions (D1).

Verdict: existing ticket "DVD IFO: build the audio and text tracks from the IFO attribute table" (#134). Its first decision asks this question. The 32 empty tracks are an official artifact. The attribute table count is defensible.

## C1: DVD caption field 1 is CC1 in Go and CC3 in official

Patterns: G00150, MPEG-PS part (29 joined files and 8 other files): official `224-CC3`, Go `224-CC1`. G00252 (38 files): official `CaptionServiceName` `CC3`, Go `CC1`. The Go text values of G00222 are `CC1`. No corpus file shows `CC1` in official for DVD captions.

Witnesses: C000522, C000523, C001433, C277378.

The format. DVD captions are MPEG-2 user data that start with `CC`, 0x01, 0xF8. Each caption block starts with a marker byte. FFmpeg documents the format as "bit 0 caption_field_odd 1=odd field (this is CC1/CC2) 0=even field (this is CC3/CC4)" (`mpeg12dec.c:2018`). FFmpeg sends a block with marker 0xFF to field 1 (`mpeg12dec.c:2048`).

Official behavior. `File_DtvccTransport` reads the DVD format. It reads the last bit of the marker as `cc_type` (`File_DtvccTransport.cpp:227-233`). Marker 0xFF gives `cc_type` 1. `cc_type` 1 selects parser 1 (`File_DtvccTransport.cpp:252`, `369`). The EIA-608 parser names the service `CC` plus `1 + cc_type*2`, so it gives `CC3` (`File_Eia608.cpp:126-129`). v26.05 has the same code.

Go behavior. Go sends marker 0xFF to field 1 and 0xFE to field 2 (`mpeg2_cc.go:153-158`). Field 1 is `CC1` (`mpeg_ps_cc_stream.go:19-22`).

Evidence. Source only. No DVD sample with captions was available on the media host, and FFmpeg cannot write DVD captions.

Verdict: decision. Go follows the documented format, so official calls field-1 captions `CC3`. upbrr reads the caption ID from JSON. If Go keeps `CC1`, the deliberate-differences list needs a line. FFmpeg also notes that some discs set the odd-field bit on all blocks (`mpeg12dec.c:2023-2027`). A fix must decide how to treat those discs.

## C2: Text shows "Caption service name"

Patterns: G00222 (47 files, text outputs only). Go text has `Caption service name : CC1`. Official text has no such line.

Witnesses: C000162, C000522, C001433.

Official behavior. The EIA-608 parser fills `CaptionServiceName` with the option `N NT` (`File_Eia608.cpp:128-129`). JSON shows the field in `extra`, and text does not show it. v26.05 is the same (v26.05 `File_Eia608.cpp:135-136`). The triage sheet marks G00222 "extra in v26 too".

Go behavior. `buildCCTextStream` adds a text field "Caption service name" (`mpeg_ps_cc_stream.go:113`).

Verdict: bug.

## C3: Text has no "Start time (commands)"

Patterns: G00257 (35 MPEG-PS files and 1 MPEG-TS file, text outputs only). Official shows `Start time (commands)`. Go shows nothing.

Witnesses: C000067, C000520, C000522.

Official behavior. The EIA-608 parser fills `Duration_Start_Command` (`File_Eia608.cpp:177-178`). `Duration_Start_Command/String` has the option `Y NT` (`Text.csv:59-60`), so text shows it as "Start time (commands)".

Go behavior. Go writes `Duration_Start_Command` to JSON (`mpeg_ps_cc_stream.go:137`, `mpeg_ts_captions.go:220`). The text fields stop at "Start time" and "End time" (`mpeg_ps_cc_stream.go:103-106`). No Go code writes the label "Start time (commands)".

Verdict: bug.

## C4: Single-file MPEG-PS does not extend the parse for captions

Patterns. These files are not joined, but Go has fewer RLE tracks than official:

- G00509 and G00512, MPEG-PS part: official Text #1 `RLE`, Go Text #1 `EIA-608` (C633576, C633578).
- The same files in G00064 (missing RLE tracks), G00562 (`MuxingMode`), G00494, G00516 and G00520 (caption fields on the first Text track), G00334 and G00337.

Witnesses: C633576, C633578. The research for #71 found the same files as cause F25.

Official behavior. When MPEG-2 user data holds captions, the video parser sets `MustExtendParsingDuration` (`File_Mpegv.cpp:3260`). The MPEG-PS parser takes this flag from its sub-parsers (`File_MpegPs.cpp:4032-4035`). The flag makes the parsed size four times larger: `program_mux_rate*50*4*4` (`File_MpegPs.cpp:2501`). In the longer window, official finds subtitle packets that start later.

Go behavior. Go extends the window for captions only in `ParseMPEGPSFiles` (`mpeg_ps_stream.go:700-726`). Go uses that function for a joined title set, an IFO and a menu VOB (`analyze.go:1723-1724`). A single VOB, or a program stream outside `VIDEO_TS`, goes to `ParseMPEGPSWithOptions` (`mpeg_ps.go:39-91`). That function reads a fixed window at the start and at the end, and it has no caption rule.

Evidence. In C633576, official has three RLE tracks with `FirstPacketOrder` 4, 5 and 6. Go finds none of them, but it finds the caption. The files are not joined. A real-file proof needs a file with captions, and none was available.

Verdict: probable bug. The witness file must show which Go path the file takes.

## O1: MPEG-PS StreamOrder comes from fixed rules, not the system header

Patterns:

- G00404 (17 files): official Audio `StreamOrder` 0, Go none.
- G00337 (24 files): official Text `StreamOrder` 0, Go none.
- G00582, MPEG-PS part (C636230): official none, Go 0, 1, 2, 3 on four Text tracks.
- G00487, MPEG-PS part (C610409, MPEG-1): official Audio 1, Go 0.

Witnesses: C001178, C001867, C529405, C529688, C610409, C636230.

Official behavior. `StreamOrder` comes from the system header (start code 0xBB). Each listed `stream_id` above 0xB9 gets the next index (`File_MpegPs.cpp:2543-2598`). The wildcards 0xB8 (all audio) and 0xB9 (all video) get no index. All substreams of private stream 1 (0xBD) share the index of 0xBD (`File_MpegPs.cpp:630-643`). A caption track gets the index of its video stream (`File_MpegPs.cpp:676-687`). A DVD system header lists 0xB9, 0xB8, 0xBD and 0xBF. So on a DVD, video and captions get no `StreamOrder`, and every AC-3 and RLE track gets 0.

Go behavior. Go does not read the system header for `StreamOrder`. It uses fixed rules:

1. Audio in an MPEG-1 stream gets 0 (`mpeg_ps.go:486-488`).
2. If the stream has a 0xBF navigation stream, no track gets a `StreamOrder` (`mpeg_ps.go:452-462`, `1671`).
3. For a file name of a DVD title VOB, menu VOB or IFO, audio that is not MPEG Audio and RLE text get 0 (`dvd.go:2014-2035`, `2246-2263`, `2302-2307`).
4. If the duration is shorter than 1 s, no track gets a `StreamOrder` (`analyze.go:1758-1762`). The comment says that MediaInfo does this for one-frame menu VOB files.
5. Else, the shared finalizer gives each track its position in kind order (O2).

Evidence:

- H3 and H4 (DVD bytes, not in `VIDEO_TS`): official Audio and RLE text `StreamOrder` 0. Go none (rule 2).
- S2 (MPEG-1, video listed first): official Video 0, Audio 1. Go Video 0, Audio 0 (rule 1).
- S3 (`-f vob`, the system header lists 0xE0 and 0xBD): both tools give Video 0, Audio 1.
- S5 (the S3 bytes as `VIDEO_TS/VTS_01_1.VOB`): official Audio 1. Go Audio 0 (rule 3).
- Corpus: C001178 (0.556 s), C001867 (0.501 s) and C529405 (0.598 s) are shorter than 1 s, and official writes `StreamOrder` 0. So rule 4 is not an official rule. In C001594 and C290075, the PTS reset bug (#118) gives Go a duration of about 0.5 s against about 1300 s. Rule 4 then also drops `StreamOrder`.

Verdict: bug.

## O2: The shared finalizer writes StreamOrder in kind order

Patterns:

- G00487, MP4 part (10 files): official Audio 0, Go 1. G00498 (10 files): official Video 1, Go 0.
- G00626 (4 AVI files): official none, Go Video 0. These are uncompressed RGB files.

Witnesses: C608243, C608415, C608619 (MP4). C608393, C608397 (AVI).

Official behavior. MP4 gives each `trak` the next `StreamOrder` in file order (`File_Mpeg4_Elements.cpp:4393-4394`, current source). AVI fills `StreamOrder` from the stream number (`File_Riff.cpp:327`, `381`, current source).

Go behavior. If a parser sets no `StreamOrder`, the shared finalizer sorts the tracks by kind and writes the position (`field_finalize.go:95-114`). The MP4 parser sets no `StreamOrder`, so video always comes first.

Evidence. S1 (audio `trak` first): official Audio 0, Video 1. Go Video 0, Audio 1. The AVI part is not proven on a file. The finalizer is the only Go code that writes the value there.

Verdict: bug for MP4. Probable bug for AVI. The witness file must show why official writes no `StreamOrder` for these RGB streams.

## O3: DVD VOB tracks are sorted by duration, and the caption is put second

Patterns. Non-joined parts of G00307 (Audio `@typeorder`, 8 files) and G00320 (Text `@typeorder`, 6 files). Example C000523: official Text #2, #3, #4 are Go Text #3, #4, #2.

Witnesses: C000523, C000556, C633494, C633574.

Official behavior. `Streams_Fill` fills the main streams in `stream_id` order, then the private-stream-1 substreams in substream order (`File_MpegPs.cpp:264-278`). So MPEG audio (0xC0) comes before AC-3 (0xBD 0x80), and the AC-3 tracks follow their substream number. Official makes the caption tracks later, in `Streams_Finish`, after the RLE tracks exist (`File_MpegPs.cpp:648-690`). So the caption is the last Text track. The corpus agrees: C000523 has three RLE tracks, and official gives the caption Text #4.

Go behavior. `sortDVDPayloadStreams` sorts audio by duration, longest first (`dvd.go:2208-2215`). It gives a caption the sort key 32.5, so the caption comes after RLE 0x20 and before RLE 0x21 (`dvd.go:2216-2227`). The VOB paths and the IFO path call this function (`dvd.go:2047`, `2267`, `2927`, `2963`).

Verdict: bug. #134 covers the IFO part, and it names the same function. The research ticket "Field order, track order and numeric precision" (#80) covers order in general.

## A1: MPEG-PS drops an audio stream that has no PTS and no frame

Patterns: G00038, MPEG Audio part (C001264, C282830): official has an MPEG Audio track with stream ID 0xC9 or 0xDD and no channel count. Go has no track. G00857 and one file of G00481 are the same files.

Official behavior. Official makes a track for each PES stream ID that it sees. The track can have no format details.

Go behavior. `finalizeMPEGPS` drops an audio stream with no PTS, no AC-3 frame and no audio header (`mpeg_ps.go:525-527`).

Verdict: decision. The stream has no audio that a player can use, so the Go output can be more useful. But the track list is part of the output shape. The witness files must confirm that the Go rule is the cause.

## K1: Matroska text writes the wrong label and value for the source medium ID

Patterns: G00318 and G00319 (Video, 26 files), G00401 and G00403 (Text, 17 files), G00436 and G00438 (Audio, 14 files). All are text outputs only. Official `ID in the original source medium : 4113 (0x1011)`. Go `Original source medium ID : 4113`. For DVD sources, official `189 (0xBD)32 (0x20)`, Go `189-32`.

Witnesses: C000070, C000119, C000282.

Official behavior. The label of `OriginalSourceMedium_ID` is "ID in the original source medium" (`MediaInfo_Config_Automatic.cpp:1113`). Text shows the `/String` value (`MediaInfo_Config_Automatic.cpp:4319`). `Mk_ID_String_From_Source_ID` makes the `/String` value in hex form (`File_Mk.cpp:632-660`).

Go behavior. `matroska_scan.go:1486-1488` adds the text field "Original source medium ID" with the decimal value.

Verdict: bug.

## K2: Matroska converts or hides SOURCE_ID without the statistics-tag test

Patterns: G00541, G00542, G00543 (Video, Audio and Text, 8 files each, text outputs only). Official text has a line `SOURCE_ID` with a value that is not hex. Go text has no line.

Witnesses: C000235, C008834.

Official behavior. Official converts `SOURCE_ID` only when the statistics tags of the track list it (`File_Mk.cpp:920-930`). The value must be six hex digits that start with `00` or `01` (`File_Mk.cpp:572-630`). Else the tag stays a plain tag with its own name, and text shows it.

Go behavior. Go converts each hex `SOURCE_ID`, also when no statistics tags list it. It puts a value that is not hex into JSON `extra` only (`matroska.go:3781-3787`).

Evidence:

- S6 (value not hex): official text `SOURCE_ID : <value>`. Go text has no line. JSON is the same.
- S7 (`001011`, no statistics tags): official text and JSON keep `SOURCE_ID : 001011`. Go writes `Original source medium : Blu-ray`, `Original source medium ID : 4113`, and JSON `OriginalSourceMedium_ID`.

Verdict: bug.

## M1: MP4 has no Other tracks

Patterns: G00395 (18 files): official has Other tracks, Go has none. The tracks are QuickTime time code (`tmcd`), RTP hint tracks, and MPEG-4 object and scene description tracks. G00384 (`OtherCount`, 18 files) has the same cause.

Witnesses: C608258, C608261, C608266, C608339.

Go behavior. go-mediainfo has no Other stream kind (`report.go:8-18`). The MP4 parser drops each track whose handler is not `vide`, `soun`, `text`, `sbtl` or `subt` (`mp4.go:423-429`, `631-641`).

Verdict: bug. The Other kind is part of the MediaInfo output shape.

## M2: MP4 does not read captions in AVC

Patterns: MP4 parts of G00064, G00281 and G00457 (C531701, C531702). Official has the Text tracks `1-CC1` (EIA-608) and `1-1` (EIA-708), with MuxingMode "SCTE 128 / DTVCC Transport". Go has only the timed text track.

Go behavior. Go parses captions only in MPEG-2 user data and in MPEG-TS (`mpeg2_cc.go`, `mpeg_ts_captions.go`). The MP4 and AVC code has no caption parser.

Verdict: bug. This is a missing feature, and `AGENTS.md` does not list it as a known gap.

## V1: AVI type-1 DV gives no tracks

Patterns: G00031 (Video), the AVI parts of G00038 (Audio) and G00039 (`AudioCount`). One file (C609329). Official has DV video and PCM audio.

Go behavior. `parseAVIStrh` knows the stream types `vids`, `auds` and `txts` only (`avi.go:947-954`). A type-1 DV file has one `iavs` stream that carries both video and audio. Official reads it (`File_Riff_Elements.cpp:1277`, `1614`, current source).

Verdict: probable bug. The witness file must confirm that it is type-1 DV.

## V2: AVI text shows the key MuxingMode_PackedBitstream

Patterns: G00402 (17 files, text outputs only): official `Packed bitstream`, Go `MuxingMode_PackedBitstream`.

Witnesses: C000167, C000168.

Official behavior. The translation table maps the key to "Packed bitstream" (`MediaInfo_Config_Automatic.cpp:1039`).

Go behavior. `avi.go:469` writes the key as the text value.

Verdict: bug.

## B1: BDAV text General ID falls back to the program number

Patterns: G00373 (20 files, text outputs only): official `0 (0x0)`, Go `1 (0x1)`. JSON has no difference.

Witnesses: C000009, C000010.

Go behavior. If the `transport_stream_id` is 0, Go writes the program number in the text General ID (`mpeg_ts.go:3051-3056`).

Verdict: bug. This is not proven on a file, because BDAV files can stall the media host.

## B2: BDAV audio "Muxing mode" is in JSON only

Patterns: G00426 (15 files, text outputs only): official `Stream extension` or `Blu-ray`, Go no line.

Go behavior. Some BDAV paths set `MuxingMode` as a JSON fact only (`mpeg_ts.go:366`, `2344`).

Verdict: existing ticket "Text: show the format facts that JSON already has" (#151). That ticket lists `mpeg_ts.go:2507`. Add these two lines to it.

## X1 and X2: Deliberate and existing tickets

- G00188 (65 Matroska files, new Image track): the Matroska cover attachment. `AGENTS.md` lists this as a deliberate difference.
- G00490 (11 heic files, missing Image track): "HEIF image files: scope and format name" (#167).
- G00621 (4 MP4 files, MuxingMode `Container profile=...`): "AVC and HEVC: take profile, level and tier from the SPS" (#155), decision 2.
- G00180, Ogg part, and G01438 (Ogg stream ID without hex): "Ogg and MP4: stream ID hex, ..." (#58), item 1.
- G00100 (`CompleteName_Last`): #116.

## Not explained

- U1: Go misses all audio and sees one video frame (C001099, C001563, C529688, C529706). Patterns: G00038 and G00039, MPEG-PS part. Go reports a duration of 0.033 s or 0.040 s against 13 s to 31 s. The PTS reset bug (#118) can be the cause, but #118 does not explain the missing AC-3 track.
- U2: Go finds an RLE track `189-32` that official does not find (C633430). Patterns: G00326 and G00499. The official RLE parser accepts any packet (`File_Rle.cpp:62-66`), so official did not see the packet.
- U3: AVI RGB files without `StreamOrder` in official (O2, AVI part).

## Questions for the contributor

These need the witness files:

1. C633576, C633578: what is the file name, and is the file in a `VIDEO_TS` folder with other VOB files of the same title set? Does Go find the RLE tracks with `--ParseSpeed=1`?
2. C001099, C529688: does the file have a PTS reset near the start? At which byte offset is the first AC-3 packet?
3. C001264, C282830: do the PES packets of stream 0xC9 or 0xDD have a PTS, and do they hold MPEG audio frames?
4. C633430: at which byte offset is the first packet of substream 0x20, and how large is it?
5. C609329: is the AVI stream type `iavs`?
6. C608393: which AVI stream header fields does the file have? Official writes no `StreamOrder` there.

## Corpus witness index

| Cause | Witness files |
|---|---|
| D1 | C000014, C000741, C001434, C001594, C000006, C000013, C000066, C000162 |
| D2 | C000005, C000012, C000013, C000015, C000024 |
| D3 | C000012, C000065, C000072, C001437 |
| D4 | C000022, C000066, C000132, C000162 |
| C1 | C000522, C000523, C001433, C277378 |
| C2 | C000162, C000522, C001433 |
| C3 | C000067, C000520, C000522 |
| C4 | C633576, C633578 |
| O1 | C001178, C001867, C529405, C529688, C610409, C636230 |
| O2 | C608243, C608415, C608619, C608393, C608397 |
| O3 | C000523, C000556, C633494, C633574 |
| A1 | C001264, C282830 |
| K1 | C000070, C000119, C000282 |
| K2 | C000235, C008834 |
| M1 | C608258, C608261, C608266, C608339 |
| M2 | C531701, C531702 |
| V1 | C609329 |
| V2 | C000167, C000168 |
| B1 | C000009, C000010 |
| B2 | C000009, C000010 |
