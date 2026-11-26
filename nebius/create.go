package nebius

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	commonpb "github.com/nebius/gosdk/proto/nebius/common/v1"
	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
	"golang.org/x/crypto/ssh"
)

const (
	managedByLabel      = "managed-by"
	managedByValue      = "docker-machine-driver-nebius"
	rancherMachineLabel = "rancher-machine"
	rancherClusterLabel = "rancher-cluster"
)

// Create creates one instance for this machine, adopting an existing instance
// with the same deterministic name. Any failure after this call created the
// instance triggers a compensating delete.
func (d *Driver) Create() (err error) {
	start := time.Now()
	logID := ""
	defer func() { LogOperation(context.Background(), "create", logID, start, err) }()

	if err := d.PreCreateCheck(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), d.CreateTimeout)
	defer cancel()

	api, err := d.apiClient(ctx)
	if err != nil {
		return err
	}

	signer, err := d.ensureSSHKey()
	if err != nil {
		return err
	}

	name := NormalizeName(d.MachineName)
	existing, err := api.FindInstancesByName(ctx, d.ProjectID, name)
	if err != nil {
		return fmt.Errorf("nebius: lookup instance %q: %s", name, MapError(err).Error())
	}

	created := false
	var instance *computepb.Instance
	switch len(existing) {
	case 0:
		instance, err = d.createInstance(ctx, api, name, signer)
		if err != nil {
			return err
		}
		created = true
	case 1:
		instance = existing[0]
		if instance.GetMetadata().GetLabels()[managedByLabel] != managedByValue {
			return fmt.Errorf("nebius: instance %q exists but is not managed by %s", name, managedByValue)
		}
	default:
		return fmt.Errorf("nebius: found %d instances named %q; refusing to choose one", len(existing), name)
	}

	d.InstanceID = instance.GetMetadata().GetId()
	if d.InstanceID == "" {
		err = fmt.Errorf("nebius: instance %q has no ID", name)
		if created {
			err = d.cleanupFailedCreate(api, name, err)
		}
		return err
	}
	logID = d.InstanceID

	if instanceState := instance.GetStatus().GetState(); instanceState != computepb.InstanceStatus_RUNNING {
		err = fmt.Errorf("nebius: instance %s is %s, expected RUNNING", d.InstanceID, instanceState)
		if created {
			err = d.compensate(api, err)
		}
		return err
	}

	ip := PublicIPv4(instance)
	if ip == "" && d.PublicIP == "none" && d.UserDataFile != "" {
		ip = PrivateIPv4(instance)
	}
	if ip == "" {
		if d.PublicIP == "none" {
			err = fmt.Errorf("%w (public-ip=none)", errNoPrivateIP)
		} else {
			err = fmt.Errorf("%w (public-ip=%s)", errNoPublicIP, d.PublicIP)
		}
		if created {
			err = d.compensate(api, err)
		}
		return err
	}
	d.IPAddress = ip

	// With userdata, Rancher runs its install script and there may be no
	// public IP; the driver's own SSH probe is intentionally skipped.
	if d.UserDataFile == "" {
		waitSSH := d.waitSSH
		if waitSSH == nil {
			waitSSH = waitForSSH
		}
		address := net.JoinHostPort(ip, strconv.Itoa(d.SSHPort))
		if err = waitSSH(ctx, address, d.SSHUser, signer, d.CreateTimeout); err != nil {
			if created {
				err = d.compensate(api, err)
			}
			return err
		}
	}
	return nil
}

// renderCloudInit renders the driver's minimal cloud-config, or merges the
// driver SSH user into the cloud-config supplied by Rancher.
func (d *Driver) renderCloudInit(authorizedKey string) (string, error) {
	cfg := CloudInit{User: d.SSHUser, SSHAuthorizedKeys: []string{authorizedKey}}
	if d.UserDataFile == "" {
		return RenderCloudInit(cfg)
	}
	raw, err := os.ReadFile(d.UserDataFile)
	if err != nil {
		return "", fmt.Errorf("nebius: read userdata %s: %w", d.UserDataFile, err)
	}
	return MergeUserData(string(raw), cfg)
}

func (d *Driver) createInstance(ctx context.Context, api nebiusAPI, name string, signer ssh.Signer) (*computepb.Instance, error) {
	authorizedKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	cloudInit, err := d.renderCloudInit(authorizedKey)
	if err != nil {
		return nil, err
	}

	req := d.createInstanceRequest(name, cloudInit)
	instance, err := api.CreateInstance(ctx, req, IdempotencyKey(d.ProjectID, d.MachineName), d.CreateTimeout)
	if err != nil {
		return nil, d.cleanupFailedCreate(api, name, err)
	}
	return instance, nil
}

// createInstanceRequest builds the gosdk create request. recoveryPolicy is
// intentionally not set (API default); preemptible VMs follow the spot price
// with onPreemption=STOP.
func (d *Driver) createInstanceRequest(name, cloudInit string) *computepb.CreateInstanceRequest {
	nic := &computepb.NetworkInterfaceSpec{
		Name:      "eth0",
		SubnetId:  d.SubnetID,
		IpAddress: &computepb.IPAddress{},
	}
	switch d.PublicIP {
	case "static":
		nic.PublicIpAddress = &computepb.PublicIPAddress{Static: true}
	case "dynamic":
		nic.PublicIpAddress = &computepb.PublicIPAddress{}
	}
	for _, groupID := range d.SecurityGroups {
		nic.SecurityGroups = append(nic.SecurityGroups, &computepb.SecurityGroup{Id: groupID})
	}

	diskType, _ := parseDiskType(d.DiskType)
	diskSpec := &computepb.DiskSpec{
		Type: diskType,
		Size: &computepb.DiskSpec_SizeGibibytes{SizeGibibytes: int64(d.DiskSizeGiB)},
	}
	if d.ImageID != "" {
		diskSpec.Source = &computepb.DiskSpec_SourceImageId{SourceImageId: d.ImageID}
	} else {
		diskSpec.Source = &computepb.DiskSpec_SourceImageFamily{
			SourceImageFamily: &computepb.SourceImageFamily{ImageFamily: d.ImageFamily},
		}
	}

	spec := &computepb.InstanceSpec{
		Resources: &computepb.ResourcesSpec{
			Platform: d.Platform,
			Size:     &computepb.ResourcesSpec_Preset{Preset: d.Preset},
		},
		BootDisk: &computepb.AttachedDiskSpec{
			AttachMode: computepb.AttachedDiskSpec_READ_WRITE,
			Type: &computepb.AttachedDiskSpec_ManagedDisk{ManagedDisk: &computepb.ManagedDisk{
				Name: NormalizeName(name + "-boot"),
				Spec: diskSpec,
			}},
		},
		NetworkInterfaces: []*computepb.NetworkInterfaceSpec{nic},
		CloudInitUserData: cloudInit,
		Hostname:          name,
	}
	if d.Preemptible {
		spec.Preemptible = &computepb.PreemptibleSpec{OnPreemption: computepb.PreemptibleSpec_STOP}
		spec.PricingModel = &computepb.InstanceSpec_FollowsSpotPrice{
			FollowsSpotPrice: &computepb.FollowsSpotPriceSpec{},
		}
	}

	return &computepb.CreateInstanceRequest{
		Metadata: &commonpb.ResourceMetadata{
			ParentId: d.ProjectID,
			Name:     name,
			Labels:   d.ownershipLabels(),
		},
		Spec: spec,
	}
}

func (d *Driver) ownershipLabels() map[string]string {
	labels := make(map[string]string, len(d.Labels)+3)
	for key, value := range d.Labels {
		labels[key] = value
	}
	labels[managedByLabel] = managedByValue
	labels[rancherMachineLabel] = NormalizeName(d.MachineName)
	if clusterID := strings.TrimSpace(os.Getenv("RANCHER_CLUSTER_ID")); clusterID != "" {
		labels[rancherClusterLabel] = NormalizeName(clusterID)
	}
	return labels
}

// compensate deletes an instance created by this call and clears cached state.
// A failed delete keeps InstanceID so Remove can retry; the error carries the id
// for manual cleanup.
func (d *Driver) compensate(api nebiusAPI, cause error) error {
	id := d.InstanceID
	if id == "" {
		return cause
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.DeleteTimeout)
	defer cancel()
	if err := api.DeleteInstance(ctx, id, d.DeleteTimeout); err != nil {
		return fmt.Errorf("%w (compensating delete of %s failed: %s)", cause, id, MapError(err).Error())
	}
	d.InstanceID = ""
	d.IPAddress = ""
	return cause
}

// cleanupFailedCreate best-effort deletes a possibly created instance when the
// create call itself failed (for example on operation timeout).
func (d *Driver) cleanupFailedCreate(api nebiusAPI, name string, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), d.DeleteTimeout)
	defer cancel()
	instances, err := api.FindInstancesByName(ctx, d.ProjectID, name)
	if err == nil {
		for _, instance := range instances {
			if instance.GetMetadata().GetLabels()[managedByLabel] != managedByValue {
				continue
			}
			id := instance.GetMetadata().GetId()
			d.InstanceID = id
			if deleteErr := api.DeleteInstance(ctx, id, d.DeleteTimeout); deleteErr != nil {
				return fmt.Errorf("nebius: create instance %q: %s (cleanup of %s failed: %s)",
					name, MapError(cause).Error(), id, MapError(deleteErr).Error())
			}
			d.InstanceID = ""
			break
		}
	}
	return fmt.Errorf("nebius: create instance %q: %s", name, MapError(cause).Error())
}

func parseDiskType(value string) (computepb.DiskSpec_DiskType, bool) {
	switch value {
	case "NETWORK_SSD":
		return computepb.DiskSpec_NETWORK_SSD, true
	case "NETWORK_HDD":
		return computepb.DiskSpec_NETWORK_HDD, true
	case "NETWORK_SSD_NON_REPLICATED":
		return computepb.DiskSpec_NETWORK_SSD_NON_REPLICATED, true
	case "NETWORK_SSD_IO_M3":
		return computepb.DiskSpec_NETWORK_SSD_IO_M3, true
	default:
		return computepb.DiskSpec_UNSPECIFIED, false
	}
}
