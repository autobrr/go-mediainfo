# AC-3 metadata fields: root causes (issue #78)

Research date: 2026-10-18. Map issue: #65. Corpus run: 2026-10-17.

This file groups the AC-3 and E-AC-3 metadata patterns of the corpus run into root causes. The metadata fields are `dialnorm`, `compr`, `dynrng`, `dsurmod`, `bsid`, `acmod`, `lfeon`, the mix level fields and the statistics. The mix level fields are `cmixlev`, `surmixlev`, `mixlevel`, `roomtyp`, `dmixmod`, `ltrtcmixlev`, `ltrtsurmixlev`, `lorocmixlev`, `lorosurmixlev` and `adconvtyp`. The statistics are the `*_Average`, `*_Minimum`, `*_Maximum` and `*_Count` fields. The triage sheet puts 89 patterns on 1254 files in this family. The triage put 17 more patterns of the same fields in other groups. This file covers all 106 patterns.

Each cause has one of these verdicts:

- Bug. go-mediainfo is wrong and must change.
- Deliberate. go-mediainfo corrects a proven bug in official v23.04. `AGENTS.md` does not list the difference yet. ADR 0001 rule 2 requires one line in the "Deliberate differences" list.
- Decision. go-mediainfo follows a newer MediaInfo, or go-mediainfo can be more correct than v23.04. The maintainer must choose. ADR 0001 rule 2 requires a recorded reason, or a revert to v23.04.
- Covered. An open ticket already holds the cause.

The file refers to corpus files by C-id and to patterns by G-id. It names no release, path or tag value.

## Sources and method

- go-mediainfo: `origin/main` at `edbef00`. The code under `internal/mediainfo` is the same as at `6a37300`, the base of the corpus binary. Citations are `internal/mediainfo/<file>:<line>`.
- MediaInfoLib v23.04: the tag `v23.04` of the local MediaInfoLib checkout. Citations are `Source/MediaInfo/<file>:<line>` at that tag.
- MediaInfoLib master: commit `dd11d797` of the same checkout. Its version string is `MediaInfoLib - v26.05` (`MediaInfo/MediaInfo_Config.cpp:141`). The text says "master" for these citations.
- Corpus data: the HTML archive of the 2026-10-17 run and the v26.10 presence lists. Local scripts counted files and value pairs by pattern, output and container. The scripts also sorted the files by two facts:
  - VOB title set. The file is a title VOB of a DVD title set (`VTS_nn_1.VOB` and higher). Go joins these files (#116).
  - `audprodie`. The first AC-3 frame carries the audio production information. Go then reports `mixlevel` in JSON (pattern G00058).
- Synthetic files, made with `ffmpeg` on the local machine. Each file holds generated tones and no library material.

| ID | Stream | Metadata that `ffmpeg` writes |
|---|---|---|
| S1 | AC-3 5.1, 448 kb/s, 10 s | dialnorm -27, `audprodie` 1 (mixlevel 105, roomtyp Large) |
| S2 | AC-3 5.1, 448 kb/s, 10 s | dialnorm -27, `audprodie` 0 (control) |
| S3 | AC-3 5.1, 448 kb/s, 10 s | dialnorm -27, `audprodie` 1 (mixlevel 105, roomtyp 0) |
| S4 | AC-3 2.0, 192 kb/s, 10 s | Dolby Surround mode 2, `audprodie` 1 (mixlevel 90, roomtyp Small) |
| S5 | AC-3 5.1, 448 kb/s, 10 s | bsid 6 with `xbsi1` (Lo/Ro, Lt/Rt levels) and `xbsi2` (HDCD), `audprodie` 1 |
| S6 | AC-3 5.1, 448 kb/s, 2 s | `cmixlev` code 2, `surmixlev` code 2 |

S1 to S5 exist as raw `.ac3`, as Matroska, and as an MPEG-2 program stream (`ffmpeg -f vob`). S6 is Matroska only. `ffmpeg` writes `dynrnge` 0 in every audio block, so no synthetic file carries a dynamic range word.

- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The Go binary was a local `linux/amd64` build of `edbef00` at `/tmp/go-mediainfo-research-78`. The runs used S1 to S6 and four parity samples: `mp4-01`, `mkv-05`, `dvd-01` and `dvd-02`. The research removed its files from the host after the runs.
- Host commands: `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5 <f>`, `mediainfo --ParseSpeed=0.5 <f>`, `mediainfo --ParseSpeed=0.5 --Language=raw <f>`, the same three outputs from the Go binary, and `mediainfo --Details=1 <f>` for a bit trace. Each command ran under `ionice -c3 nice -n10 timeout 300`.

In this file, "text" is the default text output. "Raw text" is the text output with `--Language=raw`. CSV and HTML use the same lines as text.

## How official MediaInfo fills AC-3 metadata

1. Each field has an option string. Position 0 controls the text and raw text outputs. Position 4 controls JSON and XML (`MediaInfo/MediaInfo_Const.h:93-97` at v23.04). `N NT` hides a field from text and shows it in JSON. `N NTN` hides it from both. `Y NTN` shows it in text only.
2. v23.04 gives every metadata field `N NT` and every `/String` twin `N NTN` (`Audio/File_Ac3.cpp:1340-1385`, `:1496-1585`). Thus v23.04 shows no AC-3 metadata line in text or raw text. JSON has `bsid`, `dialnorm`, `compr`, `dynrng`, `dsurmod`, `acmod`, `lfeon`, `dialnorm_Average`, `dialnorm_Minimum`, the four `compr_*` fields and the four `dynrng_*` fields. JSON never has `dialnorm_Maximum` or `dialnorm_Count`, because they are `N NTN` (`:1505`, `:1509`).
3. v23.04 reads the mix level fields and drops them. It skips `cmixlev`, `surmixlev`, `mixlevel`, `roomtyp`, the `xbsi1` levels and `adconvtyp` (`Audio/File_Ac3.cpp:2295-2297`, `:2308-2310`, `:2330-2340`).
4. v23.04 reads `mixlevel` and `mixlevel2` as 8 bits (`Audio/File_Ac3.cpp:2309`, `:2322`). ATSC A/52 defines `mixlevel` as 5 bits. master reads 5 bits (`Audio/File_Ac3.cpp:2416`, `:2429` at master). The E-AC-3 path reads 5 bits in both versions (`:2602`, `:2609` at v23.04).
5. master changes the text rules. The `/String` twins of `dialnorm`, `compr`, `dynrng`, `dsurmod`, `dialnorm_Average`, `dialnorm_Minimum` and `dialnorm_Maximum` become `Y NTN` (master `Audio/File_Ac3.cpp:1401`, `:1412`, `:1424`, `:1439`, `:1606-1614`). The `compr_*` and `dynrng_*` statistics and `dialnorm_Count` stay hidden from text (master `:1616`, `:1636-1692`).
6. master fills the mix level fields from the first frame (master `Audio/File_Ac3.cpp:1532-1576`, `:3909-3917`). It fills `cmixlev` and `surmixlev`, and `surmixlev` code 2 gives `-inf`. It fills `mixlevel` and `roomtyp` only when `audprodie` is set, and `roomtyp` only when it is not 0. It fills `dmixmod` when the code is not 0. It fills the four `xbsi1` levels, and code 7 gives `-inf` (`:231-245`). It fills `adconvtyp` only for HDCD. The raw values are `N NT`. The text values are `Y NTN` or `Y NTY`.
7. The text label of `dialnorm` is `Dialog Normalization` (`Resource/Text/Language/DefaultLanguage.csv:336` at v23.04). The other fields use their raw names as labels.
8. The v26.10 presence lists agree with master. They list `Dialog Normalization` and `compr` as extra in Go only on additional tracks. They list `cmixlev`, `dmixmod`, `roomtyp`, `ltrtcmixlev` and `lorocmixlev` among the fields that v26.10 has and Go does not have on some files.

## Summary

| Cause | Verdict | Patterns | Files (about) |
|---|---|---|---|
| M1. Text shows the AC-3 metadata lines, with a different rule per container | Decision | G00014, G00015, G00016, G00017, G00021, G00040, G00044, G00052, G00059, G00061, G00069 | 1240 |
| M2. Text shows the `compr_*` and `dynrng_*` statistics | Bug | G00264, G00265, G00266, G00338, G00339, G00340, G00359, G00417 (parts that are not DVD Video) | 21 |
| M3. JSON has the mix level fields that master added | Decision, same family as `dmixmod` in #83 | G00041, G00043, G00058, G00068 | 609 |
| M4. Mix level values and rules differ from master and from A/52 | Bug, scope depends on M3 | part of G00068 (`Not indicated`), part of G00043 (`0 dB`) | 30 |
| M5. Official v23.04 reads `mixlevel` as 8 bits, so its `dynrng` is wrong | Deliberate | the `audprodie` part of G00062, G00085, G00090, G00142, G00146, G00275 to G00278, G00153 to G00156, G00162 | 284 |
| M6. MPEG-TS and BDAV hide `dynrng` by channel mode | Bug | no pattern | not known |
| M7. JSON has `dialnorm_Maximum` when the minimum and the maximum differ | Bug | G00313 (parts that are not DVD Video) | 14 |
| M8. Official v23.04 averages `dialnorm` code 0 as 0 dB | Deliberate | G00605 (Matroska part) | 2 |
| M9. The DVD IFO report copies the VOB AC-3 metadata, with duplicate lines | Covered by #116 and #111 | DVD Video parts of all patterns above, and G00431, G00440 to G00454, G00465 to G00469, G00489, G00548 to G00550, G00567, G00606, G00632, G00635, G00657, G01802 to G01805 | 14 |
| M10. Go joins the VOB title set, so it reads other frames | Covered by #116 | G00133, G01110, G01787, G01793, G01795, and the VOB parts of G00062, G00073, G00084, G00085, G00086, G00090, G00102, G00142, G00143, G00583 to G00586, G00605 | 260 |
| M11. Go reads a different frame window for the statistics outside VOB title sets | Not explained, needs a witness file. The MPEG-PS part touches #119. | the other parts of G00073, G00084, G00086, G00102, G00143, and 13 files of the `dynrng` patterns | 70 |
| M12. `compr` code 0 is `0.00` in Go and `-0.00` in official | Decision | G01112, G01791, G01792 | 2 |
| Fields that v26.10 also emits | Covered by #83 | G00095, G00096, G00290 to G00297, G00444, G00451, G00656, G00661 | 216 |

One file can be in more than one cause. The causes explain all 1254 files of the family. M11 and the patterns in "Not explained" need a witness file for the last step.

## M1. Text shows the AC-3 metadata lines

### What goes wrong

v23.04 shows no AC-3 metadata line in any text output (rule 2). Go shows these lines. Each container writes its own list, so the lists differ:

- MPEG-PS writes `Dialog Normalization`, `compr`, `cmixlev`, `surmixlev`, `dmixmod`, the `xbsi1` levels, `mixlevel`, `roomtyp` and the three `dialnorm_*` lines into the text fields (`mpeg_ps.go:1226-1290`). It writes `dynrng`, `dsurmod`, `bsid`, `acmod` and `lfeon` only for DVD (`:1223`, `:1238-1248`).
- MPEG-TS and BDAV write a similar list and the statistics (`mpeg_ts.go:2563-2591`, `:2674-2701`). M2 covers the statistics.
- AVI and MP4 share one writer (`avi.go:1622-1665`, called from `mp4_probe.go:311`). It also writes `dynrng` and the statistics. It writes `dialnorm_Maximum` only when the value differs from the minimum (`avi.go:1649`).
- Matroska writes no line in text. Raw text shows the JSON extras through a shared projection (`field_project_raw_text.go:1111-1124`, `:1167-1190`, `:2008-2010`). The projection also adds a `dialnorm_Maximum` line from `dialnorm` when JSON has none (`field_project_raw_text.go:305-320`).

Thus Matroska text follows v23.04, and the other containers follow neither v23.04 nor master.

### Proof

- S1, S2, S4 and S5 on the host. Official text and raw text have no AC-3 metadata line for any container. Go text has the lines for the VOB copies and none for the Matroska copies. Go raw text has the lines for both. For S1 as VOB, Go text shows `Dialog Normalization: -27 dB`, `cmixlev: -4.5 dB`, `surmixlev: -6 dB`, `mixlevel: 105 dB`, `roomtyp: Large` and the three `dialnorm_*` lines.
- `mp4-01` (MP4) and `dvd-02` (title VOB) on the host. Official text has no line. Go text has `Dialog Normalization` and the `dialnorm_*` lines.
- Corpus. For G00014, the difference is only in raw text on 923 Matroska tracks. It is in text on 321 MPEG-PS, 15 BDAV, 7 MP4, 3 AVI and 1 MPEG-TS tracks.

### Verdict

Decision. master shows most of these lines in text (rules 5 and 6), and v26.10 agrees (rule 8). The maintainer must choose one rule for all containers. One choice is v23.04, with no AC-3 metadata line in text or raw text. The other choice is master, with the `/String` lines of rules 5 and 6 in text and raw text for every container. The statistics stay hidden for both choices (M2).

The fix is one shared writer for all containers. Both choices need it, because the four writers above disagree now.

Witness files: C000014, C000064, C000067 (MPEG-PS text), C000034, C000035, C000037 (Matroska raw text).

## M2. Text shows the compr and dynrng statistics

### What goes wrong

MPEG-TS and BDAV write `compr_Average`, `compr_Minimum`, `compr_Maximum` and the three `dynrng_*` lines into text (`mpeg_ts.go:2686-2688`, `:2699-2701`). AVI and MP4 write the same lines, and also `compr_Count` and `dynrng_Count` (`avi.go:1653-1664`). v23.04 hides them from text (rule 2). master also hides them (rule 5). M9 covers the DVD part of these patterns.

### Proof

Source only. The corpus shows the lines in Go text for BDAV, MP4, AVI and MPEG-TS files. Official text never has them.

### Verdict

Bug. Both MediaInfo versions hide these lines. The fix of M1 can remove them, but they must stay hidden for both choices of the M1 decision.

Witness files: C000031, C000033, C000060.

## M3. JSON has the mix level fields that master added

### What goes wrong

Go reads `cmixlev`, `surmixlev`, `mixlevel` and `roomtyp` and writes them to JSON (`ac3.go:278-301`, `ac3.go:389-408`, `mpeg_ps.go:1504-1551`, `mpeg_ts.go:2650-2655`, `matroska_ac3_canonical.go:341-357`). v23.04 skips these fields (rule 3). master fills them (rule 6). The values agree with master in most files. For S1, Go writes `cmixlev` `-4.5`, `surmixlev` `-6 dB`, `mixlevel` `105` and `roomtyp` `Large`. These are the values that `ffmpeg` wrote.

### Verdict

Decision. This is the same family as `dmixmod`, the `xbsi1` levels and `adconvtyp`. #83 holds those as "fields where Go matches v26.10". The triage put the four fields of M3 in the candidate group, because some v26.10 comparisons still list them as extra. M4 explains those files. The maintainer can settle M3 together with the `dmixmod` line of #83.

Witness files: C000013, C000014, C000022.

## M4. Mix level values and rules differ from master and from A/52

### What goes wrong

If the mix level fields stay (M3), these values are wrong:

1. `surmixlev` code 2 means "no surround in the downmix" (A/52 table 5.10, gain 0). Go writes `0 dB` (`ac3.go:1828-1829`), which means full level. master writes `-inf dB` (master `Audio/File_Ac3.cpp:1543`).
2. `roomtyp` code 0 means "not indicated". MPEG-PS writes `roomtyp: Not indicated` (`ac3.go:1874-1875`, `mpeg_ps.go:1276-1277`, `mpeg_ps.go:1550-1551`). master writes no `roomtyp` for code 0 (master `:1556`). Matroska already drops it (`matroska_ac3_canonical.go:355`).
3. The `xbsi1` level code 7 means gain 0. Go drops the field (`ac3.go:1852-1853`). master writes `-inf` (master `:233`).
4. The containers write different sets. Matroska never writes the four `xbsi1` levels (`matroska_ac3_canonical.go:304-401`). MPEG-PS never writes `adconvtyp`, and it writes the levels only for `bsid` 6 or less (`mpeg_ps.go:1520-1543`).

### Proof

- S6 in Matroska. The official bit trace shows `surmixlev - Surround Mix Level: 2`. Go JSON shows `"surmixlev":"0 dB"`.
- S3 in Matroska. Go writes `mixlevel` and no `roomtyp`. In the corpus, `Not indicated` occurs in G00068 on 28 MPEG-PS tracks and on no Matroska track.
- S5. Go JSON for the VOB copy has `ltrtcmixlev`, `ltrtsurmixlev`, `lorocmixlev` and `lorosurmixlev`, and no `adconvtyp`. Go JSON for the Matroska copy has `adconvtyp` `HDCD` and no level.
- The v26.10 presence list has `ltrtcmixlev` and `lorocmixlev` among the fields that v26.10 has and Go does not have.

### Verdict

Bug. Item 1 is wrong for both choices of M3. Items 2 to 4 apply if the fields stay.

Witness files: C629507 (`surmixlev` `0 dB`), C001102, C001392, C001393 (`Not indicated`).

## M5. Official v23.04 reads mixlevel as 8 bits, so its dynrng is wrong

### What goes wrong

When the first AC-3 frame has `audprodie` set, v23.04 reads `mixlevel` as 8 bits instead of 5 (rule 4). All bits after it move by 3. Official then reads `roomtyp`, the time codes, `addbsie`, the block switch flags, `dynrnge` and `dynrng` from the wrong bits (`Audio/File_Ac3.cpp:2309-2368`). It counts these values in the `dynrng` statistics (`:3826-3837`). Thus the `dynrng` and `dynrng_*` values of official are noise:

- Official can report `dynrng` where the stream has none.
- Official can drop `dynrng` where the stream has one.
- Official reports wide ranges. For example, official reports a maximum of 23.37 dB where Go reports 4.22 dB.

Go reads `mixlevel` as 5 bits (`ac3.go:389-408`). Go then reads `dynrng` at the A/52 position (`ac3.go:537-547`, `ac3.go:1908-1935`). The fields before `mixlevel`, for example `dialnorm` and `compr`, are correct in both.

### Proof

- S1 (raw AC-3) on the host. The official bit trace shows `mixlevel - Mixing Level: 202 (0xCA) - (8 bits)`, then `dynrnge: Yes` and `dynrng: 252 (0xFC)`. `ffmpeg` writes no dynamic range word. Official JSON has `"dynrng":"-0.56"` and `dynrng_*` statistics for S1 and S3, in all three containers. Go has no `dynrng`.
- S2 (no `audprodie`) on the host. The official bit trace shows `dynrnge: No`, and official JSON has no `dynrng`. Thus the `dynrng` of S1 comes from the shift.
- S4 and S5 also have `audprodie` set. There, official reads `dynrnge` 0 from the moved bits. The noise depends on the bits that follow.
- Corpus. Outside the VOB title sets, 171 files of the `dynrng` patterns have `audprodie` and 13 files do not. All 32 files outside the VOB title sets where only official has `dynrng` statistics have `audprodie`. M11 holds the 13 other files.

### Verdict

Deliberate. Go is correct, and master has the fix. Add one line to the "Deliberate differences" list in `AGENTS.md`, with S1 as the proof. M6 removes a rule that copies this official bug.

Witness files: C000052, C000056, C000084 (Matroska, `audprodie` set).

## M6. MPEG-TS and BDAV hide dynrng by channel mode

### What goes wrong

MPEG-TS writes no `dynrng` for an AC-3 stereo track. BDAV writes no `dynrng_*` statistics for a mono track (`mpeg_ts.go:2628-2632`, `mpeg_ts.go:2690-2701`). The code comments say that MediaInfoLib does this. `File_Ac3.cpp` has no such rule. It fills `dynrng` from the first frame for every channel mode (`:1354-1365` at v23.04). The rule probably copies the M5 noise of the TS samples, because official loses `dynrng` by chance on stereo files with `audprodie` (S4).

### Proof

Source only. The corpus has no pattern, because official often loses the same values by chance. A test file needs a real dynamic range word, and `ffmpeg` cannot write one.

### Verdict

Bug. Go hides a real value with a rule that has no source in MediaInfoLib.

Witness files: none. A BDAV or MPEG-TS file with AC-3 stereo and a `dynrng` value can prove it.

## M7. JSON has dialnorm_Maximum when the minimum and the maximum differ

### What goes wrong

MPEG-PS, MP4 and Matroska write `dialnorm_Maximum` to JSON when it differs from `dialnorm_Minimum` (`mpeg_ps.go:1562`, `analyze.go:712`, `matroska_ac3_canonical.go:366`). v23.04 and master never show `dialnorm_Maximum` in JSON. The field is `N NTN` (rule 2, and master `:1612`).

### Proof

Source and corpus. G00313 has no "missing property" partner, so official JSON never has `dialnorm_Maximum`. M9 covers the DVD part of G00313.

### Verdict

Bug.

Witness files: C000682, C000873, C001178.

## M8. Official v23.04 averages dialnorm code 0 as 0 dB

### What goes wrong

The `dialnorm` code 0 is reserved, and decoders use -31 dB for it. v23.04 maps code 0 to -31 for `dialnorm`, `dialnorm_Minimum` and `dialnorm_Maximum` (`Audio/File_Ac3.cpp:1342`, `:1486-1489`). For `dialnorm_Average` it uses the raw code, so code 0 adds 0 dB (`:1490`). master has the same code. Go converts each code to dB first and gets -31 (`ac3.go:1612-1618`, `ac3.go:1754-1759`).

### Proof

Source and corpus. G00605 has the pair `0` to `-31` on two Matroska files. `ffmpeg` cannot write code 0, so no synthetic file proves it.

### Verdict

Deliberate. Go agrees with its own `dialnorm` value and with A/52. Add one line to `AGENTS.md`. The MPEG-PS pairs of G00605 (`-31` to `-29`) belong to M10.

Witness files: C010448, C010449.

## M9. The DVD IFO report copies the VOB AC-3 metadata

### What goes wrong

For an IFO, Go parses the VOB files of the title set with `dvdExtras` (`dvd.go:254`). MPEG-PS then writes each metadata field twice in text, as the raw value and as the value with `dB`. It also writes `bsid`, `acmod`, `lfeon`, `dialnorm_Count` and the statistics in text, and `*_String` keys in JSON (`mpeg_ps.go:1223-1300`, `mpeg_ps.go:1465-1600`).

### Proof

`dvd-01` (IFO) on the host. Official JSON has no `extra` object on the audio track, and official text has no metadata line. Go JSON has 21 extra keys, among them `dialnorm_String`, `compr_String` and `dialnorm_Count`. Go text has 22 lines, among them `Dialog Normalization: -31` and `Dialog Normalization: -31 dB`.

### Verdict

Covered by #116 and #111. #116 decides if the IFO report reads the VOB files. #111 removes VOB values from the IFO report. The AC-3 metadata must follow the same decision. The research for #75 found the same cause for `ServiceKind` (cause K3).

Witness files: C000006, C000008, C000013.

## M10. Go joins the VOB title set, so it reads other frames

### What goes wrong

Go joins all VOB files of a title set into one input (#116). It reads the head of the first VOB and the tail of the last VOB. Official reads only the named VOB. Thus the first frame and the frames in the statistics differ. This changes the first-frame values (`compr`, `bsid`, `acmod`, `lfeon`) and all statistics.

### Proof

Corpus. 127 of the 130 files of G00133 are a second or later title VOB. In G00133, the first-frame `compr` differs. 260 files of the statistics patterns are title VOBs.

### Verdict

Covered by #116.

Witness files: C000014, C000522, C000523.

## M11. Go reads a different frame window outside VOB title sets

### What goes wrong

The statistics counts also differ on files that are not title VOBs. This occurs on 28 Matroska, 9 BDAV, 1 MPEG-TS and 20 MPEG-PS files. On Matroska, Go counts 0.1 % to 5 % fewer frames than official, for example 1061 against 1101. On BDAV, Go counts about 1.6 times more, for example 1297 against 761. Different frames also give different averages, minimums and maximums.

Go uses its own window rules:

- A frame target for Matroska (`matroska_scan.go:1042`, `:1081`, `:3144`).
- Count offsets for Matroska E-AC-3 (`matroska_ac3_canonical.go:371-385`).
- A cap of 128 frames for MPEG-PS (`mpeg_ps_stream.go:55-56`, `:594-596`).
- A separate statistics window for MPEG-TS and BDAV (`ac3.go:1478-1481`).

None of these rules is in `File_Ac3.cpp`. The official window comes from the container parsers and `ParseSpeed`.

### Proof

Not proven. The synthetic files have no `compr` and no dynamic range word, so they show no count. The research did not find the official frame window in the source.

### Verdict

Not explained. A witness file must show the frame count that official parses. For MPEG-PS outside DVD, #119 changes the cap of 128 frames, and the counts can move with it.

Witness files: C000056, C000084, C000088 (Matroska), C000031, C000033, C000060 (BDAV), C000682, C000688, C000689 (MPEG-PS).

## M12. compr code 0 is 0.00 in Go and -0.00 in official

### What goes wrong

The `compr` code 0 means 0 dB. Official calculates `6.02 + 20 log10(16/32)`, which gives -0.0006, and writes `-0.00` (`Audio/File_Ac3.cpp:1348`, `:1521`). Go returns exactly 0 for code 0 and writes `0.00` (`ac3.go:1800-1807`). Go copies the sign only for `compr_Minimum` (`ac3.go:1645-1647`). master adds the exact 0 for the single `compr` value (master `:1405-1406`), but not for the statistics.

### Verdict

Decision. Go is more correct. The difference is a sign on 2 files. The maintainer can keep `0.00` with one line in `AGENTS.md`, or copy `-0.00` for `compr_Average` and `compr_Maximum`.

Witness files: C001165, C531126.

## Fields that v26.10 also emits

The triage put these patterns in the group "newer MediaInfo emits it", and #83 holds them: G00095 and G00096 (`dmixmod`), G00290 to G00297 (the `xbsi1` levels), G00444 and G00451 (`dialnorm_Count`), and G00656 and G00661 (`adconvtyp`). The `dialnorm_Count` patterns occur only on DVD IFO files, so M9 is their cause.

## Not explained

- G00489, MPEG-PS part (3 files: C002287, C002289, C282830). Go writes `dsurmod` `0`, and official writes none. Official fills `dsurmod` only when the first frame is stereo (`Audio/File_Ac3.cpp:1374`). Go takes `dsurmod` from the first stereo frame (`ac3.go:1339-1342`). A witness file must show if the first frame is not stereo.
- The v26.10 presence list has `cmixlev` and `dmixmod` on C000037, and `roomtyp` on C000052, among the fields that v26.10 has and Go does not have. v23.04 has no pattern for them. A witness file must show the frame values.

## Side findings

- For dual mono (`acmod` 0), `parseAC3Frame` reads the fields in the wrong order. Go reads `dialnorm2` right after `dialnorm`, `compr2` right after `compr`, and `langcod2` right after `langcod` (`ac3.go:325-330`, `:347-367`, `:377-388`). A/52 puts all fields for channel 2 after `audprodie` and `roomtyp`. Thus Go reads `compr`, `mixlevel`, `roomtyp` and `dynrng` of a dual mono stream from the wrong bits. v23.04 and master read the A/52 order (`Audio/File_Ac3.cpp:2301-2324` at v23.04). The corpus has no `acmod` 0 file in this family.
- A raw `.ac3` file (S1) gives `Format: Unknown` in Go. Official parses it. `AGENTS.md` does not list raw AC-3 as a gap.
- In Go, the AC-3 duration of the S1 VOB is 4 s 96 ms, which is 128 frames. #119 holds this.

## Proposed bug tickets

### M1 and M2: one rule for AC-3 metadata lines in text

Title: "AC-3: write the metadata lines in text with one rule for all containers"

Decisions before the fix:

1. Does text show the AC-3 metadata lines as v23.04 does (no line), or as MediaInfo v26.05 and later do (the `/String` lines)?
2. Does raw text follow the same rule as text?
3. Do the `compr_*` and `dynrng_*` statistics and `dialnorm_Count` stay out of text for both choices?

### M4: mix level values

Title: "AC-3: write the mix level values with the A/52 meanings"

Decisions before the fix:

1. Does `surmixlev` code 2 give `-inf dB`, as MediaInfo v26.05 writes it?
2. Does `roomtyp` code 0 give no field, and does the `xbsi1` level code 7 give `-inf`?
3. Must Matroska and MPEG-PS write the same set of mix level fields?

### M6: dynrng and the channel mode

Title: "MPEG-TS and BDAV: stop hiding dynrng by channel mode"

Decisions before the fix:

1. Which BDAV or MPEG-TS sample with AC-3 stereo and a real `dynrng` value proves the fix?

### M7: dialnorm_Maximum in JSON

Title: "AC-3: leave dialnorm_Maximum out of JSON"

Decisions before the fix:

1. Does JSON leave out `dialnorm_Maximum` for all containers, as both MediaInfo versions do?

## Questions for the contributor

1. M11: on C000056 and C000031, how many AC-3 frames does official parse? `mediainfo --Details=1` shows each frame.
2. G00489: on C002287, is the first AC-3 frame stereo?
3. C000037 and C000052: what are `cmixlev`, `dmixmod` and `roomtyp` in the first AC-3 frame?
4. M6: is there a BDAV or MPEG-TS file with AC-3 stereo and a `dynrng` value?
