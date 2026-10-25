# 0002: DVD parity reference

Status: accepted, 2026-10-21.

## Context

Issue #248 adopts official MediaInfo v26.10 for all DVD output, not only
title-set joining and IFO reports. The maintainer confirmed this scope and
retained the rule that demonstrably wrong upstream values must be corrected.
The [v26.10 release](https://github.com/MediaArea/MediaInfo/releases/tag/v26.10)
fixes loss of IFO stream languages after VOB merge. PR #247, which removed
joining to match v23.04, was closed unmerged. Issue #248 supersedes #116.

ADR 0001 requires a new ADR when the reference version changes. This decision
changes the DVD reference only; it does not claim the implementation already
matches it.

## Decision

1. Official MediaInfo CLI with MediaInfoLib v26.10 is the reference for all DVD
   output in every supported output format. Non-DVD output keeps the v23.04
   reference and existing deliberate differences.
2. A measured fact must still be correct. Do not reproduce a demonstrably wrong
   v26.10 value for parity. Prove each correction with a real file, pin it with
   a regression test, explain it in the commit, and record it in the deliberate
   differences list in `AGENTS.md`. Existing evidence-backed corrections and
   safety rules remain in force.
3. Reporting conventions follow v26.10 for DVD. Preserve MediaInfo-compatible
   field names, schemas, text labels and ordering; do not invent output fields.
   ADR 0001's JSON/text-first work order still applies. Incorrect measured facts
   in any format remain bugs.
4. Before a comparison, identify the official reference artifact and platform,
   record the package and executable SHA-256 hashes, and run the executable with
   `--Version` to verify MediaInfoLib v26.10.
   Record source revisions for source-based comparisons or builds. A release
   label or download link alone is not executable verification. Keep the
   existing comparison options and normalization rules in `AGENTS.md`.
5. Preserve frozen corpus results. Write new verification results separately,
   recording the go-mediainfo commit, reference identity, sample IDs, commands
   and results. Keep media, release names, paths and the sample map private.
6. For incomplete DVD title sets, retain go-mediainfo's existing selection of
   present numbered members after a gap. The maintainer approved this convention
   difference: the verified Windows v26.10 witness stops before a gap in layouts
   1,3 and 1,2,4, but selects member 5 in layout 1,2,3,5. Both tools report accurate
   sizes for the members they select. This is not a demonstrated upstream
   arithmetic or safety defect. For this DVD gap-selection decision only, this
   overrides decision 3 above and ADR 0001's convention and three-category
   requirements in decisions 4 and 6: record the difference as
   "Maintainer-approved convention". Other differences retain the existing rules.

## Reference identity

The official release tags resolve to:

- MediaInfo: `f057dd3649bc8692a83863b67770e22c9b9dd81a`.
- MediaInfoLib: `153ad67a62172534b401e9d9a5752dc759fff77b`.

The official Windows x64 CLI package is
[MediaInfo_CLI_26.10_Windows_x64.zip](https://mediaarea.net/download/binary/mediainfo/26.10/MediaInfo_CLI_26.10_Windows_x64.zip).
The [verified Windows reference record](https://github.com/autobrr/go-mediainfo/issues/248#issuecomment-6326999896)
confirms MediaInfoLib v26.10 at runtime and records these SHA-256 hashes:

- Executable: `35d5bdf5bd3ef0776c50e22492b4e5b0ee687f9c115d9b61f4feba3e881dfbf9`.
- Package: `4cc94bd272903914c04f4f9a9f53a6c94b5ea6c6454cf738d29fa182c41e8a29`.

Verify the artifact and runtime version when reusing it. Other platforms may
use their official v26.10 CLI artifact under the same verification rules.

## Consequences

DVD comparisons now use v26.10; non-DVD comparisons do not change. Previous
measurements from the build of PR #247 are not evidence about current main.
Parser and formatter changes still need test-first regressions and real-file
verification before publication. Adopting this reference does not by itself
establish complete DVD parity or resolve #248. The separate start hold for #229
remains unchanged.
