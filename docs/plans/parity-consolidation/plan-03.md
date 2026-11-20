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

The checked implementation criteria record delivered behavior; the merge-ready evidence checklist remains incomplete.

- [ ] Complete the [acceptance matrix and PR evidence checklist](README.md#acceptance-matrix-and-pr-checklist) for this plan’s existing criteria and the checks below.

[Shared validation requirements](README.md#validation-and-evidence). Test all imported mappings, whole-tag case, suffixes, undefined/unknown members, Macintosh gaps and elng order/bounds. Compare non-DVD language witnesses with v23.04 and DVD/backup controls with v26.10; keep corrected code-128 evidence distinct from parity.

For Plan 03, map language inputs through actual container ingress, normalization, stream/Menu assignment and output projection. Include Macintosh/elng ordering and bounds, whole-tag suffix spelling versus friendly names, positional unknown Menu members, and DVD backup/high-byte controls already required above. Check exact language identity/value and member count/order, not merely whether a language name appears somewhere. Where the changed path supports public Report reconstruction or edits, check JSON/XML and friendly/raw projection after those operations; a normalizer-only fixture cannot establish container or lazy-projection behavior. Keep code-128 correction evidence separate from convention parity. This validation refinement adds no language taxonomy or implementation scope and does not change the owner's stage-start authorization or holds.

## Implementation and outcome history

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.

2026-11-14: prepared a local implementation candidate covering the shared versioned language policy, Macintosh/elng ingress, Ogg logical-stream comments, track-backed chapter language, positional TS Menu/caption display and DVD byte sanitization. Parser-to-report regressions include public reconstruction, edits/deletions and lazy render order. Verification follows the owner's additional [unmerged #265 matrix guidance](https://github.com/autobrr/go-mediainfo/blob/05051af3c7b5a88121d98b091ba57962ad065e82/docs/plans/parity-consolidation/README.md#acceptance-matrix-and-pr-checklist), alongside current main policy.

The local continuation passed full Go tests, vet, four generator tests, all 21 three-second mutation-fuzz smoke targets and dependency/format checks on unchanged frozen-08 runtime content. Fresh candidate/current-main/pinned-reference comparisons completed 555 invocations across JSON, XML, friendly text, raw text and HTML: 30 available October originals, five music controls and two separately identified derivatives. Scoped comparisons preserve non-language media fields and rows against current main; HTML checks ordered table rows and XML excludes the creating-library metadata. Unrelated reference gaps remain separately recorded. This is targeted evidence, not whole-report reference parity, a nightly fuzz campaign or a full-corpus run. Four previously inaccessible originals are now readable. Two original whole-tag witnesses remain unavailable; three restored readable originals have fresh comparisons but their historical byte identity cannot be established. The owner accepts publication with those five evidence gaps; they remain recorded limits. The code-128 real-payload control verifies `cy`/Welsh against the tested runtime candidate and Apple's numeric table, so its deliberate-difference entry is recorded in `AGENTS.md`. ADR 0001 and ADR 0002 remain unchanged because their correctness, reference and output policies already cover the correction. Publication does not claim issue closure or satisfy the separate full-corpus gate. Plan 10 retains final #58 closure and #229 remains held.

2026-11-14 review follow-up: constrained `ftyp` brand reads to the declared box, resolved complete hyphenated language names before suffix parsing, and preserved repeated Ogg LANGUAGE comments in producer order. All-undefined lists now omit Language consistently after public edits and reconstruction. Focused/full tests, vet, dependency/format checks and all 21 native Go fuzz smoke targets pass on the new runtime, using three seconds per target with no tolerated failures. A controlled replay reproduces the regressions on the prior production code. Fresh scoped comparisons ran 160 successful invocations over seven originals and one retained derivative, five formats and four binaries: the new candidate, applicable pinned reference, prior candidate and current-main baseline. All 40 normalized prior-candidate comparisons match; all 40 non-language preservation checks against main pass. Reference language/identity vectors match seven of eight cases per format, with the approved Welsh correction kept separate. These checks do not relabel the earlier 555 captures as validation of new runtime bytes. Generator/table inputs and policy files are unchanged in this review follow-up; matching earlier generator/table results are reused. Acceptance uses the owner's explicit five-gap disposition, without claiming recovered originals or full-corpus parity. The separate full-corpus gate, Plan 10's final #58 closure and #229 hold remain unchanged.

2026-11-16: [PR #266](https://github.com/autobrr/go-mediainfo/pull/266) is open and unmerged at [`66d919782a3368d637dbf367c24396e81262dd20`](https://github.com/autobrr/go-mediainfo/commit/66d919782a3368d637dbf367c24396e81262dd20). This records existing Plan 03 work; no new runtime or corpus validation is claimed.

The separate November 16 corpus receipt compares base `10fabcc6f1b30ad2414df24d0326bca9b845266c` with candidate `66d919782a3368d637dbf367c24396e81262dd20`, covering 3,558 readable originals out of 3,576 across both corpora. Its strict result remains blocked by pre-existing malformed XML/OLDXML on 24 inputs and duplicate JSON chapter-timestamp keys on one. The invalid outputs were byte-identical between the tested base and candidate; equality does not establish structured-output validity. Those defects remain outside Plan 03 scope. This historical receipt does not satisfy the merged corpus policy for a later runtime or waive its fresh Go base/candidate gate over both available corpora.

2026-11-17 review preparation: a new local candidate preserves the selected DVD
language policy through public edits, keeps ambiguous Fields-only names as text,
and restores the pinned original-text fallback for names whose reverse lookup
exceeds three characters. Ogg ingress handles Director details and bracketed
codes; raw suffix formatting shares the friendly display rule without selecting
a second reference version. RIFF/FORM signatures take precedence over MP4 atom
sniffing. Exact-code regressions and 13 decoded authored AAC controls document
the existing Apple mappings for code 50 and codes 128–138, with 139 omitted, in
`AGENTS.md`. These controls are additional evidence, not recovered originals.
Both ADRs remain unchanged: their correctness, reference and output rules already
cover this work.

The owner's separate November 16 corpus run tested published head `66d9197`,
not this new runtime. It completed 49,812 full-corpus Go captures, 28 derivative
Go captures and 259 targeted official captures, but strict acceptance remained
blocked by malformed XML/OLDXML on 24 originals and duplicate JSON timestamp keys
on one. The earlier five original-evidence limits remain recorded. The new local
candidate still needs its own applicable [corpus gate](https://github.com/autobrr/go-mediainfo/blob/29cb6281a0cee35adc024e3743e6a826c4ab68e8/docs/plans/parity-consolidation/README.md#validation-and-evidence);
targeted controls do not establish full-corpus acceptance. Plan 10 retains final
#58 closure and #229 remains held.

2026-11-20 local integration: merged main `ce28ad9` and retained its corpus policy alongside both dated Plan 03 histories. Imported the review candidate without changing its runtime/test/table/script/module bytes. The November 19 checks and authored-control captures remain evidence for that exact runtime, not a new full-corpus result. Integrated checks and review are recorded separately; the merge-ready base/candidate corpus gate remains pending. No new corpus run, native-blocker waiver or issue closure is implied.

2026-11-20 CodeRabbit review follow-up: the local candidate fixes TS/MP4 signature
precedence, blank `elng` budget consumption, Ogg language-detail retention after
public edits, retained DVD policy for lazy General XML, opaque raw-language text,
and empty positions within a single Ogg comment. Friendly-text checks now require
the complete Language row. The existing IFO-only fixture confirms canonical `en`
and friendly `English`; ordinary known-code paths already seed the code, so the
DVD fallback and intentional aggregate suppression are unchanged. Full Go tests,
vet, module checks and all 21 native three-second fuzz targets pass on this new
runtime. Ten newly authored controls produced 150 successful five-format captures
against the candidate, published `590fcc4` and verified v23.04. These are scoped
comparisons, not whole-report reference equality. `AGENTS.md` records the two
observed corrections: retaining the trailing empty Ogg member and recognizing the
valid TS controls that v23.04 misdetects. Both ADRs remain unchanged because their
reference, measured-fact and output-shape rules already govern these corrections.
Authored controls do not replace original witnesses. The full corpus remains
paused, and this new runtime still requires its own merge-ready corpus gate;
the prior strict-output defects and original-evidence limits remain unresolved.
