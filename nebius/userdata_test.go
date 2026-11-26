package nebius

import (
	"strings"
	"testing"
)

const testExistingKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExistingKey rancher"

func TestMergeUserDataPreservesSections(t *testing.T) {
	raw := "#cloud-config\n" +
		"runcmd:\n  - echo hello\n" +
		"write_files:\n  - path: /etc/motd\n    content: hi\n"

	out, err := MergeUserData(raw, CloudInit{
		User:              "ubuntu",
		SSHAuthorizedKeys: []string{testPublicKey},
	})
	if err != nil {
		t.Fatalf("MergeUserData: %v", err)
	}
	for _, want := range []string{
		"runcmd",
		"echo hello",
		"write_files",
		"/etc/motd",
		"ssh_authorized_keys",
		testPublicKey,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("merged userdata missing %q:\n%s", want, out)
		}
	}
}

func TestMergeUserDataAppendsKeyToExistingUser(t *testing.T) {
	raw := "#cloud-config\n" +
		"users:\n" +
		"  - name: ubuntu\n" +
		"    shell: /bin/zsh\n" +
		"    ssh_authorized_keys:\n" +
		"      - " + testExistingKey + "\n"

	out, err := MergeUserData(raw, CloudInit{
		User:              "ubuntu",
		SSHAuthorizedKeys: []string{testPublicKey, "  " + testPublicKey + "  "},
	})
	if err != nil {
		t.Fatalf("MergeUserData: %v", err)
	}
	if strings.Count(out, testExistingKey) != 1 {
		t.Errorf("existing key lost or duplicated:\n%s", out)
	}
	if strings.Count(out, testPublicKey) != 1 {
		t.Errorf("merged key not deduplicated:\n%s", out)
	}
	if !strings.Contains(out, "/bin/zsh") {
		t.Errorf("existing shell not preserved:\n%s", out)
	}
}

func TestMergeUserDataEmptyMatchesRender(t *testing.T) {
	cfg := CloudInit{User: "ubuntu", SSHAuthorizedKeys: []string{testPublicKey}}
	want, err := RenderCloudInit(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := MergeUserData("  \n", cfg)
	if err != nil {
		t.Fatalf("MergeUserData: %v", err)
	}
	if got != want {
		t.Errorf("empty userdata = %q, want RenderCloudInit output %q", got, want)
	}
}

func TestMergeUserDataRejectsNonCloudConfig(t *testing.T) {
	_, err := MergeUserData("#!/bin/bash\necho hi\n", CloudInit{
		User:              "ubuntu",
		SSHAuthorizedKeys: []string{testPublicKey},
	})
	if err == nil {
		t.Fatal("expected error for non-cloud-config userdata")
	}
	if !strings.Contains(err.Error(), "cloud-config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMergeUserDataSizeLimit(t *testing.T) {
	raw := "#cloud-config\nwrite_files:\n  - path: /big\n    content: " + strings.Repeat("A", MaxCloudInitBytes) + "\n"
	_, err := MergeUserData(raw, CloudInit{
		User:              "ubuntu",
		SSHAuthorizedKeys: []string{testPublicKey},
	})
	if err == nil {
		t.Fatal("expected size limit error")
	}
	if !strings.Contains(err.Error(), "32768") {
		t.Errorf("error does not mention the API limit: %v", err)
	}
}
