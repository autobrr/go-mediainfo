package mediainfo

import (
	"encoding/json"
)

type jsonMedia struct {
	Media jsonMediaBody `json:"media"`
}

type jsonMediaBody struct {
	Ref   string              `json:"@ref,omitempty"`
	Track []map[string]string `json:"track"`
}

func RenderJSON(reports []Report) string {
	if len(reports) == 1 {
		return renderJSONSingle(reports[0])
	}

	tracks := make([]map[string]string, 0)
	for _, report := range reports {
		entry := map[string]string{"@type": "General", "@ref": report.Ref}
		for _, field := range report.General.Fields {
			entry[field.Name] = field.Value
		}
		tracks = append(tracks, entry)
	}

	payload := jsonMedia{Media: jsonMediaBody{Track: tracks}}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data)
}

func renderJSONSingle(report Report) string {
	tracks := make([]map[string]string, 0, len(report.Streams)+1)
	tracks = append(tracks, streamToJSON(report.General))
	for _, stream := range report.Streams {
		tracks = append(tracks, streamToJSON(stream))
	}
	payload := jsonMedia{Media: jsonMediaBody{Ref: report.Ref, Track: tracks}}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data)
}

func streamToJSON(stream Stream) map[string]string {
	entry := map[string]string{"@type": string(stream.Kind)}
	for _, field := range stream.Fields {
		entry[field.Name] = field.Value
	}
	return entry
}
