package mediainfo

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestAnalyzeFileContextCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := AnalyzeFileContext(ctx, filepath.Join("samples", "sample.mkv"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestAnalyzeFileContextMatchesAnalyzeFile pins that the context wrapper does
// not change analysis output on any supported container.
func TestAnalyzeFileContextMatchesAnalyzeFile(t *testing.T) {
	samples := []string{
		"sample.mkv", "sample.mp4", "sample.ts", "sample.avi", "sample.flac",
		"sample.mp3", "sample.mpg", "sample.ogg", "sample.vob", "sample.wav",
		"sample_ac3.vob", "sample_x265.mkv",
	}
	for _, name := range samples {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("samples", name)
			plain, err := AnalyzeFile(path)
			if err != nil {
				t.Fatalf("AnalyzeFile: %v", err)
			}
			withCtx, err := AnalyzeFileContext(context.Background(), path)
			if err != nil {
				t.Fatalf("AnalyzeFileContext: %v", err)
			}
			plainOut, err := Render([]Report{plain}, OutputJSON)
			if err != nil {
				t.Fatalf("render plain: %v", err)
			}
			ctxOut, err := Render([]Report{withCtx}, OutputJSON)
			if err != nil {
				t.Fatalf("render ctx: %v", err)
			}
			if plainOut != ctxOut {
				t.Fatalf("context analysis diverged from plain analysis for %s", name)
			}
		})
	}
}
