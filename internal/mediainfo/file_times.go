//go:build !darwin

package mediainfo

import "os"

func fileTimes(path string) (string, string, string, string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", "", "", "", false
	}
	mod := info.ModTime()
	// On Linux we don't have a portable creation time (birth time) via os.FileInfo.
	// MediaInfo typically omits created timestamps in this case, but keeps modified time.
	modUTC := mod.UTC().Format("2006-01-02 15:04:05 MST")
	modLocal := mod.Local().Format("2006-01-02 15:04:05")
	return "", "", modUTC, modLocal, true
}
