package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shenjiecode/keyfolio/internal/vault"
)

type DeviceSettings struct {
	IdleLockMinutes int `json:"idle_lock_minutes"`
}
type idleState struct {
	mu           sync.Mutex
	settings     DeviceSettings
	lastActivity time.Time
	now          func() time.Time
}

func loadIdle(dir string) (*idleState, error) {
	state := &idleState{now: time.Now, lastActivity: time.Now()}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 4096 || json.Unmarshal(raw, &state.settings) != nil || !validMinutes(state.settings.IdleLockMinutes) {
		return nil, errors.New("本机自动锁定设置损坏，请保留文件后检查 settings.json")
	}
	return state, nil
}
func validMinutes(n int) bool { return n >= 0 && n <= 1440 }
func (s *Server) activity() {
	s.idle.mu.Lock()
	defer s.idle.mu.Unlock()
	s.idle.lastActivity = s.idle.now()
}
func (s *Server) ExpireIdle() {
	s.idle.mu.Lock()
	defer s.idle.mu.Unlock()
	limit := time.Duration(s.idle.settings.IdleLockMinutes) * time.Minute
	if limit > 0 && s.idle.now().Sub(s.idle.lastActivity) >= limit && !s.Vault.Locked() {
		s.Access.Lock()
	}
}
func (s *Server) serveMaintenance(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/settings":
		if r.Method == "GET" {
			s.idle.mu.Lock()
			out := s.idle.settings
			s.idle.mu.Unlock()
			writeJSON(w, out)
			return true
		}
		if !method(w, r, "POST") {
			return true
		}
		var in DeviceSettings
		if !decode(w, r, &in) {
			return true
		}
		if !validMinutes(in.IdleLockMinutes) {
			apiError(w, errors.New("自动锁定需为 0–1440 分钟；0 表示关闭"), 400)
			return true
		}
		raw, _ := json.Marshal(in)
		s.idle.mu.Lock()
		err := writeLocal(filepath.Join(s.deviceDir, "settings.json"), raw)
		if err == nil {
			s.idle.settings = in
			s.idle.lastActivity = s.idle.now()
		}
		s.idle.mu.Unlock()
		if err != nil {
			apiError(w, err, 500)
		} else {
			writeJSON(w, in)
		}
	case "/api/activity":
		if !method(w, r, "POST") {
			return true
		}
		s.activity()
		writeJSON(w, map[string]bool{"ok": true})
	case "/api/history":
		if !method(w, r, "GET") {
			return true
		}
		out, err := s.Vault.History(r.URL.Query().Get("key"))
		if err != nil {
			apiError(w, err, 409)
		} else {
			writeJSON(w, out)
		}
	case "/api/history/entry":
		if !method(w, r, "GET") {
			return true
		}
		out, err := s.Vault.HistoricalEntry(r.URL.Query().Get("key"), r.URL.Query().Get("revision"))
		if err != nil {
			apiError(w, err, 409)
		} else {
			writeJSON(w, out)
		}
	case "/api/history/restore":
		if !method(w, r, "POST") {
			return true
		}
		var in vault.RestoreChange
		if !decode(w, r, &in) {
			return true
		}
		out, err := s.Vault.Restore(in)
		if err != nil {
			apiError(w, err, 409)
		} else {
			s.activity()
			writeJSON(w, out)
		}
	case "/api/backup":
		if !method(w, r, "POST") {
			return true
		}
		out, err := s.Vault.Backup()
		if err != nil {
			apiError(w, err, 409)
			return true
		}
		s.activity()
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=keyfolio-backup-"+time.Now().UTC().Format("20060102-150405")+".zip")
		_, _ = w.Write(out)
	default:
		return false
	}
	return true
}
func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	var in struct {
		Archive   []byte `json:"archive"`
		Directory string `json:"directory"`
		Password  string `json:"password"`
	}
	// Base64 expands the encrypted archive; this limit applies only to restoration.
	if !decodeLimit(w, r, &in, 90<<20) {
		return
	}
	defer clear(in.Archive)
	if err := vault.RestoreBackup(in.Archive, in.Directory, in.Password); err != nil {
		apiError(w, err, 400)
		return
	}
	in.Password = ""
	// Do not switch the active vault or persist a new location on a failed restore.
	raw, _ := json.Marshal(map[string]string{"directory": filepath.Clean(in.Directory)})
	if err := writeLocal(filepath.Join(s.deviceDir, "location.json"), raw); err != nil {
		apiError(w, errors.New("备份已恢复，但本机目录设置保存失败；请手动切换到恢复目录"), 500)
		return
	}
	if err := s.Vault.SelectDirectory(in.Directory); err != nil {
		apiError(w, err, 400)
		return
	}
	s.Access.Sweep()
	writeJSON(w, map[string]bool{"ok": true})
}
