package server

import (
	"context"
	"errors"
	"github.com/shenjiecode/keyfolio/internal/platform"
	"sync"
	"time"
)

type systemState struct {
	mu        sync.Mutex
	event     platform.SystemEvent
	sequence  uint64
	reason    string
	heartbeat time.Time
	now       func() time.Time
}
type SystemStatus struct {
	Available  bool   `json:"available"`
	Locked     bool   `json:"locked"`
	Sleeping   bool   `json:"sleeping"`
	Enabled    bool   `json:"enabled"`
	Message    string `json:"message"`
	LockReason string `json:"lock_reason"`
}

func (s *Server) systemProtected() bool {
	s.idle.mu.Lock()
	defer s.idle.mu.Unlock()
	return s.idle.settings.LockOnSystemEvents
}
func (s *Server) SystemStatus() SystemStatus {
	enabled := s.systemProtected()
	s.system.mu.Lock()
	defer s.system.mu.Unlock()
	e := s.system.event
	message := "系统锁屏与休眠监测已开启。"
	if !e.Available {
		message = "当前无法监测系统锁屏事件，建议使用手动或空闲锁定。"
	}
	if !enabled {
		message = "系统事件锁定已关闭。"
	}
	return SystemStatus{Available: e.Available, Locked: e.Locked, Sleeping: e.Sleeping, Enabled: enabled, Message: message, LockReason: s.system.reason}
}

// Monitor callbacks use the same access-service lock as manual and idle locking.
// Hold the gate until locking finishes, so a subsequent unlock cannot overtake it.
func (s *Server) SystemEvent(e platform.SystemEvent) {
	enabled := s.systemProtected()
	s.system.mu.Lock()
	defer s.system.mu.Unlock()
	previous := s.system.event
	lost := previous.Available && !e.Available
	if !e.Available {
		e.Locked = previous.Locked
		e.Sleeping = previous.Sleeping
	}
	trigger := e.Lock || lost || (e.Locked && !previous.Locked) || (e.Sleeping && !previous.Sleeping)
	s.system.event = e
	if enabled && trigger {
		reason := e.Reason
		if reason == "" {
			reason = "系统锁屏、休眠或监测中断"
		}
		s.lockSystem(reason)
	}
}
func (s *Server) lockSystem(reason string) {
	s.system.sequence++
	s.system.reason = reason
	s.Access.Lock()
}

// StartSystemMonitor waits for the initial observation before startup unlock.
// If readiness times out, auto-unlock fails closed.
func (s *Server) StartSystemMonitor(ctx context.Context) {
	ready := make(chan struct{})
	var once sync.Once
	go platform.WatchSystem(ctx, func(e platform.SystemEvent) { s.SystemEvent(e); once.Do(func() { close(ready) }) })
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ready:
	case <-ctx.Done():
	case <-timer.C:
	}
}
func (s *Server) beginUnlock(automatic bool) (uint64, error) {
	enabled := s.systemProtected()
	s.system.mu.Lock()
	defer s.system.mu.Unlock()
	e := s.system.event
	if enabled {
		if e.Locked || e.Sleeping {
			return 0, errors.New("系统会话仍锁定或休眠，请先解锁系统并等待监测恢复")
		}
		if automatic && (!e.Available || s.system.sequence != 0) {
			return 0, errors.New("系统会话无法确认或已发生锁定事件，本次未自动解锁；请使用主密码")
		}
	}
	return s.system.sequence, nil
}
func (s *Server) finishUnlock(sequence uint64) error {
	enabled := s.systemProtected()
	s.system.mu.Lock()
	defer s.system.mu.Unlock()
	if enabled && (sequence != s.system.sequence || s.system.event.Locked || s.system.event.Sleeping) {
		s.Access.Lock()
		return errors.New("解锁期间收到系统锁定事件，资料库仍保持锁定")
	}
	s.system.reason = ""
	return nil
}
func (s *Server) unlockPassword(password string) error {
	return s.unlockWith(func() error { return s.Vault.UnlockForVault(password, s.boundID()) })
}
func (s *Server) unlockWith(operation func() error) error {
	sequence, err := s.beginUnlock(false)
	if err != nil {
		return err
	}
	if err = operation(); err != nil {
		return err
	}
	b, err := s.Vault.Binding()
	if err == nil {
		err = s.bindID(b.VaultID)
	}
	if err != nil {
		s.Access.Lock()
		return err
	}
	return s.finishUnlock(sequence)
}

// A wall-clock heartbeat also catches suspend/process stalls before the first
// HTTP/MCP request after resuming. Go's Linux monotonic clock excludes suspend.
// Clock discontinuities deliberately err on the side of an extra lock.
func (s *Server) SystemHeartbeat() {
	enabled := s.systemProtected()
	s.system.mu.Lock()
	defer s.system.mu.Unlock()
	now := s.system.now()
	previous := s.system.heartbeat
	s.system.heartbeat = now
	if previous.IsZero() {
		return
	}
	delta := now.UnixNano() - previous.UnixNano()
	if enabled && (delta > int64(15*time.Second) || delta < int64(-5*time.Second)) {
		s.lockSystem("检测到休眠、进程停顿或系统时间跳变")
	}
}
func (s *Server) enforceSystemSetting() {
	if !s.systemProtected() {
		return
	}
	s.system.mu.Lock()
	defer s.system.mu.Unlock()
	if s.system.event.Locked || s.system.event.Sleeping {
		s.lockSystem("已启用系统事件锁定，当前系统会话不可用")
	}
}
