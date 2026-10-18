# Encoder tags: root causes (issue #76)

Research date: 2026-10-18. Map issue: #65. Corpus run: 2026-10-17.

This file groups the encoder patterns of the corpus run into root causes. The ticket names 31 patterns in the fields `Encoded_Library`, `Encoded_Library_Name`, `Encoded_Library_Version`, `Encoded_Library_Date`, `Encoded_Library_Settings`, `Writing Library` and `Encoded by`. The patterns come from the triage clusters 36, 71, 80, 89, 91, 101, 107, 115, 124, 129, 150, 156, 175, 176 and 191 to 195. upbrr reads `Encoded_Library_Settings` and `Encoded_Library_Name`, so the causes that change these two fields come first.

Each cause has one of these verdicts:

- Bug. go-mediainfo is wrong and must change.
- Decision. go-mediainfo follows a newer MediaInfo, or go-mediainfo can be more correct than v23.04. `AGENTS.md` does not record it. ADR 0001 (#64) rule 2 requires a recorded reason, or a revert to v23.04.
- Existing ticket. An open bug ticket already covers the root cause.

The file refers to corpus files by C-id and to patterns by G-id. When the triage sheet gives no G-id for a pattern, the file uses the cluster number. The file names no release, path or tag value. It describes a corpus value only by its shape. All values in the synthetic files are made up for this research or are public tool names.

## Sources and method

- go-mediainfo: `origin/main` at `6a37300`. Citations are `internal/mediainfo/<file>:<line>` at that commit. The cited files did not change up to `edbef00`.
- MediaInfoLib: tag `v23.04`, read from GitHub for `MediaArea/MediaInfoLib`. Citations are `Source/MediaInfo/<file>:<line>` at that tag, unless the text says "master". "Master" is the local checkout at `~/github/oss/MediaInfoLib`, which is newer than v26.10. The version tests read `File_Mk.cpp` and `File_Mpeg4_Elements.cpp` at all tags from `v23.06` to `v26.10`.
- Triage data: the local triage sheet of the 2026-10-17 run. It gives the outputs, containers and value shapes for each cluster. It does not map a witness file to one pattern, so the witness lists below are per cluster.
- Synthetic files: T1 to T23, made on the local machine with `ffmpeg`, `libx264`, `libx265`, `libvpx`, `libtheora`, `libvorbis` and `libopus`. A script patched bytes in some files, for example to add an SEI message or to change the Matroska `MuxingApp`. The section "Synthetic files" lists them.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The Go binary was a local `linux/amd64` build of `6a37300` at `/tmp/go-mediainfo-research-76`. The research copied only the synthetic files to the host and read no library file. The research removed its files from the host after the runs.
- Host commands: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5 <f>`, `mediainfo --ParseSpeed=0.5 <f>`, `mediainfo --ParseSpeed=0.5 --Language=raw <f>`, and the same three outputs from the Go binary. Each command ran under `ionice -c3 nice -n10 timeout 300`.

In this file, "text" is the default text output. "Raw text" is the text output with `--Language=raw`.

## How official v23.04 fills the encoder fields

Five rules explain most patterns. The causes below refer to them.

1. Text shows `Encoded_Library/String` with the label `Writing library`. JSON shows `Encoded_Library`, `Encoded_Library_Name`, `Encoded_Library_Version` and `Encoded_Library_Date`. Text does not show the name, version or date fields. The options are `N YTY`, `Y NT` and `N NTY` in `MediaInfo_Config_Automatic.cpp:4597-4603` for General, `:4978-4984` for Video and `:5309-5315` for Audio. The labels are at `:405-408`.
2. `Encoded_Library/String` is `Name Version (Date)` when a parser sets a name. Without a name, it is `Encoded_Library` (`File__Analyze_Streams_Finish.cpp:55-81`, `:1739-1753`).
3. When a field already has a value, `Fill` appends ` / ` and the new value (`File__Analyze_Streams.cpp:1045-1052`).
4. Matroska tags without a target go to General only. Tags with a track target go to that track (`File_Mk.cpp:779-787`, `:995-1033`). `ENCODER` becomes `Encoded_Library`, and `ENCODER_SETTINGS` becomes `Encoded_Library_Settings` (`File_Mk.cpp:3341-3342`). A stream parser that runs after the tags replaces the tag value. T4 and T14 show this for the x264 SEI.
5. A stream parser sets the name and the version only for the encoder strings that it knows. For an unknown string under a known SEI UUID or in MPEG-2 user data, it sets the name to the full string (`File_Avc.cpp:3524-3541`, `File_Hevc.cpp:2970-2977`, `File_Mpegv.cpp:3237-3238`).

## Summary

| Cause | Verdict | Patterns | Files (about) |
|---|---|---|---|
| R1. Matroska General tags go to the video track | Bug | G00474, candidate parts of G00200 and G00591 | 12 |
| R2. SEI string of another encoder gets no `Encoded_Library_Name` | Bug | G00473 (AVC part) | up to 12 |
| R3. HEVC ATEME SEI: Go knows one product prefix only | Bug | G00315, G00473, G00492 (HEVC parts) | up to 7 |
| R4. MPEG-2 user data: Go knows two encoders only | Bug | G00315 (MPEG-2 part), G00473 and G00492 (MPEG-PS parts), G00375 (MPEG-PS part) | up to 26 |
| R5. MPEG-PS General library drops the `encoded by ` prefix | Bug | G00375 (MPEG-PS part) | 5 |
| R6. Matroska video `ENCODER` tag: Go keeps known encoder names only | Bug | G00315 (WebM and Matroska parts) | up to 17 |
| R7. Matroska General `ENCODER` tag is joined only when it starts with `Lavf` | Bug | G00375 (Matroska part) | up to 14 |
| R8. Matroska `CodecName` becomes `Encoded_Library` | Bug | cluster 195 | 1 |
| R9. Vorbis and Theora vendor strings: Go does not use the official split | Bug | G00508, clusters 124, 156, 175 and 176 (Ogg and WebM parts) | 11 |
| R10. Ogg text shows the library name and date lines | Bug | text parts of clusters 156, 175 and 176 | 2 |
| R11. Text `Writing library` does not use rule 2 | Bug | text parts of G00375 and of clusters 156 and 175, parts of G00130 and G00245 (sheet group "deliberate") | not counted |
| R12. `EncodedBy` has no text label | Bug | clusters 150 and 191 to 194 | 4 |
| R13. MP4 and MOV: no General library from the handler manufacturer | Bug | G00616, candidate MP4 part of G00547 | 4 |
| R14. Matroska Theora: Go does not read the vendor string | Bug | no pattern mapped | 0 known |
| R15. MP4 visual compressor name becomes `Encoded_Library` | Decision | G00200 (MP4 part) | 56 |
| R16. Matroska General `Encoded_Library` is split into name and version | Decision | G00369, G00370, raw-text part of G00375 | 20 |
| R17. Matroska `ENCODER_OPTIONS` becomes `Encoded_Library_Settings` | Decision | G00304, also G00303 and G00306 | 29 |
| R18. MP4 does not parse the MPEG-4 Visual stream | Existing ticket #98 | G00315, G00473, G00492 (MPEG-4 Visual parts), MP4 part of cluster 156 | about 4 |

The section "Patterns not fully explained" lists G00547, G00591 and the cluster 101 and 129 patterns without a G-id.

## R1. Matroska General tags go to the video track

### What goes wrong

`applyMatroskaEncoders` uses the General `ENCODER` and `ENCODER_SETTINGS` tags for a video track that has no track tag (`matroska.go:4897-4903`). It then fills `Encoding settings` on the video track (`matroska.go:4912-4915`). The x264 SEI comes later, and Go keeps a value of `Encoding settings` that is already there (`matroska_scan.go:2242`). Go thus loses the x264 SEI settings.

Official v23.04 puts tags without a target in General only (rule 4). The video settings come from the SEI.

### Proof

| File | Field | Official | Go |
|---|---|---|---|
| T20, x264 with SEI, General `ENCODER_SETTINGS` | Video `Encoded_Library_Settings` | SEI options, joined with ` / ` | the General tag value |
| T21, x264 without SEI, General `ENCODER_SETTINGS` | Video `Encoded_Library_Settings` | absent | the General tag value |

General `Encoded_Library_Settings` is the same in both programs. Text and raw text show the same difference as JSON.

### Verdict

Bug. upbrr reads `Encoded_Library_Settings`, so this cause has the largest effect on upbrr in this ticket.

Witness files (cluster 115): C009476, C010392, C608715.

## R2. SEI string of another encoder gets no `Encoded_Library_Name`

### What goes wrong

An AVC stream can carry a user data SEI with the x264 UUID and the name of another encoder. Go reads the string with `findAVCEncoderInfo` (`x264.go:75-95`). Go sets the name only through `splitEncodedLibrary`, which knows only `x264`, `x265` and one fixed service name (`json_fields.go:697-716`, `analyze.go:2910`). A second rule sets the name only when the string contains the word `Encoder` (`analyze.go:2916-2919`).

The HEVC path has the same gap. For an x265 UUID with a string that does not start with `x265`, Go stops (`hevc_x265.go:89-91`). Official sets the name to the full string (rule 5).

### Proof

T22 is AVC with the x264 UUID and the string `ExampleAVC 2.1`, in Matroska and in MP4.

| Output | Official | Go |
|---|---|---|
| Video `Encoded_Library` | `ExampleAVC 2.1` | `ExampleAVC 2.1` |
| Video `Encoded_Library_Name` | `ExampleAVC 2.1` | absent |

Text and raw text are the same in both programs. This agrees with the scope of G00473, which is JSON and XML only.

### Verdict

Bug.

Witness files (cluster 101): C001029, C608715, C632268.

## R3. HEVC ATEME SEI: Go knows one product prefix only

### What goes wrong

Go reads an SEI with the ATEME UUID only when the string starts with the one product name that Go knows (`hevc_x265.go:26-31`). Go also trims spaces from the string. Official takes the full payload as `Encoded_Library`. For a string that starts with `ATEME `, it sets the name to the text before the first digit and the version to the rest. It does not trim (`File_Hevc.cpp:2845-2874`).

### Proof

T10 is HEVC in Matroska with an ATEME UUID SEI. The string has a different ATEME product name, a version and one trailing space.

| Output | Official | Go |
|---|---|---|
| Video `Encoded_Library` | the full string, trailing space kept | absent |
| Video `Encoded_Library_Name` | the product name | absent |
| Video `Encoded_Library_Version` | the version, trailing space kept | absent |
| Text `Writing library` | the full string | absent |

The corpus agrees in shape. One G00315 value is an ATEME string with a trailing space.

### Verdict

Bug.

Witness files (clusters 36, 101 and 107): C608340, C608712, C610651, C001029, C608715, C632268.

## R4. MPEG-2 user data: Go knows two encoders only

### What goes wrong

Go reads an MPEG-2 user data string only when it contains `TMPGEnc` or `Saar Software` (`mpeg2_video.go:275-291`). Official takes any printable run from the user data packet, with a fixed character set, and keeps it when it has 8 or more characters (`File_Mpegv.cpp:3135-3242`). It fills Video and General `Encoded_Library`, `Encoded_Library_Name` and `Encoded_Library_Version` (`File_Mpegv.cpp:1415-1424`). For an unknown string, the name is the full string.

### Proof

T9 is MPEG-2 with the user data string `Example MPEG-2 Encoder v1.2.3-20200101`, as an elementary stream, in MPEG-PS and in MP4.

| File | Official | Go |
|---|---|---|
| T9, elementary stream | General and Video `Encoded_Library` and `Encoded_Library_Name` | none |
| T9, MPEG-PS | General `Encoded_Library`, Video `Encoded_Library` and `Encoded_Library_Name` | none |
| T9, MP4 | Video `Encoded_Library` and `Encoded_Library_Name` | none |

Official text and raw text show `Writing library`. Go shows no line. The corpus agrees in shape. One G00315 value is an encoder string with a version and a build date.

### Verdict

Bug.

Witness files (clusters 36, 101, 107 and 91): C608340, C608712, C610651, C001029, C608715, C632268, C001014.

## R5. MPEG-PS General library drops the `encoded by ` prefix

### What goes wrong

For MPEG-PS, Go copies the video `Encoded_Library` to General, but it first removes the prefix `encoded by ` (`analyze.go:1776-1778`). Official copies the full string (`File_Mpegv.cpp:1421`).

### Proof

T15 is MPEG-PS with a TMPGEnc user data string that starts with `encoded by `. Official General `Encoded_Library` is the full string. Go General `Encoded_Library` has no prefix. Text and raw text show the same difference.

### Verdict

Bug. One fix can cover R4 and R5, because both are in the MPEG-2 library path.

Witness files (cluster 91): C000070, C001014, C531302.

## R6. Matroska video `ENCODER` tag: Go keeps known encoder names only

### What goes wrong

For a video track, Go uses the track `ENCODER` tag only when the value contains one of five encoder names (`matroska.go:4904-4910`). The code comment says that this keeps muxer names out. Official fills every track tag (rule 4). The stream parser then replaces the tag value when it finds an encoder string.

### Proof

T3 is WebM with VP9 and Opus from `ffmpeg`. Each track has an `ENCODER` tag. Official Video `Encoded_Library` is the libavcodec and libvpx-vp9 string. Go has no Video `Encoded_Library` and no text line. The Opus track is the same in both programs.

### Verdict

Bug.

Witness files (cluster 36): C608340, C608712, C610651.

## R7. Matroska General `ENCODER` tag is joined only when it starts with `Lavf`

### What goes wrong

Go joins the General `ENCODER` tag to `MuxingApp` only when the tag starts with `Lavf` (`analyze.go:1047-1050`). Official joins every General `ENCODER` tag that is not equal to `MuxingApp`, with ` / ` (`File_Mk.cpp:783-786`, rule 3).

### Proof

T13 is Matroska with the `MuxingApp` from `ffmpeg` and a General `ENCODER` tag `Example 1.23`. Official General `Encoded_Library` is `<MuxingApp> / Example 1.23`. Go has `<MuxingApp>` only. T20 and T21 show the same with a General tag that starts with `x264`.

### Verdict

Bug.

Witness files (cluster 91): C000070, C001014, C531302.

## R8. Matroska `CodecName` becomes `Encoded_Library`

### What goes wrong

Go reads the Matroska `CodecName` element (`matroska.go:1795-1797`). When the value contains `Lavc`, Go uses it as `Encoded_Library` for audio and video tracks (`matroska.go:2403-2405`, `matroska_ac3_canonical.go:98-99`, and the same test in the other `matroska_*_canonical.go` files). Official only traces `CodecName` and fills no field (`File_Mk.cpp:1866`).

### Proof

T18 is AC-3 in Matroska with a `CodecName` element and a track `ENCODER` tag. The two values are different. Official Audio `Encoded_Library` is the tag value. Go Audio `Encoded_Library` is the `CodecName` value. Go thus prefers `CodecName` to the tag. On a track without a tag, Go adds a field that official does not have.

### Verdict

Bug.

Witness file (cluster 195): C634123.

## R9. Vorbis and Theora vendor strings: Go does not use the official split

### What goes wrong

Official splits the vendor string of a Vorbis comment header with one rule for all containers (`File_VorbisCom.cpp:110-175`):

1. It looks up the full string in a table of known Vorbis vendors (`MediaInfo_Config_Automatic.cpp:6673-6709`). The table gives the version and the date.
2. Without a table entry, it reads the name, the date and the version from the string. The name is the text before ` I `.
3. It changes the name to `libVorbis` or `libTheora` only when the string contains `libVorbis I` or `libTheora I`.

Go has two different splitters, and neither follows this rule:

- Matroska Vorbis (`matroska_vorbis.go:107-134`) has no table. For a table vendor, Go gives the date digits as the version. Official gives a release number.
- Ogg (`ogg.go:478-523`) sets the name from the stream format, not from the string. It sets a version only for two dates, and it formats the date from any word of 8 digits.

### Proof

| File | Field | Official | Go |
|---|---|---|---|
| T6, Ogg Vorbis, the vendor is a libavformat string | Audio `Encoded_Library_Name` | absent | `libVorbis` |
| T16, Ogg Theora, vendor `X I 20090822` | Video `Encoded_Library_Name` | `X` | `libTheora` |
| T16 | Video `Encoded_Library_Version` | `20090822` | absent |

A local unit test, not committed, called `splitMatroskaVorbisLibrary` and `splitOggLibrary` with table vendors from `MediaInfo_Config_Automatic.cpp`. For `Xiph.Org libVorbis I 20050304`, the table gives the version `1.1.1/1.1.2`. Go Matroska gives `20050304`, and Go Ogg gives no version. For an aoTuV beta 5 vendor, the table gives `Beta 5`. Go Matroska gives the short beta code with a trailing space.

T6 in Matroska has a vendor that is not in the table. Its JSON is the same in both programs.

### Verdict

Bug. G00508 is JSON and XML only, which agrees with a different version value.

Witness files (clusters 124, 156, 175 and 176): C007001, C007291, C639274, C000232, C609615.

## R10. Ogg text shows the library name and date lines

### What goes wrong

The Ogg parser fills `Encoded_Library_Name`, `Encoded_Library_Version` and `Encoded_Library_Date` in the shared field store (`ogg.go:478-493`). Go then prints them in text, with their raw names as labels. Official never shows these fields in text (rule 1).

### Proof

T6 (Ogg Vorbis), T7 and T16 (Ogg Theora and Vorbis): Go text has the lines `Encoded_Library_Name` and `Encoded_Library_Date` under Video and Audio. Official text has no such lines.

### Verdict

Bug. upbrr pastes the text output into tracker uploads, so these lines show in the uploads.

Witness files (clusters 156, 175 and 176): C000232, C609615.

## R11. Text `Writing library` does not use rule 2

### What goes wrong

Each Go parser adds a text field `Writing library` with the raw library string, for example `matroska.go:2388` for the Vorbis vendor and `mpeg_ps.go:1074` for MPEG-2. Go builds `Name Version (Date)` only in raw text, and only for some encoders (`field_project_raw_text.go:1806-1891`). Official text shows `Encoded_Library/String`, which follows rule 2 for every stream.

### Proof

| File | Official text `Writing library` | Go text `Writing library` |
|---|---|---|
| T6, Matroska Vorbis | `libVorbis <version> (<date>)` | the raw vendor string |
| T15, MPEG-PS TMPGEnc, Video | `TMPGEnc <version>` | the raw user data string |
| T16, Ogg Theora, Video | `X 20090822 (2009-08-22)` | the raw vendor string |

For T15, Go raw text has the official value, but Go text does not. For x264 and x265, Go text and official text are the same.

### Verdict

Bug. The triage sheet puts G00130 (`Writing library`, Audio, 131 files), G00245 (Video, 39 files) and G00363 (Audio, missing, 21 files) in its "deliberate" group. These patterns are not part of the listed deliberate difference, which is about `Encoded_Application`. This cause explains at least part of G00130 and G00245. The maintainer must confirm the new group for these patterns.

## R12. `EncodedBy` has no text label

### What goes wrong

Go reads the Matroska `ENCODED_BY` tag as `EncodedBy` (`matroska_tags.go:163-164`). The field schema has no entry and no text label for `EncodedBy` (`field_schema.go:96-98` holds the encoder entries). The text output then omits the General value and shows the track values with the raw label `EncodedBy`. Official uses the label `Encoded by` (`MediaInfo_Config_Automatic.cpp:408`, General options at `:4557`).

### Proof

T19 is Matroska with `ENCODED_BY` tags on General, Video and Audio. JSON is the same in both programs. Official text has three lines `Encoded by`. Go text has no General line and two lines `EncodedBy`.

### Verdict

Bug.

Witness files (clusters 150 and 191 to 194): C000111, C000203, C000242, C000253.

## R13. MP4 and MOV: no General library from the handler manufacturer

### What goes wrong

Official reads the component manufacturer of each `hdlr` box. When all handlers name the same manufacturer, it fills General `Encoded_Library` and `Encoded_Library_Name` from a small table. For example, `appl` gives `Apple QuickTime` (`File_Mpeg4_Elements.cpp:4254-4260`, `File_Mpeg4.cpp:165-176`, `:1452-1463`). A QuickTime version key in the `mdta` metadata adds the version (`File_Mpeg4_Elements.cpp:3773-3776`). Go does not read the handler manufacturer.

### Proof

T23 is a MOV file from `ffmpeg`. A script changed the manufacturer of the video handler to `appl`. Official General has `Encoded_Library` and `Encoded_Library_Name` equal to `Apple QuickTime`, and text has the line `Writing library`. Go has neither.

### Verdict

Bug.

Witness file (cluster 80): C608243.

## R14. Matroska Theora: Go does not read the vendor string

### What goes wrong

For a Theora track in Matroska, official runs the Ogg parser on the codec private data (`File_Mk.cpp:4811-4817`). It fills Video `Encoded_Library` from the vendor string. The Go Matroska parser has no Theora path, so it has no Video `Encoded_Library`.

### Proof

T17 is Matroska with Theora and Vorbis from `ffmpeg`. Official Video `Encoded_Library` is the libtheora vendor string. Go has no value.

### Verdict

Bug. No corpus pattern maps to this cause with certainty. The effect is small.

## R15. MP4 visual compressor name becomes `Encoded_Library`

### What goes wrong

Go reads the compressor name of the MP4 visual sample entry as `Writing library` and `Encoded_Library` (`mp4_codec.go:239-241`, `:449-457`, `:946-958`). v23.04 skips the compressor name (`File_Mpeg4_Elements.cpp:6090-6100`). MediaInfoLib v25.03 and later fill `Encoded_Library` from it (master `File_Mpeg4_Elements.cpp:6586-6601`, `:6643`). The line is absent in `v24.12` and present in `v25.03`.

In the corpus, most values of G00200 are a generic codec description and name no encoder.

### Proof

T1 is MP4 with H.264, no SEI, and a generic codec description as the compressor name. Official has no Video `Encoded_Library`. Go has the compressor name in JSON, text and raw text. T2 adds the x264 SEI. Then both programs show the x264 library, so the SEI has priority in Go.

T8 shows a side effect. In an MP4 file with MPEG-4 Visual, official shows the library from the user data of the stream. Go shows the compressor name instead (see R18).

### Verdict

Decision. Go follows v25.03 and later, and `AGENTS.md` does not record it. A generic codec description gives no information about the encoder, so the newer behavior is not clearly more correct.

Witness files (cluster 36): C608340, C608712, C610651.

## R16. Matroska General `Encoded_Library` is split into name and version

### What goes wrong

Go splits the Matroska `MuxingApp` into `Encoded_Library_Name` and `Encoded_Library_Version` for a list of known applications (`analyze.go:1051-1055`, `writing_app.go:14-56`). v23.04 does not split it (`File_Mk.cpp:3155-3165`). Master splits the General name and version of the library and of the application in one step (master `File__Analyze_Streams_Finish.cpp:3432-3531`, `:3547-3550`). `AGENTS.md` lists the split for `Encoded_Application` only.

The Go split has two more effects:

1. Go splits after it joins the General `ENCODER` tag. The version then contains ` / ` and the tag value.
2. Go builds the raw text `Encoded_Library/String` from the parts, so the `v` before the version is lost. Official raw text shows the full `Encoded_Library` (rule 2).

`writing_app.go:49-53` also holds a list of mkvmerge versions that Go does not split. The MediaInfoLib source has no such list.

### Proof

T11 is Matroska with a `MuxingApp` of the libmakemkv shape: the name, `v` and a version, a bracket list of library versions, and a platform tag. Official JSON has no General name or version. Go JSON has `Encoded_Library_Name` `libmakemkv` and a version that ends with ` / ` and the `ENCODER` tag. Official raw text has the `v`, and Go raw text does not. Text is the same in both programs. T12 has the mkvmerge library string, and its output is the same in both programs.

### Verdict

Decision. Go follows the newer MediaInfo for one group of applications. The maintainer can add the library to the deliberate line for `Encoded_Application`, or revert to v23.04. Effect 1 is a bug in both cases.

Witness files (clusters 80, 89 and 91): C000070, C531300, C531302, C608243, C001014.

## R17. Matroska `ENCODER_OPTIONS` becomes `Encoded_Library_Settings`

### What goes wrong

Go maps the Matroska tag `ENCODER_OPTIONS` to `Encoded_Library_Settings` (`matroska_tags.go:167`, `matroska.go:3733`). v23.04 keeps it as the raw tag `ENCODER_OPTIONS`, because `File_Mk.cpp:3341-3342` maps only `ENCODER_SETTINGS`. MediaInfoLib v26.05 and later map it (master `File_Mk.cpp:3454`). The line is absent in `v26.01` and present in `v26.05`.

### Proof

T5 is Opus in Matroska with a track tag `ENCODER_OPTIONS` that holds an opusenc option string. Official JSON has `extra/ENCODER_OPTIONS`, and text has a line `ENCODER_OPTIONS`. Go JSON has `Encoded_Library_Settings`, and text has a line `Encoding settings`.

### Verdict

Decision. Go follows v26.05 and later, and `AGENTS.md` does not record it. The newer mapping puts the opusenc options in the same field as other encoder settings, and upbrr reads that field. G00304, G00303 and G00306 move together.

Witness files (cluster 71): C000242, C007382, C530452.

## R18. MP4 does not parse the MPEG-4 Visual stream

### What goes wrong

Official reads the encoder string from the MPEG-4 Visual user data. It sets the name, the version and the date from the DivX and XviD tables (`File_Mpeg4v.cpp:534-539`, `:1319-1393`). The Go MP4 path does not parse the MPEG-4 Visual stream. Open ticket #98 names this root cause for the VOL header.

### Proof

T8 is MP4 with XviD. Official has Video `Encoded_Library` from the user data, with the name `XviD` and the version. Go has the compressor name (R15) and no name or version.

### Verdict

Existing ticket #98. Its decisions do not name the library fields yet. The fix for #98 can add them.

Witness files (cluster 156): C609727, C609771.

## Patterns not fully explained

- G00547: `Writing Library` with a capital `L`, General, missing in Go, 8 files in MP4, MPEG-PS and WebM. MediaInfoLib has no field and no label with this name. Official text uses `Writing library`. The pattern can be a tag name from the file, or the General library from R4 and R13. A witness file must settle it.
- The second pattern of cluster 129: `Writing Library`, new in Go, 1 file. Go maps a Matroska tag `WRITING_LIBRARY` to this name (`matroska_tags.go:343-344`). The research made no test file for it.
- G00591: Video `Encoded_Library_Version`, new in Go, 5 files, all outputs. The candidates are R1 (a General `ENCODER` tag that starts with `x264`, on a track without its own tag) and R15 (a compressor name that starts with `x264`). The research did not reproduce it.
- The new property and the different value of cluster 101 (Video `Encoded_Library_Name`) have no G-id in the sheet. R1 and R15 are candidates for the new name.
- The split of G00492 (10 files) between R3, R4 and R18 is not known.

## Questions that need a witness file

1. C000200, C001029 and C608795: where does the General field `Writing Library` come from? Is it a tag in the file, or the official `Writing library` line?
2. C001029, C608715 and C632268: which encoder string does the video stream carry? Is it an ATEME SEI, MPEG-2 user data, or an SEI under the x264 UUID with another name?
3. C009476, C010392 and C608715: does the file have a Matroska `ENCODER_SETTINGS` tag without a target? R1 expects one.
4. One file of G00591: what are the General and video `ENCODER` tags, and what is the MP4 compressor name?
5. C007001, C007291 and C639274: is the Vorbis vendor string one of the entries of the official table?

## Side findings

- Go text omits the General `Encoding settings` line when JSON has it (T20 and T21). Official text shows it.
- In Ogg, the General `Encoded_Application` comes from the first stream in Go and from the last stream in official (T7 and T16). Official fills it from the `ENCODER` comment of each stream (`File_VorbisCom.cpp:252`).
- Go does not read the QuickTime `©swr` user data atom, so a MOV file has no General `Encoded_Application` (T23). The triage sheet puts G00488 (`Writing application`, General, missing, 11 files) in its "deliberate" group. This cause is not part of the deliberate difference.
- In Matroska Vorbis, official text shows `Writing application` before `Writing library`. Go shows them in the other order (T6). This is part of the order group of the sheet.
- Cluster 59 is outside this ticket. G00322 adds `'` before an encoding settings value that starts with `-`. This is the CSV formula guard (`output_safety.go:82-93`), and the scope is CSV. ADR 0001 rule 3 thus puts it out of scope, as the title research did for its cause R8. G00432 is MP3 in AVI. Official builds the LAME settings string from the LAME header, and Go does not. The research did not test it.

## Synthetic files

All video is the `testsrc2` pattern, 2 s at 25 fps. All audio is a 440 Hz sine.

| File | Container and streams | Change |
|---|---|---|
| T1 | MP4, H.264 | SEI removed, generic compressor name |
| T2 | MP4, H.264 | x264 SEI kept, generic compressor name |
| T3 | WebM, VP9 and Opus | none |
| T4 | Matroska, H.264 and AAC | none |
| T5 | Matroska, Opus | track tag `ENCODER_OPTIONS` |
| T6 | Ogg and Matroska, Vorbis | none |
| T7 | Ogg, Theora and Vorbis | none |
| T8 | MP4, XviD, and MPEG-4 Visual from libavcodec | none |
| T9 | MPEG-2 elementary stream, MPEG-PS and MP4 | user data string added |
| T10 | Matroska, HEVC | x265 info SEI off, ATEME UUID SEI added |
| T11 | Matroska, H.264 and AAC | `MuxingApp` of the libmakemkv shape |
| T12 | Matroska, H.264 and AAC | `MuxingApp` of the mkvmerge library shape |
| T13 | Matroska, H.264 and AAC | General `ENCODER` tag `Example 1.23` |
| T14 | Matroska, H.264 | track tags `ENCODER` and `ENCODER_SETTINGS` |
| T15 | MPEG-PS, MPEG-2 | TMPGEnc user data string |
| T16 | Ogg, Theora and Vorbis | Theora vendor `X I 20090822`, page CRC fixed |
| T17 | Matroska, Theora and Vorbis | none |
| T18 | Matroska, AC-3 | `CodecName` element added |
| T19 | Matroska, H.264 and AAC | `ENCODED_BY` tags on General, Video and Audio |
| T20 | Matroska, H.264 | General tags `ENCODER` (x264 shape) and `ENCODER_SETTINGS` |
| T21 | Matroska, H.264 | as T20, SEI removed |
| T22 | Matroska and MP4, H.264 | SEI removed, x264 UUID SEI with `ExampleAVC 2.1` added |
| T23 | MOV, MPEG-4 Visual | manufacturer of the video handler set to `appl` |
