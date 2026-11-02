# Plan 20: Format settings and Vorbis setup

[Initiative index](README.md) · Stable plan ID 20

## Goal and scope

One codec-scoped format-settings summary and corresponding friendly/raw bindings, with the missing bounded Vorbis floor setup parser included as real fact production.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#151](https://github.com/autobrr/go-mediainfo/issues/151) | show the format facts that JSON already has | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/151#issuecomment-6252916751), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/151#issuecomment-6520286777) |
| [#152](https://github.com/autobrr/go-mediainfo/issues/152) | write `Format_Settings` with official values only | [Discussion](https://github.com/autobrr/go-mediainfo/issues/152#issuecomment-6520249308) |
| [#153](https://github.com/autobrr/go-mediainfo/issues/153) | use the official labels for AC-3 mode, Vorbis floor and GMC | [Discussion](https://github.com/autobrr/go-mediainfo/issues/153#issuecomment-6520222862) |

## Acceptance criteria

- [ ] Share a codec-scoped format-settings composition over actual parsed facts with compatible friendly/raw bindings.
- [ ] Do not invent BVOP/PNS or duplicate aggregates; keep GMC numeric facts separate from display.
- [ ] Implement bounded Vorbis Floor0/Floor1 fact production and integration; presentation-only work cannot close #151.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s shared projections.

**Coordination and exclusions:** [Plan 17](plan-17.md) owns AC-3 semantic values. FLAC mask/cadence work is separate.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Require Floor0/Floor1 controls, bounded malformed setup/fuzz cases and analysis-to-render checks; preserve non-Vorbis format summaries and numeric GMC.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
