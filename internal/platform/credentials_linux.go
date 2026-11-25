//go:build linux

package platform

import (
	"context"
	dbus "github.com/godbus/dbus/v5"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const servicePath = dbus.ObjectPath("/org/freedesktop/secrets")
const serviceInterface = "org.freedesktop.Secret.Service"

type nativeCredentials struct{}
type secretValue struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

func NativeCredentials() Credentials { return nativeCredentials{} }
func CredentialProviderName() string { return "Linux Secret Service（已解锁的系统凭据库）" }
func connectSecrets(ctx context.Context) (*dbus.Conn, error) {
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" {
		directory := os.Getenv("XDG_RUNTIME_DIR")
		if !filepath.IsAbs(directory) {
			return nil, ErrUnavailable
		}
		address = "unix:path=" + filepath.Join(directory, "bus")
	}
	return connectLocalBus(ctx, address)
}

// Authentication is bounded even for the long-lived system-event connection.
// The bus context keeps its original lifetime; only dialing/auth uses 5 seconds.
func connectLocalBus(ctx context.Context, address string) (*dbus.Conn, error) {
	if !strings.HasPrefix(address, "unix:") || strings.Contains(address, ";") {
		return nil, ErrUnavailable
	}
	socket := ""
	for _, part := range strings.Split(strings.TrimPrefix(address, "unix:"), ",") {
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		decoded, err := url.PathUnescape(value)
		if err != nil {
			return nil, ErrUnavailable
		}
		if name == "path" && filepath.IsAbs(decoded) {
			socket = decoded
		}
		if name == "abstract" && decoded != "" {
			socket = "\x00" + decoded
		}
	}
	if socket == "" {
		return nil, ErrUnavailable
	}
	connection, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, ErrUnavailable
	}
	deadline := time.Now().Add(5 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	_ = connection.SetDeadline(deadline)
	bus, err := dbus.NewConn(connection, dbus.WithContext(ctx))
	if err != nil {
		connection.Close()
		return nil, ErrUnavailable
	}
	if err = bus.Auth([]dbus.Auth{dbus.AuthExternal(strconv.Itoa(os.Getuid()))}); err == nil {
		err = bus.Hello()
	}
	if err != nil {
		bus.Close()
		return nil, ErrUnavailable
	}
	_ = connection.SetDeadline(time.Time{})
	return bus, nil
}
func openSecretSession(ctx context.Context, c *dbus.Conn) (dbus.ObjectPath, error) {
	var output dbus.Variant
	var session dbus.ObjectPath
	err := c.Object("org.freedesktop.secrets", servicePath).CallWithContext(ctx, serviceInterface+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&output, &session)
	if err != nil || session == "/" || !session.IsValid() {
		return "", ErrUnavailable
	}
	return session, nil
}
func findSecrets(ctx context.Context, c *dbus.Conn, id string) ([]dbus.ObjectPath, error) {
	var unlocked, locked []dbus.ObjectPath
	err := c.Object("org.freedesktop.secrets", servicePath).CallWithContext(ctx, serviceInterface+".SearchItems", 0, map[string]string{"application": ServiceName, "credential": id}).Store(&unlocked, &locked)
	if err != nil || len(locked) > 0 {
		return nil, ErrUnavailable
	}
	return unlocked, nil
}
func (nativeCredentials) Get(ctx context.Context, id string) ([]byte, error) {
	if !validCredentialID(id) {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := connectSecrets(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	items, err := findSecrets(ctx, c, id)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	if len(items) != 1 {
		return nil, ErrUnavailable
	}
	session, err := openSecretSession(ctx, c)
	if err != nil {
		return nil, err
	}
	var secret secretValue
	err = c.Object("org.freedesktop.secrets", items[0]).CallWithContext(ctx, "org.freedesktop.Secret.Item.GetSecret", 0, session).Store(&secret)
	if err != nil || secret.Session != session || len(secret.Parameters) != 0 || len(secret.Value) > 4096 {
		clear(secret.Value)
		return nil, ErrUnavailable
	}
	return secret.Value, nil
}
func (nativeCredentials) Set(ctx context.Context, id string, value []byte) error {
	if !validCredentialID(id) || len(value) > 4096 {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := connectSecrets(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	var collection dbus.ObjectPath
	service := c.Object("org.freedesktop.secrets", servicePath)
	if err = service.CallWithContext(ctx, serviceInterface+".ReadAlias", 0, "default").Store(&collection); err != nil {
		return ErrUnavailable
	}
	if collection == "/" {
		if err = service.CallWithContext(ctx, serviceInterface+".ReadAlias", 0, "login").Store(&collection); err != nil {
			return ErrUnavailable
		}
	}
	if collection == "/" {
		return ErrUnavailable
	}
	obj := c.Object("org.freedesktop.secrets", collection)
	var locked dbus.Variant
	if err = obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, "org.freedesktop.Secret.Collection", "Locked").Store(&locked); err != nil || locked.Value() != false {
		return ErrUnavailable
	}
	session, err := openSecretSession(ctx, c)
	if err != nil {
		return err
	}
	properties := map[string]dbus.Variant{"org.freedesktop.Secret.Item.Label": dbus.MakeVariant("Keyfolio device unlock"), "org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(map[string]string{"application": ServiceName, "credential": id})}
	var item, prompt dbus.ObjectPath
	err = obj.CallWithContext(ctx, "org.freedesktop.Secret.Collection.CreateItem", 0, properties, secretValue{Session: session, Parameters: []byte{}, Value: value, ContentType: "application/octet-stream"}, true).Store(&item, &prompt)
	if err != nil {
		return ErrUnavailable
	}
	if prompt != "/" {
		_ = c.Object("org.freedesktop.secrets", prompt).CallWithContext(ctx, "org.freedesktop.Secret.Prompt.Dismiss", 0).Err
		return ErrUnavailable
	}
	if item == "/" {
		return ErrUnavailable
	}
	return nil
}
func (nativeCredentials) Delete(ctx context.Context, id string) error {
	if !validCredentialID(id) {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := connectSecrets(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	items, err := findSecrets(ctx, c, id)
	if err != nil {
		return err
	}
	for _, item := range items {
		var prompt dbus.ObjectPath
		if err = c.Object("org.freedesktop.secrets", item).CallWithContext(ctx, "org.freedesktop.Secret.Item.Delete", 0).Store(&prompt); err != nil {
			return ErrUnavailable
		}
		if prompt != "/" {
			_ = c.Object("org.freedesktop.secrets", prompt).CallWithContext(ctx, "org.freedesktop.Secret.Prompt.Dismiss", 0).Err
			return ErrUnavailable
		}
	}
	return nil
}
