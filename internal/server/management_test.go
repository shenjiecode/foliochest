package server

import (
	"encoding/json"
	"github.com/shenjiecode/keyfolio/internal/vault"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagementHTTPRequiresHumanSessionAndCSRF(t *testing.T) {
	b := setupHTTP(t)
	entry := b.seed()
	rename := map[string]any{"key": entry.Key, "new_key": "renamed", "expected_revision": entry.Revision}
	if code, _ := b.request("POST", "/api/entry/rename", rename, "wrong"); code != 403 {
		t.Fatal(code)
	}
	fresh := openBrowser(t, b.app, b.url, nil)
	if code, _ := fresh.request("POST", "/api/entry/rename", rename, fresh.csrf); code != 401 {
		t.Fatal("new browser changed key", code)
	}
	if code, _ := b.request("GET", "/api/entry/rename", nil, ""); code != 405 {
		t.Fatal(code)
	}
	code, data := b.request("POST", "/api/entry/rename", rename, b.csrf)
	if code != 200 {
		t.Fatal(code, string(data))
	}
	var renamed vault.Summary
	if err := json.Unmarshal(data, &renamed); err != nil {
		t.Fatal(err)
	}
	if renamed.Key != "renamed" {
		t.Fatal(renamed)
	}
	deletion := map[string]any{"key": renamed.Key, "expected_heads": []string{renamed.Revision}}
	if code, _ = b.request("POST", "/api/entry/delete", deletion, "wrong"); code != 403 {
		t.Fatal(code)
	}
	if code, _ = fresh.request("POST", "/api/entry/delete", deletion, fresh.csrf); code != 401 {
		t.Fatal(code)
	}
	code, data = b.request("POST", "/api/entry/delete", deletion, b.csrf)
	if code != 200 {
		t.Fatal(code, string(data))
	}
	var deleted vault.Summary
	_ = json.Unmarshal(data, &deleted)
	code, data = b.request("GET", "/api/entries", nil, "")
	if code != 200 || string(data) != "[]\n" {
		t.Fatal("deleted entry exposed", code, string(data))
	}
	code, data = b.request("GET", "/api/entries?scope=trash", nil, "")
	if code != 200 {
		t.Fatal(code)
	}
	var trash []vault.Summary
	_ = json.Unmarshal(data, &trash)
	if len(trash) != 1 || trash[0].Key != "renamed" {
		t.Fatal(trash)
	}
	restore := map[string]any{"key": "renamed", "expected_heads": []string{deleted.Revision}, "source_revision": entry.Revision}
	code, data = b.request("POST", "/api/history/restore", restore, b.csrf)
	if code != 200 {
		t.Fatal(code, string(data))
	}
	current, err := b.app.Vault.Get("renamed")
	if err != nil || current.Value != "SYNTHETIC_PRIVATE_VALUE" {
		t.Fatal(err)
	}
}
func TestUpgradeLegacyHTTPAndRevokeEnrollment(t *testing.T) {
	b, credentials := setupDeviceHTTP(t)
	root := t.TempDir()
	err := filepath.WalkDir("testdata/v1", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("testdata/v1", path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = b.app.Vault.SelectDirectory(root); err != nil {
		t.Fatal(err)
	}
	if code, data := b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf); code != 200 {
		t.Fatal(code, string(data))
	}
	if b.app.Vault.Status().Format != 1 {
		t.Fatal("fixture not format 1")
	}
	enrollBrowser(b, true)
	request, err := b.app.Access.Read("client", "Synthetic", []vault.ReadSpec{{Key: "legacy/synthetic", IncludeValue: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err = b.app.Access.Decide(request.RequestID, true); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"password": "wrong-password", "confirmed": true}
	if code, _ := b.request("POST", "/api/vault/upgrade", body, b.csrf); code == 200 {
		t.Fatal("bad password upgraded")
	}
	if !b.app.device.record.Enabled {
		t.Fatal("bad password disabled enrollment")
	}
	body["password"] = "synthetic-test-password"
	code, data := b.request("POST", "/api/vault/upgrade", body, b.csrf)
	if code != 200 || !b.app.Vault.Locked() || b.app.device.record.Enabled || b.app.device.record.BrowserHash != "" {
		t.Fatal("upgrade did not lock/revoke", code, string(data))
	}
	credentials.mu.Lock()
	remaining := len(credentials.values)
	credentials.mu.Unlock()
	if remaining != 0 {
		t.Fatal("old credentials not deleted")
	}
	if result, err := b.app.Access.Result("client", request.RequestID); err == nil || len(result.Reads) > 0 {
		t.Fatal("upgrade retained approved buffer", err)
	}
	code, _ = b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	current, err := b.app.Vault.Get("legacy/synthetic")
	if err != nil || current.Value != "SYNTHETIC_LEGACY_VALUE" || b.app.Vault.Status().Format != 2 {
		t.Fatal(err)
	}
	code, data = b.request("GET", "/api/state", nil, "")
	if code != 200 || !strings.Contains(string(data), `"format":2`) {
		t.Fatal(code, string(data))
	}
}
func TestUpgradeHTTPRequiresConfirmationAndSession(t *testing.T) {
	b := setupHTTP(t)
	body := map[string]any{"password": "synthetic-test-password", "confirmed": true}
	if code, _ := b.request("POST", "/api/vault/upgrade", body, "wrong"); code != 403 {
		t.Fatal(code)
	}
	fresh := openBrowser(t, b.app, b.url, nil)
	if code, _ := fresh.request("POST", "/api/vault/upgrade", body, fresh.csrf); code != 401 {
		t.Fatal(code)
	}
	body["confirmed"] = false
	if code, _ := b.request("POST", "/api/vault/upgrade", body, b.csrf); code != 400 {
		t.Fatal(code)
	}
	body["confirmed"] = true
	if code, _ := b.request("POST", "/api/vault/upgrade", body, b.csrf); code != 409 {
		t.Fatal("upgraded format 2", code)
	}
}
