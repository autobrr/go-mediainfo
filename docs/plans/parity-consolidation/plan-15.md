# Plan 15: MP4 codec sample entries

[Initiative index](README.md) · Stable plan ID 15

## Goal and scope

Use structural bounded ES descriptors and validated sample-entry geometry for MP4 AAC/SBR/PS, MPEG/private Ogg dispatch, PCM, H.263 and MPEG-4 Visual.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#92](https://github.com/autobrr/go-mediainfo/issues/92) | read H.263 sample entries as video | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/92#issuecomment-6252614626), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/92#issuecomment-6522146503) |
| [#98](https://github.com/autobrr/go-mediainfo/issues/98) | parse the MPEG-4 Visual VOL header | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/98#issuecomment-6252504490), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/98#issuecomment-6252614420), [Discussion 3](https://github.com/autobrr/go-mediainfo/issues/98#issuecomment-6252844865), [Discussion 4](https://github.com/autobrr/go-mediainfo/issues/98#issuecomment-6521860775) |
| [#113](https://github.com/autobrr/go-mediainfo/issues/113) | read MP3 in an `mp4a` entry as MPEG Audio | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/113#issuecomment-6252614864), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/113#issuecomment-6521437001) |
| [#123](https://github.com/autobrr/go-mediainfo/issues/123) | mark the bit rate CBR within the 0.5 % tolerance | [Discussion](https://github.com/autobrr/go-mediainfo/issues/123#issuecomment-6521223986) |
| [#161](https://github.com/autobrr/go-mediainfo/issues/161) | read explicit SBR and PS as Matroska does | [Discussion](https://github.com/autobrr/go-mediainfo/issues/161#issuecomment-6519751123) |
| [#162](https://github.com/autobrr/go-mediainfo/issues/162) | read PCM sample entries | [Discussion](https://github.com/autobrr/go-mediainfo/issues/162#issuecomment-6519726290) |

## Acceptance criteria

- [ ] Use structural bounded ES descriptors/sample-entry reads for supported codec dispatch; reuse co64-capable sample addressing.
- [ ] Correct shared Visual syntax/VBV and supported colour/library facts before exposing MPEG-4 Visual.
- [ ] Preserve SBR/PS, PCM and bitrate tolerance semantics; #113 requires Plan 16's low-rate Layer II framing correction.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 05](plan-05.md), [Plan 06](plan-06.md) and [Plan 16](plan-16.md)'s corrected shared MPEG audio framing.

**Coordination and exclusions:** [Plan 21](plan-21.md) owns brands and General-only image containers. Do not absorb those into codec sample entries.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Test malformed/nested descriptors and new parsers with fuzzing, H.263/Visual, MPEG/Ogg dispatch, PCM and explicit AAC SBR/PS; retain other codec-entry and co64 controls.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
