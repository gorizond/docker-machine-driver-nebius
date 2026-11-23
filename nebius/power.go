package nebius

import (
	"context"
	"errors"
	"fmt"
	"time"

	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
)

// powerTimeout bounds one start/stop action. The delete timeout is the only
// operator-tunable timeout in the contract, so power actions reuse it.
func (d *Driver) powerTimeout() time.Duration {
	if d.DeleteTimeout > 0 {
		return d.DeleteTimeout
	}
	return 5 * time.Minute
}

// Start starts a stopped machine and waits for RUNNING. Already-running
// machines are left untouched (idempotent).
func (d *Driver) Start() error {
	return d.runPowerAction("start", d.powerTimeout(), func(ctx context.Context, api nebiusAPI, id string) error {
		instance, err := api.GetInstance(ctx, id)
		if err != nil {
			return err
		}
		if instance.GetStatus().GetState() == computepb.InstanceStatus_RUNNING {
			return nil
		}
		return api.StartInstance(ctx, id, d.powerTimeout())
	})
}

// Stop stops a machine and waits for STOPPED. Already-stopped machines are
// left untouched (idempotent).
func (d *Driver) Stop() error {
	return d.runPowerAction("stop", d.powerTimeout(), func(ctx context.Context, api nebiusAPI, id string) error {
		instance, err := api.GetInstance(ctx, id)
		if err != nil {
			return err
		}
		if instance.GetStatus().GetState() == computepb.InstanceStatus_STOPPED {
			return nil
		}
		return api.StopInstance(ctx, id, d.powerTimeout())
	})
}

// Restart performs a bounded stop -> wait STOPPED -> start -> wait RUNNING
// sequence. An already-stopped machine skips the stop step.
func (d *Driver) Restart() error {
	return d.runPowerAction("restart", 2*d.powerTimeout(), func(ctx context.Context, api nebiusAPI, id string) error {
		instance, err := api.GetInstance(ctx, id)
		if err != nil {
			return err
		}
		if instance.GetStatus().GetState() != computepb.InstanceStatus_STOPPED {
			if err := api.StopInstance(ctx, id, d.powerTimeout()); err != nil {
				return err
			}
		}
		return api.StartInstance(ctx, id, d.powerTimeout())
	})
}

// Kill stops the machine. The public Nebius API has no hard-kill operation, so
// Kill is Stop (documented assumption, see FR-012 and research R9).
func (d *Driver) Kill() error { return d.Stop() }

func (d *Driver) runPowerAction(operation string, timeout time.Duration, fn func(context.Context, nebiusAPI, string) error) (err error) {
	start := time.Now()
	defer func() { LogOperation(context.Background(), operation, d.InstanceID, start, err) }()

	if d.InstanceID == "" {
		return errors.New("nebius: machine has no instance yet")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	api, err := d.apiClient(ctx)
	if err != nil {
		return err
	}
	if err := fn(ctx, api, d.InstanceID); err != nil {
		return fmt.Errorf("nebius: %s instance %s: %s", operation, d.InstanceID, MapError(err).Error())
	}
	return nil
}
