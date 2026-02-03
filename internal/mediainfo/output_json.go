package mediainfo

import (
	"bytes"
	"encoding/json"
)

type jsonMediaOut struct {
	Ref    string
	Tracks []jsonTrackOut
}

type jsonTrackOut struct {
	Fields []jsonKV
}

func RenderJSON(reports []Report) string {
	if len(reports) == 1 {
		return renderJSONPayload(buildJSONPayload(reports[0]))
	}
	payloads := make([]jsonPayloadOut, 0, len(reports))
	for _, report := range reports {
		payloads = append(payloads, buildJSONPayload(report))
	}
	return renderJSONPayloads(payloads)
}

type jsonPayloadOut struct {
	CreatingLibrary []jsonKV
	Media           jsonMediaOut
}

func buildJSONPayload(report Report) jsonPayloadOut {
	return jsonPayloadOut{
		CreatingLibrary: jsonCreatingLibraryFields(),
		Media:           buildJSONMedia(report),
	}
}

func jsonCreatingLibraryFields() []jsonKV {
	return []jsonKV{
		{Key: "name", Val: "MediaInfoLib"},
		{Key: "version", Val: MediaInfoLibVersion},
		{Key: "url", Val: MediaInfoLibURL},
	}
}

func renderJSONPayload(payload jsonPayloadOut) string {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	writeJSONField(&buf, "creatingLibrary", renderJSONObject(payload.CreatingLibrary), true)
	buf.WriteString(",\n")
	writeJSONField(&buf, "media", renderJSONMedia(payload.Media), true)
	buf.WriteString("\n}")
	return buf.String()
}

func renderJSONPayloads(payloads []jsonPayloadOut) string {
	var buf bytes.Buffer
	buf.WriteString("[\n")
	for i, payload := range payloads {
		if i > 0 {
			buf.WriteString(",\n")
		}
		buf.WriteString(renderJSONPayload(payload))
	}
	buf.WriteString("\n]")
	return buf.String()
}

func renderJSONMedia(media jsonMediaOut) string {
	fields := []jsonKV{{Key: "@ref", Val: media.Ref}}
	tracks := make([]string, 0, len(media.Tracks))
	for _, track := range media.Tracks {
		tracks = append(tracks, renderJSONObject(track.Fields))
	}
	fields = append(fields, jsonKV{Key: "track", Val: renderJSONArray(tracks), Raw: true})
	return renderJSONObject(fields)
}

func renderJSONObject(fields []jsonKV) string {
	var buf bytes.Buffer
	buf.WriteString("{")
	for i, field := range fields {
		if i > 0 {
			buf.WriteString(",\n")
		}
		writeJSONField(&buf, field.Key, field.Val, field.Raw)
	}
	buf.WriteString("}")
	return buf.String()
}

func renderJSONArray(items []string) string {
	var buf bytes.Buffer
	buf.WriteString("[")
	for i, item := range items {
		if i > 0 {
			buf.WriteString(",\n")
		}
		buf.WriteString(item)
	}
	buf.WriteString("]")
	return buf.String()
}

func writeJSONField(buf *bytes.Buffer, key, value string, raw bool) {
	buf.WriteString("\"")
	buf.WriteString(key)
	buf.WriteString("\":")
	if raw {
		buf.WriteString(value)
		return
	}
	buf.WriteString(renderJSONString(value))
}

func renderJSONString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
