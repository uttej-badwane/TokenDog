// Package proxy implements TokenDog's HTTPS proxy mode — the architectural
// pivot from the PreToolUse hook approach. The proxy intercepts every
// request Claude Code (or any AI client respecting HTTPS_PROXY) sends to
// api.anthropic.com, parses the Anthropic Messages API payload, filters
// the tool_result content blocks, and forwards the modified payload.
//
// Why this exists: the hook architecture only lets TD modify a tool's
// INPUT, never its output. To get any leverage at all, TD has to wrap
// commands (`td git status`), which (a) shows up in shell history and
// (b) only addresses Bash output (~5-8% of total tokens). The proxy
// addresses everything the model receives — Bash, file reads, MCP
// responses, web fetches — and does so invisibly.
//
// Cert subsystem (this file): generates a per-machine CA cert + key,
// stored under ~/.config/tokendog/proxy/. The CA signs leaf certs for
// api.anthropic.com on the fly during MITM. The CA cert must be installed
// in the system trust store for the AI client's TLS to validate.
package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// CertDir returns the directory holding the CA cert + key. Callers create
// it on first use; this function only computes the path.
func CertDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tokendog", "proxy"), nil
}

// CACertPath / CAKeyPath are the canonical filenames for the CA materials.
func CACertPath() (string, error) {
	dir, err := CertDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ca-cert.pem"), nil
}

func CAKeyPath() (string, error) {
	dir, err := CertDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ca-key.pem"), nil
}

// CAExists reports whether a usable CA cert + key already live on disk.
// Callers use this to decide between "first install" and "reuse existing"
// flows. We never overwrite an existing CA — replacing it would break
// every downstream client that already trusts the old one.
func CAExists() (bool, error) {
	cert, err := CACertPath()
	if err != nil {
		return false, err
	}
	key, err := CAKeyPath()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(cert); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if _, err := os.Stat(key); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GenerateCA creates a new CA cert + private key on disk. Idempotent — if
// CAExists() returns true, this is a no-op. Returns the cert path on
// success.
//
// Cert details: ECDSA P-256 (small, modern, fast), 10-year validity,
// CN "TokenDog Local CA — <hostname>". The hostname suffix lets users
// distinguish multiple machines' CAs in their keychain.
func GenerateCA() (string, error) {
	exists, err := CAExists()
	if err != nil {
		return "", err
	}
	certPath, _ := CACertPath()
	if exists {
		return certPath, nil
	}

	dir, err := CertDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "local"
	}
	serialMax := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialMax)
	if err != nil {
		return "", err
	}

	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "TokenDog Local CA — " + hostname,
			Organization: []string{"TokenDog"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, priv.Public(), priv)
	if err != nil {
		return "", fmt.Errorf("create cert: %w", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", err
	}

	if err := writePEM(certPath, "CERTIFICATE", derBytes, 0644); err != nil {
		return "", err
	}
	keyPath, _ := CAKeyPath()
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyBytes, 0600); err != nil {
		return "", err
	}
	return certPath, nil
}

func writePEM(path, blockType string, derBytes []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: derBytes})
}
