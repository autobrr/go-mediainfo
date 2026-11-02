# Plan 02: Tags and titles

[Initiative index](README.md) · Stable plan ID 02

## Goal and scope

Unify typed metadata ingestion, scope/target routing, title selection and General display. Include numeric MP4 mdta/ilst keys, Android metadata, BMP cover recognition, ©nam trimming, handler eligibility, FLAC disc tags, Matroska non-track tag routing and mapped text fields.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

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

- [ ] Route typed tags to the correct General/track target; preserve ordered values and unknown metadata.
- [ ] Keep parse-time handler eligibility separate from final fill-if-empty Movie/Track selection.
- [ ] Implement the settled bounded BMP exception, mapped FLAC disc fields and Matroska display mappings without chapter-target expansion.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s field identities and display interfaces.

**Coordination and exclusions:** [Plan 04](plan-04.md) owns encoder provenance; chapter-target output is excluded from #30.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Exercise raw numeric mdta/ilst, duplicate/non-track tags, title and handler eligibility, BMP header bounds and malformed metadata; compare tag/title witnesses without publishing their values.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
