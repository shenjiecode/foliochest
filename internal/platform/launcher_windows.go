//go:build windows

package platform

import "golang.org/x/sys/windows"

func OpenBrowser(address string) error {
	target, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("open")
	return windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
}
func ShowLaunchError(message string) {
	text, _ := windows.UTF16PtrFromString(message)
	title, _ := windows.UTF16PtrFromString("资料匣 · FolioChest")
	_, _ = windows.MessageBox(0, text, title, windows.MB_OK|windows.MB_ICONERROR)
}
