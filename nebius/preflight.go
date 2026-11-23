package nebius

import (
	"context"
	"fmt"
	"time"

	commonpb "github.com/nebius/gosdk/proto/nebius/common/v1"
	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
	iampb "github.com/nebius/gosdk/proto/nebius/iam/v1"
	vpcpb "github.com/nebius/gosdk/proto/nebius/vpc/v1"
)

const preflightTimeout = 2 * time.Minute

// PreflightConfig is the read-only configuration checked before create.
type PreflightConfig struct {
	ProjectID      string
	SubnetID       string
	ImageFamily    string
	ImageID        string
	Platform       string
	Preset         string
	SecurityGroups []string
}

// Preflight verifies that the project is accessible and that the subnet,
// image, platform/preset and security groups exist. It creates nothing.
func (c *Client) Preflight(ctx context.Context, cfg PreflightConfig) error {
	_, err := withAuthRetry(ctx, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, c.preflight(ctx, cfg)
	})
	return err
}

func (c *Client) preflight(ctx context.Context, cfg PreflightConfig) error {
	if _, err := c.sdk.Services().IAM().V1().Project().Get(ctx, &iampb.GetProjectRequest{Id: cfg.ProjectID}); err != nil {
		return fmt.Errorf("project %s is not accessible: %w", cfg.ProjectID, err)
	}

	subnet, err := c.sdk.Services().VPC().V1().Subnet().Get(ctx, &vpcpb.GetSubnetRequest{Id: cfg.SubnetID})
	if err != nil {
		return fmt.Errorf("subnet %s: %w", cfg.SubnetID, err)
	}
	if parent := subnet.GetMetadata().GetParentId(); parent != "" && parent != cfg.ProjectID {
		return fmt.Errorf("subnet %s belongs to %s, not project %s", cfg.SubnetID, parent, cfg.ProjectID)
	}

	if cfg.ImageID != "" {
		if _, err := c.sdk.Services().Compute().V1().Image().Get(ctx, &computepb.GetImageRequest{Id: cfg.ImageID}); err != nil {
			return fmt.Errorf("image %s: %w", cfg.ImageID, err)
		}
	} else {
		if _, err := c.sdk.Services().Compute().V1().Image().GetLatestByFamily(ctx, &computepb.GetImageLatestByFamilyRequest{
			ImageFamily: cfg.ImageFamily,
		}); err != nil {
			return fmt.Errorf("image family %s: %w", cfg.ImageFamily, err)
		}
	}

	platform, err := c.sdk.Services().Compute().V1().Platform().GetByName(ctx, &commonpb.GetByNameRequest{
		ParentId: cfg.ProjectID,
		Name:     cfg.Platform,
	})
	if err != nil {
		return fmt.Errorf("platform %s: %w", cfg.Platform, err)
	}
	presetFound := false
	for _, preset := range platform.GetSpec().GetPresets() {
		if preset.GetName() == cfg.Preset {
			presetFound = true
			break
		}
	}
	if !presetFound {
		return fmt.Errorf("preset %s is not available on platform %s", cfg.Preset, cfg.Platform)
	}

	for _, groupID := range cfg.SecurityGroups {
		if _, err := c.sdk.Services().VPC().V1().SecurityGroup().Get(ctx, &vpcpb.GetSecurityGroupRequest{Id: groupID}); err != nil {
			return fmt.Errorf("security group %s: %w", groupID, err)
		}
	}
	return nil
}
