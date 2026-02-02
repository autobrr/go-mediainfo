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

type jsonMediaList struct {
	Media []jsonMediaBody `json:"media"`
}

func RenderJSON(reports []Report) string {
	if len(reports) == 1 {
		return renderJSONSingle(reports[0])
	}

	media := make([]jsonMediaBody, 0, len(reports))
	for _, report := range reports {
		media = append(media, buildJSONMedia(report))
	}
	payload := jsonMediaList{Media: media}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data)
}

func renderJSONSingle(report Report) string {
	payload := jsonMedia{Media: buildJSONMedia(report)}
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

func buildJSONMedia(report Report) jsonMediaBody {
	tracks := make([]map[string]string, 0, len(report.Streams)+1)
	general := streamToJSON(report.General)
	if report.Ref != "" {
		general["@ref"] = report.Ref
	}
	tracks = append(tracks, general)
	for _, stream := range orderTracks(report.Streams) {
		tracks = append(tracks, orderedJSONTrack(stream))
	}
	return jsonMediaBody{Ref: report.Ref, Track: tracks}
}
