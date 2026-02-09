package mediainfo

import "encoding/binary"

type pngInfo struct {
	Width      int
	Height     int
	BitDepth   int
	ColorSpace string
}

func parsePNGInfo(data []byte) (pngInfo, bool) {
	// Minimal PNG parse: signature + IHDR.
	if len(data) < 8+8+13 {
		return pngInfo{}, false
	}
	sig := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	for i := 0; i < len(sig); i++ {
		if data[i] != sig[i] {
			return pngInfo{}, false
		}
	}

	// First chunk should be IHDR.
	chunkLen := int(binary.BigEndian.Uint32(data[8:12]))
	if chunkLen < 13 || 8+8+chunkLen > len(data) {
		return pngInfo{}, false
	}
	if string(data[12:16]) != "IHDR" {
		return pngInfo{}, false
	}
	ihdr := data[16 : 16+chunkLen]
	if len(ihdr) < 13 {
		return pngInfo{}, false
	}
	w := int(binary.BigEndian.Uint32(ihdr[0:4]))
	h := int(binary.BigEndian.Uint32(ihdr[4:8]))
	bitDepth := int(ihdr[8])
	colorType := ihdr[9]

	cs := ""
	switch colorType {
	case 0, 4: // grayscale / grayscale+alpha
		cs = "Y"
	case 2, 3, 6: // truecolor / indexed / truecolor+alpha
		// MediaInfo reports RGB even when an alpha channel is present.
		cs = "RGB"
	}

	return pngInfo{Width: w, Height: h, BitDepth: bitDepth, ColorSpace: cs}, true
}
