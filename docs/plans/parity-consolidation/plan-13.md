# Plan 13: Caption discovery and identity

[Initiative index](README.md) · Stable plan ID 13

## Goal and scope

Implement bounded caption discovery and parent association across PS/DVD and MP4, then emit correct caption identities and presentation. Include parse-window extension, CC1/CC3 semantics, Start time commands and removal of unsupported service labels. #210 explicitly includes ctts presentation timing, supported edit mapping, a bounded discovery window beyond the generic 32-sample probe, and stateful 708 validation.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#179](https://github.com/autobrr/go-mediainfo/issues/179) | use the video Delay for the caption Delay | [Discussion](https://github.com/autobrr/go-mediainfo/issues/179#issuecomment-6519039086) |
| [#201](https://github.com/autobrr/go-mediainfo/issues/201) | remove the "Caption service name" line from text | [Discussion](https://github.com/autobrr/go-mediainfo/issues/201#issuecomment-6518508302) |
| [#202](https://github.com/autobrr/go-mediainfo/issues/202) | show "Start time (commands)" | [Discussion](https://github.com/autobrr/go-mediainfo/issues/202#issuecomment-6518488642) |
| [#203](https://github.com/autobrr/go-mediainfo/issues/203) | extend the parse window when captions are found, for single files too | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/203#issuecomment-6334452780), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/203#issuecomment-6395603983), [Discussion 3](https://github.com/autobrr/go-mediainfo/issues/203#issuecomment-6518471506) |
| [#210](https://github.com/autobrr/go-mediainfo/issues/210) | read EIA-608 and EIA-708 captions in AVC | [Discussion](https://github.com/autobrr/go-mediainfo/issues/210#issuecomment-6518013643) |
| [#215](https://github.com/autobrr/go-mediainfo/issues/215) | CC1 or CC3 | [Discussion](https://github.com/autobrr/go-mediainfo/issues/215#issuecomment-6523225413) |

## Acceptance criteria

- [ ] Discover captions with bounded parsing/window extension and actual parent/service association; do not use display order as identity.
- [ ] Implement MP4 ctts/edit timing, supported sample addressing and stateful 708 validation beyond the generic 32-sample probe.
- [ ] Emit supported Start-time commands and CC1/CC3 identity while preserving timing facts and omitting unsupported service labels.
- [ ] Add the explicit measured-fact entry in `AGENTS.md` required by [#215](https://github.com/autobrr/go-mediainfo/issues/215#issuecomment-6523225413), citing the verified caption-service witness and PR #41.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md), [Plan 08](plan-08.md), [Plan 10](plan-10.md), [Plan 19](plan-19.md)'s contracts.

**Coordination and exclusions:** Coordinate shared MPEG-PS probe changes with [Plan 11](plan-11.md) without blocking its independent timing/count work. [Plan 12](plan-12.md) owns DVD identity mapping. Share bounded MP4 time addressing with Plan 14 without embedding caption decoding in that helper.

## Validation and evidence

- [ ] Complete the [acceptance matrix and PR evidence checklist](README.md#acceptance-matrix-and-pr-checklist) for this plan’s existing criteria and the checks below.

[Shared validation requirements](README.md#validation-and-evidence). Require actual caption evidence with composition/edit timing, service transitions, malformed SEI/708, independent parent association and bounded window extension; fuzz newly exposed parsers.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
