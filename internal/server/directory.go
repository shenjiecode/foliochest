package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/shenjiecode/keyfolio/internal/platform"
)

func (s *Server) directoryPicker(w http.ResponseWriter, r *http.Request, id string, session *webSession) {
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
	if !s.picker.TryLock() {
		apiError(w, errors.New("已有文件夹选择窗口，请先完成或取消"), 409)
		return
	}
	defer s.picker.Unlock()
	generation := s.Vault.Generation()
	valid := func() bool {
		if s.Vault.Generation() != generation {
			return false
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		current := s.sessions[id]
		return current != nil && current.CSRF == session.CSRF && time.Now().Before(current.Expires)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	// Native dialogs are outside the React tree: close them on lock/session change.
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !valid() {
					cancel()
					return
				}
			}
		}
	}()
	path, err := s.chooseDirectory(ctx, in.Directory)
	if !valid() {
		apiError(w, errors.New("网页会话或资料库已变化，请重新选择文件夹"), 409)
		return
	}
	if ctx.Err() != nil {
		apiError(w, errors.New("目录选择已取消或超时，请重试"), 408)
		return
	}
	if errors.Is(err, platform.ErrDirectoryCanceled) {
		writeJSON(w, map[string]any{"cancelled": true})
		return
	}
	if err != nil {
		apiError(w, err, 503)
		return
	}
	writeJSON(w, map[string]any{"cancelled": false, "directory": path})
}
