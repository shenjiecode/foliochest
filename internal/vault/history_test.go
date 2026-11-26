package vault

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestHistoryRestoreKeepsCurrentPermissions(t *testing.T) {
	v := testVault(t)
	first := add(t, v, "entry", Allow, Allow)
	changed, err := v.Commit([]Change{{Key: "entry", ExpectedRevision: first.Revision, Value: ptr("replacement"), ReadPolicy: ptr(Deny), WritePolicy: ptr(Ask)}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	h, err := v.History("entry")
	if err != nil || len(h.Versions) != 2 || len(h.Heads) != 1 {
		t.Fatal(h, err)
	}
	old, err := v.HistoricalEntry("entry", first.Revision)
	if err != nil || old.Value != "SYNTHETIC_SECRET_entry" {
		t.Fatal(err)
	}
	restored, err := v.Restore(RestoreChange{Key: "entry", SourceRevision: first.Revision, ExpectedHeads: h.Heads})
	if err != nil || restored.ReadPolicy != Deny || restored.WritePolicy != Ask || restored.Revision == first.Revision {
		t.Fatal(restored, err)
	}
	latest, err := v.Get("entry")
	if err != nil || latest.Value != old.Value || latest.Parents[0] != changed[0].Revision {
		t.Fatal(latest, err)
	}
	if _, err = v.Restore(RestoreChange{Key: "entry", SourceRevision: first.Revision, ExpectedHeads: h.Heads}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale restore accepted", err)
	}
	if _, err = v.HistoricalEntry("another", first.Revision); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-entry history", err)
	}
	v.Lock()
	if _, err = v.History("entry"); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
}
func TestMergeBindsAllHeadsAndPreservesHistory(t *testing.T) {
	left := testVault(t)
	initial := add(t, left, "entry", Allow, Allow)
	otherDir := filepath.Join(t.TempDir(), "other")
	copyFiles(t, left.root, otherDir)
	right := New(otherDir)
	if err := right.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	a, err := left.Commit([]Change{{Key: "entry", ExpectedRevision: initial.Revision, Value: ptr("branch a"), ReadPolicy: ptr(Deny)}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = right.Commit([]Change{{Key: "entry", ExpectedRevision: initial.Revision, Value: ptr("branch b"), WritePolicy: ptr(Ask)}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	copyFiles(t, right.root, left.root)
	h, err := left.History("entry")
	if err != nil || h.State != "conflict" || len(h.Heads) != 2 {
		t.Fatal(h, err)
	}
	if _, err = left.Restore(RestoreChange{Key: "entry", SourceRevision: a[0].Revision, ExpectedHeads: h.Heads[:1]}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	merged, err := left.Restore(RestoreChange{Key: "entry", SourceRevision: a[0].Revision, ExpectedHeads: h.Heads, Value: ptr("both changes")})
	if err != nil || merged.ReadPolicy != Deny || merged.WritePolicy != Ask || merged.State != "ready" {
		t.Fatal(merged, err)
	}
	current, err := left.Get("entry")
	if err != nil || len(current.Parents) != 2 || current.Value != "both changes" {
		t.Fatal(current, err)
	}
	copyFiles(t, left.root, right.root)
	copy, err := right.Get("entry")
	if err != nil || copy.Revision != current.Revision {
		t.Fatal(copy, err)
	}
	h, err = left.History("entry")
	if err != nil || len(h.Versions) != 4 {
		t.Fatal(h, err)
	}
}
func TestHumanBodySearchDoesNotChangeDiscovery(t *testing.T) {
	v := testVault(t)
	add(t, v, "entry", Deny, Deny)
	rows, err := v.List(ListOptions{Query: "SYNTHETIC_SECRET"})
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	rows, err = v.List(ListOptions{Query: "synthetic_secret", Scope: ListBody})
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	v.Lock()
	if _, err = v.List(ListOptions{Query: "secret", Scope: ListBody}); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
}
