# Plan 09: Matroska identity and timing

[Initiative index](README.md) · Stable plan ID 09

## Goal and scope

Preserve EBML string/DocType identity, WebM-specific estimates, explicit Matroska service flags, independent track timing, rate selection, SOURCE_ID eligibility and chapter display.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#110](https://github.com/autobrr/go-mediainfo/issues/110) | measure the frame rate when DefaultDuration is missing | [Discussion](https://github.com/autobrr/go-mediainfo/issues/110#issuecomment-6521511015) |
| [#122](https://github.com/autobrr/go-mediainfo/issues/122) | use the default overhead ratio for the video size estimate | [Discussion](https://github.com/autobrr/go-mediainfo/issues/122#issuecomment-6521248445) |
| [#164](https://github.com/autobrr/go-mediainfo/issues/164) | report WebM from the DocType | [Discussion](https://github.com/autobrr/go-mediainfo/issues/164#issuecomment-6519556957) |
| [#165](https://github.com/autobrr/go-mediainfo/issues/165) | trim zero padding from CodecID | [Discussion](https://github.com/autobrr/go-mediainfo/issues/165#issuecomment-6519505469) |
| [#169](https://github.com/autobrr/go-mediainfo/issues/169) | make the text Service kind line agree with JSON ServiceKind | [Discussion](https://github.com/autobrr/go-mediainfo/issues/169#issuecomment-6519405114) |
| [#208](https://github.com/autobrr/go-mediainfo/issues/208) | convert SOURCE_ID only when the statistics tags list it | [Discussion](https://github.com/autobrr/go-mediainfo/issues/208#issuecomment-6518189812) |
| [#223](https://github.com/autobrr/go-mediainfo/issues/223) | build chapter names with the official display rule | [Discussion](https://github.com/autobrr/go-mediainfo/issues/223#issuecomment-6517559792) |
| [#228](https://github.com/autobrr/go-mediainfo/issues/228) | write a track duration from the Segment Duration with millisecond precision | [Discussion](https://github.com/autobrr/go-mediainfo/issues/228#issuecomment-6517393213) |
| [#245](https://github.com/autobrr/go-mediainfo/issues/245) | take the audio duration from the track, not the Segment | [Discussion](https://github.com/autobrr/go-mediainfo/issues/245#issuecomment-6511890015) |

## Acceptance criteria

- [ ] Preserve EBML/DocType identity and stable dispatch when display Format becomes WebM.
- [ ] Separate measured sizes from WebM estimates, track timing from Segment fallback, and explicit service flags from display.
- [ ] Verify valid DURATION tags, distinct-source commentary, SOURCE_ID eligibility and chapter display without erasing unknown metadata.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md), [Plan 03](plan-03.md) and [Plan 10](plan-10.md)'s shared rate-source rules.

**Coordination and exclusions:** [Plan 07](plan-07.md) owns scan facts. Segment duration is not a substitute for independent track evidence.

## Validation and evidence

- [ ] Complete the [acceptance matrix and PR evidence checklist](README.md#acceptance-matrix-and-pr-checklist) for this plan’s existing criteria and the checks below.

[Shared validation requirements](README.md#validation-and-evidence). Test valid/missing DefaultDuration and tags, unknown EBML strings, WebM estimates, chapter displays and independent per-track durations; use shared-rate controls from Plan 10.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
