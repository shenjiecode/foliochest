package vault

import (
	"errors"
	"slices"
	"sort"
)

type VersionInfo struct {
	Metadata
	Head      bool `json:"head"`
	Available bool `json:"available"`
}
type History struct {
	Key      string        `json:"key"`
	State    string        `json:"state"`
	Heads    []string      `json:"heads"`
	Versions []VersionInfo `json:"versions"`
}
type RestoreChange struct {
	Key            string   `json:"key"`
	TargetKey      string   `json:"target_key,omitempty"`
	ExpectedHeads  []string `json:"expected_heads"`
	SourceRevision string   `json:"source_revision"`
	Description    *string  `json:"description,omitempty"`
	Group          *string  `json:"group,omitempty"`
	Value          *string  `json:"value,omitempty"`
	Format         *string  `json:"format,omitempty"`
}

// History and HistoricalEntry are human-only interfaces.
func (v *Vault) History(key string) (History, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.fresh(); err != nil {
		return History{}, err
	}
	it, ok := v.items[key]
	if !ok {
		return History{}, ErrNotFound
	}
	out := History{Key: key, State: it.state, Heads: []string{}, Versions: []VersionInfo{}}
	ids := map[string]bool{}
	heads := map[string]bool{}
	for _, h := range it.heads {
		ids[h.meta.EntryID] = true
		heads[h.meta.Revision] = true
		out.Heads = append(out.Heads, h.meta.Revision)
	}
	for _, x := range v.versions {
		if !ids[x.meta.EntryID] {
			continue
		}
		meta := x.meta
		meta.Parents = slices.Clone(meta.Parents)
		out.Versions = append(out.Versions, VersionInfo{Metadata: meta, Head: heads[meta.Revision], Available: v.batchStates[x.batch.BatchID] == 2})
	}
	sort.Slice(out.Versions, func(i, j int) bool {
		a, b := out.Versions[i], out.Versions[j]
		if a.Head != b.Head {
			return a.Head
		}
		if a.UpdatedAt != b.UpdatedAt {
			return a.UpdatedAt > b.UpdatedAt
		}
		return a.Revision < b.Revision
	})
	sort.Strings(out.Heads)
	return out, nil
}
func (v *Vault) historical(key, revision string) (*version, error) {
	it, ok := v.items[key]
	if !ok {
		return nil, ErrNotFound
	}
	x, ok := v.versions[revision]
	if !ok {
		return nil, ErrNotFound
	}
	for _, h := range it.heads {
		if h.meta.EntryID == x.meta.EntryID {
			if v.batchStates[x.batch.BatchID] != 2 {
				return nil, ErrUnavailable
			}
			return x, nil
		}
	}
	return nil, ErrNotFound
}
func (v *Vault) HistoricalEntry(key, revision string) (Entry, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.fresh(); err != nil {
		return Entry{}, err
	}
	x, err := v.historical(key, revision)
	if err != nil {
		return Entry{}, err
	}
	d, err := v.document(x)
	meta := x.meta
	meta.Parents = slices.Clone(meta.Parents)
	return Entry{Metadata: meta, Document: d}, err
}
func restrictive(a, b Policy) Policy {
	if a == Deny || b == Deny {
		return Deny
	}
	if a == Ask || b == Ask {
		return Ask
	}
	return Allow
}

// Restore creates a new version with all reviewed heads and their stricter policies.
func (v *Vault) Restore(in RestoreChange) (Summary, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.fresh(); err != nil {
		return Summary{}, err
	}
	it, actual, read, write, err := v.reviewHeads(in.Key, in.ExpectedHeads)
	if err != nil {
		return Summary{}, err
	}
	id := it.heads[0].meta.EntryID
	target := in.Key
	if in.TargetKey != "" {
		target = in.TargetKey
	}
	if target != in.Key && v.cfg.Format < 2 {
		return Summary{}, ErrUpgrade
	}
	if err = v.availableName(target, id); err != nil {
		return Summary{}, err
	}
	x, err := v.historical(in.Key, in.SourceRevision)
	if err != nil {
		return Summary{}, err
	}
	if x.meta.Deleted {
		return Summary{}, errors.New("删除记录不含正文，请选择删除前的内容版本恢复")
	}
	d, err := v.document(x)
	if err != nil {
		return Summary{}, err
	}
	m := Metadata{EntryID: id, Revision: newID(), Parents: actual, Key: target, Description: x.meta.Description, ReadPolicy: read, WritePolicy: write, UpdatedAt: stamp()}
	groups := itemGroups(it)
	if len(groups) > 1 && in.Group == nil {
		return Summary{}, errors.New("当前分支的分组不一致，请明确选择恢复后的分组")
	}
	m.Group = groups[0]
	if in.Group != nil {
		m.Group = *in.Group
	}
	if m.Group != "" && v.cfg.Format < 3 {
		return Summary{}, ErrGroupUpgrade
	}
	if in.Description != nil {
		m.Description = *in.Description
	}
	if in.Value != nil {
		d.Value = *in.Value
	}
	if in.Format != nil {
		d.Format = *in.Format
	}
	if err = validateMetadata(m); err != nil {
		return Summary{}, err
	}
	if err = validateDocument(d); err != nil {
		return Summary{}, err
	}
	return v.publishOne(m, d)
}
