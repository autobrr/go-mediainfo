# Plan 19: Relative delay pipeline

[Initiative index](README.md) · Stable plan ID 19

## Goal and scope

One checked delay quantizer and relative-delay derivation, then friendly display, valid-zero presence and late-start stream policy.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

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

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 08](plan-08.md)'s supported delay inputs; settle the shared contract before final integration in Plans 13/14/16.

**Coordination and exclusions:** Raw visibility belongs to Plan 01; MP4 timecode origin/inheritance belongs to Plan 14.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Test missing/zero/negative/overflow inputs, rounding boundaries, video absence and real late-start timelines across supported containers and public rebuilding.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
