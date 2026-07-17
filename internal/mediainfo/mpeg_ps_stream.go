package mediainfo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"os"
)

type psStreamParser struct {
	streams      map[uint16]*psStream
	streamOrder  []uint16
	videoParsers map[uint16]*mpeg2VideoParser
	videoPTS     ptsTracker
	anyPTS       ptsTracker
	packetOrder  int
	quickAC3     bool
	quickAC3Max  uint64
	sampled      bool
	section      int
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

func newPSStreamParser(opts mpegPSOptions) *psStreamParser {
	parseSpeed := opts.parseSpeed
	if parseSpeed == 0 {
		parseSpeed = 1
	}
	return &psStreamParser{
		streams:      map[uint16]*psStream{},
		streamOrder:  []uint16{},
		videoParsers: map[uint16]*mpeg2VideoParser{},
		quickAC3:     parseSpeed < 1 && !opts.dvdExtras,
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
				return found
			}
			if len(buf) > 2 {
				buf = append(buf[:0], buf[len(buf)-2:]...)
				pos = 0
			}
			if !readMore() {
				return found
			}
			continue
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
			}
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
			for {
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
				}
			}
			kind, format := mapPSStream(streamID, subID)
			if kind != "" {
				entry := p.ensureStream(streamID, subID, kind, format)
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
			if pos <= payloadStart {
				pos = payloadStart + 1
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
			}
		}
		kind, format := mapPSStream(streamID, subID)
		if kind == "" {
			pos = payloadStart
			continue
		}
		entry := p.ensureStream(streamID, subID, kind, format)
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

func ParseMPEGPSFiles(paths []string, size int64, opts mpegPSOptions) (ContainerInfo, []Stream, bool) {
	if len(paths) == 0 {
		return ContainerInfo{}, nil, false
	}
	parser := newPSStreamParser(opts)
	parsedAny := false
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return ContainerInfo{}, nil, false
		}
		if parseMPEGPSFileSample(parser, file, opts) {
			parsedAny = true
		}
		_ = file.Close()
	}
	if !parsedAny {
		return ContainerInfo{}, nil, false
	}
	opts2 := opts
	opts2.sampled = parser.sampled
	return finalizeMPEGPS(parser.streams, parser.streamOrder, parser.videoParsers, parser.videoPTS, parser.anyPTS, size, opts2)
}

func parseMPEGPSFileSample(parser *psStreamParser, file *os.File, opts mpegPSOptions) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	size := info.Size()
	if size <= 0 {
		return false
	}
	reader := func(r io.Reader) bool {
		buf := bufio.NewReaderSize(r, 1<<20)
		return parser.parseReader(buf)
	}

	parseSpeed := opts.parseSpeed
	if parseSpeed == 0 {
		parseSpeed = 1
	}
	if parseSpeed >= 1 {
		return reader(file)
	}

	sampleSize := int64(8 << 20)
	if parseSpeed > 0 && parseSpeed < 1 {
		sampleSize = max(int64(float64(sampleSize)*parseSpeed), 4<<20)
	}
	if opts.dvdParsing && sampleSize < 8<<20 {
		sampleSize = 8 << 20
	}
	if size <= sampleSize {
		return reader(file)
	}

	parsedAny := false
	parser.sampled = true
	first := io.NewSectionReader(file, 0, sampleSize)
	if reader(first) {
		parsedAny = true
	}
	if size > sampleSize*2 {
		tailSample := sampleSize
		if opts.dvdParsing && parseSpeed < 1 {
			tailSample = min(tailSample, int64(8<<20))
		}
		start := size - tailSample
		last := io.NewSectionReader(file, start, tailSample)
		if reader(last) {
			parsedAny = true
		}
	}
	return parsedAny
}
