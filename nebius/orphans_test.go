package nebius

import (
	"testing"

	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
)

func TestFilterManagedInstances(t *testing.T) {
	plain := testInstance("computeinstance-plain", "plain", "")
	foreign := testInstance("computeinstance-foreign", "foreign", "")
	foreign.Metadata.Labels = map[string]string{managedByLabel: "other-driver"}
	owned := testInstance("computeinstance-owned", "owned", "")
	owned.Metadata.Labels = map[string]string{managedByLabel: managedByValue}
	clusterOwned := testInstance("computeinstance-cluster", "cluster", "")
	clusterOwned.Metadata.Labels = map[string]string{
		managedByLabel:      managedByValue,
		rancherClusterLabel: "cluster-a",
	}

	all := []*computepb.Instance{plain, foreign, owned, clusterOwned}

	got := FilterManagedInstances(all, "")
	if len(got) != 2 || got[0].GetMetadata().GetId() != "computeinstance-owned" ||
		got[1].GetMetadata().GetId() != "computeinstance-cluster" {
		t.Errorf("filter without cluster = %v", instanceIDs(got))
	}

	got = FilterManagedInstances(all, "Cluster_A")
	if len(got) != 1 || got[0].GetMetadata().GetId() != "computeinstance-cluster" {
		t.Errorf("filter with normalized cluster = %v", instanceIDs(got))
	}

	if got := FilterManagedInstances(all, "cluster-b"); len(got) != 0 {
		t.Errorf("filter for another cluster = %v", instanceIDs(got))
	}
	if got := FilterManagedInstances(nil, ""); len(got) != 0 {
		t.Errorf("nil input = %v", instanceIDs(got))
	}
}

func instanceIDs(instances []*computepb.Instance) []string {
	ids := make([]string, 0, len(instances))
	for _, instance := range instances {
		ids = append(ids, instance.GetMetadata().GetId())
	}
	return ids
}
