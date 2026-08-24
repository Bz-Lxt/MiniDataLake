package persist

import "os"

func DeleteFile(path string) error {
	if path == "" {
		return nil
	}
	return os.Remove(path)
}
