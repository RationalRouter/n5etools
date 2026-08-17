package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser launches the OS's default browser at url. Failure is
// non-fatal — the caller should log the URL as a fallback so the app is
// still usable (e.g. on a machine with no default browser handler
// configured, which happens on the dev's own Linux box).
func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		// The empty string is the window title argument "start" expects
		// before the URL; omitting it makes "start" treat the URL itself
		// as the title when it contains characters like "&".
		cmd, args = "cmd", []string{"/c", "start", "", url}
	case "darwin":
		cmd, args = "open", []string{url}
	default: // linux and other unix-likes
		cmd, args = "xdg-open", []string{url}
	}
	if err := exec.Command(cmd, args...).Start(); err != nil {
		return fmt.Errorf("launching browser: %w", err)
	}
	return nil
}
