package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeKeyFile(t *testing.T, passphrase string) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	if passphrase != "" {
		//nolint:staticcheck // legacy PEM encryption used for test coverage
		block, err = x509.EncryptPEMBlock(rand.Reader, block.Type, block.Bytes, []byte(passphrase), x509.PEMCipherAES256)
		if err != nil {
			t.Fatalf("encrypt key: %v", err)
		}
	}

	pemBytes := pem.EncodeToMemory(block)
	if pemBytes == nil {
		t.Fatal("encode key: got nil")
	}

	path := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	return path
}

func TestLoadPrivateKey_Unencrypted(t *testing.T) {
	path := writeKeyFile(t, "")

	signer, err := LoadPrivateKey(path, nil)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	if signer == nil {
		t.Fatal("LoadPrivateKey: expected signer")
	}
}

func TestLoadPrivateKey_EncryptedRequiresPassphrase(t *testing.T) {
	path := writeKeyFile(t, "s3cret")

	_, err := LoadPrivateKey(path, nil)
	if !errors.Is(err, ErrPassphraseRequired) {
		t.Fatalf("LoadPrivateKey: expected ErrPassphraseRequired, got %v", err)
	}
}

func TestLoadPrivateKey_EncryptedWithPassphrase(t *testing.T) {
	path := writeKeyFile(t, "s3cret")

	signer, err := LoadPrivateKey(path, func(string) (string, error) {
		return "s3cret", nil
	})
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	if signer == nil {
		t.Fatal("LoadPrivateKey: expected signer")
	}
}

func TestLoadPrivateKey_EncryptedWrongPassphrase(t *testing.T) {
	path := writeKeyFile(t, "s3cret")

	_, err := LoadPrivateKey(path, func(string) (string, error) {
		return "wrong", nil
	})
	if err == nil {
		t.Fatal("LoadPrivateKey: expected error for wrong passphrase")
	}
}

func TestConnectAgentMissingSock(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	_, err := ConnectAgent()
	if !errors.Is(err, ErrSSHAgentUnavailable) {
		t.Fatalf("ConnectAgent: expected ErrSSHAgentUnavailable, got %v", err)
	}
}
