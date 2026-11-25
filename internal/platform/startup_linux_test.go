//go:build linux

package platform

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeAutostartIsolated(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "Keyfolio 测试 tool")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nprintf '%s\\0' \"$@\" > \"$KEYFOLIO_STARTUP_ARGS_FILE\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	device := filepath.Join(dir, "设备 space % quote\" single'")
	startup := &NativeStartup{Executable: exe, DeviceDir: device, Address: "127.0.0.1:7463", ConfigDir: filepath.Join(dir, "config")}
	if status, err := startup.Status(); err != nil || status.Enabled {
		t.Fatal(status, err)
	}
	if err := startup.Set(true); err != nil {
		t.Fatal(err)
	}
	if status, err := startup.Status(); err != nil || !status.Enabled || !status.Current {
		t.Fatal(status, err)
	}
	raw, err := os.ReadFile(startup.startupPath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("--vault")) || bytes.Contains(raw, []byte("password")) {
		t.Fatal("sensitive/stale args")
	}
	if tool, err := exec.LookPath("desktop-file-validate"); err == nil {
		if output, err := exec.Command(tool, startup.startupPath()).CombinedOutput(); err != nil {
			t.Fatalf("desktop validation: %s %v", output, err)
		}
	}
	if gio, err := exec.LookPath("gio"); err == nil {
		outputFile := filepath.Join(dir, "received-args")
		command := exec.Command(gio, "launch", startup.startupPath())
		command.Env = append(os.Environ(), "KEYFOLIO_STARTUP_ARGS_FILE="+outputFile)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("gio launch: %s %v", output, err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if content, e := os.ReadFile(outputFile); e == nil && len(content) > 0 {
				raw = content
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		expected := strings.Join([]string{"--device-dir", device, "--listen", "127.0.0.1:7463", ""}, "\x00")
		if string(raw) != expected {
			t.Fatalf("Exec arguments changed: %q vs %q", raw, expected)
		}
	}
	// Fedora's real generator runs against only the disposable config tree.
	generator := "/usr/lib/systemd/user-generators/systemd-xdg-autostart-generator"
	if _, err := os.Stat(generator); err == nil {
		for _, name := range []string{"normal", "early", "late", "empty"} {
			if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
				t.Fatal(err)
			}
		}
		command := exec.Command(generator, filepath.Join(dir, "normal"), filepath.Join(dir, "early"), filepath.Join(dir, "late"))
		command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+startup.ConfigDir, "XDG_CONFIG_DIRS="+filepath.Join(dir, "empty"), "XDG_CURRENT_DESKTOP=GNOME")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("generator: %s %v", output, err)
		}
		units, err := filepath.Glob(filepath.Join(dir, "late", "*.service"))
		if err != nil || len(units) != 1 {
			t.Fatalf("generator skipped valid startup: %s %v", output, err)
		}
		unit, err := os.ReadFile(units[0])
		if err != nil || !bytes.Contains(unit, []byte("--device-dir")) || !bytes.Contains(unit, []byte("%%")) {
			t.Fatal("invalid generated unit", err)
		}
	}
	if err = startup.Set(false); err != nil {
		t.Fatal(err)
	}
	for _, character := range []string{"$", "`", "\\"} {
		incompatible := *startup
		incompatible.DeviceDir = filepath.Join(dir, "device"+character)
		if err = incompatible.Set(true); err == nil {
			t.Fatal("incompatible desktop quoting accepted", character)
		}
	}
	if status, err := startup.Status(); err != nil || status.Enabled {
		t.Fatal(status, err)
	}
	if err = startup.Set(false); err != nil {
		t.Fatal("disable not idempotent", err)
	}
}
