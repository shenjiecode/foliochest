package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func peerVault(t *testing.T, v *Vault) *Vault {
	t.Helper()
	root := filepath.Join(t.TempDir(), "peer")
	copyFiles(t, v.root, root)
	peer := New(root)
	if err := peer.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	return peer
}
func TestDeleteRestoresIdentityAndCurrentPolicy(t *testing.T) {
	v := testVault(t)
	first := add(t, v, "entry", Deny, Ask)
	initial, _ := v.Get("entry")
	changed, err := v.Commit([]Change{{Key: "entry", ExpectedRevision: first.Revision, Attachments: ptr([]Attachment{{Name: "synthetic.txt", Data: []byte("SYNTHETIC_ATTACHMENT")}})}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := v.Delete(DeleteChange{Key: "entry", ExpectedHeads: []string{changed[0].Revision}})
	if err != nil || deleted.State != "deleted" || !deleted.Deleted {
		t.Fatal(deleted, err)
	}
	if _, err = v.Get("entry"); !errors.Is(err, ErrDeleted) {
		t.Fatal("deleted entry readable", err)
	}
	if _, err = v.ReadMany([]ReadSpec{{Key: "entry", IncludeValue: true}}, true); !errors.Is(err, ErrDeleted) {
		t.Fatal("approved read bypassed deletion", err)
	}
	if _, err = v.Commit([]Change{{Key: "entry", Description: ptr("reuse"), Value: ptr("new")}}, true, true); !errors.Is(err, ErrDeleted) {
		t.Fatal("deleted key reused", err)
	}
	for _, list := range []func(string) ([]Summary, error){v.List, v.Search} {
		rows, err := list("")
		if err != nil || len(rows) != 0 {
			t.Fatal("deletion exposed", rows, err)
		}
	}
	trash, err := v.Trash("")
	if err != nil || len(trash) != 1 || trash[0].EntryID != initial.EntryID {
		t.Fatal(trash, err)
	}
	tombstone, err := v.HistoricalEntry("entry", deleted.Revision)
	if err != nil || tombstone.Value != "" || len(tombstone.Attachments) != 0 || !tombstone.Deleted {
		t.Fatal("tombstone contains content", err)
	}
	if _, err = v.Restore(RestoreChange{Key: "entry", SourceRevision: deleted.Revision, ExpectedHeads: []string{deleted.Revision}}); err == nil {
		t.Fatal("restored empty deletion record")
	}
	archive, err := v.Backup()
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "restored")
	if err = RestoreBackup(archive, destination, testPassword); err != nil {
		t.Fatal(err)
	}
	copy := New(destination)
	if err = copy.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	if _, err = copy.Get("entry"); !errors.Is(err, ErrDeleted) {
		t.Fatal("backup revived deleted entry", err)
	}
	restored, err := v.Restore(RestoreChange{Key: "entry", SourceRevision: changed[0].Revision, ExpectedHeads: []string{deleted.Revision}})
	if err != nil || restored.ReadPolicy != Deny || restored.WritePolicy != Ask || restored.Deleted {
		t.Fatal(restored, err)
	}
	current, err := v.Get("entry")
	if err != nil || current.EntryID != initial.EntryID || current.Value != initial.Value || len(current.Attachments) != 1 {
		t.Fatal("restore changed identity/content", err)
	}
	if rows, _ := v.Trash(""); len(rows) != 0 {
		t.Fatal(rows)
	}
}
func TestRenameKeepsHistoryAndDoesNotRestoreOldName(t *testing.T) {
	v := testVault(t)
	first := add(t, v, "old", Deny, Ask)
	before, _ := v.Get("old")
	after, err := v.Rename(RenameChange{Key: "old", NewKey: "new", ExpectedRevision: first.Revision})
	if err != nil || after.Key != "new" || after.EntryID != before.EntryID {
		t.Fatal(after, err)
	}
	if _, err = v.Get("old"); !errors.Is(err, ErrNotFound) {
		t.Fatal("old alias still active", err)
	}
	current, _ := v.Get("new")
	if current.Value != before.Value || current.ReadPolicy != Deny || current.WritePolicy != Ask {
		t.Fatal("rename changed data")
	}
	history, err := v.History("new")
	if err != nil || len(history.Versions) != 2 {
		t.Fatal(history, err)
	}
	restored, err := v.Restore(RestoreChange{Key: "new", ExpectedHeads: history.Heads, SourceRevision: first.Revision})
	if err != nil || restored.Key != "new" {
		t.Fatal("history implicitly reverted name", restored, err)
	}
	// A historical name is not an alias; it can be assigned to a DIFFERENT entry.
	replacement := add(t, v, "old", Ask, Ask)
	if replacement.EntryID == before.EntryID {
		t.Fatal("reused stable identity")
	}
	history, err = v.History("old")
	if err != nil || len(history.Versions) != 1 {
		t.Fatal("history crossed identities", history, err)
	}
	if _, err = v.HistoricalEntry("old", first.Revision); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err = v.Rename(RenameChange{Key: "new", NewKey: "old", ExpectedRevision: restored.Revision}); err == nil {
		t.Fatal("collision accepted")
	}
}
func TestConcurrentRenameEditAndDelete(t *testing.T) {
	for _, operation := range []string{"edit", "rename", "delete"} {
		t.Run(operation, func(t *testing.T) {
			left := testVault(t)
			first := add(t, left, "old", Allow, Allow)
			right := peerVault(t, left)
			renamed, err := left.Rename(RenameChange{Key: "old", NewKey: "new", ExpectedRevision: first.Revision})
			if err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "edit":
				_, err = right.Commit([]Change{{Key: "old", ExpectedRevision: first.Revision, Value: ptr("offline edit"), ReadPolicy: ptr(Deny)}}, true, true)
			case "rename":
				_, err = right.Rename(RenameChange{Key: "old", NewKey: "other", ExpectedRevision: first.Revision})
			case "delete":
				_, err = right.Delete(DeleteChange{Key: "old", ExpectedHeads: []string{first.Revision}})
			}
			if err != nil {
				t.Fatal(err)
			}
			copyFiles(t, right.root, left.root)
			history, err := left.History("new")
			if err != nil || history.State != "conflict" || len(history.Heads) != 2 {
				t.Fatal(history, err)
			}
			rows, err := left.List("")
			if err != nil || len(rows) != 2 {
				t.Fatal(rows, err)
			}
			for _, row := range rows {
				if row.State != "conflict" {
					t.Fatal(row)
				}
				if _, err = left.Get(row.Key); !errors.Is(err, ErrConflict) {
					t.Fatal("divergent alias exposed", err)
				}
			}
			if _, err = left.Delete(DeleteChange{Key: "new", ExpectedHeads: history.Heads[:1]}); !errors.Is(err, ErrConflict) {
				t.Fatal("partial heads deletion", err)
			}
			if operation == "delete" {
				result, err := left.Delete(DeleteChange{Key: "new", ExpectedHeads: history.Heads})
				if err != nil || !result.Deleted {
					t.Fatal(result, err)
				}
				if rows, _ := left.List(""); len(rows) != 0 {
					t.Fatal(rows)
				}
			} else {
				result, err := left.Restore(RestoreChange{Key: "new", TargetKey: "resolved", ExpectedHeads: history.Heads, SourceRevision: renamed.Revision})
				if err != nil || result.Key != "resolved" || result.State != "ready" {
					t.Fatal(result, err)
				}
				if operation == "edit" && result.ReadPolicy != Deny {
					t.Fatal("merge relaxed permissions")
				}
				rows, _ = left.List("")
				if len(rows) != 1 || rows[0].Key != "resolved" {
					t.Fatal("candidate names remained aliases", rows)
				}
			}
		})
	}
}
func TestDeleteVersusOfflineEditDoesNotResurrect(t *testing.T) {
	left := testVault(t)
	first := add(t, left, "entry", Allow, Allow)
	right := peerVault(t, left)
	if _, err := left.Delete(DeleteChange{Key: "entry", ExpectedHeads: []string{first.Revision}}); err != nil {
		t.Fatal(err)
	}
	if _, err := right.Commit([]Change{{Key: "entry", ExpectedRevision: first.Revision, Value: ptr("offline")}}, true, true); err != nil {
		t.Fatal(err)
	}
	copyFiles(t, right.root, left.root)
	if _, err := left.Get("entry"); !errors.Is(err, ErrConflict) {
		t.Fatal("offline edit revived deletion", err)
	}
	h, _ := left.History("entry")
	if len(h.Heads) != 2 {
		t.Fatal(h)
	}
	result, err := left.Restore(RestoreChange{Key: "entry", SourceRevision: first.Revision, ExpectedHeads: h.Heads})
	if err != nil || result.State != "ready" {
		t.Fatal(result, err)
	}
}
func TestMissingOrTamperedTombstoneFailsClosed(t *testing.T) {
	for _, tamper := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "tampered"}[tamper], func(t *testing.T) {
			v := testVault(t)
			first := add(t, v, "entry", Allow, Allow)
			deleted, err := v.Delete(DeleteChange{Key: "entry", ExpectedHeads: []string{first.Revision}})
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(v.root, "records", v.versions[deleted.Revision].batch.BatchID+".json")
			if tamper {
				raw, _ := os.ReadFile(file)
				var b batch
				_ = json.Unmarshal(raw, &b)
				b.Changes[0].Deleted = false
				raw, _ = json.Marshal(b)
				if err = os.WriteFile(file, raw, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = os.Remove(file); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 3; i++ {
				if _, err = v.Get("entry"); err == nil {
					t.Fatal("old content returned after missing deletion")
				}
			}
			if _, err = v.Backup(); err == nil {
				t.Fatal("incomplete deletion backed up")
			}
		})
	}
}
func TestMissingRenamedHeadCannotExposeFormerName(t *testing.T) {
	v := testVault(t)
	first := add(t, v, "old", Allow, Allow)
	renamed, err := v.Rename(RenameChange{Key: "old", NewKey: "new", ExpectedRevision: first.Revision})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(v.root, "records", v.versions[renamed.Revision].batch.BatchID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		for _, name := range []string{"old", "new"} {
			if _, err = v.Get(name); err == nil {
				t.Fatal("missing rename exposed ancestor", name)
			}
		}
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = v.Get("new"); err != nil {
		t.Fatal(err)
	}
	if _, err = v.Get("old"); !errors.Is(err, ErrNotFound) {
		t.Fatal("historical name became alias", err)
	}
}
func TestTombstoneReservesNameAgainstRename(t *testing.T) {
	v := testVault(t)
	deleted := add(t, v, "deleted", Ask, Ask)
	source := add(t, v, "source", Ask, Ask)
	if _, err := v.Delete(DeleteChange{Key: "deleted", ExpectedHeads: []string{deleted.Revision}}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Rename(RenameChange{Key: "source", NewKey: "deleted", ExpectedRevision: source.Revision}); err == nil {
		t.Fatal("rename overwrote trash name")
	}
}
func TestRenameWaitsForEntireDependencyChain(t *testing.T) {
	left := testVault(t)
	first := add(t, left, "old", Ask, Ask)
	right := peerVault(t, left)
	middle, err := right.Commit([]Change{{Key: "old", ExpectedRevision: first.Revision, Value: ptr("middle")}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := right.Rename(RenameChange{Key: "old", NewKey: "new", ExpectedRevision: middle[0].Revision})
	if err != nil {
		t.Fatal(err)
	}
	id := right.versions[renamed.Revision].batch.BatchID
	raw, _ := os.ReadFile(filepath.Join(right.root, "records", id+".json"))
	_ = os.WriteFile(filepath.Join(left.root, "records", id+".json"), raw, 0600)
	for _, name := range []string{"old", "new"} {
		if _, err = left.Get(name); err == nil {
			t.Fatal("partial sync exposed a branch")
		}
	}
	copyFiles(t, right.root, left.root)
	current, err := left.Get("new")
	if err != nil || current.Value != "middle" {
		t.Fatal(err)
	}
}
func TestRenameCollisionDifferentIdentitiesCannotBeMerged(t *testing.T) {
	left := testVault(t)
	first := add(t, left, "entry", Ask, Ask)
	right := peerVault(t, left)
	_, err := left.Rename(RenameChange{Key: "entry", NewKey: "occupied", ExpectedRevision: first.Revision})
	if err != nil {
		t.Fatal(err)
	}
	add(t, right, "occupied", Allow, Allow)
	copyFiles(t, right.root, left.root)
	h, err := left.History("occupied")
	if err != nil || h.State != "conflict" {
		t.Fatal(h, err)
	}
	if _, err = left.Delete(DeleteChange{Key: "occupied", ExpectedHeads: h.Heads}); err == nil {
		t.Fatal("deleted multiple identities")
	}
	if _, err = left.Restore(RestoreChange{Key: "occupied", ExpectedHeads: h.Heads, SourceRevision: first.Revision}); err == nil {
		t.Fatal("merged identities")
	}
}
func TestManagementRejectsStaleRequestsAndInvalidNames(t *testing.T) {
	v := testVault(t)
	first := add(t, v, "entry", Ask, Ask)
	for _, name := range []string{"", "entry", "bad\nkey"} {
		if _, err := v.Rename(RenameChange{Key: "entry", NewKey: name, ExpectedRevision: first.Revision}); err == nil {
			t.Fatal("accepted invalid name", name)
		}
	}
	if _, err := v.Rename(RenameChange{Key: "entry", NewKey: "other", ExpectedRevision: ""}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err := v.Delete(DeleteChange{Key: "entry", ExpectedHeads: []string{first.Revision, first.Revision}}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	v.Lock()
	if _, err := v.Delete(DeleteChange{Key: "entry", ExpectedHeads: []string{first.Revision}}); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
	if _, err := v.Rename(RenameChange{Key: "entry", NewKey: "new", ExpectedRevision: first.Revision}); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
}
func legacyVault(t *testing.T) *Vault {
	t.Helper()
	v := testVault(t)
	c, err := wrapVersion(testPassword, v.cfg.VaultID, v.key, 1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.MarshalIndent(c, "", "  ")
	if err = os.WriteFile(filepath.Join(v.root, "vault.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	if err = v.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestExplicitFormatUpgradePreservesRecordsAndPassword(t *testing.T) {
	v := legacyVault(t)
	first := add(t, v, "entry", Ask, Ask)
	if v.Status().Format != 1 || v.versions[first.Revision].batch.Format != 1 {
		t.Fatal("legacy write silently upgraded")
	}
	if _, err := v.Rename(RenameChange{Key: "entry", NewKey: "new", ExpectedRevision: first.Revision}); !errors.Is(err, ErrUpgrade) {
		t.Fatal(err)
	}
	if _, err := v.Delete(DeleteChange{Key: "entry", ExpectedHeads: []string{first.Revision}}); !errors.Is(err, ErrUpgrade) {
		t.Fatal(err)
	}
	before := bytes.Clone(v.key)
	record := filepath.Join(v.root, "records", v.versions[first.Revision].batch.BatchID+".json")
	raw, _ := os.ReadFile(record)
	_, credential, err := v.DeviceCredential(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(credential)
	archive, err := v.Backup()
	if err != nil {
		t.Fatal(err)
	}
	if err = v.UpgradeFormat("wrong-password"); err == nil || v.Locked() {
		t.Fatal("bad upgrade changed state", err)
	}
	if err = v.UpgradeFormat(testPassword); err != nil || !v.Locked() {
		t.Fatal(err)
	}
	if err = v.UnlockDevice(credential); err == nil {
		t.Fatal("old device binding accepted")
	}
	if err = v.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, v.key) || v.Status().Format != 2 {
		t.Fatal("upgrade rotated data key or kept format 1")
	}
	nextRaw, _ := os.ReadFile(record)
	if !bytes.Equal(raw, nextRaw) {
		t.Fatal("upgrade rewrote immutable history")
	}
	if _, err = v.Rename(RenameChange{Key: "entry", NewKey: "new", ExpectedRevision: first.Revision}); err != nil {
		t.Fatal(err)
	}
	mixed, err := v.Backup()
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"legacy": archive, "mixed": mixed} {
		dir := filepath.Join(t.TempDir(), name)
		if err = RestoreBackup(data, dir, testPassword); err != nil {
			t.Fatal(name, err)
		}
	}
}
func TestLegacyPasswordChangeDoesNotUpgradeFormat(t *testing.T) {
	v := legacyVault(t)
	add(t, v, "entry", Ask, Ask)
	password := "SYNTHETIC_NEW_PASSWORD"
	if err := v.ChangePassword(testPassword, password); err != nil {
		t.Fatal(err)
	}
	if err := v.Unlock(password); err != nil || v.Status().Format != 1 {
		t.Fatal("password change migrated without confirmation", err)
	}
}
func TestFutureFormatWithUnknownFieldsDoesNotFallBack(t *testing.T) {
	v := testVault(t)
	add(t, v, "entry", Allow, Allow)
	path := filepath.Join(v.root, "records", newID()+".json")
	if err := os.WriteFile(path, []byte(`{"format":3,"future_only_field":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get("entry"); err == nil {
		t.Fatal("future format ignored after strict decoding failed")
	}
}
func TestFormatTwoRecordsCannotBeIgnoredUnderOldConfig(t *testing.T) {
	v := legacyVault(t)
	first := add(t, v, "entry", Ask, Ask)
	cfg, _ := os.ReadFile(filepath.Join(v.root, "vault.json"))
	if err := v.UpgradeFormat(testPassword); err != nil {
		t.Fatal(err)
	}
	if err := v.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Delete(DeleteChange{Key: "entry", ExpectedHeads: []string{first.Revision}}); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	if err := os.WriteFile(filepath.Join(v.root, "vault.json"), cfg, 0600); err != nil {
		t.Fatal(err)
	}
	if err := v.Unlock(testPassword); err == nil {
		t.Fatal("ignored newer-format records")
	}
	if _, err := v.Get("entry"); err == nil {
		t.Fatal("served pre-deletion value")
	}
}
