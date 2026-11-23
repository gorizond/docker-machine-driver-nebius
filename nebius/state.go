package nebius

import (
	"github.com/docker/machine/libmachine/state"
	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
)

// MapInstanceState maps a Nebius instance state to a docker-machine state.
func MapInstanceState(s computepb.InstanceStatus_InstanceState) state.State {
	switch s {
	case computepb.InstanceStatus_CREATING,
		computepb.InstanceStatus_UPDATING,
		computepb.InstanceStatus_STARTING:
		return state.Starting
	case computepb.InstanceStatus_RUNNING:
		return state.Running
	case computepb.InstanceStatus_STOPPING,
		computepb.InstanceStatus_DELETING:
		return state.Stopping
	case computepb.InstanceStatus_STOPPED:
		return state.Stopped
	case computepb.InstanceStatus_ERROR:
		return state.Error
	default:
		// UNSPECIFIED and anything unknown.
		return state.None
	}
}

// MachineState maps an instance to a machine state; nil means NotFound.
func MachineState(inst *computepb.Instance) state.State {
	if inst == nil || inst.GetStatus() == nil {
		return state.None
	}
	return MapInstanceState(inst.GetStatus().GetState())
}
