//go:build !darwin

package proxy

import (
	"fmt"
	"runtime"
)

// DaemonInstall on non-macOS platforms returns instructions for the
// equivalent local supervisor on this OS. Linux: write a systemd --user
// unit. Windows: register a scheduled task. Both are platform-specific
// enough that we punt rather than build them half-right.
func DaemonInstall() (string, error) {
	return "", fmt.Errorf("auto-start daemon install not yet supported on %s. Manual setup:\n"+
		"  Linux: create ~/.config/systemd/user/tokendog-proxy.service then `systemctl --user enable --now tokendog-proxy`\n"+
		"  Windows: schtasks /Create /TN TokenDogProxy /TR \"td proxy start\" /SC ONLOGON /RL HIGHEST", runtime.GOOS)
}

func DaemonUninstall() error {
	return fmt.Errorf("auto-start daemon uninstall not yet supported on %s — see DaemonInstall message for the command you used to install", runtime.GOOS)
}

func DaemonStatus() (string, error) {
	return fmt.Sprintf("daemon supervision not supported on %s; use your OS's service manager", runtime.GOOS), nil
}
