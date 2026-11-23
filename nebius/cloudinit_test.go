package nebius

import (
	"strings"
	"testing"
)

const testPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB7testkey nebius"

func TestRenderCloudInit(t *testing.T) {
	out, err := RenderCloudInit(CloudInit{
		User:              "ubuntu",
		SSHAuthorizedKeys: []string{testPublicKey},
	})
	if err != nil {
		t.Fatalf("RenderCloudInit: %v", err)
	}
	for _, want := range []string{
		"#cloud-config\n",
		"name: \"ubuntu\"",
		"sudo: ALL=(ALL) NOPASSWD:ALL",
		"shell: /bin/bash",
		"ssh_authorized_keys:",
		testPublicKey,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("cloud-init missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "package_upgrade") {
		t.Errorf("cloud-init must not enable package_upgrade:\n%s", out)
	}
}

func TestRenderCloudInitValidation(t *testing.T) {
	if _, err := RenderCloudInit(CloudInit{SSHAuthorizedKeys: []string{testPublicKey}}); err == nil {
		t.Error("expected error for empty user")
	}
	if _, err := RenderCloudInit(CloudInit{User: "ubuntu"}); err == nil {
		t.Error("expected error for missing public key")
	}
	if _, err := RenderCloudInit(CloudInit{User: "ubuntu", SSHAuthorizedKeys: []string{"   "}}); err == nil {
		t.Error("expected error for blank public key")
	}
}

func TestRenderCloudInitSizeLimit(t *testing.T) {
	huge := "ssh-ed25519 " + strings.Repeat("A", MaxCloudInitBytes)
	_, err := RenderCloudInit(CloudInit{
		User:              "ubuntu",
		SSHAuthorizedKeys: []string{huge},
	})
	if err == nil {
		t.Fatal("expected size limit error")
	}
	if !strings.Contains(err.Error(), "32768") {
		t.Errorf("error does not mention the API limit: %v", err)
	}
}
