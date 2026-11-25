//go:build windows

package platform

import (
	"errors"
	"golang.org/x/sys/windows/registry"
	"os"
	"strings"
	"syscall"
	"unicode/utf16"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func (s *NativeStartup) startupName() string { return "Keyfolio-" + DeviceID(s.DeviceDir) }
func (s *NativeStartup) command() (string, error) {
	args, err := s.args()
	if err != nil {
		return "", err
	}
	for i := range args {
		args[i] = syscall.EscapeArg(args[i])
	}
	command := strings.Join(args, " ")
	if len(utf16.Encode([]rune(command))) > 260 {
		return "", errors.New("Windows 自启动命令超过 260 字符，请缩短程序或本机配置目录路径")
	}
	return command, nil
}
func (s *NativeStartup) Status() (StartupStatus, error) {
	status := StartupStatus{Supported: true}
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(s.startupName())
	if errors.Is(err, registry.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	expected, e := s.command()
	status.Enabled = value != ""
	status.Current = e == nil && value == expected
	return status, nil
}
func (s *NativeStartup) Set(enabled bool) error {
	if !enabled {
		key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer key.Close()
		err = key.DeleteValue(s.startupName())
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	command, err := s.command()
	if err != nil {
		return err
	}
	if info, e := os.Stat(s.Executable); e != nil || !info.Mode().IsRegular() {
		return errors.New("自启动程序不存在")
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(s.startupName(), command)
}
