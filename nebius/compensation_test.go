package nebius

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// scriptedAPI returns a scripted sequence of FindInstancesByName results while
// delegating everything else to fakeAPI.
type scriptedAPI struct {
	*fakeAPI
	findResults [][]*computepb.Instance
	findErrs    []error
	findCalls   int
}

func (s *scriptedAPI) FindInstancesByName(context.Context, string, string) ([]*computepb.Instance, error) {
	index := s.findCalls
	s.findCalls++
	var err error
	if index < len(s.findErrs) {
		err = s.findErrs[index]
	}
	if index < len(s.findResults) {
		return s.findResults[index], err
	}
	return nil, err
}

func TestCompensatingDeleteFailureKeepsInstanceID(t *testing.T) {
	instance := testInstance("computeinstance-1", "node-1", "203.0.113.10")
	api := &fakeAPI{
		created:   instance,
		instances: map[string]*computepb.Instance{"computeinstance-1": instance},
		deleteErr: status.Error(codes.Unavailable, "api down"),
	}
	d := newDriverWithAPI(t, api)
	d.waitSSH = func(context.Context, string, string, ssh.Signer, time.Duration) error {
		return errors.New("ssh timeout")
	}

	err := d.Create()
	if err == nil {
		t.Fatal("expected create failure")
	}
	if !strings.Contains(err.Error(), "computeinstance-1") {
		t.Errorf("error does not carry instance id: %v", err)
	}
	if !strings.Contains(err.Error(), "compensating delete") {
		t.Errorf("error does not mention compensation: %v", err)
	}
	if len(api.deleteCalls) != 1 {
		t.Errorf("delete calls = %v, want exactly one", api.deleteCalls)
	}
	if d.InstanceID != "computeinstance-1" {
		t.Errorf("instance id cleared after failed compensation: %q", d.InstanceID)
	}
}

func TestCleanupFailureKeepsInstanceID(t *testing.T) {
	instance := testInstance("computeinstance-1", "node-1", "203.0.113.10")
	instance.Metadata.Labels = map[string]string{managedByLabel: managedByValue}
	fake := &fakeAPI{
		createErr: status.Error(codes.Unavailable, "connection reset"),
		instances: map[string]*computepb.Instance{"computeinstance-1": instance},
		deleteErr: status.Error(codes.Unavailable, "api down"),
	}
	api := &scriptedAPI{
		fakeAPI:     fake,
		findResults: [][]*computepb.Instance{nil, {instance}},
	}
	d := newDriverWithAPI(t, api)

	err := d.Create()
	if err == nil {
		t.Fatal("expected create failure")
	}
	if !strings.Contains(err.Error(), "computeinstance-1") {
		t.Errorf("error does not carry instance id: %v", err)
	}
	if fake.createCalls != 1 || len(fake.deleteCalls) != 1 {
		t.Errorf("calls: create=%d delete=%v", fake.createCalls, fake.deleteCalls)
	}
	if d.InstanceID != "computeinstance-1" {
		t.Errorf("instance id not kept for manual cleanup: %q", d.InstanceID)
	}
}

func TestAdoptedInstanceNotDeletedOnSSHFailure(t *testing.T) {
	instance := testInstance("computeinstance-adopt", "node-1", "203.0.113.11")
	instance.Metadata.Labels = map[string]string{managedByLabel: managedByValue}
	api := &fakeAPI{found: []*computepb.Instance{instance}}
	d := newDriverWithAPI(t, api)
	d.waitSSH = func(context.Context, string, string, ssh.Signer, time.Duration) error {
		return errors.New("ssh timeout")
	}

	if err := d.Create(); err == nil {
		t.Fatal("expected SSH failure")
	}
	if len(api.deleteCalls) != 0 {
		t.Errorf("adopted instance must not be deleted: %v", api.deleteCalls)
	}
	if d.InstanceID != "computeinstance-adopt" {
		t.Errorf("adopted instance id lost: %q", d.InstanceID)
	}
}
