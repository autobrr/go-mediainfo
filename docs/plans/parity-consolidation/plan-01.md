# Plan 01: Report schema and rendering

[Initiative index](README.md) · Stable plan ID 01

## Goal and scope

Use one versioned registry to identify, alias, order, show and format report fields. Apply it to friendly/raw text and JSON. Cover every listed issue's binding: Menu names/order, delay visibility, statistics deduplication, format/CodecID descriptions, audio/FPS rows, byte grouping and source-ID display.

Close #50 with #107 rather than a separate FLAC patch. Include the small producer corrections required by these issues, such as present BDAV General ID zero. Before changing Format display names, stop count/rate logic from depending on those names.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#50](https://github.com/autobrr/go-mediainfo/issues/50) | FLAC Audio tracks missing framerate (SPF) information | [Discussion](https://github.com/autobrr/go-mediainfo/issues/50#issuecomment-6523106447) |
| [#54](https://github.com/autobrr/go-mediainfo/issues/54) | Limit the raw-text projection to labels and order | [Discussion](https://github.com/autobrr/go-mediainfo/issues/54#issuecomment-6522467303) |
| [#102](https://github.com/autobrr/go-mediainfo/issues/102) | print the minimum and maximum frame rate with official labels | [Discussion](https://github.com/autobrr/go-mediainfo/issues/102#issuecomment-6521725815) |
| [#107](https://github.com/autobrr/go-mediainfo/issues/107) | show the frame rate line in text output | [Discussion](https://github.com/autobrr/go-mediainfo/issues/107#issuecomment-6521563482) |
| [#108](https://github.com/autobrr/go-mediainfo/issues/108) | add the thousands separator | [Discussion](https://github.com/autobrr/go-mediainfo/issues/108#issuecomment-6521546525) |
| [#114](https://github.com/autobrr/go-mediainfo/issues/114) | add the FPS unit to the video frame rate text | [Discussion](https://github.com/autobrr/go-mediainfo/issues/114#issuecomment-6521410804) |
| [#117](https://github.com/autobrr/go-mediainfo/issues/117) | show the seconds in the duration text | [Discussion](https://github.com/autobrr/go-mediainfo/issues/117#issuecomment-6521393495) |
| [#125](https://github.com/autobrr/go-mediainfo/issues/125) | show the maximum bit rate and the Vorbis stream size | [Discussion](https://github.com/autobrr/go-mediainfo/issues/125#issuecomment-6521166807) |
| [#136](https://github.com/autobrr/go-mediainfo/issues/136) | use the official labels and values for scan order, picture structure, store method, time code source and GOP | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/136#issuecomment-6252615440), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/136#issuecomment-6521002906) |
| [#149](https://github.com/autobrr/go-mediainfo/issues/149) | take `Format/Info` and the text Format name from the official Format table | [Discussion](https://github.com/autobrr/go-mediainfo/issues/149#issuecomment-6520360630) |
| [#150](https://github.com/autobrr/go-mediainfo/issues/150) | show `Codec ID/Info` and `Codec ID/Hint` from the CodecID tables | [Discussion](https://github.com/autobrr/go-mediainfo/issues/150#issuecomment-6520316766) |
| [#156](https://github.com/autobrr/go-mediainfo/issues/156) | write "1 frame" for one reference frame | [Discussion](https://github.com/autobrr/go-mediainfo/issues/156#issuecomment-6520079917) |
| [#170](https://github.com/autobrr/go-mediainfo/issues/170) | hide the video Delay and Original delay lines | [Discussion](https://github.com/autobrr/go-mediainfo/issues/170#issuecomment-6519368542) |
| [#172](https://github.com/autobrr/go-mediainfo/issues/172) | show `Video_Delay/String` once and hide `Source_Delay` | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/172#issuecomment-6252970410), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/172#issuecomment-6519312928) |
| [#207](https://github.com/autobrr/go-mediainfo/issues/207) | write "ID in the original source medium" with the hex value | [Discussion](https://github.com/autobrr/go-mediainfo/issues/207#issuecomment-6518241378) |
| [#212](https://github.com/autobrr/go-mediainfo/issues/212) | translate `MuxingMode_PackedBitstream` | [Discussion](https://github.com/autobrr/go-mediainfo/issues/212#issuecomment-6517812740) |
| [#213](https://github.com/autobrr/go-mediainfo/issues/213) | write the General ID 0 instead of the program number | [Discussion](https://github.com/autobrr/go-mediainfo/issues/213#issuecomment-6517781717) |
| [#218](https://github.com/autobrr/go-mediainfo/issues/218) | translate field names that are not in the Menu table | [Discussion](https://github.com/autobrr/go-mediainfo/issues/218#issuecomment-6517669462) |
| [#221](https://github.com/autobrr/go-mediainfo/issues/221) | order Menu fields by the official Menu table | [Discussion](https://github.com/autobrr/go-mediainfo/issues/221#issuecomment-6517597021) |
| [#222](https://github.com/autobrr/go-mediainfo/issues/222) | write the Menu service fields with their field names | [Discussion](https://github.com/autobrr/go-mediainfo/issues/222#issuecomment-6517580608) |
| [#224](https://github.com/autobrr/go-mediainfo/issues/224) | take the field order from the MediaInfo stream tables | [Discussion](https://github.com/autobrr/go-mediainfo/issues/224#issuecomment-6517525867) |
| [#225](https://github.com/autobrr/go-mediainfo/issues/225) | put each standard key in the MediaInfo stream table order | [Discussion](https://github.com/autobrr/go-mediainfo/issues/225#issuecomment-6517497154) |
| [#226](https://github.com/autobrr/go-mediainfo/issues/226) | Keep extra fields in the parser fill order | [Discussion](https://github.com/autobrr/go-mediainfo/issues/226#issuecomment-6517463665) |
| [#227](https://github.com/autobrr/go-mediainfo/issues/227) | show "Statistics Tags Issue" once, with the official label | [Discussion](https://github.com/autobrr/go-mediainfo/issues/227#issuecomment-6517411044) |
| [#246](https://github.com/autobrr/go-mediainfo/issues/246) | Text File size has no thousands separator from 1000 to 1023 MiB | [Discussion](https://github.com/autobrr/go-mediainfo/issues/246#issuecomment-6523826843) |

## Acceptance criteria

These checked items record #256's accepted delivery and tested cases, not exhaustive producer coverage. The remaining MP3 producer-order case is tracked below in #260.

- [x] Verify every issue's actual field binding, not only the registry. Preserve canonical numeric facts and explicit dynamic-extra options.
- [x] Use selected version/kind tables for identity, aliases, order and visibility. Keep producer tag/fill order and allowed duplicate multiplicity.
- [x] Preserve public Report rebuilding, mutation, round-trip and idempotent behavior. #58 serial/formatting is a partial contribution only.

## Dependencies and non-goals

**Required contracts:** Foundation; no earlier plan contract is required.

**Coordination and exclusions:** Later parser plans consume the table interfaces; their producer eligibility work is separate.

## Validation and evidence

[Shared validation requirements](README.md#validation-and-evidence). Retain ordering/duplicates/numeric-type and public mutation/rebuild regressions. Revalidate affected issue witnesses and shared projection controls after review changes; distinguish each tested tree and subset.

[PR #256](https://github.com/autobrr/go-mediainfo/pull/256) records the tested revisions, checks and limitations. Keep these historical stages separate:

- The [earlier maintainer response](https://github.com/autobrr/go-mediainfo/pull/256#issuecomment-6572958573) covers an 84-file witness/control sweep on its identified tree. The [55b7a52 follow-up](https://github.com/autobrr/go-mediainfo/pull/256#discussion_r4440686214) covers 32 affected witnesses and shared controls in 12 configurations, with all 384 before/after profiles byte-identical.
- The later [b30a90c correction](https://github.com/autobrr/go-mediainfo/commit/b30a90c8504e864a8770effc2fe466478152ddcb) records 24 original witnesses/controls in 12 configurations plus two AVI controls in their six available frozen-reference configurations, for 300 captures per Go build. The earlier r2 tree separately covered 84 originals in 12 configurations. Neither is a sweep of the final merged head.
- The PR's maintainer follow-up for [4caa6fa](https://github.com/autobrr/go-mediainfo/commit/4caa6fa7670074c4037ddf80d59d095d430e7312) through [7f6d5db](https://github.com/autobrr/go-mediainfo/commit/7f6d5db379e2b608e880f906f042e5369111bc95) reports 57 real-file checks for Theora presentation. It does not establish that these files belong to the October corpus.
- Final-head [CI at 5e61a5a](https://github.com/autobrr/go-mediainfo/actions/runs/43118228064) passed. The final commits preserve compound-format descriptions after unrelated public edits, raw stream-size display without a known total file size, and caller edits/deletion of Format profile. Their regression tests and CI are distinct from the earlier corpus captures; no complete merged-head corpus sweep is claimed.

Retain the earlier sweep's disclosed WAV/Go-Other/exact-TS-JOC real-witness limits, synthetic-only triggers and existing output-dialect/producer gaps. Later checks do not retroactively fill those coverage gaps. Raw captures and private mappings are not repository artifacts.

## Implementation and outcome history

[PR #256](https://github.com/autobrr/go-mediainfo/pull/256) merged on 2026-11-11 at [283023f](https://github.com/autobrr/go-mediainfo/commit/283023f3b755da7adb2632c655f01f2f4d63d1c2), incorporating final head [5e61a5a](https://github.com/autobrr/go-mediainfo/commit/5e61a5a97f1c49eebab91f4cbd9608c26b3ce60e). Reuse that implementation as the foundation for later plans. Merge is not evidence of release inclusion or complete native parity.

[#260](https://github.com/autobrr/go-mediainfo/issues/260) remains an open MP3 ID3 producer-order follow-up to #226's accepted requirement. #256 did not change the map-to-extra producer path. [Retained music-corpus evidence](https://github.com/autobrr/go-mediainfo/issues/260#issuecomment-6811500432) verifies 122 reference repeat sets were stable and 121 Go sets varied at merged commit 283023f; this is verification of existing captures, not a new candidate run or fix. Publisher placement is a separate difference. Keep #260 visible without adding it to the original 25 closing targets, reopening #226 or inferring new implementation authority from this record.

The #58 serial/display and near-minute-formatting contribution is delivered; Ogg language and MP4 last/source-last-frame timing remain with Plans 03 and 10. #58 remains partial and #81 remains unresolved.

2026-11-02: reconciled delivered-but-unmerged Plan 01, maintainer follow-ups and affected-subset evidence; retained later-plan producer boundaries.

2026-11-12: recorded #256's verified merge and later follow-ups, separated historical validation from exact-head CI, and retained the open #260 producer gap and partial #58 scope.

2026-11-12: the separate [draft PR #262](https://github.com/autobrr/go-mediainfo/pull/262), head `6605d5b55c32bf4e80d3502d346af3592218c765`, addresses #260's ID3 producer order under the owner's one-time open-PR-cap exception. Its fresh targeted run covers 175 files (160 music, 15 October), 6,248 captures, and 150 applicable source-order checks; repeated candidate/reference outputs were stable while 120 of 122 baseline MP3 witnesses varied. Preserve its URL-collision and Publisher-placement limits and exact tested-source distinction. This is an open follow-up, not part of merged #256 or the original 25-target allocation. Its historical access gaps do not imply current unavailability: subsequent Plan 02 checks analyzed 3,157 October entries, leaving 16 missing. No full final-head corpus sweep is claimed for either PR.
