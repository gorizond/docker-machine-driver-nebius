package nebius

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeOptions map[string]any

func (o fakeOptions) String(key string) string {
	v, _ := o[key].(string)
	return v
}

func (o fakeOptions) StringSlice(key string) []string {
	v, _ := o[key].([]string)
	return v
}

func (o fakeOptions) Int(key string) int {
	v, _ := o[key].(int)
	return v
}

func (o fakeOptions) Bool(key string) bool {
	v, _ := o[key].(bool)
	return v
}

func TestCreateFlagsAreNamespaced(t *testing.T) {
	for _, flag := range NewDriver("node-1", "/tmp/store").GetCreateFlags() {
		name := flag.String()
		if !strings.HasPrefix(name, "nebius-") {
			t.Errorf("flag %q does not start with nebius-", name)
		}
	}
}

func TestCreateFlagsSingleUserDataFlag(t *testing.T) {
	markers := []string{"user-data", "userdata", "custom-data", "cloud-config"}
	var found []string
	for _, flag := range NewDriver("node-1", "/tmp/store").GetCreateFlags() {
		name := strings.ToLower(flag.String())
		for _, marker := range markers {
			if strings.Contains(name, marker) {
				found = append(found, name)
				break
			}
		}
	}
	if len(found) != 1 || found[0] != "nebius-userdata" {
		t.Fatalf("userdata flags = %v, want exactly [nebius-userdata]", found)
	}
}

func TestCreateFlagsExactSet(t *testing.T) {
	expected := []string{
		"nebius-sa-id",
		"nebius-public-key-id",
		"nebius-private-key",
		"nebius-private-key-path",
		"nebius-project-id",
		"nebius-api-endpoint",
		"nebius-subnet-id",
		"nebius-image-family",
		"nebius-image-id",
		"nebius-platform",
		"nebius-preset",
		"nebius-disk-size-gib",
		"nebius-disk-type",
		"nebius-public-ip",
		"nebius-userdata",
		"nebius-security-groups",
		"nebius-preemptible",
		"nebius-ssh-user",
		"nebius-ssh-port",
		"nebius-create-timeout",
		"nebius-delete-timeout",
		"nebius-labels",
	}

	flags := NewDriver("node-1", "/tmp/store").GetCreateFlags()
	if len(flags) != len(expected) {
		t.Fatalf("got %d flags, want %d", len(flags), len(expected))
	}
	got := make(map[string]interface{}, len(flags))
	for _, flag := range flags {
		got[flag.String()] = flag.Default()
	}
	for _, name := range expected {
		if _, ok := got[name]; !ok {
			t.Errorf("missing flag %q", name)
		}
	}
}

func TestCreateFlagDefaults(t *testing.T) {
	defaults := make(map[string]interface{})
	for _, flag := range NewDriver("node-1", "/tmp/store").GetCreateFlags() {
		defaults[flag.String()] = flag.Default()
	}

	want := map[string]interface{}{
		"nebius-api-endpoint":   "api.nebius.cloud:443",
		"nebius-disk-size-gib":  50,
		"nebius-disk-type":      "NETWORK_SSD",
		"nebius-public-ip":      "static",
		"nebius-userdata":       "",
		"nebius-ssh-user":       "ubuntu",
		"nebius-ssh-port":       22,
		"nebius-create-timeout": "20m",
		"nebius-delete-timeout": "5m",
		"nebius-image-family":   "",
		"nebius-image-id":       "",
	}
	for name, want := range want {
		if got := defaults[name]; got != want {
			t.Errorf("default for %s = %v, want %v", name, got, want)
		}
	}
}

func fullOptions() fakeOptions {
	return fakeOptions{
		"nebius-sa-id":            "serviceaccount-test",
		"nebius-public-key-id":    "publickey-test",
		"nebius-private-key-path": "/tmp/key.pem",
		"nebius-project-id":       "project-test",
		"nebius-subnet-id":        "vpcsubnet-test",
		"nebius-image-family":     "ubuntu24.04-driverless",
		"nebius-platform":         "cpu-d3",
		"nebius-preset":           "4vcpu-16gb",
		"nebius-security-groups":  []string{"sg-1", "sg-2"},
		"nebius-labels":           []string{"team:test", "env:dev"},
		"nebius-ssh-user":         "ubuntu",
		"nebius-ssh-port":         2222,
		"nebius-public-ip":        "dynamic",
		"nebius-disk-size-gib":    80,
		"nebius-disk-type":        "NETWORK_HDD",
		"nebius-preemptible":      true,
		"nebius-create-timeout":   "30m",
		"nebius-delete-timeout":   "7m",
	}
}

func TestSetConfigFromFlags(t *testing.T) {
	d := NewDriver("node-1", "/tmp/store")
	opts := fullOptions()
	opts["nebius-userdata"] = "/tmp/userdata.yaml"
	if err := d.SetConfigFromFlags(opts); err != nil {
		t.Fatalf("SetConfigFromFlags: %v", err)
	}
	if d.SAID != "serviceaccount-test" || d.PublicKeyID != "publickey-test" || d.ProjectID != "project-test" {
		t.Errorf("credentials not parsed: %+v", d)
	}
	if d.SubnetID != "vpcsubnet-test" || d.ImageFamily != "ubuntu24.04-driverless" ||
		d.Platform != "cpu-d3" || d.Preset != "4vcpu-16gb" {
		t.Errorf("machine config not parsed: %+v", d)
	}
	if d.DiskSizeGiB != 80 || d.DiskType != "NETWORK_HDD" || d.PublicIP != "dynamic" || !d.Preemptible {
		t.Errorf("disk/public config not parsed: %+v", d)
	}
	if d.SSHPort != 2222 || d.CreateTimeout.Minutes() != 30 || d.DeleteTimeout.Minutes() != 7 {
		t.Errorf("ssh/timeouts not parsed: %+v", d)
	}
	if len(d.SecurityGroups) != 2 || len(d.Labels) != 2 || d.Labels["team"] != "test" {
		t.Errorf("groups/labels not parsed: %+v", d)
	}
	if d.UserDataFile != "/tmp/userdata.yaml" {
		t.Errorf("userdata path not parsed: %q", d.UserDataFile)
	}
	if err := d.validateStatic(); err != nil {
		t.Errorf("validateStatic on full config: %v", err)
	}
}

func TestSetConfigFromFlagsKeepsDefaults(t *testing.T) {
	d := NewDriver("node-1", "/tmp/store")
	if err := d.SetConfigFromFlags(fakeOptions{}); err != nil {
		t.Fatalf("SetConfigFromFlags: %v", err)
	}
	if d.SSHUser != "ubuntu" || d.SSHPort != 22 || d.DiskSizeGiB != 50 ||
		d.DiskType != "NETWORK_SSD" || d.PublicIP != "static" ||
		d.CreateTimeout != 20*time.Minute || d.DeleteTimeout != 5*time.Minute {
		t.Fatalf("defaults lost: %+v", d)
	}
}

func TestSetConfigFromFlagsRejectsBadDurations(t *testing.T) {
	for _, flag := range []string{"nebius-create-timeout", "nebius-delete-timeout"} {
		d := NewDriver("node-1", "/tmp/store")
		if err := d.SetConfigFromFlags(fakeOptions{flag: "not-a-duration"}); err == nil {
			t.Errorf("%s: expected error for bad duration", flag)
		}
		if err := d.SetConfigFromFlags(fakeOptions{flag: "0s"}); err == nil {
			t.Errorf("%s: expected error for zero duration", flag)
		}
	}
}

func TestPreCreateCheckRequiredFields(t *testing.T) {
	d := NewDriver("node-1", "/tmp/store")
	if err := d.PreCreateCheck(); err == nil {
		t.Fatal("expected error for empty required config")
	}

	d = NewDriver("node-1", "/tmp/store")
	if err := d.SetConfigFromFlags(fullOptions()); err != nil {
		t.Fatal(err)
	}
	if err := d.validateStatic(); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

func TestPreCreateCheckImageRules(t *testing.T) {
	d := NewDriver("node-1", "/tmp/store")
	if err := d.SetConfigFromFlags(fullOptions()); err != nil {
		t.Fatal(err)
	}

	both := fullOptions()
	both["nebius-image-id"] = "computeimage-test"
	if err := d.SetConfigFromFlags(both); err != nil {
		t.Fatal(err)
	}
	if err := d.PreCreateCheck(); err == nil {
		t.Error("expected error when both image-family and image-id are set")
	}

	none := fullOptions()
	delete(none, "nebius-image-family")
	d = NewDriver("node-1", "/tmp/store")
	if err := d.SetConfigFromFlags(none); err != nil {
		t.Fatal(err)
	}
	if err := d.PreCreateCheck(); err == nil {
		t.Error("expected error when no image is configured")
	}
}

func TestPreCreateCheckPublicIP(t *testing.T) {
	d := NewDriver("node-1", "/tmp/store")
	opts := fullOptions()
	opts["nebius-public-ip"] = "floating"
	if err := d.SetConfigFromFlags(opts); err != nil {
		t.Fatal(err)
	}
	if err := d.PreCreateCheck(); err == nil {
		t.Error("expected error for invalid public-ip mode")
	}
}

func TestValidateStaticRequiresUserDataForNone(t *testing.T) {
	d := NewDriver("node-1", "/tmp/store")
	opts := fullOptions()
	opts["nebius-public-ip"] = "none"
	if err := d.SetConfigFromFlags(opts); err != nil {
		t.Fatal(err)
	}
	err := d.validateStatic()
	if err == nil || !strings.Contains(err.Error(), "userdata") {
		t.Fatalf("expected userdata requirement, got %v", err)
	}
}

func TestValidateUserDataFile(t *testing.T) {
	valid := filepath.Join(t.TempDir(), "userdata.yaml")
	if err := os.WriteFile(valid, []byte("#cloud-config\nruncmd:\n  - echo hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	d := NewDriver("node-1", "/tmp/store")
	opts := fullOptions()
	opts["nebius-public-ip"] = "none"
	opts["nebius-userdata"] = valid
	if err := d.SetConfigFromFlags(opts); err != nil {
		t.Fatal(err)
	}
	if err := d.validateStatic(); err != nil {
		t.Errorf("validateStatic rejected valid userdata config: %v", err)
	}
	if err := validateUserDataFile(d.UserDataFile); err != nil {
		t.Errorf("validateUserDataFile rejected valid file: %v", err)
	}

	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("#!/bin/bash\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateUserDataFile(bad); err == nil || !strings.Contains(err.Error(), bad) {
		t.Errorf("non-cloud-config error = %v, want error naming %s", err, bad)
	}

	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if err := validateUserDataFile(missing); err == nil || !strings.Contains(err.Error(), missing) {
		t.Errorf("missing-file error = %v, want error naming %s", err, missing)
	}
}
