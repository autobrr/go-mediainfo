package mediainfo

import (
	"math"
	"strings"
)

type ac3Info struct {
	bitRateKbps int64
	sampleRate  float64
	channels    uint64
	layout      string
	bsid        int
	bsmod       int
	acmod       int
	lfeon       int
	dsurmod     int
	hasDsurmod  bool
	serviceKind string
	frameRate   float64
	spf         int

	// Frame-scoped raw codes, used for MediaInfo-style stats (histogram-based).
	// When aggregating, these fields come from the merged frame, not the accumulator.
	dialnormCode uint8
	compre       bool
	comprCode    uint8
	dynrnge      bool
	dynrngCode   uint8
	dynrngParsed bool

	framesMerged int

	dialnorm      int
	dialnormSum   float64
	dialnormCount int
	dialnormMin   int
	dialnormMax   int
	hasDialnorm   bool
	comprDB       float64
	comprCount    int
	comprSum      float64
	comprSumDB    float64
	comprMin      float64
	comprMax      float64
	hasCompr      bool
	comprIsDB     bool
	comprFieldDB  float64
	hasComprField bool
	dynrngDB      float64
	hasDynrng     bool
	dynrngCount   int
	dynrngSum     float64
	dynrngMin     float64
	dynrngMax     float64
	dynrngeSeen   bool // "ever seen" (MediaInfo dynrnge_Exists)

	// MediaInfo-style stats histograms. Nil until first merge.
	comprs           []uint32
	dynrngs          []uint32
	cmixlevDB        float64
	hasCmixlev       bool
	surmixlevDB      float64
	hasSurmixlev     bool
	mixlevel         int
	hasMixlevel      bool
	roomtyp          string
	hasRoomtyp       bool
	dmixmod          string
	hasDmixmod       bool
	ltrtcmixlevDB    float64
	hasLtrtcmixlev   bool
	ltrtsurmixlevDB  float64
	hasLtrtsurmixlev bool
	lorocmixlevDB    float64
	hasLorocmixlev   bool
	lorosurmixlevDB  float64
	hasLorosurmixlev bool
	hasJOC           bool
	hasJOCComplex    bool
	jocComplexity    int
	jocObjects       int
	hasJOCDyn        bool
	jocDynObjects    int
	hasJOCBed        bool
	jocBedCount      uint64
	jocBedLayout     string

	eac3FrameType         int
	eac3ChannelMap        uint16
	hasEAC3ChannelMap     bool
	eac3ChannelMapLayout  string
	eac3ChannelMapChannel uint64
}

type ac3BitReader struct {
	data   []byte
	bitPos int
	limit  int
}

func (br *ac3BitReader) maxBits() int {
	if br.limit > 0 {
		return br.limit
	}
	return len(br.data) * 8
}

func (br *ac3BitReader) readBits(n int) (uint32, bool) {
	if n <= 0 || br.bitPos+n > br.maxBits() {
		return 0, false
	}
	var value uint32
	for range n {
		byteVal := br.data[br.bitPos>>3]
		bit := (byteVal >> (7 - (br.bitPos & 7))) & 0x01
		value = (value << 1) | uint32(bit)
		br.bitPos++
	}
	return value, true
}

func (br *ac3BitReader) skipBits(n int) bool {
	if n <= 0 {
		return true
	}
	if br.bitPos+n > br.maxBits() {
		return false
	}
	br.bitPos += n
	return true
}

func (br *ac3BitReader) remaining() int {
	return br.maxBits() - br.bitPos
}

func (br *ac3BitReader) alignToByte() bool {
	rem := br.bitPos & 7
	if rem == 0 {
		return true
	}
	return br.skipBits(8 - rem)
}

func (br *ac3BitReader) readVariableBits(bits int) (uint32, bool) {
	if bits <= 0 {
		return 0, false
	}
	var value uint32
	for {
		part, ok := br.readBits(bits)
		if !ok {
			return 0, false
		}
		value += part
		cont, ok := br.readBits(1)
		if !ok {
			return 0, false
		}
		if cont == 0 {
			break
		}
		value <<= bits
		value += 1 << bits
	}
	return value, true
}

func parseAC3Frame(payload []byte) (ac3Info, int, bool) {
	var info ac3Info
	if len(payload) < 7 {
		return info, 0, false
	}
	br := ac3BitReader{data: payload}
	if sync, ok := br.readBits(16); !ok || sync != 0x0B77 {
		return info, 0, false
	}
	if _, ok := br.readBits(16); !ok { // crc1
		return info, 0, false
	}
	fscod, ok := br.readBits(2)
	if !ok {
		return info, 0, false
	}
	frmsizecod, ok := br.readBits(6)
	if !ok {
		return info, 0, false
	}
	frameSize := ac3FrameSizeBytes(int(fscod), int(frmsizecod))
	if frameSize == 0 {
		return info, 0, false
	}
	// Don't let bit parsing run past the frame boundary. MediaInfoLib parses within the syncframe;
	// reading beyond can smear values across frame boundaries and skew TS/BDAV stats.
	if len(payload) >= frameSize {
		br.limit = frameSize * 8
	}
	bsid, ok := br.readBits(5)
	if !ok {
		return info, 0, false
	}
	// Core AC-3 bitstream_id is 0..10.
	// Rejecting out-of-range values avoids false-positive sync matches when scanning TS payloads.
	if bsid > 10 {
		return info, 0, false
	}
	// fscod==3 is invalid for legacy AC-3 (bsid<=8). MediaInfo rejects these frames.
	if bsid <= 8 && fscod == 3 {
		return info, 0, false
	}
	bsmod, ok := br.readBits(3)
	if !ok {
		return info, 0, false
	}
	acmod, ok := br.readBits(3)
	if !ok {
		return info, 0, false
	}
	if acmod != 0 {
		// acmod==1 is mono (C); it does not carry cmixlev. Reading it would shift the bitstream
		// and corrupt downstream fields (e.g. lfeon/dialnorm/compr), which breaks TS parity.
		if acmod&1 != 0 && acmod != 1 {
			cmixlev, ok := br.readBits(2)
			if !ok {
				return info, 0, false
			}
			if value, ok := ac3CenterMixLevelDB(cmixlev); ok {
				info.cmixlevDB = value
				info.hasCmixlev = true
			}
		}
		if acmod&4 != 0 {
			surmixlev, ok := br.readBits(2)
			if !ok {
				return info, 0, false
			}
			if value, ok := ac3SurroundMixLevelDB(surmixlev); ok {
				info.surmixlevDB = value
				info.hasSurmixlev = true
			}
		}
	}
	if acmod == 2 {
		dsurmod, ok := br.readBits(2)
		if !ok {
			return info, 0, false
		}
		info.dsurmod = int(dsurmod)
		info.hasDsurmod = true
	}
	lfeonVal, ok := br.readBits(1)
	if !ok {
		return info, 0, false
	}
	dialnorm, ok := br.readBits(5)
	if !ok {
		return info, 0, false
	}
	info.hasDialnorm = true
	info.dialnorm = ac3DialnormDB(dialnorm)
	info.dialnormCode = uint8(dialnorm)
	info.dialnormCount = 1
	info.dialnormSum = math.Pow(10.0, float64(info.dialnorm)/10.0)
	info.dialnormMin = info.dialnorm
	info.dialnormMax = info.dialnorm
	if acmod == 0 {
		// Dual-mono (1+1): dialnorm2 is present before compre/compr.
		if _, ok := br.readBits(5); !ok {
			return info, 0, false
		}
	}
	compre, ok := br.readBits(1)
	if !ok {
		return info, 0, false
	}
	if compre == 1 {
		compr, ok := br.readBits(8)
		if !ok {
			return info, 0, false
		}
		info.compre = true
		info.comprCode = uint8(compr)
		info.comprFieldDB = ac3ComprDB(uint8(compr))
		info.hasComprField = true
		info.hasCompr = true
		info.comprDB = info.comprFieldDB
	}
	if acmod == 0 {
		// Dual-mono: optional compr2 follows. Use it for stats if compr1 is absent.
		compr2e, ok := br.readBits(1)
		if !ok {
			return info, 0, false
		}
		if compr2e == 1 {
			compr2, ok := br.readBits(8)
			if !ok {
				return info, 0, false
			}
			if !info.compre {
				info.compre = true
				info.comprCode = uint8(compr2)
				info.comprFieldDB = ac3ComprDB(uint8(compr2))
				info.hasComprField = true
				info.hasCompr = true
				info.comprDB = info.comprFieldDB
			}
		}
	}
	langcode, ok := br.readBits(1)
	if !ok {
		return info, 0, false
	}
	if langcode == 1 {
		if _, ok = br.readBits(8); !ok {
			return info, 0, false
		}
	}
	if acmod == 0 {
		// Dual-mono: optional language code for channel 2.
		langcode2, ok := br.readBits(1)
		if !ok {
			return info, 0, false
		}
		if langcode2 == 1 {
			if _, ok = br.readBits(8); !ok {
				return info, 0, false
			}
		}
	}
	audprodie, ok := br.readBits(1)
	if !ok {
		return info, 0, false
	}
	if audprodie == 1 {
		mixlevel, ok := br.readBits(5)
		if !ok {
			return info, 0, false
		}
		roomtyp, ok := br.readBits(2)
		if !ok {
			return info, 0, false
		}
		info.mixlevel = int(mixlevel) + 80
		info.hasMixlevel = true
		if value, ok := ac3RoomType(roomtyp); ok {
			info.roomtyp = value
			info.hasRoomtyp = true
		}
	}
	if acmod == 0 {
		// Dual-mono: optional audio production info for channel 2.
		audprodi2e, ok := br.readBits(1)
		if !ok {
			return info, 0, false
		}
		if audprodi2e == 1 {
			if _, ok := br.readBits(5); !ok {
				return info, 0, false
			}
			if _, ok := br.readBits(2); !ok {
				return info, 0, false
			}
		}
	}
	if _, ok := br.readBits(1); !ok { // copyrightb
		return info, 0, false
	}
	if _, ok := br.readBits(1); !ok { // origbs
		return info, 0, false
	}
	if bsid == 6 {
		// bsid==0x06 repurposes the timecode bits for Dolby extensions (xbsi1/xbsi2).
		// Match MediaInfoLib File_Ac3.cpp bit layout to keep subsequent parsing aligned.
		xbsi1e, ok := br.readBits(1)
		if !ok {
			return info, 0, false
		}
		if xbsi1e == 1 {
			dmixmod, ok := br.readBits(2)
			if !ok {
				return info, 0, false
			}
			if value := ac3PreferredDownmix(dmixmod); value != "" {
				info.dmixmod = value
				info.hasDmixmod = true
			}
			ltrtcmixlev, ok := br.readBits(3)
			if !ok {
				return info, 0, false
			}
			if value, ok := ac3ExtendedMixLevelDB(ltrtcmixlev); ok {
				info.ltrtcmixlevDB = value
				info.hasLtrtcmixlev = true
			}
			ltrtsurmixlev, ok := br.readBits(3)
			if !ok {
				return info, 0, false
			}
			if value, ok := ac3ExtendedMixLevelDB(ltrtsurmixlev); ok {
				info.ltrtsurmixlevDB = value
				info.hasLtrtsurmixlev = true
			}
			lorocmixlev, ok := br.readBits(3)
			if !ok {
				return info, 0, false
			}
			if value, ok := ac3ExtendedMixLevelDB(lorocmixlev); ok {
				info.lorocmixlevDB = value
				info.hasLorocmixlev = true
			}
			lorosurmixlev, ok := br.readBits(3)
			if !ok {
				return info, 0, false
			}
			if value, ok := ac3ExtendedMixLevelDB(lorosurmixlev); ok {
				info.lorosurmixlevDB = value
				info.hasLorosurmixlev = true
			}
		}
		xbsi2e, ok := br.readBits(1)
		if !ok {
			return info, 0, false
		}
		if xbsi2e == 1 {
			// dsurexmod (2) + dheadphonmod (2) + adconvtyp (1) + xbsi2 (8) + encinfo (1)
			if _, ok := br.readBits(14); !ok {
				return info, 0, false
			}
		}
	} else {
		timecod1e, ok := br.readBits(1)
		if !ok {
			return info, 0, false
		}
		if timecod1e == 1 {
			if _, ok := br.readBits(14); !ok {
				return info, 0, false
			}
		}
		timecod2e, ok := br.readBits(1)
		if !ok {
			return info, 0, false
		}
		if timecod2e == 1 {
			if _, ok := br.readBits(14); !ok {
				return info, 0, false
			}
		}
	}
	addbsie, ok := br.readBits(1)
	if !ok {
		return info, 0, false
	}
	if addbsie == 1 {
		addbsil, ok := br.readBits(6)
		if !ok {
			return info, 0, false
		}
		for i := 0; i < int(addbsil)+1; i++ {
			if _, ok := br.readBits(8); !ok {
				return info, 0, false
			}
		}
	}
	if dynrnge, code, ok := parseAC3Dynrng(&br, int(acmod)); ok {
		info.dynrngParsed = true
		info.dynrnge = dynrnge
		if dynrnge {
			info.dynrngCode = code
			info.dynrngDB = ac3DynrngDB(code)
			info.hasDynrng = true
		} else {
			info.dynrngCode = 0
		}
	}

	sampleRate := ac3SampleRate(int(fscod))
	bitRate := ac3BitrateKbps(int(frmsizecod))
	channels, layout := ac3ChannelLayout(int(acmod), lfeonVal == 1)
	frameRate := 0.0
	spf := 1536
	if sampleRate > 0 {
		frameRate = sampleRate / float64(spf)
	}

	info = ac3Info{
		bitRateKbps:      bitRate,
		sampleRate:       sampleRate,
		channels:         channels,
		layout:           layout,
		bsid:             int(bsid),
		bsmod:            int(bsmod),
		acmod:            int(acmod),
		lfeon:            int(lfeonVal),
		dsurmod:          info.dsurmod,
		hasDsurmod:       info.hasDsurmod,
		serviceKind:      ac3ServiceKind(int(bsmod)),
		frameRate:        frameRate,
		spf:              spf,
		dialnorm:         info.dialnorm,
		dialnormCode:     info.dialnormCode,
		dialnormSum:      info.dialnormSum,
		dialnormCount:    info.dialnormCount,
		dialnormMin:      info.dialnormMin,
		dialnormMax:      info.dialnormMax,
		hasDialnorm:      info.hasDialnorm,
		comprDB:          info.comprDB,
		compre:           info.compre,
		comprCode:        info.comprCode,
		comprCount:       info.comprCount,
		comprSum:         info.comprSum,
		comprSumDB:       info.comprSumDB,
		comprMin:         info.comprMin,
		comprMax:         info.comprMax,
		comprIsDB:        info.comprIsDB,
		comprFieldDB:     info.comprFieldDB,
		hasCompr:         info.hasCompr,
		hasComprField:    info.hasComprField,
		dynrngDB:         info.dynrngDB,
		hasDynrng:        info.hasDynrng,
		dynrnge:          info.dynrnge,
		dynrngCode:       info.dynrngCode,
		dynrngParsed:     info.dynrngParsed,
		dynrngSum:        info.dynrngSum,
		dynrngCount:      info.dynrngCount,
		dynrngMin:        info.dynrngMin,
		dynrngMax:        info.dynrngMax,
		dynrngeSeen:      info.dynrngeSeen,
		cmixlevDB:        info.cmixlevDB,
		hasCmixlev:       info.hasCmixlev,
		surmixlevDB:      info.surmixlevDB,
		hasSurmixlev:     info.hasSurmixlev,
		mixlevel:         info.mixlevel,
		hasMixlevel:      info.hasMixlevel,
		roomtyp:          info.roomtyp,
		hasRoomtyp:       info.hasRoomtyp,
		dmixmod:          info.dmixmod,
		hasDmixmod:       info.hasDmixmod,
		ltrtcmixlevDB:    info.ltrtcmixlevDB,
		hasLtrtcmixlev:   info.hasLtrtcmixlev,
		ltrtsurmixlevDB:  info.ltrtsurmixlevDB,
		hasLtrtsurmixlev: info.hasLtrtsurmixlev,
		lorocmixlevDB:    info.lorocmixlevDB,
		hasLorocmixlev:   info.hasLorocmixlev,
		lorosurmixlevDB:  info.lorosurmixlevDB,
		hasLorosurmixlev: info.hasLorosurmixlev,
	}
	return info, frameSize, true
}

func parseEAC3Frame(payload []byte) (ac3Info, int, bool) {
	return parseEAC3FrameWithOptions(payload, true)
}

func parseEAC3FrameWithOptions(payload []byte, parseJOC bool) (ac3Info, int, bool) {
	var info ac3Info
	if len(payload) < 7 {
		return info, 0, false
	}
	br := ac3BitReader{data: payload}
	if sync, ok := br.readBits(16); !ok || sync != 0x0B77 {
		return info, 0, false
	}
	strmtyp, ok := br.readBits(2) // strmtyp
	if !ok {
		return info, 0, false
	}
	if _, ok := br.readBits(3); !ok { // substreamid
		return info, 0, false
	}
	frmsiz, ok := br.readBits(11)
	if !ok {
		return info, 0, false
	}
	frameSize := int((frmsiz + 1) * 2)
	// Bound bit parsing to this syncframe when we have enough bytes buffered.
	if len(payload) >= frameSize {
		br.limit = frameSize * 8
	}
	fscod, ok := br.readBits(2)
	if !ok {
		return info, 0, false
	}
	fscod2 := uint32(0)
	numblkscod := uint32(0)
	if fscod == 3 {
		val, ok := br.readBits(2)
		if !ok {
			return info, 0, false
		}
		fscod2 = val
		numblkscod = 3
	} else {
		val, ok := br.readBits(2)
		if !ok {
			return info, 0, false
		}
		numblkscod = val
	}
	acmod, ok := br.readBits(3)
	if !ok {
		return info, 0, false
	}
	lfeonVal, ok := br.readBits(1)
	if !ok {
		return info, 0, false
	}
	bsid, ok := br.readBits(5)
	if !ok {
		return info, 0, false
	}
	// Basic sanity: E-AC-3 bitstream_id is typically >= 10 (and often 16). Rejecting clearly
	// invalid values reduces false-positive sync matches when scanning concatenated frames.
	if bsid < 10 {
		return info, 0, false
	}
	if strmtyp == 0 {
		dialnorm, ok := br.readBits(5)
		if !ok {
			return info, 0, false
		}
		info.hasDialnorm = true
		info.dialnorm = ac3DialnormDB(dialnorm)
		info.dialnormCode = uint8(dialnorm)
		info.dialnormCount = 1
		info.dialnormSum = math.Pow(10.0, float64(info.dialnorm)/10.0)
		info.dialnormMin = info.dialnorm
		info.dialnormMax = info.dialnorm

		compre, ok := br.readBits(1)
		if !ok {
			return info, 0, false
		}
		if compre == 1 {
			compr, ok := br.readBits(8)
			if !ok {
				return info, 0, false
			}
			info.compre = true
			info.comprCode = uint8(compr)
			info.comprDB = ac3ComprDB(uint8(compr))
			info.hasCompr = true
		}
	} else if strmtyp == 1 {
		for i := 0; i < eac3DialnormFieldCount(int(acmod)); i++ {
			if _, ok := br.readBits(5); !ok { // dialnorm
				return info, 0, false
			}
			compre, ok := br.readBits(1)
			if !ok {
				return info, 0, false
			}
			if compre == 1 {
				if _, ok := br.readBits(8); !ok { // compr
					return info, 0, false
				}
			}
		}
		chanmape, ok := br.readBits(1)
		if !ok {
			return info, 0, false
		}
		if chanmape == 1 {
			chanmap, ok := br.readBits(16)
			if !ok {
				return info, 0, false
			}
			if layout, count := eac3ChannelMapLayout(uint16(chanmap)); layout != "" {
				info.hasEAC3ChannelMap = true
				info.eac3ChannelMap = uint16(chanmap)
				info.eac3ChannelMapLayout = layout
				info.eac3ChannelMapChannel = count
			}
		}
	}
	if parseEAC3MetadataExtension(&br, &info, int(strmtyp), int(numblkscod), int(acmod), lfeonVal == 1, int(fscod)) {
		// Extension type A is Dolby's JOC/Atmos signal in E-AC-3 additional bitstream info.
		info.hasJOC = true
		info.hasJOCComplex = true
	}

	sampleRate := eac3SampleRate(int(fscod), int(fscod2))
	spf := eac3SamplesPerFrame(int(numblkscod))
	frameRate := 0.0
	if sampleRate > 0 && spf > 0 {
		frameRate = sampleRate / float64(spf)
	}
	bitRate := int64(0)
	if sampleRate > 0 && spf > 0 && frameSize > 0 {
		bitRate = int64(math.Round(float64(frameSize*8) * sampleRate / (float64(spf) * 1000.0)))
	}
	channels, layout := ac3ChannelLayout(int(acmod), lfeonVal == 1)
	var jocMeta eac3JOCMeta
	if parseJOC {
		jocPayload := payload
		if frameSize > 0 && frameSize < len(jocPayload) {
			jocPayload = jocPayload[:frameSize]
		}
		if meta, ok := parseEAC3EMDF(jocPayload); ok {
			jocMeta = meta
		}
	}
	info = ac3Info{
		bitRateKbps:   bitRate,
		sampleRate:    sampleRate,
		channels:      channels,
		layout:        layout,
		bsid:          int(bsid),
		bsmod:         info.bsmod,
		acmod:         int(acmod),
		lfeon:         int(lfeonVal),
		serviceKind:   info.serviceKind,
		frameRate:     frameRate,
		spf:           spf,
		dialnorm:      info.dialnorm,
		dialnormCode:  info.dialnormCode,
		dialnormSum:   info.dialnormSum,
		dialnormCount: info.dialnormCount,
		dialnormMin:   info.dialnormMin,
		dialnormMax:   info.dialnormMax,
		hasDialnorm:   info.hasDialnorm,
		comprDB:       info.comprDB,
		compre:        info.compre,
		comprCode:     info.comprCode,
		comprCount:    info.comprCount,
		comprSum:      info.comprSum,
		comprMin:      info.comprMin,
		comprMax:      info.comprMax,
		dynrngDB:      info.dynrngDB,
		hasDynrng:     info.hasDynrng,
		hasCompr:      info.hasCompr,
		hasJOC:        info.hasJOC || jocMeta.hasJOC,
		hasJOCComplex: info.hasJOCComplex || jocMeta.hasJOCComplex,
		jocComplexity: firstNonZero(info.jocComplexity, jocMeta.jocComplexity),
		jocObjects:    jocMeta.jocObjects,
		hasJOCDyn:     jocMeta.hasJOCDyn,
		jocDynObjects: jocMeta.jocDynObjects,
		hasJOCBed:     jocMeta.hasJOCBed,
		jocBedCount:   jocMeta.jocBedCount,
		jocBedLayout:  jocMeta.jocBedLayout,

		eac3FrameType:         int(strmtyp),
		eac3ChannelMap:        info.eac3ChannelMap,
		hasEAC3ChannelMap:     info.hasEAC3ChannelMap,
		eac3ChannelMapLayout:  info.eac3ChannelMapLayout,
		eac3ChannelMapChannel: info.eac3ChannelMapChannel,
	}
	return info, frameSize, true
}

// parseEAC3MetadataExtension advances over E-AC-3 metadata fields through addbsi
// and records Dolby extension type A, which signals JOC/Atmos plus its complexity
// index. It returns false when the extension is absent or the bounded frame data
// ends before the field can be read.
func parseEAC3MetadataExtension(br *ac3BitReader, info *ac3Info, strmtyp int, numblkscod int, acmod int, lfeon bool, fscod int) bool {
	if br == nil || info == nil {
		return false
	}
	if mixmdate, ok := br.readBits(1); !ok {
		return false
	} else if mixmdate == 1 {
		if acmod > 2 {
			if !br.skipBits(2) {
				return false
			}
			if acmod&1 != 0 {
				if !br.skipBits(6) {
					return false
				}
			}
			if acmod&4 != 0 {
				if !br.skipBits(6) {
					return false
				}
			}
		}
		if lfeon {
			lfeMixLevelExists, ok := br.readBits(1)
			if !ok {
				return false
			}
			if lfeMixLevelExists == 1 && !br.skipBits(5) {
				return false
			}
		}
		if strmtyp == 0 {
			for i := 0; i < eac3DialnormFieldCount(acmod); i++ {
				programScaleExists, ok := br.readBits(1)
				if !ok {
					return false
				}
				if programScaleExists == 1 && !br.skipBits(6) {
					return false
				}
			}
			externalScaleExists, ok := br.readBits(1)
			if !ok {
				return false
			}
			if externalScaleExists == 1 && !br.skipBits(6) {
				return false
			}
			mixData, ok := br.readBits(2)
			if !ok {
				return false
			}
			switch mixData {
			case 1:
				if !br.skipBits(5) {
					return false
				}
			case 2:
				if !br.skipBits(12) {
					return false
				}
			case 3:
				size, ok := br.readBits(5)
				if !ok || !br.skipBits((int(size)+2)*8) {
					return false
				}
			}
			if acmod < 2 {
				for i := 0; i < eac3DialnormFieldCount(acmod); i++ {
					panInfoExists, ok := br.readBits(1)
					if !ok {
						return false
					}
					if panInfoExists == 1 && !br.skipBits(14) {
						return false
					}
				}
			}
			mixConfigExists, ok := br.readBits(1)
			if !ok {
				return false
			}
			if mixConfigExists == 1 {
				blocks := eac3BlockCount(numblkscod)
				for i := 0; i < blocks; i++ {
					if blocks == 1 {
						if !br.skipBits(5) {
							return false
						}
						continue
					}
					blockMixExists, ok := br.readBits(1)
					if !ok {
						return false
					}
					if blockMixExists == 1 && !br.skipBits(5) {
						return false
					}
				}
			}
		}
	}
	if infomdate, ok := br.readBits(1); !ok {
		return false
	} else if infomdate == 1 {
		bsmod, ok := br.readBits(3)
		if !ok || !br.skipBits(2) {
			return false
		}
		info.bsmod = int(bsmod)
		info.serviceKind = ac3ServiceKind(int(bsmod))
		if acmod == 2 && !br.skipBits(4) {
			return false
		}
		if acmod >= 6 && !br.skipBits(2) {
			return false
		}
		for i := 0; i < eac3DialnormFieldCount(acmod); i++ {
			audioProdInfoExists, ok := br.readBits(1)
			if !ok {
				return false
			}
			if audioProdInfoExists == 1 && !br.skipBits(8) {
				return false
			}
		}
		if fscod != 3 && !br.skipBits(1) {
			return false
		}
	}
	if strmtyp == 0 && numblkscod != 3 {
		if !br.skipBits(1) {
			return false
		}
	}
	if strmtyp == 2 && numblkscod == 3 {
		if !br.skipBits(6) {
			return false
		}
	} else if strmtyp == 2 {
		convertExists, ok := br.readBits(1)
		if !ok {
			return false
		}
		if convertExists == 1 && !br.skipBits(6) {
			return false
		}
	}
	addbsie, ok := br.readBits(1)
	if !ok || addbsie == 0 {
		return false
	}
	addbsil, ok := br.readBits(6)
	if !ok {
		return false
	}
	additionalBytes := int(addbsil) + 1
	for i := 0; i < additionalBytes; i++ {
		if i == 0 {
			if !br.skipBits(7) {
				return false
			}
			flag, ok := br.readBits(1)
			if !ok {
				return false
			}
			if flag == 1 {
				complexity, ok := br.readBits(8)
				if !ok {
					return false
				}
				info.jocComplexity = int(complexity)
				return true
			}
			continue
		}
		if !br.skipBits(8) {
			return false
		}
	}
	return false
}

// eac3BlockCount maps the E-AC-3 numblkscod field to audio blocks per frame.
func eac3BlockCount(numblkscod int) int {
	switch numblkscod {
	case 0:
		return 1
	case 1:
		return 2
	case 2:
		return 3
	case 3:
		return 6
	default:
		return 0
	}
}

// firstNonZero returns the first non-zero value in priority order.
func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

// eac3DialnormFieldCount returns how many dialog-normalization/compression field groups
// are present before dependent-stream channel mapping for the given E-AC-3 coding mode.
func eac3DialnormFieldCount(acmod int) int {
	if acmod == 0 {
		return 2
	}
	return 1
}

// eac3CustomChannelMapLayouts maps the 16 dependent E-AC-3 custom channel-map bits
// to MediaInfo channel labels in bitstream order.
var eac3CustomChannelMapLayouts = [][]string{
	{"L"},
	{"C"},
	{"R"},
	{"Ls"},
	{"Rs"},
	{"Lc", "Rc"},
	{"Lb", "Rb"},
	{"Cb"},
	{"Tc"},
	{"Lsd", "Rsd"},
	{"Lw", "Rw"},
	{"Tfl", "Tfr"},
	{"Tfc"},
	{"Tbl", "Tbr"},
	{"LFE2"},
	{"LFE"},
}

// eac3ChannelMapLayout converts a dependent E-AC-3 custom channel map to a stable
// MediaInfo-style channel layout and channel count.
func eac3ChannelMapLayout(mask uint16) (string, uint64) {
	seen := map[string]bool{}
	for i, layout := range eac3CustomChannelMapLayouts {
		if mask&(1<<uint(15-i)) == 0 {
			continue
		}
		for _, ch := range layout {
			seen[ch] = true
		}
	}
	return orderedChannelLayout(seen)
}

// channelLayoutOrder is the preferred output order for merged codec-derived channel labels.
var channelLayoutOrder = []string{
	"L", "R", "C", "LFE", "Ls", "Rs", "Lb", "Rb", "Lc", "Rc", "Lw", "Rw",
	"Cb", "Tc", "Lsd", "Rsd", "Tfl", "Tfr", "Tfc", "Tbl", "Tbr", "LFE2",
}

// orderedChannelLayout renders a set of channel labels and consumes known labels from seen.
func orderedChannelLayout(seen map[string]bool) (string, uint64) {
	if len(seen) == 0 {
		return "", 0
	}
	parts := make([]string, 0, len(seen))
	for _, ch := range channelLayoutOrder {
		if seen[ch] {
			parts = append(parts, ch)
			delete(seen, ch)
		}
	}
	for ch := range seen {
		parts = append(parts, ch)
	}
	return strings.Join(parts, " "), uint64(len(parts))
}

// mergeAudioChannelLayouts combines core and extension channel layouts without duplicating
// channels already present in the core stream.
func mergeAudioChannelLayouts(layouts ...string) (uint64, string) {
	seen := map[string]bool{}
	for _, layout := range layouts {
		for _, ch := range strings.Fields(layout) {
			seen[ch] = true
		}
	}
	layout, count := orderedChannelLayout(seen)
	return count, layout
}

func eac3SampleRate(fscod int, fscod2 int) float64 {
	if fscod == 3 {
		switch fscod2 {
		case 0:
			return 24000
		case 1:
			return 22050
		case 2:
			return 16000
		default:
			return 0
		}
	}
	return ac3SampleRate(fscod)
}

func eac3SamplesPerFrame(numblkscod int) int {
	switch numblkscod {
	case 0:
		return 256
	case 1:
		return 512
	case 2:
		return 768
	case 3:
		return 1536
	default:
		return 0
	}
}

func (info *ac3Info) mergeFrame(frame ac3Info) {
	if info.framesMerged == 0 {
		// Base fields are first-frame-only in MediaInfo.
		info.hasCompr = frame.compre
		if frame.compre {
			info.hasComprField = true
			info.comprFieldDB = frame.comprDB
			info.comprDB = frame.comprDB
		}
		info.hasDynrng = frame.dynrnge
		if frame.dynrnge {
			info.dynrngDB = frame.dynrngDB
		}
	}

	if frame.bitRateKbps > 0 && info.bitRateKbps == 0 {
		info.bitRateKbps = frame.bitRateKbps
	}
	if frame.sampleRate > 0 && info.sampleRate == 0 {
		info.sampleRate = frame.sampleRate
	}
	if frame.channels > 0 && info.channels == 0 {
		info.channels = frame.channels
	}
	if frame.layout != "" && info.layout == "" {
		info.layout = frame.layout
	}
	if frame.bsid > 0 && info.bsid == 0 {
		info.bsid = frame.bsid
	}
	if frame.bsmod > 0 && info.bsmod == 0 {
		info.bsmod = frame.bsmod
	}
	if frame.acmod > 0 && info.acmod == 0 {
		info.acmod = frame.acmod
	}
	if frame.hasDsurmod && !info.hasDsurmod {
		info.dsurmod = frame.dsurmod
		info.hasDsurmod = true
	}
	if frame.lfeon > 0 && info.lfeon == 0 {
		info.lfeon = frame.lfeon
	}
	if frame.serviceKind != "" && info.serviceKind == "" {
		info.serviceKind = frame.serviceKind
	}
	if frame.frameRate > 0 && info.frameRate == 0 {
		info.frameRate = frame.frameRate
	}
	if frame.spf > 0 && info.spf == 0 {
		info.spf = frame.spf
	}
	if frame.hasCmixlev && !info.hasCmixlev {
		info.cmixlevDB = frame.cmixlevDB
		info.hasCmixlev = true
	}
	if frame.hasSurmixlev && !info.hasSurmixlev {
		info.surmixlevDB = frame.surmixlevDB
		info.hasSurmixlev = true
	}

	// Stats: histogram-based to match MediaInfo.
	if info.comprs == nil {
		info.comprs = make([]uint32, 256)
	}
	if info.dynrngs == nil {
		info.dynrngs = make([]uint32, 256)
	}
	if frame.compre {
		// MediaInfoLib uses 0xFF as the "unset" initializer for compr. When compre is set but the
		// value is still 0xFF, it is effectively treated as not present for stats (but may still
		// be used for the single-value "compr" field).
		if frame.comprCode != 0xFF {
			info.comprs[frame.comprCode]++
		}
	}
	if frame.dynrnge {
		info.dynrngeSeen = true
	}
	// MediaInfoLib counts dynrng for every parsed frame, using 0 when dynrnge is absent.
	// The dynrng_* fields are only emitted if dynrnge has been seen at least once.
	if frame.dynrngParsed {
		// MediaInfoLib uses 0xFF as an "unset" initializer for dynrng. When dynrnge is set but the
		// value is still 0xFF, it is treated as not present for stats.
		if frame.dynrngCode != 0xFF {
			info.dynrngs[frame.dynrngCode]++
		}
	}
	if frame.hasMixlevel && !info.hasMixlevel {
		info.mixlevel = frame.mixlevel
		info.hasMixlevel = true
	}
	if frame.hasRoomtyp && !info.hasRoomtyp {
		info.roomtyp = frame.roomtyp
		info.hasRoomtyp = true
	}
	if frame.hasDialnorm {
		if info.dialnormCount == 0 {
			info.dialnorm = frame.dialnorm
			info.dialnormSum = frame.dialnormSum
			info.dialnormCount = frame.dialnormCount
			info.dialnormMin = frame.dialnormMin
			info.dialnormMax = frame.dialnormMax
			info.hasDialnorm = true
		} else {
			info.dialnormSum += frame.dialnormSum
			info.dialnormCount += frame.dialnormCount
			if frame.dialnormMin < info.dialnormMin {
				info.dialnormMin = frame.dialnormMin
			}
			if frame.dialnormMax > info.dialnormMax {
				info.dialnormMax = frame.dialnormMax
			}
		}
		info.hasDialnorm = true
	}
	if frame.hasJOC && !info.hasJOC {
		info.hasJOC = true
	}
	if frame.hasJOCComplex && !info.hasJOCComplex {
		info.hasJOCComplex = true
		info.jocComplexity = frame.jocComplexity
	}
	if frame.jocObjects > 0 && info.jocObjects == 0 {
		info.jocObjects = frame.jocObjects
	}
	if frame.hasJOCDyn && !info.hasJOCDyn {
		info.hasJOCDyn = true
		info.jocDynObjects = frame.jocDynObjects
	}
	if frame.hasJOCBed && !info.hasJOCBed {
		info.hasJOCBed = true
		info.jocBedCount = frame.jocBedCount
		info.jocBedLayout = frame.jocBedLayout
	}
	if frame.hasDmixmod && !info.hasDmixmod {
		info.dmixmod = frame.dmixmod
		info.hasDmixmod = true
	}
	if frame.hasLtrtcmixlev && !info.hasLtrtcmixlev {
		info.ltrtcmixlevDB = frame.ltrtcmixlevDB
		info.hasLtrtcmixlev = true
	}
	if frame.hasLtrtsurmixlev && !info.hasLtrtsurmixlev {
		info.ltrtsurmixlevDB = frame.ltrtsurmixlevDB
		info.hasLtrtsurmixlev = true
	}
	if frame.hasLorocmixlev && !info.hasLorocmixlev {
		info.lorocmixlevDB = frame.lorocmixlevDB
		info.hasLorocmixlev = true
	}
	if frame.hasLorosurmixlev && !info.hasLorosurmixlev {
		info.lorosurmixlevDB = frame.lorosurmixlevDB
		info.hasLorosurmixlev = true
	}
	if frame.hasEAC3ChannelMap && !info.hasEAC3ChannelMap {
		info.hasEAC3ChannelMap = true
		info.eac3ChannelMap = frame.eac3ChannelMap
		info.eac3ChannelMapLayout = frame.eac3ChannelMapLayout
		info.eac3ChannelMapChannel = frame.eac3ChannelMapChannel
	}

	info.framesMerged++
}

// mergeFrameBase updates only the first-frame metadata fields used for single-value JSON/text
// output (e.g. dialnorm/compr/dynrng), without accumulating histogram-based stats.
// This is used for TS/BDAV where MediaInfoLib may compute *_Average/*_Count from a bounded window.
func (info *ac3Info) mergeFrameBase(frame ac3Info) {
	if info.framesMerged == 0 {
		// Base fields are first-frame-only in MediaInfo.
		info.hasCompr = frame.compre
		if frame.compre {
			info.hasComprField = true
			info.comprFieldDB = frame.comprDB
			info.comprDB = frame.comprDB
		}
		info.hasDynrng = frame.dynrnge
		if frame.dynrnge {
			info.dynrngDB = frame.dynrngDB
		}
		if frame.hasDialnorm {
			info.dialnorm = frame.dialnorm
			info.hasDialnorm = true
		}
	}

	if frame.bitRateKbps > 0 && info.bitRateKbps == 0 {
		info.bitRateKbps = frame.bitRateKbps
	}
	if frame.sampleRate > 0 && info.sampleRate == 0 {
		info.sampleRate = frame.sampleRate
	}
	if frame.channels > 0 && info.channels == 0 {
		info.channels = frame.channels
	}
	if frame.layout != "" && info.layout == "" {
		info.layout = frame.layout
	}
	if frame.bsid > 0 && info.bsid == 0 {
		info.bsid = frame.bsid
	}
	if frame.bsmod > 0 && info.bsmod == 0 {
		info.bsmod = frame.bsmod
	}
	if frame.acmod > 0 && info.acmod == 0 {
		info.acmod = frame.acmod
	}
	if frame.hasDsurmod && !info.hasDsurmod {
		info.dsurmod = frame.dsurmod
		info.hasDsurmod = true
	}
	if frame.lfeon > 0 && info.lfeon == 0 {
		info.lfeon = frame.lfeon
	}
	if frame.serviceKind != "" && info.serviceKind == "" {
		info.serviceKind = frame.serviceKind
	}
	if frame.frameRate > 0 && info.frameRate == 0 {
		info.frameRate = frame.frameRate
	}
	if frame.spf > 0 && info.spf == 0 {
		info.spf = frame.spf
	}
	if frame.hasCmixlev && !info.hasCmixlev {
		info.cmixlevDB = frame.cmixlevDB
		info.hasCmixlev = true
	}
	if frame.hasSurmixlev && !info.hasSurmixlev {
		info.surmixlevDB = frame.surmixlevDB
		info.hasSurmixlev = true
	}
	if frame.hasMixlevel && !info.hasMixlevel {
		info.mixlevel = frame.mixlevel
		info.hasMixlevel = true
	}
	if frame.hasRoomtyp && !info.hasRoomtyp {
		info.roomtyp = frame.roomtyp
		info.hasRoomtyp = true
	}
	if frame.hasJOC && !info.hasJOC {
		info.hasJOC = true
	}
	if frame.hasJOCComplex && !info.hasJOCComplex {
		info.hasJOCComplex = true
		info.jocComplexity = frame.jocComplexity
	}
	if frame.jocObjects > 0 && info.jocObjects == 0 {
		info.jocObjects = frame.jocObjects
	}
	if frame.hasJOCDyn && !info.hasJOCDyn {
		info.hasJOCDyn = true
		info.jocDynObjects = frame.jocDynObjects
	}
	if frame.hasJOCBed && !info.hasJOCBed {
		info.hasJOCBed = true
		info.jocBedCount = frame.jocBedCount
		info.jocBedLayout = frame.jocBedLayout
	}
	if frame.hasDmixmod && !info.hasDmixmod {
		info.dmixmod = frame.dmixmod
		info.hasDmixmod = true
	}
	if frame.hasLtrtcmixlev && !info.hasLtrtcmixlev {
		info.ltrtcmixlevDB = frame.ltrtcmixlevDB
		info.hasLtrtcmixlev = true
	}
	if frame.hasLtrtsurmixlev && !info.hasLtrtsurmixlev {
		info.ltrtsurmixlevDB = frame.ltrtsurmixlevDB
		info.hasLtrtsurmixlev = true
	}
	if frame.hasLorocmixlev && !info.hasLorocmixlev {
		info.lorocmixlevDB = frame.lorocmixlevDB
		info.hasLorocmixlev = true
	}
	if frame.hasLorosurmixlev && !info.hasLorosurmixlev {
		info.lorosurmixlevDB = frame.lorosurmixlevDB
		info.hasLorosurmixlev = true
	}
	if frame.hasEAC3ChannelMap && !info.hasEAC3ChannelMap {
		info.hasEAC3ChannelMap = true
		info.eac3ChannelMap = frame.eac3ChannelMap
		info.eac3ChannelMapLayout = frame.eac3ChannelMapLayout
		info.eac3ChannelMapChannel = frame.eac3ChannelMapChannel
	}

	info.framesMerged++
}

func (info ac3Info) dialnormStats() (int, int, int, bool) {
	if info.dialnormCount == 0 {
		return 0, 0, 0, false
	}
	avg := int(math.Round(10.0 * math.Log10(info.dialnormSum/float64(info.dialnormCount))))
	return avg, info.dialnormMin, info.dialnormMax, true
}

func (info ac3Info) comprStats() (float64, float64, float64, int, bool) {
	if len(info.comprs) == 0 {
		return 0, 0, 0, 0, false
	}
	sumIntensity := 0.0
	count := 0
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)
	for code, c := range info.comprs {
		if c == 0 {
			continue
		}
		value := ac3ComprDB(uint8(code))
		if value < minVal {
			minVal = value
		}
		if value > maxVal {
			maxVal = value
		}
		sumIntensity += float64(c) * math.Pow(10.0, value/10.0)
		count += int(c)
	}
	if count == 0 {
		return 0, 0, 0, 0, false
	}
	avg := 10.0 * math.Log10(sumIntensity/float64(count))
	return avg, minVal, maxVal, count, true
}

func (info ac3Info) dynrngStats() (float64, float64, float64, int, bool) {
	if len(info.dynrngs) == 0 || !info.dynrngeSeen {
		return 0, 0, 0, 0, false
	}
	sumIntensity := 0.0
	count := 0
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)
	for code, c := range info.dynrngs {
		if c == 0 {
			continue
		}
		value := ac3DynrngDB(uint8(code))
		if value < minVal {
			minVal = value
		}
		if value > maxVal {
			maxVal = value
		}
		sumIntensity += float64(c) * math.Pow(10.0, value/10.0)
		count += int(c)
	}
	if count == 0 {
		return 0, 0, 0, 0, false
	}
	avg := 10.0 * math.Log10(sumIntensity/float64(count))
	return avg, minVal, maxVal, count, true
}

func ac3SampleRate(code int) float64 {
	switch code {
	case 0:
		return 48000
	case 1:
		return 44100
	case 2:
		return 32000
	default:
		return 0
	}
}

func ac3BitrateKbps(code int) int64 {
	bitRates := []int64{32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 448, 512, 576, 640}
	if code < 0 || code > 37 {
		return 0
	}
	idx := code >> 1
	if idx < 0 || idx >= len(bitRates) {
		return 0
	}
	return bitRates[idx]
}

func ac3FrameSizeBytes(fscod, frmsizecod int) int {
	if fscod < 0 || fscod > 2 || frmsizecod < 0 || frmsizecod >= len(ac3FrameSizeWords) {
		return 0
	}
	return ac3FrameSizeWords[frmsizecod][fscod] * 2
}

var ac3FrameSizeWords = [38][3]int{
	{64, 69, 96},
	{64, 70, 96},
	{80, 87, 120},
	{80, 88, 120},
	{96, 104, 144},
	{96, 105, 144},
	{112, 121, 168},
	{112, 122, 168},
	{128, 139, 192},
	{128, 140, 192},
	{160, 174, 240},
	{160, 175, 240},
	{192, 208, 288},
	{192, 209, 288},
	{224, 243, 336},
	{224, 244, 336},
	{256, 278, 384},
	{256, 279, 384},
	{320, 348, 480},
	{320, 349, 480},
	{384, 417, 576},
	{384, 418, 576},
	{448, 487, 672},
	{448, 488, 672},
	{512, 557, 768},
	{512, 558, 768},
	{640, 696, 960},
	{640, 697, 960},
	{768, 835, 1152},
	{768, 836, 1152},
	{896, 975, 1344},
	{896, 976, 1344},
	{1024, 1114, 1536},
	{1024, 1115, 1536},
	{1152, 1253, 1728},
	{1152, 1254, 1728},
	{1280, 1393, 1920},
	{1280, 1394, 1920},
}

func ac3DialnormDB(code uint32) int {
	if code == 0 {
		return -31
	}
	return -int(code)
}

var ac3DynrngBase = []float64{
	6.02,
	12.04,
	18.06,
	24.08,
	-18.06,
	-12.04,
	-6.02,
	0.00,
}

var ac3ComprBase = []float64{
	6.02,
	12.04,
	18.06,
	24.08,
	30.10,
	36.12,
	42.14,
	48.16,
	-42.14,
	-36.12,
	-30.10,
	-24.08,
	-18.06,
	-12.04,
	-6.02,
	0.00,
}

func ac3DynrngDB(code uint8) float64 {
	if code == 0 {
		return 0
	}
	base := ac3DynrngBase[code>>5]
	fine := 20.0 * math.Log10(float64(0x20+int(code&0x1F))/64.0)
	return base + fine
}

func ac3ComprDB(code uint8) float64 {
	if code == 0 {
		return 0
	}
	base := ac3ComprBase[code>>4]
	fine := 20.0 * math.Log10(float64(0x10+int(code&0x0F))/32.0)
	return base + fine
}

func ac3CenterMixLevelDB(code uint32) (float64, bool) {
	switch code {
	case 0:
		return -3.0, true
	case 1:
		return -4.5, true
	case 2:
		return -6.0, true
	default:
		return 0, false
	}
}

func ac3SurroundMixLevelDB(code uint32) (float64, bool) {
	switch code {
	case 0:
		return -3, true
	case 1:
		return -6, true
	case 2:
		return 0, true
	default:
		return 0, false
	}
}

// ac3ExtendedMixLevelDB maps AC-3 xbsi extended mix-level codes to dB values.
func ac3ExtendedMixLevelDB(code uint32) (float64, bool) {
	switch code {
	case 0:
		return 3.0, true
	case 1:
		return 1.5, true
	case 2:
		return 0.0, true
	case 3:
		return -1.5, true
	case 4:
		return -3.0, true
	case 5:
		return -4.5, true
	case 6:
		return -6.0, true
	default:
		return 0, false
	}
}

// ac3PreferredDownmix maps AC-3 xbsi preferred stereo downmix codes to MediaInfo labels.
func ac3PreferredDownmix(code uint32) string {
	switch code {
	case 1:
		return "Lt/Rt"
	case 2:
		return "Lo/Ro"
	default:
		return ""
	}
}

func ac3RoomType(code uint32) (string, bool) {
	switch code {
	case 0:
		return "Not indicated", true
	case 1:
		return "Large", true
	case 2:
		return "Small", true
	default:
		return "", false
	}
}

func ac3FullBandwidthChannels(acmod int) int {
	switch acmod {
	case 0:
		return 2
	case 1:
		return 1
	case 2:
		return 2
	case 3:
		return 3
	case 4:
		return 3
	case 5:
		return 4
	case 6:
		return 4
	case 7:
		return 5
	default:
		return 0
	}
}

func parseAC3Dynrng(br *ac3BitReader, acmod int) (bool, byte, bool) {
	nfchans := ac3FullBandwidthChannels(acmod)
	if nfchans <= 0 {
		return false, 0, false
	}
	for range nfchans {
		if _, ok := br.readBits(1); !ok {
			return false, 0, false
		}
	}
	for range nfchans {
		if _, ok := br.readBits(1); !ok {
			return false, 0, false
		}
	}
	dynrnge, ok := br.readBits(1)
	if !ok {
		return false, 0, false
	}
	if dynrnge == 0 {
		return false, 0, true
	}
	dynrng, ok := br.readBits(8)
	if !ok {
		return false, 0, false
	}
	return true, byte(dynrng), true
}

func ac3ChannelLayout(acmod int, lfeon bool) (uint64, string) {
	var layout []string
	switch acmod {
	case 0:
		layout = []string{"L", "R"}
	case 1:
		layout = []string{"C"}
	case 2:
		layout = []string{"L", "R"}
	case 3:
		layout = []string{"L", "R", "C"}
	case 4:
		layout = []string{"L", "R", "S"}
	case 5:
		layout = []string{"L", "R", "C", "S"}
	case 6:
		layout = []string{"L", "R", "Ls", "Rs"}
	case 7:
		layout = []string{"L", "R", "C", "Ls", "Rs"}
	default:
		return 0, ""
	}
	if lfeon {
		withLFE := make([]string, 0, len(layout)+1)
		inserted := false
		for _, ch := range layout {
			withLFE = append(withLFE, ch)
			if ch == "C" {
				withLFE = append(withLFE, "LFE")
				inserted = true
			}
		}
		if !inserted {
			withLFE = append(withLFE, "LFE")
		}
		layout = withLFE
	}
	return uint64(len(layout)), strings.Join(layout, " ")
}

func ac3ServiceKind(bsmod int) string {
	switch bsmod {
	case 0:
		return "Complete Main"
	case 1:
		return "Music and Effects"
	case 2:
		return "Visually Impaired"
	case 3:
		return "Hearing Impaired"
	case 4:
		return "Dialogue"
	case 5:
		return "Commentary"
	case 6:
		return "Emergency"
	case 7:
		return "Voice Over"
	default:
		return ""
	}
}

func ac3ServiceKindCode(bsmod int) string {
	switch bsmod {
	case 0:
		return "CM"
	case 1:
		return "ME"
	case 2:
		return "VI"
	case 3:
		return "HI"
	case 4:
		return "D"
	case 5:
		return "C"
	case 6:
		return "E"
	case 7:
		return "VO"
	default:
		return ""
	}
}
