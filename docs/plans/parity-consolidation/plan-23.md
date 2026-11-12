# Plan 23: AVI Type-1 DV demultiplexing

[Initiative index](README.md) · Stable plan ID 23

## Goal and scope

Implement Type-1 AVI DV demultiplexing and its audio/video reporting as a dedicated bounded feature.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#211](https://github.com/autobrr/go-mediainfo/issues/211) | read type-1 DV (`iavs`) streams | [Discussion](https://github.com/autobrr/go-mediainfo/issues/211#issuecomment-6517901776) |

## Acceptance criteria

- [ ] Validate bounded DV DIF packets/sequences and demultiplex actual Type-1 audio/video identities.
- [ ] Integrate audio extraction, counts and projections with Type-2/non-DV preservation controls.
- [ ] Verify real Type-1 evidence; no closure from iavs recognition or placeholder tracks alone.

## Dependencies and non-goals

**Required contracts:** [Plan 16](plan-16.md)'s RIFF entry contract and [Plan 01](plan-01.md)'s projections.

**Coordination and exclusions:** DIF validation and real audio extraction require their own controls; this is not a RIFF-label correction.

## Validation and evidence

[Shared validation requirements](README.md#validation-and-evidence). Require actual Type-1 DV plus Type-2/non-DV controls, malformed/truncated DIF bounds, extracted audio identity and analysis-to-report checks; fuzz the new parser.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
