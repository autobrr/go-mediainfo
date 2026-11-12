# Final follow-up: cumulative review and repository cleanup

[Initiative index](README.md) · Separate stage after Plans 01–24

## Goal and scope

After all 24 existing plans have been implemented, use a separate follow-up PR to review the cumulative initiative, check regressions with a completely fresh run on every available original file in both the October corpus and the separate music corpus authorized in [#261](https://github.com/autobrr/go-mediainfo/issues/261), and remove the temporary plan work from the repository. This stage adds no numbered implementation plan or closing target. The implementation plans retain their records until this follow-up is ready.

This documentation PR does not start that review PR or corpus sweep. No qualifying final full-corpus result is claimed here.

## Prerequisites

- Reconcile all 24 plan outcomes, owning issue decisions, implementation PRs and actual integration state. Verify cross-plan acceptance, including partial contributions such as #58; do not treat deferred research or an open required scope as delivered.
- Identify the exact cumulative candidate, current main and recorded pre-initiative baseline. Required implementation changes must be present in the tested candidate; unmerged work is not silently counted as integrated. Preserve concurrent unrelated work and existing owner holds.
- Confirm the final format/mode matrix, argv, normalization and comparison rules against current repository guidance and the agreed harness workflow. Resolve any consequential open gate decision, including #81, before claiming final acceptance.
- Check the two-open-Audionut-PR cap before publication. This later stage does not authorize creating a third PR, merging other work or removing a hold for capacity.

## Cumulative review

Review the combined initiative changes and their public API, parser, projection, compatibility, safety and data-integrity contracts. Distinguish new regressions, accepted deliberate differences and existing limitations using the canonical issue decisions and current AGENTS/ADRs. Inspect retained tests and evidence rather than assuming earlier approvals cover the cumulative candidate.

Address supported findings minimally, preserve required behavior and obtain independent review. Record each consequential finding's fix or evidence-backed disposition. A cleanup label does not authorize removing necessary tests, parser tables, durable guidance or safeguards, or changing unrelated policy.

## Fresh complete-corpus gate

The gate requires new candidate executions on **all available original media files in both corpora**, with every inventory entry accounted for. Adding the music corpus must not replace or reduce October coverage; unavailable entries remain explicit gaps and cannot count as a complete all-files pass. Selected witnesses, synthetic substitutes, playable derivatives, cached prior outputs and earlier successful sweeps cannot satisfy it.

1. Verify both canonical original inventories and their separate identities/counts before scheduling. The historical October inventory contains 3,173 files and #261 records 403 music files; reconcile each actual manifest rather than assuming old counts prove completeness. Account for every original entry in each corpus, including currently unsupported containers. Keep October files (C IDs), music files (MUS IDs), supplementary samples (S IDs) and derivatives distinct; never relabel supplementary or derived evidence as an original witness.
2. Verify actual availability and source identity against each inventory, preserving required pre/post metadata checks. Record missing, unreadable, changed or otherwise unavailable originals by sanitized ID. Do not shrink the denominator to available or supported files. Any missing, unreadable or unexecuted original blocks a complete all-files pass.
3. Pin candidate and baseline source commit/tree, executable hashes and build/runtime identity. Verify the applicable official reference artifact/platform, package and executable SHA-256, runtime version and source revision. Apply the then-current [AGENTS guidance](../../../AGENTS.md#parity-harness), [ADR 0001](../../adr/0001-parity-baseline-and-output-scope.md) and [ADR 0002](../../adr/0002-dvd-parity-reference.md), with any policy change explicitly resolved.
4. Generate fresh candidate and applicable official-reference captures for every original entry in the agreed matrix, with exact recorded argv and bounded execution receipts. Execute any baseline used for the regression comparison freshly too. Record actual exit/error outcomes for unsupported inputs; an expected unsupported result is distinct from a file that was never executed. Do not assume same-named output modes are equivalent.
5. Write the campaign into a distinct result location tied to its exact candidate identity. Preserve frozen references, previous runs and source media unchanged. Verify source/reference/binary identities and output hashes before accepting the results. Historical captures remain context, not replacements for the fresh final run.
6. Compare the fresh results using the agreed normalization, preserving field presence, order, duplicates and numeric facts. Classify differences against accepted scope, protected behavior and documented deliberate differences. Investigate regressions and unexplained changes; do not normalize away a failure or silently adopt a new convention.
7. Record each corpus's original inventory count, expected configurations/captures, freshly executed counts, successes, failures, expected unsupported outcomes and every missing/unreadable/unexecuted file. Reconcile totals separately for each corpus and in aggregate, and report all blockers and limitations. Publish sanitized aggregate/necessary field evidence and regression dispositions; keep raw captures, media names, release names, local paths, metadata values and the source map private.

The final exact candidate must satisfy this full-inventory gate for both corpora. A later consequential behavior change requires a renewed full-inventory check on the revised candidate; an affected subset alone cannot replace final acceptance. Unresolved regressions or incomplete original-file execution prevent claiming a successful final gate.

## Cleanup acceptance

- [ ] All existing implementation-plan acceptance and integration outcomes are reconciled, with research/partial dispositions explicit.
- [ ] Cumulative review findings are fixed or have supported dispositions, and the exact candidate has independently reviewed changes and applicable checks.
- [ ] The fresh complete-corpus campaign reconciles every original file and required configuration in both corpora, with verified identities, fresh captures, comparisons, coverage counts and regression dispositions. Missing/unreadable/unexecuted originals and unresolved regressions block completion.
- [ ] Remove the temporary `docs/plans/parity-consolidation/` directory, including this stage record, in the separate follow-up PR. Preserve durable AGENTS/ADR guidance, necessary tests, provenance data and public evidence.
- [ ] Check incoming repository, issue and PR links. Update active pointers to durable issue/PR evidence or commit-pinned historical plan links so removal leaves no broken references. Preserve other authors' content and immutable history; add a scoped clarification where an old external reference cannot be edited.
- [ ] Preserve the final campaign identity, sanitized coverage/results, limitations, finding dispositions and cleanup record in the follow-up PR and relevant issue evidence before removing the temporary files. Verify the actual merge outcome separately; do not claim released behavior from a merge.

## Implementation and outcome history

Not started. No follow-up PR, final corpus execution or cleanup is performed by the current plan-documentation change.

2026-11-02: added the separately approved final review/full-corpus/cleanup stage after the 24 existing implementation groups. This supersedes assigning cleanup to the last implementation PR, without changing plan IDs, issue ownership or global repository policy.

2026-11-12: expanded the final campaign to retain the October corpus and include the separate music inventory under #261. Missing coverage remains explicit; the existing #81 decisions about gate operation, reference recapture and run frequency still require separate resolution. This addition does not start the final campaign or activate #229.
