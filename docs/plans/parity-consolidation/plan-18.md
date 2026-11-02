# Plan 18: FLAC evidence and cadence

[Initiative index](README.md) · Stable plan ID 18

## Goal and scope

One FLAC evidence pipeline for absent-mask defaults, explicit zero/nonzero masks, Matroska overrides, encoded-frame-size CBR and nominal fixed-block cadence.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

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

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s field contract; no broad video implementation prerequisite.

**Coordination and exclusions:** #56 absent-mask defaults and #251 explicit-mask evidence are distinct. #50/#49 belong to Plans 01/04.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Reuse playable October-derived controls for eight-channel absent-mask and explicit masks, labeling derivatives separately from originals. Test malformed/partial/conflicting masks, all four FLAC containers, original evidence after copying/mutation, encoded frame sizes and complete fixed-block inventories.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
