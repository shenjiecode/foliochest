package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shenjiecode/keyfolio/internal/access"
	"github.com/shenjiecode/keyfolio/internal/platform"
	"github.com/shenjiecode/keyfolio/internal/vault"
)

type enrollment struct {
	Enabled        bool          `json:"enabled"`
	Directory      string        `json:"directory,omitempty"`
	Binding        vault.Binding `json:"binding"`
	CredentialID   string        `json:"credential_id,omitempty"`
	BrowserHash    string        `json:"browser_hash,omitempty"`
	BrowserExpires time.Time     `json:"browser_expires,omitempty"`
}
type deviceState struct {
	mu             sync.Mutex
	record         enrollment
	credentials    platform.Credentials
	startup        platform.Startup
	autoGeneration uint64
	attempted      bool
	notice         string
}

func loadDevice(directory, address string) (*deviceState, error) {
	startup, err := platform.NewStartup(directory, address)
	if err != nil {
		return nil, err
	}
	d := &deviceState{credentials: platform.NativeCredentials(), startup: startup}
	path := filepath.Join(directory, "device.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<10 {
		return nil, errors.New("本机 device.json 类型或大小不受支持")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 16<<10+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 16<<10 || json.Unmarshal(raw, &d.record) != nil {
		return nil, errors.New("本机设备设置损坏，请保留后检查 device.json")
	}
	if d.record.Enabled && (d.record.CredentialID == "" || !filepath.IsAbs(d.record.Directory) || len(d.record.Binding.ConfigHash) != 64 || len(d.record.Binding.VaultID) != 32) {
		return nil, errors.New("本机设备设置不完整，请保留后检查 device.json")
	}
	return d, nil
}
func (s *Server) saveEnrollment(record enrollment) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return writeLocal(filepath.Join(s.deviceDir, "device.json"), raw)
}
func (s *Server) deviceNotice() string {
	s.device.mu.Lock()
	defer s.device.mu.Unlock()
	return s.device.notice
}
func browserHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
func (s *Server) trustCookieName() string { return "kf_trusted_" + platform.DeviceID(s.deviceDir) }
func (s *Server) trustedBrowser(r *http.Request) bool {
	cookie, err := r.Cookie(s.trustCookieName())
	if err != nil || len(cookie.Value) != 48 {
		return false
	}
	d := s.device
	return d.record.Enabled && time.Now().Before(d.record.BrowserExpires) && subtle.ConstantTimeCompare([]byte(d.record.BrowserHash), []byte(browserHash(cookie.Value))) == 1
}
func (s *Server) clearTrustCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: s.trustCookieName(), Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

// Authenticate only a separately enrolled browser, and only for the generation
// unlocked at process startup (or explicitly enrolled by a password-confirmed UI).
func (s *Server) authenticateTrusted(r *http.Request, sessionID string, session *webSession) {
	if s.authorized(session) {
		return
	}
	d := s.device
	d.mu.Lock()
	defer d.mu.Unlock()
	generation := s.Vault.Generation()
	if d.autoGeneration == 0 || d.autoGeneration != generation || s.Vault.Locked() || !s.trustedBrowser(r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if stored := s.sessions[sessionID]; stored != nil {
		stored.Authenticated = true
		stored.Generation = generation
		session.Authenticated = true
		session.Generation = generation
	}
}

// TryAutoUnlock is one-shot. It does not retry after manual/idle locking and
// never grants authentication to a new browser just because the vault is open.
func (s *Server) TryAutoUnlock(ctx context.Context) {
	d := s.device
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.attempted {
		return
	}
	d.attempted = true
	if !d.record.Enabled {
		return
	}
	sequence, err := s.beginUnlock(true)
	if err != nil {
		d.notice = err.Error()
		return
	}
	root, err := platform.Canonical(s.Vault.Status().Directory)
	if err != nil || root != d.record.Directory {
		d.notice = "资料库目录已变化，本次未自动解锁；请使用主密码并重新授权设备。"
		return
	}
	raw, err := d.credentials.Get(ctx, d.record.CredentialID)
	if err != nil {
		d.notice = "设备自动解锁失败：系统凭据不可用或已锁定，请使用主密码。"
		return
	}
	defer clear(raw)
	if err = s.Vault.UnlockDevice(raw); err != nil {
		d.notice = "设备凭据与资料库不匹配或文件不完整，请使用主密码并重新授权设备。"
		return
	}
	if err = s.finishUnlock(sequence); err != nil {
		d.notice = err.Error()
		return
	}
	binding, err := s.Vault.Binding()
	if err != nil || binding != d.record.Binding {
		s.Access.Lock()
		d.notice = "资料库配置已变化，请使用主密码重新授权设备。"
		return
	}
	d.autoGeneration = s.Vault.Generation()
	s.activity()
}

// Caller holds device.mu. Persist a disabled marker BEFORE deleting a credential
// so a locked/unavailable keyring cannot silently leave auto-unlock enabled.
func (s *Server) disableDevice(ctx context.Context) (string, error) {
	d := s.device
	disabled := d.record
	disabled.Enabled = false
	disabled.BrowserHash = ""
	disabled.BrowserExpires = time.Time{}
	d.autoGeneration = 0
	if err := s.saveEnrollment(disabled); err != nil {
		return "", errors.New("无法保存停用状态，未完成操作；请检查本机配置目录权限")
	}
	d.record = disabled
	if d.record.CredentialID == "" {
		return "", nil
	}
	if err := d.credentials.Delete(ctx, d.record.CredentialID); err != nil && !errors.Is(err, platform.ErrNotFound) {
		return "自动解锁已关闭，但系统中的旧设备凭据尚未删除。解锁系统凭据库后，再次点击清理。", nil
	}
	if err := s.saveEnrollment(enrollment{}); err != nil {
		return "旧系统凭据已删除，但本机清理标记未更新，请稍后再次清理。", nil
	}
	d.record = enrollment{}
	return "", nil
}
func (s *Server) serveDevice(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/device":
		if !method(w, r, "GET") {
			return true
		}
		binding, bindingErr := s.Vault.Binding()
		d := s.device
		d.mu.Lock()
		defer d.mu.Unlock()
		startup, err := d.startup.Status()
		startupError := ""
		if err != nil {
			startupError = "无法读取当前系统启动项，请检查系统设置。"
		}
		directory, e := platform.Canonical(s.Vault.Status().Directory)
		writeJSON(w, map[string]any{"auto_unlock": d.record.Enabled, "matches_vault": bindingErr == nil && e == nil && directory == d.record.Directory && binding == d.record.Binding, "remembered_browser": s.trustedBrowser(r), "cleanup_pending": !d.record.Enabled && d.record.CredentialID != "", "provider": platform.CredentialProviderName(), "startup": startup, "startup_error": startupError, "notice": d.notice})
	case "/api/device/startup":
		if !method(w, r, "POST") {
			return true
		}
		var in struct {
			Enabled bool `json:"enabled"`
		}
		if !decode(w, r, &in) {
			return true
		}
		if in.Enabled {
			raw, _ := json.Marshal(map[string]string{"directory": s.Vault.Status().Directory})
			if err := writeLocal(filepath.Join(s.deviceDir, "location.json"), raw); err != nil {
				apiError(w, err, 500)
				return true
			}
		}
		if err := s.device.startup.Set(in.Enabled); err != nil {
			apiError(w, err, 400)
		} else {
			s.activity()
			writeJSON(w, map[string]bool{"ok": true})
		}
	case "/api/device/unlock":
		if !method(w, r, "POST") {
			return true
		}
		var in struct {
			Enabled         bool   `json:"enabled"`
			Password        string `json:"password"`
			RememberBrowser bool   `json:"remember_browser"`
		}
		if !decode(w, r, &in) {
			return true
		}
		defer func() { in.Password = "" }()
		d := s.device
		d.mu.Lock()
		defer d.mu.Unlock()
		if !in.Enabled {
			warning, err := s.disableDevice(r.Context())
			s.clearTrustCookie(w)
			if err != nil {
				apiError(w, err, 500)
			} else {
				d.notice = warning
				writeJSON(w, map[string]any{"ok": true, "warning": warning})
			}
			return true
		}
		binding, raw, err := s.Vault.DeviceCredential(in.Password)
		if err != nil {
			apiError(w, err, 400)
			return true
		}
		defer clear(raw)
		generation := s.Vault.Generation()
		directory, err := platform.Canonical(s.Vault.Status().Directory)
		if err != nil {
			apiError(w, err, 400)
			return true
		}
		if err = platform.SeparateDirectories(directory, s.deviceDir); err != nil {
			apiError(w, err, 400)
			return true
		}
		warning, err := s.disableDevice(r.Context())
		s.clearTrustCookie(w)
		if err != nil {
			apiError(w, err, 500)
			return true
		}
		if warning != "" {
			d.notice = warning
			apiError(w, errors.New(warning), 409)
			return true
		}
		// Track the fresh ID before writing the OS secret, including ambiguous failures.
		d.record = enrollment{CredentialID: access.ID(), Directory: directory, Binding: binding}
		if err = s.saveEnrollment(d.record); err != nil {
			apiError(w, err, 500)
			return true
		}
		if err = d.credentials.Set(r.Context(), d.record.CredentialID, raw); err != nil {
			apiError(w, platform.ErrUnavailable, 400)
			return true
		}
		checked, err := s.Vault.Binding()
		if err != nil || checked != binding || generation != s.Vault.Generation() {
			_, _ = s.disableDevice(r.Context())
			apiError(w, vault.ErrChanged, 409)
			return true
		}
		next := d.record
		next.Enabled = true
		token := ""
		if in.RememberBrowser {
			token = access.ID()
			next.BrowserHash = browserHash(token)
			next.BrowserExpires = time.Now().Add(90 * 24 * time.Hour)
		}
		if err = s.saveEnrollment(next); err != nil {
			_, _ = s.disableDevice(r.Context())
			apiError(w, err, 500)
			return true
		}
		d.record = next
		d.autoGeneration = generation
		d.notice = ""
		if token != "" {
			http.SetCookie(w, &http.Cookie{Name: s.trustCookieName(), Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 90 * 24 * 60 * 60, Expires: next.BrowserExpires})
		}
		s.activity()
		writeJSON(w, map[string]bool{"ok": true})
	case "/api/vault/upgrade":
		if !method(w, r, "POST") {
			return true
		}
		var in struct {
			Password  string `json:"password"`
			Confirmed bool   `json:"confirmed"`
		}
		if !decode(w, r, &in) {
			return true
		}
		defer func() { in.Password = "" }()
		if !in.Confirmed {
			apiError(w, errors.New("请先备份并确认已更新所有设备程序、暂停其他设备写入"), 400)
			return true
		}
		if s.Vault.Status().Format != 1 {
			apiError(w, errors.New("资料库不需要升级"), 409)
			return true
		}
		if err := s.Vault.VerifyPassword(in.Password); err != nil {
			apiError(w, err, 400)
			return true
		}
		d := s.device
		d.mu.Lock()
		defer d.mu.Unlock()
		warning, err := s.disableDevice(r.Context())
		s.clearTrustCookie(w)
		if err != nil {
			apiError(w, err, 500)
			return true
		}
		if err = s.Vault.UpgradeFormat(in.Password); err != nil {
			apiError(w, errors.New("格式升级未完成；本机自动解锁已关闭。"+err.Error()), 409)
			return true
		}
		s.Access.Lock()
		d.notice = warning
		writeJSON(w, map[string]any{"ok": true, "warning": warning})
	case "/api/vault/password":
		if !method(w, r, "POST") {
			return true
		}
		var in struct {
			Current string `json:"current_password"`
			Next    string `json:"new_password"`
		}
		if !decode(w, r, &in) {
			return true
		}
		defer func() { in.Current = ""; in.Next = "" }()
		if err := vault.ValidatePassword(in.Next); err != nil {
			apiError(w, err, 400)
			return true
		}
		if in.Current == in.Next {
			apiError(w, errors.New("新主密码不能与当前主密码相同"), 400)
			return true
		}
		if err := s.Vault.VerifyPassword(in.Current); err != nil {
			apiError(w, err, 400)
			return true
		}
		d := s.device
		d.mu.Lock()
		defer d.mu.Unlock()
		warning, err := s.disableDevice(r.Context())
		s.clearTrustCookie(w)
		if err != nil {
			apiError(w, err, 500)
			return true
		}
		if err = s.Vault.ChangePassword(in.Current, in.Next); err != nil {
			apiError(w, errors.New("主密码变更未完成；本机自动解锁已关闭。"+err.Error()), 409)
			return true
		}
		s.Access.Lock()
		d.notice = warning
		writeJSON(w, map[string]any{"ok": true, "warning": warning})
	default:
		return false
	}
	return true
}
