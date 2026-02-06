package mediainfo

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type continuousFileSet struct {
	LastPath  string
	TotalSize int64
	LastSize  int64
	Count     int
}

func detectContinuousFileSet(path string) (continuousFileSet, bool) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// trailing digits
	i := len(name)
	for i > 0 {
		c := name[i-1]
		if c < '0' || c > '9' {
			break
		}
		i--
	}
	if i == len(name) {
		return continuousFileSet{}, false
	}
	prefix := name[:i]
	digits := name[i:]
	width := len(digits)
	start, err := strconv.Atoi(digits)
	if err != nil {
		return continuousFileSet{}, false
	}

	// Require at least one following file to match MediaInfo's default behavior.
	next := filepath.Join(dir, fmt.Sprintf("%s%0*d%s", prefix, width, start+1, ext))
	if _, err := os.Stat(next); err != nil {
		return continuousFileSet{}, false
	}

	var total int64
	var last string
	var lastSize int64
	count := 0
	for n := start; n < start+10_000; n++ {
		p := filepath.Join(dir, fmt.Sprintf("%s%0*d%s", prefix, width, n, ext))
		st, err := os.Stat(p)
		if err != nil {
			break
		}
		total += st.Size()
		last = p
		lastSize = st.Size()
		count++
	}
	if count < 2 || last == "" || last == path {
		return continuousFileSet{}, false
	}
	return continuousFileSet{LastPath: last, TotalSize: total, LastSize: lastSize, Count: count}, true
}
