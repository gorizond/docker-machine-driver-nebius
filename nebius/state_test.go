package nebius

import (
	"testing"

	"github.com/docker/machine/libmachine/state"
	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
)

func TestMapInstanceState(t *testing.T) {
	tests := []struct {
		in   computepb.InstanceStatus_InstanceState
		want state.State
	}{
		{computepb.InstanceStatus_CREATING, state.Starting},
		{computepb.InstanceStatus_UPDATING, state.Starting},
		{computepb.InstanceStatus_STARTING, state.Starting},
		{computepb.InstanceStatus_RUNNING, state.Running},
		{computepb.InstanceStatus_STOPPING, state.Stopping},
		{computepb.InstanceStatus_DELETING, state.Stopping},
		{computepb.InstanceStatus_STOPPED, state.Stopped},
		{computepb.InstanceStatus_ERROR, state.Error},
		{computepb.InstanceStatus_UNSPECIFIED, state.None},
	}

	for _, tt := range tests {
		if got := MapInstanceState(tt.in); got != tt.want {
			t.Errorf("MapInstanceState(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestMachineState(t *testing.T) {
	if got := MachineState(nil); got != state.None {
		t.Errorf("MachineState(nil) = %s, want None", got)
	}
	inst := &computepb.Instance{
		Status: &computepb.InstanceStatus{State: computepb.InstanceStatus_RUNNING},
	}
	if got := MachineState(inst); got != state.Running {
		t.Errorf("MachineState(running) = %s, want Running", got)
	}
	inst.Status = nil
	if got := MachineState(inst); got != state.None {
		t.Errorf("MachineState(no status) = %s, want None", got)
	}
}
