package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shenjiecode/keyfolio/internal/updater"
)

func TestUpdateHelperWaitsAndVerifiesBeforeLaunch(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("installation targets")
	}
	for _, tamper := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified", true: "tampered"}[tamper], func(t *testing.T) {
			device, stage := t.TempDir(), t.TempDir()
			owner, locked, err := acquireInstance(device)
			if err != nil || !locked {
				t.Fatal(err)
			}
			kind := "rpm"
			if runtime.GOOS == "windows" {
				kind = "exe"
			}
			name, err := updater.Filename("1.0.0", runtime.GOOS, runtime.GOARCH, kind)
			if err != nil {
				owner.close()
				t.Skip(err)
			}
			raw := []byte("synthetic installer, not executable")
			sum := sha256.Sum256(raw)
			path := filepath.Join(stage, name)
			if err = os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			original := filepath.Join(t.TempDir(), "original")
			if err = os.WriteFile(original, []byte("synthetic original"), 0600); err != nil {
				t.Fatal(err)
			}
			plan := updateManifest{Device: device, Original: original, Package: updater.Package{Version: "1.0.0", Name: name, Path: path, Size: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])}}
			manifest := filepath.Join(stage, "install.json")
			data, _ := json.Marshal(plan)
			if err = os.WriteFile(manifest, data, 0600); err != nil {
				t.Fatal(err)
			}
			called := make(chan string, 1)
			done := make(chan error, 1)
			go func() {
				done <- executeUpdatePlan(manifest, filepath.Join(stage, "helper"), func(path string) error { called <- path; return nil }, 2*time.Second)
			}()
			select {
			case <-called:
				t.Fatal("installer launched while old instance owns lock")
			case <-time.After(150 * time.Millisecond):
			}
			if tamper {
				if err = os.WriteFile(path, []byte("tampered"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			owner.close()
			err = <-done
			if tamper {
				if err == nil {
					t.Fatal("tamper accepted")
				}
				select {
				case <-called:
					t.Fatal("tampered file launched")
				default:
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if <-called != path {
					t.Fatal("wrong install path")
				}
			}
			result, err := os.ReadFile(filepath.Join(stage, "result.txt"))
			if err != nil || len(result) == 0 {
				t.Fatal("no handoff outcome", err)
			}
			originalRaw, _ := os.ReadFile(original)
			if strings.TrimSpace(string(originalRaw)) != "synthetic original" {
				t.Fatal("helper modified original")
			}
		})
	}
}
