//go:build linux

package platform

import (
	"context"
	dbus "github.com/godbus/dbus/v5"
	"os"
	"time"
)

const loginService = "org.freedesktop.login1"
const loginManager = "org.freedesktop.login1.Manager"
const loginSession = "org.freedesktop.login1.Session"
const loginRoot = dbus.ObjectPath("/org/freedesktop/login1")

func WatchSystem(ctx context.Context, emit func(SystemEvent)) {
	address := os.Getenv("DBUS_SYSTEM_BUS_ADDRESS")
	if address == "" {
		address = "unix:path=/run/dbus/system_bus_socket"
	}
	// Publish failure only on a transition; permanent unavailability must not
	// repeatedly lock a vault the human explicitly opened using the password.
	previous := SystemEvent{}
	started := false
	notify := func(e SystemEvent) {
		if !started || e.Available || previous.Available {
			emit(e)
		}
		previous = e
		started = true
	}
	for ctx.Err() == nil {
		conn, err := connectLocalBus(ctx, address)
		if err == nil {
			err = watchLogind(ctx, conn, uint32(os.Getuid()), uint32(os.Getpid()), notify)
			_ = conn.Close()
		}
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

type loginSnapshot struct {
	event SystemEvent
	path  dbus.ObjectPath
	owner string
}

func loginProperties(ctx context.Context, c *dbus.Conn, path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	var values map[string]dbus.Variant
	err := c.Object(loginService, path).CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", 0, iface).Store(&values)
	return values, err
}
func loginBool(values map[string]dbus.Variant, name string) (bool, bool) {
	value, ok := values[name]
	if !ok {
		return false, false
	}
	result, ok := value.Value().(bool)
	return result, ok
}
func sessionSnapshot(ctx context.Context, c *dbus.Conn, path dbus.ObjectPath, uid uint32) (SystemEvent, error) {
	if !path.IsValid() || path == "/" {
		return SystemEvent{}, ErrUnavailable
	}
	props, err := loginProperties(ctx, c, path, loginSession)
	if err != nil {
		return SystemEvent{}, err
	}
	var user struct {
		UID  uint32
		Path dbus.ObjectPath
	}
	userProperty, found := props["User"]
	if !found || userProperty.Value() == nil || userProperty.Store(&user) != nil || user.UID != uid {
		return SystemEvent{}, ErrUnavailable
	}
	kind, _ := props["Type"].Value().(string)
	remote, remoteOK := loginBool(props, "Remote")
	active, activeOK := loginBool(props, "Active")
	locked, lockedOK := loginBool(props, "LockedHint")
	if !remoteOK || remote || !activeOK || !lockedOK || (kind != "x11" && kind != "wayland") {
		return SystemEvent{}, ErrUnavailable
	}
	return SystemEvent{Available: true, Locked: locked || !active}, nil
}
func readLogind(ctx context.Context, c *dbus.Conn, uid, pid uint32) (loginSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var owner string
	if err := c.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, loginService).Store(&owner); err != nil {
		return loginSnapshot{}, err
	}
	manager := c.Object(loginService, loginRoot)
	var path dbus.ObjectPath
	_ = manager.CallWithContext(ctx, loginManager+".GetSessionByPID", 0, pid).Store(&path)
	event, err := sessionSnapshot(ctx, c, path, uid)
	if err != nil {
		// XDG autostart can run as a systemd user unit, outside any login session.
		// Resolve this user's display session, not an arbitrary other user's seat.
		var userPath dbus.ObjectPath
		if err = manager.CallWithContext(ctx, loginManager+".GetUser", 0, uid).Store(&userPath); err != nil {
			return loginSnapshot{}, err
		}
		props, err := loginProperties(ctx, c, userPath, "org.freedesktop.login1.User")
		if err != nil {
			return loginSnapshot{}, err
		}
		var display struct {
			ID   string
			Path dbus.ObjectPath
		}
		displayProperty, found := props["Display"]
		if !found || displayProperty.Value() == nil || displayProperty.Store(&display) != nil {
			return loginSnapshot{}, ErrUnavailable
		}
		path = display.Path
		event, err = sessionSnapshot(ctx, c, path, uid)
		if err != nil {
			return loginSnapshot{}, err
		}
	}
	props, err := loginProperties(ctx, c, loginRoot, loginManager)
	if err != nil {
		return loginSnapshot{}, err
	}
	sleep, ok := loginBool(props, "PreparingForSleep")
	shutdown, shutdownOK := loginBool(props, "PreparingForShutdown")
	if !ok || !shutdownOK {
		return loginSnapshot{}, ErrUnavailable
	}
	event.Sleeping = sleep || shutdown
	return loginSnapshot{event: event, path: path, owner: owner}, nil
}
func watchLogind(ctx context.Context, c *dbus.Conn, uid, pid uint32, emit func(SystemEvent)) error {
	signals := make(chan *dbus.Signal, 32)
	c.Signal(signals)
	defer c.RemoveSignal(signals)
	setup, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := c.AddMatchSignalContext(setup, dbus.WithMatchSender(loginService), dbus.WithMatchPathNamespace(loginRoot))
	cancel()
	if err != nil {
		return err
	}
	snapshot, err := readLogind(ctx, c, uid, pid)
	if err != nil {
		return err
	}
	emit(snapshot.event)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	refresh := func() error {
		next, err := readLogind(ctx, c, uid, pid)
		if err != nil {
			return err
		}
		next.event.Lock = next.path != snapshot.path || next.owner != snapshot.owner || next.event.Sleeping != snapshot.event.Sleeping
		if next.event.Lock {
			next.event.Reason = "系统会话变化或休眠恢复"
		}
		emit(next.event)
		snapshot = next
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.Context().Done():
			return ErrUnavailable
		case <-ticker.C:
			if err = refresh(); err != nil {
				return err
			}
		case sig, ok := <-signals:
			if !ok {
				return ErrUnavailable
			}
			// Match rules don't constrain directed signals. Verify the actual daemon
			// owner too, so another same-bus client cannot impersonate logind.
			if sig == nil || sig.Sender != snapshot.owner {
				continue
			}
			if event, handled := logindEvent(sig, snapshot); handled {
				snapshot.event = event
				emit(event)
			} else if sig.Name == "org.freedesktop.DBus.Properties.PropertiesChanged" && (sig.Path == snapshot.path || sig.Path == loginRoot) {
				if err = refresh(); err != nil {
					return err
				}
			} else if sig.Path == loginRoot && (sig.Name == loginManager+".SessionRemoved" || sig.Name == loginManager+".SessionNew") {
				if err = refresh(); err != nil {
					return err
				}
			}
		}
	}
}
func logindEvent(sig *dbus.Signal, snapshot loginSnapshot) (SystemEvent, bool) {
	e := snapshot.event
	e.Lock = false
	e.Reason = ""
	if sig.Sender != snapshot.owner {
		return e, false
	}
	if sig.Path == loginRoot && (sig.Name == loginManager+".PrepareForSleep" || sig.Name == loginManager+".PrepareForShutdown") {
		if len(sig.Body) != 1 {
			return e, false
		}
		starting, ok := sig.Body[0].(bool)
		if !ok {
			return e, false
		}
		e.Sleeping = starting
		e.Lock = true
		e.Reason = "系统休眠、退出或恢复"
		return e, true
	}
	if sig.Path == snapshot.path && sig.Name == loginSession+".Lock" && len(sig.Body) == 0 {
		e.Locked = true
		e.Lock = true
		e.Reason = "系统锁屏"
		return e, true
	}
	// Unlock requests are not proof of an unlocked screen. Only a subsequent
	// validated LockedHint/Active snapshot can clear the blocked state.
	return e, false
}
