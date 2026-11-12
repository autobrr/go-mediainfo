# Plan 17: AC3 semantic projection

[Initiative index](README.md) · Stable plan ID 17

## Goal and scope

Project parsed AC-3/E-AC-3 facts consistently through every existing producer. Preserve dual-mono bit alignment, explicit latest surround metadata, mix levels and measured dynrng/statistics. Apply the required JSON/display policy and report JOC only from actual evidence.

## Issue ownership and canonical decisions

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#53](https://github.com/autobrr/go-mediainfo/issues/53) | Consolidate AC-3 family facts/projection with explicit container context | [Discussion](https://github.com/autobrr/go-mediainfo/issues/53#issuecomment-6522588788) |
| [#160](https://github.com/autobrr/go-mediainfo/issues/160) | take the surround mode from the last parsed frame | [Discussion](https://github.com/autobrr/go-mediainfo/issues/160#issuecomment-6519797918) |
| [#194](https://github.com/autobrr/go-mediainfo/issues/194) | write the metadata lines in text with one rule for all containers | [Discussion](https://github.com/autobrr/go-mediainfo/issues/194#issuecomment-6518650210) |
| [#195](https://github.com/autobrr/go-mediainfo/issues/195) | write the mix level values with the A/52 meanings | [Discussion](https://github.com/autobrr/go-mediainfo/issues/195#issuecomment-6518633338) |
| [#196](https://github.com/autobrr/go-mediainfo/issues/196) | stop hiding dynrng by channel mode | [Discussion](https://github.com/autobrr/go-mediainfo/issues/196#issuecomment-6518601122) |
| [#197](https://github.com/autobrr/go-mediainfo/issues/197) | leave `dialnorm_Maximum` out of JSON | [Discussion](https://github.com/autobrr/go-mediainfo/issues/197#issuecomment-6518549690) |
| [#198](https://github.com/autobrr/go-mediainfo/issues/198) | read the dual mono channel 2 fields in the correct order | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/198#issuecomment-6518535076), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/198#issuecomment-6518614905) |
| [#199](https://github.com/autobrr/go-mediainfo/issues/199) | Preserve supported AC-3 corrections and document their evidence | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/199#issuecomment-6253116967), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/199#issuecomment-6523304989) |
| [#243](https://github.com/autobrr/go-mediainfo/issues/243) | report JOC only from a JOC payload | [Discussion](https://github.com/autobrr/go-mediainfo/issues/243#issuecomment-6517054924) |

## Acceptance criteria

- [ ] Route all reachable AC-3 producers through one presence-aware private family owner, retaining explicit zero and real independent/dependent/JOC syntax.
- [ ] Keep accepted-observation statistics, first/current metadata ownership and latest explicit surround policy distinct; remove unsupported fitted populations.
- [ ] Preserve finite/muted gain semantics, ordinary versus DVD aggregate visibility, valid timing/size and deliberate factual corrections.
- [ ] Record the evidence-backed M5/M8 measured-fact corrections in `AGENTS.md` as required by [#199](https://github.com/autobrr/go-mediainfo/issues/199#issuecomment-6523304989); do not claim baseline-compatible zero-sign formatting as a new deliberate difference.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s projection contract. GitHub currently records [#53](https://github.com/autobrr/go-mediainfo/issues/53) as blocked by [#52](https://github.com/autobrr/go-mediainfo/issues/52); preserve that native blocker until reconciled on the issues. The [shared-pattern decision](https://github.com/autobrr/go-mediainfo/issues/53#issuecomment-6522588788) resolves the design question but does not remove the native edge.

**Coordination and exclusions:** [Plan 20](plan-20.md) owns format-settings summary bindings. Containers retain duration/size ownership; no generic HEVC framework is required.

## Validation and evidence

- [ ] Complete the [acceptance matrix and PR evidence checklist](README.md#acceptance-matrix-and-pr-checklist) for this plan’s existing criteria and the checks below.

[Shared validation requirements](README.md#validation-and-evidence). Require changing/dual-mono metadata, independently checked statistics populations and actual dependent/JOC controls across reachable containers. Retain the established TS/BDAV comparison and non-JOC/zero/absent/public lifecycle tests.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
