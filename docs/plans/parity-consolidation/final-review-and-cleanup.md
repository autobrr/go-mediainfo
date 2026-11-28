# Final follow-up: cumulative review and repository cleanup

[Initiative index](README.md) · Separate stage after Plans 01–24

## Goal and scope

After all 24 existing plans have been implemented, use a separate follow-up PR to review the cumulative initiative, check regressions with a completely fresh run on every available original file in the October, [#261 music](https://github.com/autobrr/go-mediainfo/issues/261) and [Matroska AV1](README.md#matroska-av1-corpus) corpora, and remove the temporary plan work from the repository. This stage adds no numbered implementation plan or closing target. The implementation plans retain their records until this follow-up is ready.

This documentation PR does not start that review PR or corpus sweep. No qualifying final full-corpus result is claimed here.

## Prerequisites

- Reconcile all 24 plan outcomes, owning issue decisions, implementation PRs and actual integration state. Verify cross-plan acceptance, including partial contributions such as #58; do not treat deferred research or an open required scope as delivered.
- Identify the exact cumulative candidate and its PR base/current main. Retain the recorded pre-initiative baseline as context, not a replacement for the PR-base comparison. Required implementation changes must be present in the tested candidate; unmerged work is not silently counted as integrated. Preserve concurrent unrelated work and existing owner holds.
- Confirm the final format/mode matrix, argv, normalization and comparison rules against current repository guidance and the owner-approved [#81 corpus-validation policy](https://github.com/autobrr/go-mediainfo/issues/81#issuecomment-6909285169). Any later consequential policy change needs its own explicit resolution.
- Check the two-open-Audionut-PR cap before publication. This later stage does not authorize creating a third PR, merging other work or removing a hold for capacity.

## Cumulative review

Review the combined initiative changes and their public API, parser, projection, compatibility, safety and data-integrity contracts. Distinguish new regressions, accepted deliberate differences and existing limitations using the canonical issue decisions and current AGENTS/ADRs. Inspect retained tests and evidence rather than assuming earlier approvals cover the cumulative candidate.

Address supported findings minimally, preserve required behavior and obtain independent review. Record each consequential finding's fix or evidence-backed disposition. A cleanup label does not authorize removing necessary tests, parser tables, durable guidance or safeguards, or changing unrelated policy.

The [shared acceptance matrix and PR checklist](README.md#acceptance-matrix-and-pr-checklist) also apply to this cumulative candidate. Include actual entry-point and relevant edited-report/render-order probes alongside whole-file captures; a full CLI sweep does not by itself exercise public API reconstruction. Preserve exact identity/value/cardinality/order checks and legitimate duplicates. These targeted checks supplement, never replace, the fresh inventory-wide gate below.

## Fresh complete-corpus gate

Following the accepted [corpus-validation policy](https://github.com/autobrr/go-mediainfo/issues/81#issuecomment-6909285169), freshly capture the Go base and exact merge-ready candidate on **all available original media files in all three corpora**, accounting for every inventory entry. Reuse verified frozen official outputs under that policy; the fresh inventory-wide run is Go-only unless a reference/version/options change requires affected official recaptures. Preserve each inventory and its coverage. Selected subsets, derivatives and earlier Go captures cannot replace this run.

1. Verify the three canonical original inventories and their separate identities/counts before scheduling. Reconcile the actual manifests for the 3,173 October entries, 403 music entries and three Matroska AV1 originals. Account for every original entry in each corpus, including currently unsupported containers. Keep October files (C IDs), music files (MUS IDs), Matroska AV1 files (AV1 IDs), supplementary samples (S IDs) and derivatives distinct; never relabel supplementary or derived evidence as an original witness.
2. Verify actual availability and source identity against each inventory, preserving pre/post metadata checks. List absent originals as gaps without failing the gate or shrinking an inventory. A present file that cannot be opened blocks the gate until access is fixed; an available but unexecuted file also blocks it. Record changed source identity and other evidence limits separately.
3. Pin base/candidate source commits/trees, executable hashes and build/runtime identity. Retain verified official artifact/platform, package/executable hashes, runtime/source revision and saved-capture argv. For every run, use current [AGENTS options](../../../AGENTS.md#parity-harness): official `--ParseSpeed=0.5` and no `--File_TestContinuousFileNames=0` override. Preserve [ADR 0001](../../adr/0001-parity-baseline-and-output-scope.md) and [ADR 0002](../../adr/0002-dvd-parity-reference.md).
4. Generate fresh Go base and candidate captures for every available original in text, raw text, raw JSON and raw XML; add HTML, CSV or OLDXML when their rendering behavior changes. Reuse frozen official outputs with compatible verified identity/options. Recapture only affected official files when reference binary, version or options change; if saved DVD witnesses used `--File_TestContinuousFileNames=0`, recapture them once with v26.10 and current options. Unknown saved options remain an evidence gap until verified. Record exact argv, bounded receipts and actual unsupported/error outcomes; never count an unexecuted file as an expected unsupported result.
5. Write the campaign into a distinct result location tied to its exact candidate identity. Preserve frozen references, previous runs and source media unchanged. Verify source/reference/binary identities and output hashes. Earlier Go captures cannot replace the fresh base/candidate run; frozen official reuse is intentional and must retain its provenance.
6. Compare base and candidate with the agreed normalization, preserving presence, order, duplicates and numeric facts. List every changed non-witness file with a one-line reason; unexplained changes block acceptance. Keep targeted official comparisons, protected behavior and evidence-backed deliberate differences distinct; do not normalize away a failure.
7. Record each corpus's original inventory count, configurations, witness/control/full-run counts, successes, failures, expected unsupported outcomes and all absent/inaccessible/unexecuted files. Reconcile totals separately and in aggregate. Publish the [concise PR evidence block](README.md#acceptance-matrix-and-pr-checklist) with longer sanitized detail collapsed; keep raw captures, media names, release names, paths, private metadata and the source map private.

The final exact candidate must satisfy this available-corpus gate for all three inventories. A later output-changing fix requires renewed affected witness checks and fresh base/candidate corpus comparison at the revised merge-ready head. Absent originals remain explicit gaps; inaccessible present files, available unexecuted files, unexplained differences and unresolved regressions block acceptance. Passing with reported absences does not claim recovery of every historical original or complete native parity.

## Cleanup acceptance

- [ ] All existing implementation-plan acceptance and integration outcomes are reconciled, with research/partial dispositions explicit.
- [ ] Cumulative review findings are fixed or have supported dispositions, and the exact candidate has independently reviewed changes and applicable checks.
- [ ] Fresh base/candidate Go captures cover every available original and required format in all three corpora, with verified identities/options, frozen-reference provenance, explained changes and reconciled counts. Absences are reported gaps; inaccessible present files, available unexecuted files, unexplained differences and unresolved regressions block completion.
- [ ] Remove the temporary `docs/plans/parity-consolidation/` directory, including this stage record, in the separate follow-up PR. Preserve durable AGENTS/ADR guidance, necessary tests, provenance data and public evidence.
- [ ] Check incoming repository, issue and PR links. Update active pointers to durable issue/PR evidence or commit-pinned historical plan links so removal leaves no broken references. Preserve other authors' content and immutable history; add a scoped clarification where an old external reference cannot be edited.
- [ ] Preserve the final campaign identity, sanitized coverage/results, limitations, finding dispositions and cleanup record in the follow-up PR and relevant issue evidence before removing the temporary files. Verify the actual merge outcome separately; do not claim released behavior from a merge.

## Implementation and outcome history

Not started. No follow-up PR, final corpus execution or cleanup is performed by the current plan-documentation change.

2026-11-02: added the separately approved final review/full-corpus/cleanup stage after the 24 existing implementation groups. This supersedes assigning cleanup to the last implementation PR, without changing plan IDs, issue ownership or global repository policy.

2026-11-12: expanded the final campaign to retain the October corpus and include the separate music inventory under #261. At that entry, #81 decisions about gate operation, reference recapture and run frequency still required separate resolution. This addition did not start the final campaign or activate #229.

2026-11-16: adopted the owner-approved [#81 corpus-validation policy](https://github.com/autobrr/go-mediainfo/issues/81#issuecomment-6909285169): fresh Go base/candidate coverage of both available corpora, targeted witness rechecks, frozen official reuse, current AGENTS options, four required outputs and explained non-witness changes. Absent files remain gaps; inaccessible present files block the gate. No campaign, cleanup, issue closure or #229 start is claimed.

2026-11-27: added the separate three-original Matroska AV1 inventory to the final gate. Preserve all three inventories and report their results separately.
