//go:build !windows

package app

import (
	"errors"
	"reflect"
	"testing"
)

func TestLinuxInstallerUsesSystemUIWithoutShellOrSudo(t *testing.T) {
	for _, tc := range []struct {
		desktop   string
		available []string
		want      []string
	}{
		{"GNOME", []string{"gnome-software", "xdg-open"}, []string{"/synthetic/gnome-software", "--local-filename", "/synthetic/a b.rpm"}},
		{"KDE", []string{"plasma-discover", "gnome-software"}, []string{"/synthetic/plasma-discover", "--local-filename", "/synthetic/a b.rpm"}},
		{"", []string{"xdg-open"}, []string{"/synthetic/xdg-open", "/synthetic/a b.rpm"}},
	} {
		cmd, err := linuxInstallerCommand("/synthetic/a b.rpm", tc.desktop, func(name string) (string, error) {
			for _, available := range tc.available {
				if name == available {
					return "/synthetic/" + name, nil
				}
			}
			return "", errors.New("unavailable")
		})
		if err != nil || !reflect.DeepEqual(cmd.Args, tc.want) {
			t.Fatal(cmd, err)
		}
	}
}
