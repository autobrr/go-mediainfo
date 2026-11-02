# Plan 05: Video geometry

[Initiative index](README.md) · Stable plan ID 05

## Goal and scope

A shared geometry evidence model and container-specific merge rules for primary/Original/Sampled/Stored dimensions, SAR/DAR, rotation and display names. Include #60’s Theora visibility, ratio and selected-bitrate density acceptance.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#60](https://github.com/autobrr/go-mediainfo/issues/60) | raw field names, aspect ratio, and frame rate differ from official | [Discussion](https://github.com/autobrr/go-mediainfo/issues/60#issuecomment-6522818926) |
| [#84](https://github.com/autobrr/go-mediainfo/issues/84) | use the container display size for the aspect ratio | [Discussion](https://github.com/autobrr/go-mediainfo/issues/84#issuecomment-6522406477) |
| [#85](https://github.com/autobrr/go-mediainfo/issues/85) | calculate the aspect ratio from the SAR, `pasp` and the `tkhd` size | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/85#issuecomment-6522366029), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/85#issuecomment-6540256978) |
| [#86](https://github.com/autobrr/go-mediainfo/issues/86) | read the rotation from the `tkhd` matrix | [Discussion](https://github.com/autobrr/go-mediainfo/issues/86#issuecomment-6522323287) |
| [#87](https://github.com/autobrr/go-mediainfo/issues/87) | put the container size and the stream size in the correct fields | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/87#issuecomment-6522297302), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/87#issuecomment-6540255759) |
| [#88](https://github.com/autobrr/go-mediainfo/issues/88) | Write `Sampled_Width` and `Sampled_Height` only from a stream parser | [Discussion](https://github.com/autobrr/go-mediainfo/issues/88#issuecomment-6522260704) |
| [#89](https://github.com/autobrr/go-mediainfo/issues/89) | hide the pixel aspect ratio line and use the ratio name for AVI and Ogg | [Discussion](https://github.com/autobrr/go-mediainfo/issues/89#issuecomment-6522212779) |
| [#90](https://github.com/autobrr/go-mediainfo/issues/90) | print "Original display aspect ratio" and use the ratio names in raw text | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/90#issuecomment-6522195045), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/90#issuecomment-6540258089) |

## Acceptance criteria

- [ ] Keep coded, container, visible, Sampled and Stored evidence distinct; honor valid pasp and matrix rotation semantics.
- [ ] Do not manufacture Sampled dimensions from codec names or alter measured values to fit a display.
- [ ] Close #60 only with named DAR, selected-bitrate density, hidden components and the Plan 01 FPS/display contribution verified together.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s Theora/display contribution; supplies geometry to Plans 07/15.

**Coordination and exclusions:** [Plan 08](plan-08.md) owns MPEG sequence geometry. Matrix rotation does not swap encoded dimensions.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Test square/non-square pasp, rotation, dimension conflicts and absent density prerequisites. Recompare C000232/Theora plus geometry controls without changing selected bitrate to fit text.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
