package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shenjiecode/keyfolio/internal/updater"
)

func syntheticPrepared(t *testing.T, b *testBrowser) *preparedUpdate {
	t.Helper()
	raw := []byte("synthetic installer never execute")
	hash := sha256.Sum256(raw)
	path := filepath.Join(t.TempDir(), "synthetic.rpm")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return &preparedUpdate{ID: "synthetic-ready", Generation: b.app.Vault.Generation(), Package: updater.Package{Name: filepath.Base(path), Path: path, Size: int64(len(raw)), SHA256: hex.EncodeToString(hash[:]), Version: "1.0.0"}}
}
func TestInstallUpdateAuthConfirmationAndHandoff(t *testing.T) {
	b := setupHTTP(t)
	ready := syntheticPrepared(t, b)
	b.app.preparedUpdate = ready
	launched, stopped := false, false
	b.app.InstallUpdate = func(p updater.Package) error {
		if !b.app.Vault.Locked() || p.Path != ready.Package.Path {
			t.Error("handoff before lock or wrong file")
		}
		launched = true
		return nil
	}
	b.app.Shutdown = func() { stopped = true }
	fresh := openBrowser(t, b.app, b.url, nil)
	if code, _ := fresh.request("POST", "/api/app/install-update", map[string]any{"id": ready.ID, "confirmed": true}, fresh.csrf); code != 401 {
		t.Fatal(code)
	}
	for _, tc := range []struct {
		id        string
		confirmed bool
		csrf      string
		want      int
	}{
		{ready.ID, true, "wrong", 403}, {ready.ID, false, b.csrf, 409}, {"forged", true, b.csrf, 409},
	} {
		code, _ := b.request("POST", "/api/app/install-update", map[string]any{"id": tc.id, "confirmed": tc.confirmed}, tc.csrf)
		if code != tc.want || launched || stopped {
			t.Fatal("unsafe handoff", code)
		}
	}
	code, data := b.request("POST", "/api/app/install-update", map[string]any{"id": ready.ID, "confirmed": true}, b.csrf)
	if code != 200 || !launched || !stopped || b.app.preparedUpdate != nil {
		t.Fatal(code, string(data), launched, stopped)
	}
}
func TestInstallUpdateRejectsTamperStaleAndLaunchFailure(t *testing.T) {
	for _, scenario := range []string{"tamper", "stale", "launch-failure"} {
		t.Run(scenario, func(t *testing.T) {
			b := setupHTTP(t)
			ready := syntheticPrepared(t, b)
			b.app.preparedUpdate = ready
			called, stopped := false, false
			b.app.InstallUpdate = func(updater.Package) error { called = true; return errors.New("synthetic launch failure") }
			b.app.Shutdown = func() { stopped = true }
			switch scenario {
			case "tamper":
				if err := os.WriteFile(ready.Package.Path, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "stale":
				ready.Generation++
			}
			code, _ := b.request("POST", "/api/app/install-update", map[string]any{"id": ready.ID, "confirmed": true}, b.csrf)
			if scenario == "launch-failure" {
				if code != 500 || !called || !b.app.Vault.Locked() {
					t.Fatal(code, called)
				}
			} else if code != 409 || called {
				t.Fatal(code, called)
			}
			if stopped {
				t.Fatal("failed handoff stopped service")
			}
		})
	}
}
func TestDownloadUpdateRequiresConfirmationAndHuman(t *testing.T) {
	b := setupHTTP(t)
	if code, _ := b.request("POST", "/api/app/download-update", map[string]any{"version": "1.0.0"}, b.csrf); code != 400 {
		t.Fatal(code)
	}
	fresh := openBrowser(t, b.app, b.url, nil)
	if code, _ := fresh.request("POST", "/api/app/download-update", map[string]any{"version": "1.0.0", "confirmed": true}, fresh.csrf); code != 401 {
		t.Fatal(code)
	}
	if code, _ := b.request("POST", "/api/app/download-update", map[string]any{"version": "1.0.0", "confirmed": true}, b.csrf); code != 409 {
		t.Fatal("unsupported launcher", code)
	}
}
