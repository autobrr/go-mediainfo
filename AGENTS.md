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

## Notes
- Update this file as we learn more about CLI behavior, formats, and edge cases.
