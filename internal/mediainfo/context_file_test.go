package mediainfo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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
