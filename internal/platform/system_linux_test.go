//go:build linux

package platform

import (
	"context"
	dbus "github.com/godbus/dbus/v5"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const fakeSession = dbus.ObjectPath("/org/freedesktop/login1/session/synthetic")
const fakeUser = dbus.ObjectPath("/org/freedesktop/login1/user/synthetic")

type fakeLogind struct {
	mu                       sync.Mutex
	uid                      uint32
	locked, sleeping, remote bool
	active                   bool
	kind                     string
	pidSession               bool
	omit                     string
}

func (f *fakeLogind) GetSessionByPID(pid uint32) (dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.pidSession {
		return "/", dbus.NewError("org.freedesktop.login1.NoSessionForPID", nil)
	}
	return fakeSession, nil
}
func (f *fakeLogind) GetUser(uid uint32) (dbus.ObjectPath, *dbus.Error) { return fakeUser, nil }
func (f *fakeLogind) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	values := map[string]dbus.Variant{}
	switch iface {
	case loginManager:
		values["PreparingForSleep"] = dbus.MakeVariant(f.sleeping)
		values["PreparingForShutdown"] = dbus.MakeVariant(false)
	case loginSession:
		values["User"] = dbus.MakeVariant(struct {
			UID  uint32
			Path dbus.ObjectPath
		}{f.uid, fakeUser})
		values["Type"] = dbus.MakeVariant(f.kind)
		values["Remote"] = dbus.MakeVariant(f.remote)
		values["Active"] = dbus.MakeVariant(f.active)
		values["LockedHint"] = dbus.MakeVariant(f.locked)
	case "org.freedesktop.login1.User":
		values["Display"] = dbus.MakeVariant(struct {
			ID   string
			Path dbus.ObjectPath
		}{"synthetic", fakeSession})
	default:
		return nil, dbus.NewError("org.freedesktop.DBus.Error.UnknownInterface", nil)
	}
	delete(values, f.omit)
	return values, nil
}
func isolatedSystemBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon unavailable")
	}
	root := t.TempDir()
	socket := filepath.Join(root, "bus")
	config := filepath.Join(root, "bus.conf")
	xml := "<busconfig><type>session</type><listen>unix:path=" + socket + "</listen><auth>EXTERNAL</auth><policy context=\"default\"><allow own=\"*\"/><allow send_destination=\"*\"/><allow receive_sender=\"*\"/></policy></busconfig>"
	if err = os.WriteFile(config, []byte(xml), 0600); err != nil {
		t.Fatal(err)
	}
	process := exec.Command(daemon, "--nofork", "--nopidfile", "--config-file="+config)
	if err = process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Process.Kill(); _ = process.Wait() })
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		c, err := net.DialTimeout("unix", socket, 50*time.Millisecond)
		if err == nil {
			c.Close()
			return "unix:path=" + socket
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("isolated system bus did not start")
	return ""
}
func exportFakeLogind(t *testing.T, address string, f *fakeLogind) *dbus.Conn {
	t.Helper()
	c, err := connectLocalBus(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	for path, iface := range map[dbus.ObjectPath]string{loginRoot: loginManager, fakeSession: loginSession, fakeUser: "org.freedesktop.login1.User"} {
		if err = c.Export(f, path, "org.freedesktop.DBus.Properties"); err != nil {
			t.Fatal(err)
		}
		if iface == loginManager {
			if err = c.Export(f, path, iface); err != nil {
				t.Fatal(err)
			}
		}
	}
	reply, err := c.RequestName(loginService, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatal(reply, err)
	}
	return c
}
func awaitSystemEvent(t *testing.T, events <-chan SystemEvent, predicate func(SystemEvent) bool, limit time.Duration) SystemEvent {
	t.Helper()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if predicate(event) {
				return event
			}
		case <-timer.C:
			t.Fatal("expected system event not observed")
			return SystemEvent{}
		}
	}
}
func TestNativeLogindIsolated(t *testing.T) {
	address := isolatedSystemBus(t)
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", address)
	fake := &fakeLogind{uid: uint32(os.Getuid()), active: true, kind: "wayland"}
	provider := exportFakeLogind(t, address, fake)
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan SystemEvent, 64)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		WatchSystem(ctx, func(e SystemEvent) {
			select {
			case events <- e:
			case <-ctx.Done():
			}
		})
	}()
	defer func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Error("observer did not stop")
		}
	}()
	event := awaitSystemEvent(t, events, func(e SystemEvent) bool { return e.Available }, 3*time.Second)
	if event.Locked || event.Sleeping {
		t.Fatal(event)
	}
	// GetSessionByPID deliberately fails: actual XDG user-service fallback goes
	// through this user's Display and verifies the session UID/type.
	fake.mu.Lock()
	fake.locked = true
	fake.mu.Unlock()
	if err := provider.Emit(fakeSession, loginSession+".Lock"); err != nil {
		t.Fatal(err)
	}
	awaitSystemEvent(t, events, func(e SystemEvent) bool { return e.Locked && e.Lock }, 3*time.Second)
	fake.mu.Lock()
	fake.locked = false
	fake.mu.Unlock()
	if err := provider.Emit(fakeSession, "org.freedesktop.DBus.Properties.PropertiesChanged", loginSession, map[string]dbus.Variant{"LockedHint": dbus.MakeVariant(false)}, []string{}); err != nil {
		t.Fatal(err)
	}
	awaitSystemEvent(t, events, func(e SystemEvent) bool { return e.Available && !e.Locked }, 3*time.Second)
	fake.mu.Lock()
	fake.sleeping = true
	fake.mu.Unlock()
	if err := provider.Emit(loginRoot, loginManager+".PrepareForSleep", true); err != nil {
		t.Fatal(err)
	}
	awaitSystemEvent(t, events, func(e SystemEvent) bool { return e.Sleeping && e.Lock }, 3*time.Second)
	fake.mu.Lock()
	fake.sleeping = false
	fake.mu.Unlock()
	if err := provider.Emit(loginRoot, loginManager+".PrepareForSleep", false); err != nil {
		t.Fatal(err)
	}
	awaitSystemEvent(t, events, func(e SystemEvent) bool { return !e.Sleeping && e.Lock }, 3*time.Second)
	// Switching away from the session is also a lock, even without LockedHint.
	fake.mu.Lock()
	fake.active = false
	fake.mu.Unlock()
	_ = provider.Emit(fakeSession, "org.freedesktop.DBus.Properties.PropertiesChanged", loginSession, map[string]dbus.Variant{"Active": dbus.MakeVariant(false)}, []string{})
	awaitSystemEvent(t, events, func(e SystemEvent) bool { return e.Locked }, 3*time.Second)
	_ = provider.Close()
	awaitSystemEvent(t, events, func(e SystemEvent) bool { return !e.Available }, 4*time.Second)
	fake.mu.Lock()
	fake.active = true
	fake.mu.Unlock()
	_ = exportFakeLogind(t, address, fake)
	awaitSystemEvent(t, events, func(e SystemEvent) bool { return e.Available && !e.Locked }, 8*time.Second)
}
func TestLogindSignalValidation(t *testing.T) {
	snapshot := loginSnapshot{path: fakeSession, owner: ":1.24", event: SystemEvent{Available: true}}
	valid := dbus.Signal{Sender: snapshot.owner, Path: loginRoot, Name: loginManager + ".PrepareForSleep", Body: []interface{}{true}}
	for _, alter := range []func(*dbus.Signal){
		func(s *dbus.Signal) { s.Sender = ":1.25" },
		func(s *dbus.Signal) { s.Path = "/other" },
		func(s *dbus.Signal) { s.Body = []interface{}{"true"} },
		func(s *dbus.Signal) { s.Body = []interface{}{true, false} },
		func(s *dbus.Signal) { s.Name = loginSession + ".Unlock"; s.Path = fakeSession; s.Body = nil },
	} {
		signal := valid
		alter(&signal)
		if _, handled := logindEvent(&signal, snapshot); handled {
			t.Fatal("untrusted/malformed signal accepted", signal.Name)
		}
	}
}
func TestLogindRejectsOtherUsersRemoteAndNonGraphical(t *testing.T) {
	address := isolatedSystemBus(t)
	fake := &fakeLogind{uid: uint32(os.Getuid()), active: true, kind: "wayland"}
	_ = exportFakeLogind(t, address, fake)
	c, err := connectLocalBus(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, kind := range []string{"uid", "remote", "tty", "missing-user", "missing-display"} {
		fake.mu.Lock()
		fake.uid = uint32(os.Getuid())
		fake.remote = false
		fake.kind = "wayland"
		fake.omit = ""
		switch kind {
		case "uid":
			fake.uid++
		case "remote":
			fake.remote = true
		case "tty":
			fake.kind = "tty"
		case "missing-user":
			fake.omit = "User"
		case "missing-display":
			fake.omit = "Display"
		}
		fake.mu.Unlock()
		if _, err = readLogind(context.Background(), c, uint32(os.Getuid()), uint32(os.Getpid())); err == nil {
			t.Fatal("unrelated session accepted", kind)
		}
	}
}
func TestSystemBusRejectsNetworkTransport(t *testing.T) {
	if _, err := connectLocalBus(context.Background(), "tcp:host=127.0.0.1,port=1"); err == nil {
		t.Fatal("accepted remote bus")
	}
}
