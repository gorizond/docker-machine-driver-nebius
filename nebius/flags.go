package nebius

import (
	"regexp"
	"strings"
	"time"

	"github.com/docker/machine/libmachine/mcnflag"
)

const (
	defaultAPIEndpoint = "api.nebius.cloud:443"

	defaultDiskSizeGiB       = 50
	defaultDiskType          = "NETWORK_SSD"
	defaultPublicIP          = "static"
	defaultCreateTimeoutFlag = "20m"
	defaultDeleteTimeoutFlag = "5m"

	defaultSSHUser = "ubuntu"
	defaultSSHPort = 22
)

// GetCreateFlags returns the flags exposed by the nebius driver.
//
// FR-007: no flag may contain user-data, userdata, custom-data or
// cloud-config. Otherwise rancher-machine treats the value as a file and
// stops running WithCustomScript, so rancher-system-agent never installs.
func (d *Driver) GetCreateFlags() []mcnflag.Flag {
	return []mcnflag.Flag{
		// Cloud credentials.
		mcnflag.StringFlag{
			Name:  "nebius-sa-id",
			Usage: "Nebius service account ID (serviceaccount-...)",
		},
		mcnflag.StringFlag{
			Name:  "nebius-public-key-id",
			Usage: "Nebius public key ID (publickey-...)",
		},
		mcnflag.StringFlag{
			Name:  "nebius-private-key",
			Usage: "Inline PEM private key of the service account (CLI mode)",
		},
		mcnflag.StringFlag{
			Name:  "nebius-private-key-path",
			Usage: "Path to the PEM private key of the service account",
		},
		mcnflag.StringFlag{
			Name:  "nebius-project-id",
			Usage: "Nebius project ID (project-...)",
		},
		mcnflag.StringFlag{
			Name:  "nebius-api-endpoint",
			Usage: "Nebius API endpoint (host:port)",
			Value: defaultAPIEndpoint,
		},

		// Machine configuration.
		mcnflag.StringFlag{
			Name:  "nebius-subnet-id",
			Usage: "Nebius subnet ID (vpcsubnet-...)",
		},
		mcnflag.StringFlag{
			Name:  "nebius-image-family",
			Usage: "Image family to boot from (e.g. ubuntu24.04-driverless)",
		},
		mcnflag.StringFlag{
			Name:  "nebius-image-id",
			Usage: "Exact image ID to boot from (mutually exclusive with image-family)",
		},
		mcnflag.StringFlag{
			Name:  "nebius-platform",
			Usage: "Nebius compute platform (e.g. cpu-d3)",
		},
		mcnflag.StringFlag{
			Name:  "nebius-preset",
			Usage: "Nebius platform preset (e.g. 4vcpu-16gb)",
		},
		mcnflag.IntFlag{
			Name:  "nebius-disk-size-gib",
			Usage: "Boot disk size in GiB",
			Value: defaultDiskSizeGiB,
		},
		mcnflag.StringFlag{
			Name:  "nebius-disk-type",
			Usage: "Boot disk type (NETWORK_SSD, NETWORK_HDD, ...)",
			Value: defaultDiskType,
		},
		mcnflag.StringFlag{
			Name:  "nebius-public-ip",
			Usage: "Public IP mode: static, dynamic or none",
			Value: defaultPublicIP,
		},
		mcnflag.StringSliceFlag{
			Name:  "nebius-security-groups",
			Usage: "Comma-separated security group IDs",
		},
		mcnflag.BoolFlag{
			Name:  "nebius-preemptible",
			Usage: "Create a preemptible instance (irreversible)",
		},
		mcnflag.StringFlag{
			Name:  "nebius-ssh-user",
			Usage: "SSH user created via cloud-init",
			Value: defaultSSHUser,
		},
		mcnflag.IntFlag{
			Name:  "nebius-ssh-port",
			Usage: "SSH port",
			Value: defaultSSHPort,
		},
		mcnflag.StringFlag{
			Name:  "nebius-create-timeout",
			Usage: "Maximum time to wait for instance creation and SSH",
			Value: defaultCreateTimeoutFlag,
		},
		mcnflag.StringFlag{
			Name:  "nebius-delete-timeout",
			Usage: "Maximum time to wait for instance deletion",
			Value: defaultDeleteTimeoutFlag,
		},
		mcnflag.StringSliceFlag{
			Name:  "nebius-labels",
			Usage: "Additional labels as key:value,key:value",
		},
	}
}

// parseDurationFlag parses a user-supplied duration flag.
func parseDurationFlag(name, value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, &invalidFlagError{name: name, value: value, reason: "must be greater than zero"}
	}
	return d, nil
}

type invalidFlagError struct {
	name   string
	value  string
	reason string
}

func (e *invalidFlagError) Error() string {
	return "nebius: invalid --" + e.name + " value " + e.value + ": " + e.reason
}

// parseLabels parses label items in key:value form. Items may also contain
// commas when Rancher passes a single comma-separated string.
func parseLabels(raw []string) (map[string]string, error) {
	labels := make(map[string]string, len(raw))
	for _, item := range raw {
		for _, pair := range strings.Split(item, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			key, value, ok := strings.Cut(pair, ":")
			if !ok {
				return nil, &invalidFlagError{name: "nebius-labels", value: pair, reason: "expected key:value"}
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "" {
				return nil, &invalidFlagError{name: "nebius-labels", value: pair, reason: "empty label key"}
			}
			labels[key] = value
		}
	}
	if len(labels) == 0 {
		return nil, nil
	}
	return labels, nil
}

var labelValueRe = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_.]{0,61}[A-Za-z0-9])?$`)

func validLabelKey(key string) bool {
	if len(key) > 63 || key == "" {
		return false
	}
	return labelValueRe.MatchString(key)
}

func validLabelValue(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 63 {
		return false
	}
	return labelValueRe.MatchString(value)
}
