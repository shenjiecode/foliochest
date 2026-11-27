//go:build !windows

package app

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func detachUpdate(cmd *exec.Cmd)       { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func updateExecutableFree(string) bool { return true }
func linuxInstallerCommand(path, desktop string, lookup func(string) (string, error)) (*exec.Cmd, error) {
	tools := []string{"gnome-software", "plasma-discover"}
	if strings.Contains(strings.ToLower(desktop), "kde") {
		tools[0], tools[1] = tools[1], tools[0]
	}
	for _, tool := range tools {
		if executable, err := lookup(tool); err == nil {
			return exec.Command(executable, "--local-filename", path), nil
		}
	}
	executable, err := lookup("xdg-open")
	if err != nil {
		return nil, err
	}
	return exec.Command(executable, path), nil
}
func launchInstaller(path string) error {
	cmd, err := linuxInstallerCommand(path, os.Getenv("XDG_CURRENT_DESKTOP"), exec.LookPath)
	if err != nil {
		return installerError(err)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return installerError(err)
	}
	return nil
}
