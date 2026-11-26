package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestUnselectedVaultNeverUsesWorkingDirectory(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	v := New("")
	if err := v.Init("synthetic-test-password"); err == nil {
		t.Fatal("unselected init")
	}
	files, _ := os.ReadDir(directory)
	if len(files) != 0 {
		t.Fatal("unselected init wrote files")
	}
	trap := New(directory)
	if err := trap.Init("synthetic-test-password"); err != nil {
		t.Fatal(err)
	}
	_, raw, err := trap.DeviceCredential("synthetic-test-password")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	if status := v.Status(); status.Directory != "" || status.Exists || status.Unlocked {
		t.Fatal(status)
	}
	if err := v.Unlock("synthetic-test-password"); err == nil {
		t.Fatal("unselected unlock read cwd")
	}
	if err := v.UnlockDevice(raw); err == nil {
		t.Fatal("unselected device unlock read cwd")
	}
}
func TestCreateAtPreservesActiveVaultOnFailure(t *testing.T) {
	root := t.TempDir()
	v := New(root)
	if err := v.Init("synthetic-test-password"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(root, "vault.json"))
	generation := v.Generation()
	occupied := t.TempDir()
	if err := os.WriteFile(filepath.Join(occupied, "keep.txt"), []byte("SYNTHETIC"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"", "relative", occupied} {
		if err := v.CreateAt(target, "synthetic-test-password"); err == nil {
			t.Fatalf("created %q", target)
		}
		if v.Status().Directory != root || v.Generation() != generation || v.Locked() {
			t.Fatal("failed create changed active vault")
		}
	}
	target := filepath.Join(t.TempDir(), "new")
	if err := v.CreateAt(target, "short"); err == nil {
		t.Fatal("short password")
	}
	if v.Status().Directory != root || v.Generation() != generation {
		t.Fatal("invalid password changed selection")
	}
	if err := v.CreateAt(target, "synthetic-new-password"); err != nil {
		t.Fatal(err)
	}
	if v.Status().Directory != target || !v.Status().Exists || v.Locked() {
		t.Fatal(v.Status())
	}
	after, _ := os.ReadFile(filepath.Join(root, "vault.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("old vault altered")
	}
}
