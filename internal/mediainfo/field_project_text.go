package mediainfo

import "sort"

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
		entries := make([]fieldEntry, 0, len(stream.Fields))
		for _, entry := range stream.Fields {
			if entry.Options.ShowText {
				entries = append(entries, entry)
			}
		}
		if !stream.TextAlreadyOrdered {
			sort.SliceStable(entries, func(left, right int) bool {
				leftOrder := textFieldOrder(stream.Kind, firstNonEmpty(entries[left].TextLabel, string(entries[left].Name)))
				rightOrder := textFieldOrder(stream.Kind, firstNonEmpty(entries[right].TextLabel, string(entries[right].Name)))
				if leftOrder != rightOrder {
					return leftOrder < rightOrder
				}
				return entries[left].Sequence < entries[right].Sequence
			})
		}
		fields := make([]Field, 0, len(entries))
		for _, entry := range entries {
			label := firstNonEmpty(entry.TextLabel, string(entry.Name))
			fields = append(fields, Field{Name: label, Value: entry.Value.Text})
		}
		projected.Streams = append(projected.Streams, textStreamProjection{Kind: stream.TextKind, Fields: fields})
	}
	return projected
}
