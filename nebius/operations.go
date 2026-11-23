package nebius

import (
	"context"
	"fmt"
	"time"

	"github.com/nebius/gosdk/operations"
)

const operationPollInterval = 2 * time.Second

// waitCreate waits for a create operation with a 2s poll interval, bounded by
// the create timeout.
func waitCreate(ctx context.Context, op operations.Operation, timeout time.Duration) error {
	return waitOperation(ctx, op, timeout, "create")
}

// waitDelete waits for a delete operation with a 2s poll interval, bounded by
// the delete timeout.
func waitDelete(ctx context.Context, op operations.Operation, timeout time.Duration) error {
	return waitOperation(ctx, op, timeout, "delete")
}

// waitStart waits for a start operation, bounded by the power-action timeout.
func waitStart(ctx context.Context, op operations.Operation, timeout time.Duration) error {
	return waitOperation(ctx, op, timeout, "start")
}

// waitStop waits for a stop operation, bounded by the power-action timeout.
func waitStop(ctx context.Context, op operations.Operation, timeout time.Duration) error {
	return waitOperation(ctx, op, timeout, "stop")
}

func waitOperation(ctx context.Context, op operations.Operation, timeout time.Duration, kind string) error {
	if op == nil {
		return fmt.Errorf("nebius: %s returned no operation", kind)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := op.Wait(ctx, operations.PollInterval(operationPollInterval)); err != nil {
		return fmt.Errorf("nebius: %s operation %s failed: %w", kind, op.ID(), err)
	}
	return nil
}
