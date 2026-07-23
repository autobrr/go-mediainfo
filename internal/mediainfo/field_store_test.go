package mediainfo

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestFieldStoreFillModesClearAndGet(t *testing.T) {
	store := &fieldStore{}
	ref := store.Prepare(StreamGeneral)
	store.Fill(ref, "Format", "Matroska", fillReplace)
	store.Fill(ref, "Format", "ignored", fillFirstNonEmpty)
	store.Fill(ref, "Tag", "one", fillReplace)
	store.Fill(ref, "Tag", "two", fillAppend)

	if got, ok := store.Get(ref, "Format"); !ok || got != "Matroska" {
		t.Fatalf("Format = %q, %v, want Matroska, true", got, ok)
	}
	if got, ok := store.Get(ref, "Tag"); !ok || got != "one / two" {
		t.Fatalf("Tag = %q, %v, want joined value", got, ok)
	}
	store.Clear(ref, "Tag")
	if _, ok := store.Get(ref, "Tag"); ok {
		t.Fatal("Tag remains after Clear")
	}
}

func TestCanonicalProjectionsCanRenderConcurrently(t *testing.T) {
	report, err := AnalyzeFile(filepath.Join("samples", "sample.mkv"))
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}
	renderers := []struct {
		name     string
		render   func() string
		validate func(string) error
		want     string
	}{
		{
			name:   "JSON",
			render: func() string { return RenderJSON([]Report{report}) },
			validate: func(output string) error {
				if !json.Valid([]byte(output)) {
					return fmt.Errorf("invalid JSON")
				}
				return nil
			},
		},
		{
			name:   "XML",
			render: func() string { return RenderXML([]Report{report}) },
			validate: func(output string) error {
				return xml.Unmarshal([]byte(output), &struct{}{})
			},
		},
		{
			name:   "text",
			render: func() string { return RenderText([]Report{report}) },
			validate: func(output string) error {
				if !strings.Contains(output, "Format                                   : Matroska") {
					return fmt.Errorf("missing Matroska format field")
				}
				return nil
			},
		},
	}
	for index := range renderers {
		renderers[index].want = renderers[index].render()
		if err := renderers[index].validate(renderers[index].want); err != nil {
			t.Fatalf("baseline %s output: %v", renderers[index].name, err)
		}
	}

	var wait sync.WaitGroup
	for index := range 12 {
		wait.Go(func() {
			renderer := renderers[index%len(renderers)]
			output := renderer.render()
			if output != renderer.want {
				t.Errorf("concurrent %s output differs from baseline", renderer.name)
			}
			if err := renderer.validate(output); err != nil {
				t.Errorf("concurrent %s output: %v", renderer.name, err)
			}
		})
	}
	wait.Wait()
}

func TestAnalyzedSampleCanonicalStoresValidate(t *testing.T) {
	for _, name := range []string{"sample.mp4", "sample.mkv", "sample.ts", "sample.avi", "sample.mpg", "sample.vob", "sample_ac3.vob", "sample.mp3", "sample.flac", "sample.wav", "sample.ogg"} {
		t.Run(name, func(t *testing.T) {
			report, err := AnalyzeFile(filepath.Join("samples", name))
			if err != nil {
				t.Fatalf("AnalyzeFile: %v", err)
			}
			if err := validateFieldStore(report.General.reportStore); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDirectParsersSeedCanonicalFields(t *testing.T) {
	for _, name := range []string{"sample.mp3", "sample.flac", "sample.wav", "sample.ogg", "sample.mp4", "sample.mpg", "sample.avi", "sample.vob", "sample_ac3.vob", "sample.ts"} {
		t.Run(name, func(t *testing.T) {
			report, err := AnalyzeFile(filepath.Join("samples", name))
			if err != nil {
				t.Fatalf("AnalyzeFile: %v", err)
			}
			if len(report.Streams) == 0 || len(report.Streams[0].canonicalSeed) == 0 {
				t.Fatal("parser did not retain a canonical seed")
			}
			store := report.General.reportStore
			if store == nil || len(store.streams) < 2 {
				t.Fatal("analysis did not attach the canonical store")
			}
			for _, stored := range store.streams[1:] {
				for _, entry := range stored.Fields {
					key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
					if key == "@type" || key == "@typeorder" || key == "StreamOrder" {
						continue
					}
					if entry.Projected {
						t.Fatalf("field %q came from the legacy projection", entry.Name)
					}
				}
			}
		})
	}
}

func TestMP4ContainerFactsUseDirectCanonicalFill(t *testing.T) {
	report, err := AnalyzeFile(filepath.Join("samples", "sample.mp4"))
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}
	if len(report.Streams) == 0 {
		t.Fatal("analysis returned no streams")
	}
	for index, stream := range report.Streams {
		if len(stream.canonicalSeed) == 0 {
			t.Fatalf("stream %d did not retain a canonical seed", index)
		}
		keys := []string{"ID", "Format", "Duration", "StreamSize", "FrameCount"}
		if stream.Kind == StreamVideo {
			keys = append(keys, "FrameRate")
		}
		for _, key := range keys {
			foundDirect := false
			for _, entry := range stream.canonicalSeed {
				structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
				if structuredKey == key && entry.Options.ShowStructured && !entry.Projected {
					foundDirect = true
					break
				}
			}
			if !foundDirect {
				t.Fatalf("stream %d field %q did not use direct canonical fill", index, key)
			}
		}
	}
}

func TestFieldStoreFinalizationAndProjections(t *testing.T) {
	store := &fieldStore{ref: "sample.mkv"}
	ref := store.Prepare(StreamGeneral)
	store.Fill(ref, "Format", "Matroska", fillReplace)
	store.Fill(ref, "Duration", "1234", fillReplace)
	finalizeFieldStore(store)

	if got, ok := store.Get(ref, "Duration/String"); !ok || got != "1 s 234 ms" {
		t.Fatalf("Duration/String = %q, %v", got, ok)
	}
	report := Report{General: Stream{Kind: StreamGeneral, reportStore: store}}
	snapshot := captureLegacyReportState(report, true)
	report.General.reportSnapshot = &snapshot
	textProjection := projectTextReport(report)
	if got := textProjection.Streams[0].Fields; !reflect.DeepEqual(got, []Field{{Name: "Format", Value: "Matroska"}, {Name: "Duration", Value: "1 s 234 ms"}}) {
		t.Fatalf("text fields = %#v", got)
	}
	structuredProjection := projectStructuredReport(report)
	var duration structuredField
	for _, field := range structuredProjection.Streams[0].Fields {
		if field.Key == "Duration" {
			duration = field
		}
	}
	if duration.Key != "Duration" || duration.Value.Text != "1.234" {
		t.Fatalf("structured fields = %#v", structuredProjection.Streams[0].Fields)
	}
}

func TestLegacyReportAdapterRoundTripPreservesPublicRenderState(t *testing.T) {
	report := Report{
		Ref: "sample.mkv",
		General: Stream{
			Kind:    StreamGeneral,
			Fields:  []Field{{Name: "Format", Value: "Matroska"}},
			JSON:    map[string]string{"Format": "Matroska"},
			JSONRaw: map[string]string{"extra": `{"Custom":"value"}`},
		},
		Streams: []Stream{{
			Kind:                   StreamVideo,
			Fields:                 []Field{{Name: "Format", Value: "AVC"}},
			JSON:                   map[string]string{"Format": "AVC"},
			JSONRaw:                map[string]string{"extra": `{"CodecConfigurationBox":"avcC"}`},
			JSONSkipFrameRateRatio: true,
		}},
	}
	beforeJSON := RenderJSON([]Report{report})
	store := legacyReportToFieldStore(report)
	roundTrip := fieldStoreToLegacyReport(store)
	if before, after := captureLegacyReportState(report, false), captureLegacyReportState(roundTrip, false); !reflect.DeepEqual(before, after) {
		t.Fatalf("legacy state changed:\nbefore=%#v\nafter=%#v", before, after)
	}
	if afterJSON := RenderJSON([]Report{roundTrip}); afterJSON != beforeJSON {
		t.Fatalf("JSON changed after round trip:\nbefore=%s\nafter=%s", beforeJSON, afterJSON)
	}
}

func TestCanonicalSnapshotFallsBackAfterLegacyMutation(t *testing.T) {
	report := Report{
		Ref:     "sample.mkv",
		General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}},
	}
	attachCanonicalStore(&report)
	if output := RenderJSON([]Report{report}); !strings.Contains(output, `"Format":"Matroska"`) {
		t.Fatalf("initial JSON = %s", output)
	}
	report.General.Fields[0].Value = "AVI"
	if output := RenderJSON([]Report{report}); !strings.Contains(output, `"Format":"AVI"`) || strings.Contains(output, `"Format":"Matroska"`) {
		t.Fatalf("mutated JSON = %s", output)
	}
}

func TestCanonicalSnapshotPreservesLegacyJSONDeletion(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamGeneral)
	builder.Fill("Format", "Matroska", "Format", "Matroska")
	builder.Fill("Title", "Delete me", "Title", "Delete me")
	builder.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{{
		Key: "Keep", Value: structuredNode{Kind: structuredString, Text: "yes"},
	}}})
	report := Report{Ref: "sample.mkv", General: builder.Snapshot(canonicalStreamPolicy{})}
	attachCanonicalStore(&report)

	delete(report.General.JSON, "Title")
	delete(report.General.JSONRaw, "extra")
	output := RenderJSON([]Report{report})
	if strings.Contains(output, `"Title"`) || strings.Contains(output, `"extra"`) {
		t.Fatalf("deleted compatibility keys were restored: %s", output)
	}
	if !strings.Contains(output, `"Format":"Matroska"`) {
		t.Fatalf("sibling compatibility key was lost: %s", output)
	}
}

// TestReplaceCanonicalSeedFillRestoresXMLVisibility verifies a later display
// fill promotes an earlier JSON-only scalar into the shared XML projection.
func TestReplaceCanonicalSeedFillRestoresXMLVisibility(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.StructuredJSONOnly("BitRate_Nominal", "1000000")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	replaceCanonicalSeedFill(&stream, "BitRate_Nominal", "1000000", "Nominal bit rate", "1 000 kb/s")

	report := Report{
		Ref:     "canonical-xml-visibility.mkv",
		General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}, JSON: map[string]string{"Format": "Matroska"}},
		Streams: []Stream{stream},
	}
	attachCanonicalStore(&report)
	if output := RenderXML([]Report{report}); !strings.Contains(output, "<BitRate_Nominal>1000000</BitRate_Nominal>") {
		t.Fatalf("XML omitted promoted direct scalar: %s", output)
	}
}

// TestCanonicalAdapterPublishesUpdatedScalar verifies canonical parser values
// are materialized only when the public compatibility snapshot is attached.
func TestCanonicalAdapterPublishesUpdatedScalar(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "TrueHD", "Format", "TrueHD")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	replaceCanonicalSeedFill(&stream, "Format", "MLP FBA", "Format", "MLP FBA")
	report := Report{Ref: "scalar.mkv", General: Stream{Kind: StreamGeneral}, Streams: []Stream{stream}}
	attachCanonicalStore(&report)

	if got := report.Streams[0].JSON["Format"]; got != "MLP FBA" {
		t.Fatalf("compatibility Format = %q, want MLP FBA", got)
	}
}

// TestCanonicalAdapterPublishesUpdatedObject verifies canonical node mutation
// is materialized at the public compatibility seam.
func TestCanonicalAdapterPublishesUpdatedObject(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{{
		Key: "First", Value: structuredNode{Kind: structuredString, Text: "one"},
	}}})
	stream := builder.Snapshot(canonicalStreamPolicy{})
	appendCanonicalSeedObjectMembers(&stream, "extra", []structuredMember{{
		Key: "Second", Value: structuredNode{Kind: structuredString, Text: "two"},
	}})
	report := Report{Ref: "node.mkv", General: Stream{Kind: StreamGeneral}, Streams: []Stream{stream}}
	attachCanonicalStore(&report)

	if got := report.Streams[0].JSONRaw["extra"]; got != `{"First":"one","Second":"two"}` {
		t.Fatalf("compatibility extra = %s", got)
	}
}

func TestPartialGeneralCanonicalSeedReplacesLegacyScalarProjection(t *testing.T) {
	general := Stream{
		Kind:   StreamGeneral,
		Fields: []Field{{Name: "Title", Value: "Canonical title"}},
		JSON:   map[string]string{"Title": "Canonical title"},
	}
	replaceCanonicalSeedFill(&general, "Title", "Canonical title", "", "")
	report := Report{Ref: "partial-general.mkv", General: general}
	attachCanonicalStore(&report)

	if output := RenderJSON([]Report{report}); strings.Count(output, `"Title":`) != 1 {
		t.Fatalf("JSON Title count = %d: %s", strings.Count(output, `"Title":`), output)
	}
	if output := RenderXML([]Report{report}); strings.Count(output, "<Title>") != 1 {
		t.Fatalf("XML Title count = %d: %s", strings.Count(output, "<Title>"), output)
	}
}

func TestRefreshCanonicalCompatibilitySnapshotDropsStalePublicValues(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamGeneral)
	builder.DirectStructured("Format", "Matroska")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	stream.JSON["FrameCount"] = "100"

	refreshCanonicalCompatibilitySnapshot(&stream)

	if _, exists := stream.JSON["FrameCount"]; exists {
		t.Fatalf("FrameCount survived canonical deletion: %#v", stream.JSON)
	}
	if got := stream.JSON["Format"]; got != "Matroska" {
		t.Fatalf("unrelated compatibility value = %q, want Matroska", got)
	}
}

func TestCanonicalProjectionPolicySuppressesStreamOrder(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Fill("Format", "AVC", "Format", "AVC")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	omitCanonicalStreamOrder(&stream)
	report := Report{Ref: "policy.mkv", General: Stream{Kind: StreamGeneral}, Streams: []Stream{stream}}

	attachCanonicalStore(&report)

	if output := RenderJSON([]Report{report}); strings.Contains(output, `"StreamOrder"`) {
		t.Fatalf("canonical policy emitted StreamOrder: %s", output)
	}
	if !report.Streams[0].JSONSkipStreamOrder {
		t.Fatal("legacy StreamOrder flag was not published")
	}
}
