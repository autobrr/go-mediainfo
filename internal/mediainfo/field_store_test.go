package mediainfo

import (
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
	var wait sync.WaitGroup
	for index := range 12 {
		wait.Go(func() {
			switch index % 3 {
			case 0:
				_ = RenderJSON([]Report{report})
			case 1:
				_ = RenderXML([]Report{report})
			case 2:
				_ = RenderText([]Report{report})
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

func TestSmallAudioParsersSeedCanonicalFields(t *testing.T) {
	for _, name := range []string{"sample.mp3", "sample.flac", "sample.wav", "sample.ogg"} {
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
			for _, entry := range store.streams[1].Fields {
				if entry.Projected {
					t.Fatalf("field %q came from the legacy projection", entry.Name)
				}
			}
		})
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
