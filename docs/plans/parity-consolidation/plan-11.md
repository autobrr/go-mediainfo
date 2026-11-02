# Plan 11: PS duration and complete counts

[Initiative index](README.md) · Stable plan ID 11

## Goal and scope

Separate complete frame counts from bounded metadata sampling, establish supported PS timelines across resets/wraps and retain each DVD audio track’s actual counts/duration.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#118](https://github.com/autobrr/go-mediainfo/issues/118) | measure the duration across a PTS reset | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/118#issuecomment-6291782276), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/118#issuecomment-6334448032), [Discussion 3](https://github.com/autobrr/go-mediainfo/issues/118#issuecomment-6396278446), [Discussion 4](https://github.com/autobrr/go-mediainfo/issues/118#issuecomment-6396600988), [Discussion 5](https://github.com/autobrr/go-mediainfo/issues/118#issuecomment-6521379782) |
| [#119](https://github.com/autobrr/go-mediainfo/issues/119) | apply the AC-3 frame cap only when sampling | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/119#issuecomment-6334449132), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/119#issuecomment-6521337268) |
| [#239](https://github.com/autobrr/go-mediainfo/issues/239) | stop replacing a shorter track's sample count | [Discussion](https://github.com/autobrr/go-mediainfo/issues/239#issuecomment-6517103830) |

## Acceptance criteria

- [ ] Separate complete frame/sample inventory from bounded metadata sampling at every ParseSpeed.
- [ ] Establish supported reset/wrap timelines without unconditional summation or fitted/clamped estimates.
- [ ] Retain each audio track's counts/duration and independently supported video timing; keep invalid RLE from driving General/video/bitrate.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 08](plan-08.md)'s MPEG state and [Plan 13](plan-13.md)'s caption/discovery interface.

**Coordination and exclusions:** Preserve #249 invalid-RLE protections; do not copy a reference duration or sum reset segments without support.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Use complete inventories and real reset/wrap/cadence witnesses, sampled versus full reads, short audio tracks and invalid-RLE preservation controls. Reject unsupported totals rather than fit expected captures.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
