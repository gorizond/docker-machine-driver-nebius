package nebius

import (
	"os"
	"strings"
	"testing"

	"github.com/docker/machine/libmachine/drivers"
	"golang.org/x/crypto/ssh"
)

func TestEnsureSSHKey(t *testing.T) {
	d := NewDriver("node-1", t.TempDir())

	signer, err := d.ensureSSHKey()
	if err != nil {
		t.Fatalf("ensureSSHKey: %v", err)
	}

	path := d.GetSSHKeyPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("key permissions = %o, want 600", perm)
	}

	authorized, err := d.SSHAuthorizedKey()
	if err != nil {
		t.Fatalf("SSHAuthorizedKey: %v", err)
	}
	if !strings.HasPrefix(authorized, "ssh-ed25519 ") {
		t.Errorf("public key %q is not ed25519", authorized)
	}

	// Second call must reuse the persisted key.
	signer2, err := d.ensureSSHKey()
	if err != nil {
		t.Fatalf("ensureSSHKey again: %v", err)
	}
	if string(ssh.MarshalAuthorizedKey(signer.PublicKey())) !=
		string(ssh.MarshalAuthorizedKey(signer2.PublicKey())) {
		t.Fatal("SSH key was regenerated instead of reused")
	}
}

func TestDriverDefaultsAndName(t *testing.T) {
	d := NewDriver("node-1", "/tmp/store")
	if d.DriverName() != "nebius" {
		t.Errorf("DriverName = %q, want nebius", d.DriverName())
	}
	if d.GetMachineName() != "node-1" {
		t.Errorf("machine name = %q", d.GetMachineName())
	}
	if d.GetSSHUsername() != "ubuntu" {
		t.Errorf("ssh user = %q", d.GetSSHUsername())
	}
}

var _ drivers.Driver = (*Driver)(nil)
