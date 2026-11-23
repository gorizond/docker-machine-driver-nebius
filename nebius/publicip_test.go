package nebius

import (
	"testing"

	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
)

func TestPublicIPv4StripsCIDR(t *testing.T) {
	instance := &computepb.Instance{
		Status: &computepb.InstanceStatus{
			NetworkInterfaces: []*computepb.NetworkInterfaceStatus{
				{PublicIpAddress: &computepb.PublicIPAddressStatus{Address: "66.201.4.10/32"}},
			},
		},
	}
	if got := PublicIPv4(instance); got != "66.201.4.10" {
		t.Fatalf("PublicIPv4() = %q, want %q", got, "66.201.4.10")
	}
}

func TestPublicIPv4PlainAddress(t *testing.T) {
	instance := &computepb.Instance{
		Status: &computepb.InstanceStatus{
			NetworkInterfaces: []*computepb.NetworkInterfaceStatus{
				{PublicIpAddress: &computepb.PublicIPAddressStatus{Address: "66.201.4.11"}},
			},
		},
	}
	if got := PublicIPv4(instance); got != "66.201.4.11" {
		t.Fatalf("PublicIPv4() = %q, want %q", got, "66.201.4.11")
	}
}

func TestPublicIPv4None(t *testing.T) {
	if got := PublicIPv4(&computepb.Instance{}); got != "" {
		t.Fatalf("PublicIPv4() = %q, want empty", got)
	}
}
