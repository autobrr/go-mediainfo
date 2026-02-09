# TODO

## Parity (1:1 MediaInfo CLI)
- Goal: 1:1 MediaInfo CLI parity (fields + values + ordering) across text/JSON/XML/CSV
- Expand format coverage + field parity across CLI outputs
- Samples: generated fixtures in `samples/` (see `samples/README.md` + `samples/generate.sh`)
- Sample parity complete: MP4/MKV/TS/AVI/MPEG Video (MPG) (text output)
- Next targets: sample sweep (PS/TS edge cases), parity audit for JSON/XML/CSV
- Remaining diffs: broader sample sweep for edge cases
- CSV parity: sample set now matches upstream (section headers, raw values, spacing, numbering)
- Implement MediaInfo JSON/XML/CSV schema parity (raw field names/values, missing fields, exact formatting) where still missing
- JSON parity: sample set complete (MP4/MKV/TS/AVI/MPEG Video)
- TS parity (real-world samples; not covered by `samples/sample.ts`):
  - ATSC PSIP-derived General metadata: Title/Movie/LawRating (e.g. `Disney Channel Movie`, `TV-PG`)
  - Per-stream `StreamSize`/`BitRate` deltas for MPEG-2 Video PID accounting (ours matches `TS payload - PES header`, official smaller)
  - AC-3 stats parity: `compr_*` / `dynrng_*` counts and extrema still differ on some broadcasts
- BDAV/M2TS parity (real-world clips):
  - General `FrameCount/TextCount/StreamSize` diffs, plus Video `StreamSize/BitRate` behavior
  - Likely shared root with TS PID size accounting; then BDAV-specific fields
- MPEG-PS/VOB JSON: VTS_02_1.VOB small diffs (General Duration/OverallBitRate, Video BitRate rounding, AC-3 extra stats, field order for MuxingMode/Delay_Source)
- MPEG-PS/VOB JSON: recheck sample_ac3.vob after AC-3 duration tweak (+1 frame)
- MPEG-PS/VOB: verify RLE subtitle Delay/Duration (first/last PTS) on more DVD samples
- MKV (The.Rookie... WEB-DL) parity complete across text/JSON/XML/CSV
- DVD: verify EIA-608 timing on more samples (Ask.Me.to.Dance IFO matched)
- DVD: verify CC frames-before-first-event count vs MediaInfo (currently derived from MPEG-2 picture count)
- DVD: run full directory parity on large DVD sets (long-running scan)
- DVD VOB/IFO: VTS_01_0.VOB audio stream detected as AAC instead of PCM; video duration/bitrate mismatch
- DVD IFO: menu output should include multiple Menu # entries (per PGC); we currently emit a single Menu
- DVD IFO: AC-3 dialnorm/compr/dynrng stat counts differ from upstream on VTS_02_0.IFO (likely sample/PTS handling)

## Post-parity
- Investigate MediaInfo issue #760: DVD IFO language/runtime regression (23.07 vs 23.06). https://github.com/MediaArea/MediaInfo/issues/760
- Regression notes: DVD IFO rework for ISO support; duration fixed in dev snapshots, but language issue persists
- Behavior: IFO inside VIDEO_TS yields VOB-derived stream details but loses audio language; same IFO copied elsewhere shows language like 23.06
- Behavior: BUP files now present the old IFO-style info (languages) in 23.07+
- Issue status: reported as still present in all versions from 23.07+
- Issue 760 details (gh): 23.07 IFO output mirrors VOB (durations/bitrate, Source VOB) and loses Language fields; 23.06 IFO output shows Language for audio/text and shorter IFO-only durations
- Issue 760 details (gh): 23.07 IFO shows VOB-derived stream details (GOP/settings/stream size) + Source VOB, while 23.06 IFO shows minimal stream info + Language
- Next step: reproduce with IFO/BUP/VOB samples + dev snapshot; document delta vs 23.06 and isolate path-based behavior
- Reproduce with IFO+VOB sample set; compare language/runtime fields and menu/chapter listings across versions
- Repro (Ask.Me.to.Dance.2022 DVD): `VIDEO_TS/VTS_02_0.IFO` inside VIDEO_TS lacks Language; copy outside VIDEO_TS shows Language=English; `VTS_02_0.BUP` shows Language even inside VIDEO_TS
