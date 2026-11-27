package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/shenjiecode/keyfolio/internal/access"
	"github.com/shenjiecode/keyfolio/internal/buildinfo"
	"github.com/shenjiecode/keyfolio/internal/updater"
)

type preparedUpdate struct {
	ID         string
	Generation uint64
	Package    updater.Package
}

func installerKind() string {
	if runtime.GOOS == "windows" {
		return "exe"
	}
	if runtime.GOOS == "linux" {
		raw, _ := os.ReadFile("/etc/os-release")
		return updater.DetectLinuxKind(string(raw))
	}
	return ""
}
func releaseClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (s *Server) downloadUpdate(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	var in struct {
		Version   string `json:"version"`
		Confirmed bool   `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.Confirmed {
		apiError(w, errors.New("请确认下载并信任 GitHub 发布的安装包"), 400)
		return
	}
	if s.InstallUpdate == nil {
		apiError(w, errors.New("当前运行方式不支持引导更新，请打开下载页"), 409)
		return
	}
	gen := s.Vault.Generation()
	ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
	defer cancel()
	latest, err := latestRelease(ctx, releaseClient(), buildinfo.Version)
	if err != nil {
		apiError(w, err, 502)
		return
	}
	if latest.Latest != in.Version || latest.Status != "available" || latest.Installer == "" {
		apiError(w, errors.New("版本已变化、不是更新版本或没有适用安装包，请重新检查更新"), 409)
		return
	}
	pkg, err := updater.Download(ctx, updater.NewClient(), filepath.Join(s.deviceDir, "updates"), latest.Latest, runtime.GOOS, runtime.GOARCH, installerKind(), latest.Size)
	if err != nil {
		apiError(w, err, 502)
		return
	}
	if gen != s.Vault.Generation() || s.Vault.Locked() {
		_ = os.RemoveAll(filepath.Dir(pkg.Path))
		apiError(w, errors.New("下载期间资料库已锁定或会话变化，请重新解锁后重试"), 409)
		return
	}
	if old := s.preparedUpdate; old != nil {
		_ = os.RemoveAll(filepath.Dir(old.Package.Path))
	}
	s.preparedUpdate = &preparedUpdate{ID: access.ID(), Generation: gen, Package: pkg}
	writeJSON(w, map[string]any{"id": s.preparedUpdate.ID, "version": pkg.Version, "name": pkg.Name, "size": pkg.Size, "path": pkg.Path})
}
func (s *Server) installUpdate(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	var in struct {
		ID        string `json:"id"`
		Confirmed bool   `json:"confirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	ready := s.preparedUpdate
	if !in.Confirmed || ready == nil || in.ID != ready.ID || ready.Generation != s.Vault.Generation() {
		apiError(w, errors.New("请重新下载并确认更新；旧会话的安装请求无效"), 409)
		return
	}
	if s.InstallUpdate == nil || s.Shutdown == nil {
		apiError(w, errors.New("无法启动安装器，请手动安装已下载文件"), 409)
		return
	}
	if err := ready.Package.Verify(); err != nil {
		apiError(w, err, 409)
		return
	}
	// Stop all data access before launching the helper; failure keeps the app locked.
	s.Access.Lock()
	if err := s.InstallUpdate(ready.Package); err != nil {
		apiError(w, err, 500)
		return
	}
	s.preparedUpdate = nil
	writeJSON(w, map[string]bool{"ok": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.Shutdown()
}
