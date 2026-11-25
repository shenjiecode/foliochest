package vault

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const MaxBackupBytes = 66 << 20

// completeSnapshot permits conflicts, but not missing parents, unverified files,
// or missing previously observed heads. Conflicts remain recoverable in a backup.
func (v *Vault) completeSnapshot() error {
	if len(v.issues) > 0 {
		return errors.New("存在未通过校验的文件，请等待同步完整；也可先手动复制整个资料库目录保留原件")
	}
	for _, it := range v.items {
		if it.state != "ready" && it.state != "conflict" && it.state != "deleted" {
			return ErrUnavailable
		}
	}
	for _, x := range v.versions {
		if v.batchStates[x.batch.BatchID] != 2 {
			return ErrUnavailable
		}
	}
	return nil
}
func (v *Vault) Backup() ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, ErrLocked
	}
	if err := v.scan(); err != nil {
		return nil, err
	}
	if err := v.completeSnapshot(); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	add := func(name string, data []byte) error {
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		h.SetMode(0600)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := add("vault.json", v.configBytes); err != nil {
		return nil, err
	}
	batches := map[string]*batch{}
	for _, x := range v.versions {
		batches[x.batch.BatchID] = x.batch
	}
	ids := make([]string, 0, len(batches))
	for id := range batches {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		raw, err := json.Marshal(batches[id])
		if err != nil {
			return nil, err
		}
		if err = add("records/"+id+".json", raw); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if buffer.Len() > MaxBackupBytes {
		return nil, errors.New("备份超过原型上限")
	}
	return buffer.Bytes(), nil
}

// RestoreBackup validates ciphertext and the password in an isolated staging
// directory, then installs it into a new/empty directory. Existing data is never
// overwritten. Only Keyfolio's archive layout is accepted; no general extraction.
func RestoreBackup(archive []byte, destination, password string) error {
	if len(archive) > MaxBackupBytes {
		return errors.New("备份超过 66 MiB")
	}
	if !filepath.IsAbs(destination) {
		return errors.New("恢复目标必须是绝对目录路径")
	}
	destination = filepath.Clean(destination)
	if err := emptyDestination(destination); err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return errors.New("无效备份 ZIP")
	}
	if len(zr.File) < 1 || len(zr.File) > 4097 {
		return errors.New("备份文件数量不正确")
	}
	seen := map[string]bool{}
	var total uint64
	hasConfig := false
	for _, f := range zr.File {
		if seen[f.Name] || !f.Mode().IsRegular() {
			return errors.New("备份包含重复或非普通文件")
		}
		seen[f.Name] = true
		limit := uint64(MaxFileBytes)
		if f.Name == "vault.json" {
			hasConfig = true
			limit = 16 << 10
		} else {
			id := strings.TrimSuffix(strings.TrimPrefix(f.Name, "records/"), ".json")
			if !validID(id) || f.Name != "records/"+id+".json" {
				return errors.New("备份包含不支持的文件路径")
			}
		}
		if f.UncompressedSize64 > limit {
			return errors.New("备份中单个文件过大")
		}
		total += f.UncompressedSize64
		if total > (64<<20)+(16<<10) {
			return errors.New("备份展开大小超过原型上限")
		}
	}
	if !hasConfig {
		return errors.New("备份缺少 vault.json")
	}
	parent := filepath.Dir(destination)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".keyfolio-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = os.Mkdir(filepath.Join(stage, "records"), 0700); err != nil {
		return err
	}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			return err
		}
		limit := int64(MaxFileBytes)
		if f.Name == "vault.json" {
			limit = 16 << 10
		}
		data, readErr := io.ReadAll(io.LimitReader(r, limit+1))
		closeErr := r.Close()
		if readErr != nil {
			return errors.New("备份文件校验失败")
		}
		if closeErr != nil {
			return closeErr
		}
		if int64(len(data)) > limit || uint64(len(data)) != f.UncompressedSize64 {
			return errors.New("备份文件大小不符")
		}
		target := filepath.Join(stage, filepath.FromSlash(f.Name))
		if err = publish(filepath.Dir(target), filepath.Base(target), data); err != nil {
			return err
		}
	}
	candidate := New(stage)
	if err = candidate.Unlock(password); err != nil {
		return err
	}
	err = candidate.completeSnapshot()
	candidate.Lock()
	if err != nil {
		return err
	}
	// Recheck after validation. Remove only an empty directory, never its contents.
	if err = emptyDestination(destination); err != nil {
		return err
	}
	if _, err = os.Lstat(destination); err == nil {
		if err = os.Remove(destination); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.Rename(stage, destination); err != nil {
		return err
	}
	if d, e := os.Open(parent); e == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
func emptyDestination(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("恢复目标不能是文件或符号链接")
	}
	files, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(files) > 0 {
		return errors.New("恢复仅支持新目录或空目录，不会覆盖已有资料")
	}
	return nil
}
