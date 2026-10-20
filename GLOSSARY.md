# go-mediainfo

go-mediainfo reads media files and reports their streams and metadata in the output formats of official MediaInfo.

## Language

### DVD

**Title set**:
The files of one DVD title: an IFO, its BUP backup, an optional menu VOB, and up to nine title VOBs.
_Avoid_: VTS (in prose)

**Title VOB**:
One of the numbered VOB files that carry a title's program, from `_1.VOB` to `_9.VOB`.
_Avoid_: part, chunk, segment

**Joining**:
Reporting several files of a set as one input, with a combined size and a last file name.
_Avoid_: aggregating, merging (in prose)
