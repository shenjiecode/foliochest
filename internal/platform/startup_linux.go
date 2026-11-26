//go:build linux

package platform

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func (s *NativeStartup) startupPath() string {
	return filepath.Join(s.ConfigDir, "autostart", "keyfolio-"+DeviceID(s.DeviceDir)+".desktop")
}
func desktopQuote(value string) string {
	// Desktop string escaping is applied before Exec quoting.
	quoted := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "$", "\\$", "`", "\\`", "%", "%%").Replace(value)
	quoted = strings.ReplaceAll(quoted, "\\", "\\\\")
	return "\"" + quoted + "\""
}
func (s *NativeStartup) desktop() ([]byte, error) {
	args, err := s.args()
	if err != nil {
		return nil, err
	}
	// systemd-xdg-autostart-generator and GIO differ for these escapes.
	// Refuse rather than register a command that may silently fail at login.
	for _, arg := range args {
		if strings.ContainsAny(arg, "$`\\\\") {
			return nil, errors.New("Linux 自启动路径暂不支持美元符、反引号或反斜杠，请使用不含这些字符的程序和本机配置目录")
		}
	}
	if strings.Contains(s.Executable, "=") {
		return nil, errors.New("Desktop Entry 不支持包含等号的程序路径")
	}
	for i := range args {
		args[i] = desktopQuote(args[i])
	}
	return []byte("[Desktop Entry]\nType=Application\nName=FolioChest\nExec=" + strings.Join(args, " ") + "\nTerminal=false\nX-Keyfolio-Device=" + DeviceID(s.DeviceDir) + "\n"), nil
}
func (s *NativeStartup) Status() (StartupStatus, error) {
	status := StartupStatus{Supported: true}
	raw, err := os.ReadFile(s.startupPath())
	if os.IsNotExist(err) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	if !bytes.Contains(raw, []byte("X-Keyfolio-Device="+DeviceID(s.DeviceDir)+"\n")) {
		return status, errors.New("同名启动项不属于当前配置")
	}
	status.Enabled = !bytes.Contains(raw, []byte("\nHidden=true")) && !bytes.Contains(raw, []byte("\nX-GNOME-Autostart-enabled=false"))
	expected, e := s.desktop()
	// A display-name change alone must not invalidate a legacy registration.
	compatible := bytes.Replace(raw, []byte("\nName=Keyfolio\n"), []byte("\nName=FolioChest\n"), 1)
	status.Current = e == nil && bytes.Equal(expected, compatible)
	return status, nil
}
func (s *NativeStartup) Set(enabled bool) error {
	if _, err := s.Status(); err != nil {
		return err
	}
	path := s.startupPath()
	if !enabled {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	raw, err := s.desktop()
	if err != nil {
		return err
	}
	if info, e := os.Stat(s.Executable); e != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return errors.New("自启动程序不存在或不可执行")
	}
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".keyfolio-startup-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	ce := file.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
