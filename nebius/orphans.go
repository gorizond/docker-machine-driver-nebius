package nebius

import (
	"strings"

	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
)

// FilterManagedInstances returns instances owned by this driver. When
// clusterID is non-empty, only instances labelled for that Rancher cluster are
// returned; the value is normalized the same way ownership labels are written.
func FilterManagedInstances(instances []*computepb.Instance, clusterID string) []*computepb.Instance {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID != "" {
		clusterID = NormalizeName(clusterID)
	}

	owned := make([]*computepb.Instance, 0, len(instances))
	for _, instance := range instances {
		labels := instance.GetMetadata().GetLabels()
		if labels[managedByLabel] != managedByValue {
			continue
		}
		if clusterID != "" && labels[rancherClusterLabel] != clusterID {
			continue
		}
		owned = append(owned, instance)
	}
	return owned
}
