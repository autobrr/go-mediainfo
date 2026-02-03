# TODO

## Parity (1:1 MediaInfo CLI)
- Goal: 1:1 MediaInfo CLI parity (fields + values + ordering) across text/JSON/XML/CSV
- Expand format coverage + field parity across CLI outputs
- Sample parity complete: MP4/MKV/TS/AVI/MPEG-PS (VOB)/MPEG Video (MPG) (text output)
- Next targets: sample sweep (PS/TS edge cases), parity audit for JSON/XML/CSV
- Remaining diffs: JSON/XML/CSV parity audit, broader sample sweep for edge cases
- Implement MediaInfo JSON/XML/CSV schema parity (raw field names/values, missing fields, exact formatting)
- JSON parity: MP4 + MKV + TS done; VOB/AVI remaining (UniqueID, delays, colors, streamable, raw sizes)
- MPEG-PS/VOB JSON: expand VOB parity sweep beyond samples (sample.vob + sample_ac3.vob match)

## Post-parity
- Investigate MediaInfo issue #760: DVD IFO language/runtime regression (23.07 vs 23.06). https://github.com/MediaArea/MediaInfo/issues/760
- Regression notes: 23.07 IFO output mirrors VOB output, missing language/runtime + menu detail present in 23.06
- Behavior from issue: IFO inside VIDEO_TS yields VOB-derived stream details but loses audio language; same IFO copied elsewhere shows language like 23.06
- Behavior from issue: BUP files now present the old IFO-style info (languages) in 23.07+
- Next step: reproduce with IFO/BUP/VOB samples + dev snapshot; document delta vs 23.06
- Reproduce with IFO+VOB sample set; compare language/runtime fields and menu/chapter listings across versions
