package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

type dolbyVisionConfig struct {
	versionMajor    uint8
	versionMinor    uint8
	profile         uint8
	level           uint8
	rpuPresent      bool
	elPresent       bool
	blPresent       bool
	compatibilityID uint8
	compressionID   uint8
}

var dolbyVisionCompatibility = []string{
	"",
	"HDR10",
	"SDR",
	"",
	"HLG",
	"",
	"Blu-ray",
}

var dolbyVisionProfilePrefixes = []string{
	"dvav",
	"dvav",
	"dvhe",
	"dvhe",
	"dvhe",
	"dvhe",
	"dvhe",
	"dvhe",
	"dvhe",
	"dvav",
	"dav1",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"dvh1",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"davc",
	"",
	"dvh8",
}

func parseDolbyVisionConfigFromPrivate(codecPrivate []byte) string {
	payload := findDolbyVisionConfig(codecPrivate)
	if len(payload) == 0 {
		return ""
	}
	config, ok := parseDolbyVisionConfig(payload)
	if !ok {
		return ""
	}
	return formatDolbyVisionHDR(config)
}

func parseDolbyVisionConfigFromPrivateRaw(codecPrivate []byte) (dolbyVisionConfig, bool) {
	payload := findDolbyVisionConfig(codecPrivate)
	if len(payload) == 0 {
		return dolbyVisionConfig{}, false
	}
	return parseDolbyVisionConfig(payload)
}

func findDolbyVisionConfig(data []byte) []byte {
	if len(data) < 4 {
		return nil
	}
	for i := 0; i+8 <= len(data); i++ {
		tag := string(data[i+4 : i+8])
		if tag != "dvcC" && tag != "dvvC" {
			continue
		}
		size := int(binary.BigEndian.Uint32(data[i : i+4]))
		if size < 8 || i+size > len(data) {
			continue
		}
		return data[i+8 : i+size]
	}
	if tag := string(data[:4]); tag == "dvcC" || tag == "dvvC" {
		return data[4:]
	}
	if idx := bytes.Index(data, []byte("dvvC")); idx != -1 {
		if extra := findDolbyVisionExtraData(data, idx+4); len(extra) > 0 {
			return extra
		}
	}
	if idx := bytes.Index(data, []byte("dvcC")); idx != -1 {
		if extra := findDolbyVisionExtraData(data, idx+4); len(extra) > 0 {
			return extra
		}
	}
	return nil
}

func findDolbyVisionExtraData(data []byte, start int) []byte {
	limit := min(len(data), start+128)
	for i := start; i+2 < limit; i++ {
		if data[i] != 0x41 || data[i+1] != 0xED {
			continue
		}
		size, sizeLen, ok := readVintSize(data, i+2)
		if !ok || size == unknownVintSize {
			continue
		}
		payloadStart := i + 2 + sizeLen
		payloadEnd := payloadStart + int(size)
		if payloadEnd <= len(data) {
			return data[payloadStart:payloadEnd]
		}
	}
	return nil
}

func parseDolbyVisionConfig(payload []byte) (dolbyVisionConfig, bool) {
	if len(payload) < 4 {
		return dolbyVisionConfig{}, false
	}
	cfg := dolbyVisionConfig{
		versionMajor: payload[0],
		versionMinor: payload[1],
	}
	if cfg.versionMajor == 0 || cfg.versionMajor > 3 {
		return cfg, false
	}
	br := newBitReader(payload[2:])
	profile := br.readBitsValue(7)
	level := br.readBitsValue(6)
	if profile == ^uint64(0) || level == ^uint64(0) {
		return dolbyVisionConfig{}, false
	}
	cfg.profile = uint8(profile)
	cfg.level = uint8(level)
	cfg.rpuPresent = br.readBitsValue(1) == 1
	cfg.elPresent = br.readBitsValue(1) == 1
	cfg.blPresent = br.readBitsValue(1) == 1
	compat := br.readBitsValue(4)
	compr := br.readBitsValue(2)
	if compat != ^uint64(0) {
		cfg.compatibilityID = uint8(compat)
	}
	if compr != ^uint64(0) {
		cfg.compressionID = uint8(compr)
	}
	return cfg, true
}

func formatDolbyVisionHDR(cfg dolbyVisionConfig) string {
	parts := []string{"Dolby Vision"}
	if cfg.versionMajor > 0 {
		parts = append(parts, fmt.Sprintf("Version %d.%d", cfg.versionMajor, cfg.versionMinor))
	}
	profilePrefix := dolbyVisionProfilePrefix(cfg.profile)
	profileDisplay := ""
	if profilePrefix != "" {
		profileDisplay = fmt.Sprintf("Profile %d", cfg.profile)
		if compatIndex := dolbyVisionCompatibilityIndex(cfg.compatibilityID); compatIndex >= 0 {
			profileDisplay = fmt.Sprintf("Profile %d.%s", cfg.profile, strings.TrimPrefix(strconv.FormatInt(int64(compatIndex), 16), "0"))
		}
	}
	if profileDisplay != "" {
		parts = append(parts, profileDisplay)
	}
	if profilePrefix != "" {
		profileTag := fmt.Sprintf("%s.%02d", profilePrefix, cfg.profile)
		parts = append(parts, fmt.Sprintf("%s.%02d", profileTag, cfg.level))
	}
	if layers := dolbyVisionLayers(cfg); layers != "" {
		parts = append(parts, layers)
	}
	if compression := dolbyVisionCompression(cfg.compressionID); compression != "" {
		parts = append(parts, compression)
	}
	result := strings.Join(parts, ", ")
	if compat := dolbyVisionCompatibilityName(cfg.compatibilityID); compat != "" {
		result += ", " + compat + " compatible"
	}
	return result
}

func dolbyVisionProfilePrefix(profile uint8) string {
	if int(profile) >= len(dolbyVisionProfilePrefixes) {
		return ""
	}
	return dolbyVisionProfilePrefixes[profile]
}

func dolbyVisionLayers(cfg dolbyVisionConfig) string {
	layers := []string{}
	if cfg.blPresent {
		layers = append(layers, "BL")
	}
	if cfg.elPresent {
		layers = append(layers, "EL")
	}
	if cfg.rpuPresent {
		layers = append(layers, "RPU")
	}
	if len(layers) == 0 {
		return ""
	}
	return strings.Join(layers, "+")
}

func dolbyVisionCompression(id uint8) string {
	switch id {
	case 0:
		return "no metadata compression"
	case 1:
		return "limited metadata compression"
	case 3:
		return "extended metadata compression"
	default:
		return ""
	}
}

func dolbyVisionCompatibilityName(id uint8) string {
	if int(id) >= len(dolbyVisionCompatibility) {
		return ""
	}
	return dolbyVisionCompatibility[id]
}

func dolbyVisionCompatibilityIndex(id uint8) int {
	if int(id) >= len(dolbyVisionCompatibility) {
		return -1
	}
	if dolbyVisionCompatibility[id] == "" {
		return -1
	}
	return int(id)
}
