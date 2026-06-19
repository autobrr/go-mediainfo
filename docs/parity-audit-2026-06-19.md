# go-mediainfo parity audit — 2026-06-19

**Question asked:** is this rewrite 1:1 with MediaInfo, and where is the original "lacking/ancient"?
**Bar:** release media only (MKV / MP4 / TS / M2TS-BDAV / DVD-VOB / common audio).
**Reference:** official `mediainfo --Output=JSON --Language=raw --ParseSpeed=0.5` (MediaInfoLib **v23.04**) vs `go-mediainfo --output=JSON --language=raw`, both run on the same files on `root@media` (Linux x86_64).

---

## TL;DR

The port is **much closer to 1:1 than not**, and it is **dramatically faster and more robust** than the original:

- **0 crashes, 0 timeouts** across 48 real files (incl. 44 GB MKV, BD50 M2TS, 4K HDR/DV).
- **go is 5–50× faster** than official at ParseSpeed=0.5 (e.g. Oppenheimer 19 GB: go **80 ms** vs official **4 582 ms**; Cape Fear 4K HDR/DV: go **1.5 s** vs official **12.1 s**).
- **Track detection is essentially perfect** — exact track counts on every file except `.mpls` (unimplemented), one TS caption track, and m4b chapters.
- After removing one systematic *non-bug* artifact (below): **7/48 byte-perfect, 16 within 2 fields, 22 within 5 fields.**
- The biggest, most visible single bug: **`General.StreamSize` is 40–50× too large** on a class of MKVs (a stream-size derivation gap), e.g. 11.7 GB reported where official says 248 MB.
- Modern-codec coverage is where it's genuinely "ancient-adjacent": **no AV1 support at all.**

This is a solid base. The remaining parity gaps cluster into ~16 well-defined items, most small, a few high-leverage.

---

## The one artifact that inflates every diff

On **100% of files**, go emits two fields official (on Linux) does not:

```
+ "File_Created_Date":       "2022-07-25 18:10:53 UTC"
+ "File_Created_Date_Local": "2022-07-25 18:10:53"
```

MediaInfoLib v23.04 on Linux leaves file *creation* time blank (it historically couldn't read birth-time); go reads it and emits it. This is go being **more complete**, not wrong — but it breaks strict 1:1. It is the reason the known-good TS controls (`Nickelodeon`, `Evermoor`) and all three DVD IFOs score "2" instead of "0". **All numbers below have this artifact removed.**

> **Decision needed:** keep `File_Created_Date` (more correct) or suppress it under a strict-parity mode/flag. Recommend: keep, but gate behind a `--strict-parity`/platform toggle so 1:1 comparisons are clean.

---

## Method

- Built `go-mediainfo` for linux/amd64, ran **both tools next to the files** (no large transfers).
- Per file: normalize both JSONs (`jq -S 'del(.creatingLibrary)'`), then a Python analyzer that **aligns tracks by @type/StreamOrder/ID** (not array index — verified order matches) and classifies every leaf field as go-missing / go-extra / value-mismatch.
- Disk-polite: `ionice -c3 nice -n10`, ParseSpeed=0.5, sequential, 240 s timeout guard.
- Raw artifacts: `/tmp/parity-audit-local/` (JSON pairs, diffs, `analysis.json`, `summary.tsv`).

**Sample (48 files):** 22 movie MKV (x264, every audio kind — AC-3/E-AC-3/DD+ 5.1&7.1/DTS/DTS-ES/DTS-HD MA/FLAC/Atmos), 2× 2160p HEVC+Dolby Vision+HDR+Atmos, 3 movie MP4, 3 TV MKV, F1+UFC web, 4 broadcast TS (incl. Nick/Evermoor controls), 4 BDAV M2TS (AVC + LPCM), 2 `.mpls`, 3 DVD IFO + 1 VOB, 1 m4b, 1 mp3.

---

## Scorecard (true field-diff, created-date artifact removed)

| true_diff | files |
|--:|---|
| **0 (perfect)** | `mp4_ac3`, `mkv_dts_d` (38-track!), `dvd_ifo_ntsc/1995/pal`, `ts_ctrl_nick`, `ts_ctrl_evermoor` |
| 1–2 | Cape Fear **4K HDR/DV** (×2), `mkv_atmos_web`, `mkv_tv_eac3`, `mp4_ac3_c`, `mkv_dts_b/c`, `mkv_tv_eac3_20`, `m2ts_hollywood_short` |
| 3–8 | most M2TS, `mkv_dtshdma`, `mkv_dts`, `mkv_eac3_51`, `mkv_eac3_51b`, `mkv_ufc_h264`, `audio_mp3`, … |
| 13–29 | `mkv_ac3_51`, `mkv_eac3_71/71b`, `mkv_dts_e/es`, `mkv_nordic`, `mkv_x264_untagged`, `mp4_ufc_aac`, `mkv_f1_x264`, `mkv_tv_ac3_720`, `mp4_ac3_b` |
| 38–75 | `dvd_vob_ntsc` (38), `mkv_flac` (70, 56 tracks), `mkv_eac3_71c` (75, 40 tracks) |
| track-count gaps | `.mpls` (1 vs 202/32 — unimplemented), `ts_ctn_thursdays` (Text 4 vs 5), `m4b` (Menu 1 vs 2 + spurious Text) |

The high scorers are **track-count × per-track-field** multipliers (a sub-heavy 40/56-track file multiplies one missing per-track field into dozens of diffs), not 40 distinct bugs.

---

## Ranked findings

Legend — **Class:** BUG (go wrong/missing) · QUIRK (official questionable, go arguably better) · LONGTAIL (known tuning) · FEATURE (unimplemented). **Conf:** code+data verified / empirical.

### Tier 1 — clear correctness bugs (fix first)

**1. `General.StreamSize` 40–50× too large; `Video.BitRate`/`Video.StreamSize` missing** — *BUG, code-verified, HIGH impact, effort M*
Files: Iron Man 2, F1, Star Wars IV, Dag 720p (4/22 large MKVs). Audio stream sizes are correct, so this is purely video: when the cluster scan doesn't directly measure video bytes, go emits **neither** `Video.StreamSize` nor `Video.BitRate`, so `General.StreamSize` collapses to ≈ whole file (e.g. **11 688 475 047** vs official **248 645 459**). Iron Man 2 even has the answer — go parsed `BitRate_Nominal=12500000` from x264 settings but never promoted it to `BitRate` or derived StreamSize from it.
*Fix:* in the Matroska size path (`matroska_scan.go` ~876–1013 + General remainder calc), when direct video bytes are unavailable, derive `Video.StreamSize` as the **remainder** (FileSize − overhead − audio − text) and back-compute `BitRate` — the remainder approach already exists for BDAV; apply it here. Promote `BitRate_Nominal`→`BitRate` when that's all there is.

**2. DTS-HD: `Format="DTS XLL"` should be `"DTS"`** — *BUG, code-verified, MED impact, effort S+M*
`matroska_scan.go:1277` literally sets `Format = "DTS XLL"`; official keeps `Format="DTS"` and puts the extension in `Format_AdditionalFeatures`. go also emits `"XLL"` where official emits `"ES XCh XLL"` — it never detects the DTS-ES core extension (XCh).
*Fix:* stop concatenating into `Format` (keep `"DTS"`); build `Format_AdditionalFeatures` from core-extension (XCh/ES) + ExSS (XLL/XBR). The `Format`-string change is trivial; XCh detection is the work.

**3. `Audio.Format_Settings_Endianness="Big"` missing on plain AC-3** — *BUG, code-verified, MED-HIGH impact, effort S*
The Matroska audio enrichment sets Endianness for E-AC-3 (`matroska_scan.go:1490`) and DTS (`:1393`) but **not plain AC-3** — so every AC-3 MKV track drops it. One-line addition in the AC-3 branch. (E-AC-3 7.1 also drops it, but via bug #5.)

**4. `Video.FrameRate_Num`/`FrameRate_Den` never emitted for Matroska** — *BUG, code-verified, MED impact, effort S*
`json_fields.go:359` only emits Num/Den for `MPEG-4 | MPEG-TS | BDAV`; **Matroska isn't in the allowlist**, so every CFR MKV is missing them (The Pianist=24, Sopranos=25, F1=50, Dag=25). Add Matroska to the gate. *(Separate value-bug: MP4/DVD NTSC fraction — UFC mp4 `FrameRate_Num=30` vs official `29970`; go didn't represent 29.97 as a fraction. Effort M.)*

### Tier 2 — parity-vs-correctness (needs your call)

**5. DD+ 7.1: go `Format="E-AC-3"` vs official `"AC-3"` + `Dep` — and this drops the AC-3-core fields** — *QUIRK/BUG, code-verified, HIGH impact, effort M-L*
Highest-leverage item. The Gentlemen / Sinners / Apocalypse Now (all DD+ **7.1**). Official treats these as **AC-3 core + dependent E-AC-3 substream** (`Format="AC-3"`, `Format_AdditionalFeatures="Dep"`), which is *why* official emits `Endianness`, `compr_*`, `dialnorm`, `SamplesPerFrame` on them — they're AC-3-core BSI fields. go labels the track flat `E-AC-3` and therefore **drops all of those** (confirmed: 5.1 E-AC-3 like Oppenheimer is byte-perfect; only the 7.1 "core+substream" case breaks). One root cause explains the Format mismatch *and* ~5 missing fields per file, on the **most common modern release audio**.
*Decision:* match official (AC-3 + Dep, parse the core) for parity and to recover the BSI fields — **recommended** — vs keep the arguably-more-correct flat `E-AC-3` and just add the missing fields.

**6. `Format_Commercial_IfAny` missing for DTS-ES Discrete & HE-AAC; `Format_AdditionalFeatures` misses `ES XCh`/`SBR`** — *BUG, empirical, MED impact, effort M*
go's commercial-name machinery works (it nails DTS-HD MA, DD+) but lacks the DTS-ES-Discrete and HE-AAC cases because it under-detects the DTS **XCh** core extension and AAC **SBR** (so it reports `LC` not `LC SBR`, never realizing it's HE-AAC). Shares root with #2.

**7. `Audio.ChannelLayout` ordering + DTS-ES `*_Original` channels** — *BUG, empirical, MED impact, effort M*
7.1: official `L R C LFE Ls Rs Lb Rb` vs go `C L R Ls Rs Lb Rb LFE` (different convention). DTS-ES: official emits `Channels_Original`/`ChannelLayout_Original`/`ChannelPositions_Original` (matrixed 6.1→5.1) that go omits; conversely go *adds* `ChannelLayout` on some DTS tracks official leaves bare.

**8. `Video.BitRate_Maximum` over-emitted** — *BUG (go-extra), empirical, LOW-MED, effort S*
Gladiator II, Casablanca, DVD VOB, Network M2TS: go emits a max bitrate official omits for these container/codec combos.

### Tier 3 — long-tail / known tuning

**9. AC-3 `extra.compr_*`/`dynrng_*` stats** — *LONGTAIL, MED impact, effort M-L*
On **Matroska** (not just the TS windows AGENTS.md tracks), go captures essentially one frame under ParseSpeed=0.5: Forrest Gump `compr_Count=1` vs official **2090**. The TS-side windowing is well-tuned; the MKV-side AC-3 stats sampling is the bigger remaining miss.

**10. `Audio.FrameCount` gating** — *BUG, empirical, LOW-MED* — emitted inconsistently (AAC tracks missing it; some AC-3 commentary tracks emit it where official doesn't). ParseSpeed-dependent gating differs from official in both directions.

**11. `Text.MuxingMode="zlib"` missing** — *BUG, empirical, LOW* — zlib-compressed Matroska subtitle tracks (Oppenheimer) lose the MuxingMode field.

**12. `Text.Duration`/`Text.Language`/`Text.extra.Source` on sub-heavy files** — *mixed, LOW per-file but inflates totals* — Sinners (40 subs) & Silence of the Lambs (56 tracks) drive the two worst scores via one or two per-track fields × many tracks.

**13. `Audio.BitRate` rounding** — *LONGTAIL, LOW* — go rounds AC-3 to clean kbps (160000) vs official exact (159880); cosmetic.

### Structural / unimplemented (bigger pieces)

**14. Blu-ray `.mpls` playlists — effectively unimplemented** — *FEATURE, HIGH for disc workflows, effort XL*
go emits **1** track; official aggregates the playlist (Network `.mpls`: 202 tracks = 100 Video + 100 Audio + Menu; Hollywood `.mpls`: 32). A real parser needs PlayList/PlayItem/STN_table → resolve referenced clip `.m2ts` → aggregate streams + chapter marks. Single largest structural gap.

**15. MPEG-TS one caption track short** — *BUG, LOW, effort M* — `ts_ctn_thursdays` Text 4 vs 5 (a missing caption service on certain Cartoon Network captures).

**16. M4B chapters** — *BUG, LOW, effort M* — audiobook `.m4b`: Menu 1 vs 2 and a spurious Text track (go doesn't fold the QuickTime text chapter track into Menu the way official does).

---

## "Lacking / ancient" — modernization candidates

In scope (release media), where the original or the port is dated:

- **AV1 — zero support in the port** (no `AV1`/`av01` anywhere in source). This is the one modern codec actively appearing in releases (WEB/anime) that go can't read at all. **Highest-value modern add** (sequence-header/OBU parser). Effort L.
- **AC-4** and **MPEG-H 3D Audio** — 0 support. Niche today (ATSC 3.0 / some streaming); low priority for this library.
- **VP9 / Opus / AVIF** — codec-ID-level only; WebM mostly rides the Matroska path. Worth fleshing out Opus (appears in anime/WEB) parsing for bit-exact parity.
- Out of scope but absent: HEIF/JXL/WebP/Theora/FFV1/ProRes.

Correctness-over-legacy-quirks candidates (where go can be *better* than v23.04, behind a flag if 1:1 matters):
- `File_Created_Date` on Linux (go already more complete).
- DD+ → `E-AC-3` labelling (go's flat label is arguably more correct than official's `AC-3`+`Dep`; see #5 for the trade-off).

---

## Recommended order (value ÷ effort)

1. **#3 AC-3 Endianness** (one line) · **#4 MKV FrameRate_Num/Den** (one gate) · **#2 DTS `Format` string** (one line) — three near-free, broadly-applied parity wins.
2. **#1 Video StreamSize remainder/nominal-promote** — kills the single most visible bug (11.7 GB→248 MB).
3. **#5 DD+ 7.1 AC-3-core+Dep** — biggest *bundle* (Format + 5 fields) on the most common release audio.
4. **#6 DTS-ES/HE-AAC detection** (XCh + SBR) → fixes commercial names + additional features together.
5. Then #7 channel layout, #8 bitrate-max, #9 MKV AC-3 stats, #10–#13 long-tail.
6. **Separate projects:** `.mpls` parser (#14, XL) and **AV1 support** (modernization, L).

Quick wins #1–#4 alone should move a large fraction of the 22 movie MKVs toward 0–2.

---

## Appendix — code-level root causes (file:line)

Verified by reading the Go source (and, for 8 items, an independent root-cause pass cross-checked against the upstream MediaInfoLib v23.04 C++; the `File_*.cpp` references below are from that pass).

**#1 StreamSize collapse** *(my analysis + 2026-06-19 experiment — deferred to its own PR)*
- Affects **untagged** MKVs only. Tagged releases (mkvmerge `NUMBER_OF_BYTES`/`BPS` Statistics Tags) get exact per-track sizes with no scan; untagged files (Iron Man 2, F1, SW-IV, Dag) hit `shouldApplyMatroskaClusterStats` (matroska.go:515) which returns false at ParseSpeed<1, so `applyMatroskaStats` (matroska_scan.go:862) never runs → no `Video.StreamSize`/`BitRate`, and `General.StreamSize` (= FileSize − Σtracks, analyze.go:491) collapses to ≈ full file. The video nominal→BitRate promotion (matroska_scan.go:1013) is *also* gated behind that scan, so Iron Man 2 keeps `BitRate_Nominal` but emits no `BitRate`.
- **Experiment result (rejected approach):** forcing the cluster scan for untagged files is (a) catastrophically slow — **52 s (Iron Man 2), 169 s (F1)** vs official 1.7 s — and (b) **not byte-exact**: go's block-byte sum = 11.685 GB vs official `StreamSize` 11.439 GB. So official does **not** sum raw block bytes.
- **Real fix:** official derives `Video.StreamSize = FileSize − Σaudio − overhead`, where overhead ≈ 1.99% on both sampled files (248 MB / 147 MB) and is computed *cheaply* (no full scan). `BitRate` = nominal if present (Iron Man 2: 12.5 M, exact) else `StreamSize×8/Duration` (F1: 5362756). Implementing this means replicating MediaInfo's Matroska overhead model (EBML/cluster framing) and feeding video into the existing remainder math (analyze.go:1134–1148 already does this for BDAV). M-effort; needs validation across a wider untagged-MKV set than this audit's 4 files. **Not a quick win — split out.**

**#2 DTS `Format="DTS XLL"`** — `matroska_scan.go:1277/1283` (sets Format to "DTS XLL"/"DTS XBR"); leaks to JSON because `splitAACFormat` (`json_fields.go:575`) only strips an `"AAC "` prefix. ES/XCh lost because `parseDTSCoreFrame` (`matroska_scan.go:2152-2153`) discards the Extension-Audio-Descriptor + Extended-Coding bits; no XCh-sync detection. Same AdditionalFeatures loss in TS path `mpeg_ts.go:2231-2236`. Upstream always `Format="DTS"` (`File_Dts.cpp:715`).

**#3 AC-3 Endianness** — gate `if probe.format == "E-AC-3"` at `matroska_scan.go:1489` excludes plain AC-3. Fix: move `stream.JSON["Format_Settings_Endianness"]="Big"` to the common AC-3/E-AC-3 section (~1467); the DTS path at `:1393` is the correct unconditional pattern.

**#4 FrameRate_Num/Den** — fragmented: `json_fields.go:227` only lifts Num/Den from a text token; fallback `json_fields.go:357` gated to MPEG-4/TS/BDAV (excludes Matroska); `format_rates.go:12` drops integer Num/Den (only emits when den>1); `rationalizeFrameRate` `json_fields.go:498` tolerance too tight + emits 30000/1001 not 29970/1000; DVD hardcode `dvd.go:270`; pulldown gap `mpeg2_video.go:661`. Fix: one central `deriveFrameRateNumDen(rate)` (port of `File__Analyze_Streams.cpp:2266-2298`) for every video stream.

**#5 DD+ 7.1 `Dep`** — `matroska_codec.go:19-20` hard-maps `A_EAC3→"E-AC-3"`; `ac3.go:503/559` reads `strmtyp` but never records dependent-substream presence; `eac3_joc.go:76-86` reads `numDepSub` then discards it; emission slot exists (`json_field_order.go:145`). Fix: record `strmtyp==1` (dependent substream) through the probe, emit `Format_AdditionalFeatures="Dep"` and match official's `Format="AC-3"`; this also restores the AC-3-core BSI fields.

**#6 DTS-ES / HE-AAC commercial** — emission machinery works (`json_fields.go:187-188`); signal detected but not surfaced: AAC SBR at `matroska.go:2991` only feeds `Format_Settings_SBR`; DTS ES/XCh discarded in `parseDTSCoreFrame`. Add "DTS-ES Discrete" / "HE-AAC" commercial-name cases.

**#7 ChannelLayout** — count-based lookup `channel_layout.go:18-21` (`8→"C L R Ls Rs Lb Rb LFE"`) instead of bitstream arrangement; E-AC-3 7.1 reaches the count fallback because `ac3ChannelLayout` (`ac3.go:1204-1242`) caps at 5.1 and `chan_loc` (`eac3_joc.go:81`) is read only for the JOC bed mask. Don't touch the shared count map; build the real layout from acmod+chan_loc. Effort L.

**#11 Text MuxingMode "zlib"** — `matroska.go:1298` only emits MuxingMode for algo 3 (Header stripping); Text tracks get `Compression_Mode="Lossless"` (`matroska.go:1713-1716`) instead of `MuxingMode="zlib"`; `parseMatroskaContentCompression` (`:1828`) returns false when no explicit algo child (drops implicit zlib default). Note `matroska_test.go:156-181` currently asserts the buggy behavior — update it. Effort S.

**Empirical-only (root-cause agent rate-limited, classified from output data):** #8 BitRate_Maximum over-emit, #9 MKV AC-3 stats window, #10 FrameCount gating, #12 Text Source/Duration, #14 `.mpls`, #15 TS caption track, #16 m4b chapters. Happy to deep-dive any on request.
