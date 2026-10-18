# Picture geometry: aspect ratio, sampled size and rotation

Research for issue #68, a child of the map issue #65. Date: 2026-10-18.

## Question

Which picture geometry fields are wrong in go-mediainfo, and what causes them? The fields are the display aspect ratio (DAR), the pixel aspect ratio (PAR), their `_Original` variants, the sampled size, the stored size and the rotation. upbrr reads `DisplayAspectRatio`, `PixelAspectRatio`, `Stored_Width` and `Stored_Height` from the JSON output.

## Answer

Eleven root causes explain the patterns. Ten are bugs. One is a newer MediaInfo behavior that needs a decision.

| ID | Root cause | Patterns | Verdict |
|---|---|---|---|
| A | Matroska: the codec aspect ratio overrides the container display size | G00089, G00151, G00242, G00507, cluster 136, part of G00331 and G00332 | Bug |
| B | MP4: the DAR ignores the SAR, the `pasp` box and the `tkhd` track size | G00089, G00151, G00242, G00331, G00332 | Bug |
| C | MP4: the rotation is always 0 | G00166, G00167 | Bug |
| D | MP4: the container size and the stream size go to the wrong fields | G00652, G00893, clusters 152, 155, 168, 172 | Bug |
| E | Every video track gets `Sampled_Width` and `Sampled_Height` | G00158, G00159, part of G00652 and G00893 | Bug |
| F | The text output shows the PAR for AVI and Ogg | G00254 | Bug |
| G | The text output never shows "Original display aspect ratio" | G00242 | Bug |
| H | The raw text output does not use the named-ratio table | G00471, the value form of G00332 | Bug |
| I | MPEG-PS: go-mediainfo truncates the PAR and does not round it | part of G00151 | Bug |
| J | MPEG-2: go-mediainfo ignores the display size in the sequence display extension | probable part of G00089 and G00151 (MPEG-PS) | Bug, not proven on a corpus file |
| K | The text DAR bins `2.39:1` and `9:16` come from a newer MediaInfoLib | part of G00089 (text only) | Decision needed |

One more cause is outside the geometry code, but it explains four small geometry patterns.

| ID | Root cause | Patterns | Verdict |
|---|---|---|---|
| L | MP4: an H.263 sample entry gets no video size | G01391, G00890, G01474, G01475, clusters 165, 171 | Bug, probable |

## Method

- go-mediainfo: `main` at `6a37300`. File and line references are to that commit.
- MediaInfoLib: v23.04, from `git -C ~/github/oss/MediaInfoLib archive v23.04`. References marked "v23.04" are to that tree. References marked "tip" are to the checkout tip `dd11d7971`, which is newer than v23.04.
- Media host: official `mediainfo --Version` reports `MediaInfoLib - v23.04`. I built go-mediainfo with `GOOS=linux GOARCH=amd64 go build ./cmd/mediainfo` and copied it to `/tmp/go-mediainfo-research-68`.
- Files: I read no corpus file and no media-library file. I made five synthetic files with ffmpeg and copied them to `/tmp/research-68` on the host:
  - `s1.mkv`: Matroska, AVC 720x576. The SPS has SAR 16:15 (DAR 4:3). The container display size is 1024x576 (DAR 16:9).
  - `s2.mp4`: MP4, AVC 720x480. The SPS has SAR 40:33. A `pasp` box has 40:33. The `tkhd` matrix rotates the track.
  - `s3.webm`: WebM, VP8 320x240.
  - `s4.avi`: AVI, MPEG-4 Visual 320x240.
  - `s5.mpg`: MPEG-PS, MPEG-2 720x480, aspect code 2 (4:3), no sequence display extension.
- Both binaries read each file with the harness options: `--Output=JSON --Language=raw --ParseSpeed=0.5` against `--output=JSON --language=raw`, the default text output, and the raw text output. The tables below show only the video track.
- go-mediainfo alone also read three local files. They are copies of a square-pixel AVC MP4. A script changed one header field in each: the `stsd` height or the `tkhd` width. Official MediaInfo did not read these files, so the official value for them comes from the source.

### Host results

| File | Field | Official v23.04 | go-mediainfo |
|---|---|---|---|
| s1.mkv | `DisplayAspectRatio` | 1.778 | 1.333 |
| s1.mkv | `DisplayAspectRatio_Original` | 1.333 | absent |
| s1.mkv | `PixelAspectRatio` | 1.422 | 1.067 |
| s1.mkv | `PixelAspectRatio_Original` | 1.067 | absent |
| s1.mkv | text "Display aspect ratio" | 16:9 | 4:3 |
| s1.mkv | text "Original display aspect ratio" | 4:3 | absent |
| s1.mkv | raw `DisplayAspectRatio_Original/Stri` | 4:3 | absent |
| s2.mp4 | `DisplayAspectRatio` | 1.818 | 1.500 |
| s2.mp4 | `DisplayAspectRatio_Original` | absent | 1.818 |
| s2.mp4 | `PixelAspectRatio` | 1.212 | 1.212 |
| s2.mp4 | `Rotation` | 270.000 | 0.000 |
| s2.mp4 | text "Display aspect ratio" | 16:9 | 3:2 |
| s2.mp4 | text "Rotation" | 270° | absent |
| s2.mp4 | raw `DisplayAspectRatio_Original/Stri` | absent | 1.818 |
| s3.webm | `Sampled_Width`, `Sampled_Height` | absent | 320, 240 |
| s4.avi | text "Pixel aspect ratio" | absent | 1.000 |
| s5.mpg | `PixelAspectRatio` | 0.889 | 0.888 |

### Local go-only results

| File | Field | go-mediainfo | Official, from the source |
|---|---|---|---|
| `stsd` height 470, SPS height 480 | `Height` | 470 | 470 |
| same | `Height_Original` | absent | 480 |
| same | `Sampled_Height` | 470 | 480 |
| same | `Stored_Height` | 480 | absent |
| `tkhd` width 853, `stsd` 720x480 | `DisplayAspectRatio` | 1.500 | 1.777 |
| Ogg Theora 320x240 | text PAR line | `PixelAspectRatio : 1.000` | absent |
| Ogg Theora 320x240 | text "Display aspect ratio" | 1.333 | 4:3 |

## How official MediaInfo merges container and stream values

Most causes depend on one rule in MediaInfoLib. Read it first.

A container parser (Matroska, MP4) fills the video track first. Then it merges the codec parser (AVC, HEVC, MPEG-2) into that track with `Merge` (v23.04 `File__Analyze.h:1238`, called at v23.04 `File_Mk.cpp:1291` and `File_Mpeg4.cpp:1033`). Before the merge, `Merge` saves the container values of `Width`, `Height`, `PixelAspectRatio` and `DisplayAspectRatio` (v23.04 `File__Analyze_Streams.cpp:2182-2185`). After the merge, it compares each saved value with the codec value as a formatted string with 3 decimals. If the two strings differ, the container value goes back into the main field, and the codec value goes into the `_Original` field (v23.04 `File__Analyze_Streams.cpp:2290-2310`).

So the container wins, and official keeps the codec value in `_Original`. go-mediainfo has no such rule in one place. Each container parser decides on its own, and most of them let the codec win.

## Root causes

### A. Matroska: the codec aspect ratio overrides the container display size

Official v23.04 sets a container DAR when a `DisplayWidth` or `DisplayHeight` element is present (v23.04 `File_Mk.cpp:4066-4094`). It snaps 1.777-1.778 to 16/9 and 1.333-1.334 to 4/3, fills the DAR with 3 decimals, and derives the PAR from it (v23.04 `File_Mk.cpp:1038-1050`). Then `Merge` keeps the container DAR and PAR and moves the SPS values to `_Original`.

go-mediainfo uses the SPS DAR when the SPS has a SAR other than 1:1, and it ignores the container display size (`internal/mediainfo/matroska_video_canonical.go:466-470`, text at `internal/mediainfo/matroska.go:2229-2234`). It uses the container values only when the coded size also differs from the visible size (`matroska_video_canonical.go:479-486`, and the same code at `matroska.go:2647-2652`). That condition has no source in MediaInfoLib. The same branch compares the two ratios with a tolerance of 1e-9, but official compares 3-decimal strings. So go-mediainfo sometimes writes `_Original` fields where official writes none.

On `s1.mkv`, official shows DAR 16:9 (PAR 1.422) and keeps 4:3 (PAR 1.067) as the original. go-mediainfo shows 4:3. This is the first example of G00089 (`1.778` to `1.333`, `16:9` to `4:3`) and of G00151 (`1.422` to `1.067`). The Matroska files in G00242 and G00507 lack the original DAR for the same reason. Cluster 136 (`PixelAspectRatio_Original`, new and missing) comes from the condition and the tolerance.

Official is correct here. The Matroska specification makes `DisplayWidth` and `DisplayHeight` the display size, and players use it over the SAR in the bitstream.

Verdict: bug. Witnesses: C634118, C629648, C007228 (cluster 56). C003438, C633550 (cluster 136). C282701, C608866, C629463 (cluster 16).

### B. MP4: the DAR ignores the SAR, the `pasp` box and the `tkhd` track size

Official v23.04 takes `Width` and `Height` from the visual sample entry. If the `tkhd` track size gives a different ratio, it fills the DAR from the `tkhd` size and derives the PAR (v23.04 `File_Mpeg4_Elements.cpp:6146-6150`, `tkhd` parse at `File_Mpeg4_Elements.cpp:8383-8426`). A `pasp` box clears that DAR and fills the PAR (v23.04 `File_Mpeg4_Elements.cpp:7681-7700`). The DAR then follows from the PAR (v23.04 `File__Analyze_Streams.cpp:3359-3408`). The AVC parser fills its own PAR and DAR from the SPS. `Merge` writes `_Original` only when the container and the stream disagree.

go-mediainfo has five gaps here:

- It takes the DAR from the sample entry size only. The text value is at `internal/mediainfo/mp4_codec.go:234-238`, and the structured value is at `mp4_codec.go:857-870`. The SAR from the SPS goes to `PixelAspectRatio`, but not into the DAR (`mp4_codec.go:288-292` and `339-343`). So the PAR and the DAR disagree with each other.
- It writes `DisplayAspectRatio_Original` every time the SPS SAR is not 1:1 (`mp4_codec.go:291` and `342`). Official writes it only when the container disagrees with the stream.
- It does not read the `pasp` box. No code in `internal/mediainfo` looks for `pasp`.
- It does not read the `tkhd` track width and height (`internal/mediainfo/mp4.go:481-538`).
- The HEVC path in MP4 does not use the SAR at all (`mp4_codec.go:660` and the lines after it).

On `s2.mp4`, official shows DAR 1.818 (16:9) with no original. go-mediainfo shows DAR 1.500 (3:2), PAR 1.212, and an extra original DAR 1.818. This explains the MPEG-4 files in G00089, and the new `DisplayAspectRatio_Original` in G00331 (20 MPEG-4 files) and G00332. The `tkhd` gap explains MP4 files where official has an original DAR and go-mediainfo has none (G00242, 13 MPEG-4 files). On the local file with `tkhd` width 853, go-mediainfo shows DAR 1.500, and the source gives 1.777.

Verdict: bug. Witnesses: C609769, C610704, C610966 (cluster 79). C608866 (clusters 16 and 30).

### C. MP4: the rotation is always 0

Official v23.04 reads the `tkhd` matrix and computes the rotation as `atan2(b, a)` in degrees. It adds 360 when the value is negative (v23.04 `File_Mpeg4_Elements.cpp:8383-8429`). It fills `Rotation` with 3 decimals. It fills the text value `Rotation/String`, for example `90°`, only when the rotation is not 0 (v23.04 `File_Mpeg4_Elements.cpp:6151-6153`). The text field is visible (v23.04 `MediaInfo_Config_Automatic.cpp:4840`, `Rotation/String;;;Y NT`).

go-mediainfo writes `Rotation` as `0.000` for every MP4 video track (`internal/mediainfo/analyze.go:367-369`). `parseTkhd` does not read the matrix (`internal/mediainfo/mp4.go:481-538`).

On `s2.mp4`, official shows `Rotation` 270.000 and the text line `270°`. go-mediainfo shows 0.000 and no text line. This is G00166 and G00167. The codec has no effect: the 77 files have AVC and HEVC.

Verdict: bug. Witnesses: C521834, C521840, C521888.

### D. MP4: the container size and the stream size go to the wrong fields

Official v23.04 takes `Width` and `Height` from the sample entry. When the SPS gives a different size, `Merge` keeps the sample entry size and moves the SPS size to `Width_Original` and `Height_Original` (v23.04 `File__Analyze_Streams.cpp:2290-2299`). `Sampled_Width` and `Sampled_Height` come only from the codec parser, so they keep the SPS size (see cause E). The AVC parser writes `Stored_Width` and `Stored_Height` only when the SPS has crop offsets, and the value is the coded size (v23.04 `File_Avc.cpp:1020-1025`).

go-mediainfo uses the sample entry size for `Width` and `Height`, and it copies them into `Sampled_*` (`internal/mediainfo/field_finalize.go:178-179`). To decide the stored size, it compares the SPS coded size with the sample entry size, not with the SPS visible size (`internal/mediainfo/mp4_codec.go:253-266`). It never writes `Width_Original` or `Height_Original`.

On the local file with `stsd` height 470 and SPS height 480, go-mediainfo shows `Sampled_Height` 470, `Stored_Height` 480 and no `Height_Original`. The source gives `Sampled_Height` 480, no `Stored_Height`, and `Height_Original` 480. This matches G00652 (`480` to `470`), cluster 152 (`Height_Original` missing) and cluster 155 (`Stored_Height` new). The width patterns have the same cause in the other direction: the SPS is 854 wide with a crop, and the sample entry says 864. These are G00893 (`854` to `864`), cluster 168 (`Stored_Width` missing) and cluster 172 (`Width_Original` missing).

Verdict: bug. upbrr reads `Stored_Width` and `Stored_Height`, so this cause changes upbrr input. Witnesses: C608279, C608321, C608715.

### E. Every video track gets `Sampled_Width` and `Sampled_Height`

Official v23.04 fills `Sampled_Width` and `Sampled_Height` when a parser fills `Width` and `Height`, but only when that parser is a stream parser (v23.04 `File__Analyze_Streams.cpp:1066-1067`). A parser is a container parser by default (v23.04 `File__Analyze.cpp:186`). The VP8 parser is a stream parser (v23.04 `File_Vp8.cpp:47`), but it fills only the format and not the size (v23.04 `File_Vp8.cpp:80-84`). So a VP8 track in WebM has no sampled size. A DVD IFO file and a codec without a stream parser have none either.

go-mediainfo copies `Width` and `Height` into `Sampled_*` for every video track that has a size (`internal/mediainfo/field_finalize.go:178-179`, and `internal/mediainfo/json_fields.go:503-508` on the older JSON path).

On `s3.webm`, official has no `Sampled_*`. go-mediainfo shows 320 and 240. This is G00158 and G00159 (WebM 60, DVD Video 18, and a few MP4 and AVI files). The triage sheet says that v26.10 also omits the field. The same copy gives the wrong value in cause D.

Verdict: bug. The value repeats `Width` and `Height`, and no stream parser measured it. Witnesses: C609018, C609038, C609044.

### F. The text output shows the PAR for AVI and Ogg

Official v23.04 hides the video `PixelAspectRatio/String` field in the text output (v23.04 `MediaInfo_Config_Automatic.cpp:4825`, `PixelAspectRatio/String;;;N NT`). The text output never shows a PAR line for video.

The go-mediainfo AVI parser fills the PAR with a text label (`internal/mediainfo/avi.go:516`). The Ogg parser fills it through the field store with no known field spec, so the store shows it in text under its raw name (`internal/mediainfo/ogg.go:406`, `internal/mediainfo/field_store.go:179-186`). The Ogg parser also fills the DAR with no display string, so the text shows `1.333` where official shows `4:3` (`ogg.go:409`).

On `s4.avi`, go-mediainfo adds the text line `Pixel aspect ratio : 1.000`, and official has no such line. This is G00254 (AVI 36, Ogg 1). The local Ogg Theora file shows `PixelAspectRatio : 1.000` and `Display aspect ratio : 1.333`.

Verdict: bug. Witnesses: C000171, C608397, C670356.

### G. The text output never shows "Original display aspect ratio"

Official v23.04 shows `DisplayAspectRatio_Original/String` in text (v23.04 `MediaInfo_Config_Automatic.cpp:4833`, `Y NT`). The value uses the named-ratio table (see cause H).

No go-mediainfo code fills a text value for `DisplayAspectRatio_Original`. The field exists only as a structured value (`internal/mediainfo/matroska_video_canonical.go:484`, `internal/mediainfo/mp4_codec.go:291` and `342`). The schema knows only `DisplayAspectRatio` (`internal/mediainfo/field_schema.go:91`). The label "Original display aspect ratio" appears only in the raw text mapping (`internal/mediainfo/field_project_raw_text.go:84`).

On `s1.mkv`, official shows `Original display aspect ratio : 4:3`, and go-mediainfo shows nothing. On `s2.mp4`, go-mediainfo has the field in JSON but not in text. Every file in G00242 (40 files) has this cause. On some of them, cause A or cause B also removes the structured value.

Verdict: bug. upbrr pastes the text output into tracker uploads. Witnesses: C007228, C629648, C634118.

### H. The raw text output does not use the named-ratio table

Official v23.04 makes every DAR string with one table: 5:4, 4:3, 3:2, 16:10, 5:3, 16:9, 1.85:1, 2.2:1, 2.25:1, 2.35:1, 2.40:1, or the number (v23.04 `File__Analyze_Streams.cpp:3435-3446`). The same function fills the `_Original` string.

The go-mediainfo raw text projection formats `DisplayAspectRatio/String` and `DisplayAspectRatio_Original/Stri` with `formatRawTextAspectRatio` (`internal/mediainfo/field_project_raw_text.go:776-781`). That function knows only 4:3 and 16:9, within 0.01. It prints the number for all other values (`field_project_raw_text.go:878-891`). The normal text output already has the full table in `formatAspectRatio` (`internal/mediainfo/format_fields.go:42-79`).

In G00471, official shows `1.85:1` and go-mediainfo shows `1.853`. In a second example, official shows `4:3` and go-mediainfo shows `1.313`. The value 1.313 is in the 1.30-1.37 bin, so official prints 4:3. On `s2.mp4`, go-mediainfo prints `1.818`, and the table gives 16:9. The new values in G00332 have the same number form.

Verdict: bug. Witnesses: C609769, C610966, C629648.

### I. MPEG-PS: go-mediainfo truncates the PAR and does not round it

The official v23.04 MPEG-2 parser fills the DAR as 4:3 or 16:9 (v23.04 `File_Mpegv.cpp:1295`). The PAR follows from the DAR, rounded to 3 decimals (v23.04 `File__Analyze_Streams.cpp:3412-3428`).

The go-mediainfo MPEG-PS path truncates the PAR to 3 decimals with `math.Floor` (`internal/mediainfo/mpeg_ps.go:972`). The TS path and the DVD path round (`internal/mediainfo/mpeg_ts.go:2164`, `internal/mediainfo/dvd.go:2557`).

On `s5.mpg` (720x480, 4:3), official shows PAR 0.889, and go-mediainfo shows 0.888. The exact value is 0.8889. This explains the MPEG-PS files in G00151 that are 720x480 at 4:3. Other common sizes give the same result both ways, for example 1.185 and 1.422.

Verdict: bug. upbrr reads `PixelAspectRatio`. Cluster 30 names no MPEG-PS witness. C282701 is a witness for cluster 30, but its container is not known.

### J. MPEG-2: go-mediainfo ignores the display size in the sequence display extension

The sequence display extension can give a display size. If it does, the aspect ratio code applies to that display rectangle, not to the full frame (ISO/IEC 13818-2). Official v23.04 converts it to the full frame: DAR = (width / height) × DAR code / (display width / display height) (v23.04 `File_Mpegv.cpp:1288-1292`). This formula agrees with the standard.

go-mediainfo reads the video format and the colour description from the extension. It stops before the display size (`internal/mediainfo/mpeg2_video.go:445-470`).

Take a 720x480 4:3 stream with a 704x480 display size. Official gives DAR 1.364 and PAR 0.909. go-mediainfo gives 1.333 and 0.889. ffmpeg cannot write a display size that differs from the coded size, so I did not run this case on the host. This cause is a likely part of the 18 MPEG-PS files in G00089 and the 17 in G00151, but no corpus file proves it.

Verdict: bug in the code. Its share of the corpus patterns is not proven.

### K. The text DAR bins `2.39:1` and `9:16` come from a newer MediaInfoLib

Official v23.04 prints `2.40:1` for every DAR from 2.37 to 2.45, and it has no `9:16` bin (v23.04 `File__Analyze_Streams.cpp:3435-3446`).

go-mediainfo prints `2.39:1` from 2.37 to 2.395, and `9:16` from 0.54 to 0.58 (`internal/mediainfo/format_fields.go:50-51` and `72-75`). These bins match the MediaInfoLib tip (tip `File__Analyze_Streams.cpp:4171-4173`). The checkout is shallow, so it does not show which release added them.

The difference is in text only. A DAR of 2.388 shows `2.39:1` in go-mediainfo and `2.40:1` in official. `docs/agents/parity-samples.md` records this on samples fresh-06, fresh-08 and fresh-09. It is part of G00089, but the triage sheet does not split G00089 by value.

Verdict: decision needed. `2.39:1` is nearer to the real value, and newer MediaInfo prints it too. But ADR 0001 rule 2 lets go-mediainfo follow newer MediaInfo only with a recorded reason. The deliberate-difference list in `AGENTS.md` has no line for these bins. The maintainer must add the line or go back to the v23.04 table.

### L. MP4: an H.263 sample entry gets no video size

`isVideoSampleEntry` accepts only `av01`, `avc1`, `avc3`, `hvc1`, `hev1`, `mp4v` and `vp09` (`internal/mediainfo/mp4_codec.go:193-200`). An H.263 track in MP4 (3GPP sample entry `s263`) gets no `Width` and no `Height`. So it also gets no `Sampled_*`, PAR or DAR.

Official v23.04 runs its H.263 parser on the track (v23.04 `Video/File_H263.cpp`). That parser is a stream parser, so the track gets a size, a sampled size, a PAR and a DAR.

Clusters 165 and 171 (`Height` and `Width` missing) have witness C528255, an MP4 file with H.263 video. The values of four small geometry patterns fit an H.263 QCIF picture (176x144 with PAR 12:11):

- G01474 and G01475: `Sampled_*` missing, 176x144.
- G00890: PAR 1.091 missing. 1.091 is 12:11.
- G01391: DAR 1.333 and 4:3 missing. 176 × 12/11 / 144 = 1.333.

Verdict: bug, probable. The witness file is not available, so the sample entry code is not proven.

## Patterns not fully explained

- G00089 (242 files) and G00151 (93 files) mix causes A, B, I, J and K. The triage sheet gives at most four value pairs for each pattern, so this research cannot give the share of each cause.
- The DVD Video file in clusters 165 and 171 (C534799, "Video unknown") has no size in go-mediainfo. This research did not find why.
- The second example of G00890 (PAR `1.000` missing) can be that DVD Video file, but nothing proves it.
- The ticket names C007002 as a witness. The triage sheet does not show which geometry pattern it belongs to.
- Matroska with square pixels: go-mediainfo uses the container display size only when its ratio differs from the pixel ratio by 0.005 or more (`internal/mediainfo/matroska_video_canonical.go:461-466`). Official uses it on any 3-decimal difference. No corpus pattern shows that this tolerance changes a value.

## Questions for the contributor

These questions need the witness files.

1. For an MPEG-PS file in G00089 or G00151: does the MPEG-2 video have a sequence display extension with a display size that is not the coded size? The official `--Details=1` output of one sequence header is enough. The answer gives the share of cause J.
2. For C528255: what is the MP4 sample entry code of the video track, for example `s263`? The answer proves or disproves cause L.
3. For C534799: what video format and size does official report, and what does go-mediainfo report? Cause L does not explain this file.
4. For C007002: which geometry pattern does it witness?
5. For two or three MPEG-4 files in G00242: what are the `tkhd` track width and height, and does the file have a `pasp` box? The answers split cause B into its `tkhd` part and its `pasp` part.
