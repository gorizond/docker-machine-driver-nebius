package nebius

import (
	"testing"

	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
)

func TestCreateRequestPublicIPModes(t *testing.T) {
	for _, mode := range []string{"static", "dynamic", "none"} {
		opts := fullOptions()
		opts["nebius-public-ip"] = mode
		d := NewDriver("node-1", t.TempDir())
		if err := d.SetConfigFromFlags(opts); err != nil {
			t.Fatal(err)
		}

		req := d.createInstanceRequest("node-1", "#cloud-config\n")
		nic := req.GetSpec().GetNetworkInterfaces()[0]
		if nic.GetIpAddress() == nil {
			t.Errorf("%s: private IP request is missing", mode)
		}
		switch mode {
		case "static":
			if nic.GetPublicIpAddress() == nil || !nic.GetPublicIpAddress().GetStatic() {
				t.Errorf("static mode did not request a static public IP")
			}
		case "dynamic":
			if nic.GetPublicIpAddress() == nil || nic.GetPublicIpAddress().GetStatic() {
				t.Errorf("dynamic mode did not request a dynamic public IP")
			}
		case "none":
			if nic.GetPublicIpAddress() != nil {
				t.Errorf("none mode must not request a public IP")
			}
		}
	}
}

func TestCreateRequestSpec(t *testing.T) {
	d := newTestDriver(t, &fakeAPI{})
	req := d.createInstanceRequest("node-1", "#cloud-config\n")
	spec := req.GetSpec()

	if spec.GetResources().GetPlatform() != "cpu-d3" || spec.GetResources().GetPreset() != "4vcpu-16gb" {
		t.Errorf("platform/preset not mapped: %v", spec.GetResources())
	}
	disk := spec.GetBootDisk()
	if disk.GetAttachMode() != computepb.AttachedDiskSpec_READ_WRITE {
		t.Errorf("attach mode = %v", disk.GetAttachMode())
	}
	if disk.GetManagedDisk().GetSpec().GetSizeGibibytes() != 80 {
		t.Errorf("disk size = %d", disk.GetManagedDisk().GetSpec().GetSizeGibibytes())
	}
	if disk.GetManagedDisk().GetSpec().GetType() != computepb.DiskSpec_NETWORK_HDD {
		t.Errorf("disk type = %v", disk.GetManagedDisk().GetSpec().GetType())
	}
	if got := disk.GetManagedDisk().GetSpec().GetSourceImageFamily().GetImageFamily(); got != "ubuntu24.04-driverless" {
		t.Errorf("image family = %q", got)
	}
	if spec.GetPreemptible().GetOnPreemption() != computepb.PreemptibleSpec_STOP {
		t.Errorf("preemptible policy = %v", spec.GetPreemptible())
	}
	if spec.GetFollowsSpotPrice() == nil {
		t.Error("preemptible instance must follow the spot price")
	}
	if spec.GetHostname() != "node-1" {
		t.Errorf("hostname = %q", spec.GetHostname())
	}
}

func TestOwnershipLabels(t *testing.T) {
	d := newTestDriver(t, &fakeAPI{})
	d.MachineName = "Node_1"
	t.Setenv("RANCHER_CLUSTER_ID", "Cluster_ABC")

	labels := d.ownershipLabels()
	if labels[managedByLabel] != managedByValue {
		t.Errorf("managed-by = %q", labels[managedByLabel])
	}
	if labels[rancherMachineLabel] != "node-1" {
		t.Errorf("rancher-machine = %q", labels[rancherMachineLabel])
	}
	if labels[rancherClusterLabel] != "cluster-abc" {
		t.Errorf("rancher-cluster = %q", labels[rancherClusterLabel])
	}
	if labels["team"] != "test" {
		t.Errorf("user label lost: %v", labels)
	}
}
