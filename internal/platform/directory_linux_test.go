//go:build linux

package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeDirectoryHelperIsolated(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	selected := filepath.Join(root, "资料 % --folder")
	for _, p := range []string{bin, selected, filepath.Join(root, "home")} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	// The real library is exercised against a fake executable in a private PATH.
	// Never open a desktop dialog or inspect the developer's actual home.
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FOLIOCHEST_TEST_ARGS\"\nif [ \"$FOLIOCHEST_TEST_MODE\" = cancel ]; then exit 1; fi\nprintf '%s\\n' \"$FOLIOCHEST_TEST_FOLDER\"\n"
	for _, name := range []string{"qarma", "zenity", "matedialog"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"select", "cancel"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNativeDirectoryHelperProcess$")
		argsFile := filepath.Join(root, "args")
		cmd.Env = []string{"PATH=" + bin, "HOME=" + filepath.Join(root, "home"), "XDG_CONFIG_HOME=" + root, "XDG_DATA_HOME=" + root, "XDG_CACHE_HOME=" + root, "XDG_RUNTIME_DIR=" + root, "DBUS_SESSION_BUS_ADDRESS=unix:path=" + filepath.Join(root, "no-bus"), "DISPLAY=:synthetic", "WAYLAND_DISPLAY=", "FOLIOCHEST_DIRECTORY_HELPER_TEST=1", "FOLIOCHEST_TEST_FOLDER=" + selected, "FOLIOCHEST_TEST_ARGS=" + argsFile, "FOLIOCHEST_TEST_MODE=" + mode}
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("%s: %v %s", mode, err, out)
		}
		raw, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{"--file-selection\n", "--directory\n", selected + string(os.PathSeparator) + "\n"} {
			if !strings.Contains(string(raw), expected) {
				t.Fatalf("missing arg %q in %q", expected, raw)
			}
		}
	}
}
func TestNativeDirectoryHelperProcess(t *testing.T) {
	if os.Getenv("FOLIOCHEST_DIRECTORY_HELPER_TEST") != "1" {
		return
	}
	folder := os.Getenv("FOLIOCHEST_TEST_FOLDER")
	result, err := PickDirectory(context.Background(), folder)
	if os.Getenv("FOLIOCHEST_TEST_MODE") == "cancel" {
		if !errors.Is(err, ErrDirectoryCanceled) || result != "" {
			t.Fatal(result, err)
		}
	} else if err != nil || result != folder {
		t.Fatal(result, err)
	}
}
