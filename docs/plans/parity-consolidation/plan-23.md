# Plan 23: AVI Type-1 DV demultiplexing

[Initiative index](README.md) · Stable plan ID 23

## Goal and scope

Implement Type-1 AVI DV demultiplexing and its audio/video reporting as a dedicated bounded feature.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#211](https://github.com/autobrr/go-mediainfo/issues/211) | read type-1 DV (`iavs`) streams | [Discussion](https://github.com/autobrr/go-mediainfo/issues/211#issuecomment-6517901776) |

## Acceptance criteria

- [ ] Validate bounded DV DIF packets/sequences and demultiplex actual Type-1 audio/video identities.
- [ ] Integrate audio extraction, counts and projections with Type-2/non-DV preservation controls.
- [ ] Verify real Type-1 evidence; no closure from iavs recognition or placeholder tracks alone.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 16](plan-16.md)'s RIFF entry contract and [Plan 01](plan-01.md)'s projections.

**Coordination and exclusions:** DIF validation and real audio extraction require their own controls; this is not a RIFF-label correction.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Require actual Type-1 DV plus Type-2/non-DV controls, malformed/truncated DIF bounds, extracted audio identity and analysis-to-report checks; fuzz the new parser.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
