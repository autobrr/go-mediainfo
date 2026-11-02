# Plan 04: Encoder provenance

[Initiative index](README.md) · Stable plan ID 04

## Goal and scope

One source-aware encoder pipeline from actual strings/tags through Name/Version/Date and Writing library. Combine Matroska General/track routing, repeated tags, Vorbis/Theora vendor strings, AVC/HEVC/MPEG user data, MP4 manufacturer and compressor exclusions. #49 folds into #190.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#49](https://github.com/autobrr/go-mediainfo/issues/49) | Audio track "Writing library" formats date in "YYYYMMDD" when mediainfo uses "(YYYY-MM-DD)" | [Discussion](https://github.com/autobrr/go-mediainfo/issues/49#issuecomment-6523077111) |
| [#181](https://github.com/autobrr/go-mediainfo/issues/181) | keep General ENCODER and ENCODER_SETTINGS tags out of the video track | [Discussion](https://github.com/autobrr/go-mediainfo/issues/181#issuecomment-6519024558) |
| [#182](https://github.com/autobrr/go-mediainfo/issues/182) | set `Encoded_Library_Name` for encoder strings that Go does not know | [Discussion](https://github.com/autobrr/go-mediainfo/issues/182#issuecomment-6518987683) |
| [#183](https://github.com/autobrr/go-mediainfo/issues/183) | read every ATEME SEI string with the official name and version split | [Discussion](https://github.com/autobrr/go-mediainfo/issues/183#issuecomment-6518964871) |
| [#184](https://github.com/autobrr/go-mediainfo/issues/184) | read the user data encoder string with the official rule | [Discussion](https://github.com/autobrr/go-mediainfo/issues/184#issuecomment-6518937664) |
| [#185](https://github.com/autobrr/go-mediainfo/issues/185) | keep every video track ENCODER tag | [Discussion](https://github.com/autobrr/go-mediainfo/issues/185#issuecomment-6518906451) |
| [#186](https://github.com/autobrr/go-mediainfo/issues/186) | join every General ENCODER tag to Encoded_Library | [Discussion](https://github.com/autobrr/go-mediainfo/issues/186#issuecomment-6518875520) |
| [#187](https://github.com/autobrr/go-mediainfo/issues/187) | stop reading CodecName as Encoded_Library | [Discussion](https://github.com/autobrr/go-mediainfo/issues/187#issuecomment-6518803073) |
| [#188](https://github.com/autobrr/go-mediainfo/issues/188) | use one official vendor split, with the library table, for Ogg and Matroska | [Discussion](https://github.com/autobrr/go-mediainfo/issues/188#issuecomment-6518777404) |
| [#189](https://github.com/autobrr/go-mediainfo/issues/189) | hide the library name, version and date lines | [Discussion](https://github.com/autobrr/go-mediainfo/issues/189#issuecomment-6518748262) |
| [#190](https://github.com/autobrr/go-mediainfo/issues/190) | build Writing library from Name, Version and Date with the official rule | [Discussion](https://github.com/autobrr/go-mediainfo/issues/190#issuecomment-6518738586) |
| [#192](https://github.com/autobrr/go-mediainfo/issues/192) | fill the General library from the handler manufacturer | [Discussion](https://github.com/autobrr/go-mediainfo/issues/192#issuecomment-6518706707) |
| [#193](https://github.com/autobrr/go-mediainfo/issues/193) | read the vendor string from CodecPrivate | [Discussion](https://github.com/autobrr/go-mediainfo/issues/193#issuecomment-6518678699) |
| [#230](https://github.com/autobrr/go-mediainfo/issues/230) | leave Encoded_Library empty for the visual compressor name | [Discussion](https://github.com/autobrr/go-mediainfo/issues/230#issuecomment-6517371937) |

## Acceptance criteria

- [ ] Use actual source strings/tags for one Name/Version/Date and Writing library pipeline.
- [ ] Preserve ordered repeated/unknown vendor strings and distinct General/track ownership.
- [ ] Exclude CodecName/visual-compressor inference and compose numeric dates once; keep codec-specific parsing rules.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md) and [Plan 02](plan-02.md)'s metadata ownership interfaces.

**Coordination and exclusions:** [Plan 07](plan-07.md)'s HEVC producer ownership; do not duplicate its selection policy.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Test repeated vendors, unknown names, date splits, source/track conflicts and no-encoder controls through every reachable producer and public rebuilding.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
