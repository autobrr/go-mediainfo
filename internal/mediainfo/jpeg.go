package mediainfo

import (
	"encoding/binary"
)

type jpegInfo struct {
	Width           int
	Height          int
	BitDepth        int
	ChromaSubsample string
	ColorSpace      string
}

func parseJPEGInfo(data []byte) (jpegInfo, bool) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return jpegInfo{}, false
	}
	i := 2
	for i+4 <= len(data) {
		// Find next marker.
		if data[i] != 0xFF {
			i++
			continue
		}
		for i < len(data) && data[i] == 0xFF {
			i++
		}
		if i >= len(data) {
			break
		}
		marker := data[i]
		i++

		// Standalone markers.
		if marker == 0xD9 || marker == 0xDA { // EOI, SOS
			break
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}
		if i+2 > len(data) {
			break
		}
		segLen := int(binary.BigEndian.Uint16(data[i : i+2]))
		i += 2
		if segLen < 2 || i+segLen-2 > len(data) {
			break
		}
		seg := data[i : i+segLen-2]
		i += segLen - 2

		if !isSOFMarker(marker) {
			continue
		}
		// SOF segment layout:
		// [precision 1][height 2][width 2][components 1][components...]
		if len(seg) < 6 {
			return jpegInfo{}, false
		}
		prec := int(seg[0])
		h := int(binary.BigEndian.Uint16(seg[1:3]))
		w := int(binary.BigEndian.Uint16(seg[3:5]))
		comps := int(seg[5])
		chroma := ""
		color := ""
		if comps >= 3 && len(seg) >= 6+comps*3 {
			// Component entries: [id 1][sampling 1][qt 1]
			var yH, yV, cbH, cbV, crH, crV int
			for c := 0; c < comps; c++ {
				base := 6 + c*3
				id := seg[base]
				samp := seg[base+1]
				hvH := int((samp >> 4) & 0x0F)
				hvV := int(samp & 0x0F)
				switch id {
				case 1:
					yH, yV = hvH, hvV
				case 2:
					cbH, cbV = hvH, hvV
				case 3:
					crH, crV = hvH, hvV
				}
			}
			if yH > 0 && yV > 0 && cbH > 0 && cbV > 0 && crH > 0 && crV > 0 {
				color = "YUV"
				switch {
				case yH == 2 && yV == 2 && cbH == 1 && cbV == 1 && crH == 1 && crV == 1:
					chroma = "4:2:0"
				case yH == 2 && yV == 1 && cbH == 1 && cbV == 1 && crH == 1 && crV == 1:
					chroma = "4:2:2"
				case yH == 1 && yV == 1 && cbH == 1 && cbV == 1 && crH == 1 && crV == 1:
					chroma = "4:4:4"
				}
			}
		}
		return jpegInfo{
			Width:           w,
			Height:          h,
			BitDepth:        prec,
			ChromaSubsample: chroma,
			ColorSpace:      color,
		}, true
	}
	return jpegInfo{}, false
}

func isSOFMarker(marker byte) bool {
	// SOF markers: C0..CF excluding C4, C8, CC.
	if marker < 0xC0 || marker > 0xCF {
		return false
	}
	switch marker {
	case 0xC4, 0xC8, 0xCC:
		return false
	default:
		return true
	}
}
