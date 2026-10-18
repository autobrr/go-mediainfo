# Service kind fields: root causes (issue #75)

Research date: 2026-10-18. Map issue: #65. Corpus run: 2026-10-17.

This file groups the `ServiceKind` patterns of the corpus run into root causes. The ticket names four patterns: G00060, G00105, G00109 and G00157.

Each cause has one of these verdicts:

- Bug. go-mediainfo is wrong and must change.
- Decision. go-mediainfo follows a newer MediaInfo, or go-mediainfo can be more correct than v23.04. `AGENTS.md` does not record it. ADR 0001 rule 2 requires a recorded reason, or a revert to v23.04.
- Out of scope. ADR 0001 rule 3 or an unsupported container excludes the pattern.

The file refers to corpus files by C-id and to patterns by G-id. It names no release, path or tag value.

## Sources and method

- go-mediainfo: `origin/main` at `6a37300`. Citations are `internal/mediainfo/<file>:<line>` at that commit.
- MediaInfoLib: the tags `v23.04` to `v26.10`, read from the GitHub API for `MediaArea/MediaInfoLib`. Citations are `Source/MediaInfo/<file>:<line>` or `Source/Resource/<file>:<line>` at the tag that the text names.
- Corpus data: the HTML archive of the 2026-10-17 run. A local script read the per-file pages. It counted the value pairs of each pattern by output and by track kind. The values are short service codes and their names, so the counts name no release.
- Synthetic file S1, made with `ffmpeg` on the local machine. It is a Matroska file with AVC video, AC-3 audio, AAC audio and one SubRip text track. The Matroska muxer writes the track flags from the stream dispositions:
  - Video: `FlagOriginal`.
  - AC-3: `FlagOriginal` and `FlagCommentary`. The AC-3 `bsmod` is 0 (Complete Main).
  - AAC: `FlagVisualImpaired`.
  - Text: `FlagHearingImpaired` and `FlagOriginal`.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The Go binary was a local `linux/amd64` build of `6a37300` at `/tmp/go-mediainfo-research-75`. The runs used S1 and two parity samples: `fresh-09` (Matroska) and `dvd-01` (DVD IFO). The research removed its files from the host after the runs.
- Host commands: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5 <f>`, `mediainfo --ParseSpeed=0.5 <f>`, `mediainfo --ParseSpeed=0.5 --Language=raw <f>`, and the same outputs from the Go binary. Each command ran under `ionice -c3 nice -n10 timeout 300`.

In this file, "text" is the default text output. "Raw text" is the text output with `--Language=raw`.

## How official MediaInfo fills ServiceKind

1. The AC-3 and E-AC-3 parser always fills `ServiceKind` from `bsmod` with a short code. It fills `ServiceKind/String` with the name (`Audio/File_Ac3.cpp:1198-1199` and `:1304-1305` at v23.04, tables at `:41-64`). `bsmod` 0 gives `CM` and `Complete Main`.
2. v23.04 does not read the Matroska track flags `FlagHearingImpaired`, `FlagVisualImpaired`, `FlagTextDescriptions`, `FlagOriginal` and `FlagCommentary`. `Multiple/File_Mk.cpp` at v23.04 has no reference to these elements and no `ServiceKind` fill. The tags from v23.06 to v26.01 are the same. The flags first appear at v26.05.
3. From v26.05, `File_Mk::Streams_Finish` first merges the stream parser, for example AC-3 (`Multiple/File_Mk.cpp:1347` at v26.10). Then it fills one code and one name for each flag that is set, in this order: `HI`, `VI`, `TD`, `O`, `C` (`Multiple/File_Mk.cpp:130-160`, `:1453-1460`, and the flag readers at `:4220-4282`, all at v26.10).
4. When a field has a value, `Fill` appends ` / ` and the new value (`File__Analyze_Streams.cpp:1046-1050` at v23.04, `:1714-1719` at v26.10). At v26.10 the duplicate check applies only to `Title` and to some General fields (`File__Analyze_Streams.cpp:1628-1712`). It does not apply to `ServiceKind`. Thus v26.10 gives `CM / O` for an AC-3 track with `bsmod` 0 and `FlagOriginal`.
5. `ServiceKind` is hidden in text. `ServiceKind/String` shows in text as `Service kind`. This is the same for Audio, Video and Text at v23.04 and at v26.10 (`Resource/Text/Stream/Audio.csv:273-274`, `Video.csv:332-333` at v23.04 and `:341-342` at v26.10, `Text.csv:276-277`).

## Summary

| Cause | Verdict | Patterns | Files |
|---|---|---|---|
| K1. Go reads the Matroska track flags, as v26.05 does | Decision | G00060, G00109, G00105, G00157 (Matroska part) | 321, 172, 180, 73 |
| K2. Go follows the v26.05 rule in JSON and raw text only | Bug, and its fix depends on K1 | No reference pattern. The cross-output check shows it on the 180 files of G00105. | 180 |
| K3. The DVD IFO report copies the AC-3 `bsmod` from the VOB | Bug, covered by #116 and #111 | G00157 (DVD part) | 14 |

The causes explain every file of the four patterns.

### Value pairs in the corpus

The counts are files. One file can have more than one value pair. Official is in the left value column.

| Pattern | Track | Official v23.04 | Go | Files |
|---|---|---|---|---|
| G00060 | Text | absent | `O` | 185 |
| G00060 | Text | absent | `HI / O` | 162 |
| G00060 | Text | absent | `HI` | 136 |
| G00060 | Text | absent | `C`, `HI / C`, `O / C`, `HI / O / C` | 15 |
| G00109 | Video | absent | `O` | 172 |
| G00105 | Audio | `CM` | `CM / O` | 168 |
| G00105 | Audio | `CM` | `CM / C` | 12 |
| G00105 | Audio | `CM` | `CM / O / C` | 6 |
| G00157 | Audio, Matroska | absent | `O` | 70 |
| G00157 | Audio, Matroska | absent | `C`, `O / C` | 6 |
| G00157 | Audio, DVD Video | absent | `CM` | 14 |

For Matroska, the pairs are in JSON, XML and raw text (`ServiceKind/String`). They are not in text. For DVD Video, the pairs are in all outputs, and text has `Service kind: Complete Main`.

The Matroska tracks of G00157 are not AC-3. Official always fills `ServiceKind` for AC-3, so an AC-3 track with a flag goes to G00105. The archive lists MLP FBA, DTS, AAC, FLAC and Opus streams for the G00157 files.

## K1. Go reads the Matroska track flags, as v26.05 does

### What goes wrong

The Matroska parser reads `FlagHearingImpaired`, `FlagOriginal` and `FlagCommentary` (`matroska.go:84-88`, `:1737-1739`, `:1824-1838`). It joins the codes `HI`, `O` and `C` with ` / ` into `ServiceKind` (`matroska.go:2504-2516`). Each Matroska stream builder writes this value as a structured field. Examples are `matroska_video_canonical.go:175`, `matroska_text_canonical.go:54`, `matroska_audio_canonical.go:122` and `:242`, and `matroska_ac3_canonical.go:108`. The AAC, FLAC, DTS, TrueHD, Opus, Vorbis and MPEG Audio builders do the same. For AC-3, Go puts the `bsmod` code first and then the flag codes (`matroska_ac3_canonical.go:226-231`). This is the v26.05 order (rule 3).

v23.04 reads no flags (rule 2). This gives three effects:

- Video and Text tracks with a flag get a new `ServiceKind` (G00109, G00060).
- Audio tracks that are not AC-3 get a new `ServiceKind` (G00157, Matroska part).
- AC-3 and E-AC-3 tracks get `CM / O` or `CM / C` where v23.04 has `CM` (G00105).

`TestParseMatroskaTrackAccessibilityFlags` (`matroska_test.go:160`) and `TestMatroskaFLACTrackSeedsDirectCanonicalFields` (`matroska_canonical_test.go:832`) pin this behavior. The code came in PR #19.

### Proof

Host runs on S1. The table shows JSON `ServiceKind` for each track.

| Track | Official v23.04 | Go |
|---|---|---|
| Video, `FlagOriginal` | absent | `O` |
| AC-3, `bsmod` 0, `FlagOriginal`, `FlagCommentary` | `CM` | `CM / O / C` |
| AAC, `FlagVisualImpaired` | absent | absent |
| Text, `FlagHearingImpaired`, `FlagOriginal` | absent | `HI / O` |

Sample `fresh-09` on the host: official has `CM` on both AC-3 tracks and no `ServiceKind` on Text. Go has `CM / O` and `CM / O / C` on the AC-3 tracks, and `HI` on three Text tracks.

By rules 3 and 4, the v26.10 source gives the same values as Go for the Video, AC-3 and Text tracks of S1. The triage sheet records that v26.10 also emits `ServiceKind` on the Video and Text tracks of G00109 and G00060. It has no v26.10 data for the Audio patterns.

### Verdict

Decision. Go matches v26.05 and later, not v23.04. `AGENTS.md` does not list it. The file declares the flags, and v23.04 ignores them, so the v26.05 value carries more of the file's data. This cause belongs to the review of fields where Go matches v26.10 (#83).

If the maintainer keeps the flags, K2 is the work that follows. If the maintainer reverts to v23.04, Go stops reading the flags, and K2 goes away.

Witness files from the ticket: C000145, C000129, C633818, C005836, C531404. Witness files from the triage sheet: C010392, C629463 and C629489 for G00060. C003255, C007264 and C629708 for G00105 and G00157. C003422 and C531418 for G00109. All are Matroska.

## K2. Go follows the v26.05 rule in JSON and raw text only

### What goes wrong

Go writes the flag codes as structured values only (`builder.Structured`, see K1). The text line `Service kind` does not use them:

- On an AC-3 or E-AC-3 track, the text line comes from `bsmod` alone (`matroska_ac3_canonical.go:223-225`). JSON has `CM / O`, and text has `Complete Main`.
- On a Video track, a Text track or another Audio track, Go writes no text line. JSON has `O` or `HI`.
- Raw text expands the structured value into `ServiceKind/String` (`field_project_raw_text.go:181-184`, `:253`, `:1152-1164`, `:1478-1480`). Thus raw text follows JSON, and text does not.

v26.05 shows the names in text (rule 5). Thus for one track, Go text follows v23.04 and Go JSON follows v26.05.

Go also reads only three of the five flags. It skips `FlagVisualImpaired` (`0x55AC`) and `FlagTextDescriptions` (`0x55AD`). v26.05 reads them (`Multiple/File_Mk.cpp:1962-1963` and `:4232-4256` at v26.10). The raw text name table has no entry for `TD` (`field_project_raw_text.go:181-184`).

### Proof

Host runs on S1 with Go:

- Text has one `Service kind` line, `Complete Main`, on the AC-3 track. JSON for that track is `CM / O / C`.
- Raw text has the `ServiceKind/String` lines `Original` (Video), `Complete Main / Original / Commentary` (AC-3) and `Hearing Impaired / Original` (Text).
- The AAC track with `FlagVisualImpaired` has no `ServiceKind`. By rules 3 and 5, v26.05 gives `VI` and `Service kind: Visually Impaired`.

Sample `fresh-09`: Go text has `Service kind: Complete Main` on both AC-3 tracks. Go JSON has `CM / O` and `CM / O / C`.

The reference patterns do not show K2, because Go text agrees with v23.04 by accident. The cross-output check of the run shows it. The archive keeps this check in its `formats` set, which has its own G-id numbers. Its page for `/ServiceKind` lists 180 Matroska files where JSON and text disagree. These are the files of G00105.

### Verdict

Bug. Today the JSON and the text of one track disagree. upbrr reads the JSON and pastes the text, so the tracker upload and the JSON that upbrr reads give different service kinds. The fix depends on K1:

- If K1 keeps the flags, the text line must show all names, and Go must read the two flags that it skips.
- If K1 reverts, the K1 fix removes the flag codes from JSON and raw text, and no work is left for K2.

Witness files: the G00105 files, for example C000145, C000129, C003255 and C007264.

## K3. The DVD IFO report copies the AC-3 bsmod from the VOB

### What goes wrong

For an IFO in VIDEO_TS, Go parses the VOB files of the title set and builds the tracks from the payload streams (`dvd.go:242-258`). The MPEG-PS parser sets `ServiceKind` from the AC-3 `bsmod` (`mpeg_ps.go:1463-1465`). The text label `Service kind` comes from `mpeg_ps_canonical.go:261-262`. The IFO report keeps this value.

The v23.04 IFO parser reads only the IFO attribute table. It does not fill `ServiceKind` (`Multiple/File_Dvdv.cpp:660-683` at v23.04, and the file has no `ServiceKind` reference).

### Proof

Sample `dvd-01` (an IFO) on the host. Official JSON has no `ServiceKind` on the AC-3 track, and official text has no `Service kind` line. Go JSON has `CM`, and Go text has `Service kind: Complete Main`.

### Verdict

Bug, and existing tickets cover the cause. "DVD: decide whether to join the VOB files of a title set" (#116) covers the VOB parse for an IFO. "DVD IFO: stop copying VOB rates and the menu frame rate" (#111) covers VOB values in the IFO report. Their witness lists already include C000006, C000008 and C000013. This research proposes no new ticket. The DVD part of G00157 can join the pattern list of #116 or #111.

Witness files: C000006, C000008, C000013, C000022, C000027, C000066, C000073, C000091, C000106, C000122, C000124, C000132, C000141, C000162.

## Side findings

- Duplicate codes. When the `bsmod` code is equal to the whole flag string, Go writes the code one time (`matroska_ac3_canonical.go:227`). v26.10 does not compare values for `ServiceKind` (rule 4). Thus `bsmod` 5 with `FlagCommentary` gives `C / C` in v26.10 and `C` in Go. No corpus file shows this. It is important only if K1 keeps the flags.
- Other containers. MP4 (`mp4_probe.go:307-309`), AVI (`avi.go:804-805`), MPEG-TS (`mpeg_ts.go:2545-2546`) and the MP4 AC-3 probe (`analyze.go:665-667`) also set `ServiceKind` from the AC-3 `bsmod`. This research found no corpus pattern for them.
