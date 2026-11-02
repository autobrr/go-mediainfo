# Plan 03: Language normalization

[Initiative index](README.md) · Stable plan ID 03

## Goal and scope

Use one primary-language normalizer and versioned name/display policy, with Macintosh/elng ingress, chapter language, TS member-by-member Menu names and DVD high-byte sanitization.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#128](https://github.com/autobrr/go-mediainfo/issues/128) | use the MediaInfo table of language names | [Discussion](https://github.com/autobrr/go-mediainfo/issues/128#issuecomment-6521137033) |
| [#129](https://github.com/autobrr/go-mediainfo/issues/129) | follow the official rules for regions, scripts and unknown codes | [Discussion](https://github.com/autobrr/go-mediainfo/issues/129#issuecomment-6521121238) |
| [#130](https://github.com/autobrr/go-mediainfo/issues/130) | map `scc` and `mol`, and decide the case of subtags | [Discussion](https://github.com/autobrr/go-mediainfo/issues/130#issuecomment-6521109191) |
| [#131](https://github.com/autobrr/go-mediainfo/issues/131) | read the Macintosh language codes in `mdhd` | [Discussion](https://github.com/autobrr/go-mediainfo/issues/131#issuecomment-6521090739) |
| [#132](https://github.com/autobrr/go-mediainfo/issues/132) | write the chapter track language on the Menu | [Discussion](https://github.com/autobrr/go-mediainfo/issues/132#issuecomment-6521065120) |
| [#133](https://github.com/autobrr/go-mediainfo/issues/133) | show language names for captions and in the Menu list | [Discussion](https://github.com/autobrr/go-mediainfo/issues/133#issuecomment-6521054992) |
| [#135](https://github.com/autobrr/go-mediainfo/issues/135) | clear a language code that is not ASCII | [Discussion](https://github.com/autobrr/go-mediainfo/issues/135#issuecomment-6521021290) |

## Acceptance criteria

- [ ] Use one versioned name/whole-tag table and canonical primary-code policy; keep serialized suffix spelling separate from friendly display.
- [ ] Decode Macintosh/elng at bounded ingress, carry chapter-track language and preserve positional Menu list members.
- [ ] Preserve valid backup/payload language while rejecting DVD high-byte data. Pin the independently supported Macintosh code-128 correction under existing ADR rules.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s versioned presentation contract; supplies Ogg language to [Plan 10](plan-10.md)'s final #58 integration.

**Coordination and exclusions:** [Plan 10](plan-10.md) owns final #58 closure; caption discovery and chapter membership remain in their own plans.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Test all imported mappings, whole-tag case, suffixes, undefined/unknown members, Macintosh gaps and elng order/bounds. Compare non-DVD language witnesses with v23.04 and DVD/backup controls with v26.10; keep corrected code-128 evidence distinct from parity.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
