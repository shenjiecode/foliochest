package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSeparateDirectories(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		vault, device string
		allowed       bool
	}{
		{root, filepath.Join(root, "device"), false}, {root, root, false},
		{filepath.Join(root, "vault"), root, true},
		{filepath.Join(root, "vault"), filepath.Join(root, "device"), true},
		{filepath.Join(root, "a"), filepath.Join(root, "ab"), true},
	} {
		if err := SeparateDirectories(test.vault, test.device); (err == nil) != test.allowed {
			t.Fatalf("%+v: %v", test, err)
		}
	}
	if runtime.GOOS != "windows" {
		real := filepath.Join(root, "real")
		if err := os.Mkdir(real, 0700); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(root, "alias")
		if err := os.Symlink(real, alias); err != nil {
			t.Fatal(err)
		}
		if err := SeparateDirectories(real, filepath.Join(alias, "future", "device")); err == nil {
			t.Fatal("symlink directory bypass")
		}
	}
}
func TestStartupRejectsUnsafeArguments(t *testing.T) {
	for _, path := range []string{"relative", "/tmp/go-build123/b001/exe/keyfolio", "/tmp/line\nbreak", "/tmp/nul\x00byte"} {
		startup := &NativeStartup{DeviceDir: t.TempDir(), Executable: path, Address: "127.0.0.1:7463"}
		if _, err := startup.args(); err == nil {
			t.Fatalf("unsafe path accepted %q", path)
		}
	}
}
