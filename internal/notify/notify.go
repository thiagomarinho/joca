package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Send displays an OS notification with the given title and message.
// It is a best-effort call — errors are silently ignored so a notification
// failure never disrupts the TUI.
func Send(title, message string) {
	if runtime.GOOS == "darwin" {
		sendDarwin(title, message)
	}
}

func sendDarwin(title, message string) {
	script := fmt.Sprintf(
		`display notification %q with title %q`,
		message, title,
	)
	_ = exec.Command("osascript", "-e", script).Start()
}
