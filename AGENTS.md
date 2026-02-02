# AGENTS

Owner: soup

## Scope
- Goal: 1:1 feature parity with MediaInfo CLI
- Pure Go implementation, no CGO
- Cross-platform

## Learnings / Decisions
- Command name: mediainfo
- Parity target: MediaInfo-master in this repo
- CLI option parsing: case-insensitive before "=" only; values preserve case
- Unknown --Option[=Value] forwarded to core (default value "1" if missing)
- Help text copied from upstream CLI (Help.cpp)
- Stub core: parsing not implemented yet
- Upstream C++ tree kept untracked via root .gitignore
- Post-parity target: MediaInfo issue #760 (DVD IFO language/runtime regression)
- `--output` without "=" treated as filename (matches upstream)
- `--` alone is a no-op (ignored)
- `--help` prints version line then usage
- Core analyzer implemented (General stream only): format sniff + file size
- Text output aligns with MediaInfo-style label/value columns
- JSON output implemented (minimal MediaInfo-like shape)
- MP4/MOV: parse `moov/mvhd` for duration; compute overall bitrate
- Matroska: parse Segment/Info for duration using TimecodeScale
- MPEG-TS: parse PAT/PMT, map stream types, PTS-based duration
- MPEG-PS: scan PES for stream types + PTS-based duration
- Text output now numbers stream groups (e.g., Audio #1)
- MP4: track detection via `trak/mdia/hdlr` (vide/soun/text)
- Matroska: track detection via Tracks/TrackEntry/TrackType
- MP4: sample entry parsing (`stsd`) for codec format
- Matroska: codec mapping via CodecID
- MP4: parse width/height (video) and channels/sampling rate (audio) from sample entry
- Matroska: parse track video/audio settings (pixel width/height, channels, sampling rate)
- MP4: derive frame rate from mdhd duration + stts sample count
- Matroska: derive frame rate from DefaultDuration
- General/stream fields now sorted to match MediaInfo ordering
- MPEG-TS: estimate frame rate from video PES count vs PTS span
- MPEG-TS/MPEG-PS: add video stream duration + bitrate (best-effort)
- MPEG-PS: per-stream IDs surfaced; JSON multi-file now outputs media list
- MPEG-PS: estimate frame rate from video PTS count vs duration
- Bit rate mode now emitted when bitrate is computed
- TS/PS: per-stream PTS tracking for non-video durations
- Stream common helper added for duration/bitrate fields

## Notes
- Update this file as we learn more about CLI behavior, formats, and edge cases.
