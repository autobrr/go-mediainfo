# Plan 12: DVD identity and retained facts

[Initiative index](README.md) · Stable plan ID 12

## Goal and scope

Correct PS/DVD stream identity and order, PGC-specific Menu membership, DTS private IDs and identity-aware IFO/payload mapping. Preserve valid codec, Menu and Text declarations/aggregates.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#111](https://github.com/autobrr/go-mediainfo/issues/111) | Preserve supported VOB facts and independent IFO/Menu clocks under v26.10 | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/111#issuecomment-6252265642), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/111#issuecomment-6252614254), [Discussion 3](https://github.com/autobrr/go-mediainfo/issues/111#issuecomment-6521485374) |
| [#126](https://github.com/autobrr/go-mediainfo/issues/126) | Retain supported Text duration; preserve invalid-timeline omissions | [Discussion](https://github.com/autobrr/go-mediainfo/issues/126#issuecomment-6521152185) |
| [#134](https://github.com/autobrr/go-mediainfo/issues/134) | Keep identity-aware IFO/PGC-to-payload association and languages | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/134#issuecomment-6334450246), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/134#issuecomment-6521045805) |
| [#204](https://github.com/autobrr/go-mediainfo/issues/204) | take StreamOrder from the system header | [Discussion](https://github.com/autobrr/go-mediainfo/issues/204#issuecomment-6518452833) |
| [#206](https://github.com/autobrr/go-mediainfo/issues/206) | Remove unsupported duration/caption-rank heuristics; retain supported route order | [Discussion](https://github.com/autobrr/go-mediainfo/issues/206#issuecomment-6518321703) |
| [#214](https://github.com/autobrr/go-mediainfo/issues/214) | add 0x88 to the DTS private stream ID | [Discussion](https://github.com/autobrr/go-mediainfo/issues/214#issuecomment-6517752065) |
| [#216](https://github.com/autobrr/go-mediainfo/issues/216) | build the stream lists from the program chain of each Menu | [Discussion](https://github.com/autobrr/go-mediainfo/issues/216#issuecomment-6517722522) |

## Acceptance criteria

- [ ] Preserve valid VOB codec facts, IFO/Menu clocks and supported Text durations; obsolete blanket-removal requests require evidence-backed regression disposition.
- [ ] Associate attributes/languages/facts by enabled retained-PGC and exact payload identity, including alternatives, conflicts and captions.
- [ ] Implement header-derived StreamOrder and settled heuristic removal separately from report-row order; preserve #248/#249 and do not invent a universal captions-last or numeric IFO sort.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md) and [Plan 03](plan-03.md)'s interfaces; preserve merged [Plan 24](plan-24.md)'s final-size behavior where applicable.

**Coordination and exclusions:** [Plan 13](plan-13.md) owns caption discovery. [#254](https://github.com/autobrr/go-mediainfo/issues/254) coordinates route-specific research with #134/#204, but does not block settled #206 scope. [#255](https://github.com/autobrr/go-mediainfo/issues/255) is separate; no unsupported fallback-accounting change is included.

## Validation and evidence

[Shared validation requirements](README.md#validation-and-evidence). Use declaration-only IFO/BUP, actual aggregates and standalone VOB controls; reordered audio, enabled wide/letterbox mappings, conflicts, missing companions, DTS IDs, captions/RLE and absent header entries. Verify v26.10 plus independent mapping evidence for any factual difference.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
