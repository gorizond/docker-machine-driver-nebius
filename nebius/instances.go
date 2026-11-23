package nebius

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/nebius/gosdk/conn"
	"github.com/nebius/gosdk/operations"
	commonpb "github.com/nebius/gosdk/proto/nebius/common/v1"
	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
)

const instancePageSize = 100

// GetInstance returns an instance by ID. Read-only.
func (c *Client) GetInstance(ctx context.Context, id string) (*computepb.Instance, error) {
	return withAuthRetry(ctx, func(ctx context.Context) (*computepb.Instance, error) {
		return c.sdk.Services().Compute().V1().Instance().Get(ctx, &computepb.GetInstanceRequest{Id: id})
	})
}

// GetInstanceByName returns an instance by parent ID and name. Read-only.
func (c *Client) GetInstanceByName(ctx context.Context, parentID, name string) (*computepb.Instance, error) {
	if parentID == "" {
		parentID = c.projectID
	}
	return withAuthRetry(ctx, func(ctx context.Context) (*computepb.Instance, error) {
		return c.sdk.Services().Compute().V1().Instance().GetByName(ctx, &commonpb.GetByNameRequest{
			ParentId: parentID,
			Name:     name,
		})
	})
}

// ListInstances returns every instance in the project, following pagination.
// Read-only.
func (c *Client) ListInstances(ctx context.Context, parentID string) ([]*computepb.Instance, error) {
	if parentID == "" {
		parentID = c.projectID
	}
	var all []*computepb.Instance
	pageToken := ""
	for {
		resp, err := withAuthRetry(ctx, func(ctx context.Context) (*computepb.ListInstancesResponse, error) {
			return c.sdk.Services().Compute().V1().Instance().List(ctx, &computepb.ListInstancesRequest{
				ParentId:  parentID,
				PageSize:  instancePageSize,
				PageToken: pageToken,
			})
		})
		if err != nil {
			return nil, err
		}
		all = append(all, resp.GetItems()...)
		pageToken = resp.GetNextPageToken()
		if pageToken == "" {
			return all, nil
		}
	}
}

// FindInstancesByName returns all instances with the given name. GetByName
// returns a single resource and cannot prove that no duplicate exists, so
// adopt (FR-015) filters a paginated list instead.
func (c *Client) FindInstancesByName(ctx context.Context, parentID, name string) ([]*computepb.Instance, error) {
	all, err := c.ListInstances(ctx, parentID)
	if err != nil {
		return nil, err
	}
	matches := make([]*computepb.Instance, 0, 1)
	for _, instance := range all {
		if instance.GetMetadata().GetName() == name {
			matches = append(matches, instance)
		}
	}
	return matches, nil
}

// CreateInstance creates an instance with a stable idempotency key and waits
// for its create operation and the RUNNING state. The context bounds the whole
// call; timeout additionally bounds each helper.
func (c *Client) CreateInstance(ctx context.Context, req *computepb.CreateInstanceRequest, key string, timeout time.Duration) (*computepb.Instance, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	op, err := withAuthRetry(ctx, func(ctx context.Context) (operations.Operation, error) {
		return c.sdk.Services().Compute().V1().Instance().Create(ctx, req, conn.WithIdempotencyKey(key))
	})
	if err != nil {
		return nil, err
	}
	if err := waitCreate(ctx, op, timeout); err != nil {
		return nil, err
	}

	id := strings.TrimSpace(op.ResourceID())
	if id == "" {
		parentID := req.GetMetadata().GetParentId()
		if parentID == "" {
			parentID = c.projectID
		}
		instance, err := c.GetInstanceByName(ctx, parentID, req.GetMetadata().GetName())
		if err != nil {
			return nil, err
		}
		id = instance.GetMetadata().GetId()
	}
	return c.waitRunning(ctx, id)
}

func (c *Client) waitRunning(ctx context.Context, id string) (*computepb.Instance, error) {
	return c.waitInstanceState(ctx, id, computepb.InstanceStatus_RUNNING)
}

// StartInstance starts a stopped instance, waits for the operation and for the
// RUNNING state. The caller-bounded context and timeout both cap the call.
func (c *Client) StartInstance(ctx context.Context, id string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	op, err := withAuthRetry(ctx, func(ctx context.Context) (operations.Operation, error) {
		return c.sdk.Services().Compute().V1().Instance().Start(ctx, &computepb.StartInstanceRequest{Id: id})
	})
	if err != nil {
		return err
	}
	if err := waitStart(ctx, op, timeout); err != nil {
		return err
	}
	_, err = c.waitRunning(ctx, id)
	return err
}

// StopInstance stops an instance, waits for the operation and for the STOPPED
// state. The caller-bounded context and timeout both cap the call.
func (c *Client) StopInstance(ctx context.Context, id string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	op, err := withAuthRetry(ctx, func(ctx context.Context) (operations.Operation, error) {
		return c.sdk.Services().Compute().V1().Instance().Stop(ctx, &computepb.StopInstanceRequest{Id: id})
	})
	if err != nil {
		return err
	}
	if err := waitStop(ctx, op, timeout); err != nil {
		return err
	}
	_, err = c.waitInstanceState(ctx, id, computepb.InstanceStatus_STOPPED)
	return err
}

func (c *Client) waitInstanceState(ctx context.Context, id string, desired computepb.InstanceStatus_InstanceState) (*computepb.Instance, error) {
	for {
		instance, err := c.GetInstance(ctx, id)
		if err != nil {
			return nil, err
		}
		switch state := instance.GetStatus().GetState(); state {
		case desired:
			return instance, nil
		case computepb.InstanceStatus_ERROR:
			return nil, fmt.Errorf("nebius: instance %s entered ERROR state", id)
		case computepb.InstanceStatus_DELETING:
			return nil, fmt.Errorf("nebius: instance %s is being deleted", id)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("nebius: instance %s did not reach %s: %w", id, desired, ctx.Err())
		case <-time.After(operationPollInterval):
		}
	}
}

// DeleteInstance deletes an instance and waits for the delete operation.
// NotFound is success, which keeps Remove idempotent.
func (c *Client) DeleteInstance(ctx context.Context, id string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	op, err := withAuthRetry(ctx, func(ctx context.Context) (operations.Operation, error) {
		return c.sdk.Services().Compute().V1().Instance().Delete(ctx, &computepb.DeleteInstanceRequest{Id: id})
	})
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return waitDelete(ctx, op, timeout)
}

// PublicIPv4 returns the first public IPv4 address of an instance, or "".
func PublicIPv4(instance *computepb.Instance) string {
	for _, nic := range instance.GetStatus().GetNetworkInterfaces() {
		address := nic.GetPublicIpAddress().GetAddress()
		if address == "" {
			continue
		}
		// The API reports the public address in CIDR form (e.g. "1.2.3.4/32").
		if ip, _, err := net.ParseCIDR(address); err == nil {
			return ip.String()
		}
		return address
	}
	return ""
}
