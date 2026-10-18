# Research: language fields (issue #72)

Date: 2026-10-18. Parent map: #65. Rules: ADR 0001 (PR #64) and `AGENTS.md`.

This note answers one question. Which language fields does go-mediainfo get wrong, and which root causes produce the 10 language patterns of the 2026-10-17 corpus run? upbrr reads the audio and text language from the JSON output, and it pastes the text output into tracker uploads. Each cause gets a verdict: bug, deliberate, decision, or out of scope.

In the pattern lists, "official A, Go B" gives the official v23.04 value first and the go-mediainfo value second.

## Method

- go-mediainfo: `main` at `6a37300`. Line numbers below refer to this commit.
- MediaInfoLib: tag `v23.04`, read through the GitHub API. Line numbers below refer to `v23.04`, except where the text says "v26.05". The local checkout is v26.05 (`MediaInfo_Config.cpp:141`). It stands in for newer MediaInfo.
- Media host: official `mediainfo` reports `MediaInfoLib - v23.04`. The JSON command was `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` against `go-mediainfo --output=JSON --language=raw`. The text command was `mediainfo --ParseSpeed=0.5` against `go-mediainfo`.
- Real files: parity samples `dvd-03` and `ts-01`. `ts-01` has no caption track and no Menu track, so it proves nothing for this note. No other file from the media library was read.
- Synthetic files: S1 to S7. `ffmpeg` made each one from `testsrc` and `sine`. A small script changed the `mdhd` language field in S3 and S5. The table at the end lists them.
- Corpus data: the pattern pages and the per-file pages of the corpus archive. A per-file page shows at most four value pairs for each pattern, so the counts below are lower limits. The witness C-ids come from these pages. Only the contributor can open the files.

## Summary

| Cause | Patterns | Verdict |
|---|---|---|
| L1 The language name table is not the MediaInfo table | G00147 (Matroska, 83 files), G00578 (Matroska, 5 files), G00885 | Bug, with one decision for `fil` |
| L2 Tags with subtags get a different text form | G00147 (Matroska, 13 files) | Bug |
| L3 The JSON code is re-cased, and `scc` is not mapped | G00147 (Matroska, 2 files) | Bug for `scc`, decision for the case |
| L4 MP4 ignores QuickTime language codes below 0x400 | G00092, G00093 (MP4, 217 files) | Bug for QuickTime files, decision for code 0 in ISO files |
| L5 The MP4 chapter Menu has no language | G01444 | Bug |
| L6 MPEG-TS text output shows codes, not names | G00147 (MPEG-TS, 1 file), G01442 | Bug |
| L7 The DVD IFO track list follows the VOB, not the IFO | G00147 (DVD, 2 files), G00578 (DVD), G00093 (DVD), G01443, G01446, G01447 | Bug |
| L8 The DVD IFO language `0xFFFF` is not cleared | G01445 | Bug |

No open bug ticket covers any of these causes. Issue #58 covers the Ogg `LANGUAGE` comment, but no pattern in this ticket comes from Ogg.

## How official MediaInfo builds a language

Every parser stores the code that it finds in `Language`. A shared step then makes the JSON value and the text value (`File__Analyze_Streams.cpp:1407-1534`):

1. It splits the value at ` / ` into a list, and each item at `-` into subtags.
2. It sets the first subtag to lowercase. It removes the item if the first subtag is `mis`, `und`, `???` or blank (lines 1428-1439).
3. It replaces a 3-letter first subtag with its 2-letter code from the `Iso639_1` table, if one exists (lines 1444-1445). It keeps all other subtags exactly as the file wrote them. The JSON value is this result.
4. For the text value of a tag with exactly 2 subtags, it first looks up the whole tag in the name table, for example `Language_zh-CN` (lines 1467-1470). The lookup is case-sensitive.
5. Else it looks up the first subtag. If no name exists, it uses the code itself (lines 1484-1486). Then it adds `(Region)` for exactly 2 subtags, or `-sub-sub` for 3 or more (lines 1487-1503).

The name table is `MediaInfo_Config_DefaultLanguage` in `MediaInfo_Config_Automatic.cpp`. The `Iso639_1` table starts at line 6091 of the same file.

Go does the same work in `normalizeLanguageCode` and `formatLanguage` (`language.go:423-527`), with its own tables (`language.go:8-421`).

## L1: The language name table is not the MediaInfo table

Patterns: G00147, Matroska part: official `fil`, Go `Filipino` (70 files). Official `fil (PH)`, Go `Filipino (PH)` (12 files). Official `Khmer (KH)`, Go `Central Khmer (KH)` (1 file). G00578, Matroska part: official `zxx`, Go `Silent` (3 files), and official `fil`, Go `Filipino` (2 files). G00885: official `zxx`, Go `Silent` (2 files). All are text output only. The JSON values are equal.

Witnesses: C000111, C000156, C000201 (`fil`), C005128, C005129 (`fil-PH`), C000075 (`km`), C272272, C655306, C009711, C633403 (`zxx`).

Official behavior. v23.04 has no name for `fil` and no name for `zxx`, so the text shows the code (`File__Analyze_Streams.cpp:1484-1486`). The name for `km` is `Khmer` (`MediaInfo_Config_Automatic.cpp:837`). v26.05 adds `Language_fil;Filipino` (v26.05 `MediaInfo_Config_Automatic.cpp:812`) and `Language_zxx;No linguistic content or not applicable` (v26.05 line 973).

Go behavior. Go has its own name table. It has `Filipino` for `fil` (`language.go:55`, `language.go:416`), `Silent` for `zxx` (`language.go:420`) and `Central Khmer` for `km` (`language.go:93`). A script compared the Go table with the v23.04 table. These names differ:

- Go has a different name for `bh`, `cu`, `fy`, `ia`, `km`, `li`, `nd`, `nr`, `nv`, `ny`, `os`, `ps`, `rm` and `st`. For example, v23.04 has `Slave` for `cu`, and Go has `Church Slavic`.
- Go has a name that v23.04 does not have for `fil`, `mis`, `tlh` and `zxx`.
- v23.04 has a name that Go does not have for `mo` (`Moldavian`) and `smi` (`Sami`).

The v26.05 table has the same differences, except that it also has `fil`, `zxx` and `und`.

Evidence. S6 is a Matroska file with one subtitle track for each test tag. Official text shows `zxx`, `fil` and `Khmer (KH)`. Go text shows `Silent`, `Filipino` and `Central Khmer (KH)`.

Verdict: bug. The text output goes into tracker uploads, and no MediaInfo version has these names. `Silent` for `zxx` matches neither v23.04 nor v26.05. The one exception is `fil`. Go matches v26.05 there, so it needs a decision under ADR 0001 rule 2.

## L2: Tags with subtags get a different text form

Patterns: G00147, Matroska part, text output only:

- Official `khk (Cyrl)`, Go `khk-Cyrl` (4 files).
- Official `Cantonese`, Go `Chinese (yue)` (3 files).
- Official `Chinese-Hant-HK`, Go `Chinese (Hant-HK)` (3 files).
- Official `Cantonese-Hant-HK`, Go `Cantonese (Hant-HK)` (2 files).
- Official `Chinese (China)`, Go `Chinese (CN)` (1 file).

Witnesses: C000399, C000403 (`khk-Cyrl`), C003255, C005834 (`zh-yue`), C005844, C008888 (`zh-Hant-HK`), C000079, C000081 (`yue-Hant-HK`), C000253 (`zh-CN`).

Official behavior. Three rules from the shared step apply:

1. The shared step looks up a tag with exactly 2 subtags as a whole first. v23.04 has 16 such names (`MediaInfo_Config_Automatic.cpp:788-789`, `946-954`). Examples are `Chinese (China)` for `zh-CN`, `Cantonese` for `zh-yue`, `English (United States)` for `en-us`, `Portuguese (Brazil)` for `pt-br` and `Flemish` for `nl-be`.
2. A tag with 3 or more subtags gets the form `Name-sub-sub` (`File__Analyze_Streams.cpp:1498-1503`).
3. An unknown first subtag keeps its code, and the region still gets the brackets, for example `khk (Cyrl)` (lines 1484-1497).

Go behavior. Go has only 4 whole-tag names: `es-419`, `zh-Hans`, `zh-Hant` and `zh-TW` (`language.go:198-203`). Go always writes `Name (sub-sub)` (`language.go:523-524`). If the first subtag has no name, Go returns the input unchanged (`language.go:519-521`). Go looks up the whole tag after it changes the case (`language.go:509-514`). Official looks it up in the case of the file. So official gives `English (United States)` for `en-us` but `English (US)` for `en-US`.

Evidence. S6. Official text: `khk (Cyrl)`, `Cantonese`, `Chinese-Hant-HK`, `Cantonese-Hant-HK`, `Chinese (China)`, `English (United States)`. Go text: `khk-Cyrl`, `Chinese (yue)`, `Chinese (Hant-HK)`, `Cantonese (Hant-HK)`, `Chinese (CN)`, `English (US)`.

Verdict: bug. These are display rules, not facts, and no MediaInfo version uses the Go form.

## L3: The JSON code is re-cased, and `scc` is not mapped

Patterns: G00147, Matroska part: JSON official `en-us`, Go `en-US` (1 file). JSON official `sr`, Go `scc` (1 file).

Witnesses: C009448 (`en-us`), C633413 (`scc`).

Official behavior. The JSON value keeps every subtag after the first as the file wrote it (`File__Analyze_Streams.cpp:1431-1454`). The `Iso639_1` table maps `scc` to `sr` and `mol` to `mo` (`MediaInfo_Config_Automatic.cpp:6267`).

Go behavior. `normalizeLanguageCode` writes the script subtag in title case and the region in upper case (`language.go:455`, `language.go:459`). `languageMap3To2` has no entry for `scc` or `mol`. It has an entry for `bod`, which v23.04 does not map. So v23.04 shows `bod`, and Go shows `bo`.

Evidence. S6. Official JSON: `en-us` and `sr`. Go JSON: `en-US` and `scc`. The Go text output then shows `scc` where official shows `Serbian`.

Verdict: `scc` and `mol` are a bug. The case change needs a decision. BCP 47 (RFC 5646, section 2.1.1) calls `en-US` the canonical case, so Go can be more correct. But upbrr reads this JSON field, and official keeps the case of the file.

## L4: MP4 ignores QuickTime language codes below 0x400

Patterns: G00092 (Video, 219 files) and G00093 (Audio, 217 MP4 files). Official shows `en`, and Go shows no language. 211 of the 219 files come from Android devices, with the brand `mp42`. 3 files have the brand `qt  `.

Witnesses: C527493, C521887, C521826, C521904, C525432 (Android, from the ticket). C608641, C610725, C610929 (QuickTime brand).

Official behavior. `File_Mpeg4::Language_Get` (`File_Mpeg4.cpp:2905-2918`) returns no language for `0x7FFF` and `0xFFFF`. For a value below `0x400`, it uses the Macintosh language table (`Mpeg4_Language_Apple`, `File_Mpeg4.cpp:179`). In that table, 0 is English and 1 is French. The rule applies to every brand. The `mdhd` handler fills the result (`File_Mpeg4_Elements.cpp:4342-4343`). v26.05 has the same code.

Go behavior. `decodeMP4Language` (`mp4_meta.go:88-98`) reads only the packed ISO 639-2/T form. For a value below `0x400`, the first letter is not in the range `a` to `z`, so Go returns no language.

Evidence:

- S1 is a `.mov` file. `ffmpeg` writes Macintosh codes in QuickTime files: `0x0000` for the English video and `0x0001` for the French audio. Official shows `en` and `fr`. Go shows nothing.
- S3 is an ISO MP4 with the brand `mp42`. Both `mdhd` languages are `0x0000`, as Android writes them. Official shows `en` for video and audio. Go shows nothing.
- S2 has packed codes (`eng`, `fra`). Both tools agree.

The Android `MPEG4Writer` starts the language code at 0. It changes the code only if the app gives a language. So 0 in these files means "not set", not "English".

Verdict: bug for QuickTime files, because the QuickTime format defines codes below `0x400` as Macintosh codes. For code 0 in an ISO file, Go can be more correct, because ISO/IEC 14496-12 defines only the packed form. That part needs a decision. It holds 216 of the 219 files.

## L5: The MP4 chapter Menu has no language

Pattern: G01444 (1 file). Official Menu shows `en`, and Go shows no language. Witness: C608220.

Official behavior. A chapter text track (`tref/chap`) becomes a Menu track. Its `mdhd` language fills the Menu `Language` as on any other track (`File_Mpeg4_Elements.cpp:4342-4343`).

Go behavior. `buildMP4ChapterMenu` (`mp4_probe.go:317` and on) writes the ID, Format, Codec ID, Duration, dates and `Menu For`. It never writes `Language`, although the track has the code in `LanguageCode`.

Evidence:

- S4 has a chapter track that `ffmpeg` wrote with the code `0x0000`. Official Menu #1 shows `Language : English`. Go Menu #1 has no language.
- S5 is S4 with every `mdhd` language set to `eng` (`0x15C7`). Go still writes no Menu language. So L5 is a separate cause from L4.

Verdict: bug.

## L6: MPEG-TS text output shows codes, not names

Patterns: G00147, MPEG-TS part: Text official `English`, Go `en` (1 file, text output). G01442: Menu official ` / English`, Go ` / en` (1 file, text output). Witness: C000234.

Official behavior. The caption Text track and the Menu track fill `Language` as all other tracks do. The shared step translates each item of the list (`File__Analyze_Streams.cpp:1418-1534`).

Go behavior. Two places write the code into the text field:

- Captions: `mpeg_ts_captions.go:222-224` writes `descriptor.Language`, which is a code, as the text value.
- Menu: `mpeg_ts.go:3111-3118` collects codes, and `mpeg_ts.go:3152-3153` joins them into the text value. Line 3184 writes the same list to JSON, which is correct.

The audio and video tracks use `formatLanguage` (`mpeg_ts.go:2803`), so they are correct.

Evidence. S7 is an MPEG-TS file with MPEG-2 video and English AC-3 audio. Official Menu text: `Language : / English`. Go Menu text: `Language : / en`. The JSON value is ` / en` in both. No synthetic file had a caption service descriptor. So the proof for the caption part is the code and C000234 only.

Verdict: bug.

## L7: The DVD IFO track list follows the VOB, not the IFO

Patterns: G00147, DVD part (2 files), G00578, DVD part (1 file), G00093, DVD part (1 file), G01443, G01446, G01447. Witnesses: C000013, C000022, C000066.

Official behavior. Official reads only the IFO. It makes one Audio track for each entry of the IFO audio attribute table, and one Text track for each entry of the subpicture attribute table, in table order (`File_Dvdv.cpp:660-683`, `708-719`). Track number N gets the language of entry N. The first title PGC then gives the ID of track N from stream control N (`File_Dvdv.cpp:1270-1284` for audio, `1311-1318` for text). The text ID is `0x20` plus the wide-screen stream number.

Go behavior. For an IFO in a `VIDEO_TS` folder, Go also reads the VOB files (`dvd.go:152`, `dvd.go:242-258`). The tracks then come from the VOB payload:

- `mergeDVDDeclaredStreams` keeps every payload stream. It adds the IFO entries that the VOB scan did not find (`dvd.go:2900-2970`).
- `sortDVDPayloadStreams` sorts audio by duration, longest first, and text by substream ID (`dvd.go:2204-2233`).
- `overlayDVDDeclaredLanguages` finds the language by payload ID (`dvd.go:2793-2834`). On a 16:9 title, one subpicture entry owns the wide-screen ID and the letterbox ID (`dvd.go:1189-1196`). So both payload substreams get the same language.
- Go collects the audio match across all title PGCs. If two PGCs disagree, the match gives no language (`dvd.go:2850-2852`).

The corpus pages show the effect:

- C000022 is a 16:9 title. Official has 5 Text tracks with the IDs `0x20`, `0x22`, `0x24`, `0x26`, `0x28` and the languages `en`, `fr`, `es`, `fr`, `es`. Go has 6 Text tracks with the IDs `189-32`, `189-33`, `189-34`, `189-35` and more. Go Text #2 is the letterbox copy of entry 1. So it shows `en` and `Normal`, where official shows `fr` and no extra value (G00147, G01447). Go Text #4 is the letterbox copy of entry 2, so it loses `Large` (G01446).
- C000013: the official audio IDs are `0x80` to `0x83`, with `en`, `fr`, `it`, `en`. Go puts `189-131` first, because it is the longest, and gives it no language. Then come `189-128`, `189-129` and `189-130`, with `en`, `fr`, `it`. So each language moves down one track (G00093, G00578).
- C000066 shows the same move on Text, with one track that has no language (G01443, G00147).

Evidence. `dvd-03` is a 4:3 title with 2 audio entries and 4 subpicture entries. Each entry has one payload stream. On this sample, Go and official give the same languages in the same order, with different IDs. So the cause shows only when the VOB has more streams than the IFO, or when the duration order differs from the table order. No sample on the media host has that shape. The corpus pages are the only proof of the failure.

Verdict: bug. The IFO report lists the IFO table. upbrr reads `Language` by track, so a moved language is a wrong value. Issue #111 covers other VOB values in the IFO report (frame rate, Menu values), but not the track list or the language.

## L8: The DVD IFO language 0xFFFF is not cleared

Pattern: G01445 (1 file). Official shows no language. Go shows the two bytes `0xFF 0xFF`. The text output shows them as `\xFF\xFF`, and the JSON output has a string that is not valid UTF-8. Witness: C001811.

Official behavior. If the first language byte is `0x80` or more, official clears the language (`File_Dvdv.cpp:622-623` for audio, `701-702` for text).

Go behavior. `dvdTrimLang` removes only NUL bytes and spaces (`dvd.go:1057-1059`). `parseDVDAudioAttrs` and `parseDVDSubpicAttrs` pass the result to the language functions (`dvd.go:906-918`, `dvd.go:942-946`). `normalizeLanguageCode` returns input that it cannot parse unchanged (`language.go:438`).

Evidence: the code and C001811. No sample on the media host has this value.

Verdict: bug.

## Side findings

These are outside the patterns of this ticket.

1. Go removes only `und`. Official also removes `mis`, `???` and blank codes (`File__Analyze_Streams.cpp:1433-1438`). Go shows `Uncoded languages` for `mis` (`language.go:417`).
2. Go does not read the MP4 `elng` box. Official reads it, and it has priority over `mdhd` (`File_Mpeg4_Elements.cpp:4043-4058`, `4342`).
3. On S4, official Video and Audio have `Menus : 3`, and Go has no `Menus` line. Pattern G00886 (`Menus`, Video, missing) matches this. Official Menu #1 has `Menu For : 1,2`, and Go has `Menu For : 2`.
4. On `dvd-03`, official gives every Text track the ID `32 (0x20)`. It always uses the wide-screen number, and that number is 0 on a 4:3 title (`File_Dvdv.cpp:1316`). Go gives `189-32` to `189-35`. Pattern G00150 (Text ID, 94 files) covers this ID difference.

## Synthetic files

| ID | File | Content |
|---|---|---|
| S1 | `mac-lang.mov` | H.264 and AAC, `-metadata:s:v:0 language=eng -metadata:s:a:0 language=fra`. `mdhd` codes `0x0000` and `0x0001` |
| S2 | `iso-lang.mp4` | Same streams and tags as S1, in MP4. Packed codes `eng` and `fra` |
| S3 | `android-zero.mp4` | MP4 with the brand `mp42`, both `mdhd` codes set to `0x0000` |
| S4 | `chap.mp4` | H.264 and AAC with two chapters. `ffmpeg` wrote a chapter text track with code `0x0000` |
| S5 | `chap-eng.mp4` | S4 with all `mdhd` codes set to `0x15C7` (`eng`). Run with Go only |
| S6 | `tags.mkv` | H.264 with `zxx`, and 12 SubRip tracks with the Matroska `Language` values `fil`, `fil-PH`, `khk-Cyrl`, `zh-yue`, `zh-Hant-HK`, `yue-Hant-HK`, `km-KH`, `zh-CN`, `en-us`, `scc`, `zxx`, `srp` |
| S7 | `lang.ts` | MPEG-TS with MPEG-2 video and AC-3 audio, `language=eng` |
