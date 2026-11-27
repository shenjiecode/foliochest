package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/shenjiecode/keyfolio/internal/fsutil"
	"github.com/shenjiecode/keyfolio/internal/platform"
	"github.com/shenjiecode/keyfolio/internal/vault"
)

var errSingleVault = errors.New("资料匣已绑定一个资料库，不支持创建或切换其他库；请在设置中迁移保存位置或恢复本库备份")

func (s *Server) boundID() string {
	if id := s.singleVaultID.Load(); id != nil {
		return id.(string)
	}
	return ""
}
func (s *Server) bindID(id string) error {
	if id == "" {
		return errors.New("无法确认资料库身份")
	}
	if previous := s.boundID(); previous != "" {
		if previous != id {
			return errSingleVault
		}
		return nil
	}
	raw, _ := json.Marshal(map[string]string{"vault_id": id})
	if err := fsutil.WriteFile(filepath.Join(s.deviceDir, "single-vault.json"), raw); err != nil {
		return err
	}
	s.singleVaultID.Store(id)
	s.knownVault.Store(true)
	return nil
}
func (s *Server) initSingleVault() error {
	raw, err := fsutil.ReadBounded(filepath.Join(s.deviceDir, "single-vault.json"), 4096)
	if err == nil {
		var saved struct {
			VaultID string `json:"vault_id"`
		}
		if json.Unmarshal(raw, &saved) != nil || saved.VaultID == "" {
			return errors.New("本机单库绑定记录损坏，请保留配置并检查文件")
		}
		s.singleVaultID.Store(saved.VaultID)
		s.knownVault.Store(true)
	} else if !os.IsNotExist(err) {
		return err
	}
	status := s.Vault.Status()
	if status.Exists {
		id, err := vault.IdentityAt(status.Directory)
		// Keep the recovery screen available for a damaged/missing header.
		if err == nil {
			return s.bindID(id)
		}
	}
	return nil
}
func (s *Server) hasBoundVault() bool {
	return s.knownVault.Load() || s.boundID() != "" || s.Vault.Status().Exists
}

// Activation is a maintenance operation, never a routine vault switch.
// The validated copy remains on disk on failure; original data is never deleted.
func (s *Server) activateSingleVault(w http.ResponseWriter, r *http.Request, target, id string, gen uint64) error {
	if s.Vault.Generation() != gen {
		return errors.New("资料库状态已变化；目标副本已保留，未更改保存位置")
	}
	if err := s.bindID(id); err != nil {
		return err
	}
	s.device.mu.Lock()
	warning, err := s.disableDevice(r.Context())
	s.device.notice = warning
	s.device.mu.Unlock()
	s.clearTrustCookie(w)
	if err != nil {
		return err
	}
	if s.Vault.Generation() != gen {
		return errors.New("资料库状态已变化；目标副本已保留，未更改保存位置")
	}
	oldDirectory := s.Vault.Status().Directory
	if err = s.saveLocation(target); err != nil {
		return errors.New("目标资料已准备好，但保存位置失败；原位置未变更")
	}
	if err = s.Vault.SelectDirectory(target); err != nil {
		s.Access.Lock()
		if rollbackErr := s.saveLocation(oldDirectory); rollbackErr != nil {
			return errors.New("位置更新及回退失败，请检查本机 location.json；原资料未删除")
		}
		return err
	}
	s.Access.Sweep()
	return nil
}
func (s *Server) migrateVault(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	var in struct {
		Directory string `json:"directory"`
		Password  string `json:"password"`
		Confirmed bool   `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirmed {
		apiError(w, errors.New("请确认迁移保存位置"), 400)
		return
	}
	if !filepath.IsAbs(in.Directory) {
		apiError(w, errors.New("请输入绝对目录路径"), 400)
		return
	}
	target, err := platform.Canonical(in.Directory)
	if err == nil {
		err = platform.SeparateDirectories(target, s.deviceDir)
	}
	if err != nil {
		apiError(w, err, 400)
		return
	}
	if err = platform.SeparateDirectories(target, s.Vault.Status().Directory); err != nil {
		apiError(w, errors.New("新位置不能与原资料目录相同或互相包含"), 400)
		return
	}
	if err = vault.ValidateRestoreDestination(target); err != nil {
		apiError(w, err, 400)
		return
	}
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	// Capture local group names before locking; they are not part of the ZIP.
	var reg groupRegistry
	if s.Vault.Status().Format >= 3 {
		_, reg, _, err = s.loadGroupRegistry()
		if err != nil {
			apiError(w, err, 409)
			return
		}
	}
	archive, gen, err := s.Vault.BackupAndLock(in.Password)
	if err != nil {
		apiError(w, err, 409)
		return
	}
	defer clear(archive)
	completed := false
	defer func() {
		if !completed {
			s.device.mu.Lock()
			s.device.notice = "迁移未完成，资料库已锁定。原文件未删除；请检查当前保存位置、目标目录及权限后重新解锁。"
			s.device.mu.Unlock()
		}
	}()
	s.Access.Sweep()
	id := s.boundID()
	if err = vault.RestoreBackupForVault(archive, target, in.Password, id); err != nil {
		apiError(w, err, 400)
		return
	}
	if len(reg.Names) > 0 {
		raw, _ := json.Marshal(reg)
		if err = fsutil.WriteFile(s.groupRegistryPath(id, target), raw); err != nil {
			apiError(w, errors.New("资料副本已生成，但空组迁移失败；原位置未变更"), 500)
			return
		}
	}
	if err = s.activateSingleVault(w, r, target, id, gen); err != nil {
		apiError(w, err, 409)
		return
	}
	completed = true
	s.device.mu.Lock()
	s.device.notice = "保存位置已迁移，原目录已保留；自动解锁已关闭，请重新解锁。" + s.device.notice
	s.device.mu.Unlock()
	writeJSON(w, map[string]bool{"ok": true})
}
