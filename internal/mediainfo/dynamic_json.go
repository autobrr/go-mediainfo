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

// aviJSONExtraFieldOrder defines MediaInfo's ordering for known AVI extras.
var aviJSONExtraFieldOrder = makeStructuredFieldOrder("IsTruncated", "ConformanceErrors")

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
