//go:build !darwin

package proxy

import (
	"fmt"
	"runtime"
)

// InstallCert on non-macOS just returns the cert path and tells the user
// what to do. We don't auto-install on Linux/Windows (yet) because the
// trust-store conventions vary per distro/distro-version and per Windows
// elevation model — better to print clear instructions than to silently
// break.
func InstallCert() (string, error) {
	certPath, err := CACertPath()
	if err != nil {
		return "", err
	}
	exists, err := CAExists()
	if err != nil {
		return "", err
	}
	if !exists {
		if _, err := GenerateCA(); err != nil {
			return "", err
		}
	}
	return certPath, fmt.Errorf("auto-install not yet supported on %s; install %s into your system trust store manually:\n"+
		"  Linux (Debian/Ubuntu): sudo cp %s /usr/local/share/ca-certificates/tokendog.crt && sudo update-ca-certificates\n"+
		"  Linux (Fedora/RHEL):   sudo cp %s /etc/pki/ca-trust/source/anchors/ && sudo update-ca-trust\n"+
		"  Windows: certutil -addstore -f Root %s",
		runtime.GOOS, certPath, certPath, certPath, certPath)
}

func UninstallCert() error {
	return fmt.Errorf("auto-uninstall not yet supported on %s; remove the cert manually from your trust store", runtime.GOOS)
}
