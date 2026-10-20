package mediainfo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
)

type psStreamParser struct {
	streams         map[uint16]*psStream
	streamOrder     []uint16
	videoParsers    map[uint16]*mpeg2VideoParser
	videoPTS        ptsTracker
	anyPTS          ptsTracker
	packetOrder     int
	quickAC3        bool
	quickAC3Max     uint64
	section         int
	afterProgramEnd bool
}

type psPending struct {
	entry      *psStream
	key        uint16
	flags      byte
	pts        uint64
	hasPTS     bool
	payloadPos int
	skip       int
}

const mpegPSTerminalTailMax = 16

func newPSStreamParser(opts mpegPSOptions) *psStreamParser {
	parseSpeed := opts.parseSpeed
	if parseSpeed == 0 {
		parseSpeed = 1
	}
	return &psStreamParser{
		streams:      map[uint16]*psStream{},
		streamOrder:  []uint16{},
		videoParsers: map[uint16]*mpeg2VideoParser{},
		quickAC3:     parseSpeed < 1 && !opts.dvdExtras && !opts.dvdParsing,
		quickAC3Max:  128,
	}
}

// beginSection resets only state that cannot cross a bounded-read jump and
// starts a new timing section for per-stream frame-clock reconstruction.
func (p *psStreamParser) beginSection() {
	p.section++
	for key, entry := range p.streams {
		entry.audioBuffer = nil
		entry.videoHeaderCarry = nil
		entry.videoFrameCarry = nil
		entry.videoCCCarry = nil
		entry.videoBuffer = nil
		entry.clockHasPTS = false
		entry.programEndSeen = false
		entry.terminalTracked = false
		entry.terminalBytes = nil
		entry.sampleSection = p.section
		if parser := p.videoParsers[key]; parser != nil {
			parser.startSegment()
		}
	}
}

// recordPTS retains container PTS history and the latest timestamp/frame clock
// in each sampled section. Elementary parsers advance from that anchor.
func (p *psStreamParser) recordPTS(entry *psStream, currentPTS uint64) {
	if entry.sampleSection != p.section {
		entry.sampleSection = p.section
		entry.clockHasPTS = false
	}
	entry.clockPTS = currentPTS
	entry.clockAudioStart = entry.audioFrames
	entry.clockVideoStart = entry.videoFrameCount
	entry.clockHasPTS = true
	p.anyPTS.add(currentPTS)
	entry.pts.add(currentPTS)
	if entry.kind == StreamVideo {
		p.videoPTS.add(currentPTS)
	}
}

func (p *psStreamParser) parseReader(r io.Reader) bool {
	const chunkSize = 1 << 20
	buf := make([]byte, 0, chunkSize*2)
	tmp := make([]byte, chunkSize)
	pos := 0
	found := false
	eof := false
	var pending *psPending

	readMore := func() bool {
		if eof {
			return false
		}
		for {
			n, err := r.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				return true
			}
			if err == io.EOF {
				eof = true
				return false
			}
			if err != nil {
				eof = true
				return false
			}
		}
	}
	compact := func() {
		if pos > chunkSize {
			buf = append(buf[:0], buf[pos:]...)
			pos = 0
		}
	}

parseLoop:
	for {
		if pending != nil {
			if pending.payloadPos >= len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			if pending.skip > 0 {
				avail := len(buf) - pending.payloadPos
				if avail <= pending.skip {
					pending.payloadPos += avail
					pending.skip -= avail
					if !readMore() {
						return found
					}
					continue
				}
				pending.payloadPos += pending.skip
				pending.skip = 0
			}
			next := findPESStart(buf, pending.payloadPos)
			if next >= 0 {
				if next > pending.payloadPos {
					p.consumePayload(pending.entry, pending.key, pending.flags, pending.pts, pending.hasPTS, buf[pending.payloadPos:next])
					found = true
				}
				pos = next
				pending = nil
				continue
			}
			safeEnd := len(buf) - 2
			if safeEnd > pending.payloadPos {
				p.consumePayload(pending.entry, pending.key, pending.flags, pending.pts, pending.hasPTS, buf[pending.payloadPos:safeEnd])
				found = true
				pending.payloadPos = safeEnd
			}
			if !readMore() {
				if pending.payloadPos < len(buf) {
					p.consumePayload(pending.entry, pending.key, pending.flags, pending.pts, pending.hasPTS, buf[pending.payloadPos:])
					found = true
				}
				return found
			}
			if pending.payloadPos > chunkSize {
				buf = append(buf[:0], buf[pending.payloadPos:]...)
				pending.payloadPos = 0
				pos = 0
			}
			continue
		}

		idx := findPESStart(buf, pos)
		if idx < 0 {
			if eof {
				p.appendTerminalBytes(buf[pos:])
				return found
			}
			if len(buf) > 2 {
				safeEnd := len(buf) - 2
				if pos < safeEnd {
					p.appendTerminalBytes(buf[pos:safeEnd])
					buf = append(buf[:0], buf[safeEnd:]...)
					pos = 0
				} else if pos > 0 {
					buf = append(buf[:0], buf[pos:]...)
					pos = 0
				}
			}
			if !readMore() {
				p.appendTerminalBytes(buf[pos:])
				return found
			}
			continue
		}
		if p.afterProgramEnd {
			p.appendTerminalBytes(buf[pos:idx])
			p.afterProgramEnd = false
			for _, entry := range p.streams {
				entry.programEndSeen = false
				entry.terminalTracked = false
				entry.terminalBytes = nil
			}
		}
		pos = idx
		if pos+4 > len(buf) {
			if !readMore() {
				return found
			}
			continue
		}

		streamID := buf[pos+3]
		switch streamID {
		case 0xBA:
			if pos+5 > len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			needed := 0
			switch {
			case (buf[pos+4] & 0xC0) == 0x40:
				// MPEG-2 pack headers are 14 bytes plus the declared stuffing.
				if pos+14 > len(buf) {
					if !readMore() {
						return found
					}
					continue
				}
				needed = pos + 14 + int(buf[pos+13]&0x07)
			case (buf[pos+4] & 0xF0) == 0x20:
				// MPEG-1 pack headers have a fixed 12-byte size.
				needed = pos + 12
			default:
				pos++
				continue
			}
			if needed > len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			pos = needed
			compact()
			continue
		case 0xBB, 0xBC, 0xBE:
			if pos+6 > len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			length := int(binary.BigEndian.Uint16(buf[pos+4 : pos+6]))
			needed := pos + 6 + length
			if needed > len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			pos = needed
			compact()
			continue
		case 0xBF:
			if pos+6 > len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			length := int(binary.BigEndian.Uint16(buf[pos+4 : pos+6]))
			payloadStart := pos + 6
			payloadEnd := payloadStart + length
			if payloadEnd > len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			kind, format := mapPSStream(streamID, psSubstreamNone)
			if kind != "" {
				entry := p.ensureStream(streamID, psSubstreamNone, kind, format)
				if entry.kind != StreamMenu && entry.firstPacketOrder < 0 {
					entry.firstPacketOrder = p.packetOrder
					p.packetOrder++
				}
				if payloadEnd > payloadStart {
					entry.bytes += uint64(payloadEnd - payloadStart)
					found = true
				}
			}
			pos = payloadEnd
			compact()
			continue
		case 0xB9:
			for _, entry := range p.streams {
				entry.programEndSeen = true
				entry.terminalTracked = true
				// MediaInfoLib treats a lone zero left by the final MPEG-audio
				// payload as terminal sync loss at program_end_code. Snapshot it
				// into lifecycle-owned state; later program data clears it.
				if len(entry.audioBuffer) == 1 && entry.audioBuffer[0] == 0 {
					entry.terminalBytes = []byte{0}
				} else {
					entry.terminalBytes = nil
				}
			}
			p.afterProgramEnd = true
			pos += 4
			found = true
			compact()
			continue
		}

		if pos+7 > len(buf) {
			if !readMore() {
				return found
			}
			continue
		}
		pesLen := int(binary.BigEndian.Uint16(buf[pos+4 : pos+6]))
		flags := byte(0)
		payloadStart := 0
		ptsStart := -1
		headerBytes := 0
		if (buf[pos+6] & 0xC0) == 0x80 {
			// MPEG-2 PES optional headers carry flags and an explicit header length.
			if pos+9 > len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			flags = buf[pos+7]
			headerLen := int(buf[pos+8])
			headerBytes = 3 + headerLen
			payloadStart = pos + 9 + headerLen
			if flags&0x80 != 0 {
				ptsStart = pos + 9
			}
		} else {
			// MPEG-1 PES optional headers use stuffing and marker bits instead of
			// the MPEG-2 flags/header-length pair.
			header := pos + 6
			packetEnd := len(buf)
			if pesLen > 0 {
				packetEnd = pos + 6 + pesLen
			}
			stuffingBytes := 0
			for {
				if header >= packetEnd || stuffingBytes > 16 {
					pos++
					continue parseLoop
				}
				if header >= len(buf) {
					if !readMore() {
						return found
					}
					continue
				}
				if buf[header] != 0xFF {
					break
				}
				header++
				stuffingBytes++
			}
			if header >= len(buf) {
				continue
			}
			if buf[header]&0xC0 == 0x40 {
				if header+2 > len(buf) {
					if !readMore() {
						return found
					}
					continue
				}
				header += 2
			}
			if header >= len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			switch buf[header] & 0xF0 {
			case 0x20:
				ptsStart = header
				flags = 0x80
				header += 5
			case 0x30:
				ptsStart = header
				flags = 0x80
				header += 10
			case 0x00:
				if buf[header] != 0x0F {
					pos++
					continue
				}
				header++
			default:
				pos++
				continue
			}
			headerBytes = header - (pos + 6)
			payloadStart = header
		}
		if payloadStart > len(buf) {
			if !readMore() {
				return found
			}
			continue
		}
		if pesLen > 0 && headerBytes > pesLen {
			pos++
			continue
		}
		var currentPTS uint64
		var hasPTS bool
		if ptsStart >= 0 {
			if ptsStart+5 > len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			currentPTS, hasPTS = parsePTS(buf[ptsStart:])
		}

		payloadLen := 0
		if pesLen > 0 {
			payloadLen = max(pesLen-headerBytes, 0)
			payloadEnd := payloadStart + payloadLen
			if payloadEnd > len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			if payloadEnd < payloadStart {
				payloadEnd = payloadStart
			}
			payload := buf[payloadStart:payloadEnd]
			subID := byte(psSubstreamNone)
			payloadOffset := 0
			if streamID == 0xBD && len(payload) > 0 {
				subID = payload[0]
				payloadOffset = 1
				if subID >= 0x80 && subID <= 0x87 && len(payload) > 4 {
					payloadOffset = 4
				} else if subID >= 0xA0 && subID <= 0xAF && len(payload) > 7 {
					payloadOffset = 7
				}
			}
			kind, format := mapPSStream(streamID, subID)
			if kind != "" {
				entry := p.ensureStream(streamID, subID, kind, format)
				if format == "PCM" {
					consumeDVDLPCMHeader(entry, payload)
				}
				if entry.kind != StreamMenu && entry.firstPacketOrder < 0 {
					entry.firstPacketOrder = p.packetOrder
					p.packetOrder++
				}
				if hasPTS {
					p.recordPTS(entry, currentPTS)
				}
				if payloadOffset < len(payload) {
					p.consumePayload(entry, psStreamKey(streamID, subID), flags, currentPTS, hasPTS, payload[payloadOffset:])
					found = true
				}
			}
			pos = payloadEnd
			if pos <= payloadStart && pos < len(buf) {
				pos++
			}
			compact()
			continue
		}

		if streamID == 0xBD && payloadStart >= len(buf) {
			if !readMore() {
				return found
			}
			continue
		}
		subID := byte(psSubstreamNone)
		payloadOffset := 0
		if streamID == 0xBD && payloadStart < len(buf) {
			subID = buf[payloadStart]
			payloadOffset = 1
			if subID >= 0x80 && subID <= 0x87 {
				payloadOffset = 4
			} else if subID >= 0xA0 && subID <= 0xAF {
				payloadOffset = 7
			}
		}
		kind, format := mapPSStream(streamID, subID)
		if kind == "" {
			pos = payloadStart
			continue
		}
		entry := p.ensureStream(streamID, subID, kind, format)
		if format == "PCM" && payloadStart+7 <= len(buf) {
			consumeDVDLPCMHeader(entry, buf[payloadStart:])
		}
		if entry.kind != StreamMenu && entry.firstPacketOrder < 0 {
			entry.firstPacketOrder = p.packetOrder
			p.packetOrder++
		}
		if hasPTS {
			p.recordPTS(entry, currentPTS)
		}
		pending = &psPending{
			entry:      entry,
			key:        psStreamKey(streamID, subID),
			flags:      flags,
			pts:        currentPTS,
			hasPTS:     hasPTS,
			payloadPos: payloadStart,
			skip:       payloadOffset,
		}
		pos = payloadStart
	}
}

func (p *psStreamParser) ensureStream(streamID byte, subID byte, kind StreamKind, format string) *psStream {
	key := psStreamKey(streamID, subID)
	entry := p.streams[key]
	if entry == nil {
		entry = &psStream{
			id:                streamID,
			subID:             subID,
			kind:              kind,
			format:            format,
			firstPacketOrder:  -1,
			videoLastStartPos: -1,
		}
		entry.ccOdd.firstFrame = -1
		entry.ccOdd.lastFrame = -1
		entry.ccEven.firstFrame = -1
		entry.ccEven.lastFrame = -1
		p.streams[key] = entry
		p.streamOrder = append(p.streamOrder, key)
	}
	return entry
}

func (p *psStreamParser) consumePayload(entry *psStream, key uint16, flags byte, pts uint64, hasPTS bool, payload []byte) {
	if entry == nil || len(payload) == 0 {
		return
	}
	entry.bytes += uint64(len(payload))
	if entry.kind == StreamVideo {
		consumeMPEG2Captions(entry, payload, pts, hasPTS)
		parser := p.videoParsers[key]
		if parser == nil {
			parser = &mpeg2VideoParser{}
			p.videoParsers[key] = parser
		}
		parser.consume(payload)
		entry.videoFrameCount = parser.pictureCount
		if parser.sawSequence {
			entry.videoIsMPEG2 = true
			entry.videoIsH264 = false
			entry.format = "MPEG Video"
		}
		if entry.videoIsMPEG2 {
			consumeMPEG2HeaderBytes(entry, payload, hasPTS)
			consumeMPEG2FrameBytes(entry, payload)
		} else {
			consumeH264PS(entry, payload)
		}
	}
	if entry.kind == StreamAudio {
		if entry.format == "AC-3" {
			if p.quickAC3 && entry.hasAC3 && entry.audioFrames >= p.quickAC3Max {
				return
			}
			consumeAC3PS(entry, payload)
		} else if entry.format == "MPEG Audio" {
			consumeMPEGAudioPS(entry, payload)
			// Only attempt ADTS detection when we have not identified a valid MPEG audio stream.
			if entry.mpegAudioLayer == 0 {
				consumeADTSPS(entry, payload)
				if entry.audioProfile != "" {
					entry.format = "AAC"
				}
			}
		}
	}
}

// consumeDVDLPCMHeader decodes bit depth, sampling rate, and channel count from
// the first complete DVD LPCM private-stream header.
func consumeDVDLPCMHeader(entry *psStream, payload []byte) {
	if entry == nil || len(payload) < 7 || entry.hasAudioInfo {
		return
	}
	config := payload[5]
	bitDepths := [...]int{16, 20, 24, 0}
	rates := [...]float64{48000, 96000, 0, 0}
	entry.pcmBitDepth = bitDepths[(config>>6)&0x03]
	entry.audioRate = rates[(config>>4)&0x03]
	entry.audioChannels = uint64(config&0x07) + 1
	entry.hasAudioInfo = entry.pcmBitDepth > 0 && entry.audioRate > 0 && entry.audioChannels > 0
}

func findPESStart(data []byte, start int) int {
	if start < 0 {
		start = 0
	}
	if len(data) < 4 || start+4 > len(data) {
		return -1
	}
	limit := len(data) - 3
	for pos := start; pos < limit; {
		idx := bytes.IndexByte(data[pos:limit], 0x00)
		if idx < 0 {
			return -1
		}
		pos += idx
		if data[pos+1] == 0x00 && data[pos+2] == 0x01 && isPESStreamID(data[pos+3]) {
			return pos
		}
		pos++
	}
	return -1
}

// parseMPEGPSMenu parses one DVD menu VOB. At ParseSpeed 1, it reads the full
// file. Below ParseSpeed 1, the timing comes from the first 4.2 MB. A second
// pass over the head and the tail adds the AC-3 statistics. It also adds the
// text and menu streams that only the second pass finds.
func parseMPEGPSMenu(file io.ReaderAt, size int64, opts mpegPSOptions) (ContainerInfo, []Stream, bool) {
	if size <= 0 {
		return ContainerInfo{}, nil, false
	}
	parseRange := func(parser *psStreamParser, start, length int64) bool {
		return parser.parseReader(bufio.NewReaderSize(io.NewSectionReader(file, start, length), 1<<20))
	}
	if opts.parseSpeed == 0 || opts.parseSpeed >= 1 {
		parser := newPSStreamParser(opts)
		if !parseRange(parser, 0, size) {
			return ContainerInfo{}, nil, false
		}
		opts.sampledBytes = size
		return finalizeMPEGPS(parser.streams, parser.streamOrder, parser.videoParsers, parser.videoPTS, parser.anyPTS, size, opts)
	}

	timingWindow := min(int64(4_200_000), size)
	parser := newPSStreamParser(opts)
	if !parseRange(parser, 0, timingWindow) {
		return ContainerInfo{}, nil, false
	}
	window := min(mpegPSBoundedWindow(file, size), size)
	statsOpts := opts
	statsOpts.dvdExtras = true
	statsParser := newPSStreamParser(statsOpts)
	statsParsed := parseRange(statsParser, 0, window)
	if tailStart := max(size-window, window); tailStart < size {
		statsParser.beginSection()
		statsParsed = parseRange(statsParser, tailStart, size-tailStart) || statsParsed
	}
	if statsParsed {
		copyMPEGPSAC3Stats(parser, statsParser)
		copyMPEGPSMissingStreams(parser, statsParser)
	}
	opts.sampled = size > timingWindow
	opts.sampledBytes = timingWindow
	return finalizeMPEGPS(parser.streams, parser.streamOrder, parser.videoParsers, parser.videoPTS, parser.anyPTS, size, opts)
}

// copyMPEGPSAC3Stats replaces matching destination AC-3 statistics with those
// collected by a wider source parser pass.
func copyMPEGPSAC3Stats(dst, src *psStreamParser) {
	for key, srcStream := range src.streams {
		dstStream := dst.streams[key]
		if dstStream == nil || !srcStream.hasAC3 || !dstStream.hasAC3 {
			continue
		}
		dstStream.ac3Info.dialnormSum = srcStream.ac3Info.dialnormSum
		dstStream.ac3Info.dialnormCount = srcStream.ac3Info.dialnormCount
		dstStream.ac3Info.dialnormMin = srcStream.ac3Info.dialnormMin
		dstStream.ac3Info.dialnormMax = srcStream.ac3Info.dialnormMax
		dstStream.ac3Info.hasDialnorm = srcStream.ac3Info.hasDialnorm
		dstStream.ac3Info.comprs = append([]uint32(nil), srcStream.ac3Info.comprs...)
		dstStream.ac3Info.dynrngs = append([]uint32(nil), srcStream.ac3Info.dynrngs...)
		dstStream.ac3Info.dynrngeSeen = srcStream.ac3Info.dynrngeSeen
	}
}

// copyMPEGPSMissingStreams adds text and menu streams observed only by the
// source parser while preserving source discovery order.
func copyMPEGPSMissingStreams(dst, src *psStreamParser) {
	for key, stream := range src.streams {
		if stream == nil || (stream.kind != StreamText && stream.kind != StreamMenu) || dst.streams[key] != nil {
			continue
		}
		dst.streams[key] = stream
		dst.streamOrder = append(dst.streamOrder, key)
	}
}

// appendTerminalBytes retains only the bounded tail needed to detect a final
// MPEG audio sync frame across parser input boundaries.
func (p *psStreamParser) appendTerminalBytes(data []byte) {
	if !p.afterProgramEnd || len(data) == 0 {
		return
	}
	for _, entry := range p.streams {
		remaining := mpegPSTerminalTailMax - len(entry.terminalBytes)
		if remaining <= 0 {
			continue
		}
		entry.terminalBytes = append(entry.terminalBytes, data[:min(len(data), remaining)]...)
	}
}
