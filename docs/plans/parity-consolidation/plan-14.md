# Plan 14: MP4 Other and chapter graph

[Initiative index](README.md) · Stable plan ID 14

## Goal and scope

Build a coherent MP4 track graph: physical trak order, Other tracks, bounded tmcd samples/edits, references, inherited timecode delay and chapter membership/empty-name/bitrate rules.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#176](https://github.com/autobrr/go-mediainfo/issues/176) | read the `tmcd` time code track | [Discussion](https://github.com/autobrr/go-mediainfo/issues/176#issuecomment-6519196540) |
| [#205](https://github.com/autobrr/go-mediainfo/issues/205) | write StreamOrder in `trak` order | [Discussion](https://github.com/autobrr/go-mediainfo/issues/205#issuecomment-6518362629) |
| [#209](https://github.com/autobrr/go-mediainfo/issues/209) | report time code, hint and description tracks as Other | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/209#issuecomment-6253117184), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/209#issuecomment-6518135212) |
| [#217](https://github.com/autobrr/go-mediainfo/issues/217) | list every referencing track in `Menus` and `Menu For` | [Discussion](https://github.com/autobrr/go-mediainfo/issues/217#issuecomment-6517693049) |
| [#219](https://github.com/autobrr/go-mediainfo/issues/219) | set the chapter Menu bit rate mode from the constant sample size | [Discussion](https://github.com/autobrr/go-mediainfo/issues/219#issuecomment-6517652938) |
| [#220](https://github.com/autobrr/go-mediainfo/issues/220) | drop chapters with an empty name from the JSON Menu | [Discussion](https://github.com/autobrr/go-mediainfo/issues/220#issuecomment-6517625856) |

## Acceptance criteria

- [ ] Keep physical trak order, original IDs, references and supported sample-description indices through the MP4 track graph.
- [ ] Integrate real Other tracks into counts, schemas and all supported projections; parser stubs do not complete #209.
- [ ] Parse bounded tmcd/edit addressing and chapter membership/empty-name/bitrate rules. A timecode origin is not General duration.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md), [Plan 03](plan-03.md), [Plan 10](plan-10.md), [Plan 19](plan-19.md)'s timing and delay contracts.

**Coordination and exclusions:** No generic track framework is required; Plan 13 shares bounded time addressing without sharing caption decoding.

## Validation and evidence

[Shared validation requirements](README.md#validation-and-evidence). Test multiple Other/chapter tracks, empty names, references, zero positions, sample-description changes and tmcd edit/sample bounds through analysis and public renderers.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
