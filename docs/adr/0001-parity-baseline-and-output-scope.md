# 0001: Parity baseline and output scope

Status: accepted, 2026-10-18.

## Context

An external contributor ran go-mediainfo and official MediaInfo against a corpus of 3173 files. The run compared all six output formats against MediaInfo v23.04, and the presence of each field against MediaInfo v26.10. It found 1812 difference patterns.

The two MediaInfo versions do not always agree. Some fields that go-mediainfo outputs are missing from v23.04 but present in v26.10. Some fields go the other way. Without a rule, each fix must decide again which version is correct.

The six formats also have different users. upbrr uses go-mediainfo as a library. It reads fields from the JSON output, and it pastes the text output into tracker uploads. No known user parses the XML, HTML, or CSV output.

Tracker staff compare the MediaInfo output of an upload with the output of official MediaInfo. A difference in presentation makes a correct upload look edited, and the uploader gets into trouble with staff. A wrong fact is bad data for every user.

## Decision

1. MediaInfo v23.04 stays the reference that the parity harness compares against.
2. go-mediainfo puts each output value in one of two groups:
   - A measured fact is a value that the file contains, or that go-mediainfo counts or measures from it. Examples: sizes, counts, durations, sample counts, bit rates, stream presence, metadata, and errors.
   - A reporting convention is a choice about how to show a value. Examples: rounding, snapping a frame rate to a standard cadence, num/den display, units, names, and the stream that General takes a value from.
3. A measured fact must be correct, also when official MediaInfo reports a different value.
4. A reporting convention follows v23.04, also when a different choice is defensible. When a newer MediaInfo version changes a convention on purpose, go-mediainfo can adopt the newer convention.
5. If v26.10 differs from v23.04, go-mediainfo can follow v26.10 only when the v26.10 behavior is more correct. A v26.10 field or value is more correct when it fixes a wrong v23.04 value, or when it shows real data from the file that v23.04 does not read. A convention that newer MediaInfo changed on purpose also qualifies. A guessed value, or a value fitted to sample files, is not more correct.
6. Each difference from v23.04 gets one line in the "Deliberate differences" list in `AGENTS.md`. The line starts with "Measured fact" or "Newer upstream convention" and gives the reason. Fields of one family can share a line. Without a line, the v23.04 behavior applies. The project uses no other tracking system.
7. Parity work fixes differences in the JSON and text outputs first. A difference that shows only in XML, HTML, or CSV comes after that work, unless it has the same root cause as a JSON or text difference. A wrong measured fact in any format is still a bug.

## Considered options

- Accuracy everywhere. We rejected it. A convention difference, for example 25.001 FPS where official shows 25.000 FPS, has no value for a user and causes problems with tracker staff.
- Official v23.04 everywhere, bugs included. We rejected it. It reverts the deliberate differences that fix wrong facts, for example the MP4 audio sample count and the AAC `Missing ID_END` error.

## Consequences

An agent that finds a field that matches v26.10 but not v23.04 reads the "Deliberate differences" list first. If the field is on the list, the agent keeps it.

Rule 7 sets the order of the work. It does not allow a change to the XML, HTML, or CSV schema. The output shape stays compatible with MediaInfo in all formats.

PR #62 is the first case of rule 4. Matroska statistics tags give 25.001 FPS for a file whose H.264 timing and DefaultDuration give exactly 25 FPS. Official snaps the rate to 25.000 FPS. Snapping is a convention, so go-mediainfo matches official. It also matches the num/den, Nominal, Original, and General frame-rate values that official derives from the snapped rate. Durations, frame counts, and stream sizes stay as measured.

A move of the reference to a newer MediaInfo version needs a new ADR.
