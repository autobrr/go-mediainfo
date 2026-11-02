# Plan 07: AVC HEVC video truth

[Initiative index](README.md) · Stable plan ID 07

## Goal and scope

Consolidate existing HEVC facts and projections alongside validated SPS profile/level/tier, Matroska field-order/storage integration, AVC fixed-flag policy, SEI timecode/AFD, MVC profile and HRD buffer selection.

## Issue ownership and canonical decisions

This plan is the closing owner for the targets below once their complete accepted requirements pass. Linked issue bodies and discussions are authoritative; titles alone can describe an older proposal.

| Issue | Accepted work area | Canonical scope and decisions |
| --- | --- | --- |
| [#52](https://github.com/autobrr/go-mediainfo/issues/52) | Consolidate HEVC facts/projection with source presence and precedence | [Discussion](https://github.com/autobrr/go-mediainfo/issues/52#issuecomment-6522526231) |
| [#109](https://github.com/autobrr/go-mediainfo/issues/109) | report VFR when `fixed_frame_rate_flag` is 0 | [Discussion](https://github.com/autobrr/go-mediainfo/issues/109#issuecomment-6521531512) |
| [#143](https://github.com/autobrr/go-mediainfo/issues/143) | read FieldOrder for scan type, scan order and store method | [Discussion](https://github.com/autobrr/go-mediainfo/issues/143#issuecomment-6520608609) |
| [#154](https://github.com/autobrr/go-mediainfo/issues/154) | write the HEVC tier and the VC-1 level inside Format profile | [Discussion](https://github.com/autobrr/go-mediainfo/issues/154#issuecomment-6520167906) |
| [#155](https://github.com/autobrr/go-mediainfo/issues/155) | take profile, level and tier from the SPS | [Discussion](https://github.com/autobrr/go-mediainfo/issues/155#issuecomment-6520143197) |
| [#237](https://github.com/autobrr/go-mediainfo/issues/237) | take the SEI first-frame time code with the official selection rule | [Discussion](https://github.com/autobrr/go-mediainfo/issues/237#issuecomment-6517157213) |
| [#238](https://github.com/autobrr/go-mediainfo/issues/238) | show Active Format Description | [Discussion](https://github.com/autobrr/go-mediainfo/issues/238#issuecomment-6517126581) |
| [#240](https://github.com/autobrr/go-mediainfo/issues/240) | read the MVC profile instead of a fixed string | [Discussion](https://github.com/autobrr/go-mediainfo/issues/240#issuecomment-6517083301) |
| [#244](https://github.com/autobrr/go-mediainfo/issues/244) | Project parsed NAL/VCL HRD values in order, preserving equal duplicates | [Discussion](https://github.com/autobrr/go-mediainfo/issues/244#issuecomment-6512045746) |

## Acceptance criteria

- [ ] Apply one private HEVC fact/projection owner with accepted SPS/configuration precedence, presence and provenance.
- [ ] Keep profile/level/tier, colour/HDR and encoder source selection separate from container geometry/timing; support required legacy configuration and separated fields.
- [ ] Implement settled #244 NAL-then-VCL projection, preserving equal duplicates and visibility; do not guess #253's deferred SPS-selection rule.

The issue-specific acceptance and exclusions linked above remain required; a shared helper alone is not completion. Record full versus partial delivery for each target using the [scoped workflow](README.md#scoped-workflow).

## Dependencies and non-goals

**Required contracts:** [Plan 01](plan-01.md), [Plan 05](plan-05.md), [Plan 06](plan-06.md) and the encoder interface agreed with [Plan 04](plan-04.md).

**Coordination and exclusions:** [#253](https://github.com/autobrr/go-mediainfo/issues/253) owns changing-SPS Matroska HRD selection and is not a blocker for retained #244 projection. [Plan 22](plan-22.md) owns picture/GOP analysis.

These describe integration contracts, not new native issue-dependency edges. Follow [live blocker handling](README.md#execution-order-and-dependencies) before execution; this plan does not override an owner hold or create one for unrelated work.

## Validation and evidence

Apply the [initiative validation requirements](README.md#validation-and-evidence) and the current repository guidance. Use conflicting hvcC/SPS, legacy configuration, malformed/later NAL, configuration/in-band SEI and separated-field controls; test canonical/direct/Analyze/rebuild routes and dual/absent HRD visibility.

No new implementation or corpus run is claimed by this plan record. The linked discussions contain the prior source/witness evidence and its limits.

## Implementation and outcome history

Not implemented by this documentation change. Re-read the linked issues and live dependencies before starting.

2026-11-02: migrated the approved consolidation grouping; preserved closing ownership and current issue decisions.
