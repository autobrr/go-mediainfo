# Plan 03: Language normalization

[Initiative index](README.md) · Stable plan ID 03

## Goal and scope

Use one primary-language normalizer and versioned name/display policy, with Macintosh/elng ingress, chapter language, TS member-by-member Menu names and DVD high-byte sanitization.

## Issue ownership and canonical decisions

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

- [x] Use one versioned name/whole-tag table and canonical primary-code policy; keep serialized suffix spelling separate from friendly display.
- [x] Decode Macintosh/elng at bounded ingress, carry chapter-track language and preserve positional Menu list members.
- [x] Preserve valid backup/payload language while rejecting DVD high-byte data. Pin the independently supported Macintosh code-128 correction under existing ADR rules.
- [x] Add the evidence-backed deliberate-difference entry in `AGENTS.md` required by [#131](https://github.com/autobrr/go-mediainfo/issues/131#issuecomment-6521090739) for the correctly decoded Macintosh code 128; preserve reserved/unmapped omissions.

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md)'s versioned presentation contract; supplies Ogg language to [Plan 10](plan-10.md)'s final #58 integration.

**Coordination and exclusions:** [Plan 10](plan-10.md) owns final #58 closure; caption discovery and chapter membership remain in their own plans.

## Validation and evidence

[Shared validation requirements](README.md#validation-and-evidence). Test all imported mappings, whole-tag case, suffixes, undefined/unknown members, Macintosh gaps and elng order/bounds. Compare non-DVD language witnesses with v23.04 and DVD/backup controls with v26.10; keep corrected code-128 evidence distinct from parity.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.

2026-11-14: prepared a local implementation candidate covering the shared versioned language policy, Macintosh/elng ingress, Ogg logical-stream comments, track-backed chapter language, positional TS Menu/caption display and DVD byte sanitization. Parser-to-report regressions include public reconstruction, edits/deletions and lazy render order. Verification follows the owner's additional [unmerged #265 matrix guidance](https://github.com/autobrr/go-mediainfo/blob/05051af3c7b5a88121d98b091ba57962ad065e82/docs/plans/parity-consolidation/README.md#acceptance-matrix-and-pr-checklist), alongside current main policy.

The local continuation passed full Go tests, vet, four generator tests, all 21 three-second mutation-fuzz smoke targets and dependency/format checks on unchanged frozen-08 runtime content. Fresh candidate/current-main/pinned-reference comparisons completed 555 invocations across JSON, XML, friendly text, raw text and HTML: 30 available October originals, five music controls and two separately identified derivatives. Scoped comparisons preserve non-language media fields and rows against current main; HTML checks ordered table rows and XML excludes the creating-library metadata. Unrelated reference gaps remain separately recorded. This is targeted evidence, not whole-report reference parity, a nightly fuzz campaign or a full-corpus run. Four previously inaccessible originals are now readable. Two original whole-tag witnesses remain unavailable; three restored readable originals have fresh comparisons but their historical byte identity cannot be established. The owner accepts publication with those five evidence gaps; they remain recorded limits. The code-128 real-payload control verifies `cy`/Welsh against the tested runtime candidate and Apple's numeric table, so its deliberate-difference entry is recorded in `AGENTS.md`. ADR 0001 and ADR 0002 remain unchanged because their correctness, reference and output policies already cover the correction. Publication does not claim issue closure or satisfy the separate full-corpus gate. Plan 10 retains final #58 closure and #229 remains held.

2026-11-14 review follow-up: constrained `ftyp` brand reads to the declared box, resolved complete hyphenated language names before suffix parsing, and preserved repeated Ogg LANGUAGE comments in producer order. All-undefined lists now omit Language consistently after public edits and reconstruction. Focused/full tests, vet, dependency/format checks and all 21 native Go fuzz smoke targets pass on the new runtime, using three seconds per target with no tolerated failures. A controlled replay reproduces the regressions on the prior production code. Fresh scoped comparisons ran 160 successful invocations over seven originals and one retained derivative, five formats and four binaries: the new candidate, applicable pinned reference, prior candidate and current-main baseline. All 40 normalized prior-candidate comparisons match; all 40 non-language preservation checks against main pass. Reference language/identity vectors match seven of eight cases per format, with the approved Welsh correction kept separate. These checks do not relabel the earlier 555 captures as validation of new runtime bytes. Generator/table inputs and policy files are unchanged in this review follow-up; matching earlier generator/table results are reused. Acceptance uses the owner's explicit five-gap disposition, without claiming recovered originals or full-corpus parity. The separate full-corpus gate, Plan 10's final #58 closure and #229 hold remain unchanged.
