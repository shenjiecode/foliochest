package vault

import (
	"errors"
	"slices"
	"sort"
	"time"
)

type GroupMember struct {
	Key              string `json:"key"`
	ExpectedRevision string `json:"expected_revision"`
}
type GroupInfo struct {
	Name         string        `json:"name"`
	Count        int           `json:"count"`
	DeletedCount int           `json:"deleted_count"`
	Blocked      int           `json:"blocked"`
	Members      []GroupMember `json:"members"`
}
type GroupMove struct {
	Group   string        `json:"group"`
	Entries []GroupMember `json:"entries"`
}
type GroupEdit struct {
	Source   string        `json:"source"`
	Target   string        `json:"target"`
	Expected []GroupMember `json:"expected"`
}

// A normal entry has one group. Divergent branches retain EVERY candidate;
// assigning the first head's group would conceal a concurrent move.
func itemGroups(it *item) []string {
	names := map[string]bool{}
	for _, h := range it.heads {
		names[h.meta.Group] = true
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
func groupMatches(s Summary, name string) bool {
	if len(s.CandidateGroups) > 0 {
		return slices.Contains(s.CandidateGroups, name)
	}
	return s.Group == name
}
func (v *Vault) groupCatalog() []GroupInfo {
	groups := map[string]*GroupInfo{}
	for key, it := range v.items {
		for _, name := range itemGroups(it) {
			g := groups[name]
			if g == nil {
				g = &GroupInfo{Name: name, Members: []GroupMember{}}
				groups[name] = g
			}
			if it.state == "deleted" {
				g.DeletedCount++
			} else {
				g.Count++
			}
			if (it.state != "ready" && it.state != "deleted") || len(it.heads) != 1 {
				g.Blocked++
				continue
			}
			g.Members = append(g.Members, GroupMember{Key: key, ExpectedRevision: it.heads[0].meta.Revision})
		}
	}
	out := make([]GroupInfo, 0, len(groups))
	for _, g := range groups {
		sort.Slice(g.Members, func(i, j int) bool { return g.Members[i].Key < g.Members[j].Key })
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Groups is human-only: it includes trash membership, unlike MCP discovery.
func (v *Vault) Groups() ([]GroupInfo, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, ErrLocked
	}
	if err := v.scan(); err != nil {
		return nil, err
	}
	return v.groupCatalog(), nil
}
func (v *Vault) MoveGroup(in GroupMove) ([]Summary, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, ErrLocked
	}
	if err := v.scan(); err != nil {
		return nil, err
	}
	return v.moveGroup(in)
}

// EditGroup binds the complete reviewed membership, including tombstones.
// Empty target removes the group without deleting entries. There is no
// permanent catalog; a concurrent offline writer can make the old name reappear.
func (v *Vault) EditGroup(in GroupEdit) ([]Summary, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, ErrLocked
	}
	if err := v.scan(); err != nil {
		return nil, err
	}
	if v.cfg.Format < 3 {
		return nil, ErrGroupUpgrade
	}
	if in.Source == "" || in.Source == in.Target {
		return nil, errors.New("请选择现有分组并填写不同的新组名；移除分组请将目标留空")
	}
	var source *GroupInfo
	for _, g := range v.groupCatalog() {
		if g.Name == in.Source {
			source = &g
		}
		if in.Target != "" && g.Name == in.Target {
			return nil, errors.New("目标分组已经存在；如需合并，请使用批量移动")
		}
	}
	if source == nil {
		return nil, ErrNotFound
	}
	if source.Blocked > 0 {
		return nil, errors.New("分组中有冲突、缺失或未完整同步的条目，请先处理")
	}
	if len(source.Members) > MaxBatchEntries {
		return nil, errors.New("分组超过 20 条，请先手动分批移动；不会自动拆分提交")
	}
	if len(in.Expected) != len(source.Members) {
		return nil, ErrConflict
	}
	expected := map[string]string{}
	for _, m := range in.Expected {
		if _, ok := expected[m.Key]; ok {
			return nil, ErrConflict
		}
		expected[m.Key] = m.ExpectedRevision
	}
	for _, m := range source.Members {
		if expected[m.Key] != m.ExpectedRevision {
			return nil, ErrConflict
		}
	}
	return v.moveGroup(GroupMove{Group: in.Target, Entries: in.Expected})
}
func (v *Vault) moveGroup(in GroupMove) ([]Summary, error) {
	if v.cfg.Format < 3 {
		return nil, ErrGroupUpgrade
	}
	if err := validateGroup(in.Group); err != nil {
		return nil, err
	}
	if len(in.Entries) == 0 || len(in.Entries) > MaxBatchEntries {
		return nil, errors.New("每次移动需包含 1–20 个条目")
	}
	seen := map[string]bool{}
	b := batch{batchHeader: batchHeader{Format: v.cfg.Format, VaultID: v.cfg.VaultID, BatchID: newID(), Changes: []Metadata{}}}
	docs := []Document{}
	for _, member := range in.Entries {
		if seen[member.Key] {
			return nil, errors.New("请求中 key 重复")
		}
		seen[member.Key] = true
		it := v.items[member.Key]
		if it == nil {
			return nil, ErrNotFound
		}
		if (it.state != "ready" && it.state != "deleted") || len(it.heads) != 1 {
			return nil, ErrConflict
		}
		x := it.heads[0]
		if member.ExpectedRevision == "" || member.ExpectedRevision != x.meta.Revision {
			return nil, ErrConflict
		}
		if x.meta.Group == in.Group {
			continue
		}
		m := x.meta
		m.Group = in.Group
		m.Parents = []string{x.meta.Revision}
		m.Revision = newID()
		m.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		d, err := v.document(x)
		if err != nil {
			return nil, err
		}
		b.Changes = append(b.Changes, m)
		docs = append(docs, d)
	}
	if len(docs) == 0 {
		return []Summary{}, nil
	}
	return v.publishBatch(b, docs)
}
