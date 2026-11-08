# Plan 22: AVC picture analysis

[Initiative index](README.md) · Stable plan ID 22

## Goal and scope

Analyze AVC pictures/access units within the existing bound. Determine maximum slice count and GOP patterns from supported POC, display-order and field evidence, and integrate the supported container callsites.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#158](https://github.com/autobrr/go-mediainfo/issues/158) | port the official GOP detection | [Discussion](https://github.com/autobrr/go-mediainfo/issues/158#issuecomment-6519909992) |
| [#236](https://github.com/autobrr/go-mediainfo/issues/236) | take the largest slice count, in every container | [Discussion](https://github.com/autobrr/go-mediainfo/issues/236#issuecomment-6517182197) |

## Acceptance criteria

- [ ] Use bounded AVC access-unit/picture analysis for maximum slices and GOP, with proper POC/display-order/field evidence.
- [ ] Keep the 512-picture bound, physical counts and documented supported differences.
- [ ] Exercise supported container callsites; do not promise GOP intervals for arbitrary streams.

## Dependencies and non-goals

**Required contracts:** [Plan 07](plan-07.md)'s parser fact contract.

**Coordination and exclusions:** MPEG GOP logic is separate; physical counters must not become reporting-analysis counters.

## Validation and evidence

[Shared validation requirements](README.md#validation-and-evidence). Test field/frame ordering, POC changes, bounded/partial picture populations and supported container integrations; keep independent physical counts unchanged.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
