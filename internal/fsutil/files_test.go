package fsutil

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := ReadBounded(path, 9); err != nil || string(data) != "synthetic" {
		t.Fatal(string(data), err)
	}
	for _, max := range []int64{8, 0, -1, math.MaxInt64} {
		if _, err := ReadBounded(path, max); err == nil {
			t.Fatalf("accepted limit %d", max)
		}
	}
	if _, err := ReadBounded(dir, 100); err == nil {
		t.Fatal("read a directory")
	}
	if _, err := ReadBounded(filepath.Join(dir, "missing"), 100); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := ReadBounded(empty, 0); err != nil || len(data) != 0 {
		t.Fatal(data, err)
	}
	t.Run("symlink", func(t *testing.T) {
		link := filepath.Join(dir, "link")
		if err := os.Symlink(path, link); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
		if _, err := ReadBounded(link, 100); err == nil {
			t.Fatal("followed symlink")
		}
	})
}

func TestPrivateWritesAndCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "file")
	for _, body := range []string{"synthetic one", "synthetic replacement"} {
		if err := WriteFile(path, []byte(body)); err != nil {
			t.Fatal(err)
		}
		data, err := ReadBounded(path, 100)
		if err != nil || string(data) != body {
			t.Fatal(string(data), err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatal(info.Mode())
		}
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	temp, err := WriteTemp(dir, ".pending-*", []byte("synthetic temporary"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(temp)
	data, err := os.ReadFile(temp)
	if err != nil || !bytes.Equal(data, []byte("synthetic temporary")) {
		t.Fatal(err)
	}
	if filepath.Dir(temp) != dir {
		t.Fatal("temporary file outside destination directory")
	}
	// Rename exercises the closed-file handoff, including on Windows.
	moved := filepath.Join(dir, "moved")
	if err = os.Rename(temp, moved); err != nil {
		t.Fatal(err)
	}
	if _, err = WriteTemp(path, ".pending-*", nil); err == nil {
		t.Fatal("created file inside a regular file")
	}
}

func TestFailedReplacementLeavesDestinationAndNoTemp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "occupied")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "keep")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(target, []byte("replacement")); err == nil {
		t.Fatal("replaced directory")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatal("destination removed", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 || files[0].Name() != "occupied" {
		t.Fatal("temporary file leaked", files, err)
	}
}
