//go:build !darwin && !windows

package mediainfo

import "os"

func fileTimes(path string) (string, string, string, string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", "", "", "", false
	}
	mod := info.ModTime()
	// No portable birth time via os.FileInfo on non-darwin; fall back to mod time.
	created := mod
	createdUTC := created.UTC().Format("2006-01-02 15:04:05 MST")
	createdLocal := created.Local().Format("2006-01-02 15:04:05")
	modUTC := mod.UTC().Format("2006-01-02 15:04:05 MST")
	modLocal := mod.Local().Format("2006-01-02 15:04:05")
	return createdUTC, createdLocal, modUTC, modLocal, true
}
