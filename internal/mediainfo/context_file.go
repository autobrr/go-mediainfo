package mediainfo

import (
	"context"
	"os"
)

// contextFile wraps the input file so every read observes the context first.
// It stops the analysis from issuing further reads after cancellation; a read
// already blocked in the OS still returns only when the OS completes it.
type contextFile struct {
	ctx context.Context //nolint:containedctx // io.ReaderAt has no ctx param; adapter lives one analysis
	f   *os.File
}

func (c *contextFile) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.f.Read(p)
}

func (c *contextFile) ReadAt(p []byte, off int64) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.f.ReadAt(p, off)
}

func (c *contextFile) Seek(offset int64, whence int) (int64, error) {
	return c.f.Seek(offset, whence)
}

func (c *contextFile) Stat() (os.FileInfo, error) {
	return c.f.Stat()
}

func (c *contextFile) Close() error {
	return c.f.Close()
}
