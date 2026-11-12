# Plan 21: MP4 brands and conformance

[Initiative index](README.md) · Stable plan ID 21

## Goal and scope

Share typed format/brand tables across ftyp identity, valid General-only HEIF item containers and corrected extension/conformance policy.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#163](https://github.com/autobrr/go-mediainfo/issues/163) | fill Format and CodecID from the `ftyp` brand table | [Discussion](https://github.com/autobrr/go-mediainfo/issues/163#issuecomment-6519604718) |
| [#167](https://github.com/autobrr/go-mediainfo/issues/167) | scope and format name | [Discussion](https://github.com/autobrr/go-mediainfo/issues/167#issuecomment-6523468569) |
| [#232](https://github.com/autobrr/go-mediainfo/issues/232) | remove the hand-made checks and make FileExtension_Invalid correct | [Discussion](https://github.com/autobrr/go-mediainfo/issues/232#issuecomment-6517334529) |

## Acceptance criteria

- [ ] Use typed brand/format tables with significant spaces and the accepted first-duplicate-key policy.
- [ ] Accept valid General-only HEIF item containers without moov, without inventing an Image decoder.
- [ ] Correct extension/conformance policy and remove unsupported handmade checks; do not claim full v26 conformance.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s format-table interface; no broader parser-framework prerequisite.

**Coordination and exclusions:** General-only HEIF acceptance is not Image-track decoding; full v26 conformance is excluded.

## Validation and evidence

- [ ] Complete the [acceptance matrix and PR evidence checklist](README.md#acceptance-matrix-and-pr-checklist) for this plan’s existing criteria and the checks below.

[Shared validation requirements](README.md#validation-and-evidence). Test significant-space/duplicate brands, absent moov, valid/invalid item metadata and extensions; verify General-only counts and schema without claiming image decoding.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
