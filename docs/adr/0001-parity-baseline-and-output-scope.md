# 0001: Parity baseline and output scope

Status: accepted, 2026-10-18.

## Context

An external contributor ran go-mediainfo and official MediaInfo against a corpus of 3173 files. The run compared all six output formats against MediaInfo v23.04, and the presence of each field against MediaInfo v26.10. It found 1812 difference patterns.

The two MediaInfo versions do not always agree. Some fields that go-mediainfo outputs are missing from v23.04 but present in v26.10. Some fields go the other way. Without a rule, each fix must decide again which version is correct.

The six formats also have different users. upbrr uses go-mediainfo as a library. It reads fields from the JSON output, and it pastes the text output into tracker uploads. No known user parses the XML, HTML, or CSV output.

## Decision

1. MediaInfo v23.04 stays the reference that the parity harness compares against.
2. If v26.10 differs from v23.04 and the v26.10 behavior is more correct, go-mediainfo can follow v26.10. Each such field gets one line with the reason in the "Deliberate differences" list in `AGENTS.md`. Fields of one family can share a line. Without a recorded reason, the v23.04 behavior applies.
3. Parity work fixes differences in the JSON and text outputs. A difference that shows only in XML, HTML, or CSV counts only if it has the same root cause as a JSON or text difference.

## Consequences

An agent that finds a field that matches v26.10 but not v23.04 reads the "Deliberate differences" list first. If the field is on the list, the agent keeps it.

Rule 3 sets the order of the work. It does not allow a change to the XML, HTML, or CSV schema. The output shape stays compatible with MediaInfo in all formats.

A move of the reference to a newer MediaInfo version needs a new ADR.
