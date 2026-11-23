package nebius

import (
	"errors"
	"strings"
	"testing"

	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetryAdoptsInstanceLeftAfterFailedCreate(t *testing.T) {
	instance := testInstance("computeinstance-1", "node-1", "203.0.113.30")
	instance.Metadata.Labels = map[string]string{managedByLabel: managedByValue}

	fake := &fakeAPI{
		createErr: status.Error(codes.Unavailable, "connection reset after create"),
		instances: map[string]*computepb.Instance{"computeinstance-1": instance},
	}
	api := &scriptedAPI{
		fakeAPI: fake,
		// 1st Create: absent. Cleanup lookup: network failure (no delete).
		// 2nd Create: the instance exists and must be adopted.
		findResults: [][]*computepb.Instance{nil, nil, {instance}},
		findErrs:    []error{nil, errors.New("network down"), nil},
	}
	d := newDriverWithAPI(t, api)

	if err := d.Create(); err == nil {
		t.Fatal("first Create must fail")
	}
	if fake.createCalls != 1 {
		t.Fatalf("create calls after first attempt = %d", fake.createCalls)
	}
	if len(fake.deleteCalls) != 0 {
		t.Fatalf("cleanup must fail without deleting: %v", fake.deleteCalls)
	}

	if err := d.Create(); err != nil {
		t.Fatalf("second Create must adopt: %v", err)
	}
	if fake.createCalls != 1 {
		t.Errorf("second Create created another instance: %d calls", fake.createCalls)
	}
	if len(fake.deleteCalls) != 0 {
		t.Errorf("adopt must not delete: %v", fake.deleteCalls)
	}
	if d.InstanceID != "computeinstance-1" || d.IPAddress != "203.0.113.30" {
		t.Errorf("adopted state: id=%q ip=%q", d.InstanceID, d.IPAddress)
	}

	req := fake.createReq
	if req == nil {
		t.Fatal("create request was not captured")
	}
	labels := req.GetMetadata().GetLabels()
	if labels[managedByLabel] != managedByValue || labels[rancherMachineLabel] != "node-1" {
		t.Errorf("ownership labels missing from first create: %v", labels)
	}
	if req.GetMetadata().GetName() != "node-1" {
		t.Errorf("create name = %q", req.GetMetadata().GetName())
	}
}

func TestRetryRefusesDuplicateInstances(t *testing.T) {
	first := testInstance("computeinstance-1", "node-1", "203.0.113.31")
	first.Metadata.Labels = map[string]string{managedByLabel: managedByValue}
	second := testInstance("computeinstance-2", "node-1", "203.0.113.32")
	api := &fakeAPI{found: []*computepb.Instance{first, second}}
	d := newDriverWithAPI(t, api)

	err := d.Create()
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if api.createCalls != 0 || len(api.deleteCalls) != 0 {
		t.Errorf("duplicates must not be created or deleted: create=%d delete=%v", api.createCalls, api.deleteCalls)
	}
}
