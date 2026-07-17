package mediainfo

import (
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// finalizeFieldStore derives report facts and text siblings exactly once per store.
func finalizeFieldStore(store *fieldStore) {
	if store == nil || store.finalized {
		return
	}
	store.finalized = true
	finalizeGeneratedReportFields(store)
	for streamIndex := range store.streams {
		stream := &store.streams[streamIndex]
		originalCount := len(stream.Fields)
		for index := range originalCount {
			entry := stream.Fields[index]
			spec, known := lookupFieldSpec(stream.Kind, entry.Name)
			if !entry.DeriveString || !known || spec.StringSibling == "" || hasStoredField(stream, spec.StringSibling) {
				continue
			}
			display := formatCanonicalDisplay(spec.Measure, entry.Value.Text)
			if display == "" {
				continue
			}
			siblingSpec, _ := lookupFieldSpec(stream.Kind, spec.StringSibling)
			store.appendEntry(stream, fieldEntry{
				Name:      spec.StringSibling,
				Value:     fieldValue{Text: display},
				Options:   siblingSpec.Options,
				TextLabel: siblingSpec.TextLabel,
			})
		}
	}
}

// finalizeGeneratedReportFields adds cross-stream counts, file metadata, and stream ordering.
func finalizeGeneratedReportFields(store *fieldStore) {
	if store.legacyGeneral != nil {
		return
	}
	counts := make(map[StreamKind]int)
	var generalRef streamRef = -1
	for index, stream := range store.streams {
		if stream.Kind == StreamGeneral && generalRef < 0 {
			generalRef = streamRef(index)
			continue
		}
		counts[stream.Kind]++
	}
	if generalRef >= 0 {
		fillGeneratedStructured(store, generalRef, "@type", string(StreamGeneral))
		for _, count := range []struct {
			kind StreamKind
			name fieldName
		}{
			{StreamVideo, "VideoCount"},
			{StreamAudio, "AudioCount"},
			{StreamText, "TextCount"},
			{StreamImage, "ImageCount"},
			{StreamMenu, "MenuCount"},
		} {
			if counts[count.kind] > 0 {
				fillGeneratedStructured(store, generalRef, count.name, strconv.Itoa(counts[count.kind]))
			}
		}
		if store.ref != "" {
			if extension := strings.TrimPrefix(filepath.Ext(store.ref), "."); extension != "" {
				fillGeneratedStructured(store, generalRef, "FileExtension", extension)
			}
			if size := fileSizeBytes(store.ref); size > 0 {
				store.Fill(generalRef, "FileSize", strconv.FormatInt(size, 10), fillFirstNonEmpty)
			}
			if createdUTC, createdLocal, modifiedUTC, modifiedLocal, ok := fileTimes(store.ref); ok {
				if createdUTC != "" {
					fillGeneratedStructured(store, generalRef, "File_Created_Date", createdUTC)
				}
				if createdLocal != "" {
					fillGeneratedStructured(store, generalRef, "File_Created_Date_Local", createdLocal)
				}
				fillGeneratedStructured(store, generalRef, "File_Modified_Date", modifiedUTC)
				fillGeneratedStructured(store, generalRef, "File_Modified_Date_Local", modifiedLocal)
			}
		}
	}

	indexes := make([]int, 0, len(store.streams))
	for index, stream := range store.streams {
		if stream.Kind != StreamGeneral {
			indexes = append(indexes, index)
		}
	}
	sort.SliceStable(indexes, func(left, right int) bool {
		return streamKindOrder[store.streams[indexes[left]].Kind] < streamKindOrder[store.streams[indexes[right]].Kind]
	})
	kindIndexes := make(map[StreamKind]int)
	for order, index := range indexes {
		stream := &store.streams[index]
		stream.StructuredSequence = order + 1
		kindIndexes[stream.Kind]++
		stream.TypeOrder = 0
		fillGeneratedStructured(store, streamRef(index), "@type", string(stream.Kind))
		if counts[stream.Kind] > 1 {
			stream.TypeOrder = kindIndexes[stream.Kind]
			fillGeneratedStructured(store, streamRef(index), "@typeorder", strconv.Itoa(stream.TypeOrder))
			if stream.HideTypeOrderXML {
				setStoredStructuredXMLVisibility(stream, "@typeorder", false)
			}
		}
		if !stream.SkipStreamOrder {
			fillGeneratedStructured(store, streamRef(index), "StreamOrder", strconv.Itoa(order))
		}
		finalizeComputedStreamFacts(store, streamRef(index))
	}
}

// setStoredStructuredXMLVisibility changes XML visibility for one generated or
// parser-owned structured field without affecting its JSON projection.
func setStoredStructuredXMLVisibility(stream *storedStream, name fieldName, visible bool) {
	if stream == nil || name == "" {
		return
	}
	for index := range stream.Fields {
		entry := &stream.Fields[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if key == string(name) {
			entry.Options.ShowXML = visible
		}
	}
}

// fillGeneratedStructured adds a structured field unless ref already contains name.
func fillGeneratedStructured(store *fieldStore, ref streamRef, name fieldName, value string) {
	stream := store.stream(ref)
	if stream == nil || hasStoredField(stream, name) {
		return
	}
	spec, known := structuredFieldSpec(stream.Kind, string(name))
	store.appendEntry(stream, fieldEntry{
		Name:          name,
		Value:         fieldValue{Text: value},
		Dynamic:       !known,
		Options:       fieldOptions{ShowStructured: true, ShowXML: true, ValueType: spec.Options.ValueType},
		StructuredKey: string(name),
	})
}

// finalizeComputedStreamFacts derives shared video counts, sampled dimensions, and pixel ratio.
func finalizeComputedStreamFacts(store *fieldStore, ref streamRef) {
	stream := store.stream(ref)
	if stream == nil || stream.Kind != StreamVideo || stream.SkipComputed {
		return
	}
	if !hasStoredField(stream, "FrameCount") {
		durationMilliseconds, durationOK := store.Get(ref, "Duration")
		frameRateText, frameRateOK := store.Get(ref, "FrameRate")
		if durationOK && frameRateOK {
			duration, durationErr := strconv.ParseFloat(durationMilliseconds, 64)
			frameRate, frameRateErr := strconv.ParseFloat(frameRateText, 64)
			if durationErr == nil && frameRateErr == nil && duration > 0 && frameRate > 0 {
				fillGeneratedStructured(store, ref, "FrameCount", strconv.FormatInt(int64(math.Round(duration/1000*frameRate)), 10))
			}
		}
	}
	widthText, widthOK := store.Get(ref, "Width")
	heightText, heightOK := store.Get(ref, "Height")
	if !widthOK || !heightOK {
		return
	}
	width, widthErr := strconv.ParseFloat(widthText, 64)
	height, heightErr := strconv.ParseFloat(heightText, 64)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return
	}
	fillGeneratedStructured(store, ref, "Sampled_Width", trimFloat(width))
	fillGeneratedStructured(store, ref, "Sampled_Height", trimFloat(height))
	if hasStoredField(stream, "PixelAspectRatio") {
		return
	}
	displayAspectText, ok := store.Get(ref, "DisplayAspectRatio")
	if !ok {
		return
	}
	displayAspect, err := strconv.ParseFloat(displayAspectText, 64)
	if err == nil && displayAspect > 0 {
		fillGeneratedStructured(store, ref, "PixelAspectRatio", formatJSONFloat(displayAspect/(width/height)))
	}
}

// hasStoredField reports whether stream contains name.
func hasStoredField(stream *storedStream, name fieldName) bool {
	for _, entry := range stream.Fields {
		if entry.Name == name {
			return true
		}
	}
	return false
}

// formatCanonicalDisplay renders a canonical base-unit value for text output.
func formatCanonicalDisplay(measure fieldMeasure, value string) string {
	switch measure {
	case fieldMeasureNone:
		return ""
	case fieldMeasureMilliseconds:
		milliseconds, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return formatDuration(milliseconds / 1000)
		}
	case fieldMeasureBytes:
		bytes, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return formatBytes(bytes)
		}
	case fieldMeasureBitsPerSecond:
		bits, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return formatBitrate(bits)
		}
	case fieldMeasureHertz:
		hertz, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return formatSampleRate(hertz)
		}
	case fieldMeasureChannels:
		channels, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			return formatChannels(channels)
		}
	case fieldMeasurePixels:
		pixels, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			return formatPixels(pixels)
		}
	case fieldMeasureBits:
		bits, err := strconv.ParseUint(value, 10, 8)
		if err == nil {
			return formatBitDepth(uint8(bits))
		}
	}
	return ""
}
