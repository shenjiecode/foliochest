package vault

import (
	"errors"
	"slices"
	"sort"
	"time"
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
	ExpectedHeads  []string `json:"expected_heads"`
	SourceRevision string   `json:"source_revision"`
	Description    *string  `json:"description,omitempty"`
	Value          *string  `json:"value,omitempty"`
	Format         *string  `json:"format,omitempty"`
}

// History and HistoricalEntry are human-only interfaces, never MCP tools.
func (v *Vault) History(key string) (History, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return History{}, ErrLocked
	}
	if err := v.scan(); err != nil {
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
	if len(v.key) == 0 {
		return Entry{}, ErrLocked
	}
	if err := v.scan(); err != nil {
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

// Restore creates a new version whose parents are ALL reviewed current heads.
// It never erases history or restores an older, more permissive policy.
func (v *Vault) Restore(in RestoreChange) (Summary, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return Summary{}, ErrLocked
	}
	if err := v.scan(); err != nil {
		return Summary{}, err
	}
	it, ok := v.items[in.Key]
	if !ok {
		return Summary{}, ErrNotFound
	}
	if it.state != "ready" && it.state != "conflict" {
		return Summary{}, ErrUnavailable
	}
	actual := []string{}
	id := it.heads[0].meta.EntryID
	read, write := Allow, Allow
	for _, h := range it.heads {
		if h.meta.EntryID != id {
			return Summary{}, errors.New("这是不同条目的同名冲突，暂不支持自动合并；请保留备份")
		}
		if v.batchStates[h.batch.BatchID] != 2 {
			return Summary{}, ErrUnavailable
		}
		actual = append(actual, h.meta.Revision)
		read = restrictive(read, h.meta.ReadPolicy)
		write = restrictive(write, h.meta.WritePolicy)
	}
	expected := slices.Clone(in.ExpectedHeads)
	sort.Strings(actual)
	sort.Strings(expected)
	if !slices.Equal(actual, expected) {
		return Summary{}, ErrConflict
	}
	if len(actual) > MaxBatchEntries {
		return Summary{}, errors.New("冲突分支数量超过当前合并上限")
	}
	x, err := v.historical(in.Key, in.SourceRevision)
	if err != nil {
		return Summary{}, err
	}
	d, err := v.document(x)
	if err != nil {
		return Summary{}, err
	}
	m := Metadata{EntryID: id, Revision: newID(), Parents: actual, Key: in.Key, Description: x.meta.Description, ReadPolicy: read, WritePolicy: write, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
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
	b := batch{batchHeader: batchHeader{Format: FormatVersion, VaultID: v.cfg.VaultID, BatchID: newID(), Changes: []Metadata{m}}}
	out, err := v.publishBatch(b, []Document{d})
	if err != nil {
		return Summary{}, err
	}
	return out[0], nil
}
