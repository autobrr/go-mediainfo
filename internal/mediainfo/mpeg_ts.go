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
	pid              uint16
	programNumber    uint16
	streamType       byte
	kind             StreamKind
	format           string
	frames           uint64
	bytes            uint64
	pts              ptsTracker
	hasAC3           bool
	ac3Info          ac3Info
	ac3StatsBytes    uint64
	width            uint64
	height           uint64
	videoFields      []Field
	hasVideoFields   bool
	audioProfile     string
	audioObject      int
	audioMPEGVersion int
	audioRate        float64
	audioChannels    uint64
	hasAudioInfo     bool
	audioFrames      uint64
	audioSpf         int
	audioBitRateKbps int64
	audioBitRateMode string
	videoFrameRate   float64
	pesData          []byte
	audioBuffer      []byte
	audioStarted     bool
	writingLibrary   string
	encoding         string
}

func ParseMPEGTS(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, []Field, bool) {
	return parseMPEGTSWithPacketSize(file, size, 188)
}

// ParseBDAV parses BDAV/M2TS streams (192-byte packets: 4-byte timestamp + 188-byte TS packet).
func ParseBDAV(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, []Field, bool) {
	return parseMPEGTSWithPacketSize(file, size, 192)
}

const tsPTSGap = 30 * 90000 // 30 seconds

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

func parseMPEGTSWithPacketSize(file io.ReadSeeker, size int64, packetSize int64) (ContainerInfo, []Stream, []Field, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, nil, false
	}

	reader := bufio.NewReaderSize(file, 1<<20)

	var primaryPMTPID uint16
	var primaryProgramNumber uint16
	pmtPIDToProgram := map[uint16]uint16{}
	var pcrPID uint16
	var serviceName string
	var serviceProvider string
	var serviceType string
	streams := map[uint16]*tsStream{}
	streamOrder := []uint16{}
	pendingPTS := map[uint16]*ptsTracker{}
	var videoPTS ptsTracker
	var anyPTS ptsTracker
	videoPIDs := map[uint16]struct{}{}
	var pcrPTS ptsTracker
	var pcrFull pcrTracker
	var lastPCR uint64 // 27MHz ticks
	var lastPCRPacket int64
	var hasPCR bool
	var pcrBits float64
	var pcrSeconds float64
	var pmtPointer int
	var pmtSectionLen int
	pmtAssemblies := map[uint16]*psiAssembly{}

	const tsPacketSize = int64(188)
	if packetSize != tsPacketSize && packetSize != 192 {
		packetSize = tsPacketSize
	}
	isBDAV := packetSize == 192
	tsOffset := int64(0)
	if packetSize == 192 {
		tsOffset = 4
	}
	buf := make([]byte, packetSize*2048)
	var packetIndex int64
	var tsPacketCount int64
	var psiBytes int64
	var carry int
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
				if pid == pcrPID {
					if pcr27, ok := parsePCR27(ts); ok {
						pcrFull.add(pcr27)
						// Keep legacy 90kHz PCR base for fields/flows that expect it.
						pcrPTS.add(pcr27 / 300)
						if hasPCR && pcr27 > lastPCR && packetIndex > lastPCRPacket {
							delta := pcr27 - lastPCR
							seconds := float64(delta) / 27000000.0
							if seconds > 0 {
								bytesBetween := (packetIndex - lastPCRPacket) * packetSize
								pcrBits += float64(bytesBetween * 8)
								pcrSeconds += seconds
							}
						}
						lastPCR = pcr27
						lastPCRPacket = packetIndex
						hasPCR = true
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
						if pcr != 0 {
							pcrPID = pcr
						}
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
							if pending, ok := pendingPTS[st.pid]; ok {
								existing.pts = *pending
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
						addPTS(&anyPTS, pts)
						if entry, ok := streams[pid]; ok {
							addPTS(&entry.pts, pts)
							if _, ok := videoPIDs[pid]; ok {
								addPTS(&videoPTS, pts)
							}
						} else {
							pending := pendingPTS[pid]
							if pending == nil {
								pending = &ptsTracker{}
								pendingPTS[pid] = pending
							}
							addPTS(pending, pts)
						}
					}
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

			entry, ok := streams[pid]
			if !ok {
				continue
			}

			if pesStart {
				if len(entry.pesData) > 0 {
					processPES(entry)
				}
				headerLen := int(payload[8])
				dataStart := min(9+headerLen, len(payload))
				data := payload[dataStart:]
				entry.bytes += uint64(len(data))
				if entry.kind == StreamVideo && len(data) > 0 {
					entry.pesData = append(entry.pesData[:0], data...)
				}

				if entry.kind == StreamVideo && !entry.hasVideoFields && len(data) > 0 {
					if fields, width, height, fps := parseH264FromPES(data); len(fields) > 0 {
						entry.videoFields = fields
						entry.hasVideoFields = true
						entry.width = width
						entry.height = height
						if fps > 0 {
							entry.videoFrameRate = fps
						}
					}
				}
				if entry.kind == StreamAudio {
					if !entry.audioStarted {
						entry.audioBuffer = entry.audioBuffer[:0]
						entry.audioStarted = true
					}
					if len(data) > 0 {
						consumeAudio(entry, data)
					}
				}

				if _, ok := videoPIDs[pid]; ok {
					entry.frames++
				}
				continue
			}

			if entry.kind == StreamVideo && len(entry.pesData) > 0 {
				entry.pesData = append(entry.pesData, payload...)
			}
			if entry.kind == StreamAudio && entry.audioStarted && len(payload) > 0 {
				entry.bytes += uint64(len(payload))
				consumeAudio(entry, payload)
			}
			if entry.kind != StreamAudio && len(payload) > 0 {
				entry.bytes += uint64(len(payload))
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

	for _, entry := range streams {
		if len(entry.pesData) > 0 {
			processPES(entry)
		}
	}
	for _, entry := range streams {
		if entry.kind != StreamVideo || entry.hasVideoFields {
			continue
		}
		var fields []Field
		var width uint64
		var height uint64
		var fps float64
		if packetSize == 192 {
			fields, width, height, fps = scanBDAVForH264(file, entry.pid, size)
		} else {
			fields, width, height, fps = scanTSForH264(file, entry.pid, size)
		}
		if len(fields) > 0 {
			entry.videoFields = fields
			entry.hasVideoFields = true
			entry.width = width
			entry.height = height
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
			if isBDAV && st.bytes > 0 && (st.kind == StreamVideo || st.kind == StreamAudio) {
				jsonExtras["StreamSize"] = strconv.FormatUint(st.bytes, 10)
			}
		if st.programNumber > 0 {
			jsonExtras["MenuID"] = strconv.FormatUint(uint64(st.programNumber), 10)
		}
		if st.pts.has() {
			delay := float64(st.pts.min) / 90000.0
			jsonExtras["Delay"] = fmt.Sprintf("%.9f", delay)
			jsonExtras["Delay_Source"] = "Container"
		}
		if st.kind == StreamAudio && videoPTS.has() && st.pts.has() {
			videoDelay := float64(int64(st.pts.min)-int64(videoPTS.min)) / 90000.0
			jsonExtras["Video_Delay"] = fmt.Sprintf("%.3f", videoDelay)
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
			if duration > 0 && st.videoFrameRate > 0 {
				duration += 1.0 / st.videoFrameRate
			}
			if duration > 0 {
				fields = addStreamDuration(fields, duration)
				if isBDAV {
					jsonExtras["Duration"] = fmt.Sprintf("%.3f", duration)
					if st.videoFrameRate > 0 {
						jsonExtras["FrameCount"] = strconv.Itoa(int(math.Round(duration * st.videoFrameRate)))
					}
				}
			}
			if isBDAV && st.videoFrameRate > 0 {
				fields = append(fields, Field{Name: "Frame rate", Value: fmt.Sprintf("%.3f FPS", st.videoFrameRate)})
			}
			fields = append(fields, Field{Name: "Frame rate mode", Value: "Variable"})
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
		}
			if st.kind == StreamAudio {
				duration := ptsDuration(st.pts)
				if st.audioRate > 0 && st.audioFrames > 0 && st.audioSpf > 0 {
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
				if layout := channelLayout(st.audioChannels); layout != "" {
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
					if avg, minVal, maxVal, ok := st.ac3Info.dialnormStats(); ok {
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Average", Val: strconv.Itoa(avg)})
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Minimum", Val: strconv.Itoa(minVal)})
						_ = maxVal
					}
					if avg, minVal, maxVal, count, ok := st.ac3Info.comprStats(); ok {
						extraFields = append(extraFields, jsonKV{Key: "compr_Average", Val: fmt.Sprintf("%.2f", avg)})
						extraFields = append(extraFields, jsonKV{Key: "compr_Minimum", Val: fmt.Sprintf("%.2f", minVal)})
						extraFields = append(extraFields, jsonKV{Key: "compr_Maximum", Val: fmt.Sprintf("%.2f", maxVal)})
						extraFields = append(extraFields, jsonKV{Key: "compr_Count", Val: strconv.Itoa(count)})
					}
					if avg, minVal, maxVal, count, ok := st.ac3Info.dynrngStats(); ok {
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

	info := ContainerInfo{}
	if pcrSeconds > 0 && pcrBits > 0 && size > 0 {
		overallBitrate := pcrBits / pcrSeconds
		if overallBitrate > 0 {
			if packetSize != 192 {
				info.DurationSeconds = float64(size*8) / overallBitrate
			}
			precision := overallBitrate / 9600.0
			info.OverallBitrateMin = overallBitrate - precision
			info.OverallBitrateMax = overallBitrate + precision
		}
	}
	if info.DurationSeconds == 0 {
		if packetSize == 192 {
			if duration := pcrFull.durationSeconds(); duration > 0 {
				info.DurationSeconds = duration
			} else if duration := ptsDuration(videoPTS); duration > 0 {
				info.DurationSeconds = duration
			} else if duration := ptsDuration(anyPTS); duration > 0 {
				info.DurationSeconds = duration
			}
		} else if duration := ptsDuration(videoPTS); duration > 0 {
			info.DurationSeconds = duration
		} else if duration := ptsDuration(anyPTS); duration > 0 {
			info.DurationSeconds = duration
		}
	}
	info.BitrateMode = "Variable"
	if tsPacketCount > 0 {
		base := tsPacketCount * (tsOffset + 4)
		info.StreamOverheadBytes = base + psiBytes
	}

	generalFields := []Field{}
	if primaryProgramNumber > 0 {
		generalFields = append(generalFields, Field{Name: "ID", Value: formatID(uint64(primaryProgramNumber))})
	}

	// MediaInfo CLI doesn't output a Menu track for BDAV/M2TS.
	if primaryPMTPID != 0 && packetSize == tsPacketSize {
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
		kind, format := mapTSStream(streamType)
		if kind != "" {
			streams = append(streams, tsStream{pid: pid, programNumber: programNumber, streamType: streamType, kind: kind, format: format})
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

func parseH264FromPES(data []byte) ([]Field, uint64, uint64, float64) {
	return parseH264AnnexB(data)
}

func processPES(entry *tsStream) {
	if entry.kind == StreamVideo && !entry.hasVideoFields && len(entry.pesData) > 0 {
		if fields, width, height, fps := parseH264FromPES(entry.pesData); len(fields) > 0 {
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

func consumeAudio(entry *tsStream, payload []byte) {
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
		consumeAC3(entry, payload)
	case "E-AC-3":
		if entry.audioBitRateMode == "" {
			entry.audioBitRateMode = "Variable"
		}
		consumeEAC3(entry, payload)
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

func consumeAC3(entry *tsStream, payload []byte) {
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
		entry.audioFrames++
		entry.hasAC3 = true
		// MediaInfo seems to sample only a limited window for AC-3 metadata stats.
		// Keep parity and avoid doing a full-file pass.
		const maxStatsBytes = 494 * 1024
		if entry.ac3StatsBytes < maxStatsBytes {
			entry.ac3Info.mergeFrame(info)
			entry.ac3StatsBytes += uint64(frameSize)
		}
		if !entry.hasAudioInfo && entry.ac3Info.sampleRate > 0 {
			entry.audioRate = entry.ac3Info.sampleRate
			entry.audioChannels = entry.ac3Info.channels
			entry.audioSpf = entry.ac3Info.spf
			entry.audioBitRateKbps = entry.ac3Info.bitRateKbps
			entry.hasAudioInfo = true
		}
		i += frameSize
	}
	if i > 0 {
		entry.audioBuffer = append(entry.audioBuffer[:0], entry.audioBuffer[i:]...)
	}
}

func consumeEAC3(entry *tsStream, payload []byte) {
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
		entry.audioFrames++
		entry.hasAC3 = true
		const maxStatsBytes = 494 * 1024
		if entry.ac3StatsBytes < maxStatsBytes {
			entry.ac3Info.mergeFrame(info)
			entry.ac3StatsBytes += uint64(frameSize)
		}
		if !entry.hasAudioInfo && entry.ac3Info.sampleRate > 0 {
			entry.audioRate = entry.ac3Info.sampleRate
			entry.audioChannels = entry.ac3Info.channels
			entry.audioSpf = entry.ac3Info.spf
			entry.audioBitRateKbps = entry.ac3Info.bitRateKbps
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

func scanTSForH264(file io.ReadSeeker, pid uint16, size int64) ([]Field, uint64, uint64, float64) {
	return scanTSForH264WithPacketSize(file, pid, size, 188)
}

func scanBDAVForH264(file io.ReadSeeker, pid uint16, size int64) ([]Field, uint64, uint64, float64) {
	return scanTSForH264WithPacketSize(file, pid, size, 192)
}

func scanTSForH264WithPacketSize(file io.ReadSeeker, pid uint16, size int64, packetSize int64) ([]Field, uint64, uint64, float64) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, 0, 0, 0
	}
	reader := bufio.NewReaderSize(file, 1<<20)
	const tsPacketSize = int64(188)
	if packetSize != tsPacketSize && packetSize != 192 {
		packetSize = tsPacketSize
	}
	tsOffset := int64(0)
	if packetSize == 192 {
		tsOffset = 4
	}
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
					if fields, width, height, fps := parseH264AnnexB(pesData); len(fields) > 0 {
						return fields, width, height, fps
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
		if fields, width, height, fps := parseH264AnnexB(pesData); len(fields) > 0 {
			return fields, width, height, fps
		}
	}
	return nil, 0, 0, 0
}
