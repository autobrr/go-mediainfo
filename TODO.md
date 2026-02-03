# TODO

## Parity (1:1 MediaInfo CLI)
- Goal: 1:1 MediaInfo CLI parity (fields + values + ordering)
- Expand format coverage + field parity across CLI outputs
- Sample parity complete: MP4/MKV/TS/AVI/MPEG-PS (VOB)/MPEG Video (MPG) (text output)
- Next targets: sample sweep (PS/TS edge cases), parity audit for JSON/XML/CSV
- Remaining diffs: JSON/XML/CSV parity audit, broader sample sweep for edge cases

## Post-parity
- Investigate MediaInfo issue #760: DVD IFO language/runtime regression (23.07 vs 23.06). https://github.com/MediaArea/MediaInfo/issues/760
- Reproduce with IFO+VOB sample set; compare language/runtime fields and menu/chapter listings across versions
