package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shenjiecode/keyfolio/internal/fsutil"
	"github.com/shenjiecode/keyfolio/internal/platform"
)

func rememberedLocation(directory, device string) bool {
	if directory == "" {
		return false
	}
	raw, err := fsutil.ReadBounded(filepath.Join(device, "location.json"), 64<<10)
	if err != nil {
		return false
	}
	var saved struct {
		Directory string `json:"directory"`
	}
	if json.Unmarshal(raw, &saved) != nil || saved.Directory == "" {
		return false
	}
	a, err := platform.Canonical(directory)
	if err != nil {
		return false
	}
	b, err := platform.Canonical(saved.Directory)
	if err != nil {
		return false
	}
	return a == b || runtime.GOOS == "windows" && strings.EqualFold(a, b)
}

func (s *Server) saveLocation(directory string) error {
	raw, _ := json.Marshal(map[string]string{"directory": filepath.Clean(directory)})
	return fsutil.WriteFile(filepath.Join(s.deviceDir, "location.json"), raw)
}
func (s *Server) createVault(directory, password string, session *webSession) error {
	if !s.Vault.Locked() && !s.authorized(session) {
		return errors.New("请先解锁当前网页会话")
	}
	if !filepath.IsAbs(directory) {
		return errors.New("请先选择新资料库的绝对目录路径")
	}
	target, err := platform.Canonical(directory)
	if err != nil {
		return err
	}
	current := s.Vault.Status().Directory
	if current != "" {
		current, err = platform.Canonical(current)
		if err != nil {
			return err
		}
		same := current == target || runtime.GOOS == "windows" && strings.EqualFold(current, target)
		if same && (s.knownVault.Load() || s.Vault.Status().Exists) {
			return errors.New("当前已选位置不能重新初始化；请打开已有资料库，或选择另一个新目录")
		}
	}
	if err := platform.SeparateDirectories(target, s.deviceDir); err != nil {
		return err
	}
	err = s.unlockWith(func() error { return s.Vault.CreateAt(target, password) })
	// A system event may lock a newly created vault. Still remember its location,
	// never present it as an unselected/empty destination on restart.
	if status := s.Vault.Status(); status.Directory == target && status.Exists {
		s.knownVault.Store(true)
		if saveErr := s.saveLocation(target); saveErr != nil {
			s.Access.Lock()
			return errors.New("资料库已创建，但本机位置保存失败；请检查本机配置目录权限，并手动打开所选目录")
		}
		s.Access.Sweep()
	}
	return err
}

func (s *Server) selectVault(w http.ResponseWriter, r *http.Request, session *webSession) {
	if !method(w, r, "POST") {
		return
	}
	if !s.Vault.Locked() && !s.authorized(session) {
		apiError(w, errors.New("请先解锁当前网页会话"), 401)
		return
	}
	var in struct {
		Directory string `json:"directory"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !filepath.IsAbs(in.Directory) {
		apiError(w, errors.New("请输入绝对目录路径"), 400)
		return
	}
	if err := platform.SeparateDirectories(in.Directory, s.deviceDir); err != nil {
		apiError(w, err, 400)
		return
	}
	if _, err := fsutil.ReadBounded(filepath.Join(in.Directory, "vault.json"), 16<<10); err != nil {
		apiError(w, errors.New("所选目录没有可读取的资料库，请检查位置、权限或同步状态"), 400)
		return
	}
	if err := s.saveLocation(in.Directory); err != nil {
		apiError(w, err, 500)
		return
	}
	if err := s.Vault.SelectDirectory(in.Directory); err != nil {
		apiError(w, err, 400)
		return
	}
	s.knownVault.Store(true)
	s.Access.Sweep()
	writeJSON(w, map[string]bool{"ok": true})
}
