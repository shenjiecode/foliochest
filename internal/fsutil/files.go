// Package fsutil provides bounded reads and same-directory temporary writes.
package fsutil

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
)

func ReadBounded(path string, max int64) ([]byte, error) {
	if max < 0 || max == math.MaxInt64 {
		return nil, errors.New("无效文件大小上限")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > max {
		return nil, errors.New("文件类型或大小不受支持")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("文件过大")
	}
	return b, nil
}

// WriteTemp flushes and closes a private file; the caller must publish or remove it.
func WriteTemp(dir, pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// WriteFile replaces a local file without a delete-then-rename fallback.
// Callers needing revision checks or no-overwrite publication must add their own checks.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := WriteTemp(dir, ".pending-*", data)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	SyncDir(dir)
	return nil
}

// SyncDir is best-effort: some supported platforms/filesystems reject directory fsync.
func SyncDir(dir string) {
	if f, err := os.Open(dir); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}
}
