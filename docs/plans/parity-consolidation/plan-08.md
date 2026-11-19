# Plan 08: MPEG picture clock state

[Initiative index](README.md) · Stable plan ID 08

## Goal and scope

Coordinate MPEG sequence, picture and report state. Select pulldown/scan, GOP patterns, trusted timecode, VBV/rate/matrices, Standard, geometry and first-reported colour using their own supported rules. Remove competing DVD/PS overwrites and unsupported delay heuristics.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#91](https://github.com/autobrr/go-mediainfo/issues/91) | round the pixel aspect ratio and use the display extension size | [Discussion](https://github.com/autobrr/go-mediainfo/issues/91#issuecomment-6522172216) |
| [#95](https://github.com/autobrr/go-mediainfo/issues/95) | take the colour from the first sequence header | [Discussion](https://github.com/autobrr/go-mediainfo/issues/95#issuecomment-6521949656) |
| [#112](https://github.com/autobrr/go-mediainfo/issues/112) | report 2:3 pulldown only for the exact cadence | [Discussion](https://github.com/autobrr/go-mediainfo/issues/112#issuecomment-6521464858) |
| [#120](https://github.com/autobrr/go-mediainfo/issues/120) | put the sequence header bit rate in `BitRate_Maximum` | [Discussion](https://github.com/autobrr/go-mediainfo/issues/120#issuecomment-6521304672) |
| [#137](https://github.com/autobrr/go-mediainfo/issues/137) | use the official rules for scan type, scan order and picture structure | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/137#issuecomment-6252615258), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/137#issuecomment-6520987166), [Discussion 3](https://github.com/autobrr/go-mediainfo/issues/137#issuecomment-6539013305) |
| [#138](https://github.com/autobrr/go-mediainfo/issues/138) | apply the official time code trust rules and GOP open or closed rules | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/138#issuecomment-6252754469), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/138#issuecomment-6520864390) |
| [#139](https://github.com/autobrr/go-mediainfo/issues/139) | calculate `Format_Settings_GOP` with the official picture-type rule | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/139#issuecomment-6252615046), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/139#issuecomment-6520811444) |
| [#140](https://github.com/autobrr/go-mediainfo/issues/140) | take Standard from `video_format` and use the official size and rate fallback | [Discussion](https://github.com/autobrr/go-mediainfo/issues/140#issuecomment-6520769088) |
| [#141](https://github.com/autobrr/go-mediainfo/issues/141) | stop replacing and deleting MPEG-2 video fields after the parse | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/141#issuecomment-6252754682), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/141#issuecomment-6334451133), [Discussion 3](https://github.com/autobrr/go-mediainfo/issues/141#issuecomment-6395607768), [Discussion 4](https://github.com/autobrr/go-mediainfo/issues/141#issuecomment-6520720475), [Discussion 5](https://github.com/autobrr/go-mediainfo/issues/141#issuecomment-6539010836) |
| [#142](https://github.com/autobrr/go-mediainfo/issues/142) | read `vbv_buffer_size_extension` and take BufferSize from the current sequence header | [Discussion](https://github.com/autobrr/go-mediainfo/issues/142#issuecomment-6520666441) |
| [#144](https://github.com/autobrr/go-mediainfo/issues/144) | make `modeValue` give the same result on every run | [Discussion](https://github.com/autobrr/go-mediainfo/issues/144#issuecomment-6520515903) |
| [#157](https://github.com/autobrr/go-mediainfo/issues/157) | report the quantization matrix with the official rules | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/157#issuecomment-6291782673), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/157#issuecomment-6520018441), [Discussion 3](https://github.com/autobrr/go-mediainfo/issues/157#issuecomment-6520095545), [Discussion 4](https://github.com/autobrr/go-mediainfo/issues/157#issuecomment-6520726098) |
| [#173](https://github.com/autobrr/go-mediainfo/issues/173) | take `Delay_Original` from the first-frame time code in every container | [Discussion](https://github.com/autobrr/go-mediainfo/issues/173#issuecomment-6519293787) |
| [#174](https://github.com/autobrr/go-mediainfo/issues/174) | stop shifting and deleting the delays after the parse | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/174#issuecomment-6334452047), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/174#issuecomment-6395611700), [Discussion 3](https://github.com/autobrr/go-mediainfo/issues/174#issuecomment-6519266821) |

## Acceptance criteria

- [ ] Pin separate state-selection rules: first reported colour, current VBV/Standard, latest matrix flags with first complete loaded matrices, early trusted clock and completed GOP patterns.
- [ ] Remove demonstrated competing PS/DVD overwrites and unsupported delay heuristics while preserving supported fields.
- [ ] Verify every #141 downstream-survival case and #249 protections, not merely one helper deletion.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md), [Plan 05](plan-05.md), [Plan 06](plan-06.md)'s contracts; supplies MPEG state to Plans 11/19.

**Coordination and exclusions:** [Plan 13](plan-13.md) owns caption-parent discovery. Coordinate overwrite removals with Plans 11/12/19 rather than deleting whole normalizers independently.

## Validation and evidence

- [ ] Complete the [acceptance matrix and PR evidence checklist](README.md#acceptance-matrix-and-pr-checklist) for this plan’s existing criteria and the checks below.

[Shared validation requirements](README.md#validation-and-evidence). Test changing sequence/picture state, pulldown, clock trust, GOP completion, matrix/VBV updates and every DVD downstream survival route; preserve malformed-input and invalid-RLE controls.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
