//go:build linux

package platform

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbus "github.com/godbus/dbus/v5"
)

// A real Secret Service round trip on a private bus and disposable HOME/XDG
// directories. Never contacts the desktop's existing keyring or its credentials.
func TestNativeSecretServiceIsolated(t *testing.T) {
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon unavailable")
	}
	keyring, err := exec.LookPath("gnome-keyring-daemon")
	if err != nil {
		t.Skip("gnome-keyring-daemon unavailable")
	}
	root := t.TempDir()
	socket := filepath.Join(root, "bus")
	for _, name := range []string{"home", "config", "data", "cache", "runtime", "control"} {
		if err = os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range map[string]string{"HOME": filepath.Join(root, "home"), "XDG_CONFIG_HOME": filepath.Join(root, "config"), "XDG_DATA_HOME": filepath.Join(root, "data"), "XDG_CACHE_HOME": filepath.Join(root, "cache"), "XDG_RUNTIME_DIR": filepath.Join(root, "runtime"), "GNOME_KEYRING_CONTROL": filepath.Join(root, "control"), "DBUS_SESSION_BUS_ADDRESS": "unix:path=" + socket} {
		t.Setenv(key, value)
	}
	config := filepath.Join(root, "bus.conf")
	xml := "<busconfig><type>session</type><listen>unix:path=" + socket + "</listen><auth>EXTERNAL</auth><policy context=\"default\"><allow own=\"*\"/><allow send_destination=\"*\"/><allow receive_sender=\"*\"/></policy></busconfig>"
	if err = os.WriteFile(config, []byte(xml), 0600); err != nil {
		t.Fatal(err)
	}
	bus := exec.Command(daemon, "--nofork", "--nopidfile", "--config-file="+config)
	if err = bus.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bus.Process.Kill(); _ = bus.Wait() })
	ready := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		connection, e := net.DialTimeout("unix", socket, 50*time.Millisecond)
		if e == nil {
			connection.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("isolated D-Bus startup failed")
	}
	service := exec.Command(keyring, "--foreground", "--unlock", "--components=secrets", "--control-directory="+filepath.Join(root, "control"))
	service.Stdin = strings.NewReader("synthetic-system-keyring-test-only")
	if err = service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Process.Kill(); _ = service.Wait() })
	id := strings.Repeat("a", 48)
	provider := NativeCredentials()
	ctx := context.Background()
	ready = false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if _, err = provider.Get(ctx, id); errors.Is(err, ErrNotFound) {
			ready = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("isolated Secret Service not ready: %v", err)
	}
	secret := []byte("SYNTHETIC_DEVICE_SECRET_ONLY")
	if err = provider.Set(ctx, id, secret); err != nil {
		t.Fatal("Set", err)
	}
	got, err := provider.Get(ctx, id)
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatal("Get", err)
	}
	clear(got)
	if err = provider.Delete(ctx, id); err != nil {
		t.Fatal("Delete", err)
	}
	if _, err = provider.Get(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted credential returned", err)
	}
	if err = provider.Delete(ctx, id); err != nil {
		t.Fatal("idempotent Delete", err)
	}
	if err = provider.Set(ctx, id, secret); err != nil {
		t.Fatal(err)
	}
	// Locked keyrings fail closed without creating prompts/unlocking the system.
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	connection, err := connectSecrets(bounded)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var collection dbus.ObjectPath
	object := connection.Object("org.freedesktop.secrets", servicePath)
	if err = object.CallWithContext(bounded, serviceInterface+".ReadAlias", 0, "login").Store(&collection); err != nil {
		t.Fatal(err)
	}
	if err = object.CallWithContext(bounded, serviceInterface+".Lock", 0, []dbus.ObjectPath{collection}).Err; err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Get(ctx, id); !errors.Is(err, ErrUnavailable) {
		t.Fatal("locked Get did not fail closed", err)
	}
	if err = provider.Set(ctx, id, secret); !errors.Is(err, ErrUnavailable) {
		t.Fatal("locked Set did not fail closed", err)
	}
	if err = provider.Delete(ctx, id); !errors.Is(err, ErrUnavailable) {
		t.Fatal("locked Delete did not report incomplete cleanup", err)
	}
}
func TestSecretServiceRejectsNetworkTransport(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "tcp:host=127.0.0.1,port=1")
	if _, err := NativeCredentials().Get(context.Background(), strings.Repeat("b", 48)); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
