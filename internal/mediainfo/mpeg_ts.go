package mediainfo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

type tsStream struct {
	pid                 uint16
	programNumber       uint16
	streamType          byte
	kind                StreamKind
	format              string
	frames              uint64
	bytes               uint64
	pts                 ptsTracker
	lastPTS             uint64
	hasLastPTS          bool
	seenPTS             bool
	pesPayloadRemaining int
	pesPayloadKnown     bool
	hasAC3              bool
	ac3Info             ac3Info
	ac3Stats            ac3Info
	ac3Head             []ac3Info
	ac3Tail             []ac3Info
	ac3TailPos          int
	ac3TailFull         bool
	width               uint64
	height              uint64
	storedHeight        uint64
	mpeg2Parser         *mpeg2VideoParser
	mpeg2Info           mpeg2VideoInfo
	hasMPEG2Info        bool
	videoCCCarry        []byte
	videoFrameCount     int
	ccFound             bool
	ccOdd               ccTrack
	ccEven              ccTrack
	dtvcc               dtvccState
	dtvccServices       map[int]struct{}
	language            string
	videoFields         []Field
	hasVideoFields      bool
	audioProfile        string
	audioObject         int
	audioMPEGVersion    int
	audioRate           float64
	audioChannels       uint64
	hasAudioInfo        bool
	audioFrames         uint64
	// Number of parsed AC-3/E-AC-3 frames that contributed to metadata stats sampling (bounded by
	// MediaInfoLib ParseSpeed window logic).
	audioFramesStats uint64
	// Same as audioFramesStats, but counted only from the initial (head) scan window. MediaInfoLib
	// bases the 0.75*N+4 sampling size on the first pass, even if it later scans from the end.
	audioFramesStatsHead uint64
	audioSpf             int
	audioBitRateKbps     int64
	audioBitRateMode     string
	videoFrameRate       float64
	pesData              []byte
	audioBuffer          []byte
	audioStarted         bool
	videoStarted         bool
	writingLibrary       string
	encoding             string
	// MPEG-2 GA94 caption user data is reordered by temporal_reference in MediaInfoLib before
	// feeding the DTVCC/XDS parsers. We keep a small per-GOP buffer for XDS only.
	mpeg2CurTemporalReference uint16
	mpeg2XDSReorder           []mpeg2UserDataPacket
	// XDS parsing is per cc_type (field). MediaInfoLib uses one File_Eia608 parser per cc_type.
	xds            [2]eia608XDS
	xdsLawRating   string
	xdsLastTitle   string
	xdsTitleCounts map[string]int
}

func (s *tsStream) hasValidCEA608() bool {
	if s == nil {
		return false
	}
	if s.ccOdd.found && (s.ccOdd.firstCommandPTS != 0 || s.ccOdd.firstCommandFrame > 0 || s.ccOdd.firstType != "") {
		return true
	}
	if s.ccEven.found && (s.ccEven.firstCommandPTS != 0 || s.ccEven.firstCommandFrame > 0 || s.ccEven.firstType != "") {
		return true
	}
	return false
}

type mpeg2UserDataPacket struct {
	temporalReference uint16
	data              []byte
}

func ParseMPEGTS(file io.ReadSeeker, size int64, parseSpeed float64) (ContainerInfo, []Stream, []Field, bool) {
	return parseMPEGTSWithPacketSize(file, size, 188, parseSpeed)
}

// ParseBDAV parses BDAV/M2TS streams (192-byte packets: 4-byte timestamp + 188-byte TS packet).
func ParseBDAV(file io.ReadSeeker, size int64, parseSpeed float64) (ContainerInfo, []Stream, []Field, bool) {
	return parseMPEGTSWithPacketSize(file, size, 192, parseSpeed)
}

const tsPTSGap = 30 * 90000 // 30 seconds

// MediaInfoLib default: scan up to 64 MiB from the beginning and end of a TS when ParseSpeed<0.8.
// We use this window size for TS/BDAV AC-3 stats sampling to match official outputs at ParseSpeed=0.5.
const tsStatsMaxOffset = 64 * 1024 * 1024

func ptsDuration(t ptsTracker) float64 {
	if t.hasResets() {
		return t.durationTotal()
	}
	return t.duration()
}

type pcrTracker struct {
	min uint64
	max uint64
	ok  bool
}

func (t *pcrTracker) add(pcr27 uint64) {
	if !t.ok {
		t.min = pcr27
		t.max = pcr27
		t.ok = true
		return
	}
	if pcr27 < t.min {
		t.min = pcr27
	}
	if pcr27 > t.max {
		t.max = pcr27
	}
}

func (t pcrTracker) has() bool {
	return t.ok
}

func (t pcrTracker) durationSeconds() float64 {
	if !t.ok || t.max <= t.min {
		return 0
	}
	return float64(t.max-t.min) / 27000000.0
}

func addPTS(t *ptsTracker, pts uint64) {
	if t.has() && pts > t.last && (pts-t.last) > tsPTSGap {
		t.breakSegment(pts)
		return
	}
	t.add(pts)
}

func addPTSMode(t *ptsTracker, pts uint64, breakOnGap bool) {
	if breakOnGap {
		addPTS(t, pts)
		return
	}
	// Partial TS/BDAV scans intentionally skip large regions; don't treat missing sections as discontinuities.
	t.add(pts)
}

func findTSSyncOffset(file io.ReadSeeker, packetSize int64, tsOffset int64, size int64) (int64, bool) {
	// Some TS/M2TS files have leading junk bytes and are not packet-aligned at offset 0.
	// Find the first offset where multiple consecutive packets have the 0x47 sync byte.
	const need = int64(5)
	if packetSize <= 0 {
		return 0, false
	}
	probeLen := int64(1 << 20)
	if size > 0 && size < probeLen {
		probeLen = size
	}
	if probeLen < tsOffset+packetSize*need {
		return 0, false
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, false
	}
	probe := make([]byte, probeLen)
	n, _ := io.ReadFull(file, probe)
	if int64(n) < tsOffset+packetSize*need {
		return 0, false
	}
	limit := int64(n) - (tsOffset + packetSize*need)
	for i := int64(0); i <= limit; i++ {
		if probe[i+tsOffset] != 0x47 {
			continue
		}
		ok := true
		for j := int64(1); j < need; j++ {
			if probe[i+tsOffset+j*packetSize] != 0x47 {
				ok = false
				break
			}
		}
		if ok {
			return i, true
		}
	}
	return 0, false
}

func parseMPEGTSWithPacketSize(file io.ReadSeeker, size int64, packetSize int64, parseSpeed float64) (ContainerInfo, []Stream, []Field, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, nil, false
	}

	var primaryPMTPID uint16
	var primaryProgramNumber uint16
	pmtPIDToProgram := map[uint16]uint16{}
	var serviceName string
	var serviceProvider string
	var serviceType string
	streams := map[uint16]*tsStream{}
	streamOrder := []uint16{}
	pendingPTS := map[uint16]*ptsTracker{}
	var videoPTS ptsTracker
	var anyPTS ptsTracker
	videoPIDs := map[uint16]struct{}{}
	hasMPEGVideo := false
	var pcrPTS ptsTracker
	var pcrFull pcrTracker
	// PCR spans (per PCR PID) for MediaInfo-like overall bitrate precision.
	var pmtPointer int
	var pmtSectionLen int
	pmtAssemblies := map[uint16]*psiAssembly{}
	pcrPIDs := map[uint16]struct{}{}

	type pcrSpan struct {
		startPCR    uint64
		endPCR      uint64
		startOffset int64
		endOffset   int64
		over30s     bool
		ok          bool
	}
	pcrSpans := map[uint16]*pcrSpan{}

	const tsPacketSize = int64(188)
	if packetSize != tsPacketSize && packetSize != 192 {
		packetSize = tsPacketSize
	}
	isBDAV := packetSize == 192
	tsOffset := int64(0)
	if packetSize == 192 {
		tsOffset = 4
	}

	syncOff := int64(0)
	if off, ok := findTSSyncOffset(file, packetSize, tsOffset, size); ok {
		syncOff = off
	}

	// With ParseSpeed<0.8, MediaInfoLib scans bounded windows from the beginning/end of large TS/BDAV.
	// This avoids full sequential reads for multi-GB streams while still capturing PTS/PCR and metadata.
	partialScan := parseSpeed < 0.8 && size > 0 && size > 2*tsStatsMaxOffset
	ac3StatsBounded := parseSpeed < 0.8 && size > 0
	ac3StatsHeadBytes := int64(tsStatsMaxOffset)
	ac3StatsHeadLocked := false

	maybeLockAC3HeadByPCR := func(packetOffset int64) {
		if !ac3StatsBounded || ac3StatsHeadLocked || size <= 0 {
			return
		}
		// MediaInfoLib uses a time-based bound (~30s default) for the initial scan when ParseSpeed<0.8,
		// and may shrink the begin/end byte windows once all PCR streams have exceeded it.
		if packetOffset*2 >= size {
			return
		}
		if len(pcrSpans) == 0 {
			return
		}
		over := 0
		for _, sp := range pcrSpans {
			if sp != nil && sp.over30s {
				over++
			}
		}
		if over == 0 || over != len(pcrSpans) {
			return
		}
		if packetOffset < syncOff {
			return
		}
		rel := packetOffset - syncOff
		ac3StatsHeadBytes = min(int64(tsStatsMaxOffset), rel+packetSize)
		ac3StatsHeadLocked = true
	}

	// Per-PID payload byte counts (TS payload only, excludes BDAV prefix + TS header/adaptation).
	// Used to estimate overall overhead when we don't scan the full file.
	pidPayloadBytes := map[uint16]int64{}

	var tsPacketCount int64
	var psiBytes int64
	headStoppedEarly := false
	headScannedEnd := int64(0)
	scanRange := func(start, end int64, shrinkHead bool) bool {
		if start < 0 {
			start = 0
		}
		if size > 0 && end > size {
			end = size
		}
		if end <= start {
			return true
		}
		if _, err := file.Seek(start, io.SeekStart); err != nil {
			return false
		}
		r := io.Reader(file)
		if end > start {
			r = io.LimitReader(file, end-start)
		}
		reader := bufio.NewReaderSize(r, 1<<20)
		buf := make([]byte, packetSize*2048)
		var packetIndex int64
		carry := 0
		for {
			n, err := reader.Read(buf[carry:])
			if n == 0 && err != nil {
				break
			}
			n += carry
			packetCount := n / int(packetSize)
			for i := 0; i < packetCount; i++ {
				packet := buf[int64(i)*packetSize : (int64(i)+1)*packetSize]
				packetIndex++
				packetOffset := start + (packetIndex-1)*packetSize
				if shrinkHead {
					headScannedEnd = packetOffset + packetSize
				}
				// MediaInfoLib can shrink MpegTs_JumpTo_Begin based on PCR distance (~30s default at ParseSpeed<0.8),
				// and then stops parsing the head scan at the reduced byte window.
				if shrinkHead && ac3StatsHeadLocked && packetOffset >= syncOff+ac3StatsHeadBytes {
					headStoppedEarly = true
					headScannedEnd = packetOffset
					return true
				}
				if tsOffset+tsPacketSize > int64(len(packet)) {
					continue
				}
				ts := packet[tsOffset : tsOffset+tsPacketSize]
				if ts[0] != 0x47 {
					continue
				}
				tsPacketCount++
				pid := uint16(ts[1]&0x1F)<<8 | uint16(ts[2])
				payloadStart := ts[1]&0x40 != 0
				adaptation := (ts[3] & 0x30) >> 4
				payloadIndex := 4
				if adaptation == 2 || adaptation == 3 {
					adaptationLen := int(ts[4])
					payloadIndex += 1 + adaptationLen
				}
				if _, ok := pcrPIDs[pid]; ok {
					if pcr27, ok := parsePCR27(ts); ok {
						pcrFull.add(pcr27)
						// Keep legacy 90kHz PCR base for fields/flows that expect it.
						pcrPTS.add(pcr27 / 300)
						span := pcrSpans[pid]
						if span == nil {
							span = &pcrSpan{}
							pcrSpans[pid] = span
						}
						if !span.ok {
							span.startPCR = pcr27
							span.startOffset = packetOffset
							span.ok = true
						}
						span.endPCR = pcr27
						span.endOffset = packetOffset
						if span.ok && !span.over30s && span.endPCR >= span.startPCR && span.endPCR-span.startPCR > 30*27000000 {
							span.over30s = true
						}
						maybeLockAC3HeadByPCR(packetOffset)
					}
				}
				if adaptation == 2 {
					continue
				}
				if payloadIndex >= len(ts) {
					continue
				}
				payload := ts[payloadIndex:]

				if pid == 0x1F && payloadStart {
					if sectionBytes := psiSectionBytes(payload); sectionBytes > 0 {
						psiBytes += int64(sectionBytes)
					}
					continue
				}

				if pid == 0 && payloadStart {
					programs, sectionBytes := parsePAT(payload)
					if sectionBytes > 0 {
						psiBytes += int64(sectionBytes)
					}
					for _, prog := range programs {
						if _, ok := pmtPIDToProgram[prog.PMTPID]; !ok {
							pmtPIDToProgram[prog.PMTPID] = prog.ProgramNumber
						}
						if primaryProgramNumber == 0 {
							primaryProgramNumber = prog.ProgramNumber
							primaryPMTPID = prog.PMTPID
						}
					}
					continue
				}
				if pid == 0x11 && payloadStart {
					name, provider, svcType := parseSDT(payload, primaryProgramNumber)
					if name != "" {
						serviceName = name
					}
					if provider != "" {
						serviceProvider = provider
					}
					if svcType != "" {
						serviceType = svcType
					}
					continue
				}
				if programNumber, ok := pmtPIDToProgram[pid]; ok {
					asm := pmtAssemblies[pid]
					if asm == nil {
						asm = &psiAssembly{}
						pmtAssemblies[pid] = asm
					}
					if payloadStart {
						pointer := int(payload[0])
						if 1+pointer > len(payload) {
							continue
						}
						asm.buf = append(asm.buf[:0], payload[1+pointer:]...)
						asm.expected = asm.expectedLen()
					} else if len(asm.buf) > 0 {
						asm.buf = append(asm.buf, payload...)
						if asm.expected == 0 {
							asm.expected = asm.expectedLen()
						}
					}
					if asm.expected > 0 && len(asm.buf) >= asm.expected {
						section := asm.buf[:asm.expected]
						pmPayload := append([]byte{0}, section...)
						parsed, pcr, pointer, sectionLen := parsePMT(pmPayload, programNumber)
						if pcr != 0 {
							pcrPIDs[pcr] = struct{}{}
						}
						// MediaInfo's General.StreamSize for BDAV behaves closer to counting
						// non-A/V (subtitle) PMT entries than full PMT section payload.
						textCount := 0
						for _, st := range parsed {
							if st.kind == StreamText {
								textCount++
							}
						}
						psiBytes += int64(textCount * 5)
						if pid == primaryPMTPID {
							if sectionLen > 0 {
								pmtPointer = pointer
								pmtSectionLen = sectionLen
							}
						}
						for _, st := range parsed {
							if existing, exists := streams[st.pid]; exists {
								existing.kind = st.kind
								existing.format = st.format
								existing.streamType = st.streamType
								existing.programNumber = st.programNumber
								existing.language = st.language
								if pending, ok := pendingPTS[st.pid]; ok {
									existing.pts = *pending
									existing.seenPTS = true
									if st.kind == StreamVideo {
										videoPTS.add(pending.min)
										videoPTS.add(pending.max)
									}
									delete(pendingPTS, st.pid)
								}
							} else {
								entry := st
								if pending, ok := pendingPTS[st.pid]; ok {
									entry.pts = *pending
									entry.seenPTS = true
									if st.kind == StreamVideo {
										videoPTS.add(pending.min)
										videoPTS.add(pending.max)
									}
									delete(pendingPTS, st.pid)
								}
								streams[st.pid] = &entry
								streamOrder = append(streamOrder, st.pid)
							}
							if st.kind == StreamVideo {
								videoPIDs[st.pid] = struct{}{}
								if st.format == "MPEG Video" {
									hasMPEGVideo = true
								}
							}
						}
						asm.reset()
					}
					continue
				}

				pesStart := payloadStart && len(payload) >= 9 && payload[0] == 0x00 && payload[1] == 0x00 && payload[2] == 0x01
				if pesStart {
					flags := payload[7]
					headerLen := int(payload[8])
					if len(payload) < 9+headerLen {
						continue
					}
					if flags&0x80 != 0 {
						if pts, ok := parsePTS(payload[9:]); ok {
							addPTSMode(&anyPTS, pts, !partialScan)
							if entry, ok := streams[pid]; ok {
								addPTSMode(&entry.pts, pts, !partialScan)
								entry.lastPTS = pts
								entry.hasLastPTS = true
								entry.seenPTS = true
								if _, ok := videoPIDs[pid]; ok {
									addPTSMode(&videoPTS, pts, !partialScan)
								}
							} else {
								pending := pendingPTS[pid]
								if pending == nil {
									pending = &ptsTracker{}
									pendingPTS[pid] = pending
								}
								addPTSMode(pending, pts, !partialScan)
							}
						} else if entry, ok := streams[pid]; ok {
							entry.hasLastPTS = false
						}
					} else if entry, ok := streams[pid]; ok {
						entry.hasLastPTS = false
					}
				}

				// BDAV/M2TS streams sometimes omit audio/subtitle PIDs from PMT.
				// Infer common Blu-ray PID layouts from PES payload when no PMT mapping exists.
				if packetSize == 192 && pesStart {
					if _, ok := streams[pid]; !ok {
						headerLen := int(payload[8])
						dataStart := min(9+headerLen, len(payload))
						data := payload[dataStart:]
						if kind, format, stype, ok := inferBDAVStream(pid, data); ok {
							entry := tsStream{
								pid:           pid,
								programNumber: primaryProgramNumber,
								streamType:    stype,
								kind:          kind,
								format:        format,
							}
							if pending, ok := pendingPTS[pid]; ok {
								entry.pts = *pending
								if kind == StreamVideo {
									videoPTS.add(pending.min)
									videoPTS.add(pending.max)
								}
								delete(pendingPTS, pid)
							}
							streams[pid] = &entry
							streamOrder = append(streamOrder, pid)
							if kind == StreamVideo {
								videoPIDs[pid] = struct{}{}
							}
						}
					}
				}

				// Some real-world TS captures start with PES payload before PAT/PMT is available.
				// For parity with MediaInfo, infer MPEG-2 video streams from early start codes so we
				// can parse matrices/GOP/intra_dc_precision even when PMT arrives later.
				if packetSize == 188 && pesStart {
					if _, ok := streams[pid]; !ok {
						headerLen := int(payload[8])
						dataStart := min(9+headerLen, len(payload))
						data := payload[dataStart:]
						peek := data
						if len(peek) > 512 {
							peek = peek[:512]
						}
						if bytes.Contains(peek, []byte{0x00, 0x00, 0x01, 0xB3}) {
							entry := tsStream{
								pid:           pid,
								programNumber: primaryProgramNumber,
								kind:          StreamVideo,
								format:        "MPEG Video",
							}
							if pending, ok := pendingPTS[pid]; ok {
								entry.pts = *pending
								videoPTS.add(pending.min)
								videoPTS.add(pending.max)
								delete(pendingPTS, pid)
							}
							streams[pid] = &entry
							streamOrder = append(streamOrder, pid)
							videoPIDs[pid] = struct{}{}
							hasMPEGVideo = true
						}
					}
				}

				entry, ok := streams[pid]
				if !ok {
					continue
				}

				if pesStart {
					if entry.kind == StreamVideo && !entry.videoStarted {
						// Some TS files begin mid-PES. Don't count video bytes until we've seen a PES start
						// for that PID, or we inflate StreamSize/overhead vs official MediaInfo.
						entry.videoStarted = true
					}
					if len(entry.pesData) > 0 {
						processPES(entry)
					}
					headerLen := int(payload[8])
					dataStart := min(9+headerLen, len(payload))
					data := payload[dataStart:]
					if entry.kind == StreamVideo && entry.format == "MPEG Video" {
						entry.pesPayloadKnown = false
						entry.pesPayloadRemaining = 0
						if len(payload) >= 6 {
							pesLen := int(binary.BigEndian.Uint16(payload[4:6]))
							if pesLen > 0 {
								pesPayloadLen := pesLen - (3 + headerLen)
								if pesPayloadLen < 0 {
									pesPayloadLen = 0
								}
								usable := min(len(data), pesPayloadLen)
								entry.pesPayloadRemaining = pesPayloadLen - usable
								entry.pesPayloadKnown = true
								data = data[:usable]
							}
						}
					} else {
						entry.pesPayloadKnown = false
						entry.pesPayloadRemaining = 0
					}
					entry.bytes += uint64(len(data))
					// BDAV: count PES headers as part of video payload for stream size parity.
					// MediaInfoLib's BDAV StreamSize aligns closer to TS payload bytes than ES-only bytes.
					if isBDAV && entry.kind == StreamVideo {
						pidPayloadBytes[pid] += int64(len(payload))
					} else {
						pidPayloadBytes[pid] += int64(len(data))
					}
					if entry.kind == StreamVideo && entry.format == "AVC" && len(data) > 0 {
						entry.pesData = append(entry.pesData[:0], data...)
					}

					if entry.kind == StreamVideo && entry.format == "MPEG Video" && len(data) > 0 {
						consumeMPEG2TSVideo(entry, data, entry.lastPTS, entry.hasLastPTS)
					}
					if entry.kind == StreamVideo && entry.format == "AVC" && !entry.hasVideoFields && len(data) > 0 {
						if fields, sps, ok := parseH264FromPES(data); ok && len(fields) > 0 {
							width, height, fps := sps.Width, sps.Height, sps.FrameRate
							entry.videoFields = fields
							entry.hasVideoFields = true
							entry.width = width
							entry.height = height
							if sps.CodedHeight > 0 {
								entry.storedHeight = sps.CodedHeight
							}
							if fps > 0 {
								entry.videoFrameRate = fps
							}
						}
					}
					if entry.kind == StreamAudio && len(data) > 0 {
						headEnd := syncOff + ac3StatsHeadBytes
						inHead := !ac3StatsBounded || packetOffset < headEnd
						// Parity: MediaInfoLib fills stream stats at the end of the initial (head) scan
						// when ParseSpeed<0.8, then seeks to the end for duration/PTS. Keep AC-3 stats
						// collection bounded to the head window in that mode.
						collectAC3Stats := inHead && (!ac3StatsBounded || entry.seenPTS)
						// Audio frames can be recovered mid-PES by resyncing on codec sync words.
						// Don't drop buffered bytes at PES boundaries.
						if !entry.audioStarted {
							entry.audioStarted = true
						}
						consumeAudio(entry, data, collectAC3Stats, inHead)
					}

					if _, ok := videoPIDs[pid]; ok {
						entry.frames++
					}
					continue
				}

				if entry.kind == StreamVideo && !entry.videoStarted {
					// File may start mid-PES. Don't count bytes until we see a PES start, but still
					// let the MPEG-2 parser inspect early bytes for matrices/GOP/intra_dc_precision.
					if entry.format == "MPEG Video" && len(payload) > 0 {
						consumeMPEG2TSVideo(entry, payload, entry.lastPTS, entry.hasLastPTS)
					}
					continue
				}

				if entry.kind == StreamVideo && entry.format == "AVC" && len(entry.pesData) > 0 {
					entry.pesData = append(entry.pesData, payload...)
				}
				payloadData := payload
				if entry.kind == StreamVideo && entry.format == "MPEG Video" && entry.pesPayloadKnown {
					if entry.pesPayloadRemaining <= 0 {
						continue
					}
					n := min(len(payload), entry.pesPayloadRemaining)
					payloadData = payload[:n]
					entry.pesPayloadRemaining -= n
				}
				if entry.kind == StreamVideo && entry.format == "MPEG Video" && len(payloadData) > 0 {
					consumeMPEG2TSVideo(entry, payloadData, entry.lastPTS, entry.hasLastPTS)
				}
				if entry.kind == StreamAudio && len(payload) > 0 {
					// Like MediaInfoLib, avoid parsing audio mid-PES before we've seen a PES start
					// for that PID, otherwise we may resync on false-positive AC-3 sync words and
					// skew stats vs official output.
					if !entry.audioStarted {
						continue
					}
					headEnd := syncOff + ac3StatsHeadBytes
					inHead := !ac3StatsBounded || packetOffset < headEnd
					collectAC3Stats := inHead && (!ac3StatsBounded || entry.seenPTS)
					entry.bytes += uint64(len(payload))
					pidPayloadBytes[pid] += int64(len(payload))
					consumeAudio(entry, payload, collectAC3Stats, inHead)
				}
				if entry.kind != StreamAudio && len(payloadData) > 0 {
					entry.bytes += uint64(len(payloadData))
					pidPayloadBytes[pid] += int64(len(payloadData))
				}
			}
			carry = n - packetCount*int(packetSize)
			if carry > 0 {
				copy(buf, buf[packetCount*int(packetSize):n])
			}
			if err != nil {
				break
			}
		}
		return true
	}

	// MediaInfoLib behavior at ParseSpeed<0.8:
	// - scan from the beginning up to MpegTs_MaximumOffset (64 MiB), but it may shrink this based on PCR distance (~30s)
	// - jump to near the end and scan the same window size there
	// - do not parse the middle of the file
	headStart := syncOff
	headEnd := size
	if partialScan && size > 0 {
		headEnd = min(size, syncOff+tsStatsMaxOffset)
	}
	if !scanRange(headStart, headEnd, ac3StatsBounded) {
		return ContainerInfo{}, nil, nil, false
	}
	headEndActual := headEnd
	if headScannedEnd > 0 {
		headEndActual = headScannedEnd
	}
	headBytes := headEndActual - syncOff
	if headBytes < 0 {
		headBytes = 0
	}
	jumpBytes := min(int64(tsStatsMaxOffset), headBytes)
	if ac3StatsHeadLocked && ac3StatsHeadBytes > 0 {
		jumpBytes = ac3StatsHeadBytes
	}
	shouldJump := ac3StatsBounded && size > 0 && jumpBytes > 0 && syncOff+jumpBytes < size-jumpBytes
	if shouldJump || headStoppedEarly {
		partialScan = true
		// Start slightly before the official tail window so PES/audio resync has context.
		// Stats collection remains bounded by packetOffset>=size-jumpBytes.
		tailStart := size - jumpBytes - (1 << 20)
		if tailStart < syncOff {
			tailStart = syncOff
		}
		// Align tail start to the packet boundary relative to sync offset.
		if packetSize > 0 && tailStart > syncOff {
			tailStart = syncOff + ((tailStart-syncOff)/packetSize)*packetSize
		}
		// If there is no gap, just continue sequential parsing.
		if tailStart < headEndActual {
			if headEndActual < size {
				if !scanRange(headEndActual, size, false) {
					return ContainerInfo{}, nil, nil, false
				}
			}
		} else {
			if !scanRange(tailStart, size, false) {
				return ContainerInfo{}, nil, nil, false
			}
		}
	} else if headEndActual < size {
		// Large file with no tail scan configured: continue sequential parsing.
		if !scanRange(headEndActual, size, false) {
			return ContainerInfo{}, nil, nil, false
		}
	}

	for _, entry := range streams {
		if len(entry.pesData) > 0 {
			processPES(entry)
		}
		if entry.kind == StreamVideo && entry.format == "MPEG Video" && entry.mpeg2Parser != nil && entry.mpeg2Parser.sawSequence {
			info := entry.mpeg2Parser.finalizeTS()
			entry.mpeg2Info = info
			entry.hasMPEG2Info = true
			if info.Width > 0 {
				entry.width = info.Width
			}
			if info.Height > 0 {
				entry.height = info.Height
			}
			if info.FrameRate > 0 {
				entry.videoFrameRate = info.FrameRate
			}
		}
	}
	for _, entry := range streams {
		if entry.kind != StreamVideo || entry.format != "AVC" || entry.hasVideoFields {
			continue
		}
		var fields []Field
		var width uint64
		var height uint64
		var storedHeight uint64
		var fps float64
		if packetSize == 192 {
			fields, width, height, storedHeight, fps = scanBDAVForH264(file, entry.pid, size)
		} else {
			fields, width, height, storedHeight, fps = scanTSForH264(file, entry.pid, size)
		}
		if len(fields) > 0 {
			entry.videoFields = fields
			entry.hasVideoFields = true
			entry.width = width
			entry.height = height
			if storedHeight > 0 {
				entry.storedHeight = storedHeight
			}
			if fps > 0 {
				entry.videoFrameRate = fps
			}
		}
	}

	var streamsOut []Stream
	videoDuration := ptsDuration(videoPTS)
	for i, pid := range streamOrder {
		st, ok := streams[pid]
		if !ok {
			continue
		}

		jsonExtras := map[string]string{}
		var jsonRaw map[string]string
		jsonExtras["ID"] = strconv.FormatUint(uint64(st.pid), 10)
		jsonExtras["StreamOrder"] = fmt.Sprintf("0-%d", i)
		if isBDAV && !partialScan && st.bytes > 0 && (st.kind == StreamVideo || st.kind == StreamAudio) {
			jsonExtras["StreamSize"] = strconv.FormatUint(st.bytes, 10)
		}
		if !isBDAV && hasMPEGVideo && !partialScan && st.bytes > 0 && (st.kind == StreamVideo || st.kind == StreamAudio) {
			jsonExtras["StreamSize"] = strconv.FormatUint(st.bytes, 10)
		}
		if st.programNumber > 0 {
			jsonExtras["MenuID"] = strconv.FormatUint(uint64(st.programNumber), 10)
		}
		if st.pts.has() {
			delay := float64(st.pts.first) / 90000.0
			jsonExtras["Delay"] = fmt.Sprintf("%.9f", delay)
			jsonExtras["Delay_Source"] = "Container"
		}
		if st.kind == StreamAudio && videoPTS.has() && st.pts.has() {
			// Match MediaInfo: Video_Delay is computed from millisecond-rounded stream delays.
			audioDelay := float64(st.pts.first) / 90000.0
			videoDelay := float64(videoPTS.first) / 90000.0
			audioDelay = math.Round(audioDelay*1000) / 1000
			videoDelay = math.Round(videoDelay*1000) / 1000
			jsonExtras["Video_Delay"] = fmt.Sprintf("%.3f", audioDelay-videoDelay)
		}
		if st.kind == StreamText && videoPTS.has() && st.pts.has() {
			textDelay := float64(st.pts.first) / 90000.0
			videoDelay := float64(videoPTS.first) / 90000.0
			textDelay = math.Round(textDelay*1000) / 1000
			videoDelay = math.Round(videoDelay*1000) / 1000
			jsonExtras["Video_Delay"] = fmt.Sprintf("%.3f", textDelay-videoDelay)
		}
		if st.kind == StreamVideo && st.height > 0 {
			storedHeight := st.storedHeight
			if storedHeight == 0 {
				storedHeight = st.height
			}
			// Match MediaInfo behavior: for AVC, emit a macroblock-aligned Stored_Height when it differs.
			if st.format == "AVC" && storedHeight == st.height && storedHeight%16 != 0 {
				storedHeight = ((storedHeight + 15) / 16) * 16
			}
			if storedHeight > 0 && storedHeight != st.height {
				jsonExtras["Stored_Height"] = strconv.FormatUint(storedHeight, 10)
			}
		}
		if isBDAV && st.kind == StreamVideo {
			// Match MediaInfoLib: BDAV video streams expose extra.format_identifier=HDMV.
			if jsonRaw == nil {
				jsonRaw = map[string]string{}
			}
			if _, ok := jsonRaw["extra"]; !ok {
				jsonRaw["extra"] = "{\"format_identifier\":\"HDMV\"}"
			}
			// MediaInfo reports these Blu-ray-oriented constraints for AVC streams.
			if st.format == "AVC" {
				jsonExtras["BitRate_Maximum"] = "39959808"
				jsonExtras["BufferSize"] = "30000000 / 30000000"
			}
		}
		fields := []Field{{Name: "ID", Value: formatStreamID(st.pid)}}
		if st.programNumber > 0 {
			fields = append(fields, Field{Name: "Menu ID", Value: formatID(uint64(st.programNumber))})
		}
		format := st.format
		if st.kind == StreamAudio && st.audioProfile != "" {
			format = "AAC " + st.audioProfile
		}
		if format != "" {
			fields = append(fields, Field{Name: "Format", Value: format})
		}
		if st.kind == StreamVideo && !isBDAV && hasMPEGVideo && st.format == "MPEG Video" && st.hasMPEG2Info {
			info := st.mpeg2Info
			if name := mpeg2CommercialNameTS(info); name != "" {
				fields = append(fields, Field{Name: "Commercial name", Value: name})
			}
			if info.Version != "" {
				fields = append(fields, Field{Name: "Format version", Value: info.Version})
			}
			if info.Profile != "" {
				fields = append(fields, Field{Name: "Format profile", Value: info.Profile})
			}
			if info.BVOP != nil {
				fields = append(fields, Field{Name: "Format settings", Value: "BVOP"})
				fields = append(fields, Field{Name: "Format settings, BVOP", Value: formatYesNo(*info.BVOP)})
			}
			if info.Matrix != "" {
				fields = append(fields, Field{Name: "Format settings, Matrix", Value: info.Matrix})
			}
			if info.Matrix == "Custom" && info.MatrixData != "" {
				jsonExtras["Format_Settings_Matrix_Data"] = info.MatrixData
			}
			if info.ScanType == "Interlaced" && info.GOPM > 0 && info.GOPN > 0 {
				fields = append(fields, Field{Name: "Format settings, GOP", Value: fmt.Sprintf("M=%d, N=%d", info.GOPM, info.GOPN)})
			} else if info.GOPVariable {
				fields = append(fields, Field{Name: "Format settings, GOP", Value: "Variable"})
			} else if info.GOPLength > 0 {
				fields = append(fields, Field{Name: "Format settings, GOP", Value: formatGOPLength(info.GOPLength)})
			}
			if info.ScanType == "Interlaced" && info.PictureStructure != "" {
				fields = append(fields, Field{Name: "Format settings, Picture structure", Value: info.PictureStructure})
			}
		}
		if st.kind == StreamVideo {
			if info := mapMatroskaFormatInfo(st.format); info != "" {
				fields = append(fields, Field{Name: "Format/Info", Value: info})
			}
			if st.hasVideoFields {
				fields = append(fields, st.videoFields...)
			}
			if st.streamType != 0 {
				fields = append(fields, Field{Name: "Codec ID", Value: formatTSCodecID(st.streamType)})
			}
		}
		if st.kind == StreamAudio {
			if st.audioProfile == "LC" {
				fields = append(fields, Field{Name: "Format/Info", Value: "Advanced Audio Codec Low Complexity"})
				fields = append(fields, Field{Name: "Format version", Value: formatAACVersion(st.audioMPEGVersion)})
				fields = append(fields, Field{Name: "Muxing mode", Value: "ADTS"})
			} else if info := mapMatroskaFormatInfo(st.format); info != "" {
				fields = append(fields, Field{Name: "Format/Info", Value: info})
			}
			if st.streamType != 0 {
				parts := []string{formatTSCodecID(st.streamType)}
				if st.audioObject > 0 {
					parts = append(parts, strconv.Itoa(st.audioObject))
				}
				fields = append(fields, Field{Name: "Codec ID", Value: strings.Join(parts, "-")})
			}
		}
		if st.kind == StreamVideo {
			duration := ptsDuration(st.pts)
			if duration == 0 {
				duration = videoDuration
			}
			// For TS MPEG-2 video, official mediainfo derives Duration from FrameCount and the
			// stream's FrameRate ratio (not from PTS deltas).
			if !partialScan && !isBDAV && hasMPEGVideo && st.format == "MPEG Video" && st.hasMPEG2Info && st.videoFrameCount > 0 {
				info := st.mpeg2Info
				if info.FrameRateNumer > 0 && info.FrameRateDenom > 0 {
					duration = float64(st.videoFrameCount) * float64(info.FrameRateDenom) / float64(info.FrameRateNumer)
					duration = math.Round(duration*1000) / 1000
				} else if info.FrameRate > 0 {
					duration = float64(st.videoFrameCount) / info.FrameRate
					duration = math.Round(duration*1000) / 1000
				}
			}
			if partialScan && !isBDAV && hasMPEGVideo && st.format == "MPEG Video" && st.hasMPEG2Info && duration > 0 {
				info := st.mpeg2Info
				fps := 0.0
				if info.FrameRateNumer > 0 && info.FrameRateDenom > 0 {
					fps = float64(info.FrameRateNumer) / float64(info.FrameRateDenom)
				} else if info.FrameRate > 0 {
					fps = info.FrameRate
				}
				if fps > 0 {
					// PTS deltas cover (N-1) frame intervals; official mediainfo reports full duration (N intervals).
					duration += 1.0 / fps
					duration = math.Round(duration*1000) / 1000
				}
			}
			if duration > 0 && st.videoFrameRate > 0 && st.format != "MPEG Video" {
				duration += 1.0 / st.videoFrameRate
			}
			if duration > 0 {
				fields = addStreamDuration(fields, duration)
				if isBDAV {
					jsonExtras["Duration"] = fmt.Sprintf("%.3f", duration)
					if st.videoFrameRate > 0 {
						jsonExtras["FrameCount"] = strconv.Itoa(int(math.Round(duration * st.videoFrameRate)))
					}
				} else if hasMPEGVideo && st.format == "MPEG Video" {
					jsonExtras["Duration"] = formatJSONSeconds(duration)
				}
			}
			if isBDAV && st.videoFrameRate > 0 {
				fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRateWithRatio(st.videoFrameRate)})
			}
			if !isBDAV && hasMPEGVideo && st.format == "MPEG Video" && st.hasMPEG2Info && duration > 0 {
				info := st.mpeg2Info
				if info.IntraDCPrecision > 0 {
					intra := info.IntraDCPrecision
					// Heuristic to match MediaInfoLib ParseSpeed sampling: for short TS (<= ~30s),
					// intra_dc_precision tends to reflect the initial scan window.
					if duration <= 30.0 && info.IntraDCPrecisionFirst > 0 {
						intra = info.IntraDCPrecisionFirst
					}
					if jsonRaw == nil {
						jsonRaw = map[string]string{}
					}
					jsonRaw["extra"] = renderJSONObject([]jsonKV{{Key: "intra_dc_precision", Val: strconv.Itoa(intra)}}, false)
				}
				fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
				bitrate := (float64(st.bytes) * 8) / duration
				fields = addStreamBitrate(fields, bitrate)
				if maxKbps := info.MaxBitRateKbps; maxKbps > 0 {
					fields = append(fields, Field{Name: "Maximum bit rate", Value: formatBitrate(float64(maxKbps) * 1000)})
					jsonExtras["BitRate_Maximum"] = strconv.FormatInt(maxKbps*1000, 10)
				}
				if info.BufferSize > 0 {
					jsonExtras["BufferSize"] = strconv.FormatInt(info.BufferSize, 10)
				}
				jsonDuration := math.Round(duration*1000) / 1000
				if jsonDuration > 0 {
					jsonExtras["BitRate"] = strconv.FormatInt(int64(math.Round((float64(st.bytes)*8)/jsonDuration)), 10)
				}
				frameCount := st.videoFrameCount
				if partialScan {
					fps := 0.0
					if info.FrameRateNumer > 0 && info.FrameRateDenom > 0 {
						fps = float64(info.FrameRateNumer) / float64(info.FrameRateDenom)
					} else if info.FrameRate > 0 {
						fps = info.FrameRate
					}
					if fps > 0 {
						frameCount = int(math.Round(duration * fps))
					}
				}
				if frameCount > 0 {
					jsonExtras["FrameCount"] = strconv.Itoa(frameCount)
				}
				if info.GOPDropFrame != nil {
					jsonExtras["Delay_Original"] = "0.000"
					jsonExtras["Delay_Original_Source"] = "Stream"
					jsonExtras["Delay_DropFrame"] = formatYesNo(*info.GOPDropFrame)
					jsonExtras["Delay_Original_DropFrame"] = formatYesNo(*info.GOPDropFrame)
				}
			}
			if st.format != "MPEG Video" && !isBDAV {
				fields = append(fields, Field{Name: "Frame rate mode", Value: "Variable"})
			}
			if isBDAV {
				fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
			}
			if isBDAV && duration > 0 && st.bytes > 0 {
				bitrate := (float64(st.bytes) * 8) / duration
				if bitrate > 0 {
					fields = append(fields, Field{Name: "Bit rate", Value: formatBitrate(bitrate)})
				}
			}
			if st.width > 0 {
				fields = append(fields, Field{Name: "Width", Value: formatPixels(st.width)})
			}
			if st.height > 0 {
				fields = append(fields, Field{Name: "Height", Value: formatPixels(st.height)})
			}
			if ar := formatAspectRatio(st.width, st.height); ar != "" {
				fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
			}
			if !isBDAV && hasMPEGVideo && st.format == "MPEG Video" && st.hasMPEG2Info {
				info := st.mpeg2Info
				if info.FrameRateNumer > 0 && info.FrameRateDenom > 0 {
					fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRateRatio(info.FrameRateNumer, info.FrameRateDenom)})
				} else if info.FrameRate > 0 {
					fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRate(info.FrameRate)})
				}
				if info.ColorSpace != "" {
					fields = append(fields, Field{Name: "Color space", Value: info.ColorSpace})
				}
				if info.ChromaSubsampling != "" {
					fields = append(fields, Field{Name: "Chroma subsampling", Value: info.ChromaSubsampling})
				}
				if info.BitDepth != "" {
					fields = append(fields, Field{Name: "Bit depth", Value: info.BitDepth})
				}
				if info.ScanType != "" {
					fields = append(fields, Field{Name: "Scan type", Value: info.ScanType})
				}
				if info.ScanOrder != "" {
					fields = append(fields, Field{Name: "Scan order", Value: info.ScanOrder})
				}
				fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
				if duration > 0 && st.width > 0 && st.height > 0 && st.videoFrameRate > 0 {
					bitrate := (float64(st.bytes) * 8) / duration
					if bits := formatBitsPerPixelFrame(bitrate, st.width, st.height, st.videoFrameRate); bits != "" {
						fields = append(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
					}
				}
				if info.TimeCode != "" {
					fields = append(fields, Field{Name: "Time code of first frame", Value: info.TimeCode})
				}
				if info.TimeCodeSource != "" {
					fields = append(fields, Field{Name: "Time code source", Value: info.TimeCodeSource})
				}
				if st.bytes > 0 && size > 0 {
					if value := formatStreamSize(int64(st.bytes), size); value != "" {
						fields = append(fields, Field{Name: "Stream size", Value: value})
					}
				}
			}
		}
		if st.kind == StreamAudio {
			duration := ptsDuration(st.pts)
			// PTS deltas cover (N-1) frame intervals; official mediainfo reports full duration (N intervals).
			if !isBDAV && hasMPEGVideo && st.hasAC3 && st.ac3Info.sampleRate > 0 && st.ac3Info.spf > 0 && duration > 0 {
				duration += float64(st.ac3Info.spf) / float64(st.ac3Info.sampleRate)
			}
			if !partialScan && st.hasAC3 && st.ac3Info.sampleRate > 0 && st.ac3Info.spf > 0 && st.audioFrames > 0 {
				rate := int64(st.ac3Info.sampleRate)
				if rate > 0 {
					samples := st.audioFrames * uint64(st.ac3Info.spf)
					durationMs := int64((samples * 1000) / uint64(rate))
					duration = float64(durationMs) / 1000.0
				}
			}
			if !partialScan && st.audioRate > 0 && st.audioFrames > 0 && st.audioSpf > 0 {
				rate := int64(st.audioRate)
				if rate > 0 {
					samples := st.audioFrames * uint64(st.audioSpf)
					durationMs := int64((samples * 1000) / uint64(rate))
					duration = float64(durationMs) / 1000.0
				}
			}
			if duration > 0 {
				fields = addStreamDuration(fields, duration)
				if isBDAV {
					jsonExtras["Duration"] = fmt.Sprintf("%.3f", duration)
				} else {
					jsonExtras["Duration"] = formatJSONSeconds(duration)
				}
			}
			if isBDAV && duration > 0 && st.audioBitRateKbps > 0 {
				bps := int64(st.audioBitRateKbps) * 1000
				// MediaInfo uses integer milliseconds for sizing.
				durationMs := int64(math.Round(duration * 1000))
				if durationMs > 0 && bps > 0 {
					ss := int64(math.Round(float64(bps) * float64(durationMs) / 8000.0))
					if ss > 0 {
						jsonExtras["StreamSize"] = strconv.FormatInt(ss, 10)
					}
				}
				if st.audioRate > 0 && st.audioSpf > 0 {
					frameRate := st.audioRate / float64(st.audioSpf)
					if frameRate > 0 && !strings.HasPrefix(st.format, "AAC") {
						jsonExtras["FrameCount"] = strconv.Itoa(int(math.Round(duration * frameRate)))
					}
				}
			}
			if !isBDAV && hasMPEGVideo && duration > 0 && st.audioBitRateKbps > 0 {
				bps := int64(st.audioBitRateKbps) * 1000
				// MediaInfo uses integer milliseconds for sizing.
				durationMs := int64(math.Round(duration * 1000))
				if durationMs > 0 && bps > 0 {
					ss := int64(math.Round(float64(bps) * float64(durationMs) / 8000.0))
					if ss > 0 {
						jsonExtras["StreamSize"] = strconv.FormatInt(ss, 10)
					}
				}
				if st.audioRate > 0 && st.audioSpf > 0 {
					frameRate := st.audioRate / float64(st.audioSpf)
					if frameRate > 0 && !strings.HasPrefix(st.format, "AAC") {
						jsonExtras["FrameCount"] = strconv.Itoa(int(math.Round(duration * frameRate)))
					}
				} else if st.hasAC3 && st.ac3Info.sampleRate > 0 && st.ac3Info.spf > 0 {
					frameRate := float64(st.ac3Info.sampleRate) / float64(st.ac3Info.spf)
					if frameRate > 0 {
						jsonExtras["FrameCount"] = strconv.Itoa(int(math.Round(duration * frameRate)))
					}
				}
			}

			if st.audioRate > 0 && st.audioChannels > 0 {
				mode := st.audioBitRateMode
				if mode == "" {
					mode = "Variable"
				}
				fields = append(fields, Field{Name: "Bit rate mode", Value: mode})
				if st.audioBitRateKbps > 0 {
					fields = append(fields, Field{Name: "Bit rate", Value: formatBitrate(float64(st.audioBitRateKbps) * 1000)})
				}
				fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(st.audioChannels)})
				layout := channelLayout(st.audioChannels)
				if st.format == "AC-3" && st.audioChannels == 6 {
					layout = "L R C LFE Ls Rs"
				}
				if layout != "" {
					fields = append(fields, Field{Name: "Channel layout", Value: layout})
				}
				fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(st.audioRate)})
				if st.audioSpf > 0 {
					frameRate := st.audioRate / float64(st.audioSpf)
					fields = append(fields, Field{Name: "Frame rate", Value: fmt.Sprintf("%.3f FPS (%d SPF)", frameRate, st.audioSpf)})
				}
				fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
				if videoPTS.has() && st.pts.has() {
					delay := float64(int64(st.pts.min)-int64(videoPTS.min)) * 1000 / 90000.0
					fields = append(fields, Field{Name: "Delay relative to video", Value: fmt.Sprintf("%d ms", int64(math.Round(delay)))})
				}
			}

			if st.hasAC3 {
				// Match MediaInfo's extra AC-3 metadata fields for TS/BDAV.
				jsonExtras["Format_Settings_Endianness"] = "Big"
				if st.format == "AC-3" {
					jsonExtras["Format_Commercial_IfAny"] = "Dolby Digital"
				} else if st.format == "E-AC-3" {
					jsonExtras["Format_Commercial_IfAny"] = "Dolby Digital Plus"
				}
				if st.ac3Info.spf > 0 {
					jsonExtras["SamplesPerFrame"] = strconv.Itoa(st.ac3Info.spf)
				}
				if st.ac3Info.sampleRate > 0 && duration > 0 {
					samplingCount := int64(math.Round(duration * st.ac3Info.sampleRate))
					if samplingCount > 0 {
						jsonExtras["SamplingCount"] = strconv.FormatInt(samplingCount, 10)
					}
				}
				if code := ac3ServiceKindCode(st.ac3Info.bsmod); code != "" {
					jsonExtras["ServiceKind"] = code
				}
				if st.ac3Info.serviceKind != "" {
					fields = append(fields, Field{Name: "Service kind", Value: st.ac3Info.serviceKind})
				}
				if !partialScan && !isBDAV && hasMPEGVideo && st.audioFrames > 0 && st.hasAC3 && jsonExtras["FrameCount"] == "" {
					jsonExtras["FrameCount"] = strconv.FormatUint(st.audioFrames, 10)
				}

				extraFields := []jsonKV{
					{Key: "format_identifier", Val: st.format},
				}
				if st.ac3Info.bsid > 0 {
					extraFields = append(extraFields, jsonKV{Key: "bsid", Val: strconv.Itoa(st.ac3Info.bsid)})
				}
				if st.ac3Info.hasDialnorm {
					extraFields = append(extraFields, jsonKV{Key: "dialnorm", Val: strconv.Itoa(st.ac3Info.dialnorm)})
				}
				if st.ac3Info.hasCompr {
					extraFields = append(extraFields, jsonKV{Key: "compr", Val: fmt.Sprintf("%.2f", st.ac3Info.comprDB)})
				}
				if st.ac3Info.hasDynrng {
					extraFields = append(extraFields, jsonKV{Key: "dynrng", Val: fmt.Sprintf("%.2f", st.ac3Info.dynrngDB)})
				}
				if st.ac3Info.acmod > 0 {
					extraFields = append(extraFields, jsonKV{Key: "acmod", Val: strconv.Itoa(st.ac3Info.acmod)})
				}
				if st.ac3Info.hasDsurmod {
					extraFields = append(extraFields, jsonKV{Key: "dsurmod", Val: strconv.Itoa(st.ac3Info.dsurmod)})
				}
				if st.ac3Info.lfeon >= 0 {
					extraFields = append(extraFields, jsonKV{Key: "lfeon", Val: strconv.Itoa(st.ac3Info.lfeon)})
				}
				if avg, minVal, maxVal, ok := st.ac3Stats.dialnormStats(); ok {
					extraFields = append(extraFields, jsonKV{Key: "dialnorm_Average", Val: strconv.Itoa(avg)})
					extraFields = append(extraFields, jsonKV{Key: "dialnorm_Minimum", Val: strconv.Itoa(minVal)})
					_ = maxVal
				}
				if avg, minVal, maxVal, count, ok := st.ac3Stats.comprStats(); ok {
					extraFields = append(extraFields, jsonKV{Key: "compr_Average", Val: fmt.Sprintf("%.2f", avg)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Minimum", Val: fmt.Sprintf("%.2f", minVal)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Maximum", Val: fmt.Sprintf("%.2f", maxVal)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Count", Val: strconv.Itoa(count)})
				}
				if avg, minVal, maxVal, count, ok := st.ac3Stats.dynrngStats(); ok {
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Average", Val: fmt.Sprintf("%.2f", avg)})
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Minimum", Val: fmt.Sprintf("%.2f", minVal)})
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Maximum", Val: fmt.Sprintf("%.2f", maxVal)})
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Count", Val: strconv.Itoa(count)})
				}
				if len(extraFields) > 0 {
					if jsonRaw == nil {
						jsonRaw = map[string]string{}
					}
					jsonRaw["extra"] = renderJSONObject(extraFields, false)
				}
			}
			if strings.HasPrefix(st.format, "AAC") {
				delete(jsonExtras, "FrameCount")
			}
			if !isBDAV && hasMPEGVideo && st.bytes > 0 && size > 0 {
				if value := formatStreamSize(int64(st.bytes), size); value != "" {
					fields = append(fields, Field{Name: "Stream size", Value: value})
				}
			}
			if st.language != "" {
				fields = append(fields, Field{Name: "Language", Value: formatLanguage(st.language)})
				// Official mediainfo prefers ISO 639-1 where possible (e.g. "eng" -> "en").
				jsonExtras["Language"] = normalizeLanguageCode(st.language)
			}
		}
		if st.kind == StreamText && st.streamType != 0 {
			fields = append(fields, Field{Name: "Codec ID", Value: formatTSCodecID(st.streamType)})
		}
		if st.kind == StreamVideo {
			if st.writingLibrary != "" {
				fields = append(fields, Field{Name: "Writing library", Value: st.writingLibrary})
			}
			if st.encoding != "" {
				fields = append(fields, Field{Name: "Encoding settings", Value: st.encoding})
				if bitrate, ok := findX264Bitrate(st.encoding); ok {
					fields = append(fields, Field{Name: "Nominal bit rate", Value: formatBitrate(bitrate)})
				}
			}
		}
		streamsOut = append(streamsOut, Stream{Kind: st.kind, Fields: fields, JSON: jsonExtras, JSONRaw: jsonRaw})
	}

	if !isBDAV && hasMPEGVideo {
		for _, pid := range streamOrder {
			st, ok := streams[pid]
			if !ok {
				continue
			}
			if st.kind == StreamVideo && st.format == "MPEG Video" {
				appendTSCaptionStreams(&streamsOut, st)
			}
		}
	}

	info := ContainerInfo{}
	var overallBitrate float64
	var durationCount int64
	var durationSum27 int64
	var bytesSum int64
	for _, span := range pcrSpans {
		if span == nil || !span.ok {
			continue
		}
		if span.endPCR <= span.startPCR || span.endOffset <= span.startOffset {
			continue
		}
		durationCount++
		durationSum27 += int64(span.endPCR - span.startPCR)
		bytesSum += span.endOffset - span.startOffset
	}
	if durationCount > 0 && durationSum27 > 0 && bytesSum > 0 && size > 0 {
		overallBitrate = float64(bytesSum) / (float64(durationSum27) / (8 * 27000000.0))
		if overallBitrate > 0 {
			// MediaInfo precision band:
			// OverallBitRate_Precision_{Min,Max} derived from 1-byte precision in the computed
			// byte count per PCR span plus +/-500 us tolerance per PCR span (27MHz ticks).
			//
			// Ref: MediaInfoLib File_MpegTs.cpp (Bytes_Sum +/- Duration_Count, Duration_Sum +/- 13500*Duration_Count).
			bytesSum := float64(bytesSum)
			count := float64(durationCount)
			durSum := float64(durationSum27)
			denMin := (durSum + 13500.0*count) / 27000000.0
			denMax := (durSum - 13500.0*count) / 27000000.0
			if bytesSum-count > 0 && denMin > 0 {
				info.OverallBitrateMin = (bytesSum - count) * 8 / denMin
			}
			if bytesSum+count > 0 && denMax > 0 {
				info.OverallBitrateMax = (bytesSum + count) * 8 / denMax
			}
		}
	}
	if info.DurationSeconds == 0 {
		if duration := pcrFull.durationSeconds(); duration > 0 {
			info.DurationSeconds = duration
		} else if duration := ptsDuration(videoPTS); duration > 0 {
			info.DurationSeconds = duration
		} else if duration := ptsDuration(anyPTS); duration > 0 {
			info.DurationSeconds = duration
		} else if overallBitrate > 0 && size > 0 {
			// Fallback when neither PCR nor PTS is available.
			info.DurationSeconds = float64(size*8) / overallBitrate
		}
	}
	info.BitrateMode = "Variable"
	if size > 0 && packetSize > 0 {
		totalPackets := (size - syncOff) / packetSize
		if totalPackets < 0 {
			totalPackets = 0
		}
		if isBDAV {
			avPayload := int64(0)
			for _, pid := range streamOrder {
				st := streams[pid]
				if st == nil {
					continue
				}
				if st.kind != StreamVideo && st.kind != StreamAudio {
					continue
				}
				avPayload += pidPayloadBytes[pid]
			}
			estAVPayload := avPayload
			if partialScan && tsPacketCount > 0 && totalPackets > 0 {
				estAVPayload = int64(math.Round(float64(avPayload) * float64(totalPackets) / float64(tsPacketCount)))
			}
			if estAVPayload < 0 {
				estAVPayload = 0
			}
			if estAVPayload > size {
				estAVPayload = size
			}
			overhead := size - estAVPayload
			if overhead > 0 {
				info.StreamOverheadBytes = overhead
			}
		} else if tsPacketCount > 0 || totalPackets > 0 {
			// Used as a fallback (e.g. missing StreamSize fields) but not as a stable TS overhead estimator.
			base := totalPackets * (tsOffset + 4)
			if base > 0 {
				info.StreamOverheadBytes = base + psiBytes
			}
		}
	}

	// Parity: MediaInfoLib derives MPEG-TS video bitrate/stream size from OverallBitRate minus
	// audio/text bitrates, using MPEG-TS-specific ratios (default container overhead).
	// This impacts both Video.StreamSize and General.StreamSize (overhead = FileSize - sum(stream sizes)).
	if !isBDAV && hasMPEGVideo {
		overallMid := overallBitrate
		if info.OverallBitrateMin > 0 && info.OverallBitrateMax > 0 {
			overallMid = (info.OverallBitrateMin + info.OverallBitrateMax) / 2
		}
		// MediaInfoLib uses the (integer) General_OverallBitRate value as the base for this derivation.
		overallMid = float64(int64(math.Round(overallMid)))
		if overallMid > 0 && info.DurationSeconds >= 1 {
			videoCount := 0
			var mpegVideo *tsStream
			audioBitratesOK := true
			audioSumAdjusted := 0.0
			for _, pid := range streamOrder {
				st := streams[pid]
				if st == nil {
					continue
				}
				if st.kind == StreamVideo {
					videoCount++
					if st.format == "MPEG Video" {
						mpegVideo = st
					}
				}
				if st.kind == StreamAudio {
					if st.audioBitRateKbps <= 0 {
						audioBitratesOK = false
						continue
					}
					audioBps := float64(st.audioBitRateKbps) * 1000
					audioSumAdjusted += audioBps / 0.96
				}
			}
			if videoCount == 1 && mpegVideo != nil && audioBitratesOK {
				videoBR := (overallMid*0.98 - audioSumAdjusted) * 0.97
				if videoBR >= 10000 {
					durationMs := info.DurationSeconds * 1000
					fps := 0.0
					if mpegVideo.mpeg2Info.FrameRateNumer > 0 && mpegVideo.mpeg2Info.FrameRateDenom > 0 {
						fps = float64(mpegVideo.mpeg2Info.FrameRateNumer) / float64(mpegVideo.mpeg2Info.FrameRateDenom)
					} else if mpegVideo.mpeg2Info.FrameRate > 0 {
						fps = mpegVideo.mpeg2Info.FrameRate
					}
					if partialScan {
						if d := ptsDuration(mpegVideo.pts); d > 0 {
							if fps > 0 {
								d += 1.0 / fps
								d = math.Round(d*1000) / 1000
							}
							durationMs = d * 1000
						}
					} else if mpegVideo.videoFrameCount > 0 && fps > 0 {
						// Match MediaInfoLib: it uses the formatted (rounded) FrameRate value for stable sizing.
						fps = math.Round(fps*1000) / 1000
						durationMs = float64(mpegVideo.videoFrameCount) * 1000 / fps
					}
					videoBitRateInt := int64(math.Round(videoBR))
					videoStreamSize := int64(math.Round((float64(videoBitRateInt) / 8) * (durationMs / 1000)))
					if videoBitRateInt > 0 && videoStreamSize > 0 {
						videoID := strconv.FormatUint(uint64(mpegVideo.pid), 10)
						for i := range streamsOut {
							if streamsOut[i].Kind != StreamVideo || streamsOut[i].JSON == nil {
								continue
							}
							if streamsOut[i].JSON["ID"] != videoID {
								continue
							}
							streamsOut[i].JSON["BitRate"] = strconv.FormatInt(videoBitRateInt, 10)
							streamsOut[i].JSON["StreamSize"] = strconv.FormatInt(videoStreamSize, 10)
							streamsOut[i].Fields = setFieldValue(streamsOut[i].Fields, "Bit rate", formatBitrate(float64(videoBitRateInt)))
							streamsOut[i].Fields = setFieldValue(streamsOut[i].Fields, "Stream size", formatStreamSize(videoStreamSize, size))
							break
						}
					}
				}
			}
		}
	}

	generalFields := []Field{}
	if primaryProgramNumber > 0 {
		generalFields = append(generalFields, Field{Name: "ID", Value: formatID(uint64(primaryProgramNumber))})
	}
	// General metadata from EIA-608 XDS (carried in GA94 user data) matches official mediainfo.
	if !isBDAV {
		for _, pid := range streamOrder {
			st := streams[pid]
			if st == nil || st.kind != StreamVideo {
				continue
			}
			// Keep XDS-derived General metadata only when 608 data is sufficiently synced.
			// This avoids false positives on streams where random GA94 bytes mimic XDS packets.
			if st.hasValidCEA608() && st.xdsLawRating != "" {
				generalFields = append(generalFields, Field{Name: "Law rating", Value: st.xdsLawRating})
			}
			// MediaInfoLib sets Title/Movie to the last Program Name observed.
			if st.hasValidCEA608() {
				if title := st.xdsLastTitle; title != "" {
					generalFields = append(generalFields, Field{Name: "Title", Value: title})
					generalFields = append(generalFields, Field{Name: "Movie", Value: title})
				}
			}
			break
		}
	}

	// MediaInfo only emits a Menu track for TS when DVB service descriptors are present (SDT).
	// (ATSC/PSIP streams often omit SDT and don't get a Menu track in official output.)
	if primaryPMTPID != 0 && packetSize == tsPacketSize && (serviceName != "" || serviceProvider != "" || serviceType != "") {
		menuFields := []Field{
			{Name: "ID", Value: formatID(uint64(primaryPMTPID))},
		}
		if primaryProgramNumber > 0 {
			menuFields = append(menuFields, Field{Name: "Menu ID", Value: formatID(uint64(primaryProgramNumber))})
		}
		var formats []string
		var list []string
		var listKinds []string
		var listPositions []string
		videoIndex := 0
		audioIndex := 0
		for _, pid := range streamOrder {
			st, ok := streams[pid]
			if !ok {
				continue
			}
			if st.kind != StreamVideo && st.kind != StreamAudio {
				continue
			}
			formats = append(formats, st.format)
			list = append(list, fmt.Sprintf("%s (%s)", formatStreamID(st.pid), st.format))
			switch st.kind {
			case StreamVideo:
				listKinds = append(listKinds, "1")
				listPositions = append(listPositions, strconv.Itoa(videoIndex))
				videoIndex++
			case StreamAudio:
				listKinds = append(listKinds, "2")
				listPositions = append(listPositions, strconv.Itoa(audioIndex))
				audioIndex++
			case StreamGeneral, StreamText, StreamImage, StreamMenu:
			}
		}
		if len(formats) > 0 {
			menuFields = append(menuFields, Field{Name: "Format", Value: strings.Join(formats, " / ")})
		}
		if duration := pcrFull.durationSeconds(); duration > 0 {
			menuFields = append(menuFields, Field{Name: "Duration", Value: formatDuration(duration)})
		}
		if len(list) > 0 {
			menuFields = append(menuFields, Field{Name: "List", Value: strings.Join(list, " / ")})
		}
		if serviceName != "" {
			menuFields = append(menuFields, Field{Name: "Service name", Value: serviceName})
		}
		if serviceProvider != "" {
			menuFields = append(menuFields, Field{Name: "Service provider", Value: serviceProvider})
		}
		if serviceType != "" {
			menuFields = append(menuFields, Field{Name: "Service type", Value: serviceType})
		}
		menuJSON := map[string]string{
			"StreamOrder": "0",
			"ID":          strconv.FormatUint(uint64(primaryPMTPID), 10),
		}
		if primaryProgramNumber > 0 {
			menuJSON["MenuID"] = strconv.FormatUint(uint64(primaryProgramNumber), 10)
		}
		if duration := pcrFull.durationSeconds(); duration > 0 {
			menuJSON["Duration"] = fmt.Sprintf("%.9f", duration)
		}
		if pcrFull.has() {
			delay := float64(pcrFull.min) / 27000000.0
			menuJSON["Delay"] = fmt.Sprintf("%.9f", delay)
		}
		if len(listKinds) > 0 {
			menuJSON["List_StreamKind"] = strings.Join(listKinds, " / ")
		}
		if len(listPositions) > 0 {
			menuJSON["List_StreamPos"] = strings.Join(listPositions, " / ")
		}
		menuRaw := map[string]string{}
		if pmtSectionLen > 0 {
			menuRaw["extra"] = fmt.Sprintf("{\"pointer_field\":\"%d\",\"section_length\":\"%d\"}", pmtPointer, pmtSectionLen)
		}
		streamsOut = append(streamsOut, Stream{Kind: StreamMenu, Fields: menuFields, JSON: menuJSON, JSONRaw: menuRaw})
	}

	return info, streamsOut, generalFields, true
}

func psiSectionBytes(payload []byte) int {
	if len(payload) < 8 {
		return 0
	}
	pointer := int(payload[0])
	if 1+pointer+3 > len(payload) {
		return 0
	}
	section := payload[1+pointer:]
	if len(section) < 3 {
		return 0
	}
	sectionLen := int(binary.BigEndian.Uint16(section[1:3]) & 0x0FFF)
	if sectionLen <= 0 {
		return 0
	}
	if 3+sectionLen > len(section) {
		return 0
	}
	return 3 + sectionLen
}

type patProgram struct {
	ProgramNumber uint16
	PMTPID        uint16
}

type psiAssembly struct {
	buf      []byte
	expected int
}

func (a *psiAssembly) expectedLen() int {
	if len(a.buf) < 3 {
		return 0
	}
	sectionLen := int(binary.BigEndian.Uint16(a.buf[1:3]) & 0x0FFF)
	if sectionLen <= 0 {
		return 0
	}
	return 3 + sectionLen
}

func (a *psiAssembly) reset() {
	a.buf = a.buf[:0]
	a.expected = 0
}

func parsePAT(payload []byte) ([]patProgram, int) {
	if len(payload) < 8 {
		return nil, 0
	}
	pointer := int(payload[0])
	if pointer+8 > len(payload) {
		return nil, 0
	}
	section := payload[1+pointer:]
	if len(section) < 8 {
		return nil, 0
	}
	sectionLen := int(binary.BigEndian.Uint16(section[1:3]) & 0x0FFF)
	if sectionLen+3 > len(section) {
		return nil, 0
	}
	entries := section[8 : 3+sectionLen-4]
	out := make([]patProgram, 0, len(entries)/4)
	for i := 0; i+4 <= len(entries); i += 4 {
		programNumber := binary.BigEndian.Uint16(entries[i : i+2])
		pid := binary.BigEndian.Uint16(entries[i+2:i+4]) & 0x1FFF
		if programNumber != 0 {
			out = append(out, patProgram{ProgramNumber: programNumber, PMTPID: pid})
		}
	}
	return out, 3 + sectionLen
}

func parsePMT(payload []byte, programNumber uint16) ([]tsStream, uint16, int, int) {
	if len(payload) < 12 {
		return nil, 0, 0, 0
	}
	pointer := int(payload[0])
	if pointer+12 > len(payload) {
		return nil, 0, 0, 0
	}
	section := payload[1+pointer:]
	if len(section) < 12 {
		return nil, 0, 0, 0
	}
	sectionLen := int(binary.BigEndian.Uint16(section[1:3]) & 0x0FFF)
	if sectionLen+3 > len(section) {
		return nil, 0, 0, 0
	}
	pcrPID := binary.BigEndian.Uint16(section[8:10]) & 0x1FFF
	programInfoLen := int(binary.BigEndian.Uint16(section[10:12]) & 0x0FFF)
	pos := 12 + programInfoLen
	end := 3 + sectionLen - 4
	if pos > end {
		return nil, pcrPID, pointer, sectionLen
	}
	streams := make([]tsStream, 0)
	for pos+5 <= end {
		streamType := section[pos]
		pid := binary.BigEndian.Uint16(section[pos+1:pos+3]) & 0x1FFF
		esInfoLen := int(binary.BigEndian.Uint16(section[pos+3:pos+5]) & 0x0FFF)
		language := ""
		descStart := pos + 5
		descEnd := descStart + esInfoLen
		if esInfoLen > 0 && descEnd <= end && descEnd <= len(section) {
			descs := section[descStart:descEnd]
			for i := 0; i+2 <= len(descs); {
				tag := descs[i]
				length := int(descs[i+1])
				i += 2
				if i+length > len(descs) {
					break
				}
				if tag == 0x0A && length >= 4 {
					code := string(descs[i : i+3])
					// Keep the first language descriptor value.
					if language == "" {
						language = strings.TrimSpace(code)
					}
				}
				i += length
			}
		}
		kind, format := mapTSStream(streamType)
		if kind != "" {
			streams = append(streams, tsStream{pid: pid, programNumber: programNumber, streamType: streamType, kind: kind, format: format, language: language})
		}
		pos += 5 + esInfoLen
	}
	return streams, pcrPID, pointer, sectionLen
}

func mapTSStream(streamType byte) (StreamKind, string) {
	switch streamType {
	case 0x01:
		return StreamVideo, "MPEG Video"
	case 0x02:
		return StreamVideo, "MPEG Video"
	case 0x10:
		return StreamVideo, "MPEG-4 Visual"
	case 0x1B:
		return StreamVideo, "AVC"
	case 0x24:
		return StreamVideo, "HEVC"
	case 0xEA:
		return StreamVideo, "VC-1"
	case 0x03:
		return StreamAudio, "MPEG Audio"
	case 0x04:
		return StreamAudio, "MPEG Audio"
	case 0x0F:
		return StreamAudio, "AAC"
	case 0x11:
		return StreamAudio, "AAC"
	case 0x81:
		return StreamAudio, "AC-3"
	case 0x06:
		return StreamText, "Private"
	case 0x90:
		return StreamText, "PGS"
	default:
		return "", ""
	}
}

func formatStreamID(pid uint16) string {
	return formatID(uint64(pid))
}

func formatTSCodecID(streamType byte) string {
	return strconv.FormatUint(uint64(streamType), 10)
}

func parseH264FromPES(data []byte) ([]Field, h264SPSInfo, bool) {
	return parseH264AnnexB(data)
}

func processPES(entry *tsStream) {
	if entry.kind == StreamVideo && entry.format == "AVC" && !entry.hasVideoFields && len(entry.pesData) > 0 {
		if fields, sps, ok := parseH264FromPES(entry.pesData); ok && len(fields) > 0 {
			width, height, fps := sps.Width, sps.Height, sps.FrameRate
			entry.videoFields = fields
			entry.hasVideoFields = true
			entry.width = width
			entry.height = height
			if fps > 0 {
				entry.videoFrameRate = fps
			}
		}
	}
	if entry.kind == StreamVideo && (entry.writingLibrary == "" || entry.encoding == "") && len(entry.pesData) > 0 {
		writingLib, encoding := findX264Info(entry.pesData)
		if writingLib != "" && entry.writingLibrary == "" {
			entry.writingLibrary = writingLib
		}
		if encoding != "" && entry.encoding == "" {
			entry.encoding = encoding
		}
	}
	entry.pesData = entry.pesData[:0]
}

func consumeAudio(entry *tsStream, payload []byte, collectAC3Stats bool, ac3StatsHead bool) {
	switch entry.format {
	case "AAC":
		if entry.audioSpf == 0 {
			entry.audioSpf = 1024
		}
		if entry.audioBitRateMode == "" {
			entry.audioBitRateMode = "Variable"
		}
		consumeADTS(entry, payload)
	case "AC-3":
		if entry.audioBitRateMode == "" {
			entry.audioBitRateMode = "Constant"
		}
		consumeAC3(entry, payload, collectAC3Stats, ac3StatsHead)
	case "E-AC-3":
		if entry.audioBitRateMode == "" {
			entry.audioBitRateMode = "Variable"
		}
		consumeEAC3(entry, payload, collectAC3Stats, ac3StatsHead)
	default:
	}
}

func inferBDAVStream(pid uint16, data []byte) (StreamKind, string, byte, bool) {
	// Common Blu-ray PID layout:
	// 0x12xx: PGS subtitle streams.
	if pid&0xFF00 == 0x1200 {
		return StreamText, "PGS", 0x90, true
	}

	// 0x11xx: audio streams (codec sniff).
	if pid&0xFF00 == 0x1100 {
		if len(data) > 0 {
			// AC-3 / E-AC-3 sync word
			if idx := bytes.Index(data, []byte{0x0B, 0x77}); idx >= 0 && idx < 64 {
				if _, _, ok := parseEAC3FrameWithOptions(data[idx:], true); ok {
					return StreamAudio, "E-AC-3", 0x84, true
				}
				if _, _, ok := parseAC3Frame(data[idx:]); ok {
					return StreamAudio, "AC-3", 0x81, true
				}
			}
			// ADTS AAC
			if len(data) >= 2 && data[0] == 0xFF && (data[1]&0xF0) == 0xF0 {
				return StreamAudio, "AAC", 0x0F, true
			}
		}
		return StreamAudio, "Private", 0x06, true
	}

	return "", "", 0, false
}

func consumeADTS(entry *tsStream, payload []byte) {
	if len(payload) == 0 {
		return
	}
	entry.audioBuffer = append(entry.audioBuffer, payload...)
	i := 0
	for i+7 <= len(entry.audioBuffer) {
		if entry.audioBuffer[i] != 0xFF || (entry.audioBuffer[i+1]&0xF0) != 0xF0 {
			i++
			continue
		}
		if (entry.audioBuffer[i+1] & 0x06) != 0 {
			i++
			continue
		}
		mpegID := (entry.audioBuffer[i+1] >> 3) & 0x01
		protectionAbsent := entry.audioBuffer[i+1] & 0x01
		profile := (entry.audioBuffer[i+2] >> 6) & 0x03
		samplingIndex := (entry.audioBuffer[i+2] >> 2) & 0x0F
		channelConfig := ((entry.audioBuffer[i+2] & 0x01) << 2) | ((entry.audioBuffer[i+3] >> 6) & 0x03)
		frameLen := ((int(entry.audioBuffer[i+3]) & 0x03) << 11) | (int(entry.audioBuffer[i+4]) << 3) | ((int(entry.audioBuffer[i+5]) >> 5) & 0x07)
		headerLen := 7
		if protectionAbsent == 0 {
			headerLen = 9
		}
		if samplingIndex == 0x0F || frameLen < headerLen {
			i++
			continue
		}
		if i+frameLen > len(entry.audioBuffer) {
			break
		}
		if i+frameLen+1 < len(entry.audioBuffer) {
			if entry.audioBuffer[i+frameLen] != 0xFF || (entry.audioBuffer[i+frameLen+1]&0xF0) != 0xF0 {
				i++
				continue
			}
		}
		entry.audioFrames++
		if !entry.hasAudioInfo {
			objType := int(profile) + 1
			sampleRate := adtsSampleRate(int(samplingIndex))
			if sampleRate > 0 {
				entry.audioProfile = mapAACProfile(objType)
				entry.audioObject = objType
				entry.audioMPEGVersion = adtsMPEGVersion(mpegID)
				entry.audioRate = sampleRate
				entry.audioChannels = uint64(channelConfig)
				entry.audioSpf = 1024
				entry.hasAudioInfo = true
			}
		}
		i += frameLen
	}
	if i > 0 {
		entry.audioBuffer = append(entry.audioBuffer[:0], entry.audioBuffer[i:]...)
	}
}

const ac3SampleMax = 512

func pushAC3Sample(entry *tsStream, info ac3Info) {
	if len(entry.ac3Head) < ac3SampleMax {
		entry.ac3Head = append(entry.ac3Head, info)
	}
	if !entry.ac3TailFull {
		entry.ac3Tail = append(entry.ac3Tail, info)
		if len(entry.ac3Tail) == ac3SampleMax {
			entry.ac3TailFull = true
			entry.ac3TailPos = 0
		}
		return
	}
	entry.ac3Tail[entry.ac3TailPos] = info
	entry.ac3TailPos++
	if entry.ac3TailPos >= ac3SampleMax {
		entry.ac3TailPos = 0
	}
}

func orderedAC3Tail(entry *tsStream) []ac3Info {
	if !entry.ac3TailFull {
		return entry.ac3Tail
	}
	out := make([]ac3Info, 0, len(entry.ac3Tail))
	out = append(out, entry.ac3Tail[entry.ac3TailPos:]...)
	out = append(out, entry.ac3Tail[:entry.ac3TailPos]...)
	return out
}

func consumeAC3(entry *tsStream, payload []byte, collectStats bool, statsHead bool) {
	if len(payload) == 0 {
		return
	}
	entry.audioBuffer = append(entry.audioBuffer, payload...)
	i := 0
	for i+7 <= len(entry.audioBuffer) {
		if entry.audioBuffer[i] != 0x0B || entry.audioBuffer[i+1] != 0x77 {
			i++
			continue
		}
		info, frameSize, ok := parseAC3Frame(entry.audioBuffer[i:])
		if !ok || frameSize <= 0 {
			i++
			continue
		}
		if i+frameSize > len(entry.audioBuffer) {
			break
		}
		// Avoid false-positive sync matches by requiring the next frame sync when available.
		if i+frameSize+1 < len(entry.audioBuffer) {
			if entry.audioBuffer[i+frameSize] != 0x0B || entry.audioBuffer[i+frameSize+1] != 0x77 {
				i++
				continue
			}
		}
		entry.audioFrames++
		entry.hasAC3 = true
		entry.ac3Info.mergeFrameBase(info)
		if collectStats {
			entry.ac3Stats.mergeFrame(info)
		}
		if !entry.hasAudioInfo && info.sampleRate > 0 {
			entry.audioRate = info.sampleRate
			entry.audioChannels = info.channels
			entry.audioSpf = info.spf
			entry.audioBitRateKbps = info.bitRateKbps
			entry.hasAudioInfo = true
		}
		i += frameSize
	}
	if i > 0 {
		entry.audioBuffer = append(entry.audioBuffer[:0], entry.audioBuffer[i:]...)
	}
}

func consumeEAC3(entry *tsStream, payload []byte, collectStats bool, statsHead bool) {
	if len(payload) == 0 {
		return
	}
	entry.audioBuffer = append(entry.audioBuffer, payload...)
	i := 0
	for i+7 <= len(entry.audioBuffer) {
		if entry.audioBuffer[i] != 0x0B || entry.audioBuffer[i+1] != 0x77 {
			i++
			continue
		}
		info, frameSize, ok := parseEAC3FrameWithOptions(entry.audioBuffer[i:], true)
		if !ok || frameSize <= 0 {
			i++
			continue
		}
		if i+frameSize > len(entry.audioBuffer) {
			break
		}
		// Avoid false-positive sync matches by requiring the next frame sync when available.
		if i+frameSize+1 < len(entry.audioBuffer) {
			if entry.audioBuffer[i+frameSize] != 0x0B || entry.audioBuffer[i+frameSize+1] != 0x77 {
				i++
				continue
			}
		}
		entry.audioFrames++
		entry.hasAC3 = true
		entry.ac3Info.mergeFrameBase(info)
		if collectStats {
			entry.ac3Stats.mergeFrame(info)
		}
		if !entry.hasAudioInfo && info.sampleRate > 0 {
			entry.audioRate = info.sampleRate
			entry.audioChannels = info.channels
			entry.audioSpf = info.spf
			entry.audioBitRateKbps = info.bitRateKbps
			entry.hasAudioInfo = true
		}
		i += frameSize
	}
	if i > 0 {
		entry.audioBuffer = append(entry.audioBuffer[:0], entry.audioBuffer[i:]...)
	}
}

func adtsSampleRate(index int) float64 {
	switch index {
	case 0:
		return 96000
	case 1:
		return 88200
	case 2:
		return 64000
	case 3:
		return 48000
	case 4:
		return 44100
	case 5:
		return 32000
	case 6:
		return 24000
	case 7:
		return 22050
	case 8:
		return 16000
	case 9:
		return 12000
	case 10:
		return 11025
	case 11:
		return 8000
	case 12:
		return 7350
	default:
		return 0
	}
}

func parsePCR27(packet []byte) (uint64, bool) {
	adaptation := (packet[3] & 0x30) >> 4
	if adaptation != 2 && adaptation != 3 {
		return 0, false
	}
	adaptLen := int(packet[4])
	if adaptLen < 7 || 5+adaptLen > len(packet) {
		return 0, false
	}
	flags := packet[5]
	if flags&0x10 == 0 {
		return 0, false
	}
	base := (uint64(packet[6]) << 25) |
		(uint64(packet[7]) << 17) |
		(uint64(packet[8]) << 9) |
		(uint64(packet[9]) << 1) |
		(uint64(packet[10]) >> 7)
	ext := (uint64(packet[10]&0x01) << 8) | uint64(packet[11])
	return base*300 + ext, true
}

func parseSDT(payload []byte, programNumber uint16) (string, string, string) {
	if len(payload) < 11 {
		return "", "", ""
	}
	pointer := int(payload[0])
	if pointer+11 > len(payload) {
		return "", "", ""
	}
	section := payload[1+pointer:]
	if len(section) < 11 || section[0] != 0x42 {
		return "", "", ""
	}
	sectionLen := int(binary.BigEndian.Uint16(section[1:3]) & 0x0FFF)
	if sectionLen+3 > len(section) {
		return "", "", ""
	}
	services := section[11 : 3+sectionLen-4]
	pos := 0
	for pos+5 <= len(services) {
		serviceID := binary.BigEndian.Uint16(services[pos : pos+2])
		descLen := int(binary.BigEndian.Uint16(services[pos+3:pos+5]) & 0x0FFF)
		descStart := pos + 5
		descEnd := descStart + descLen
		if descEnd > len(services) {
			break
		}
		if programNumber == 0 || serviceID == programNumber {
			name, provider, serviceType := parseServiceDescriptor(services[descStart:descEnd])
			return name, provider, serviceType
		}
		pos = descEnd
	}
	return "", "", ""
}

func parseServiceDescriptor(buf []byte) (string, string, string) {
	pos := 0
	for pos+2 <= len(buf) {
		tag := buf[pos]
		length := int(buf[pos+1])
		dataStart := pos + 2
		dataEnd := dataStart + length
		if dataEnd > len(buf) {
			break
		}
		if tag == 0x48 && length >= 2 {
			data := buf[dataStart:dataEnd]
			if len(data) < 2 {
				return "", "", ""
			}
			serviceType := mapServiceType(data[0])
			provLen := int(data[1])
			if 2+provLen >= len(data) {
				return "", "", serviceType
			}
			provider := string(data[2 : 2+provLen])
			nameLen := int(data[2+provLen])
			if 3+provLen+nameLen > len(data) {
				return "", provider, serviceType
			}
			name := string(data[3+provLen : 3+provLen+nameLen])
			return name, provider, serviceType
		}
		pos = dataEnd
	}
	return "", "", ""
}

func mapServiceType(value byte) string {
	switch value {
	case 0x01:
		return "digital television"
	case 0x02:
		return "digital radio sound"
	default:
		return ""
	}
}

func scanTSForH264(file io.ReadSeeker, pid uint16, size int64) ([]Field, uint64, uint64, uint64, float64) {
	return scanTSForH264WithPacketSize(file, pid, size, 188)
}

func scanBDAVForH264(file io.ReadSeeker, pid uint16, size int64) ([]Field, uint64, uint64, uint64, float64) {
	return scanTSForH264WithPacketSize(file, pid, size, 192)
}

func scanTSForH264WithPacketSize(file io.ReadSeeker, pid uint16, size int64, packetSize int64) ([]Field, uint64, uint64, uint64, float64) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, 0, 0, 0, 0
	}
	const tsPacketSize = int64(188)
	if packetSize != tsPacketSize && packetSize != 192 {
		packetSize = tsPacketSize
	}
	tsOffset := int64(0)
	if packetSize == 192 {
		tsOffset = 4
	}
	if off, ok := findTSSyncOffset(file, packetSize, tsOffset, size); ok {
		if _, err := file.Seek(off, io.SeekStart); err != nil {
			return nil, 0, 0, 0, 0
		}
	} else {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, 0, 0, 0, 0
		}
	}
	reader := bufio.NewReaderSize(file, 1<<20)
	buf := make([]byte, packetSize*2048)
	var pesData []byte
	readPackets := int64(0)
	maxPackets := int64(0)
	if size > 0 {
		maxPackets = size / packetSize
	}
	carry := 0
	for maxPackets == 0 || readPackets < maxPackets {
		n, err := reader.Read(buf[carry:])
		if n == 0 && err != nil {
			break
		}
		n += carry
		packetCount := n / int(packetSize)
		for i := 0; i < packetCount; i++ {
			packet := buf[int64(i)*packetSize : (int64(i)+1)*packetSize]
			readPackets++
			if maxPackets > 0 && readPackets > maxPackets {
				break
			}
			if tsOffset+tsPacketSize > int64(len(packet)) {
				continue
			}
			ts := packet[tsOffset : tsOffset+tsPacketSize]
			if ts[0] != 0x47 {
				continue
			}
			pktPid := uint16(ts[1]&0x1F)<<8 | uint16(ts[2])
			if pktPid != pid {
				continue
			}
			payloadStart := ts[1]&0x40 != 0
			adaptation := (ts[3] & 0x30) >> 4
			payloadIndex := 4
			switch adaptation {
			case 2:
				continue
			case 3:
				adaptLen := int(ts[4])
				payloadIndex += 1 + adaptLen
			}
			if payloadIndex >= len(ts) {
				continue
			}
			payload := ts[payloadIndex:]
			if payloadStart && len(payload) >= 9 && payload[0] == 0x00 && payload[1] == 0x00 && payload[2] == 0x01 {
				if len(pesData) > 0 {
					if fields, sps, ok := parseH264AnnexB(pesData); ok && len(fields) > 0 {
						return fields, sps.Width, sps.Height, sps.CodedHeight, sps.FrameRate
					}
				}
				headerLen := int(payload[8])
				dataStart := min(9+headerLen, len(payload))
				pesData = append(pesData[:0], payload[dataStart:]...)
				continue
			}
			if len(pesData) > 0 {
				pesData = append(pesData, payload...)
			}
		}
		carry = n - packetCount*int(packetSize)
		if carry > 0 {
			copy(buf, buf[packetCount*int(packetSize):n])
		}
		if maxPackets > 0 && readPackets >= maxPackets {
			break
		}
		if err != nil {
			break
		}
	}
	if len(pesData) > 0 {
		if fields, sps, ok := parseH264AnnexB(pesData); ok && len(fields) > 0 {
			return fields, sps.Width, sps.Height, sps.CodedHeight, sps.FrameRate
		}
	}
	return nil, 0, 0, 0, 0
}
