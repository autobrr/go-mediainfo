# Title and movie name fields: root causes (issue #73)

Research date: 2026-10-18. Map issue: #65. Corpus run: 2026-10-17.

This file groups the title patterns of the corpus run into root causes. The ticket names five General patterns. This file also covers the track `Title` patterns and the General `Track name`, `Collection` and `Album` patterns, because the same code produces them.

Each cause has one of these verdicts:

- Bug. go-mediainfo is wrong and must change.
- Decision. go-mediainfo follows a newer MediaInfo, or go-mediainfo can be more correct than v23.04. `AGENTS.md` does not record it. ADR 0001 rule 2 requires a recorded reason, or a revert to v23.04.
- Out of scope. ADR 0001 rule 3 or an unsupported container excludes the pattern.

The file refers to corpus files by C-id and to patterns by G-id. It names no release, path, title or tag value. It describes a corpus title only by its shape. All titles in the synthetic files are made up for this research.

## Sources and method

- go-mediainfo: `origin/main` at `6a37300`. Citations are `internal/mediainfo/<file>:<line>` at that commit.
- MediaInfoLib: tag `v23.04`, read from the GitHub API for `MediaArea/MediaInfoLib`. Citations are `Source/MediaInfo/<file>:<line>` at that tag, unless the text says "master". "Master" is the local checkout at `~/github/oss/MediaInfoLib`, which is newer than v26.10. The version tests on the tags `v23.04` to `v26.10` used the same API.
- Corpus data: the HTML archive of the 2026-10-17 run. Local scripts counted the outputs, containers and value shapes for each pattern. The scripts print no values.
- Synthetic files: T1 to T12, made with `ffmpeg` on the local machine. The section "Synthetic files" lists them.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The Go binary was a local `linux/amd64` build of `6a37300` at `/tmp/go-mediainfo-research-73`. The research copied only the synthetic files to the host and read no library file. The research removed its files from the host after the runs.
- Host commands: `mediainfo --ParseSpeed=0.5 <f>`, `mediainfo --ParseSpeed=0.5 --Language=raw <f>`, `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5 <f>`, and the same three outputs from the Go binary. Each command ran under `ionice -c3 nice -n10 timeout 300`.

In this file, "text" is the default text output. "Raw text" is the text output with `--Language=raw`. CSV and HTML use the text labels, so they follow text.

## How official v23.04 fills the title fields

Three rules in MediaInfoLib v23.04 explain most patterns. The causes below refer to them.

1. A parser fills General `Title`. After the parse, `File__Analyze::Tags()` copies `Title` to `Movie` when the file has a video stream and no `Collection`. In all other cases it copies `Title` to `Track` (`File__Analyze_Streams.cpp:2654-2660`). It also fills an empty `Title` from `Movie` or `Track` (`File__Analyze_Streams.cpp:2676-2679`).
2. In v23.04, General `Title` has the options `N NTY`. `Movie` and `Track` have `Y YTY` (`MediaInfo_Config_Automatic.cpp:4480`, `:4488`, `:4507`). The first character controls text and raw text. The fifth character controls JSON and XML (`MediaInfo_Inform.cpp:691-702`). Thus text shows `Movie name` or `Track name`, raw text shows `Movie` or `Track`, and both hide `Title`. JSON shows `Title` and `Movie`, or `Title` and `Track`. The labels come from `MediaInfo_Config_Automatic.cpp:1032` and `:1314`.
3. When a parser fills a field that already has a value, v23.04 appends ` / ` and the new value (`File__Analyze_Streams.cpp:1046-1050`). v23.04 does not compare the two values, and it does not trim spaces.

v26.10 changed rules 2 and 3. General `Title` is `Y YTY` and `Movie` is `N YTY` (master `Source/Resource/Text/Stream/General.csv:176`, `:184`), so v26.10 text shows `Title`. From v25.10, `Fill` trims spaces from the `Title` of each stream kind. It also drops a value that is equal to, or a prefix of, the value that is already there (master `File__Analyze_Streams.cpp:1628-1720`). The trim code is absent in `v25.04` and present in `v25.10`.

## Summary

| Cause | Verdict | Patterns | Files (about) |
|---|---|---|---|
| R1. No shared step picks `Movie` or `Track`, and text shows `Title` | Bug | G00035 and G00036 (text part), G00042 (JSON part), G00628 (MP4 part), G00639 | 25 |
| R2. Raw text shows `Title` and `Movie name` instead of `Movie` | Bug | G00035, G00036, G00042 (raw text part) | 465 |
| R3. Go keeps one value where v23.04 joins two equal values | Decision | G00887, G00899, G00546 (MP4 part), G00900 | 11 |
| R4. Go trims spaces from titles | Decision | G00383, G00389, G00627, and the value of 18 files in G00036 and G00042 | 22 |
| R5. MP4 handler names become track titles | Bug | G00181, G00197 | 71 |
| R6. Standalone FLAC tags reach JSON but not text | Bug | G00035 (FLAC part), G00628 (FLAC part), G00901, G01371, G01372, G01490, G01491 | 1 |
| R7. AVI track name with an undefined Windows-1252 byte | Decision | G00546 (AVI part) | 1 |
| R8. CSV formula guard and Windows Media | Out of scope | G01487, G00036 (Windows Media part), G01456, G01488, G01489 | 2 |

The causes explain every pattern of this family in the corpus.

### Pattern outputs

The ticket lists the outputs for each pattern as a union over all files. The split by file is different.

| Pattern | Matroska and WebM (444 files) | MP4 | Ogg, MPEG-TS | FLAC | Windows Media |
|---|---|---|---|---|---|
| G00035 `Title` new | raw text | text and raw text (21) | text and raw text (1 each) | raw text (1) | none |
| G00036 `Movie` missing | raw text | text and raw text (18) | text and raw text (1 each) | none | all outputs (1) |
| G00042 `Movie name` new | raw text | JSON and XML (3) | none | none | none |

For 444 Matroska and WebM files, JSON and text already match official. Only raw text differs.

## R1. No shared step picks `Movie` or `Track`, and text shows `Title`

### What goes wrong

Go has no step like `File__Analyze::Tags()`. Each parser sets the title fields in its own way:

- MP4 reads `©nam` into a text field with the label `Title` (`mp4_udta.go:65`, `analyze.go:248`). It always sets the structured `Title` and `Movie` (`analyze.go:250-252`).
- Ogg adds a text field `Title` and sets `Title` and `Movie` (`ogg.go:554-558`).
- MPEG-TS adds a text field `Title` from the XDS program name (`mpeg_ts.go:3074-3078`). It sets `Title` and `Movie` (`analyze.go:1218-1225`).
- Matroska hides the text field `Title` (`analyze.go:974-977`). It always sets `Movie` and adds a text field `Movie name` (`analyze.go:1058-1064`, `matroska_tags.go:434-449`).

These effects follow:

1. MP4, Ogg and MPEG-TS text shows `Title`. Official text shows `Movie name` (G00035 and G00036 text part, 23 files). upbrr pastes this text into tracker uploads.
2. Go never fills `Track`. A file without video, or an MP4 file with a TV show name, gets `Movie` in JSON. Official gives `Track` in JSON and `Track name` in text (G00042 JSON part, G00628 MP4 part). This affects MP4, Matroska audio, Ogg audio and the Ogg FLAC title that `AGENTS.md` keeps on purpose (issue #55).
3. Go does not read the MP4 `tvsh` atom. The `ilst` table has only `©nam`, `©alb` and `©cmt` (`mp4_udta.go:61-67`). Thus Go has no `Collection` (G00639), and it cannot apply the rule "video and no `Collection`".

### Proof

Host runs on synthetic files. The JSON columns show the `Title`, `Movie`, `Track` and `Collection` keys of General.

| File | Official text | Go text | Official JSON | Go JSON |
|---|---|---|---|---|
| T1, Matroska with video | `Movie name` | `Movie name` | `Title`, `Movie` | `Title`, `Movie` |
| T4, MP4 with video | `Movie name` | `Title` | `Title`, `Movie` | `Title`, `Movie` |
| T8, Ogg Theora and Vorbis | `Movie name` | `Title` | `Title`, `Movie` | `Title`, `Movie` |
| T5, MP4 with video and a TV show name | `Collection`, `Track name` | `Title` | `Title`, `Track`, `Collection` | `Title`, `Movie` |
| T6, MP4 audio only | `Track name` | `Title` | `Title`, `Track` | `Title`, `Movie` |
| T9, Matroska audio only | `Track name` | `Movie name` | `Title`, `Track` | `Title`, `Movie` |
| T11, Ogg Vorbis audio only | `Track name` | `Title` | `Title`, `Track` | `Title`, `Movie` |
| T12, Ogg FLAC | no title line | `Title` | no title keys | `Title`, `Movie` |

The corpus agrees. On C531699, C531701 and C531702 (MP4 with a TV show name), official has `Collection` and `Track`, and Go has `Movie`. On the 18 MP4 files of G00036, the Go `Title` text value is equal to the official `Movie name` value.

### Verdict

Bug. One shared step after the parse can replace the per-parser code. The step fills `Movie` or `Track` from `Title` with the v23.04 rule. It shows the `Movie name` or `Track name` label in text, and it hides `Title` in text. The MP4 parser must also read `tvsh` into `Collection`.

Witness files: C000220, C608266 (MP4 with video), C531699 (MP4 with a TV show name), C000232 (Ogg), C000234 (MPEG-TS).

## R2. Raw text shows `Title` and `Movie name` instead of `Movie`

### What goes wrong

Raw text gets its General title lines from two places:

1. The structured derivations add `Title` for every stream kind, General too (`field_project_raw_text.go:860-862`).
2. The Matroska text field `Movie name` has no raw rule, so the projection uses the friendly label as the raw name (`field_project_raw_text.go:716-717`, `field_project_raw_text.go:1334-1339`). The alias table has no entry for `Movie name` or `Track name` (`field_project_raw_text.go:45`).

Official raw text shows only `Movie` or `Track`, because v23.04 hides General `Title` (rule 2).

### Proof

Host runs. For T1, official raw text has one line, `Movie`. Go raw text has two lines, `Title` and `Movie name`. For T4, T5, T6 and T8, Go raw text has `Title` and no `Movie` or `Track`.

The corpus agrees. For 418 of the 444 Matroska and WebM files, the Go raw `Title` and `Movie name` values are equal to the official raw `Movie` value. R3 and R4 explain the other 20 files.

### Verdict

Bug. It shows only in raw text. ADR 0001 counts raw text as a text output, but upbrr reads the default text and JSON. Thus this fix has a lower priority than R1. A fix of R1 alone does not fix raw text, because the derivation at `field_project_raw_text.go:860` adds `Title` again.

Witness files: C000115, C000203, C008507, C275376, C627345, C629489.

## R3. Go keeps one value where v23.04 joins two equal values

### What goes wrong

When a file has two equal title sources, v23.04 shows both, joined by ` / ` (rule 3). Go shows one value.

- Matroska General. The Segment Info `Title` fills `Title` (`File_Mk.cpp:3197-3206`). A file-level `TITLE` tag fills it again. `File_Mk::Streams_Finish` tries to skip an equal tag. But it compares the tag with the `Title` of `StreamKind_Last`, which is the last track, not General (`File_Mk.cpp:785`). Master has the same line (master `File_Mk.cpp:843`). Thus v23.04 shows the value twice. Go merges the two sources. It keeps one value when they are equal or when one is a prefix of the other (`matroska_tags.go:434-440`, `matroska_tags.go:465-478`).
- MP4 tracks. The `hdlr` name fills the track `Title` (`File_Mpeg4_Elements.cpp:4137`). The track `udta/name` fills it again. Go uses the `udta/name` value, or the handler name when no `udta/name` exists, so it has one value (`analyze.go:340`).

From v25.10, official keeps one value too (master `File__Analyze_Streams.cpp:1709-1720`).

### Proof

- T3, Matroska with a segment title and an equal file-level `TITLE` tag. Official text, raw text and JSON show the title twice, joined by ` / `. Go shows it once.
- T10, MP4 with a handler name and an equal `udta/name`. Official audio `Title` shows the value twice, joined by ` / `. Go shows it once.
- Corpus: in all 11 files the official value has two equal parts joined by ` / `, and the Go value is one part.

### Verdict

Decision. Go matches v25.10 and later, and one value is more correct. The maintainer can keep it and add a line to the deliberate differences in `AGENTS.md`, or revert to the v23.04 join.

Witness files: C000115, C000203 (Matroska General), C608228 (MP4 audio), C000221, C000222 (MP4 text).

## R4. Go trims spaces from titles

### What goes wrong

Go trims leading and trailing spaces from these values:

- the Matroska segment title (`matroska.go:1558-1561`)
- the Matroska track name (`matroska.go:1780-1781`)
- the MP4 track `udta/name` (`mp4_udta.go:93`) and the MP4 handler name (`mp4_meta.go:112`)
- the AVI `strn` name (`avi.go:1686`)

v23.04 keeps the spaces. From v25.10, official trims too (master `File__Analyze_Streams.cpp:1698-1708`).

Go does not trim the MP4 `©nam` value (`mp4_udta.go:79`). That value keeps the v23.04 behavior, so Go is not consistent.

### Proof

- T2, Matroska with a segment title and an audio track name that each end with a space. Official text, raw text and JSON keep the space. Go removes it.
- Corpus: G00383 and G00389 have 18 Matroska files with a whitespace-only difference. In the same 18 files, the official raw `Movie` value is the Go value plus a trailing space (G00036, G00042). G00627 has 4 files with an audio `Title` that ends with a space.

### Verdict

Decision. Go matches v25.10 and later. A trailing space carries no meaning for a reader, but it is in the file. The maintainer can keep the trim and add a line to `AGENTS.md`, or keep the spaces as v23.04 does. If Go keeps the trim, the MP4 `©nam` value needs it too.

Witness files: C000108, C000165 (Matroska General), C000303 (Matroska audio), C531699 (MP4 audio).

## R5. MP4 handler names become track titles

### What goes wrong

Official v23.04 clears a handler name that contains `Handler` or ` handler`. It also clears a name that starts with `Gestionnaire `, `Module `, `Gestor ` or `Procedura ` (`File_Mpeg4_Elements.cpp:4132-4133`). Master has the same test (master `File_Mpeg4_Elements.cpp:4539-4540`). Go drops only four exact names: `SoundHandler`, `VideoHandler`, `MetaHandler` and `SubtitleHandler` (`analyze.go:338-344`). All other handler names become the track `Title` in every output.

Official also fills the handler name only when General `Title` is still empty at that point of the parse (`File_Mpeg4_Elements.cpp:4137`).

### Proof

- T4, MP4 with handler names that contain `Media Handler`. Official shows no Video or Audio `Title`. Go shows the handler name as `Title` for both tracks, in text, raw text and JSON.
- Corpus: all 131 Go values in G00181 (71 video tracks) and G00197 (60 audio tracks) match the official name test. Each value is a handler name that names a muxing tool.

### Verdict

Bug.

Witness files: C531699, C606728, C608249, C608283.

## R6. Standalone FLAC tags reach JSON but not text

### What goes wrong

The FLAC parser writes the Vorbis comment fields with `SetSame`, which makes a structured fact with no text label (`flac.go:712-717`). `TITLE` becomes `Title` and `Track` (`flac.go:758-762`). JSON matches official. Text shows no `Album`, `Album/Performer`, `Track name`, `Track name/Position` or `Track name/Total` line. Raw text shows `Title` from the generic derivation (R2), and no `Track` or `Album`.

The colour gap in #93 has the same mechanism, but #93 covers only colour and HDR fields.

### Proof

T7, FLAC with `TITLE`, `ALBUM` and `TRACKNUMBER`. Official text shows `Album`, `Track name` and `Track name/Position`. Go text shows none of them. Both JSON outputs have `Title`, `Track`, `Album` and `Track_Position`.

### Verdict

Bug. The corpus has one witness, but each tagged FLAC file has this problem in text output.

Witness file: C226345.

## R7. AVI track name with an undefined Windows-1252 byte

### What goes wrong

Go decodes the AVI `strn` name as Windows-1252. It maps the five undefined bytes (0x81, 0x8D, 0x8F, 0x90, 0x9D) to U+FFFD (`avi.go:1669-1687`). On C000172 the name is not Windows-1252 text. Go shows one U+FFFD where the official value has no character. The other characters are equal.

The reference run probably used the Windows build of MediaInfo, which decodes the bytes with the system code page. This research did not test the official decode.

### Verdict

Decision, low priority. U+FFFD tells the reader that a byte was not decodable. Dropping the byte matches the reference.

Witness file: C000172.

## R8. Out of scope

- G01487 (C279810, CSV only). The Go CSV writer puts `'` before a value that starts with a formula character, to stop formula injection in spreadsheets (`output_safety.go:83-88`). Official does not. ADR 0001 rule 3 excludes a CSV-only difference. `AGENTS.md` does not list this guard as a deliberate difference.
- G00036 Windows Media part, G01456, G01488 and G01489 (C609152). Go cannot parse Windows Media. The map issue keeps unsupported containers out of the tickets.

## Proposed bug tickets

1. Pick `Movie` or `Track` with one shared rule, and show its label in text (R1). Witnesses: C000220, C608266, C531699, C000232, C000234. Decisions before the fix:
   - Must one step after the parse fill `Movie` when the file has video and no `Collection`, and `Track` in all other cases, as v23.04 does?
   - Must text show `Movie name` or `Track name` and hide `Title`, as v23.04 does, or show `Title`, as v26.10 does? Matroska text already follows v23.04.
   - Must the Ogg FLAC title that `AGENTS.md` keeps (issue #55) become `Track` with the `Track name` label, as standalone FLAC does in official?
2. Raw text: show `Movie` or `Track` for General, and drop `Title` (R2). Witnesses: C000115, C008507, C275376. Decisions before the fix:
   - Must the raw General stream skip the generic `Title` derivation when text hides `Title`?
   - Must each display-only General field get its raw name from a table, so that no friendly label such as `Movie name` reaches raw text?
3. MP4: drop handler names with the official name test (R5). Witnesses: C531699, C606728, C608249. Decisions before the fix:
   - Must Go drop each handler name that contains `Handler` or ` handler`, or starts with `Gestionnaire `, `Module `, `Gestor ` or `Procedura `?
   - Official keeps a handler name only when General `Title` is still empty when it reads the `hdlr` box, so the result depends on the box order. Must Go copy this condition?
4. FLAC: show the Vorbis comment fields in text (R6). Witness: C226345. Decisions before the fix:
   - Which text labels must the FLAC facts get? Official uses `Album`, `Album/Performer`, `Track name`, `Track name/Position` and `Track name/Total`.
   - Must this fix use the same shared step as ticket 1, so that FLAC, Ogg and MP4 get the title labels from one place?

R3, R4 and R7 need a maintainer decision, not a bug ticket. #83 is the place for R3 and R4, because Go matches v25.10 and later there.

## Questions for the contributor (witness files)

1. C000115 and C000203. Does each file have a segment title and a file-level `TITLE` tag with the same value? Does the last track have no track name? R3 expects both.
2. C531699, C531701 and C531702. Does the `ilst` box have a `tvsh` atom? R1 expects that it holds the official `Collection` value.
3. C000172. Which byte is in the `strn` chunk where Go shows U+FFFD? Did the reference run use the Windows build of MediaInfo?
4. C000234. Does the title come from the XDS program name in the EIA-608 captions? No synthetic file reproduced this path.

## Side findings

- MediaInfoLib bug: `File_Mk::Streams_Finish` compares a file-level `TITLE` tag with the title of the last track, not with General `Title` (`File_Mk.cpp:785`, master `File_Mk.cpp:843`). The `Fill` change in v25.10 hides the effect. The bug is upstream, and Go needs no change for it.
- The Go CSV formula guard (R8) is a deliberate safety choice, but `AGENTS.md` does not list it.
- R1 needs `tvsh` support in MP4. #106 covers only `mdta` keys, so no open ticket covers `tvsh`.

## Synthetic files

Each file is 1 second of `testsrc` video, a 440 Hz sine, or both, made with `ffmpeg`. All titles are made up. The local scratch scripts are not in the repository.

| ID | Content |
|---|---|
| T1 | Matroska, AVC and AAC, segment title |
| T2 | T1 with a segment title and an audio track name that each end with a space |
| T3 | T1 with an equal file-level `TITLE` tag (written as `title-eng`, which gives a `TITLE` tag with a language) |
| T4 | MP4, AVC and AAC, `©nam`, video and audio handler names that contain `Media Handler` |
| T5 | MP4, AVC and AAC, `©nam` and a TV show name (`tvsh`) |
| T6 | MP4 audio only (AAC), `©nam` |
| T7 | FLAC with `TITLE`, `ALBUM` and `TRACKNUMBER` |
| T8 | Ogg, Theora and Vorbis, `TITLE` |
| T9 | Matroska audio only (Opus), segment title |
| T10 | MP4, AVC and AAC, audio handler name equal to the audio `udta/name` |
| T11 | Ogg Vorbis audio only, `TITLE` |
| T12 | Ogg FLAC, `TITLE` |
