package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestListOptionsScopes(t *testing.T) {
	v := testVault(t)
	grouped(t, v, "a", "A")
	grouped(t, v, "b", "")
	gone := grouped(t, v, "trash", "A")
	if _, err := v.Delete(DeleteChange{Key: gone.Key, ExpectedHeads: []string{gone.Revision}}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		options ListOptions
		count   int
	}{
		{ListOptions{}, 2},
		{ListOptions{Group: ptr("")}, 1},
		{ListOptions{Group: ptr("a")}, 0},
		{ListOptions{Query: "SYNTHETIC_BODY"}, 0},
		{ListOptions{Query: "SYNTHETIC_BODY", Scope: ListBody, Group: ptr("A")}, 1},
		{ListOptions{Scope: ListTrash, Group: ptr("A")}, 1},
		{ListOptions{Scope: ListTrash, Group: ptr("")}, 0},
		{ListOptions{Scope: ListTrash, Query: "SYNTHETIC_BODY"}, 0},
	} {
		rows, err := v.List(test.options)
		if err != nil || len(rows) != test.count {
			t.Fatal(test, rows, err)
		}
	}
	if _, err := v.List(ListOptions{Scope: 255}); err == nil {
		t.Fatal("accepted unknown scope")
	}
	v.Lock()
	if _, err := v.List(ListOptions{}); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
}

func TestFormatOneParentNameIsCheckedOnRead(t *testing.T) {
	v := legacyVault(t)
	entry := add(t, v, "old", Ask, Ask)
	original := v.versions[entry.Revision]
	document, err := v.document(original)
	if err != nil {
		t.Fatal(err)
	}
	meta := original.meta
	meta.Key = "renamed"
	meta.Parents = []string{meta.Revision}
	meta.Revision = newID()
	candidate := batch{batchHeader: batchHeader{Format: 1, VaultID: v.cfg.VaultID, BatchID: newID(), Changes: []Metadata{meta}}}
	plain, err := json.Marshal([]Document{document})
	if err != nil {
		t.Fatal(err)
	}
	candidate.Payload, err = seal(v.key, plain, candidate.aad())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err = publish(filepath.Join(v.root, "records"), candidate.BatchID+".json", raw); err != nil {
		t.Fatal(err)
	}
	if err = v.Scan(); err != nil {
		t.Fatal(err)
	}
	if v.batchStates[candidate.BatchID] == 2 {
		t.Fatal("accepted authenticated format-1 rename")
	}
	if _, err = v.Get("renamed"); err == nil {
		t.Fatal("returned unsupported renamed content")
	}
}

func TestPublicationAndConfigReplacementKeepTheirOwnGuards(t *testing.T) {
	dir := t.TempDir()
	name := "record.json"
	old := []byte("synthetic original")
	if err := publish(dir, name, old); err != nil {
		t.Fatal(err)
	}
	if err := publish(dir, name, []byte("replacement")); err == nil {
		t.Fatal("overwrote immutable record")
	}
	if raw, err := os.ReadFile(filepath.Join(dir, name)); err != nil || !bytes.Equal(raw, old) {
		t.Fatal(err)
	}

	v := testVault(t)
	path := filepath.Join(v.root, "vault.json")
	external := append(bytes.Clone(v.configBytes), '\n')
	if err := os.WriteFile(path, external, 0600); err != nil {
		t.Fatal(err)
	}
	v.mu.Lock()
	err := v.replaceConfig(v.cfg)
	v.mu.Unlock()
	if !errors.Is(err, ErrChanged) || !v.Locked() {
		t.Fatal("missed pre-replace comparison", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(raw, external) {
		t.Fatal("overwrote external config", err)
	}
	pending, err := filepath.Glob(filepath.Join(v.root, ".pending-*"))
	if err != nil || len(pending) != 0 {
		t.Fatal("temporary config leaked", pending, err)
	}
}
