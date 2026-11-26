package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shenjiecode/keyfolio/internal/access"
	"github.com/shenjiecode/keyfolio/internal/fsutil"
	"github.com/shenjiecode/keyfolio/internal/platform"
	"github.com/shenjiecode/keyfolio/internal/vault"
)

type webSession struct {
	CSRF          string
	Authenticated bool
	Generation    uint64
	Expires       time.Time
}
type Server struct {
	Vault       *vault.Vault
	Access      *access.Service
	baseURL     string
	host        string
	deviceDir   string
	clients     *clients
	mu          sync.Mutex
	sessions    map[string]*webSession
	mcpHandlers map[string]http.Handler
	static      fs.FS
	system      *systemState
	idle        *idleState
	device      *deviceState
	maintenance sync.Mutex
}

func New(v *vault.Vault, deviceDir, baseURL string, static fs.FS) (*Server, error) {
	deviceDir, err := platform.Canonical(deviceDir)
	if err != nil {
		return nil, err
	}
	if err = platform.SeparateDirectories(v.Status().Directory, deviceDir); err != nil {
		return nil, err
	}
	c, err := newClients(deviceDir)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	idle, err := loadIdle(deviceDir)
	if err != nil {
		return nil, err
	}
	device, err := loadDevice(deviceDir, u.Host)
	if err != nil {
		return nil, err
	}
	return &Server{system: &systemState{heartbeat: time.Now(), now: time.Now}, device: device, idle: idle, Vault: v, Access: access.New(v, baseURL), baseURL: baseURL, host: u.Host, deviceDir: deviceDir, clients: c, sessions: map[string]*webSession{}, mcpHandlers: map[string]http.Handler{}, static: static}, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if r.Host != s.host {
		http.Error(w, "invalid host", http.StatusForbidden)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != s.baseURL {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}
	if r.URL.Path != "/" && r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "cross-site request denied", http.StatusForbidden)
		return
	}
	s.SystemHeartbeat()
	s.ExpireIdle()
	if r.URL.Path == "/mcp" {
		s.serveMCP(w, r)
		return
	}
	if r.URL.Path == "/favicon.ico" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "method not allowed", 405)
			return
		}
		if r.URL.Path == "/" {
			s.session(w, r, true)
		}
		http.FileServer(http.FS(s.static)).ServeHTTP(w, r)
		return
	}
	sessionID, session := s.session(w, r, r.Method == "GET" && r.URL.Path == "/api/state")
	if session == nil {
		apiError(w, errors.New("请先打开资料库网页"), http.StatusUnauthorized)
		return
	}
	s.authenticateTrusted(r, sessionID, session)
	if r.Method != "GET" {
		csrf := r.Header.Get("X-FolioChest-CSRF")
		if csrf == "" {
			csrf = r.Header.Get("X-Keyfolio-CSRF")
		} // compatibility with existing clients
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || subtle.ConstantTimeCompare([]byte(csrf), []byte(session.CSRF)) != 1 {
			apiError(w, errors.New("请求校验失败，请刷新网页"), 403)
			return
		}
	}
	if r.Method != "GET" && r.URL.Path != "/api/activity" && r.URL.Path != "/api/vault/lock" {
		if !s.maintenance.TryLock() {
			apiError(w, errors.New("另一个网页操作尚未完成，请稍后重试"), 409)
			return
		}
		defer s.maintenance.Unlock()
	}
	switch r.URL.Path {
	case "/api/state":
		if !method(w, r, "GET") {
			return
		}
		status := s.Vault.Status()
		authenticated := session.Authenticated && session.Generation == s.Vault.Generation() && status.Unlocked
		if !authenticated {
			status.Entries = 0
			status.Issues = []string{}
		}
		writeJSON(w, map[string]any{"vault": status, "authenticated": authenticated, "csrf": session.CSRF, "version": "0.5.0-dev", "device_notice": s.deviceNotice(), "system": s.SystemStatus()})
	case "/api/vault/create", "/api/vault/unlock":
		if !method(w, r, "POST") {
			return
		}
		var in struct {
			Password string `json:"password"`
		}
		if !decode(w, r, &in) {
			return
		}
		err := s.unlockPassword(in.Password, r.URL.Path == "/api/vault/create")
		if err != nil {
			apiError(w, err, 400)
			return
		}
		s.mu.Lock()
		if cookie, e := r.Cookie("kf_session"); e == nil {
			if ss := s.sessions[cookie.Value]; ss != nil {
				ss.Authenticated = true
				ss.Generation = s.Vault.Generation()
			}
		}
		s.mu.Unlock()
		s.activity()
		writeJSON(w, map[string]bool{"ok": true})
	case "/api/vault/restore":
		if !s.Vault.Locked() && !s.authorized(session) {
			apiError(w, vault.ErrLocked, 401)
			return
		}
		s.restoreBackup(w, r)
	case "/api/vault/select":
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
		if err := s.Vault.SelectDirectory(in.Directory); err != nil {
			apiError(w, err, 400)
			return
		}
		data, _ := json.Marshal(map[string]string{"directory": filepath.Clean(in.Directory)})
		if err := fsutil.WriteFile(filepath.Join(s.deviceDir, "location.json"), data); err != nil {
			apiError(w, err, 500)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		if !s.authorized(session) {
			apiError(w, vault.ErrLocked, 401)
			return
		}
		s.serveAuthenticated(w, r)
	}
}
func (s *Server) authorized(session *webSession) bool {
	return session.Authenticated && session.Generation == s.Vault.Generation() && !s.Vault.Locked()
}
func (s *Server) session(w http.ResponseWriter, r *http.Request, create bool) (string, *webSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, ss := range s.sessions {
		if now.After(ss.Expires) || now.Round(0).After(ss.Expires.Round(0)) {
			delete(s.sessions, id)
		}
	}
	if cookie, err := r.Cookie("kf_session"); err == nil {
		if ss, ok := s.sessions[cookie.Value]; ok {
			copySession := *ss
			return cookie.Value, &copySession
		}
	}
	if !create || len(s.sessions) >= 64 {
		return "", nil
	}
	id := access.ID()
	ss := &webSession{CSRF: access.ID(), Expires: now.Add(8 * time.Hour)}
	s.sessions[id] = ss
	http.SetCookie(w, &http.Cookie{Name: "kf_session", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 8 * 60 * 60})
	copySession := *ss
	return id, &copySession
}
func (s *Server) serveAuthenticated(w http.ResponseWriter, r *http.Request) {
	if s.serveGroups(w, r) {
		return
	}
	if s.serveDevice(w, r) {
		return
	}
	if s.serveMaintenance(w, r) {
		return
	}
	switch r.URL.Path {
	case "/api/vault/lock":
		if !method(w, r, "POST") {
			return
		}
		s.Access.Lock()
		writeJSON(w, map[string]bool{"ok": true})
	case "/api/entries":
		if !method(w, r, "GET") {
			return
		}
		query := r.URL.Query()
		options := vault.ListOptions{Query: query.Get("q")}
		switch query.Get("scope") {
		case "trash":
			options.Scope = vault.ListTrash
		case "body":
			options.Scope = vault.ListBody
		}
		if query.Has("group") {
			group := query.Get("group")
			options.Group = &group
		}
		out, err := s.Vault.List(options)
		if err != nil {
			apiError(w, err, 409)
			return
		}
		writeJSON(w, out)
	case "/api/entry":
		if !method(w, r, "GET") {
			return
		}
		out, err := s.Vault.Get(r.URL.Query().Get("key"))
		if err != nil {
			apiError(w, err, 409)
			return
		}
		writeJSON(w, out)
	case "/api/entries/save":
		if !method(w, r, "POST") {
			return
		}
		var in struct {
			Changes []vault.Change `json:"changes"`
		}
		if !decode(w, r, &in) {
			return
		}
		out, err := s.Vault.Commit(in.Changes, true, true)
		if err != nil {
			apiError(w, err, 409)
			return
		}
		writeJSON(w, out)
	case "/api/approvals":
		if !method(w, r, "GET") {
			return
		}
		writeJSON(w, s.Access.Pending())
	case "/api/approvals/decide":
		if !method(w, r, "POST") {
			return
		}
		var in struct {
			ID     string `json:"id"`
			Accept bool   `json:"accept"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.Access.Decide(in.ID, in.Accept); err != nil {
			apiError(w, err, 409)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	case "/api/clients":
		if !method(w, r, "GET") {
			return
		}
		writeJSON(w, s.clients.All())
	case "/api/clients/create":
		if !method(w, r, "POST") {
			return
		}
		var in struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		client, token, err := s.clients.Add(in.Name)
		if err != nil {
			apiError(w, err, 400)
			return
		}
		writeJSON(w, map[string]any{"client": client, "token": token, "url": s.baseURL + "/mcp"})
	case "/api/clients/revoke":
		if !method(w, r, "POST") {
			return
		}
		var in struct {
			ID string `json:"id"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.clients.Delete(in.ID); err != nil {
			apiError(w, err, 500)
			return
		}
		s.Access.RevokeClient(in.ID)
		s.mu.Lock()
		delete(s.mcpHandlers, in.ID)
		s.mu.Unlock()
		writeJSON(w, map[string]bool{"ok": true})
	default:
		apiError(w, errors.New("接口不存在"), 404)
	}
}
func method(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method != want {
		w.Header().Set("Allow", want)
		apiError(w, errors.New("不支持此请求方法"), 405)
		return false
	}
	return true
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeLimit(w, r, dst, vault.MaxFileBytes)
}
func decodeLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		apiError(w, errors.New("请求格式不正确或内容过大"), 400)
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		apiError(w, errors.New("请求包含额外内容"), 400)
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
