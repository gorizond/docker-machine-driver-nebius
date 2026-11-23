package nebius

import (
	"strings"
	"testing"

	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
)

func TestRemoveByInstanceID(t *testing.T) {
	api := &fakeAPI{}
	d := newTestDriver(t, api)
	d.InstanceID = "computeinstance-1"
	d.IPAddress = "203.0.113.10"

	if err := d.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(api.deleteCalls) != 1 || api.deleteCalls[0] != "computeinstance-1" {
		t.Fatalf("delete calls = %v", api.deleteCalls)
	}
	if d.InstanceID != "" || d.IPAddress != "" {
		t.Errorf("state not cleared: id=%q ip=%q", d.InstanceID, d.IPAddress)
	}
}

func TestRemoveWhenAbsentIsIdempotent(t *testing.T) {
	api := &fakeAPI{}
	d := newTestDriver(t, api)

	if err := d.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(api.deleteCalls) != 0 {
		t.Errorf("delete calls = %v, want none", api.deleteCalls)
	}
}

func TestRemoveAdoptsLookupByLabel(t *testing.T) {
	existing := testInstance("computeinstance-adopt", "node-1", "203.0.113.11")
	existing.Metadata.Labels = map[string]string{managedByLabel: managedByValue}
	api := &fakeAPI{found: []*computepb.Instance{existing}}
	d := newTestDriver(t, api)

	if err := d.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(api.deleteCalls) != 1 || api.deleteCalls[0] != "computeinstance-adopt" {
		t.Fatalf("delete calls = %v", api.deleteCalls)
	}
}

func TestRemoveRefusesForeignKey(t *testing.T) {
	existing := testInstance("computeinstance-foreign", "node-1", "203.0.113.12")
	existing.Metadata.Labels = map[string]string{managedByLabel: "other-driver"}
	api := &fakeAPI{found: []*computepb.Instance{existing}}
	d := newTestDriver(t, api)

	err := d.Remove()
	if err == nil || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("expected foreign-instance error, got %v", err)
	}
	if len(api.deleteCalls) != 0 {
		t.Errorf("foreign instance must not be deleted: %v", api.deleteCalls)
	}
}

func TestGetIPModes(t *testing.T) {
	none := newTestDriver(t, &fakeAPI{})
	none.PublicIP = "none"
	if _, err := none.GetIP(); err == nil {
		t.Fatal("expected error for public-ip=none")
	}

	cached := newTestDriver(t, &fakeAPI{})
	cached.IPAddress = "203.0.113.20"
	if ip, err := cached.GetIP(); err != nil || ip != "203.0.113.20" {
		t.Errorf("cached GetIP = %q, %v", ip, err)
	}

	fetched := newTestDriver(t, &fakeAPI{instances: map[string]*computepb.Instance{
		"computeinstance-1": testInstance("computeinstance-1", "node-1", "203.0.113.21"),
	}})
	fetched.InstanceID = "computeinstance-1"
	if ip, err := fetched.GetIP(); err != nil || ip != "203.0.113.21" {
		t.Errorf("fetched GetIP = %q, %v", ip, err)
	}
	if fetched.IPAddress != "203.0.113.21" {
		t.Errorf("GetIP did not cache the address: %q", fetched.IPAddress)
	}

	missing := newTestDriver(t, &fakeAPI{instances: map[string]*computepb.Instance{
		"computeinstance-2": testInstance("computeinstance-2", "node-1", ""),
	}})
	missing.InstanceID = "computeinstance-2"
	if _, err := missing.GetIP(); err == nil {
		t.Fatal("expected error for instance without public IPv4")
	}
}
