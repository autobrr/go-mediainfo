package mediainfo

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
)

// mp4Metadata keeps capture metadata separate from presentation timing and
// retains unknown named values in the same meta atom's encounter order.
type mp4Metadata struct {
	fields             []Field
	extras             []jsonKV
	frameRateReal      float64
	displayAspectRatio string
	vendorVersion      string
}

// mp4MetaChildren accepts the ISO FullBox and QuickTime layouts. The prefix is
// recognized structurally rather than searching arbitrary payload bytes.
func mp4MetaChildren(payload []byte) []byte {
	if len(payload) >= 4 && binary.BigEndian.Uint32(payload[:4]) == 0 {
		return payload[4:]
	}
	return payload
}

func parseMP4Meta(payload []byte, budget *embeddedAssetBudget) mp4Metadata {
	var m mp4Metadata
	children := mp4MetaChildren(payload)
	h, ok := findMP4Box(children, "hdlr")
	if !ok || len(h) < 12 || string(h[8:12]) != "mdta" {
		return m
	}
	keyData, ok := findMP4Box(children, "keys")
	if !ok || len(keyData) < 8 {
		return m
	}
	count := binary.BigEndian.Uint32(keyData[4:8])
	if count > uint32(embeddedAssetMaxItems) || uint64(count) > uint64(len(keyData)-8)/8 {
		return m
	}
	keys := make([]string, count)
	pos := 8
	for i := range keys {
		if len(keyData)-pos < 8 {
			return mp4Metadata{}
		}
		size := uint64(binary.BigEndian.Uint32(keyData[pos : pos+4]))
		if size < 8 || size > uint64(len(keyData)-pos) {
			return mp4Metadata{}
		}
		if string(keyData[pos+4:pos+8]) == "mdta" && budget.reserveItem() == embeddedAssetAccepted && budget.reserveString(size-8, embeddedAssetMaxNameBytes) == embeddedAssetAccepted {
			keys[i] = string(keyData[pos+8 : pos+int(size)])
		}
		pos += int(size)
	}
	ilst, ok := findMP4Box(children, "ilst")
	if !ok {
		return m
	}
	for off := int64(0); off+8 <= int64(len(ilst)); {
		size, key, header := readMP4BoxHeaderFrom(ilst, off)
		if size <= 0 {
			break
		}
		index := binary.BigEndian.Uint32([]byte(key))
		if index > 0 && uint64(index) <= uint64(len(keys)) && keys[index-1] != "" {
			item := sliceBox(ilst, off+header, size-header)
			for p := int64(0); p+8 <= int64(len(item)); {
				n, typ, h := readMP4BoxHeaderFrom(item, p)
				if n <= 0 {
					break
				}
				if typ == "data" {
					data := sliceBox(item, p+h, n-h)
					if len(data) >= 8 && int64(len(data)-8) <= embeddedAssetMaxStringBytes {
						if value, valid := decodeMP4MetadataValue(data); valid && value != "" && budget.reserveString(uint64(len(value)), embeddedAssetMaxStringBytes) == embeddedAssetAccepted {
							m.put(keys[index-1], value)
						}
					}
				}
				p += n
			}
		}
		off += size
	}
	return m
}

// decodeMP4MetadataValue uses the declared type and exact numeric width. A
// malformed later value is rejected, so it cannot replace a valid earlier one.
func decodeMP4MetadataValue(data []byte) (string, bool) {
	if len(data) < 8 || int64(len(data)-8) > embeddedAssetMaxStringBytes {
		return "", false
	}
	kind := binary.BigEndian.Uint32(data[:4])
	v := data[8:]
	switch kind {
	case 1, 3:
		return strings.TrimRight(string(v), "\x00"), true
	case 21, 22:
		if len(v) != 1 && len(v) != 2 && len(v) != 4 && len(v) != 8 {
			return "", false
		}
		var n uint64
		for _, b := range v {
			n = n<<8 | uint64(b)
		}
		if kind == 22 {
			return strconv.FormatUint(n, 10), true
		}
		shift := uint(64 - len(v)*8)
		return strconv.FormatInt(int64(n<<shift)>>shift, 10), true
	case 23, 24:
		var n float64
		if kind == 23 && len(v) == 4 {
			n = float64(math.Float32frombits(binary.BigEndian.Uint32(v)))
		} else if kind == 24 && len(v) == 8 {
			n = math.Float64frombits(binary.BigEndian.Uint64(v))
		} else {
			return "", false
		}
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return "", false
		}
		return strconv.FormatFloat(n, 'f', -1, 64), true
	}
	return "", false
}

func (m *mp4Metadata) put(key, value string) {
	field := ""
	switch key {
	case "com.android.capture.fps":
		if f, err := strconv.ParseFloat(value, 64); err == nil && f > 0 && !math.IsInf(f, 0) && !math.IsNaN(f) {
			m.frameRateReal = f
		}
		return
	case "aspect_ratio":
		if a, b, ok := strings.Cut(value, ":"); ok {
			x, e1 := strconv.ParseFloat(a, 64)
			y, e2 := strconv.ParseFloat(b, 64)
			if e1 != nil || e2 != nil || y == 0 {
				return
			}
			value = strconv.FormatFloat(x/y, 'f', 3, 64)
		}
		if f, err := strconv.ParseFloat(value, 64); err == nil && f > 0 && !math.IsNaN(f) && !math.IsInf(f, 0) {
			m.displayAspectRatio = value
		}
		return
	case "com.apple.quicktime.copyright":
		field = "Copyright"
	case "com.apple.quicktime.displayname":
		field = "Title"
		value = strings.TrimSpace(value)
	case "date_recorded":
		field = "Recorded_Date"
		if len(value) >= 10 && value[4] == ':' && value[7] == ':' {
			value = value[:4] + "-" + value[5:7] + "-" + value[8:]
		}
	case "DATE":
		field = "Encoded_Date"
	case "PERFORMER":
		field = "Performer"
	case "PUBLISHER":
		field = "Publisher"
	case "com.apple.quicktime.comment":
		field = "Comment"
	case "com.apple.quicktime.description":
		field = "Description"
	case "com.apple.quicktime.version", "com.apple.quicktime.player.version":
		m.vendorVersion = strings.SplitN(value, " ", 2)[0]
		return
	case "com.apple.finalcutstudio.media.uuid":
		key = "Media/UUID"
	case "com.apple.finalcutstudio.media.history.uuid":
		key = "Media/History/UUID"
	case "com.universaladid.idregistry":
		field = "UniversalAdID_Registry"
	case "com.universaladid.idvalue":
		field = "UniversalAdID_Value"
	case "iTunEXTC":
		field = "ContentRating"
	case "iTunMOVI":
		key = "iTunMOVI"
	case "iTunNORM", "iTunes_CDDB_IDs", "iTunSMPB":
		return
	case "com.android.version":
		m.fields = setFieldValue(m.fields, "Encoded_OperatingSystem_CompanyName", "Google")
		m.fields = setFieldValue(m.fields, "Encoded_OperatingSystem_Name", "Android")
		field = "Encoded_OperatingSystem_Version"
	case "com.apple.quicktime.location.ISO6709":
		field = "Recorded_Location"
		value = mp4RecordedLocation(value)
	}
	if value == "" {
		return
	}
	if field != "" {
		m.fields = setFieldValue(m.fields, field, value)
	} else {
		m.extras = setJSONField(m.extras, key, value)
	}
}

// mp4RecordedLocation applies the adopted ISO6709 display conversion only to
// a complete signed coordinate string; other metadata retains its raw value.
func mp4RecordedLocation(value string) string {
	// The upstream adapter first pads existing DMS minute/second components.
	// Leave degrees, hemispheres and unrelated suffixes untouched.
	padded := value
	changed := false
	start := 0
	for range 2 {
		degree := strings.Index(padded[start:], "°")
		if degree < 0 {
			break
		}
		degree += start + len("°")
		end := strings.IndexByte(padded[degree:], ' ')
		if end < 0 {
			end = len(padded)
		} else {
			end += degree
		}
		minute := strings.IndexByte(padded[degree:end], '\'')
		if minute < 0 {
			break
		}
		minute += degree
		dot := strings.IndexByte(padded[degree:minute], '.')
		digits := minute - degree
		if dot >= 0 {
			digits = dot
		}
		if digits == 1 {
			padded = padded[:degree] + "0" + padded[degree:]
			minute++
			end++
			changed = true
		}
		second := strings.IndexByte(padded[minute+1:end], '"')
		if second >= 0 {
			second += minute + 1
			dot = strings.IndexByte(padded[minute+1:second], '.')
			digits = second - minute - 1
			if dot >= 0 {
				digits = dot
			}
			if digits == 1 {
				padded = padded[:minute+1] + "0" + padded[minute+1:]
				end++
				changed = true
			}
		}
		start = end
	}
	if changed {
		return padded
	}
	slash := strings.IndexByte(value, '/')
	if slash < 2 || len(value) < 2 || (value[0] != '+' && value[0] != '-') {
		return value
	}
	lon := strings.IndexAny(value[1:slash], "+-")
	if lon < 0 {
		return value
	}
	lon++
	alt := strings.IndexAny(value[lon+1:slash], "+-")
	if alt >= 0 {
		alt += lon + 1
	}
	end := slash
	if alt >= 0 {
		end = alt
	}
	latValue, lonValue := value[1:lon], value[lon+1:end]
	valid := func(s string) bool {
		if s == "" {
			return false
		}
		dots := 0
		for _, r := range s {
			if r == '.' {
				dots++
			} else if r < '0' || r > '9' {
				return false
			}
		}
		return dots <= 1 && s != "."
	}
	if !valid(latValue) || !valid(lonValue) {
		return value
	}
	trim := func(s string) string {
		for len(s) > 1 && s[0] == '0' && s[1] >= '0' && s[1] <= '9' {
			s = s[1:]
		}
		return s
	}
	latValue, lonValue = trim(latValue), trim(lonValue)
	altitude := ""
	if alt >= 0 {
		if !valid(value[alt+1 : slash]) {
			return value
		}
		altitude = value[alt:slash]
		if altitude[0] == '+' {
			altitude = trim(altitude[1:])
		}
		if strings.Contains(altitude, ".") && strings.Trim(altitude, "0.-") == "" {
			altitude = ""
		}
	}
	if strings.Trim(latValue, "0.") == "" && strings.Trim(lonValue, "0.") == "" && altitude == "" {
		return ""
	}
	latHem, lonHem := "N", "E"
	if value[0] == '-' {
		latHem = "S"
	}
	if value[lon] == '-' {
		lonHem = "W"
	}
	result := latValue + "°" + latHem + " " + lonValue + "°" + lonHem
	if altitude != "" {
		result += " " + altitude + "m"
	}
	return result
}

func mp4HandlerTitleEligible(name string) bool {
	if strings.Contains(name, "Handler") || strings.Contains(name, " handler") {
		return false
	}
	for _, prefix := range []string{"Gestionnaire ", "Module ", "Gestor ", "Procedura "} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

func (m *mp4Metadata) merge(other mp4Metadata) {
	for _, f := range other.fields {
		m.fields = setFieldValue(m.fields, f.Name, f.Value)
	}
	for _, f := range other.extras {
		m.extras = setJSONField(m.extras, f.Key, f.Val)
	}
	if other.frameRateReal > 0 {
		m.frameRateReal = other.frameRateReal
	}
	if other.displayAspectRatio != "" {
		m.displayAspectRatio = other.displayAspectRatio
	}
	if other.vendorVersion != "" {
		m.vendorVersion = other.vendorVersion
	}
}

func parseMP4MetadataChildren(buf []byte, m *mp4Metadata, budget *embeddedAssetBudget) {
	for offset := int64(0); offset+8 <= int64(len(buf)); {
		size, typ, header := readMP4BoxHeaderFrom(buf, offset)
		if size <= 0 {
			break
		}
		payload := sliceBox(buf, offset+header, size-header)
		if typ == "meta" {
			m.merge(parseMP4Meta(payload, budget))
		}
		// QuickTime copyright-xyz stores a length and language before the value;
		// it is not an indexed keys/ilst item.
		if typ == "\xa9xyz" && len(payload) >= 4 {
			n := int(binary.BigEndian.Uint16(payload[:2]))
			if n <= len(payload)-4 && budget.reserveString(uint64(n), embeddedAssetMaxStringBytes) == embeddedAssetAccepted {
				value := mp4RecordedLocation(strings.TrimRight(string(payload[4:4+n]), "\x00"))
				if value != "" {
					m.fields = setFieldValue(m.fields, "Recorded_Location", value)
				}
			}
		}
		offset += size
	}
}

func mp4GeneralHasTitle(fields []Field) bool {
	for _, f := range fields {
		if f.Name == "Title" && f.Value != "" {
			return true
		}
	}
	return false
}

// mp4VendorName maps the handler manufacturer, retaining unknown fourCCs.
func mp4VendorName(v uint32) string {
	switch v {
	case 0x33495658:
		return "3ivX"
	case 0x6170706c:
		return "Apple QuickTime"
	case 0x6e696b6f:
		return "Nikon"
	case 0x6f6c796d:
		return "Olympus"
	case 0x6f6d6e65:
		return "Omneon"
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return string(b[:])
}

// applyExtraNames encodes only the appended metadata range. Codec-extra names
// retain their identities; exact collisions also need distinct raw labels.
func (m mp4Metadata) applyExtraNames(builder *canonicalStreamBuilder, start int) {
	if len(m.extras) == 0 {
		return
	}
	stream := builder.store.stream(builder.ref)
	for i := range stream.Fields {
		entry := &stream.Fields[i]
		if firstNonEmpty(entry.StructuredKey, string(entry.Name)) != "extra" || entry.Node == nil || entry.Node.Kind != structuredObject {
			continue
		}
		end := start + len(m.extras)
		if start < 0 || end > len(entry.Node.Object) {
			continue
		}
		raw := *entry.Node
		raw.Object = append([]structuredMember(nil), raw.Object...)
		node := raw
		node.Object = append([]structuredMember(nil), raw.Object...)
		rawNames := make(map[string]struct{}, len(node.Object))
		reserved := make(map[string]struct{}, len(node.Object))
		used := make(map[string]struct{}, len(node.Object))
		for index, member := range node.Object {
			rawNames[member.Key] = struct{}{}
			if index < start || index >= end {
				reserved[member.Key] = struct{}{}
				used[member.Key] = struct{}{}
			}
		}
		for index := start; index < end; index++ {
			name := node.Object[index].Key
			node.Object[index].Key = metadataDynamicJSONName(name, used)
			if _, collision := reserved[name]; collision {
				raw.Object[index].Key = metadataDynamicJSONName(name, rawNames)
			}
		}
		entry.Node = &raw
		entry.StructuredOverride = &node
	}
}
