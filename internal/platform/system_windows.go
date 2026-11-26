//go:build windows

package platform

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var eventUser32 = windows.NewLazySystemDLL("user32.dll")
var eventWTS = windows.NewLazySystemDLL("wtsapi32.dll")
var eventKernel = windows.NewLazySystemDLL("kernel32.dll")
var eventClassCounter atomic.Uint64

type eventWindowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type eventMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}

const (
	eventClose   = 0x0010
	eventTimer   = 0x0113
	eventPower   = 0x0218
	eventSession = 0x02B1
)

func windowsSession(session uint32) (SystemEvent, error) {
	var buffer *byte
	var size uint32
	ok, _, _ := eventWTS.NewProc("WTSQuerySessionInformationW").Call(0, uintptr(session), 25, uintptr(unsafe.Pointer(&buffer)), uintptr(unsafe.Pointer(&size)))
	if ok == 0 || buffer == nil {
		return SystemEvent{}, ErrUnavailable
	}
	defer eventWTS.NewProc("WTSFreeMemory").Call(uintptr(unsafe.Pointer(buffer)))
	if size < 20 || size > 64<<10 {
		return SystemEvent{}, ErrUnavailable
	}
	// WTSINFOEXW has a DWORD level, 8-byte-aligned union, then three DWORDs.
	raw := unsafe.Slice(buffer, int(size))
	if binary.LittleEndian.Uint32(raw[:4]) != 1 || binary.LittleEndian.Uint32(raw[8:12]) != session {
		return SystemEvent{}, ErrUnavailable
	}
	state := binary.LittleEndian.Uint32(raw[12:16])
	flags := binary.LittleEndian.Uint32(raw[16:20])
	if flags > 1 {
		return SystemEvent{}, ErrUnavailable
	}
	// Windows 10/11 semantics.
	return SystemEvent{Available: true, Locked: flags == 0 || state != 0}, nil
}
func WatchSystem(ctx context.Context, emit func(SystemEvent)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var session uint32
	ok, _, _ := eventKernel.NewProc("ProcessIdToSessionId").Call(uintptr(os.Getpid()), uintptr(unsafe.Pointer(&session)))
	if ok == 0 {
		emit(SystemEvent{})
		<-ctx.Done()
		return
	}
	state := SystemEvent{}
	available := false
	first := true
	notify := func(e SystemEvent) {
		if !e.Available {
			e.Locked = state.Locked
			e.Sleeping = state.Sleeping
		}
		if first || e.Available || available || e.Lock {
			emit(e)
		}
		state = e
		available = e.Available
		first = false
	}
	refresh := func(force bool, reason string) {
		next, err := windowsSession(session)
		if err != nil {
			next = SystemEvent{}
		}
		if !force {
			next.Sleeping = state.Sleeping
		}
		next.Lock = force
		next.Reason = reason
		notify(next)
	}
	// One callback for the observer lifetime; retries do not leak Go callback
	// slots. All mutable native state is confined to this locked OS thread.
	proc := windows.NewCallback(func(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
		switch msg {
		case eventClose:
			eventUser32.NewProc("PostQuitMessage").Call(0)
			return 0
		case eventTimer:
			if wparam == 1 {
				refresh(false, "")
			}
			return 0
		case eventSession:
			if lparam != uintptr(session) {
				return 0
			}
			switch wparam {
			case 2, 4, 6, 7: // console/remote disconnect, logoff, session lock
				next := state
				next.Locked = true
				next.Available = true
				next.Lock = true
				next.Reason = "系统锁屏、会话断开或退出"
				notify(next)
			case 1, 3, 5, 8: // reconnect/logon/unlock never unlocks the vault
				refresh(false, "")
			}
			return 0
		case eventPower:
			switch wparam {
			case 4: // PBT_APMSUSPEND
				next := state
				next.Sleeping = true
				next.Available = true
				next.Lock = true
				next.Reason = "系统即将休眠"
				notify(next)
			case 7, 18: // PBT_APMRESUMESUSPEND, PBT_APMRESUMEAUTOMATIC
				state.Sleeping = false
				refresh(true, "系统从休眠恢复")
			}
			return 1
		}
		result, _, _ := eventUser32.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wparam, lparam)
		return result
	})
	for ctx.Err() == nil {
		state.Sleeping = false
		_ = pumpSystemWindow(ctx, proc, func() { refresh(false, "") })
		if ctx.Err() != nil {
			return
		}
		notify(SystemEvent{})
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func pumpSystemWindow(ctx context.Context, proc uintptr, ready func()) error {
	instance, _, _ := eventKernel.NewProc("GetModuleHandleW").Call(0)
	name, _ := windows.UTF16PtrFromString(fmt.Sprintf("KeyfolioSystemEvents-%d-%d", os.Getpid(), eventClassCounter.Add(1)))
	class := eventWindowClass{Proc: proc, Instance: instance, Name: name}
	class.Size = uint32(unsafe.Sizeof(class))
	atom, _, _ := eventUser32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return ErrUnavailable
	}
	defer func() {
		eventUser32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(name)), instance)
		runtime.KeepAlive(name)
	}()
	// An invisible top-level window receives session and power notifications.
	hwnd, _, _ := eventUser32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		return ErrUnavailable
	}
	defer eventUser32.NewProc("DestroyWindow").Call(hwnd)
	registered, _, _ := eventWTS.NewProc("WTSRegisterSessionNotification").Call(hwnd, 0)
	if registered == 0 {
		return ErrUnavailable
	}
	defer eventWTS.NewProc("WTSUnRegisterSessionNotification").Call(hwnd)
	power, _, _ := eventUser32.NewProc("RegisterSuspendResumeNotification").Call(hwnd, 0)
	if power == 0 {
		return ErrUnavailable
	}
	defer eventUser32.NewProc("UnregisterSuspendResumeNotification").Call(power)
	timer, _, _ := eventUser32.NewProc("SetTimer").Call(hwnd, 1, 2000, 0)
	if timer == 0 {
		return ErrUnavailable
	}
	defer eventUser32.NewProc("KillTimer").Call(hwnd, 1)
	done := make(chan struct{})
	workerDone := make(chan struct{})
	defer func() { close(done); <-workerDone }()
	go func() {
		defer close(workerDone)
		select {
		case <-ctx.Done():
			eventUser32.NewProc("PostMessageW").Call(hwnd, eventClose, 0, 0)
		case <-done:
		}
	}()
	ready()
	var msg eventMessage
	for {
		result, _, _ := eventUser32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) < 0 {
			return ErrUnavailable
		}
		if result == 0 {
			return nil
		}
		eventUser32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		eventUser32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
}
