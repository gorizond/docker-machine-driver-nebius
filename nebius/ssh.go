package nebius

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	sshProbeInterval = 3 * time.Second
	sshDialTimeout   = 10 * time.Second
)

// ensureSSHKey returns the machine SSH signer. It reuses the key at
// BaseDriver.GetSSHKeyPath() or generates a new ed25519 key pair with 0600
// permissions. The private key never leaves the machine store.
func (d *Driver) ensureSSHKey() (ssh.Signer, error) {
	path := d.GetSSHKeyPath()
	if data, err := os.ReadFile(path); err == nil {
		if signer, err := ssh.ParsePrivateKey(data); err == nil {
			return signer, nil
		}
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("nebius: generate SSH key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(privateKey, "nebius")
	if err != nil {
		return nil, fmt.Errorf("nebius: marshal SSH key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("nebius: create SSH key directory: %w", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, fmt.Errorf("nebius: write SSH key: %w", err)
	}
	return ssh.NewSignerFromKey(privateKey)
}

// SSHAuthorizedKey returns the authorized_keys line for the machine key.
func (d *Driver) SSHAuthorizedKey() (string, error) {
	signer, err := d.ensureSSHKey()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), nil
}

// waitForSSH blocks until an SSH login succeeds or the timeout expires.
func waitForSSH(ctx context.Context, addr, user string, signer ssh.Signer, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         sshDialTimeout,
	}
	for {
		client, err := ssh.Dial("tcp", addr, cfg)
		if err == nil {
			_ = client.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("nebius: SSH on %s was not ready within %s", addr, timeout)
			}
			return fmt.Errorf("nebius: waiting for SSH on %s: %w", addr, ctx.Err())
		case <-time.After(sshProbeInterval):
		}
	}
}
