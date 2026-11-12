# Plan 19: Relative delay pipeline

[Initiative index](README.md) · Stable plan ID 19

## Goal and scope

Quantize supported delays once and derive relative delay from applicable inputs. Then apply friendly display, valid-zero presence and the supported late-start stream policy.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#171](https://github.com/autobrr/go-mediainfo/issues/171) | build "Delay relative to video" from `Video_Delay` and hide it at 0 ms | [Discussion](https://github.com/autobrr/go-mediainfo/issues/171#issuecomment-6519345454) |
| [#175](https://github.com/autobrr/go-mediainfo/issues/175) | use one rounding rule for all containers | [Discussion](https://github.com/autobrr/go-mediainfo/issues/175#issuecomment-6519239338) |
| [#177](https://github.com/autobrr/go-mediainfo/issues/177) | write audio `Video_Delay` only when the file has video | [Discussion](https://github.com/autobrr/go-mediainfo/issues/177#issuecomment-6519133818) |
| [#180](https://github.com/autobrr/go-mediainfo/issues/180) | Delay of a stream that starts after the first part of the file | [Discussion](https://github.com/autobrr/go-mediainfo/issues/180#issuecomment-6523413672) |

## Acceptance criteria

- [ ] Derive relative delay only with both applicable supported inputs using one checked rounding/quantization policy.
- [ ] Preserve valid-zero presence and friendly omission at zero; remove text-only AVC guesses.
- [ ] Establish supported late-start semantics rather than an arbitrary one-minute assumption; preserve independent timecode ownership.

## Dependencies and non-goals

**Required contracts:** [Plan 08](plan-08.md)'s supported delay inputs; settle the shared contract before final integration in Plans 13/14/16.

**Coordination and exclusions:** Raw visibility belongs to Plan 01; MP4 timecode origin/inheritance belongs to Plan 14.

## Validation and evidence

- [ ] Complete the [acceptance matrix and PR evidence checklist](README.md#acceptance-matrix-and-pr-checklist) for this plan’s existing criteria and the checks below.

[Shared validation requirements](README.md#validation-and-evidence). Test missing/zero/negative/overflow inputs, rounding boundaries, video absence and real late-start timelines across supported containers and public rebuilding.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
