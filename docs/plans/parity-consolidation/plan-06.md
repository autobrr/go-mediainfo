# Plan 06: Colour and HDR

[Initiative index](README.md) · Stable plan ID 06

## Goal and scope

Unify colour namespaces, explicit presence, HDR component alignment, luminance precision and friendly/raw projections. Remove guessed Dolby Vision/profile colour facts and duplicate Original transfer rows.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#93](https://github.com/autobrr/go-mediainfo/issues/93) | show the colour and HDR lines that JSON already has | [Discussion](https://github.com/autobrr/go-mediainfo/issues/93#issuecomment-6522117007) |
| [#94](https://github.com/autobrr/go-mediainfo/issues/94) | Use one table of colour names for all parsers | [Discussion](https://github.com/autobrr/go-mediainfo/issues/94#issuecomment-6521976598) |
| [#96](https://github.com/autobrr/go-mediainfo/issues/96) | set `colour_description_present` only when the VUI has a colour description | [Discussion](https://github.com/autobrr/go-mediainfo/issues/96#issuecomment-6521899199) |
| [#97](https://github.com/autobrr/go-mediainfo/issues/97) | write the stream version in the HDR summary | [Discussion](https://github.com/autobrr/go-mediainfo/issues/97#issuecomment-6521879001) |
| [#99](https://github.com/autobrr/go-mediainfo/issues/99) | Mastering display luminance: use the official number of decimals | [Discussion](https://github.com/autobrr/go-mediainfo/issues/99#issuecomment-6521816556) |
| [#100](https://github.com/autobrr/go-mediainfo/issues/100) | read the Dolby Vision colour from the file | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/100#issuecomment-6253495459), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/100#issuecomment-6521780146) |
| [#101](https://github.com/autobrr/go-mediainfo/issues/101) | print `transfer_characteristics_Original` once | [Discussion](https://github.com/autobrr/go-mediainfo/issues/101#issuecomment-6521743895) |

## Acceptance criteria

- [ ] Share explicit colour namespaces/presence and aligned HDR components across supported producers and displays.
- [ ] Remove Dolby Vision/profile or BT.2020 guesses; preserve unknown codes without invented names.
- [ ] Use supported luminance precision and preserve adopted HDR conventions; keep first-sequence MPEG selection in Plan 08.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s display contract; supplies colour/HDR policy to Plan 07.

**Coordination and exclusions:** [Plan 08](plan-08.md) owns MPEG colour snapshot selection (#95).

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Require actual Dolby Vision and generic-HDR controls, absent/unspecified fields, partial combined components and fractional luminance; distinguish generated controls from original witnesses.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
