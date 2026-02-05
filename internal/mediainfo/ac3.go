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
	dynrngeSeen   bool
	cmixlevDB     float64
	hasCmixlev    bool
	surmixlevDB   float64
	hasSurmixlev  bool
	mixlevel      int
	hasMixlevel   bool
	roomtyp       string
	hasRoomtyp    bool
}

type ac3BitReader struct {
	data   []byte
	bitPos int
}

func (br *ac3BitReader) readBits(n int) (uint32, bool) {
	if n <= 0 || br.bitPos+n > len(br.data)*8 {
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
	bsid, ok := br.readBits(5)
	if !ok {
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
	if acmod == 0 {
		if _, ok = br.readBits(2); !ok {
			return info, 0, false
		}
		if _, ok = br.readBits(2); !ok {
			return info, 0, false
		}
	} else {
		if acmod&1 != 0 {
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
		info.comprFieldDB = ac3ComprDB(uint8(compr))
		info.hasComprField = true
		info.hasCompr = true
		info.comprDB = info.comprFieldDB
		if compr != 0xFF {
			info.comprSum = math.Pow(10.0, info.comprDB/10.0)
			info.comprCount = 1
			info.comprMin = info.comprDB
			info.comprMax = info.comprDB
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
	if _, ok := br.readBits(1); !ok { // copyrightb
		return info, 0, false
	}
	if _, ok := br.readBits(1); !ok { // origbs
		return info, 0, false
	}
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
		if dynrnge {
			info.dynrngeSeen = true
			if code != 0xFF {
				value := ac3DynrngDB(code)
				info.dynrngDB = value
				info.hasDynrng = true
				info.dynrngSum = math.Pow(10.0, value/10.0)
				info.dynrngCount = 1
				info.dynrngMin = value
				info.dynrngMax = value
			}
		} else {
			info.dynrngSum = 1
			info.dynrngCount = 1
			info.dynrngMin = 0
			info.dynrngMax = 0
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
		bitRateKbps:   bitRate,
		sampleRate:    sampleRate,
		channels:      channels,
		layout:        layout,
		bsid:          int(bsid),
		bsmod:         int(bsmod),
		acmod:         int(acmod),
		lfeon:         int(lfeonVal),
		dsurmod:       info.dsurmod,
		hasDsurmod:    info.hasDsurmod,
		serviceKind:   ac3ServiceKind(int(bsmod)),
		frameRate:     frameRate,
		spf:           spf,
		dialnorm:      info.dialnorm,
		dialnormSum:   info.dialnormSum,
		dialnormCount: info.dialnormCount,
		dialnormMin:   info.dialnormMin,
		dialnormMax:   info.dialnormMax,
		hasDialnorm:   info.hasDialnorm,
		comprDB:       info.comprDB,
		comprCount:    info.comprCount,
		comprSum:      info.comprSum,
		comprSumDB:    info.comprSumDB,
		comprMin:      info.comprMin,
		comprMax:      info.comprMax,
		comprIsDB:     info.comprIsDB,
		comprFieldDB:  info.comprFieldDB,
		hasCompr:      info.hasCompr,
		hasComprField: info.hasComprField,
		dynrngDB:      info.dynrngDB,
		hasDynrng:     info.hasDynrng,
		dynrngSum:     info.dynrngSum,
		dynrngCount:   info.dynrngCount,
		dynrngMin:     info.dynrngMin,
		dynrngMax:     info.dynrngMax,
		dynrngeSeen:   info.dynrngeSeen,
		cmixlevDB:     info.cmixlevDB,
		hasCmixlev:    info.hasCmixlev,
		surmixlevDB:   info.surmixlevDB,
		hasSurmixlev:  info.hasSurmixlev,
		mixlevel:      info.mixlevel,
		hasMixlevel:   info.hasMixlevel,
		roomtyp:       info.roomtyp,
		hasRoomtyp:    info.hasRoomtyp,
	}
	return info, frameSize, true
}

func parseEAC3Frame(payload []byte) (ac3Info, int, bool) {
	var info ac3Info
	if len(payload) < 7 {
		return info, 0, false
	}
	br := ac3BitReader{data: payload}
	if sync, ok := br.readBits(16); !ok || sync != 0x0B77 {
		return info, 0, false
	}
	if _, ok := br.readBits(2); !ok { // strmtyp
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
	fscod, ok := br.readBits(2)
	if !ok {
		return info, 0, false
	}
	fscod2 := uint32(0)
	if fscod == 3 {
		val, ok := br.readBits(2)
		if !ok {
			return info, 0, false
		}
		fscod2 = val
	}
	numblkscod, ok := br.readBits(2)
	if !ok {
		return info, 0, false
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
	dialnorm, ok := br.readBits(5)
	if !ok {
		return info, 0, false
	}
	info.hasDialnorm = true
	info.dialnorm = ac3DialnormDB(dialnorm)
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
		info.comprDB = ac3ComprDB(uint8(compr))
		info.comprSum = math.Pow(10.0, info.comprDB/10.0)
		info.comprCount = 1
		info.comprMin = info.comprDB
		info.comprMax = info.comprDB
		info.hasCompr = true
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
	info = ac3Info{
		bitRateKbps:   bitRate,
		sampleRate:    sampleRate,
		channels:      channels,
		layout:        layout,
		bsid:          int(bsid),
		bsmod:         0,
		acmod:         int(acmod),
		lfeon:         int(lfeonVal),
		serviceKind:   ac3ServiceKind(0),
		frameRate:     frameRate,
		spf:           spf,
		dialnorm:      info.dialnorm,
		dialnormSum:   info.dialnormSum,
		dialnormCount: info.dialnormCount,
		dialnormMin:   info.dialnormMin,
		dialnormMax:   info.dialnormMax,
		hasDialnorm:   info.hasDialnorm,
		comprDB:       info.comprDB,
		comprCount:    info.comprCount,
		comprSum:      info.comprSum,
		comprMin:      info.comprMin,
		comprMax:      info.comprMax,
		dynrngDB:      info.dynrngDB,
		hasDynrng:     info.hasDynrng,
		hasCompr:      info.hasCompr,
	}
	return info, frameSize, true
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
	if frame.hasComprField {
		if !info.hasComprField || frame.comprFieldDB > info.comprFieldDB {
			info.comprFieldDB = frame.comprFieldDB
			info.hasComprField = true
		}
	}
	if frame.hasCompr && !info.hasCompr {
		info.comprDB = frame.comprDB
		info.hasCompr = true
	}
	if frame.comprCount > 0 {
		if info.comprCount == 0 {
			info.comprIsDB = frame.comprIsDB
			info.comprSum = frame.comprSum
			info.comprSumDB = frame.comprSumDB
			info.comprCount = frame.comprCount
			info.comprMin = frame.comprMin
			info.comprMax = frame.comprMax
		} else {
			if info.comprIsDB {
				info.comprSumDB += frame.comprSumDB
			} else {
				info.comprSum += frame.comprSum
			}
			info.comprCount += frame.comprCount
			if frame.comprMin < info.comprMin {
				info.comprMin = frame.comprMin
			}
			if frame.comprMax > info.comprMax {
				info.comprMax = frame.comprMax
			}
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
	if frame.hasDynrng && !info.hasDynrng {
		info.dynrngDB = frame.dynrngDB
		info.hasDynrng = true
	}
	if frame.dynrngeSeen {
		info.dynrngeSeen = true
	}
	if frame.dynrngCount > 0 {
		if info.dynrngCount == 0 {
			info.dynrngSum = frame.dynrngSum
			info.dynrngCount = frame.dynrngCount
			info.dynrngMin = frame.dynrngMin
			info.dynrngMax = frame.dynrngMax
		} else {
			info.dynrngSum += frame.dynrngSum
			info.dynrngCount += frame.dynrngCount
			if frame.dynrngMin < info.dynrngMin {
				info.dynrngMin = frame.dynrngMin
			}
			if frame.dynrngMax > info.dynrngMax {
				info.dynrngMax = frame.dynrngMax
			}
		}
	}
	if frame.hasDialnorm {
		if info.dialnormCount == 0 {
			info.dialnorm = frame.dialnorm
			info.dialnormSum = frame.dialnormSum
			info.dialnormCount = frame.dialnormCount
			info.dialnormMin = frame.dialnormMin
			info.dialnormMax = frame.dialnormMax
			info.hasDialnorm = true
			return
		}
		info.dialnormSum += frame.dialnormSum
		info.dialnormCount += frame.dialnormCount
		if frame.dialnormMin < info.dialnormMin {
			info.dialnormMin = frame.dialnormMin
		}
		if frame.dialnormMax > info.dialnormMax {
			info.dialnormMax = frame.dialnormMax
		}
		info.hasDialnorm = true
	}
}

func (info ac3Info) dialnormStats() (int, int, int, bool) {
	if info.dialnormCount == 0 {
		return 0, 0, 0, false
	}
	avg := int(math.Round(10.0 * math.Log10(info.dialnormSum/float64(info.dialnormCount))))
	return avg, info.dialnormMin, info.dialnormMax, true
}

func (info ac3Info) comprStats() (float64, float64, float64, int, bool) {
	if info.comprCount == 0 {
		return 0, 0, 0, 0, false
	}
	var avg float64
	if info.comprIsDB {
		avg = info.comprSumDB / float64(info.comprCount)
	} else {
		avg = 10.0 * math.Log10(info.comprSum/float64(info.comprCount))
	}
	return avg, info.comprMin, info.comprMax, info.comprCount, true
}

func (info ac3Info) dynrngStats() (float64, float64, float64, int, bool) {
	if info.dynrngCount == 0 || !info.dynrngeSeen {
		return 0, 0, 0, 0, false
	}
	avg := 10.0 * math.Log10(info.dynrngSum/float64(info.dynrngCount))
	return avg, info.dynrngMin, info.dynrngMax, info.dynrngCount, true
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
