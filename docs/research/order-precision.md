# Research: field order, track order and numeric precision (issue #80)

Date: 2026-10-18. Parent map: #65. Rules: ADR 0001 and `AGENTS.md`.

This note answers one question. Which field order, track order and numeric precision differences of the 2026-10-17 corpus run come from go-mediainfo bugs, and which root causes make them? The run found 11 patterns: G00022, G00034, G00116, G00138, G00350, G01812 (order), and G00530, G00615, G01112, G01791, G01792 (precision). Each cause gets a verdict: bug, deliberate, decision, or out of scope. When an open ticket already covers a cause, the note names that ticket.

Field order is part of the compatible output shape. upbrr pastes the text output into tracker uploads, so a line in the wrong place is a visible difference.

## Method

- go-mediainfo: `main` at `edbef00`. Line numbers below refer to this commit.
- MediaInfoLib: tag `v23.04` in a local clone. Line numbers below refer to `v23.04`. The field order comes from `Source/Resource/Text/Stream/*.csv` at the same tag. In these files, the line number of a field is its position in the stream. The first letter of the fourth column (`Y` or `N`) tells if the text output shows the field.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. Text: `mediainfo --ParseSpeed=0.5` against `go-mediainfo`, with the `ReportBy` line removed. JSON: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` against `go-mediainfo --output=JSON --language=raw`. For key order, both JSON outputs went through `jq -c '[.media.track[]|keys_unsorted]'`. Raw text: `mediainfo --Language=raw --ParseSpeed=0.5` against `go-mediainfo --language=raw`. Each run used `ionice -c3 nice -n10 timeout 300`.
- Real files: samples `mp4-01`, `fresh-12` (text and JSON key order) and `dvd-02` (raw text). No other library file was read.
- Synthetic files: S1 to S8, listed at the end. `ffmpeg` made each one from `testsrc2` and `sine`. They contain no library content.
- Corpus data: the per-file pages of the corpus archive give the field lists of each order pattern. For each file, a script found the fields that are not in the longest common subsequence of the official list and the Go list. This note says that these fields "moved". The note groups the files by the moved field and its neighbors. Counts are numbers of files. One file can be in more than one group.
- A throwaway Go test (not committed) printed `textFieldOrder` for the labels below and listed which keys the JSON order tables contain.

## How official orders fields

Official MediaInfo keeps one field list per stream kind (General, Video, Audio, Text, Other, Image, Menu). The order of this list is the order of the CSV file of that kind. Every output walks the fields by position. First come the standard fields in CSV order, then the fields that the parser added by name and that are not in the CSV (`MediaInfo_Inform.cpp:678-679`, `MediaInfo_Internal.cpp:2516`). This note calls the second group "extra fields". Extra fields keep the order in which the parser filled them (`File__Analyze_Streams.cpp:1826`). In JSON and XML, extra fields go into the `extra` object at the end of the track.

The order of the standard fields is therefore the same for every container. Only the set of fields changes.

## How go-mediainfo orders fields

- Text, CSV and HTML: `projectTextStore` sorts by a number from `textFieldOrder` and then by fill sequence (`field_project_text.go:46-53`). The numbers come from two hand-made tables: `textGeneralFieldOrder` for General and `textStreamFieldOrder` for all other kinds (`field_order_registry.go:7-148`, `151-157`). A label that is not in the table gets `unregisteredFieldOrder` and prints after all registered labels (`field_schema.go:196-202`). HTML and CSV use the same text projection (`output_html.go:13`, `output_csv.go:12`).
- Raw text: separate hand-made tables (`field_project_raw_text.go:1971-2012`), and a second set for DVD files (`field_project_raw_text.go:650-681`).
- JSON and XML: one hand-made table per container and kind (`field_order_registry.go:159-674`). A key that is not in the table goes after all known keys, so it goes after `extra` (`field_project_structured.go:107-125`, `json_fields.go:34-53`).

## Summary

| Cause | Patterns (part) | Outputs | Verdict | Ticket |
|---|---|---|---|---|
| R1 Text order table does not follow the stream tables | G00034, G00116, G00138, G00350, G01812 (text parts, about 800 file groups) | text, CSV, HTML | Bug | new |
| R2 JSON order tables miss keys and put some keys in the wrong place | G00034, G00116, G00138, G00350 (JSON parts, 56 file groups) | JSON, XML | Bug | new |
| R3 Extra fields get fixed places instead of fill order | G00116 (8), G00138 (8), G00350 (2), G00034 (1, not proven) | text, raw text | Bug | new |
| R4 Raw text shows "Statistics Tags Issue" twice | G00034 (13), G00116 (13), G00138 (6) | raw text | Bug | new |
| R5 DVD raw text table has no `Encoded_Library/String` | G00034 (4) | raw text | Bug | new, or part of R1 |
| R6 Track sequence follows from missing or extra tracks | G00022 | all | Covered by other tickets | #77, #79, #116, #134, #167, #176, `AGENTS.md` |
| R7 Matroska track duration from the Segment Duration has 9 decimals | G00530, G00615 | JSON, XML | Bug | new |
| R8 AC-3 `compr` for gain code 0 | G01112, G01791, G01792 | JSON, XML | Decision | new |
| R9 Differences only in HTML | parts of G00116 (10), G00350 (4) | HTML only | Out of scope (ADR 0001 rule 3) | none |

v26.10 gives no reason to keep a Go-only order. None of the 11 patterns has v26.10 data.

## R1: Text order table does not follow the stream tables

Patterns and witnesses. The moved fields and the number of files:

| Moved field (text label) | Official place | Go place | Files | Containers | Witnesses |
|---|---|---|---|---|---|
| Standard | after Frame rate | after Color space | about 250 | MPEG-PS, Matroska, MPEG-4 | C000014, C000017, C000034 |
| Format settings, GOP | after Format settings, Reference frames | after Format settings | about 120 | MPEG-4, Matroska | C000147, C000209, C000220 |
| colour_primaries_Original, transfer_characteristics_Original, matrix_coefficients_Original | after the base colour line | last, after Codec configuration box | about 55 | MPEG-4 | C518295, C521913, C000220 |
| Muxing mode | after the last Format settings line | after Format profile | 41 | Matroska, AVI | C000042, C000168, C000167 |
| Commercial name | after Format | after Format version | 1 | MPEG-TS | C000234 |
| Alternate group | after Default and Forced | after Tagged date | 54 | MPEG-4 | C000204, C000208 |
| Writing application (Audio) | after Title, before Writing library | after Forced | 43 | WebM, Matroska | C608371, C000045 |
| Forced | after Default | after Encoded date | 17 | Matroska, MPEG-4 | C000260, C000221 |
| Count of elements (Text) | after Frame rate | after Compression mode | 68 | Matroska | C000150, C000161 |
| Muxing mode, more info and Start time (Text) | after Format and after Duration | after Stream size | 40 | MPEG-PS, MPEG-TS | C000067, C000520, C000234 |
| Album, Description (General) | after Frame rate, before Encoded date | after Writing application | 13 | MPEG-4 | C000220, C608835, C531699 |
| Format profile (General) | before Format settings | after Format settings | 3 | AVI | C000172 |
| List (Menu) | after Duration | after Language | 1 | MPEG-TS | C000234 |
| Scan type, store method | after Scan type | after Forced | 2 | Matroska | C000185, C529429 |

Official behavior. The v23.04 CSV positions give the official order:

- `Video.csv:24-25` Format_Commercial_IfAny, then Format_Version. `Video.csv:55`, `60`, `64`, `65` RefFrames, GOP, MuxingMode, CodecID. `Video.csv:200`, `202`, `206` FrameCount, Standard, ColorSpace. `Video.csv:213`, `218` ScanType, ScanType_StoreMethod. `Video.csv:339`, `341`, `342` Forced, AlternateGroup, Encoded_Date. `Video.csv:358`, `360` transfer_characteristics, transfer_characteristics_Original.
- `Audio.csv:251`, `253`, `259` Title, Encoded_Application, Encoded_Library. `Audio.csv:278`, `280`, `282`, `283` Default, Forced, AlternateGroup, Encoded_Date.
- `Text.csv:32-34` MuxingMode, MuxingMode_MoreInfo, CodecID. `Text.csv:67` Duration_Start ("Start time"). `Text.csv:148`, `162`, `171` FrameRate, ElementCount, Compression_Mode. `Text.csv:303-304` FirstDisplay_Delay_Frames, FirstDisplay_Type.
- `General.csv:67`, `68`, `71` Format_Version, Format_Profile, Format_Settings. `General.csv:183`, `187`, `264`, `275`, `286`, `340` Movie, Album, Description, Encoded_Date, Encoded_Application, Comment.
- `Menu.csv:62`, `63`, `65` List, Title, Language.

Go behavior. The throwaway test printed these numbers from `textFieldOrder`:

- Not in the table, so they print last: "Writing application" for Audio (the table has it for General only, `field_order_registry.go:26`), "Muxing mode, more info", "Start time", "Count of frames before first event", "Type of the first event", "transfer_characteristics_Original", "Scan type, store method", "Album", "Description", "Comment".
- In the wrong place: "Muxing mode" 6 before "Format settings" 7 (`field_order_registry.go:43`, `46`). "Format settings, GOP" 12 before "Format settings, CABAC" 13 (`:51-52`). "Count of elements" 42 after "Compression mode" 36 (`:81`, `:88`). "Encoded date" 51 between "Default" 49 and "Forced" 53 (`:96-99`). "Alternate group" 58 after "Tagged date" 52 (`:140`). General "Format settings" 3 before "Format profile" 4 before "Format version" 5 (`:14-16`).
- Ties that the fill sequence breaks: "Standard" and "Color space" are both 32 (`:74-75`). "Commercial name" and "Format version" are both 4 (`:40-41`). When the parser fills Standard after Color space, Standard prints second.
- One table for all non-General kinds (`field_order_registry.go:151-157`). Menu uses the stream table, where "List" is 60 and "Title" is 44 (`:91`, `:142`). So List prints after Title and Language.

Evidence.

- S1 and S7 (MP4, AVC with a PAL VUI, two AAC tracks, mov_text, General tags). Official text has `Format settings, Reference frames`, then `Format settings, GOP`. Go has `Format settings, GOP` right after `Format settings`. Official has `Frame rate`, `Standard`, `Color space`. Go has `Frame rate`, `Color space`, `Standard`. Official General has `Frame rate`, `Movie name`, `Album`, `Description`, `Encoded date`, `Tagged date`, `Writing application`, `Comment`. Go General has `Frame rate`, `Encoded date`, `Tagged date`, `Writing application`, `Title`, `Album`, `Comment`, `Description`.
- S7 (S1 with creation dates). Official Audio and Text have `Default`, `Forced`, `Alternate group`, `Encoded date`, `Tagged date`. Go has `Default`, `Encoded date`, `Tagged date`, `Forced`, `Alternate group`.
- S5 (MPEG-2 PAL in MPEG-PS). Go prints `Standard` before `Color space` here, because the parser fills Standard first. This shows that the result of the tie depends on the fill order.
- Samples `mp4-01` and `fresh-12`: no order difference. These files do not have the moved fields (no Alternate group, no Compression mode on the PGS tracks).

Scope. The text output, and CSV and HTML through the same projection. The JSON order of these fields is cause R2.

Overlap. The two Matroska files with "Scan type, store method" are witnesses of #136. That ticket covers the store method label and the "Picture structure" line, which prints last for the same reason. The label "Title" in General is #145. The missing text line "Original display aspect ratio" is #90. These tickets change labels. They do not change the order table.

Verdict: bug. One root cause: the text order is a hand-made table, not the order of the stream tables.

## R2: JSON order tables miss keys and put some keys in the wrong place

Patterns and witnesses.

| Moved key | Official place | Go place | Files | Container | Witnesses |
|---|---|---|---|---|---|
| Standard (Video) | after FrameCount | after `extra` | 29 | MPEG-4 | C608346, C608347 |
| DisplayAspectRatio_Original (Video) | after DisplayAspectRatio | after `extra` | 13 | MPEG-4 | C608222, C608267 |
| Encoded_Application and Comment (General) | Encoded_Application before Comment | Comment before Encoded_Application | 6 | MPEG-4 | C608466, C608835 |
| Description (General) | after Title and Album | after all known keys | 3 | MPEG-4 | C531699, C531701 |
| BitDepth, StreamSize, Encoded_Library_Date (Audio) | BitDepth after SamplingCount, StreamSize before Encoded_Library | BitDepth after `extra`, Encoded_Library before StreamSize | 3 | FLAC | C000001, C000310, C226345 |
| Album, Genre, Label, Part, Performer, file dates (General) | CSV order | tag order, file dates first | 1 | FLAC | C226345 |
| MuxingMode (Text) | after Format | after Delay | 1 | MPEG-PS | C636230 |

Official behavior. JSON keys follow the CSV order of the kind, then `extra`. Examples: `Video.csv:169`, `171` DisplayAspectRatio, DisplayAspectRatio_Original. `Video.csv:200`, `202` FrameCount, Standard. `General.csv:175`, `187`, `264`, `279`, `286`, `340` Title, Album, Description, File_Created_Date, Encoded_Application, Comment. `Audio.csv:153`, `159`, `205`, `258` BitDepth, Compression_Mode, StreamSize, Encoded_Library. `Text.csv:32`, `173` MuxingMode, Delay.

Go behavior.

- `structuredMP4VideoFieldOrder` has no `Standard` and no `DisplayAspectRatio_Original` (`field_order_registry.go:318-333`). Unknown keys sort after `extra` (`field_project_structured.go:120-124`).
- `structuredMP4GeneralFieldOrder` puts `Comment` before `Encoded_Application` and has no `Description` (`field_order_registry.go:309-316`).
- `structuredAudioFieldOrder`, the default for FLAC, has no `BitDepth` and no `Encoded_Library_Date`. It puts `Encoded_Library` 35 before `StreamSize` 36 (`field_order_registry.go:191-202`, `:198`).
- `structuredTextFieldOrder`, the default for MPEG-PS, has no `MuxingMode`. It puts `CodecID` before `MuxingMode_MoreInfo` (`field_order_registry.go:254-261`, `:256`).
- `structuredGeneralFieldOrder` lists the file dates but no tag fields. So the FLAC Vorbis comment fields keep the tag order after the dates (`field_order_registry.go:159-167`).
- Go has 25 order tables for six containers and a default. Official has one per kind.

Evidence. S1 (MP4). Official Video keys: `... DisplayAspectRatio DisplayAspectRatio_Original Rotation ... FrameCount Standard ColorSpace ...`. Go: `... DisplayAspectRatio Rotation ... FrameCount ColorSpace ... extra Standard DisplayAspectRatio_Original`. Official General: `Title Movie Album Description File_Modified_Date File_Modified_Date_Local Encoded_Application Comment`. Go: `Title Movie Album File_Created_Date ... File_Modified_Date_Local Comment Encoded_Application Description`. Sample `mp4-01` has equal JSON key order, because the file has none of these keys.

Scope. JSON and XML. upbrr reads JSON by key, so the effect on upbrr is small. The key order is still part of the compatible output shape.

Verdict: bug.

## R3: Extra fields get fixed places instead of fill order

Patterns and witnesses.

- G00116, text: a Matroska tag such as `SOURCE` prints after "Bed channel configuration" in Go. Official prints the tag first. 8 files: C000153, C000154, C007147, C529865.
- G00138, raw text: a Matroska Text `SOURCE` tag prints after `SOURCE_ID` in Go. Official prints `SOURCE` right after `Forced/String`. 8 files: C000235, C008834, C008837.
- G00350, raw text: a custom General tag in WebM (C610956) and a Vorbis comment tag in FLAC (C226345) print in another place.
- G00034, text and JSON: a Video `Comment` prints after `Source` in Go and before it in official. 1 file: C043124. This note did not prove the cause for this file.

Official behavior. Fields that are not in the CSV keep the fill order (`File__Analyze_Streams.cpp:1826`). For Matroska, official fills the track tags from a `std::map`, so they come in tag name order (`File_Mk.h:478`, `File_Mk.cpp:1031`). Official fills the tags before it finishes and merges the codec parser (`File_Mk.cpp:1043`, `1290`). So a tag such as `SOURCE` comes before parser fields such as the bed channel configuration or the FLAC MD5.

Go behavior. Go puts many extra fields at fixed numbers. In text, the Atmos and AC-3 fields have 53, the same as `Forced` (`field_order_registry.go:99-131`). Tags that are not in the table print after them. In raw text, the stream table pins `Source`, `Source_ID`, `SOURCE`, `SOURCE_ID` and some General tags (`field_project_raw_text.go:1978`, `1990`, `2006`). The raw Text table has `Source` and `Source_ID` but not `SOURCE` (`field_project_raw_text.go:1990`). The FLAC MD5 is pinned after all tags (`field_order_registry.go:147`). That pin agrees with official for one tag, but it is not a general rule.

Evidence. S2 (Matroska, FLAC with a `SOURCE` tag and statistics tags with `SOURCE_ID`): text and raw text agree for this file. Official and Go both show `SOURCE` before `MD5 of the unencoded content`. The corpus files with the difference also have Atmos fields or a Text `SOURCE` tag. This note did not make a synthetic file with both.

Scope. Text and raw text.

Verdict: bug. Official keeps extra fields in the parser fill order. Go replaces that rule with fixed numbers.

## R4: Raw text shows "Statistics Tags Issue" twice

Patterns. G00034 raw text (13 files), G00116 raw text (13), G00138 raw text (6). Witnesses: C000146, C000160, C529596, C007226, C007289, C286256.

Official behavior. `File_Mk.cpp:834` fills one field, `Statistics Tags Issue`, before the `FromStats_*` fields (`File_Mk.cpp:934`). Raw text shows it once, with this label, after the last standard field.

Go behavior. Go makes two copies. The `extra` member `Statistics_Tags_Issue` (`matroska.go:4594`, `matroska_scan.go:1453`) gets the raw label `  Issue` from `field_project_raw_text.go:145`. This label is in the table before `FromStats_*` (`field_project_raw_text.go:1990`, `2005`). The text field `Statistics Tags Issue` (`matroska_scan.go:1458-1459`) has no raw label in the table, so it prints last.

Evidence. S2 (Matroska, statistics tags with a writing date before the segment date). Official raw video: `colour_range`, `Statistics Tags Issue`, `FromStats_BitRate` ... `FromStats_StreamSize`. Go: `colour_range`, `  Issue`, `FromStats_BitRate` ... `FromStats_StreamSize`, `Statistics Tags Issue`. The text output is equal for S2.

Scope. Raw text. The same corpus files also show the difference in HTML. Go HTML uses the text projection, and the S2 text is equal. So this note cannot explain the HTML part (see the open questions).

Verdict: bug.

## R5: DVD raw text table has no `Encoded_Library/String`

Patterns. G00034 raw text, 4 MPEG-PS files: C001014, C001038, C277690, C633506.

Official behavior. In `Video.csv:318`, Encoded_Library comes after StreamSize and before the colour fields.

Go behavior. When the General format is DVD Video or a Menu has the format DVD-Video (`field_project_raw_text.go:368-387`), raw text uses `rawTextDVDVideoFieldOrder` (`field_project_raw_text.go:651-659`). This table has no `Encoded_Library/String`, so the row prints after `matrix_coefficients`.

Evidence. Sample `dvd-02`, raw text. Official Video: `StreamSize/String`, `Encoded_Library/String`, ..., `transfer_characteristics`, `matrix_coefficients`. Go: `StreamSize/String`, ..., `transfer_characteristics`, `matrix_coefficients`, `Encoded_Library/String`.

Verdict: bug. It has the same root cause as R1 (a hand-made table) in the raw text path. It can be part of the R1 ticket.

## R6: Track sequence

Pattern. G00022 has 805 files, and 243 of them are in containers that Go parses. The pattern compares the list of tracks and IDs. In each of the 243 files, another pattern also shows a whole track that is missing or extra, or a different ID. The table gives the companion patterns.

| Container | Files | What differs | Owner |
|---|---|---|---|
| Matroska | 65 | Go adds an Image track for the cover (G00188) | Deliberate, `AGENTS.md` |
| MPEG-4 | 18 | Go has no `Other` time code track (G00395) | #176 |
| MPEG-4 | 2 | a Text track is missing in Go (C531701, C531702) | not known, see the open questions |
| heic | 11 | Go has no Image tracks (G00490) | #167 |
| DVD Video | 30 | Menu count, track IDs, Text count (G00030, G00064, G00150, G00180) | #134, #79, #116 |
| MPEG-PS | 116 | Text tracks missing or extra, Audio tracks missing or extra, IDs in another order (G00064, G00326, G00524, G00038, G00150, G00180) | #77 |
| AVI | 1 | Go has no Audio and no Video track (C609329) | not known, see the open questions |

Go behavior. Go sorts tracks by kind and keeps the parser order inside a kind (`streams.go:5-14`, `order.go:3-10`). Official uses the same kind order. Go has no `Other` kind in `streamKindOrder`. When #176 adds the time code track, the track must go between Text and Image, as in official.

Verdict: no cause of its own. The pattern count drops when the tickets in the table close.

## R7: Matroska track duration from the Segment Duration has 9 decimals

Patterns. G00530 (Audio, 8 files: 7 WebM, 1 Matroska) and G00615 (Video, 4 WebM files). JSON and XML only. Official `34.119`, Go `34.119000000`. Witnesses: C608371, C608689, C670517, C608643, C609237.

Official behavior. When a track has a `DURATION` tag, official fills the track duration with as many decimals as the tag has (`File_Mk.cpp:1027`). JSON then shows 9 decimals. When the track has no tag, the track duration has whole milliseconds, so JSON shows 3 decimals. `MediaInfo_Inform.cpp:811-819` adds 3 to the decimals of the millisecond value. The General duration has 0 decimals in milliseconds (`File_Mk.cpp:773`).

Go behavior. When the Segment Duration element is 8 bytes, Go sets the precision to 9 (`matroska.go:1533-1542`). Each track that takes the Segment Duration gets 9 decimals (`matroska_audio_canonical.go:77-88`, `matroska_video_canonical.go:142-152`, and the other `matroska_*_canonical.go` files).

Evidence.

- S3 (WebM from `ffmpeg`, with `DURATION` tags): official and Go both show `3.000000000` and `3.008000000`.
- S6 (S3 with the `DURATION` tag names changed) and S8 (the same in Matroska): official Video `3.000`, Audio `3.008`. Go Video `3.008000000`, Audio `3.008000000`. The decimals differ. The video value also differs. That value belongs to the measured duration work (#66).

Verdict: bug. 9 decimals of a value that the file stores in milliseconds add no precision.

## R8: AC-3 `compr` for gain code 0

Patterns. G01112 `extra/compr` (2 files, MPEG-PS and Matroska), G01791 `extra/compr_Average` and G01792 `extra/compr_Maximum` (1 Matroska file). JSON and XML only. Official `-0.00`, Go `0.00`. Witnesses: C001165, C531126.

Official behavior. The gain is `AC3_compr[code>>4] + 20*log10((16+(code&15))/32)` (`File_Ac3.cpp:1348`, `1521`). The table uses the rounded step 6.02 (`File_Ac3.cpp:553-571`). For code 0 the result is 6.02 - 6.0206 = -0.0006, which prints as `-0.00`. `compr_Average`, `compr_Minimum` and `compr_Maximum` use the same value. The current MediaInfoLib has the same table (`File_Ac3.cpp:605-623` on the default branch).

Go behavior. `ac3ComprDB` returns exactly 0 for code 0 (`ac3.go:1800-1803`). `comprStats` then changes the minimum to negative zero to match official (`ac3.go:1645-1647`). It does not change the single value, the average or the maximum.

The AC-3 gain word for code 0 means a gain of exactly 0 dB. So `0.00` is the correct value, and `-0.00` comes from the rounded 6.02 constant. Go uses the official value for `compr_Minimum` only, and the exact value for the other three fields.

Verdict: decision. The maintainer must choose one rule for all four fields. If Go keeps `0.00`, the difference goes on the deliberate list in `AGENTS.md`.

## R9: Differences only in HTML

- G00116: `MD5 of the unencoded content` and `SOURCE` in Matroska FLAC (10 files, C000235, C008834, C000002, C000004).
- G00350: `Attachments` and `ErrorDetectionType` against custom General tags in Matroska (4 files, C000203, C000242, C000289, C000303).

For these files the pattern reports HTML only. Go HTML uses the text projection, and the text output has no order difference. So the official HTML order differs from the official text order here. ADR 0001 rule 3 says that a difference in HTML only counts if it has the same cause as a JSON or text difference.

Verdict: out of scope.

## Proposed tickets

1. Text: take the field order from the MediaInfo stream tables (R1, and R5 for the DVD raw text table).
2. JSON: put each standard key in the MediaInfo stream table order (R2).
3. Keep extra fields in the parser fill order (R3).
4. Raw text: show "Statistics Tags Issue" once, with the official label (R4).
5. Matroska: write a track duration from the Segment Duration with millisecond precision (R7).
6. Decision: AC-3 `compr` for gain code 0, `0.00` or `-0.00` (R8).

## Open questions for the contributor

- R7: do the tracks in C608371 and C608643 have a `DURATION` tag? Is the Segment Duration element 8 bytes?
- R4: in C000160 and C529596, where is "Statistics Tags Issue" in the text output of each tool? The HTML difference has no cause yet.
- R3: in C043124, which tags does the video track have, and where does `Source` come from?
- R6: which Text track is missing in Go for C531701 and C531702 (MP4)? Why does Go find no Audio or Video track in C609329 (AVI)?
- R9: for C000235 and C000203, is the text output order of both tools equal?

## Synthetic files

- S1: MP4. AVC 720x576 25 fps from `libx264` with `videoformat=pal`. Then `h264_metadata` changes the VUI transfer and sample aspect ratio. Two AAC tracks, one `mov_text` track, General `title`, `album`, `description`, `comment`.
- S2: Matroska. AVC, FLAC, SRT. Hand-written statistics tags per track, with a video statistics date before the segment date. FLAC has a `SOURCE` tag and `SOURCE_ID`. The SRT track has a `SOURCE` tag.
- S3: WebM. VP9 and Opus from `ffmpeg`, with the default per-track `ENCODER` and `DURATION` tags.
- S4: S3 remuxed. This note does not use it.
- S5: MPEG-PS. MPEG-2 720x576 25 fps and AC-3.
- S6: S3 with each `DURATION` tag name changed to `DURATIOX`, so no track has a duration tag.
- S7: S1 with `creation_time` on the file and on each track.
- S8: S3 remuxed to Matroska, then changed like S6.
