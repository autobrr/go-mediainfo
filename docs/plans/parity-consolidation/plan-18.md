# Plan 18: FLAC evidence and cadence

[Initiative index](README.md) · Stable plan ID 18

## Goal and scope

Use one FLAC evidence pipeline to distinguish absent masks, explicit zero/nonzero masks and Matroska overrides. Classify CBR from encoded frame sizes. Preserve supported nominal fixed-block cadence without inventing a variable-block cadence.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#56](https://github.com/autobrr/go-mediainfo/issues/56) | Use FLAC defaults only when explicit mask evidence is absent | [Discussion](https://github.com/autobrr/go-mediainfo/issues/56#issuecomment-6522995988) |
| [#57](https://github.com/autobrr/go-mediainfo/issues/57) | report CBR when STREAMINFO frame sizes are equal | [Discussion](https://github.com/autobrr/go-mediainfo/issues/57#issuecomment-6522942242) |
| [#115](https://github.com/autobrr/go-mediainfo/issues/115) | Retain supported nominal fixed-block cadence and exact sampling inventory | [Discussion](https://github.com/autobrr/go-mediainfo/issues/115#issuecomment-6523664516) |
| [#251](https://github.com/autobrr/go-mediainfo/issues/251) | Honor explicit masks and expose original FLAC mask evidence through the agreed library API | [Discussion](https://github.com/autobrr/go-mediainfo/issues/251#issuecomment-6523764324) |

## Acceptance criteria

- [ ] Use supported absent-mask defaults, while explicit zero, partial, invalid, conflicting and nonzero declarations retain distinct semantics.
- [ ] Honor #251's agreed read-only original-analysis evidence accessor and existing type-alias surface; add no default MediaInfo fields or Report layout change.
- [ ] Classify CBR from encoded frame sizes; preserve validated nominal fixed-block cadence and exact inventory, with no universal variable-block assumption.
- [ ] Add the evidence-backed `Measured fact` entry in `AGENTS.md` required by [#115](https://github.com/autobrr/go-mediainfo/issues/115#issuecomment-6523664516) for validated fixed-block FLAC nominal SPF/cadence; retain the exact sampling inventory and variable-block exclusions.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s field contract; no broad video implementation prerequisite.

**Coordination and exclusions:** #56 absent-mask defaults and #251 explicit-mask evidence are distinct. #50/#49 belong to Plans 01/04.

## Validation and evidence

- [ ] Complete the [acceptance matrix and PR evidence checklist](README.md#acceptance-matrix-and-pr-checklist) for this plan’s existing criteria and the checks below.

[Shared validation requirements](README.md#validation-and-evidence). Reuse playable October-derived controls for eight-channel absent-mask and explicit masks, labeling derivatives separately from originals. Test malformed/partial/conflicting masks, all four FLAC containers, original evidence after copying/mutation, encoded frame sizes and complete fixed-block inventories.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
