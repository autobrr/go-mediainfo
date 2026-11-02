# Plan 10: MP4 presentation timing

[Initiative index](README.md) · Stable plan ID 10

## Goal and scope

Implement MP4 source versus presentation timing as one pipeline: stts classification, exact rate tuples, matched source-field/trim selection, exact VFR byte/tick bitrate and General longest represented stream. Own final integration closure for #58.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#58](https://github.com/autobrr/go-mediainfo/issues/58) | Complete Ogg language and MP4 terminal timing; verify Plan 01 contributions | [Discussion](https://github.com/autobrr/go-mediainfo/issues/58#issuecomment-6522904574) |
| [#103](https://github.com/autobrr/go-mediainfo/issues/103) | detect VFR with the official `stts` rules | [Discussion](https://github.com/autobrr/go-mediainfo/issues/103#issuecomment-6521715332) |
| [#104](https://github.com/autobrr/go-mediainfo/issues/104) | Calculate `FrameRate_Num` and `FrameRate_Den` with one shared rule | [Discussion](https://github.com/autobrr/go-mediainfo/issues/104#issuecomment-6521683503) |
| [#105](https://github.com/autobrr/go-mediainfo/issues/105) | Compare the container frame rate with the stream frame rate | [Discussion](https://github.com/autobrr/go-mediainfo/issues/105#issuecomment-6521658942) |
| [#121](https://github.com/autobrr/go-mediainfo/issues/121) | take the General duration from the longest stream | [Discussion](https://github.com/autobrr/go-mediainfo/issues/121#issuecomment-6521270392) |
| [#124](https://github.com/autobrr/go-mediainfo/issues/124) | match the official trigger for `Source_*` fields and the trimmed stream size | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/124#issuecomment-6252754938), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/124#issuecomment-6521197554) |
| [#127](https://github.com/autobrr/go-mediainfo/issues/127) | exact or official rounding | [Discussion](https://github.com/autobrr/go-mediainfo/issues/127#issuecomment-6523529641) |

## Acceptance criteria

- [ ] Keep source and presentation timing/bytes distinct through stts, rate tuples, trimming and General duration selection.
- [ ] Preserve absent versus zero and calculate exact VFR byte/tick bitrate; never divide source bytes by trimmed duration.
- [ ] Close #58 only after Plan 01 serial/near-minute formatting, Plan 03 Ogg language and this plan's MP4 last/source-last-frame timing all pass.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md) and [Plan 03](plan-03.md)'s #58 contributions; supplies rate-source rules to Plan 09 and timing to Plan 14.

**Coordination and exclusions:** [Plan 09](plan-09.md) consumes shared rate rules. Do not absorb unrelated caption, Other or codec-entry work.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Test exact stts tuples, terminal samples, zero/absent fields, trimmed/source byte ownership, longest represented stream and all four #58 subcases through public projections.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
