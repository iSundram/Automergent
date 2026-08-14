package installer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	updateLockStaleThreshold = 30 * 24 * time.Hour
	updateLockMaxWait        = 15 * time.Second
	updateLockInitialBackoff = 200 * time.Millisecond
	updateLockMaxBackoff     = 2 * time.Second
)

// UpdateLock prevents concurrent update/rollback operations.
type UpdateLock struct {
	lockPath string
	lockFile *os.File
}

// AcquireUpdateLock acquires a filesystem lock for update operations.
func AcquireUpdateLock() (*UpdateLock, error) {
	configDir, err := automergentConfigDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}
	return acquireUpdateLockWithParams(
		filepath.Join(configDir, "update.lock"),
		updateLockStaleThreshold,
		updateLockMaxWait,
		updateLockInitialBackoff,
	)
}

func acquireUpdateLockWithParams(lockPath string, staleThreshold, maxWait, initialBackoff time.Duration) (*UpdateLock, error) {
	deadline := time.Now().Add(maxWait)
	backoff := initialBackoff
	if backoff <= 0 {
		backoff = updateLockInitialBackoff
	}

	for {
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, _ = fmt.Fprintf(lockFile, "pid=%d acquired=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
			_ = lockFile.Sync()
			return &UpdateLock{lockPath: lockPath, lockFile: lockFile}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}

		info, statErr := os.Stat(lockPath)
		if statErr == nil && time.Since(info.ModTime()) > staleThreshold {
			_ = os.Remove(lockPath)
			continue
		}
		if statErr != nil && errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another update appears to be in progress")
		}

		time.Sleep(backoff)
		backoff *= 2
		if backoff > updateLockMaxBackoff {
			backoff = updateLockMaxBackoff
		}
	}
}

// Release releases an acquired update lock.
func (l *UpdateLock) Release() error {
	if l == nil {
		return nil
	}
	if l.lockFile != nil {
		if err := l.lockFile.Close(); err != nil {
			return err
		}
	}
	if err := os.Remove(l.lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func automergentConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".automergent"), nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%d.%d", filepath.Base(path), os.Getpid(), time.Now().UnixNano()))
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	writeErr := error(nil)
	if _, err := f.Write(data); err != nil {
		writeErr = err
	} else if err := f.Sync(); err != nil {
		writeErr = err
	}
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}

	if err := replaceFileWithTemp(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func replaceFileWithTemp(tempPath, targetPath string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(tempPath, targetPath)
	}

	if _, err := os.Stat(targetPath); errors.Is(err, os.ErrNotExist) {
		return os.Rename(tempPath, targetPath)
	}

	backupPath := fmt.Sprintf("%s.bak.%d", targetPath, time.Now().UnixNano())
	if err := os.Rename(targetPath, backupPath); err != nil {
		return err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Rename(backupPath, targetPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func installBinaryFromSource(sourceBinary, targetBinary string) error {
	stagePath := fmt.Sprintf("%s.stage.%d.%d", targetBinary, os.Getpid(), time.Now().UnixNano())
	if err := copyFile(sourceBinary, stagePath); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(stagePath, 0755); err != nil {
			_ = os.Remove(stagePath)
			return err
		}
	}

	if runtime.GOOS != "windows" {
		if err := os.Rename(stagePath, targetBinary); err != nil {
			_ = os.Remove(stagePath)
			return err
		}
		return nil
	}

	if _, err := os.Stat(targetBinary); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(stagePath)
		return err
	}
	if _, err := os.Stat(targetBinary); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(stagePath, targetBinary); err != nil {
			_ = os.Remove(stagePath)
			return err
		}
		return nil
	}

	oldPath := fmt.Sprintf("%s.old.%d", targetBinary, time.Now().UnixNano())
	if err := os.Rename(targetBinary, oldPath); err != nil {
		_ = os.Remove(stagePath)
		return err
	}
	if err := os.Rename(stagePath, targetBinary); err != nil {
		_ = os.Rename(oldPath, targetBinary)
		_ = os.Remove(stagePath)
		return err
	}
	_ = os.Remove(oldPath)
	return nil
}
