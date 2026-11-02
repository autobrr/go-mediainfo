# Plan 01: Report schema and rendering

[Initiative index](README.md) · Stable plan ID 01

## Goal and scope

One versioned report-field registry, canonical identities, aliases, ordering, visibility and scalar formatting across friendly/raw/JSON output. Include each listed leaf’s actual binding: Menu names/order, delay visibility, statistics deduplication, format/CodecID labels, audio/FPS rows, byte grouping and source-ID display. #50 closes with #107, not a separate FLAC patch. This includes the small producer corrections explicitly required by those tickets, such as present BDAV General ID zero; changing Format display names must first stop count/rate logic from relying on those names.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

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

- [x] Verify every issue's actual field binding, not only the registry. Preserve canonical numeric facts and explicit dynamic-extra options.
- [x] Use selected version/kind tables for identity, aliases, order and visibility. Keep producer tag/fill order and allowed duplicate multiplicity.
- [x] Preserve public Report rebuilding, mutation, round-trip and idempotent behavior. #58 serial/formatting is a partial contribution only.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** Foundation; no earlier plan contract is required.

**Coordination and exclusions:** Later parser plans consume the table interfaces; their producer eligibility work is separate.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Retain ordering/duplicates/numeric-type and public mutation/rebuild regressions. Revalidate affected issue witnesses and shared projection controls after review changes; distinguish each tested tree and subset.

[PR #256](https://github.com/autobrr/go-mediainfo/pull/256) records the acceptance and validation limits. The [maintainer response](https://github.com/autobrr/go-mediainfo/pull/256#issuecomment-6572958573) covers the 84-witness sweep on its identified earlier tree; the [final follow-up](https://github.com/autobrr/go-mediainfo/pull/256#discussion_r4440686214) covers 32 actual witnesses in 12 configurations, all 384 before/after profiles byte-identical. [Exact-head CI](https://github.com/autobrr/go-mediainfo/actions/runs/41419735655) passed. These are separate tested revisions, not a final-head full-corpus run. Retain disclosed WAV/Go-Other/exact-TS-JOC real-witness limits, synthetic-only equal-sampling trigger and existing output-dialect/producer gaps. Raw captures and private mappings are not repository artifacts.

## Implementation and outcome history

Delivered in open [PR #256](https://github.com/autobrr/go-mediainfo/pull/256) at [55b7a52](https://github.com/autobrr/go-mediainfo/commit/55b7a523317384d5d8d608f731605dd3c4aff402); not merged or released at this reconciliation. Its 25 closing targets remain issue-tracker state, not automatic closures here. #58 is partial; #81 remains unresolved.

2026-11-02: reconciled delivered-but-unmerged Plan 01, maintainer follow-ups and final affected-subset evidence; retained later-plan producer boundaries.
