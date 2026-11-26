package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func grouped(t *testing.T, v *Vault, key, group string) Summary {
	t.Helper()
	out, err := v.Commit([]Change{{Key: key, Description: ptr("Synthetic description"), Value: ptr("SYNTHETIC_BODY"), Group: ptr(group)}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	return out[0]
}
func member(s Summary) GroupMember { return GroupMember{Key: s.Key, ExpectedRevision: s.Revision} }
func findGroup(t *testing.T, v *Vault, name string) GroupInfo {
	t.Helper()
	groups, err := v.Groups()
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if g.Name == name {
			return g
		}
	}
	t.Fatal("missing group", name)
	return GroupInfo{}
}
func TestGroupMetadataDiscoveryAndValidation(t *testing.T) {
	v := testVault(t)
	a := grouped(t, v, "a", "项目 A")
	grouped(t, v, "b", "")
	for _, query := range []struct {
		q, g string
		n    int
	}{{"", "项目 A", 1}, {"", "", 1}, {"项目 A", "项目 A", 1}, {"SYNTHETIC_BODY", "项目 A", 0}, {"", "不存在", 0}} {
		rows, err := v.List(ListOptions{Query: query.q, Group: ptr(query.g)})
		if err != nil || len(rows) != query.n {
			t.Fatal(query, rows, err)
		}
	}
	rows, err := v.List(ListOptions{Query: "SYNTHETIC_BODY", Group: ptr("项目 A"), Scope: ListBody})
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	// Every batch keeps body and attachment contents encrypted.
	for _, p := range records(t, v) {
		raw, _ := os.ReadFile(p)
		if bytes.Contains(raw, []byte("SYNTHETIC_BODY")) {
			t.Fatal("body leaked")
		}
	}
	for _, name := range []string{" 项目", "项目 ", "a/b", "a\\b", "a\nb", "a\tb", "未分组", strings.Repeat("字", 65)} {
		before := len(records(t, v))
		if _, err = v.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, Group: &name}}, true, true); err == nil {
			t.Fatal("bad group accepted", name)
		}
		if len(records(t, v)) != before {
			t.Fatal("invalid group published")
		}
	}
	long := strings.Repeat("字", 64)
	if _, err = v.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, Group: &long}}, true, true); err != nil {
		t.Fatal(err)
	}
}
func TestGroupMovesPreserveIdentityContentPoliciesAndHistory(t *testing.T) {
	v := testVault(t)
	a := add(t, v, "a", Deny, Allow)
	file := Attachment{Name: "synthetic.txt", MediaType: "text/plain", Data: []byte("SYNTHETIC_ATTACHMENT")}
	updates, err := v.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, Attachments: ptr([]Attachment{file}), Group: ptr("A")}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	a = updates[0]
	before, _ := v.Get("a")
	moved, err := v.MoveGroup(GroupMove{Group: "B", Entries: []GroupMember{member(a)}})
	if err != nil {
		t.Fatal(err)
	}
	current, err := v.Get("a")
	if err != nil || current.Group != "B" || current.EntryID != before.EntryID || current.ReadPolicy != Deny || current.WritePolicy != Allow || !reflect.DeepEqual(current.Document, before.Document) {
		t.Fatal(current, err)
	}
	renamed, err := v.Rename(RenameChange{Key: "a", NewKey: "renamed", ExpectedRevision: moved[0].Revision})
	if err != nil || renamed.Group != "B" {
		t.Fatal(renamed, err)
	}
	old, err := v.HistoricalEntry("renamed", a.Revision)
	if err != nil || old.Group != "A" {
		t.Fatal(old, err)
	}
	restored, err := v.Restore(RestoreChange{Key: "renamed", ExpectedHeads: []string{renamed.Revision}, SourceRevision: a.Revision})
	if err != nil || restored.Group != "B" {
		t.Fatal("history silently restored old group", restored, err)
	}
	dropped, err := v.Delete(DeleteChange{Key: "renamed", ExpectedHeads: []string{restored.Revision}})
	if err != nil || dropped.Group != "B" {
		t.Fatal(dropped, err)
	}
	info := findGroup(t, v, "B")
	if info.Count != 0 || info.DeletedCount != 1 {
		t.Fatal(info)
	}
	if _, err = v.EditGroup(GroupEdit{Source: "B", Target: "", Expected: info.Members}); err != nil {
		t.Fatal(err)
	}
	trash, _ := v.List(ListOptions{Scope: ListTrash})
	restored, err = v.Restore(RestoreChange{Key: "renamed", ExpectedHeads: []string{trash[0].Revision}, SourceRevision: a.Revision})
	if err != nil || restored.Group != "" {
		t.Fatal("restored removed group", restored, err)
	}
}
func TestGroupBatchIsAtomicAndPinsWholeMembership(t *testing.T) {
	v := testVault(t)
	a := grouped(t, v, "a", "A")
	b := grouped(t, v, "b", "A")
	before := len(records(t, v))
	for _, entries := range [][]GroupMember{
		{member(a), {Key: "b", ExpectedRevision: "stale"}},
		{member(a), member(a)},
		{member(a), {Key: "missing", ExpectedRevision: a.Revision}},
	} {
		if _, err := v.MoveGroup(GroupMove{Group: "B", Entries: entries}); err == nil {
			t.Fatal("invalid move accepted")
		}
		if len(records(t, v)) != before {
			t.Fatal("partial batch published")
		}
	}
	snapshot := findGroup(t, v, "A")
	if _, err := v.EditGroup(GroupEdit{Source: "A", Target: "C", Expected: []GroupMember{member(a)}}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	grouped(t, v, "new", "A")
	if _, err := v.EditGroup(GroupEdit{Source: "A", Target: "C", Expected: snapshot.Members}); !errors.Is(err, ErrConflict) {
		t.Fatal("unreviewed member changed", err)
	}
	snapshot = findGroup(t, v, "A")
	grouped(t, v, "other", "B")
	if _, err := v.EditGroup(GroupEdit{Source: "A", Target: "B", Expected: snapshot.Members}); err == nil {
		t.Fatal("silently merged groups")
	}
	out, err := v.MoveGroup(GroupMove{Group: "B", Entries: []GroupMember{member(a), member(b)}})
	if err != nil || len(out) != 2 {
		t.Fatal(out, err)
	}
	if v.versions[out[0].Revision].batch.BatchID != v.versions[out[1].Revision].batch.BatchID {
		t.Fatal("move was split")
	}
	history, _ := v.History("a")
	if len(history.Versions) != 2 {
		t.Fatal(history)
	}
}
func TestGroupRenameIncludesTrashAndBackupRoundTrip(t *testing.T) {
	v := testVault(t)
	grouped(t, v, "a", "A")
	b := grouped(t, v, "b", "A")
	if _, err := v.Delete(DeleteChange{Key: "b", ExpectedHeads: []string{b.Revision}}); err != nil {
		t.Fatal(err)
	}
	g := findGroup(t, v, "A")
	if g.Count != 1 || g.DeletedCount != 1 || len(g.Members) != 2 {
		t.Fatal(g)
	}
	if _, err := v.EditGroup(GroupEdit{Source: "A", Target: "新组", Expected: g.Members}); err != nil {
		t.Fatal(err)
	}
	active, _ := v.List(ListOptions{})
	trash, _ := v.List(ListOptions{Scope: ListTrash})
	if len(active) != 1 || len(trash) != 1 || active[0].Group != "新组" || trash[0].Group != "新组" {
		t.Fatal(active, trash)
	}
	archive, err := v.Backup()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "restored")
	if err = RestoreBackup(archive, dir, testPassword); err != nil {
		t.Fatal(err)
	}
	restored := New(dir)
	if err = restored.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	actual := findGroup(t, restored, "新组")
	if actual.Count != 1 || actual.DeletedCount != 1 {
		t.Fatal(actual)
	}
}
func TestConcurrentGroupsKeepAllCandidatesAndRequireExplicitResolution(t *testing.T) {
	left := testVault(t)
	first := grouped(t, left, "entry", "")
	right := peerVault(t, left)
	if _, err := left.MoveGroup(GroupMove{Group: "A", Entries: []GroupMember{member(first)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := right.MoveGroup(GroupMove{Group: "B", Entries: []GroupMember{member(first)}}); err != nil {
		t.Fatal(err)
	}
	copyFiles(t, right.root, left.root)
	for _, group := range []string{"A", "B"} {
		rows, err := left.List(ListOptions{Group: ptr(group)})
		if err != nil || len(rows) != 1 || rows[0].State != "conflict" || !reflect.DeepEqual(rows[0].CandidateGroups, []string{"A", "B"}) {
			t.Fatal(rows, err)
		}
		g := findGroup(t, left, group)
		if g.Blocked != 1 {
			t.Fatal(g)
		}
		if _, err = left.EditGroup(GroupEdit{Source: group, Target: "C", Expected: g.Members}); err == nil {
			t.Fatal("changed unresolved group")
		}
	}
	if _, err := left.Get("entry"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	h, _ := left.History("entry")
	in := RestoreChange{Key: "entry", ExpectedHeads: h.Heads, SourceRevision: first.Revision}
	if _, err := left.Restore(in); err == nil {
		t.Fatal("silently chose group")
	}
	if _, err := left.Delete(DeleteChange{Key: "entry", ExpectedHeads: h.Heads}); err == nil {
		t.Fatal("deletion silently chose group")
	}
	in.Group = ptr("C")
	merged, err := left.Restore(in)
	if err != nil || merged.Group != "C" || merged.State != "ready" {
		t.Fatal(merged, err)
	}
}
func TestGroupMoveAndOfflineEditConflict(t *testing.T) {
	left := testVault(t)
	a := grouped(t, left, "a", "A")
	right := peerVault(t, left)
	_, err := left.MoveGroup(GroupMove{Group: "B", Entries: []GroupMember{member(a)}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = right.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, Value: ptr("SYNTHETIC_OFFLINE")}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	copyFiles(t, right.root, left.root)
	rows, err := left.List(ListOptions{})
	if err != nil || len(rows) != 1 || rows[0].State != "conflict" || len(rows[0].CandidateGroups) != 2 {
		t.Fatal(rows, err)
	}
}
func TestGroupTamperAndMissingHeadCannotExposeAncestor(t *testing.T) {
	for _, tamper := range []bool{false, true} {
		t.Run(fmt.Sprint(tamper), func(t *testing.T) {
			v := testVault(t)
			a := grouped(t, v, "a", "A")
			moved, err := v.MoveGroup(GroupMove{Group: "B", Entries: []GroupMember{member(a)}})
			if err != nil {
				t.Fatal(err)
			}
			x := v.versions[moved[0].Revision]
			p := filepath.Join(v.root, "records", x.batch.BatchID+".json")
			if tamper {
				raw, _ := os.ReadFile(p)
				var b batch
				if err = json.Unmarshal(raw, &b); err != nil {
					t.Fatal(err)
				}
				b.Changes[0].Group = "forged"
				raw, _ = json.Marshal(b)
				if err = os.WriteFile(p, raw, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = os.Remove(p); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = v.Get("a"); err == nil {
				t.Fatal("served ancestor")
			}
			rows, err := v.List(ListOptions{})
			if err != nil || rows[0].State != "missing" {
				t.Fatal(rows, err)
			}
		})
	}
}
func TestGroupLimitsAndLegacyWrites(t *testing.T) {
	v := testVault(t)
	for i := 0; i < 21; i++ {
		grouped(t, v, fmt.Sprint(i), "A")
	}
	g := findGroup(t, v, "A")
	before := len(records(t, v))
	if _, err := v.EditGroup(GroupEdit{Source: "A", Target: "B", Expected: g.Members}); err == nil {
		t.Fatal("oversized group renamed")
	}
	if len(records(t, v)) != before {
		t.Fatal("split oversized mutation")
	}
	legacy := legacyVault(t)
	a := add(t, legacy, "a", Ask, Ask)
	if _, err := legacy.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, Group: ptr("A")}}, true, true); !errors.Is(err, ErrGroupUpgrade) {
		t.Fatal(err)
	}
	rows, err := legacy.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, Group: ptr(""), Value: ptr("legacy edit")}}, true, true)
	if err != nil || legacy.Status().Format != 1 || rows[0].Group != "" {
		t.Fatal(rows, err)
	}
}
func TestAICannotMoveGroupsOrDropExistingGroup(t *testing.T) {
	v := testVault(t)
	a := grouped(t, v, "a", "A")
	out, err := v.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, WritePolicy: ptr(Allow)}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	a = out[0]
	if _, err = v.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, Group: ptr("")}}, false, true); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	value := "SYNTHETIC_REPLACEMENT"
	out, err = v.Commit([]Change{{Key: "a", ExpectedRevision: a.Revision, Value: &value}}, false, false)
	if err != nil || out[0].Group != "A" {
		t.Fatal(out, err)
	}
}
