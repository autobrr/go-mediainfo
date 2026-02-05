package mediainfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err == nil {
		root := findRepoRoot(wd)
		if root != "" {
			_ = os.Chdir(root)
		}
	}

	os.Exit(m.Run())
}

func findRepoRoot(start string) string {
	dir := start
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			return ""
		}
		dir = next
	}
	return ""
}
