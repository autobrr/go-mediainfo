# TODO

## Parity (1:1 MediaInfo CLI)
- Goal: 1:1 MediaInfo CLI parity (fields + values + ordering) across text/JSON/XML/CSV
- Expand format coverage + field parity across CLI outputs
- Sample parity complete: MP4/MKV/TS/AVI/MPEG-PS (VOB)/MPEG Video (MPG) (text output)
- Next targets: sample sweep (PS/TS edge cases), parity audit for JSON/XML/CSV
- Remaining diffs: broader sample sweep for edge cases
- CSV parity: sample set now matches upstream (section headers, raw values, spacing, numbering)
- Implement MediaInfo JSON/XML/CSV schema parity (raw field names/values, missing fields, exact formatting) where still missing
- JSON parity: sample set complete (MP4/MKV/TS/AVI/MPEG Video/VOB)
- MPEG-PS/VOB JSON: expand VOB parity sweep beyond samples (sample.vob + sample_ac3.vob match)
- MPEG-PS/VOB: verify RLE subtitle Delay/Duration (first/last PTS) on more DVD samples
- MKV (The.Rookie... WEB-DL) parity complete across text/JSON/XML/CSV
- DVD: verify EIA-608 timing on more samples (Ask.Me.to.Dance IFO matched)
- DVD: verify CC frames-before-first-event count vs MediaInfo (currently derived from MPEG-2 picture count)
- DVD: run full directory parity on large DVD sets (long-running scan)

## Post-parity
- Investigate MediaInfo issue #760: DVD IFO language/runtime regression (23.07 vs 23.06). https://github.com/MediaArea/MediaInfo/issues/760
- Regression notes: DVD IFO rework for ISO support; duration fixed in dev snapshots, but language issue persists
- Behavior: IFO inside VIDEO_TS yields VOB-derived stream details but loses audio language; same IFO copied elsewhere shows language like 23.06
- Behavior: BUP files now present the old IFO-style info (languages) in 23.07+
- Issue status: reported as still present in all versions from 23.07+
- Issue 760 details (gh): 23.07 IFO output mirrors VOB (durations/bitrate, Source VOB) and loses Language fields; 23.06 IFO output shows Language for audio/text and shorter IFO-only durations
- Next step: reproduce with IFO/BUP/VOB samples + dev snapshot; document delta vs 23.06 and isolate path-based behavior
- Reproduce with IFO+VOB sample set; compare language/runtime fields and menu/chapter listings across versions
- Repro (Ask.Me.to.Dance.2022 DVD): `VIDEO_TS/VTS_02_0.IFO` inside VIDEO_TS lacks Language; copy outside VIDEO_TS shows Language=English; `VTS_02_0.BUP` shows Language even inside VIDEO_TS
