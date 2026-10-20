package mediainfo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type cancelOnCheckContext struct {
	context.Context
	cancelAfter int
	checks      int
	done        chan struct{}
}

func (c *cancelOnCheckContext) Err() error {
	c.checks++
	if c.checks < c.cancelAfter {
		return nil
	}
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	return context.Canceled
}

func (c *cancelOnCheckContext) Done() <-chan struct{} {
	return c.done
}

func TestContextFileStopsReadsAfterCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cf := &contextFile{ctx: ctx, f: f}
	defer cf.Close()

	buf := make([]byte, 4)
	if _, err := cf.Read(buf); err != nil {
		t.Fatalf("read before cancel: %v", err)
	}
	if _, err := cf.ReadAt(buf, 2); err != nil {
		t.Fatalf("readat before cancel: %v", err)
	}
	if _, err := cf.Seek(0, 0); err != nil {
		t.Fatalf("seek before cancel: %v", err)
	}

	cancel()
	if _, err := cf.Seek(0, 0); err != nil {
		t.Fatalf("seek is deliberately not context-gated: %v", err)
	}
	if _, err := cf.Read(buf); !errors.Is(err, context.Canceled) {
		t.Fatalf("read after cancel: %v", err)
	}
	if _, err := cf.ReadAt(buf, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("readat after cancel: %v", err)
	}
}

func TestAnalyzeFileContextChecksTitleVOBReads(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "VIDEO_TS")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, maxSniffBytes)
	copy(data, []byte{0x00, 0x00, 0x01, 0xBA})
	if err := os.WriteFile(filepath.Join(dir, "VTS_01_1.VOB"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := &cancelOnCheckContext{Context: context.Background(), cancelAfter: 3, done: make(chan struct{})}

	_, err := AnalyzeFileWithOptionsContext(ctx, filepath.Join(dir, "VTS_01_1.VOB"), defaultAnalyzeOptions())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AnalyzeFileWithOptionsContext error = %v, want context.Canceled", err)
	}
	if ctx.checks < 4 {
		t.Fatalf("context checked %d times; title VOB reads were not gated", ctx.checks)
	}
}
