package mediainfo

import (
	"bytes"
	"encoding/json"
)

// jsonMediaOut is the legacy renderer's media object assembled from structured fields.
type jsonMediaOut struct {
	Ref    string
	Tracks []jsonTrackOut
}

// jsonTrackOut is one ordered legacy JSON track.
type jsonTrackOut struct {
	Fields []jsonKV
}

// RenderJSON renders reports as MediaInfo-compatible JSON from structured projections.
func RenderJSON(reports []Report) string {
	if len(reports) == 1 {
		return renderJSONPayload(buildJSONPayload(reports[0])) + "\n"
	}
	payloads := make([]jsonPayloadOut, 0, len(reports))
	for _, report := range reports {
		payloads = append(payloads, buildJSONPayload(report))
	}
	return renderJSONPayloads(payloads) + "\n"
}

// jsonPayloadOut contains one top-level JSON payload and library metadata.
type jsonPayloadOut struct {
	CreatingLibrary []jsonKV
	Media           jsonMediaOut
}

func buildJSONPayload(report Report) jsonPayloadOut {
	return jsonPayloadOut{
		CreatingLibrary: jsonCreatingLibraryFields(),
		Media:           buildProjectedJSONMedia(report),
	}
}

// buildProjectedJSONMedia adapts the ordered structured projection to the JSON renderer.
func buildProjectedJSONMedia(report Report) jsonMediaOut {
	projected := projectStructuredReport(report)
	tracks := make([]jsonTrackOut, 0, len(projected.Streams))
	for _, stream := range projected.Streams {
		tracks = append(tracks, jsonTrackOut{Fields: structuredFieldsToJSON(stream.Fields)})
	}
	return jsonMediaOut{Ref: projected.Ref, Tracks: tracks}
}

func jsonCreatingLibraryFields() []jsonKV {
	return []jsonKV{
		{Key: "name", Val: AppName},
		{Key: "version", Val: FormatVersion(AppVersion)},
		{Key: "url", Val: AppURL},
	}
}

func renderJSONPayload(payload jsonPayloadOut) string {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	writeJSONField(&buf, "creatingLibrary", renderJSONObject(payload.CreatingLibrary, false), true)
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
	tracks := make([]string, 0, len(media.Tracks))
	for _, track := range media.Tracks {
		tracks = append(tracks, renderJSONTrack(track.Fields))
	}
	return renderJSONMediaObject(media.Ref, tracks)
}

func renderJSONMediaObject(ref string, tracks []string) string {
	var buf bytes.Buffer
	buf.WriteString("{")
	writeJSONField(&buf, "@ref", ref, false)
	buf.WriteString(",")
	writeJSONField(&buf, "track", renderJSONArray(tracks, false), true)
	buf.WriteString("}")
	return buf.String()
}

func renderJSONArray(items []string, multiline bool) string {
	var buf bytes.Buffer
	buf.WriteString("[")
	for i, item := range items {
		if i > 0 {
			if multiline {
				buf.WriteString(",\n")
			} else {
				buf.WriteString(",")
			}
		}
		buf.WriteString(item)
	}
	buf.WriteString("]")
	return buf.String()
}

func renderJSONTrack(fields []jsonKV) string {
	var buf bytes.Buffer
	buf.WriteString("{")
	inlineCount := 2
	if len(fields) > 2 && fields[1].Key == "@typeorder" && fields[2].Key == "StreamOrder" {
		inlineCount = 3
	}
	if len(fields) > 2 && fields[1].Key == "@typeorder" && fields[2].Key == "ID" {
		inlineCount = 3
	}
	for i, field := range fields {
		if i > 0 {
			if i < inlineCount {
				buf.WriteString(",")
			} else {
				buf.WriteString(",\n")
			}
		}
		writeJSONField(&buf, field.Key, field.Val, field.Raw)
	}
	buf.WriteString("}")
	return buf.String()
}

func renderJSONObject(fields []jsonKV, multiline bool) string {
	var buf bytes.Buffer
	buf.WriteString("{")
	for i, field := range fields {
		if i > 0 {
			if multiline {
				buf.WriteString(",\n")
			} else {
				buf.WriteString(",")
			}
		}
		writeJSONField(&buf, field.Key, field.Val, field.Raw)
	}
	buf.WriteString("}")
	return buf.String()
}

func writeJSONField(buf *bytes.Buffer, key, value string, raw bool) {
	buf.WriteString(renderJSONString(key))
	buf.WriteString(":")
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
