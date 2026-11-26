package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type batchHeader struct {
	Format  int        `json:"format"`
	VaultID string     `json:"vault_id"`
	BatchID string     `json:"batch_id"`
	Changes []Metadata `json:"changes"`
}
type batch struct {
	batchHeader
	Payload []byte `json:"payload"`
}

func (b batch) aad() []byte {
	data, _ := json.Marshal(b.batchHeader)
	return append([]byte("keyfolio/batch/v1\x00"), data...)
}

type version struct {
	meta  Metadata
	batch *batch
	index int
}
type item struct {
	heads []*version
	state string
	count int
}

type Vault struct {
	mu          sync.Mutex
	root        string
	cfg         config
	configBytes []byte
	key         []byte
	generation  uint64
	items       map[string]*item
	identities  map[string]*item
	versions    map[string]*version
	batchStates map[string]int
	issues      []string
	scanErr     error
	recordBytes int64
	recordFiles int
}

func New(root string) *Vault        { return &Vault{root: root, items: map[string]*item{}} }
func (v *Vault) Generation() uint64 { v.mu.Lock(); defer v.mu.Unlock(); return v.generation }
func (v *Vault) Locked() bool       { v.mu.Lock(); defer v.mu.Unlock(); return len(v.key) == 0 }
func (v *Vault) lock() {
	clear(v.key)
	v.key = nil
	v.configBytes = nil
	v.items = map[string]*item{}
	v.identities = map[string]*item{}
	v.versions = nil
	v.batchStates = nil
	v.issues = nil
	v.scanErr = nil
	v.recordBytes = 0
	v.recordFiles = 0
	v.generation++
}
func (v *Vault) Lock() { v.mu.Lock(); defer v.mu.Unlock(); v.lock() }
func (v *Vault) SelectDirectory(root string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !filepath.IsAbs(root) || strings.TrimSpace(root) == "" {
		return errors.New("请输入绝对目录路径")
	}
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		return errors.New("目标不是目录")
	}
	v.lock()
	v.root = filepath.Clean(root)
	return nil
}
func (v *Vault) Init(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	files, err := os.ReadDir(v.root)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(files) != 0 {
		return errors.New("创建资料库需要空目录；已有资料库请解锁")
	}
	c, key, err := newConfig(password)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		clear(key)
		return err
	}
	if err = publish(v.root, "vault.json", data); err != nil {
		clear(key)
		return err
	}
	if err = os.MkdirAll(filepath.Join(v.root, "records"), 0700); err != nil {
		clear(key)
		return err
	}
	v.lock()
	v.cfg = c
	v.configBytes = data
	v.key = key
	return v.scan()
}
func (v *Vault) Unlock(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	data, err := readBounded(filepath.Join(v.root, "vault.json"), 16<<10)
	if err != nil {
		return errors.New("无法读取资料库配置，请确认目录与同步状态")
	}
	var c config
	if err = decodeStrict(data, &c); err != nil {
		return errors.New("资料库配置不完整或不受支持")
	}
	key, err := c.unlock(password)
	if err != nil {
		return err
	}
	if len(v.key) > 0 && bytes.Equal(data, v.configBytes) {
		clear(key)
		return v.scan()
	}
	v.lock()
	v.cfg = c
	v.configBytes = data
	v.key = key
	if err = v.scan(); err != nil {
		v.lock()
		return err
	}
	return nil
}
func (v *Vault) Status() Status {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, err := os.Stat(filepath.Join(v.root, "vault.json"))
	issues := append([]string{}, v.issues...)
	if v.scanErr != nil {
		issues = append(issues, v.scanErr.Error())
	}
	count := 0
	for _, it := range v.items {
		if it.state != "deleted" {
			count++
		}
	}
	format := 0
	if len(v.key) > 0 {
		format = v.cfg.Format
	}
	return Status{Format: format, Directory: v.root, Exists: err == nil, Unlocked: len(v.key) > 0, Entries: count, Issues: issues}
}
func (v *Vault) Scan() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return ErrLocked
	}
	return v.scan()
}
func (v *Vault) scan() error {
	data, err := readBounded(filepath.Join(v.root, "vault.json"), 16<<10)
	if err != nil || !bytes.Equal(data, v.configBytes) {
		v.lock()
		return ErrChanged
	}
	recordDir := filepath.Join(v.root, "records")
	if info, err := os.Lstat(recordDir); err != nil || !info.IsDir() {
		v.scanErr = errors.New("记录目录缺失或类型不受支持")
		return v.scanErr
	}
	files, err := os.ReadDir(recordDir)
	if err != nil {
		v.scanErr = errors.New("记录目录不可用，等待文件恢复")
		return v.scanErr
	}
	if len(files) > 4096 {
		v.scanErr = errors.New("记录文件超过原型上限 4096，请停止写入并导出备份")
		return v.scanErr
	}
	versions := map[string]*version{}
	groups := map[string][]*version{}
	invalid := map[string]bool{}
	issues := []string{}
	var totalBytes int64
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		if info, e := f.Info(); e == nil {
			totalBytes += info.Size()
			if totalBytes > 64<<20 {
				v.scanErr = errors.New("记录总大小超过原型上限 64 MiB，请先保留完整备份")
				return v.scanErr
			}
		}
		raw, e := readBounded(filepath.Join(v.root, "records", f.Name()), MaxFileBytes)
		var b batch
		if e == nil {
			e = decodeStrict(raw, &b)
		}
		if b.Format > v.cfg.Format || b.Format > FormatVersion {
			v.scanErr = errors.New("发现比当前库配置更新的记录格式，请停止操作并同步完整配置、升级所有设备程序")
			return v.scanErr
		}
		if e == nil {
			e = v.validateBatch(&b)
		}
		if e != nil {
			issues = append(issues, fmt.Sprintf("记录 %s 未通过完整性校验，等待完整同步或人工处理", f.Name()))
			continue
		}
		for idx, m := range b.Changes {
			if prev, ok := versions[m.Revision]; ok {
				if prev.batch.BatchID != b.BatchID || !bytes.Equal(prev.batch.Payload, b.Payload) {
					invalid[m.EntryID] = true
					invalid[prev.meta.EntryID] = true
				}
				continue
			}
			x := &version{meta: m, batch: &b, index: idx}
			versions[m.Revision] = x
			groups[m.EntryID] = append(groups[m.EntryID], x)
		}
	}
	// A multi-entry batch is usable only when every parent dependency of every
	// entry is available. This prevents partial application after out-of-order sync.
	batchState := map[string]int{}
	var batchReady func(*batch) bool
	batchReady = func(b *batch) bool {
		if state := batchState[b.BatchID]; state != 0 {
			return state == 2
		}
		batchState[b.BatchID] = 1
		for _, m := range b.Changes {
			if invalid[m.EntryID] {
				batchState[b.BatchID] = 3
				return false
			}
			for _, parent := range m.Parents {
				p, ok := versions[parent]
				if !ok || p.meta.EntryID != m.EntryID || (b.Format == 1 && p.meta.Key != m.Key) || p.batch.BatchID == b.BatchID || !batchReady(p.batch) {
					batchState[b.BatchID] = 3
					return false
				}
			}
		}
		batchState[b.BatchID] = 2
		return true
	}
	v.indexVersions(groups, versions, invalid, batchReady)
	v.versions = versions
	v.batchStates = batchState
	v.issues = issues
	v.recordBytes = totalBytes
	v.recordFiles = len(files)
	v.scanErr = nil
	return nil
}
func (v *Vault) validateBatch(b *batch) error {
	if b.Format < 1 || b.Format > v.cfg.Format || b.Format > FormatVersion || b.VaultID != v.cfg.VaultID || !validID(b.BatchID) || len(b.Changes) == 0 || len(b.Changes) > MaxBatchEntries {
		return errors.New("无效批次")
	}
	seen := map[string]bool{}
	for _, m := range b.Changes {
		if err := validateMetadata(m); err != nil {
			return err
		}
		if b.Format < 3 && m.Group != "" {
			return ErrGroupUpgrade
		}
		if b.Format == 1 && m.Deleted {
			return errors.New("删除记录需要格式 2")
		}
		if seen[m.EntryID] {
			return errors.New("批次包含重复条目")
		}
		seen[m.EntryID] = true
	}
	plain, err := unseal(v.key, b.Payload, b.aad())
	if err != nil {
		return err
	}
	defer clear(plain)
	if len(plain) > MaxPayloadBytes {
		return errors.New("批次过大")
	}
	var docs []Document
	if err = decodeStrict(plain, &docs); err != nil {
		return err
	}
	if len(docs) != len(b.Changes) {
		return errors.New("批次内容数量不符")
	}
	for i, d := range docs {
		if b.Changes[i].Deleted && (d.Value != "" || len(d.Attachments) != 0 || d.Format != "text") {
			return errors.New("删除记录不能包含正文或附件")
		}
		if err = validateDocument(d); err != nil {
			return err
		}
	}
	return nil
}
func (v *Vault) document(x *version) (Document, error) {
	plain, err := unseal(v.key, x.batch.Payload, x.batch.aad())
	if err != nil {
		return Document{}, ErrUnavailable
	}
	defer clear(plain)
	var docs []Document
	if err = json.Unmarshal(plain, &docs); err != nil || x.index >= len(docs) {
		return Document{}, ErrUnavailable
	}
	return docs[x.index], nil
}
func (v *Vault) ready(key string) (*version, error) {
	it, ok := v.items[key]
	if !ok {
		return nil, ErrNotFound
	}
	if it.state == "deleted" {
		return nil, ErrDeleted
	}
	if it.state == "conflict" {
		return nil, ErrConflict
	}
	if it.state != "ready" || len(it.heads) != 1 {
		return nil, ErrUnavailable
	}
	return it.heads[0], nil
}
func summary(it *item) Summary {
	m := it.heads[0].meta
	groups := itemGroups(it)
	group := groups[0]
	var candidates []string
	if len(groups) > 1 {
		group = ""
		candidates = groups
	}
	return Summary{Group: group, CandidateGroups: candidates, EntryID: m.EntryID, Deleted: it.state == "deleted", Key: m.Key, Description: m.Description, Revision: m.Revision, ReadPolicy: m.ReadPolicy, WritePolicy: m.WritePolicy, State: it.state, Versions: it.count}
}
func (v *Vault) List(query string, group ...string) ([]Summary, error) {
	return v.list(query, false, false, group)
}
func (v *Vault) Trash(query string, group ...string) ([]Summary, error) {
	return v.list(query, false, true, group)
}

// Search is a human-only search. MCP discovery must continue to use List.
func (v *Vault) Search(query string, group ...string) ([]Summary, error) {
	return v.list(query, true, false, group)
}

func (v *Vault) list(query string, body, deleted bool, group []string) ([]Summary, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, ErrLocked
	}
	if err := v.scan(); err != nil {
		return nil, err
	}
	out := []Summary{}
	query = strings.ToLower(strings.TrimSpace(query))
	for key, it := range v.items {
		if (it.state == "deleted") != deleted {
			continue
		}
		s := summary(it)
		s.Key = key
		if len(group) > 0 && !groupMatches(s, group[0]) {
			continue
		}
		matches := query == "" || strings.Contains(strings.ToLower(s.Key+" "+s.Description+" "+strings.Join(itemGroups(it), " ")), query)
		if !matches && body && it.state == "ready" {
			doc, err := v.document(it.heads[0])
			if err != nil {
				return nil, err
			}
			matches = strings.Contains(strings.ToLower(doc.Value), query)
		}
		if matches {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
func (v *Vault) Get(key string) (Entry, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return Entry{}, ErrLocked
	}
	if err := v.scan(); err != nil {
		return Entry{}, err
	}
	x, err := v.ready(key)
	if err != nil {
		return Entry{}, err
	}
	d, err := v.document(x)
	return Entry{Metadata: x.meta, Document: d}, err
}
func (v *Vault) PrepareReads(specs []ReadSpec) ([]ReadSpec, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, false, ErrLocked
	}
	if err := v.scan(); err != nil {
		return nil, false, err
	}
	return v.prepareReads(specs)
}
func (v *Vault) prepareReads(specs []ReadSpec) ([]ReadSpec, bool, error) {
	if len(specs) == 0 || len(specs) > MaxBatchEntries {
		return nil, false, errors.New("每次请求需包含 1–20 个条目")
	}
	out := make([]ReadSpec, len(specs))
	needs := false
	seen := map[string]bool{}
	for i, s := range specs {
		if seen[s.Key] {
			return nil, false, errors.New("请求中 key 重复")
		}
		seen[s.Key] = true
		x, err := v.ready(s.Key)
		if err != nil {
			return nil, false, err
		}
		if s.Revision != "" && s.Revision != x.meta.Revision {
			return nil, false, ErrConflict
		}
		if x.meta.ReadPolicy == Deny {
			return nil, false, ErrDenied
		}
		needs = needs || x.meta.ReadPolicy == Ask
		if !s.IncludeValue && !s.AllAttachments && len(s.Attachments) == 0 {
			return nil, false, errors.New("请选择正文或附件")
		}
		if len(s.Attachments) > 16 {
			return nil, false, errors.New("单个条目最多选择 16 个附件")
		}
		for _, name := range s.Attachments {
			if name == "" || len(name) > 240 {
				return nil, false, errors.New("附件名称不正确")
			}
		}
		s.Revision = x.meta.Revision
		s.Attachments = append([]string{}, s.Attachments...)
		out[i] = s
	}
	return out, needs, nil
}
func (v *Vault) ReadMany(specs []ReadSpec, approved bool) ([]ReadResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, ErrLocked
	}
	if err := v.scan(); err != nil {
		return nil, err
	}
	pinned, needs, err := v.prepareReads(specs)
	if err != nil {
		return nil, err
	}
	if needs && !approved {
		return nil, ErrApproval
	}
	out := []ReadResult{}
	totalBytes := 0
	for _, s := range pinned {
		x, _ := v.ready(s.Key)
		d, err := v.document(x)
		if err != nil {
			return nil, err
		}
		r := ReadResult{Key: s.Key, Revision: x.meta.Revision}
		if s.IncludeValue {
			value := d.Value
			r.Value = &value
			r.Format = d.Format
		}
		wanted := map[string]bool{}
		for _, name := range s.Attachments {
			wanted[name] = true
		}
		for _, a := range d.Attachments {
			if s.AllAttachments || wanted[a.Name] {
				r.Attachments = append(r.Attachments, a)
				delete(wanted, a.Name)
			}
		}
		if len(wanted) > 0 {
			return nil, errors.New("指定附件不存在")
		}
		if r.Value != nil {
			totalBytes += len(*r.Value)
		}
		for _, a := range r.Attachments {
			totalBytes += len(a.Data)
		}
		if totalBytes > MaxPayloadBytes {
			return nil, errors.New("单次读取超过 8 MiB，请分开请求")
		}
		out = append(out, r)
	}
	return out, nil
}
func (v *Vault) CheckChanges(changes []Change, human bool) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return false, ErrLocked
	}
	if err := v.scan(); err != nil {
		return false, err
	}
	return v.checkChanges(changes, human)
}
func (v *Vault) checkChanges(changes []Change, human bool) (bool, error) {
	if len(changes) == 0 || len(changes) > MaxBatchEntries {
		return false, errors.New("每次修改需包含 1–20 个条目")
	}
	needs := false
	seen := map[string]bool{}
	for _, c := range changes {
		if seen[c.Key] {
			return false, errors.New("请求中 key 重复")
		}
		seen[c.Key] = true
		if !human && (c.ReadPolicy != nil || c.WritePolicy != nil || c.Group != nil) {
			return false, ErrDenied
		}
		if c.Group != nil {
			if v.cfg.Format < 3 && *c.Group != "" {
				return false, ErrGroupUpgrade
			}
			if err := validateGroup(*c.Group); err != nil {
				return false, err
			}
		}
		x, err := v.ready(c.Key)
		if errors.Is(err, ErrNotFound) && human && c.ExpectedRevision == "" {
			if c.Description == nil || c.Value == nil {
				return false, errors.New("新条目需要 description 与 value")
			}
			continue
		}
		if err != nil {
			return false, err
		}
		if c.ExpectedRevision == "" || c.ExpectedRevision != x.meta.Revision {
			return false, ErrConflict
		}
		if !human {
			if x.meta.WritePolicy == Deny {
				return false, ErrDenied
			}
			needs = needs || x.meta.WritePolicy == Ask
		}
		if c.Description == nil && c.Value == nil && c.Format == nil && c.Attachments == nil && c.ReadPolicy == nil && c.WritePolicy == nil && c.Group == nil {
			return false, errors.New("修改内容为空")
		}
	}
	return needs, nil
}
func (v *Vault) Commit(changes []Change, human, approved bool) ([]Summary, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, ErrLocked
	}
	if err := v.scan(); err != nil {
		return nil, err
	}
	needs, err := v.checkChanges(changes, human)
	if err != nil {
		return nil, err
	}
	if needs && !approved {
		return nil, ErrApproval
	}
	b := batch{batchHeader: batchHeader{Format: v.cfg.Format, VaultID: v.cfg.VaultID, BatchID: newID(), Changes: []Metadata{}}}
	docs := []Document{}
	for _, c := range changes {
		m := Metadata{EntryID: newID(), Revision: newID(), Parents: []string{}, Key: c.Key, ReadPolicy: Ask, WritePolicy: Ask, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		d := Document{Format: "text", Attachments: []Attachment{}}
		if x, e := v.ready(c.Key); e == nil {
			m.EntryID = x.meta.EntryID
			m.Parents = []string{x.meta.Revision}
			m.Description = x.meta.Description
			m.Group = x.meta.Group
			m.ReadPolicy = x.meta.ReadPolicy
			m.WritePolicy = x.meta.WritePolicy
			d, err = v.document(x)
			if err != nil {
				return nil, err
			}
		}
		if c.Group != nil {
			m.Group = *c.Group
		}
		if c.Description != nil {
			m.Description = *c.Description
		}
		if c.Value != nil {
			d.Value = *c.Value
		}
		if c.Format != nil {
			d.Format = *c.Format
		}
		if c.Attachments != nil {
			d.Attachments = *c.Attachments
		}
		if c.ReadPolicy != nil {
			m.ReadPolicy = *c.ReadPolicy
		}
		if c.WritePolicy != nil {
			m.WritePolicy = *c.WritePolicy
		}
		if err = validateMetadata(m); err != nil {
			return nil, err
		}
		if err = validateDocument(d); err != nil {
			return nil, err
		}
		b.Changes = append(b.Changes, m)
		docs = append(docs, d)
	}
	return v.publishBatch(b, docs)
}

func (v *Vault) publishBatch(b batch, docs []Document) ([]Summary, error) {
	plain, err := json.Marshal(docs)
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	if len(plain) > MaxPayloadBytes {
		return nil, errors.New("批次内容超过 8 MiB，请拆分")
	}
	b.Payload, err = seal(v.key, plain, b.aad())
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	if v.recordBytes+int64(len(raw)) > 64<<20 || v.recordFiles >= 4096 {
		return nil, errors.New("本次保存会超过原型记录容量，请先保留完整备份")
	}
	if err = publish(filepath.Join(v.root, "records"), b.BatchID+".json", raw); err != nil {
		return nil, err
	}
	if err = v.scan(); err != nil {
		return nil, err
	}
	out := []Summary{}
	for _, c := range b.Changes {
		if it, ok := v.items[c.Key]; ok {
			row := summary(it)
			row.Key = c.Key
			out = append(out, row)
		}
	}
	return out, nil
}
