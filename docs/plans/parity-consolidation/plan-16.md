# Plan 16: AVI and MPEG audio integration

[Initiative index](README.md) · Stable plan ID 16

## Goal and scope

Unify RIFF codec/header precedence with bounded AVC/MPEG/AAC probing, correct shared MPEG audio framing/Layer II sizing and reporting-header selection, plus AVI delay inputs.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#159](https://github.com/autobrr/go-mediainfo/issues/159) | take the mode extension from the official frame | [Discussion](https://github.com/autobrr/go-mediainfo/issues/159#issuecomment-6519834872) |
| [#166](https://github.com/autobrr/go-mediainfo/issues/166) | use the RIFF codec tables and parse AVC, MP2 and AAC payloads | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/166#issuecomment-6291781803), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/166#issuecomment-6519483060) |
| [#178](https://github.com/autobrr/go-mediainfo/issues/178) | write the delay fields with the official conditions | [Discussion](https://github.com/autobrr/go-mediainfo/issues/178#issuecomment-6519120078) |

## Acceptance criteria

- [ ] Use RIFF codec/header precedence and bounded AVC/MPEG/AAC probing without encoder-name or percentage guesses.
- [ ] Correct shared MPEG framing/Layer II sizing and first-stream-fill mode-extension selection.
- [ ] Preserve AVI measured timing and accepted frame order; Type-1 DV stays in Plan 23.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md) and [Plan 19](plan-19.md)'s projection/delay contracts; supplies MPEG framing to Plan 15.

**Coordination and exclusions:** [Plan 23](plan-23.md) owns Type-1 DV demultiplexing; it is not part of routine RIFF integration.

## Validation and evidence

[Shared validation requirements](README.md#validation-and-evidence). Test low-rate Layer II and changing mode-extension headers, RIFF priority, bounded probes and AVI delay eligibility with actual integration controls.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
