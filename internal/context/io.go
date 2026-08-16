package context

import (
	"os"
	"path/filepath"
)

// atomicWriteFile writes data to path atomically: write to a temp file, sync,
// then rename. This prevents partial-write corruption on crash.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".automergent-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}