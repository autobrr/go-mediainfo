# Plan 21: MP4 brands and conformance

[Initiative index](README.md) · Stable plan ID 21

## Goal and scope

Share typed format/brand tables across ftyp identity, valid General-only HEIF item containers and corrected extension/conformance policy.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#163](https://github.com/autobrr/go-mediainfo/issues/163) | fill Format and CodecID from the `ftyp` brand table | [Discussion](https://github.com/autobrr/go-mediainfo/issues/163#issuecomment-6519604718) |
| [#167](https://github.com/autobrr/go-mediainfo/issues/167) | scope and format name | [Discussion](https://github.com/autobrr/go-mediainfo/issues/167#issuecomment-6523468569) |
| [#232](https://github.com/autobrr/go-mediainfo/issues/232) | remove the hand-made checks and make FileExtension_Invalid correct | [Discussion](https://github.com/autobrr/go-mediainfo/issues/232#issuecomment-6517334529) |

## Acceptance criteria

- [ ] Use typed brand/format tables with significant spaces and the accepted first-duplicate-key policy.
- [ ] Accept valid General-only HEIF item containers without moov, without inventing an Image decoder.
- [ ] Correct extension/conformance policy and remove unsupported handmade checks; do not claim full v26 conformance.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s format-table interface; no broader parser-framework prerequisite.

**Coordination and exclusions:** General-only HEIF acceptance is not Image-track decoding; full v26 conformance is excluded.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Test significant-space/duplicate brands, absent moov, valid/invalid item metadata and extensions; verify General-only counts and schema without claiming image decoding.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
