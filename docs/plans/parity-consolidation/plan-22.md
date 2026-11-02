# Plan 22: AVC picture analysis

[Initiative index](README.md) · Stable plan ID 22

## Goal and scope

One bounded AVC picture/access-unit analysis for maximum slice count and GOP pattern using proper POC/display-order/field truth and supported container callsites.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#158](https://github.com/autobrr/go-mediainfo/issues/158) | port the official GOP detection | [Discussion](https://github.com/autobrr/go-mediainfo/issues/158#issuecomment-6519909992) |
| [#236](https://github.com/autobrr/go-mediainfo/issues/236) | take the largest slice count, in every container | [Discussion](https://github.com/autobrr/go-mediainfo/issues/236#issuecomment-6517182197) |

## Acceptance criteria

- [ ] Use bounded AVC access-unit/picture analysis for maximum slices and GOP, with proper POC/display-order/field evidence.
- [ ] Keep the 512-picture bound, physical counts and documented supported differences.
- [ ] Exercise supported container callsites; do not promise GOP intervals for arbitrary streams.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 07](plan-07.md)'s parser fact contract.

**Coordination and exclusions:** MPEG GOP logic is separate; physical counters must not become reporting-analysis counters.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Test field/frame ordering, POC changes, bounded/partial picture populations and supported container integrations; keep independent physical counts unchanged.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
