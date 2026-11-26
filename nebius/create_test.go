package nebius

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	commonpb "github.com/nebius/gosdk/proto/nebius/common/v1"
	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAPI struct {
	preflightErr   error
	preflightCalls int

	found   []*computepb.Instance
	findErr error

	created     *computepb.Instance
	createErr   error
	createCalls int
	createReq   *computepb.CreateInstanceRequest
	createKey   string

	deleteErr   error
	deleteCalls []string

	startErr   error
	startCalls []string
	stopErr    error
	stopCalls  []string

	instances map[string]*computepb.Instance
}

func (f *fakeAPI) Preflight(context.Context, PreflightConfig) error {
	f.preflightCalls++
	return f.preflightErr
}

func (f *fakeAPI) FindInstancesByName(context.Context, string, string) ([]*computepb.Instance, error) {
	return f.found, f.findErr
}

func (f *fakeAPI) GetInstance(_ context.Context, id string) (*computepb.Instance, error) {
	if instance, ok := f.instances[id]; ok {
		return instance, nil
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (f *fakeAPI) CreateInstance(_ context.Context, req *computepb.CreateInstanceRequest, key string, _ time.Duration) (*computepb.Instance, error) {
	f.createCalls++
	f.createReq = req
	f.createKey = key
	return f.created, f.createErr
}

func (f *fakeAPI) DeleteInstance(_ context.Context, id string, _ time.Duration) error {
	f.deleteCalls = append(f.deleteCalls, id)
	return f.deleteErr
}

func (f *fakeAPI) StartInstance(_ context.Context, id string, _ time.Duration) error {
	f.startCalls = append(f.startCalls, id)
	if f.startErr != nil {
		return f.startErr
	}
	if instance := f.instances[id]; instance != nil && instance.Status != nil {
		instance.Status.State = computepb.InstanceStatus_RUNNING
	}
	return nil
}

func (f *fakeAPI) StopInstance(_ context.Context, id string, _ time.Duration) error {
	f.stopCalls = append(f.stopCalls, id)
	if f.stopErr != nil {
		return f.stopErr
	}
	if instance := f.instances[id]; instance != nil && instance.Status != nil {
		instance.Status.State = computepb.InstanceStatus_STOPPED
	}
	return nil
}

func testInstance(id, name, publicIP string) *computepb.Instance {
	instance := &computepb.Instance{
		Metadata: &commonpb.ResourceMetadata{Id: id, ParentId: "project-test", Name: name},
		Status: &computepb.InstanceStatus{
			State: computepb.InstanceStatus_RUNNING,
			NetworkInterfaces: []*computepb.NetworkInterfaceStatus{{
				IpAddress: &computepb.IPAddressStatus{Address: "10.0.0.10"},
			}},
		},
	}
	if publicIP != "" {
		instance.Status.NetworkInterfaces[0].PublicIpAddress = &computepb.PublicIPAddressStatus{Address: publicIP}
	}
	return instance
}

func newDriverWithAPI(t *testing.T, api nebiusAPI) *Driver {
	t.Helper()
	d := NewDriver("node-1", t.TempDir())
	if err := d.SetConfigFromFlags(fullOptions()); err != nil {
		t.Fatal(err)
	}
	d.client = api
	d.waitSSH = func(context.Context, string, string, ssh.Signer, time.Duration) error { return nil }
	return d
}

func newTestDriver(t *testing.T, api *fakeAPI) *Driver {
	t.Helper()
	return newDriverWithAPI(t, api)
}

func TestCreateCreatesWhenAbsent(t *testing.T) {
	api := &fakeAPI{created: testInstance("computeinstance-1", "node-1", "203.0.113.10")}
	d := newTestDriver(t, api)
	var sshAddress string
	d.waitSSH = func(_ context.Context, address, _ string, _ ssh.Signer, _ time.Duration) error {
		sshAddress = address
		return nil
	}

	if err := d.Create(); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if api.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", api.createCalls)
	}
	if d.InstanceID != "computeinstance-1" || d.IPAddress != "203.0.113.10" {
		t.Errorf("instance state not recorded: id=%q ip=%q", d.InstanceID, d.IPAddress)
	}
	if sshAddress != "203.0.113.10:2222" {
		t.Errorf("SSH wait address = %q, want 203.0.113.10:2222", sshAddress)
	}
	if api.createKey != IdempotencyKey("project-test", "node-1") {
		t.Errorf("idempotency key = %q", api.createKey)
	}
	labels := api.createReq.GetMetadata().GetLabels()
	if labels[managedByLabel] != managedByValue || labels[rancherMachineLabel] != "node-1" {
		t.Errorf("ownership labels missing: %v", labels)
	}
	if labels["team"] != "test" {
		t.Errorf("user labels missing: %v", labels)
	}
	if !strings.Contains(api.createReq.GetSpec().GetCloudInitUserData(), "#cloud-config") {
		t.Error("cloud-init missing from create request")
	}
	if strings.Contains(api.createReq.GetSpec().GetCloudInitUserData(), "package_upgrade") {
		t.Error("cloud-init must not enable package_upgrade")
	}
	if len(api.deleteCalls) != 0 {
		t.Errorf("unexpected delete calls: %v", api.deleteCalls)
	}
}

func TestCreateAdoptsExistingManagedInstance(t *testing.T) {
	existing := testInstance("computeinstance-adopt", "node-1", "203.0.113.11")
	existing.Metadata.Labels = map[string]string{managedByLabel: managedByValue}
	api := &fakeAPI{found: []*computepb.Instance{existing}}
	d := newTestDriver(t, api)

	if err := d.Create(); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if api.createCalls != 0 {
		t.Errorf("create calls = %d, want 0 on adopt", api.createCalls)
	}
	if d.InstanceID != "computeinstance-adopt" || d.IPAddress != "203.0.113.11" {
		t.Errorf("adopted instance not recorded: id=%q ip=%q", d.InstanceID, d.IPAddress)
	}
	if len(api.deleteCalls) != 0 {
		t.Errorf("adopt must not delete: %v", api.deleteCalls)
	}
}

func TestCreateRefusesForeignKey(t *testing.T) {
	existing := testInstance("computeinstance-foreign", "node-1", "203.0.113.12")
	existing.Metadata.Labels = map[string]string{managedByLabel: "other-driver"}
	api := &fakeAPI{found: []*computepb.Instance{existing}}
	d := newTestDriver(t, api)

	err := d.Create()
	if err == nil || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("expected foreign-instance error, got %v", err)
	}
	if api.createCalls != 0 || len(api.deleteCalls) != 0 {
		t.Errorf("foreign instance must not be created or deleted: create=%d delete=%v", api.createCalls, api.deleteCalls)
	}
}

func TestCreateRefusesMultipleInstances(t *testing.T) {
	first := testInstance("computeinstance-1", "node-1", "203.0.113.13")
	first.Metadata.Labels = map[string]string{managedByLabel: managedByValue}
	second := testInstance("computeinstance-2", "node-1", "203.0.113.14")
	api := &fakeAPI{found: []*computepb.Instance{first, second}}
	d := newTestDriver(t, api)

	err := d.Create()
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("expected duplicate-instance error, got %v", err)
	}
	if api.createCalls != 0 || len(api.deleteCalls) != 0 {
		t.Errorf("duplicates must not be created or deleted: create=%d delete=%v", api.createCalls, api.deleteCalls)
	}
}

func TestCreateCompensatesOnSSHFailure(t *testing.T) {
	api := &fakeAPI{created: testInstance("computeinstance-1", "node-1", "203.0.113.15")}
	d := newTestDriver(t, api)
	d.waitSSH = func(context.Context, string, string, ssh.Signer, time.Duration) error {
		return errors.New("ssh not ready")
	}

	if err := d.Create(); err == nil {
		t.Fatal("expected SSH failure")
	}
	if len(api.deleteCalls) != 1 || api.deleteCalls[0] != "computeinstance-1" {
		t.Fatalf("compensating delete = %v, want [computeinstance-1]", api.deleteCalls)
	}
	if d.InstanceID != "" || d.IPAddress != "" {
		t.Errorf("compensated state not cleared: id=%q ip=%q", d.InstanceID, d.IPAddress)
	}
}

func TestCreateCompensatesOnMissingPublicIP(t *testing.T) {
	api := &fakeAPI{created: testInstance("computeinstance-1", "node-1", "")}
	d := newTestDriver(t, api)

	if err := d.Create(); err == nil {
		t.Fatal("expected missing public IP error")
	}
	if len(api.deleteCalls) != 1 || api.deleteCalls[0] != "computeinstance-1" {
		t.Fatalf("compensating delete = %v, want [computeinstance-1]", api.deleteCalls)
	}
}

func TestCreateStopsOnPreflightFailure(t *testing.T) {
	api := &fakeAPI{preflightErr: status.Error(codes.NotFound, "project missing")}
	d := newTestDriver(t, api)

	if err := d.Create(); err == nil {
		t.Fatal("expected preflight failure")
	}
	if api.preflightCalls != 1 {
		t.Errorf("preflight calls = %d, want 1", api.preflightCalls)
	}
	if api.createCalls != 0 {
		t.Errorf("create calls = %d, want 0 after preflight failure", api.createCalls)
	}
}

func TestCreatePrivateIPWithUserData(t *testing.T) {
	userdataPath := filepath.Join(t.TempDir(), "userdata.yaml")
	raw := "#cloud-config\nruncmd:\n  - echo hello\nwrite_files:\n  - path: /etc/motd\n    content: hi\n"
	if err := os.WriteFile(userdataPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	api := &fakeAPI{created: testInstance("computeinstance-1", "node-1", "")}
	d := NewDriver("node-1", t.TempDir())
	opts := fullOptions()
	opts["nebius-public-ip"] = "none"
	opts["nebius-userdata"] = userdataPath
	if err := d.SetConfigFromFlags(opts); err != nil {
		t.Fatal(err)
	}
	d.client = api
	d.waitSSH = func(context.Context, string, string, ssh.Signer, time.Duration) error {
		t.Error("waitSSH must not be called when userdata is set")
		return nil
	}

	if err := d.Create(); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if api.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", api.createCalls)
	}
	if d.IPAddress != "10.0.0.10" {
		t.Errorf("IPAddress = %q, want private IP 10.0.0.10", d.IPAddress)
	}
	cloudInit := api.createReq.GetSpec().GetCloudInitUserData()
	for _, want := range []string{"runcmd", "write_files", "ssh_authorized_keys"} {
		if !strings.Contains(cloudInit, want) {
			t.Errorf("cloud-init missing %q", want)
		}
	}
	if nic := api.createReq.GetSpec().GetNetworkInterfaces()[0]; nic.GetPublicIpAddress() != nil {
		t.Errorf("private-only request has a public IP: %v", nic.GetPublicIpAddress())
	}
	if len(api.deleteCalls) != 0 {
		t.Errorf("unexpected delete calls: %v", api.deleteCalls)
	}
}

func TestCreateNoneWithoutUserDataFails(t *testing.T) {
	api := &fakeAPI{}
	d := NewDriver("node-1", t.TempDir())
	opts := fullOptions()
	opts["nebius-public-ip"] = "none"
	if err := d.SetConfigFromFlags(opts); err != nil {
		t.Fatal(err)
	}
	d.client = api

	err := d.Create()
	if err == nil {
		t.Fatal("expected error for public-ip=none without userdata")
	}
	if !strings.Contains(err.Error(), "userdata") {
		t.Errorf("error does not mention userdata: %v", err)
	}
	if api.preflightCalls != 0 {
		t.Errorf("preflight calls = %d, want 0", api.preflightCalls)
	}
	if api.createCalls != 0 || len(api.deleteCalls) != 0 {
		t.Errorf("must not create or delete: create=%d delete=%v", api.createCalls, api.deleteCalls)
	}
}

func TestCreateMissingUserDataFileFails(t *testing.T) {
	api := &fakeAPI{}
	d := NewDriver("node-1", t.TempDir())
	opts := fullOptions()
	opts["nebius-userdata"] = filepath.Join(t.TempDir(), "missing.yaml")
	if err := d.SetConfigFromFlags(opts); err != nil {
		t.Fatal(err)
	}
	d.client = api

	err := d.Create()
	if err == nil || !strings.Contains(err.Error(), "userdata") {
		t.Fatalf("expected missing-userdata error, got %v", err)
	}
	if api.createCalls != 0 || len(api.deleteCalls) != 0 {
		t.Errorf("must not create or delete: create=%d delete=%v", api.createCalls, api.deleteCalls)
	}
}

func TestCreateNonCloudConfigUserDataFails(t *testing.T) {
	userdataPath := filepath.Join(t.TempDir(), "userdata.sh")
	if err := os.WriteFile(userdataPath, []byte("#!/bin/bash\necho hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	api := &fakeAPI{}
	d := NewDriver("node-1", t.TempDir())
	opts := fullOptions()
	opts["nebius-userdata"] = userdataPath
	if err := d.SetConfigFromFlags(opts); err != nil {
		t.Fatal(err)
	}
	d.client = api

	err := d.Create()
	if err == nil || !strings.Contains(err.Error(), "cloud-config") {
		t.Fatalf("expected non-cloud-config error, got %v", err)
	}
	if api.createCalls != 0 || len(api.deleteCalls) != 0 {
		t.Errorf("must not create or delete: create=%d delete=%v", api.createCalls, api.deleteCalls)
	}
}

func TestPreCreateCheckSkipsPreflightOnStaticError(t *testing.T) {
	api := &fakeAPI{}
	d := NewDriver("node-1", t.TempDir())
	d.client = api

	if err := d.PreCreateCheck(); err == nil {
		t.Fatal("expected static validation error")
	}
	if api.preflightCalls != 0 {
		t.Errorf("preflight calls = %d, want 0", api.preflightCalls)
	}
}
