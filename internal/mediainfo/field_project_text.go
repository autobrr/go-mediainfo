package mediainfo

import (
	"sort"
	"strconv"
)

// textStreamProjection contains the text-visible fields for one stream.
type textStreamProjection struct {
	Kind   StreamKind
	Fields []Field
}

// textReportProjection contains text-visible streams in display order.
type textReportProjection struct {
	Ref     string
	Streams []textStreamProjection
}

// projectTextReport projects report through its canonical store for text renderers.
func projectTextReport(report Report) textReportProjection {
	return projectTextStore(canonicalStoreForReport(report), report.Ref)
}

// projectTextStore returns text-visible fields in stable stream and field order.
func projectTextStore(store *fieldStore, fallbackRef string) textReportProjection {
	if store == nil {
		return textReportProjection{Ref: fallbackRef}
	}
	store.projectionMu.RLock()
	defer store.projectionMu.RUnlock()
	version := reportVersion(store)
	totalSize := int64(0)
	for i := range store.streams {
		if store.streams[i].Kind == StreamGeneral {
			totalSize, _ = strconv.ParseInt(rawTextStructuredValues(&store.streams[i])["FileSize"], 10, 64)
		}
	}
	container := rawTextGeneralFormat(store)
	projected := textReportProjection{Ref: store.ref, Streams: make([]textStreamProjection, 0, len(store.streams))}
	streamIndexes := make([]int, len(store.streams))
	for index := range store.streams {
		streamIndexes[index] = index
	}
	sort.SliceStable(streamIndexes, func(left, right int) bool {
		return store.streams[streamIndexes[left]].TextSequence < store.streams[streamIndexes[right]].TextSequence
	})
	for _, streamIndex := range streamIndexes {
		stream := &store.streams[streamIndex]
		values := rawTextStructuredValues(stream)
		entries := make([]fieldEntry, 0, len(stream.Fields))
		seen := make(map[string]bool)
		for _, entry := range stream.Fields {
			if !entry.Options.ShowText {
				continue
			}
			identity := reportFieldIdentity(stream.Kind, string(entry.Name), entry.TextLabel)
			if !friendlyExistingFieldVisible(stream.Kind, identity) {
				continue
			}
			if stream.Kind == StreamVideo && values["Format"] == "Theora" && (identity == "FrameCount" || identity == "PixelAspectRatio") {
				continue
			}
			label := reportFieldLabel(version, identity, firstNonEmpty(entry.TextLabel, string(entry.Name)))
			value := friendlyReportValue(stream.Kind, identity, entry.Value.Text, values, totalSize, version)
			if identity == "Format/Info" {
				_, value = reportFormatPresentation(version, stream.Kind, values)
			}
			if value == "" {
				continue
			}
			entry.Name, entry.TextLabel, entry.Value.Text = fieldName(identity), label, value
			entries = append(entries, entry)
			seen[identity] = true
		}
		appendDisplay := func(identity, value string) {
			if value == "" || seen[identity] || !reportTextVisible(version, stream.Kind, identity) {
				return
			}
			entries = append(entries, fieldEntry{Name: fieldName(identity), TextLabel: reportFieldLabel(version, identity, identity), Value: fieldValue{Text: value}, Sequence: uint32(len(stream.Fields) + len(entries))})
			seen[identity] = true
		}
		for _, identity := range []string{"FrameRate", "FrameRate_Minimum", "FrameRate_Maximum", "BitRate_Maximum", "StreamSize", "TimeCode_Source", "Gop_OpenClosed", "Gop_OpenClosed_FirstFrame", "ScanType_StoreMethod", "ScanOrder", "ScanOrder_Original", "ScanOrder_Stored"} {
			if values[identity] == "" {
				continue
			}
			if identity == "FrameRate" && stream.Kind != StreamAudio && values["Format"] != "Theora" {
				continue
			}
			appendDisplay(identity, friendlyReportValue(stream.Kind, identity, values[identity], values, totalSize, version))
		}
		_, info := reportFormatPresentation(version, stream.Kind, values)
		appendDisplay("Format/Info", info)
		codec := storedCodecDescription(version, container, stream, values)
		appendDisplay("CodecID/Info", codec.Info)
		appendDisplay("CodecID/Hint", codec.Hint)
		sort.SliceStable(entries, func(left, right int) bool {
			leftOrder := reportFieldRank(version, stream.Kind, string(entries[left].Name))
			rightOrder := reportFieldRank(version, stream.Kind, string(entries[right].Name))
			if leftOrder != rightOrder {
				return leftOrder < rightOrder
			}
			return entries[left].Sequence < entries[right].Sequence
		})
		fields := make([]Field, 0, len(entries))
		for _, entry := range entries {
			fields = append(fields, Field{Name: entry.TextLabel, Value: entry.Value.Text})
		}
		projected.Streams = append(projected.Streams, textStreamProjection{Kind: stream.TextKind, Fields: fields})
	}
	return projected
}
