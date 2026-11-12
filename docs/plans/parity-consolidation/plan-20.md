# Plan 20: Format settings and Vorbis setup

[Initiative index](README.md) · Stable plan ID 20

## Goal and scope

Compose format-settings summaries from facts for the relevant codec. Use those facts for friendly/raw bindings and parse the missing Vorbis floor setup within explicit bounds.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#151](https://github.com/autobrr/go-mediainfo/issues/151) | show the format facts that JSON already has | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/151#issuecomment-6252916751), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/151#issuecomment-6520286777) |
| [#152](https://github.com/autobrr/go-mediainfo/issues/152) | write `Format_Settings` with official values only | [Discussion](https://github.com/autobrr/go-mediainfo/issues/152#issuecomment-6520249308) |
| [#153](https://github.com/autobrr/go-mediainfo/issues/153) | use the official labels for AC-3 mode, Vorbis floor and GMC | [Discussion](https://github.com/autobrr/go-mediainfo/issues/153#issuecomment-6520222862) |

## Acceptance criteria

- [ ] Share a codec-scoped format-settings composition over actual parsed facts with compatible friendly/raw bindings.
- [ ] Do not invent BVOP/PNS or duplicate aggregates; keep GMC numeric facts separate from display.
- [ ] Implement bounded Vorbis Floor0/Floor1 fact production and integration; presentation-only work cannot close #151.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s shared projections.

**Coordination and exclusions:** [Plan 17](plan-17.md) owns AC-3 semantic values. FLAC mask/cadence work is separate.

## Validation and evidence

[Shared validation requirements](README.md#validation-and-evidence). Require Floor0/Floor1 controls, bounded malformed setup/fuzz cases and analysis-to-render checks; preserve non-Vorbis format summaries and numeric GMC.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
