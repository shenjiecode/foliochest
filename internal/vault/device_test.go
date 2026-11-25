package vault

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPasswordRewrapPreservesRecordsAndOldBackup(t *testing.T) {
	v := testVault(t)
	row := add(t, v, "password-change", Ask, Deny)
	backup, err := v.Backup()
	if err != nil {
		t.Fatal(err)
	}
	oldBinding, credential, err := v.DeviceCredential(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(credential)
	recordPath := records(t, v)[0]
	before, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	oldConfig := bytes.Clone(v.configBytes)
	oldKey := bytes.Clone(v.key)
	defer clear(oldKey)
	oldGeneration := v.Generation()
	next := "synthetic-next-password-only"
	if err = v.ChangePassword(testPassword, next); err != nil {
		t.Fatal(err)
	}
	if !v.Locked() || v.Generation() == oldGeneration {
		t.Fatal("password change did not lock")
	}
	if err = v.Unlock(testPassword); err == nil {
		t.Fatal("old password accepted")
	}
	if err = v.UnlockDevice(credential); err == nil {
		t.Fatal("old device credential accepted new config")
	}
	if err = v.Unlock(next); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.key, oldKey) {
		t.Fatal("unexpected data-key rotation")
	}
	currentBinding, err := v.Binding()
	if err != nil || currentBinding == oldBinding || currentBinding.VaultID != oldBinding.VaultID {
		t.Fatal(currentBinding, err)
	}
	after, err := os.ReadFile(recordPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("records rewritten", err)
	}
	entry, err := v.Get("password-change")
	if err != nil || entry.Revision != row.Revision || entry.Value != "SYNTHETIC_SECRET_password-change" || entry.WritePolicy != Deny {
		t.Fatal(entry, err)
	}
	configBytes, err := os.ReadFile(filepath.Join(v.root, "vault.json"))
	if err != nil || bytes.Equal(configBytes, oldConfig) || bytes.Contains(configBytes, []byte(next)) {
		t.Fatal("bad rewrap", err)
	}
	destination := filepath.Join(t.TempDir(), "old-backup")
	if err = RestoreBackup(backup, destination, testPassword); err != nil {
		t.Fatal("old backup must remain recoverable", err)
	}
	restored := New(destination)
	if err = restored.UnlockDevice(credential); err != nil {
		t.Fatal("old key remains valid against old config", err)
	}
}
func TestPasswordRewrapRejectsInvalidRequests(t *testing.T) {
	for _, test := range []struct{ name, current, next string }{
		{"wrong-current", "incorrect-password", "new-synthetic-password"}, {"short", testPassword, "short"}, {"same", testPassword, testPassword},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := testVault(t)
			before := bytes.Clone(v.configBytes)
			generation := v.Generation()
			if err := v.ChangePassword(test.current, test.next); err == nil {
				t.Fatal("invalid change accepted")
			}
			after, _ := os.ReadFile(filepath.Join(v.root, "vault.json"))
			if !bytes.Equal(before, after) || v.Locked() || v.Generation() != generation {
				t.Fatal("failed change changed config/session")
			}
		})
	}
}
func TestPasswordRewrapRejectsMissingRecordsAndExternalConfig(t *testing.T) {
	v := testVault(t)
	add(t, v, "missing", Ask, Ask)
	before := bytes.Clone(v.configBytes)
	if err := os.Remove(records(t, v)[0]); err != nil {
		t.Fatal(err)
	}
	if err := v.ChangePassword(testPassword, "another-synthetic-password"); err == nil {
		t.Fatal("rewrapped incomplete snapshot")
	}
	after, _ := os.ReadFile(filepath.Join(v.root, "vault.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("changed config")
	}
	if err := os.WriteFile(filepath.Join(v.root, "vault.json"), append(before, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := v.ChangePassword(testPassword, "another-synthetic-password"); err == nil || !v.Locked() {
		t.Fatal("external config change not detected", err)
	}
}
func TestDeviceCredentialValidation(t *testing.T) {
	v := testVault(t)
	add(t, v, "device", Ask, Ask)
	if _, _, err := v.DeviceCredential("incorrect-password"); err == nil {
		t.Fatal("enrollment without current password")
	}
	_, credential, err := v.DeviceCredential(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(credential)
	if bytes.Contains(credential, []byte(testPassword)) {
		t.Fatal("master password in credential")
	}
	var malformed deviceCredential
	if err = json.Unmarshal(credential, &malformed); err != nil {
		t.Fatal(err)
	}
	malformed.Key[0] ^= 1
	bad, _ := json.Marshal(malformed)
	v.Lock()
	if err = v.UnlockDevice(bad); err == nil || !v.Locked() {
		t.Fatal("corrupt device secret accepted")
	}
	if err = v.UnlockDevice(credential); err != nil {
		t.Fatal(err)
	}
	entry, err := v.Get("device")
	if err != nil || entry.Value != "SYNTHETIC_SECRET_device" {
		t.Fatal(entry, err)
	}
	other := testVault(t)
	other.Lock()
	if err = other.UnlockDevice(credential); err == nil || !other.Locked() {
		t.Fatal("foreign vault accepted credential")
	}
	v.Lock()
	if _, _, err = v.DeviceCredential(testPassword); err == nil {
		t.Fatal("locked vault exported device key")
	}
}
