//go:build !windows

package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

func OpenBrowser(address string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, address).Run()
}
func ShowLaunchError(message string) {
	fmt.Fprintln(os.Stderr, message)
	if runtime.GOOS == "linux" {
		if helper, err := exec.LookPath("zenity"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = exec.CommandContext(ctx, helper, "--error", "--title=资料匣", "--text="+message).Run()
		}
	}
}
