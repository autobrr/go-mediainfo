package mediainfo

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"sort"
	"strings"
)

// orderedJSONMember retains a decoded object's key order and raw JSON value.
type orderedJSONMember struct {
	key   string
	value json.RawMessage
}

// withDynamicJSONExtras returns a shallow copy of rawExtras whose extra object
// contains structured dynamic fields after pre-existing parser extras.
func withDynamicJSONExtras(rawExtras map[string]string, fields []dynamicJSONField) map[string]string {
	if len(fields) == 0 {
		return rawExtras
	}
	out := maps.Clone(rawExtras)
	if out == nil {
		out = map[string]string{}
	}
	out["extra"] = mergeDynamicJSONObject(out["extra"], fields)
	return out
}

// mergeDynamicJSONObject merges string fields without constructing unescaped
// JSON in a container parser. Existing members keep their positions; new
// members retain the supplied deterministic order. Malformed existing JSON is
// discarded rather than copied into the rendered object.
func mergeDynamicJSONObject(raw string, fields []dynamicJSONField) string {
	members, err := parseOrderedJSONObject(raw)
	if err != nil {
		members = nil
	}
	positions := make(map[string]int, len(members))
	for i, member := range members {
		if _, exists := positions[member.key]; !exists {
			positions[member.key] = i
		}
	}
	for _, field := range fields {
		name := field.JSONName
		if name == "" {
			name = field.Name
		}
		if name == "" || field.Value == "" {
			continue
		}
		encoded, _ := json.Marshal(field.Value)
		if pos, exists := positions[name]; exists {
			var current string
			if json.Unmarshal(members[pos].value, &current) == nil {
				if current != field.Value {
					encoded, _ = json.Marshal(current + " / " + field.Value)
				}
				members[pos].value = encoded
			}
			continue
		}
		positions[name] = len(members)
		members = append(members, orderedJSONMember{key: name, value: encoded})
	}
	return renderOrderedJSONObject(members)
}

// parseOrderedJSONObject decodes one JSON object without losing member order or
// the original representation of each value. Empty input represents no members.
func parseOrderedJSONObject(raw string) ([]orderedJSONMember, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, errors.New("dynamic JSON extra is not an object")
	}
	var members []orderedJSONMember
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, errors.New("dynamic JSON extra key is not a string")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		members = append(members, orderedJSONMember{key: key, value: bytes.Clone(value)})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return members, nil
}

// renderOrderedJSONObject serializes members in their supplied order while
// preserving each raw value.
func renderOrderedJSONObject(members []orderedJSONMember) string {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, member := range members {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(renderJSONString(member.key))
		buf.WriteByte(':')
		buf.Write(member.value)
	}
	buf.WriteByte('}')
	return buf.String()
}

// matroskaJSONExtraFieldOrder defines the relative order of schema-backed
// Matroska extra members without constraining dynamic tag placement.
var matroskaJSONExtraFieldOrder = makeJSONFieldOrder(
	"SOURCE", "SOURCE_ID", "OriginalSourceMedium", "Source", "Statistics_Tags_Issue",
	"FromStats_BitRate", "FromStats_Duration", "FromStats_FrameCount", "FromStats_StreamSize",
	"ComplexityIndex", "NumberOfDynamicObjects", "BedChannelCount", "BedChannelConfiguration",
	"bsid", "dialnorm", "compr", "dynrng", "dsurmod", "acmod", "lfeon", "cmixlev", "surmixlev", "mixlevel", "roomtyp",
	"dmixmod", "ltrtcmixlev", "ltrtsurmixlev", "lorocmixlev", "lorosurmixlev", "adconvtyp",
	"dialnorm_Average", "dialnorm_Minimum", "dialnorm_Maximum", "dialnorm_Count",
	"compr_Average", "compr_Minimum", "compr_Maximum", "compr_Count",
	"dynrng_Average", "dynrng_Minimum", "dynrng_Maximum", "dynrng_Count", "MD5_Unencoded",
)

// aviJSONExtraFieldOrder defines MediaInfo's ordering for known AVI extras.
var aviJSONExtraFieldOrder = makeJSONFieldOrder("IsTruncated", "ConformanceErrors")

// orderMatroskaJSONExtra orders known Matroska extras around untouched dynamic
// members and returns malformed input unchanged.
func orderMatroskaJSONExtra(raw string) string {
	return orderJSONExtra(raw, matroskaJSONExtraFieldOrder)
}

// orderJSONExtra reorders only registered members within their existing slots,
// leaving dynamic members in their original positions.
func orderJSONExtra(raw string, order map[string]int) string {
	members, err := parseOrderedJSONObject(raw)
	if err != nil || len(members) < 2 {
		return raw
	}
	known := make([]orderedJSONMember, 0, len(members))
	positions := make([]int, 0, len(members))
	for index, member := range members {
		if _, ok := order[member.key]; ok {
			known = append(known, member)
			positions = append(positions, index)
		}
	}
	sort.SliceStable(known, func(i, j int) bool {
		return order[known[i].key] < order[known[j].key]
	})
	for index, position := range positions {
		members[position] = known[index]
	}
	return renderOrderedJSONObject(members)
}

// withAVIJSONExtraOrder returns a shallow copy with known AVI extra members
// reordered. It returns the original map when no extra object exists.
func withAVIJSONExtraOrder(rawExtras map[string]string) map[string]string {
	if rawExtras["extra"] == "" {
		return rawExtras
	}
	out := maps.Clone(rawExtras)
	out["extra"] = orderJSONExtra(out["extra"], aviJSONExtraFieldOrder)
	return out
}

// withMatroskaJSONExtraOrder returns a shallow copy with known Matroska extra
// members reordered. It returns the original map when no extra object exists.
func withMatroskaJSONExtraOrder(rawExtras map[string]string) map[string]string {
	if rawExtras["extra"] == "" {
		return rawExtras
	}
	out := maps.Clone(rawExtras)
	out["extra"] = orderMatroskaJSONExtra(out["extra"])
	return out
}
