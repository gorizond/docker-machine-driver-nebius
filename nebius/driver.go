package nebius

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/state"
	computepb "github.com/nebius/gosdk/proto/nebius/compute/v1"
	"golang.org/x/crypto/ssh"
)

const DriverName = "nebius"

// nebiusAPI is the narrow client surface used by the driver; *Client implements
// it, tests inject a fake.
type nebiusAPI interface {
	Preflight(context.Context, PreflightConfig) error
	FindInstancesByName(context.Context, string, string) ([]*computepb.Instance, error)
	GetInstance(context.Context, string) (*computepb.Instance, error)
	CreateInstance(context.Context, *computepb.CreateInstanceRequest, string, time.Duration) (*computepb.Instance, error)
	DeleteInstance(context.Context, string, time.Duration) error
	StartInstance(context.Context, string, time.Duration) error
	StopInstance(context.Context, string, time.Duration) error
}

type sshWaiter func(context.Context, string, string, ssh.Signer, time.Duration) error

// Driver is the Rancher machine driver for Nebius AI Cloud.
type Driver struct {
	*drivers.BaseDriver

	// Cloud credentials. The private key is persisted only in the machine
	// store; it is never logged or embedded in errors.
	SAID           string `json:"saId,omitempty"`
	PublicKeyID    string `json:"publicKeyId,omitempty"`
	PrivateKey     string `json:"privateKey,omitempty"`
	PrivateKeyPath string `json:"privateKeyPath,omitempty"`

	ProjectID   string `json:"projectId,omitempty"`
	APIEndpoint string `json:"apiEndpoint,omitempty"`

	// Machine configuration.
	SubnetID       string            `json:"subnetId,omitempty"`
	ImageFamily    string            `json:"imageFamily,omitempty"`
	ImageID        string            `json:"imageId,omitempty"`
	Platform       string            `json:"platform,omitempty"`
	Preset         string            `json:"preset,omitempty"`
	DiskSizeGiB    int               `json:"diskSizeGib,omitempty"`
	DiskType       string            `json:"diskType,omitempty"`
	PublicIP       string            `json:"publicIp,omitempty"`
	SecurityGroups []string          `json:"securityGroups,omitempty"`
	Preemptible    bool              `json:"preemptible,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	CreateTimeout  time.Duration     `json:"createTimeout,omitempty"`
	DeleteTimeout  time.Duration     `json:"deleteTimeout,omitempty"`

	// InstanceID is the Nebius instance backing this machine, once created
	// or adopted. Empty means the machine has no instance yet.
	InstanceID string `json:"instanceId,omitempty"`

	client  nebiusAPI
	waitSSH sshWaiter
}

var (
	_ drivers.Driver = (*Driver)(nil)
	_ nebiusAPI      = (*Client)(nil)
)

func NewDriver(hostName, storePath string) *Driver {
	return &Driver{
		BaseDriver: &drivers.BaseDriver{
			MachineName: hostName,
			StorePath:   storePath,
			SSHUser:     defaultSSHUser,
			SSHPort:     defaultSSHPort,
		},
		APIEndpoint:   defaultAPIEndpoint,
		DiskSizeGiB:   defaultDiskSizeGiB,
		DiskType:      defaultDiskType,
		PublicIP:      defaultPublicIP,
		CreateTimeout: 20 * time.Minute,
		DeleteTimeout: 5 * time.Minute,
		waitSSH:       waitForSSH,
	}
}

func (d *Driver) DriverName() string { return DriverName }

// SetConfigFromFlags copies create flags into the driver. Defaults from
// GetCreateFlags/NewDriver are kept when Rancher omits a flag.
func (d *Driver) SetConfigFromFlags(opts drivers.DriverOptions) error {
	if v := opts.String("nebius-sa-id"); v != "" {
		d.SAID = v
	}
	if v := opts.String("nebius-public-key-id"); v != "" {
		d.PublicKeyID = v
	}
	if v := opts.String("nebius-private-key"); v != "" {
		d.PrivateKey = v
	}
	if v := opts.String("nebius-private-key-path"); v != "" {
		d.PrivateKeyPath = v
	}
	if v := opts.String("nebius-project-id"); v != "" {
		d.ProjectID = v
	}
	if v := opts.String("nebius-api-endpoint"); v != "" {
		d.APIEndpoint = v
	}
	if v := opts.String("nebius-subnet-id"); v != "" {
		d.SubnetID = v
	}
	if v := opts.String("nebius-image-family"); v != "" {
		d.ImageFamily = v
	}
	if v := opts.String("nebius-image-id"); v != "" {
		d.ImageID = v
	}
	if v := opts.String("nebius-platform"); v != "" {
		d.Platform = v
	}
	if v := opts.String("nebius-preset"); v != "" {
		d.Preset = v
	}
	if v := opts.Int("nebius-disk-size-gib"); v > 0 {
		d.DiskSizeGiB = v
	}
	if v := opts.String("nebius-disk-type"); v != "" {
		d.DiskType = v
	}
	if v := opts.String("nebius-public-ip"); v != "" {
		d.PublicIP = v
	}
	if v := opts.StringSlice("nebius-security-groups"); len(v) > 0 {
		d.SecurityGroups = v
	}
	d.Preemptible = opts.Bool("nebius-preemptible")
	if v := opts.String("nebius-ssh-user"); v != "" {
		d.SSHUser = v
	}
	if v := opts.Int("nebius-ssh-port"); v > 0 {
		d.SSHPort = v
	}
	if v := opts.String("nebius-create-timeout"); v != "" {
		timeout, err := parseDurationFlag("nebius-create-timeout", v)
		if err != nil {
			return err
		}
		d.CreateTimeout = timeout
	}
	if v := opts.String("nebius-delete-timeout"); v != "" {
		timeout, err := parseDurationFlag("nebius-delete-timeout", v)
		if err != nil {
			return err
		}
		d.DeleteTimeout = timeout
	}
	if v := opts.StringSlice("nebius-labels"); len(v) > 0 {
		labels, err := parseLabels(v)
		if err != nil {
			return err
		}
		d.Labels = labels
	}
	return nil
}

// PreCreateCheck validates the static configuration and runs the read-only API
// preflight (FR-004). No resource is created on failure.
func (d *Driver) PreCreateCheck() error {
	if err := d.validateStatic(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), preflightTimeout)
	defer cancel()

	api, err := d.apiClient(ctx)
	if err != nil {
		return err
	}
	if err := api.Preflight(ctx, PreflightConfig{
		ProjectID:      d.ProjectID,
		SubnetID:       d.SubnetID,
		ImageFamily:    d.ImageFamily,
		ImageID:        d.ImageID,
		Platform:       d.Platform,
		Preset:         d.Preset,
		SecurityGroups: d.SecurityGroups,
	}); err != nil {
		return fmt.Errorf("nebius: preflight failed: %w", err)
	}
	return nil
}

// GetState reads the instance state from the API. A missing instance maps to
// state.None, so a lost machine still resolves cleanly.
func (d *Driver) GetState() (state.State, error) {
	if d.InstanceID == "" {
		return state.None, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()

	api, err := d.apiClient(ctx)
	if err != nil {
		return state.Error, err
	}
	instance, err := api.GetInstance(ctx, d.InstanceID)
	if IsNotFound(err) {
		return state.None, nil
	}
	if err != nil {
		return state.Error, MapError(err)
	}
	return MachineState(instance), nil
}

// GetSSHHostname returns the public IPv4 address of the machine.
func (d *Driver) GetSSHHostname() (string, error) { return d.GetIP() }

// GetURL links to the instance in the Nebius console.
func (d *Driver) GetURL() (string, error) {
	return "https://console.nebius.com/projects/" + url.PathEscape(d.ProjectID), nil
}

// apiClient lazily builds the SDK client. It is not safe for concurrent use,
// which matches docker-machine's sequential driver calls.
func (d *Driver) apiClient(ctx context.Context) (nebiusAPI, error) {
	if d.client != nil {
		return d.client, nil
	}
	client, err := NewClient(ctx, ClientConfig{
		ServiceAccountID: d.SAID,
		PublicKeyID:      d.PublicKeyID,
		PrivateKey:       d.PrivateKey,
		PrivateKeyPath:   d.PrivateKeyPath,
		ProjectID:        d.ProjectID,
		APIEndpoint:      d.APIEndpoint,
	})
	if err != nil {
		return nil, err
	}
	d.client = client
	return client, nil
}

var errNoPublicIP = errors.New("nebius: machine has no public IPv4 address")
