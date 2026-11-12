# Plan 02: Tags and titles

[Initiative index](README.md) · Stable plan ID 02

## Goal and scope

Unify typed metadata ingestion, scope/target routing, title selection and General display. Include numeric MP4 mdta/ilst keys, Android metadata, BMP cover recognition, ©nam trimming, handler eligibility, FLAC disc tags, Matroska non-track tag routing and mapped text fields.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#24](https://github.com/autobrr/go-mediainfo/issues/24) | recognize BMP cover art instead of "Unknown kind of value!" | [Discussion](https://github.com/autobrr/go-mediainfo/issues/24#issuecomment-5175388088) |
| [#30](https://github.com/autobrr/go-mediainfo/issues/30) | chapter/edition/attachment-targeted tags are dropped | [Issue body](https://github.com/autobrr/go-mediainfo/issues/30) |
| [#106](https://github.com/autobrr/go-mediainfo/issues/106) | read `mdta` metadata keys | [Discussion](https://github.com/autobrr/go-mediainfo/issues/106#issuecomment-6521605553) |
| [#145](https://github.com/autobrr/go-mediainfo/issues/145) | Pick `Movie` or `Track` with one shared rule, and show its label in text | [Discussion](https://github.com/autobrr/go-mediainfo/issues/145#issuecomment-6520482847) |
| [#146](https://github.com/autobrr/go-mediainfo/issues/146) | show `Movie` or `Track` for General, and drop `Title` | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/146#issuecomment-6520447053), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/146#issuecomment-6520488139) |
| [#147](https://github.com/autobrr/go-mediainfo/issues/147) | drop handler names with the official name test | [Discussion](https://github.com/autobrr/go-mediainfo/issues/147#issuecomment-6520426919) |
| [#148](https://github.com/autobrr/go-mediainfo/issues/148) | show the Vorbis comment fields in text | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/148#issuecomment-6252528524), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/148#issuecomment-6520384744) |
| [#191](https://github.com/autobrr/go-mediainfo/issues/191) | label EncodedBy as "Encoded by" | [Discussion](https://github.com/autobrr/go-mediainfo/issues/191#issuecomment-6518718446) |
| [#231](https://github.com/autobrr/go-mediainfo/issues/231) | trim spaces from the ©nam title | [Discussion](https://github.com/autobrr/go-mediainfo/issues/231#issuecomment-6517354609) |
| [#233](https://github.com/autobrr/go-mediainfo/issues/233) | read the Android version and GPS location metadata keys | [Discussion](https://github.com/autobrr/go-mediainfo/issues/233#issuecomment-6517297066) |
| [#234](https://github.com/autobrr/go-mediainfo/issues/234) | write DISCNUMBER and DISCTOTAL as Part/Position and Part/Position_Total | [Discussion](https://github.com/autobrr/go-mediainfo/issues/234#issuecomment-6517258524) |
| [#235](https://github.com/autobrr/go-mediainfo/issues/235) | show the mapped tag fields, and fix the date and the Writing frontend label | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/235#issuecomment-6291781378), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/235#issuecomment-6517237040) |

## Acceptance criteria

- [x] Route typed tags to the correct General/track target; preserve ordered values and unknown metadata.
- [x] Keep parse-time handler eligibility separate from final fill-if-empty Movie/Track selection.
- [x] Implement the settled bounded BMP exception, mapped FLAC disc fields and Matroska display mappings without per-chapter scoping/output.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s field identities and display interfaces.

**Coordination and exclusions:** [Plan 04](plan-04.md) owns encoder provenance. Under #30, chapter-, edition- or attachment-targeted tags without a track target still route to General; per-chapter scoping/output is excluded.

## Validation and evidence

[Shared validation requirements](README.md#validation-and-evidence). Exercise raw numeric mdta/ilst, duplicate/non-track tags, title and handler eligibility, BMP header bounds and malformed metadata; compare tag/title witnesses without publishing their values.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.

2026-11-12: [PR #263](https://github.com/autobrr/go-mediainfo/pull/263) implements the complete accepted scope at `a9f27b1ec54642a7757c368438bb7fb476ad8871`, based on merged Plan 01 at `283023f3b755da7adb2632c655f01f2f4d63d1c2`. Checked acceptance records the proposed implementation, not a merge or release. Scoped mdta, final title selection, handler eligibility, BMP bounds, disc fields and Matroska mappings have regressions; exact-name and generated-suffix collisions preserve codec facts and all metadata values.

Validation spans preserved candidate snapshots: 2,638 October originals plus 403 music originals on the broad earlier sweep, 515 recovered October paths on a later snapshot, and the remaining four access-restored originals in subsequent controls. This accounts for 3,157 of 3,173 October entries; 16 remain missing. Seven restored files lack historical hashes and have changed mtimes. Twenty-four music XML failures are baseline-existing. The final source was rerun on 277 originals (41 October controls and 236 music digest witnesses) in text, raw text, JSON and XML, with byte-identical output to the retained prior captures. Two generated playable controls also remained unchanged. Full tests/vet, uncached collision regressions and 71,395 affected-parser fuzz executions passed; independent cumulative review found no remaining material finding. The PR records exact source/executable identities, argv, reference selection and earlier corrective subsets. This is not a full final-head sweep, HTML/CSV recapture or complete native-parity claim. AGENTS records the three accepted deliberate differences; ADR 0001/0002 remain unchanged.
