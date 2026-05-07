//go:build darwin

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// InstallCert adds the CA cert to the macOS login keychain marked as
// trusted for SSL. Uses `security add-trusted-cert` which is the same
// command Apple's docs and mitmproxy recommend.
//
// The user will be prompted for their login password — `security` requires
// it for trust changes. We can't prompt directly, so we just exec and let
// the system's TouchID / password prompt handle it.
//
// Returns the path that was installed and an error. If the cert is already
// trusted, this is a no-op (security tolerates duplicate adds).
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

	// add-trusted-cert flags:
	//   -r trustRoot mark as trusted root
	//   -k <kc>      target keychain (full path; exec.Command doesn't
	//                expand ~)
	//
	// We use the user's login keychain rather than /Library/Keychains/System.
	// The login keychain doesn't require sudo and the trust is per-user.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	homeKC := filepath.Join(home, "Library", "Keychains", "login.keychain-db")
	cmd := exec.Command("security", "add-trusted-cert",
		"-r", "trustRoot",
		"-k", homeKC,
		certPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("security add-trusted-cert (need TouchID/password approval): %w\n%s", err, out)
	}
	return certPath, nil
}

// UninstallCert removes the CA cert from the macOS login keychain. Used by
// `td proxy uninstall`. Idempotent — silently succeeds if the cert isn't
// trusted (or already removed).
func UninstallCert() error {
	certPath, err := CACertPath()
	if err != nil {
		return err
	}
	cmd := exec.Command("security", "delete-certificate", "-c", "TokenDog Local CA", certPath)
	// Allow exit code 44 (item not found) — that's an expected idempotent
	// state. Other errors propagate.
	if out, err := cmd.CombinedOutput(); err != nil {
		// security exits 44 when the cert isn't in the keychain.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 44 {
			return nil
		}
		return fmt.Errorf("security delete-certificate: %w\n%s", err, out)
	}
	return nil
}
