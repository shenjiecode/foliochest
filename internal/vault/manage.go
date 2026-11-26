package vault

import (
	"errors"
	"slices"
	"sort"
)

type RenameChange struct {
	Key              string `json:"key"`
	ExpectedRevision string `json:"expected_revision"`
	NewKey           string `json:"new_key"`
}
type DeleteChange struct {
	Key           string   `json:"key"`
	ExpectedHeads []string `json:"expected_heads"`
}

// These operations are human-only. Deletion appends an authenticated tombstone.
func (v *Vault) Rename(in RenameChange) (Summary, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.fresh(); err != nil {
		return Summary{}, err
	}
	if v.cfg.Format < 2 {
		return Summary{}, ErrUpgrade
	}
	x, err := v.ready(in.Key)
	if err != nil {
		return Summary{}, err
	}
	if in.ExpectedRevision == "" || x.meta.Revision != in.ExpectedRevision {
		return Summary{}, ErrConflict
	}
	if in.NewKey == in.Key {
		return Summary{}, errors.New("新 key 与当前名称相同")
	}
	if err = v.availableName(in.NewKey, x.meta.EntryID); err != nil {
		return Summary{}, err
	}
	m := x.meta
	m.Key = in.NewKey
	m.Revision = newID()
	m.Parents = []string{x.meta.Revision}
	m.UpdatedAt = stamp()
	if err = validateMetadata(m); err != nil {
		return Summary{}, err
	}
	doc, err := v.document(x)
	if err != nil {
		return Summary{}, err
	}
	return v.publishOne(m, doc)
}
func (v *Vault) Delete(in DeleteChange) (Summary, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.fresh(); err != nil {
		return Summary{}, err
	}
	if v.cfg.Format < 2 {
		return Summary{}, ErrUpgrade
	}
	it, parents, read, write, err := v.reviewHeads(in.Key, in.ExpectedHeads)
	if err != nil {
		return Summary{}, err
	}
	if it.state == "deleted" {
		return Summary{}, ErrDeleted
	}
	if len(itemGroups(it)) > 1 {
		return Summary{}, errors.New("当前分支的分组不一致，请先合并并确定分组，再删除")
	}
	m := it.heads[0].meta
	m.Key = in.Key
	m.Parents = parents
	m.Revision = newID()
	m.Deleted = true
	m.ReadPolicy = read
	m.WritePolicy = write
	m.UpdatedAt = stamp()
	return v.publishOne(m, Document{Format: "text", Attachments: []Attachment{}})
}
func (v *Vault) publishOne(m Metadata, doc Document) (Summary, error) {
	b := batch{batchHeader: batchHeader{Format: v.cfg.Format, VaultID: v.cfg.VaultID, BatchID: newID(), Changes: []Metadata{m}}}
	out, err := v.publishBatch(b, []Document{doc})
	if err != nil {
		return Summary{}, err
	}
	if len(out) != 1 {
		return Summary{}, ErrUnavailable
	}
	return out[0], nil
}
func (v *Vault) availableName(key, entryID string) error {
	if it := v.items[key]; it != nil {
		for _, head := range it.heads {
			if head.meta.EntryID != entryID {
				return errors.New("目标 key 已被其他条目或回收站占用")
			}
		}
		if it.state != "ready" && it.state != "deleted" && it.state != "conflict" {
			return ErrUnavailable
		}
	}
	return nil
}
func (v *Vault) reviewHeads(key string, expected []string) (*item, []string, Policy, Policy, error) {
	it := v.items[key]
	if it == nil {
		return nil, nil, "", "", ErrNotFound
	}
	if it.state != "ready" && it.state != "conflict" && it.state != "deleted" {
		return nil, nil, "", "", ErrUnavailable
	}
	if len(expected) == 0 || len(expected) > MaxBatchEntries {
		return nil, nil, "", "", ErrConflict
	}
	id := it.heads[0].meta.EntryID
	read, write := Allow, Allow
	actual := []string{}
	for _, head := range it.heads {
		if head.meta.EntryID != id {
			return nil, nil, "", "", errors.New("这是不同条目的同名冲突，暂不支持直接合并或删除；请保留备份")
		}
		if v.batchStates[head.batch.BatchID] != 2 {
			return nil, nil, "", "", ErrUnavailable
		}
		actual = append(actual, head.meta.Revision)
		read = restrictive(read, head.meta.ReadPolicy)
		write = restrictive(write, head.meta.WritePolicy)
	}
	supplied := slices.Clone(expected)
	sort.Strings(supplied)
	sort.Strings(actual)
	if !slices.Equal(actual, supplied) {
		return nil, nil, "", "", ErrConflict
	}
	return it, actual, read, write, nil
}
