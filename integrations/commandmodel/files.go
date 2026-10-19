package commandmodel

import "os"

func defaultTempDir(pattern string) (string, error) {
	return os.MkdirTemp("", pattern)
}

func defaultRemoveAll(path string) error {
	return os.RemoveAll(path)
}
