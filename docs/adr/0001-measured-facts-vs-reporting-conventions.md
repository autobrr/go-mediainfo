# Measured facts are correct, reporting conventions follow official MediaInfo

go-mediainfo puts each output value in one of two groups. A measured fact must be correct, also when official MediaInfo reports a different value. A reporting convention follows official MediaInfo v23.04, also when a different choice is defensible.

A measured fact is a value that a file contains or that we count or measure from it. Examples: sizes, counts, durations, sample counts, bit rates, stream presence, metadata, and errors.

A reporting convention is a choice about how to show a value. Examples: rounding, snapping a frame rate to a standard cadence, num/den display, units, and the stream that General takes a value from.

## Why

Many users of this output upload to trackers. Tracker staff compare the MediaInfo output of an upload with the output of official MediaInfo. A convention difference makes a correct upload look edited, and the uploader gets into trouble with staff. A wrong fact is bad data for every user. So we match official where the difference is only presentation, and we are correct where the value is a fact. upbrr parses this output, and it needs parity where it counts for the same reason.

We compare against v23.04 so that diffs stay comparable. When a newer MediaInfo version changes a convention on purpose, we can adopt it. Each adoption gets one line in the list of deliberate differences in `AGENTS.md`. We use no other tracking system.

## Considered options

- Accuracy everywhere. We rejected it. A convention difference, for example 25.001 FPS where official shows 25.000 FPS, has no value for a user and causes problems with tracker staff.
- Official everywhere, bugs included. We rejected it. It reverts the deliberate differences that fix wrong facts, for example the MP4 audio sample count and the AAC `Missing ID_END` error.

## Consequences

The first case was PR #62. Matroska statistics tags give 25.001 FPS for a file whose H.264 timing and DefaultDuration give exactly 25 FPS. Official snaps the rate to 25.000 FPS. Snapping is a convention, so go-mediainfo matches official, and it also matches the num/den, Nominal, Original, and General frame-rate values that official derives from the snapped rate. Durations, frame counts, and stream sizes stay as measured.
