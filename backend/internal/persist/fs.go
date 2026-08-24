package persist

import "os"

// DeleteFile removes the file at the given path. A missing or empty path is a
// no-op so that callers don't need to guard against those cases.
func DeleteFile(path string) error {
	if path == "" {
		return nil
	}
	return os.Remove(path)
}

// MDLExists reports whether the given MDL path refers to an existing regular
// file (not a directory) that is large enough to hold an MDL header plus
// trailer.  It is used by the content-deduplication path to ensure that a
// manifest record pointing at a broken or stale MDL path (for example an
// empty directory left behind by a volume-recovery script) is not treated as
// a reusable table.
func MDLExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	if st.IsDir() {
		return false
	}
	return st.Size() >= int64(HeaderN+12)
}
