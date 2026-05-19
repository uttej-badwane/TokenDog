//go:build linux

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// InstallCert tries to auto-install the TokenDog CA into the system trust
// store. We attempt the two most common paths (Debian/Ubuntu with
// update-ca-certificates, then Fedora/RHEL/Arch with update-ca-trust).
// Both require sudo; if neither tool is available or the copy fails, we
// fall back to clear manual instructions.
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

	// Attempt Debian/Ubuntu path.
	if path, err := installCertDebian(certPath); err == nil {
		return path, nil
	}
	// Attempt Fedora/RHEL/Arch path.
	if path, err := installCertRHEL(certPath); err == nil {
		return path, nil
	}

	// Neither tool present or both failed — return instructions.
	return certPath, fmt.Errorf("could not auto-install cert (sudo required); install manually:\n" +
		"  Debian/Ubuntu: sudo cp " + certPath + " /usr/local/share/ca-certificates/tokendog.crt && sudo update-ca-certificates\n" +
		"  Fedora/RHEL:   sudo cp " + certPath + " /etc/pki/ca-trust/source/anchors/ && sudo update-ca-trust\n" +
		"  Arch:          sudo trust anchor --store " + certPath)
}

func installCertDebian(certPath string) (string, error) {
	if _, err := exec.LookPath("update-ca-certificates"); err != nil {
		return "", fmt.Errorf("update-ca-certificates not found")
	}
	dest := "/usr/local/share/ca-certificates/tokendog.crt"
	cpCmd := exec.Command("sudo", "cp", certPath, dest)
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("sudo cp: %w\n%s", err, out)
	}
	if out, err := exec.Command("sudo", "update-ca-certificates").CombinedOutput(); err != nil {
		return "", fmt.Errorf("update-ca-certificates: %w\n%s", err, out)
	}
	return dest, nil
}

func installCertRHEL(certPath string) (string, error) {
	if _, err := exec.LookPath("update-ca-trust"); err != nil {
		return "", fmt.Errorf("update-ca-trust not found")
	}
	dest := "/etc/pki/ca-trust/source/anchors/tokendog.crt"
	cpCmd := exec.Command("sudo", "cp", certPath, dest)
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("sudo cp: %w\n%s", err, out)
	}
	if out, err := exec.Command("sudo", "update-ca-trust").CombinedOutput(); err != nil {
		return "", fmt.Errorf("update-ca-trust: %w\n%s", err, out)
	}
	return dest, nil
}

// UninstallCert attempts to remove the cert from known Linux trust store
// locations.
func UninstallCert() error {
	candidates := []struct {
		path   string
		reload string
	}{
		{"/usr/local/share/ca-certificates/tokendog.crt", "update-ca-certificates"},
		{"/etc/pki/ca-trust/source/anchors/tokendog.crt", "update-ca-trust"},
	}
	removed := false
	for _, c := range candidates {
		if _, err := os.Stat(c.path); err != nil {
			continue
		}
		if out, err := exec.Command("sudo", "rm", c.path).CombinedOutput(); err != nil {
			return fmt.Errorf("sudo rm %s: %w\n%s", c.path, err, out)
		}
		_ = exec.Command("sudo", c.reload).Run()
		removed = true
	}
	if !removed {
		certPath, _ := CACertPath()
		return fmt.Errorf("cert not found at known Linux trust store paths; remove %s manually", filepath.Base(certPath))
	}
	return nil
}
