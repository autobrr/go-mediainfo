# Plan 24: Existing PR252

[Initiative index](README.md) · Stable plan ID 24

## Goal and scope

Preserve the final-report-size fix delivered by merged PR #252 for #250. Do not create a replacement or expand this completed group into the whole rendering bundle.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#250](https://github.com/autobrr/go-mediainfo/issues/250) | Render friendly IFO File size from final report size | [Discussion 1](https://github.com/autobrr/go-mediainfo/issues/250#issuecomment-6334757330), [Discussion 2](https://github.com/autobrr/go-mediainfo/issues/250#issuecomment-6511791926) |

## Acceptance criteria

- [x] Friendly IFO File size follows final report size after payload selection/aggregation.
- [x] Keep raw width/normalization and all assertions; share the renderer's friendly padding helper.
- [x] Pin the official Windows x64 v26.10 artifact/runtime and retain the bounded three-witness comparison evidence.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** Existing implementation; preserve its verified merged result.

**Coordination and exclusions:** No replacement PR or enlargement into Plan 01. Merge is a verified outcome, not a release claim.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. The retained three actual DVD witnesses C000013/C000027/C000122 were compared in normal text, raw text, JSON, XML, HTML and CSV (18 fresh captures). Output matched the prior fixed candidate. This evidence is bounded, not complete DVD parity.

[PR #252](https://github.com/autobrr/go-mediainfo/pull/252) contains the verified reference identity and bounded validation. Preserve known language/ID differences. The [review](https://github.com/autobrr/go-mediainfo/pull/252#pullrequestreview-5718506358) and [padding finding](https://github.com/autobrr/go-mediainfo/pull/252#discussion_r4428363352) explain the final test/body follow-up.

## Implementation and outcome history

Completed by [PR #252](https://github.com/autobrr/go-mediainfo/pull/252), merged in [3fde7a4](https://github.com/autobrr/go-mediainfo/commit/3fde7a44b3df8797734fabb3b56458cc57fdc2ff) on 2026-11-02. [#250](https://github.com/autobrr/go-mediainfo/issues/250) is closed completed. This records merge, not release.

The separate [final follow-up review and cleanup](final-review-and-cleanup.md) follows completion of all 24 implementation groups; Plan 24's numeric position does not make this merged PR the cleanup owner.

2026-11-02: recorded verified #252 merge and #250 completion; retained the existing implementation instead of budgeting a replacement PR.
