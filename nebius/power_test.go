package nebius

import (
	"strings"
	"testing"

	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
)

func powerDriver(t *testing.T, state computepb.InstanceStatus_InstanceState) (*Driver, *fakeAPI) {
	t.Helper()
	instance := testInstance("computeinstance-1", "node-1", "203.0.113.10")
	instance.Status.State = state
	api := &fakeAPI{instances: map[string]*computepb.Instance{"computeinstance-1": instance}}
	d := newDriverWithAPI(t, api)
	d.InstanceID = "computeinstance-1"
	return d, api
}

func TestStartFromStopped(t *testing.T) {
	d, api := powerDriver(t, computepb.InstanceStatus_STOPPED)
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(api.startCalls) != 1 || api.startCalls[0] != "computeinstance-1" {
		t.Errorf("start calls = %v", api.startCalls)
	}
	if len(api.stopCalls) != 0 {
		t.Errorf("unexpected stop calls: %v", api.stopCalls)
	}
	if got := api.instances["computeinstance-1"].GetStatus().GetState(); got != computepb.InstanceStatus_RUNNING {
		t.Errorf("state = %s, want RUNNING", got)
	}
}

func TestStartAlreadyRunningIsNoop(t *testing.T) {
	d, api := powerDriver(t, computepb.InstanceStatus_RUNNING)
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(api.startCalls) != 0 {
		t.Errorf("start must be skipped for a running instance: %v", api.startCalls)
	}
}

func TestStopFromRunning(t *testing.T) {
	d, api := powerDriver(t, computepb.InstanceStatus_RUNNING)
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(api.stopCalls) != 1 || api.stopCalls[0] != "computeinstance-1" {
		t.Errorf("stop calls = %v", api.stopCalls)
	}
	if got := api.instances["computeinstance-1"].GetStatus().GetState(); got != computepb.InstanceStatus_STOPPED {
		t.Errorf("state = %s, want STOPPED", got)
	}
}

func TestStopAlreadyStoppedIsNoop(t *testing.T) {
	d, api := powerDriver(t, computepb.InstanceStatus_STOPPED)
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(api.stopCalls) != 0 {
		t.Errorf("stop must be skipped for a stopped instance: %v", api.stopCalls)
	}
}

func TestKillDelegatesToStop(t *testing.T) {
	d, api := powerDriver(t, computepb.InstanceStatus_RUNNING)
	if err := d.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if len(api.stopCalls) != 1 {
		t.Errorf("kill must call stop once: %v", api.stopCalls)
	}
}

func TestRestartFromRunning(t *testing.T) {
	d, api := powerDriver(t, computepb.InstanceStatus_RUNNING)
	if err := d.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if len(api.stopCalls) != 1 || len(api.startCalls) != 1 {
		t.Errorf("restart calls: stop=%v start=%v", api.stopCalls, api.startCalls)
	}
	if got := api.instances["computeinstance-1"].GetStatus().GetState(); got != computepb.InstanceStatus_RUNNING {
		t.Errorf("state = %s, want RUNNING", got)
	}
}

func TestRestartFromStoppedSkipsStop(t *testing.T) {
	d, api := powerDriver(t, computepb.InstanceStatus_STOPPED)
	if err := d.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if len(api.stopCalls) != 0 {
		t.Errorf("restart of a stopped instance must skip stop: %v", api.stopCalls)
	}
	if len(api.startCalls) != 1 {
		t.Errorf("start calls = %v", api.startCalls)
	}
}

func TestPowerActionWithoutInstance(t *testing.T) {
	d := newTestDriver(t, &fakeAPI{})
	if err := d.Start(); err == nil || !strings.Contains(err.Error(), "no instance") {
		t.Fatalf("expected no-instance error, got %v", err)
	}
}
