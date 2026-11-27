package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shenjiecode/keyfolio/internal/vault"
)

func TestSingleVaultFirstAttachAndLockedSwitchDenied(t *testing.T) {
	root := t.TempDir()
	v := vault.New(root)
	if err := v.Init("synthetic-test-password"); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	b := setupSelectionHTTP(t, "", t.TempDir())
	for _, password := range []string{"wrong", "synthetic-test-password"} {
		code, data := b.request("POST", "/api/vault/select", map[string]string{"directory": root, "password": password}, b.csrf)
		if password == "wrong" {
			if code != 400 || b.app.hasBoundVault() {
				t.Fatalf("failed attach mutated binding: %d %s", code, data)
			}
		} else if code != 200 || b.app.Vault.Locked() {
			t.Fatalf("attach: %d %s", code, data)
		}
	}
	b.app.Access.Lock()
	target := filepath.Join(t.TempDir(), "other")
	for _, path := range []string{"/api/vault/create", "/api/vault/select"} {
		code, _ := b.request("POST", path, map[string]string{"directory": target, "password": "synthetic-test-password"}, b.csrf)
		if code < 400 {
			t.Fatal("locked binding bypass", path, code)
		}
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("denied create wrote files")
	}
	restarted := setupSelectionHTTP(t, root, b.app.deviceDir)
	if restarted.app.boundID() != b.app.boundID() {
		t.Fatal("binding not persisted")
	}
}
func TestSingleVaultMigrationKeepsDataAndLocalGroups(t *testing.T) {
	b := setupHTTP(t)
	original := b.seed()
	old := b.app.Vault.Status().Directory
	code, data := b.request("POST", "/api/groups/create", map[string]string{"name": "synthetic empty"}, b.csrf)
	if code != 200 {
		t.Fatalf("group: %d %s", code, data)
	}
	target := filepath.Join(t.TempDir(), "migrated")
	code, data = b.request("POST", "/api/vault/migrate", map[string]any{"directory": target, "password": "synthetic-test-password", "confirmed": true}, b.csrf)
	if code != 200 {
		t.Fatalf("migrate: %d %s", code, data)
	}
	if !b.app.Vault.Locked() || b.app.Vault.Status().Directory != target {
		t.Fatal("migration did not lock/rebind path")
	}
	if _, err := os.Stat(filepath.Join(old, "vault.json")); err != nil {
		t.Fatal("original removed", err)
	}
	code, data = b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf)
	if code != 200 {
		t.Fatalf("unlock: %d %s", code, data)
	}
	rows, err := b.app.Vault.List(vault.ListOptions{})
	if err != nil || len(rows) != 1 || rows[0].Revision != original.Revision {
		t.Fatal("migration changed records", rows, err)
	}
	code, data = b.request("GET", "/api/groups", nil, "")
	var groups []vault.GroupInfo
	if code != 200 || json.Unmarshal(data, &groups) != nil {
		t.Fatal("catalog", code, string(data))
	}
	found := false
	for _, g := range groups {
		if g.Name == "synthetic empty" {
			found = true
		}
	}
	if !found {
		t.Fatal("empty group lost during migration")
	}
}
func TestSingleVaultRestoreRejectsOtherIdentityBeforeWriting(t *testing.T) {
	b := setupHTTP(t)
	old := b.app.Vault.Status().Directory
	foreign := vault.New(t.TempDir())
	if err := foreign.Init("synthetic-other-password"); err != nil {
		t.Fatal(err)
	}
	archive, err := foreign.Backup()
	if err != nil {
		t.Fatal(err)
	}
	foreign.Lock()
	b.app.Access.Lock()
	target := filepath.Join(t.TempDir(), "foreign-restore")
	code, data := b.request("POST", "/api/vault/restore", map[string]any{"archive": archive, "directory": target, "password": "synthetic-other-password", "confirmed": true}, b.csrf)
	if code != 400 {
		t.Fatalf("foreign backup accepted: %d %s", code, data)
	}
	if b.app.Vault.Status().Directory != old {
		t.Fatal("foreign backup changed location")
	}
	if _, err = os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("foreign backup published")
	}
}
func TestSingleVaultUnlockRejectsReplacedIdentity(t *testing.T) {
	b := setupHTTP(t)
	root := b.app.Vault.Status().Directory
	b.app.Access.Lock()
	foreignRoot := t.TempDir()
	foreign := vault.New(foreignRoot)
	if err := foreign.Init("synthetic-other-password"); err != nil {
		t.Fatal(err)
	}
	foreign.Lock()
	raw, err := os.ReadFile(filepath.Join(foreignRoot, "vault.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "vault.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	code, _ := b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-other-password"}, b.csrf)
	if code != 400 || !b.app.Vault.Locked() {
		t.Fatal("foreign identity became accessible", code)
	}
}

func TestSingleVaultRelocateMissingOriginal(t *testing.T) {
	b := setupHTTP(t)
	entry := b.seed()
	old := b.app.Vault.Status().Directory
	id := b.app.boundID()
	b.app.Access.Lock()
	target := filepath.Join(t.TempDir(), "relocated")
	if err := os.Rename(old, target); err != nil {
		t.Fatal(err)
	}
	b = setupSelectionHTTP(t, old, b.app.deviceDir)
	foreign := vault.New(t.TempDir())
	if err := foreign.Init("synthetic-test-password"); err != nil {
		t.Fatal(err)
	}
	foreign.Lock()
	for _, input := range []map[string]string{
		{"directory": foreign.Status().Directory, "password": "synthetic-test-password"},
		{"directory": target, "password": "wrong"},
	} {
		code, _ := b.request("POST", "/api/vault/select", input, b.csrf)
		if code < 400 || b.app.Vault.Status().Directory != old || b.app.boundID() != id || !b.app.Vault.Locked() {
			t.Fatal("failed relocation changed binding", code)
		}
	}
	code, data := b.request("POST", "/api/vault/select", map[string]string{"directory": target, "password": "synthetic-test-password"}, b.csrf)
	if code != 200 || b.app.boundID() != id {
		t.Fatalf("relocate: %d %s", code, data)
	}
	got, err := b.app.Vault.Get(entry.Key)
	if err != nil || got.Revision != entry.Revision {
		t.Fatal("relocation lost records", err)
	}
}

func TestSingleVaultMigrationSnapshotRejectsLateMCPWrite(t *testing.T) {
	b := setupHTTP(t)
	entry := b.seed()
	value := "synthetic late write"
	pending, err := b.app.Access.Replace("synthetic-client", "Synthetic", []vault.Change{{Key: entry.Key, ExpectedRevision: entry.Revision, Value: &value}})
	if err != nil {
		t.Fatal(err)
	}
	archive, _, err := b.app.Vault.BackupAndLock("synthetic-test-password")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(archive)
	if err := b.app.Access.Decide(pending.RequestID, true); err == nil {
		t.Fatal("approval wrote after migration snapshot")
	}
	if _, err := b.app.Access.Replace("synthetic-client", "Synthetic", []vault.Change{{Key: entry.Key, ExpectedRevision: entry.Revision, Value: &value}}); err == nil {
		t.Fatal("MCP write accepted after migration snapshot")
	}
	target := filepath.Join(t.TempDir(), "snapshot")
	if err := vault.RestoreBackupForVault(archive, target, "synthetic-test-password", b.app.boundID()); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{b.app.Vault.Status().Directory, target} {
		v := vault.New(root)
		if err := v.Unlock("synthetic-test-password"); err != nil {
			t.Fatal(err)
		}
		got, err := v.Get(entry.Key)
		v.Lock()
		if err != nil || got.Revision != entry.Revision {
			t.Fatal("late MCP write changed snapshot or source", err)
		}
	}
}

func TestSingleVaultMigrationActivationFailurePreservesCopies(t *testing.T) {
	b := setupHTTP(t)
	entry := b.seed()
	old := b.app.Vault.Status().Directory
	// A directory at the config filename deterministically rejects atomic replacement.
	if err := os.Mkdir(filepath.Join(b.app.deviceDir, "location.json"), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "copy")
	code, _ := b.request("POST", "/api/vault/migrate", map[string]any{"directory": target, "password": "synthetic-test-password", "confirmed": true}, b.csrf)
	if code != 409 || !b.app.Vault.Locked() || b.app.Vault.Status().Directory != old {
		t.Fatal("activation failure changed active location", code)
	}
	for _, root := range []string{old, target} {
		v := vault.New(root)
		if err := v.Unlock("synthetic-test-password"); err != nil {
			t.Fatal(err)
		}
		got, err := v.Get(entry.Key)
		v.Lock()
		if err != nil || got.Revision != entry.Revision {
			t.Fatal("copy not recoverable", root, err)
		}
	}
}

func TestSingleVaultMigrationPreflightDoesNotLockOnBadTarget(t *testing.T) {
	b := setupHTTP(t)
	code, _ := b.request("POST", "/api/vault/migrate", map[string]any{"directory": b.app.Vault.Status().Directory, "password": "synthetic-test-password", "confirmed": true}, b.csrf)
	if code != 400 || b.app.Vault.Locked() {
		t.Fatal("invalid destination changed live state", code)
	}
}
