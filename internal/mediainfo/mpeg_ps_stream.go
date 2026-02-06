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
			if pos+14 > len(buf) {
				if !readMore() {
					return found
				}
				continue
			}
			stuffing := int(buf[pos+13] & 0x07)
			needed := pos + 14 + stuffing
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
		}

		if pos+9 > len(buf) {
			if !readMore() {
				return found
			}
			continue
		}
		pesLen := int(binary.BigEndian.Uint16(buf[pos+4 : pos+6]))
		if (buf[pos+6] & 0xC0) != 0x80 {
			pos++
			continue
		}
		flags := buf[pos+7]
		headerLen := int(buf[pos+8])
		payloadStart := pos + 9 + headerLen
		if payloadStart > len(buf) {
			if !readMore() {
				return found
			}
			continue
		}

		payloadLen := 0
		if pesLen > 0 {
			payloadLen = max(pesLen-3-headerLen, 0)
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
				var currentPTS uint64
				var hasPTS bool
				if (flags&0x80) != 0 && pos+9+headerLen <= len(buf) {
					if pts, ok := parsePTS(buf[pos+9:]); ok {
						currentPTS = pts
						hasPTS = true
						p.anyPTS.add(pts)
						entry.pts.add(pts)
						if entry.kind == StreamVideo {
							p.videoPTS.add(pts)
						}
					}
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
		var streamPTS uint64
		var streamHasPTS bool
		if (flags&0x80) != 0 && pos+9+headerLen <= len(buf) {
			if pts, ok := parsePTS(buf[pos+9:]); ok {
				streamPTS = pts
				streamHasPTS = true
				p.anyPTS.add(pts)
				entry.pts.add(pts)
				if entry.kind == StreamVideo {
					p.videoPTS.add(pts)
				}
			}
		}
		pending = &psPending{
			entry:      entry,
			key:        psStreamKey(streamID, subID),
			flags:      flags,
			pts:        streamPTS,
			hasPTS:     streamHasPTS,
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
		consumeMPEG2StartCodeStats(entry, payload, (flags&0x80) != 0)
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
		} else {
			consumeADTSPS(entry, payload)
			if entry.hasAudioInfo && entry.format == "MPEG Audio" {
				entry.format = "AAC"
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
	return finalizeMPEGPS(parser.streams, parser.streamOrder, parser.videoParsers, parser.videoPTS, parser.anyPTS, size, opts)
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
	if opts.dvdParsing && sampleSize < 16<<20 {
		sampleSize = 16 << 20
	}
	if size <= sampleSize {
		return reader(file)
	}

	parsedAny := false
	first := io.NewSectionReader(file, 0, sampleSize)
	if reader(first) {
		parsedAny = true
	}
	if size > sampleSize*2 {
		if opts.dvdParsing {
			mid := (size - sampleSize) / 2
			middle := io.NewSectionReader(file, mid, sampleSize)
			if reader(middle) {
				parsedAny = true
			}
		}
		start := size - sampleSize
		last := io.NewSectionReader(file, start, sampleSize)
		if reader(last) {
			parsedAny = true
		}
	}
	return parsedAny
}
