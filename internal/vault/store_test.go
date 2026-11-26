package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPassword = "synthetic-test-password-only"

func ptr[T any](x T) *T { return &x }
func testVault(t *testing.T) *Vault {
	t.Helper()
	v := New(t.TempDir())
	if err := v.Init(testPassword); err != nil {
		t.Fatal(err)
	}
	return v
}
func add(t *testing.T, v *Vault, key string, rp, wp Policy) Summary {
	t.Helper()
	changes, err := v.Commit([]Change{{Key: key, Description: ptr("public description " + key), Value: ptr("SYNTHETIC_SECRET_" + key), ReadPolicy: &rp, WritePolicy: &wp}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	return changes[0]
}
func records(t *testing.T, v *Vault) []string {
	t.Helper()
	p, err := filepath.Glob(filepath.Join(v.root, "records", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func copyFiles(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(dst, 0700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			copyFiles(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dst, e.Name()), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestEncryptionAndDiscovery(t *testing.T) {
	v := testVault(t)
	add(t, v, "demo/key", Ask, Ask)
	b, err := os.ReadFile(records(t, v)[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("SYNTHETIC_SECRET")) {
		t.Fatal("plaintext leaked to disk")
	}
	if !bytes.Contains(b, []byte("demo/key")) || !bytes.Contains(b, []byte("public description")) {
		t.Fatal("public metadata missing")
	}
	list, err := v.List(ListOptions{Query: "description"})
	if err != nil || len(list) != 1 {
		t.Fatalf("discover: %v %v", list, err)
	}
	encoded, _ := json.Marshal(list)
	if bytes.Contains(encoded, []byte("SYNTHETIC_SECRET")) {
		t.Fatal("discovery leaked value")
	}
	v.Lock()
	if _, err = v.List(ListOptions{}); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
	if err = v.Unlock("incorrect-password"); err == nil {
		t.Fatal("wrong password accepted")
	}
	if err = v.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	entry, err := v.Get("demo/key")
	if err != nil || entry.Value != "SYNTHETIC_SECRET_demo/key" {
		t.Fatal(entry, err)
	}
}
func TestReadWritePoliciesAndScopes(t *testing.T) {
	v := testVault(t)
	a := add(t, v, "ask", Ask, Ask)
	d := add(t, v, "deny", Deny, Deny)
	add(t, v, "allow", Allow, Allow)
	reads := []ReadSpec{{Key: "ask", IncludeValue: true}}
	if _, err := v.ReadMany(reads, false); !errors.Is(err, ErrApproval) {
		t.Fatal(err)
	}
	if _, err := v.ReadMany(reads, true); err != nil {
		t.Fatal(err)
	}
	if _, err := v.ReadMany([]ReadSpec{{Key: "deny", IncludeValue: true}}, true); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if _, err := v.ReadMany([]ReadSpec{{Key: "allow", IncludeValue: true}}, false); err != nil {
		t.Fatal(err)
	}
	change := Change{Key: "ask", ExpectedRevision: a.Revision, Value: ptr("REPLACED_SYNTHETIC_SECRET")}
	if _, err := v.Commit([]Change{change}, false, false); !errors.Is(err, ErrApproval) {
		t.Fatal(err)
	}
	if _, err := v.Commit([]Change{change}, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Commit([]Change{change}, false, true); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err := v.Commit([]Change{{Key: "deny", ExpectedRevision: d.Revision, Value: ptr("replacement")}}, false, true); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	current, _ := v.Get("ask")
	if _, err := v.Commit([]Change{{Key: "ask", ExpectedRevision: current.Revision, ReadPolicy: ptr(Allow)}}, false, true); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}
func TestAttachmentsAreSelected(t *testing.T) {
	v := testVault(t)
	a := add(t, v, "files", Allow, Ask)
	files := []Attachment{{Name: "first.txt", MediaType: "text/plain", Data: []byte("FIRST_SYNTHETIC")}, {Name: "second.txt", MediaType: "text/plain", Data: []byte("SECOND_SYNTHETIC")}}
	if _, err := v.Commit([]Change{{Key: "files", ExpectedRevision: a.Revision, Attachments: &files}}, true, true); err != nil {
		t.Fatal(err)
	}
	out, err := v.ReadMany([]ReadSpec{{Key: "files", Attachments: []string{"first.txt"}}}, false)
	if err != nil || len(out) != 1 || out[0].Value != nil || len(out[0].Attachments) != 1 || out[0].Attachments[0].Name != "first.txt" {
		t.Fatal(out, err)
	}
}
func TestExternalTamperAndRecovery(t *testing.T) {
	v := testVault(t)
	add(t, v, "changed", Allow, Ask)
	path := records(t, v)[0]
	original, _ := os.ReadFile(path)
	if err := os.WriteFile(path, bytes.Replace(original, []byte("public description"), []byte("tampered description"), 1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get("changed"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get("changed"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v.root, "records", "incoming.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get("changed"); err != nil {
		t.Fatal(err)
	}
	if len(v.Status().Issues) == 0 {
		t.Fatal("partial file not surfaced")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get("changed"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestTwoDevicesKeepConflict(t *testing.T) {
	a := testVault(t)
	initial := add(t, a, "shared", Ask, Ask)
	rootB := t.TempDir()
	copyFiles(t, a.root, rootB)
	b := New(rootB)
	if err := b.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	for i, v := range []*Vault{a, b} {
		value := []string{"computer A", "computer B"}[i]
		if _, err := v.Commit([]Change{{Key: "shared", ExpectedRevision: initial.Revision, Value: &value}}, true, true); err != nil {
			t.Fatal(err)
		}
	}
	copyFiles(t, filepath.Join(b.root, "records"), filepath.Join(a.root, "records"))
	list, err := a.List(ListOptions{})
	if err != nil || len(list) != 1 || list[0].State != "conflict" || list[0].Versions != 3 {
		t.Fatal(list, err)
	}
	if _, err = a.Get("shared"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
func TestBatchCannotPartiallyCommit(t *testing.T) {
	v := testVault(t)
	a := add(t, v, "a", Allow, Ask)
	b := add(t, v, "b", Allow, Deny)
	_, err := v.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, Value: ptr("new-a")}, {Key: "b", ExpectedRevision: b.Revision, Value: ptr("new-b")}}, false, true)
	if !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	got, _ := v.Get("a")
	if got.Revision != a.Revision {
		t.Fatal("partial batch committed")
	}
}
func TestConfigChangeLocksInsteadOfReinitializing(t *testing.T) {
	v := testVault(t)
	path := filepath.Join(v.root, "vault.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); !errors.Is(err, ErrChanged) {
		t.Fatal(err)
	}
	if !v.Locked() {
		t.Fatal("config change did not lock")
	}
	if err := v.Init(testPassword); err == nil {
		t.Fatal("reinitialized existing directory")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "{" {
		t.Fatal("overwrote damaged config")
	}
}
func TestOutOfOrderBatchWaitsForAllParents(t *testing.T) {
	source := testVault(t)
	a := add(t, source, "a", Allow, Ask)
	aPath := records(t, source)[0]
	b := add(t, source, "b", Allow, Ask)
	_, err := source.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, Value: ptr("new a")}, {Key: "b", ExpectedRevision: b.Revision, Value: ptr("new b")}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	copyFiles(t, source.root, destination)
	missingPath := filepath.Join(destination, "records", filepath.Base(aPath))
	if err = os.Remove(missingPath); err != nil {
		t.Fatal(err)
	}
	receiving := New(destination)
	if err = receiving.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"a", "b"} {
		if _, err = receiving.Get(key); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s should wait for entire batch: %v", key, err)
		}
	}
	original, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(missingPath, original, 0600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"a", "b"} {
		got, err := receiving.Get(key)
		if err != nil || got.Value != "new "+key {
			t.Fatal(got, err)
		}
	}
}

func TestBadMetadataAndPathAreRejected(t *testing.T) {
	v := testVault(t)
	a := add(t, v, "one", Ask, Ask)
	_, err := v.Commit([]Change{{Key: "one", ExpectedRevision: a.Revision, Attachments: &[]Attachment{{Name: "../../secret", Data: []byte("x")}}}}, true, true)
	if err == nil {
		t.Fatal("path traversal name accepted")
	}
	if _, err = v.Commit([]Change{{Key: "empty-description", Description: ptr(""), Value: ptr("value")}}, true, true); err == nil {
		t.Fatal("empty description accepted")
	}
	if err = v.SelectDirectory("relative/path"); err == nil {
		t.Fatal("relative root accepted")
	}
	if _, err = v.List(ListOptions{Query: strings.Repeat("x", 100)}); err != nil {
		t.Fatal(err)
	}
}
