package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Send displays an OS notification with the given title, message, and an
// optional URL to open when the notification is clicked.
// It is a best-effort call — errors are silently ignored so a notification
// failure never disrupts the TUI.
func Send(title, message, url string) {
	if runtime.GOOS == "darwin" {
		sendDarwin(title, message, url)
	}
}

// sendDarwin sends a macOS notification. If terminal-notifier is installed it
// is used so the notification is clickable (opens url in the browser).
// Falls back to osascript which does not support click actions.
func sendDarwin(title, message, url string) {
	if path, err := exec.LookPath("terminal-notifier"); err == nil {
		args := []string{"-title", title, "-message", message}
		if url != "" {
			args = append(args, "-open", url)
		}
		_ = exec.Command(path, args...).Start()
		return
	}
	script := fmt.Sprintf(`display notification %q with title %q`, message, title)
	_ = exec.Command("osascript", "-e", script).Start()
}
