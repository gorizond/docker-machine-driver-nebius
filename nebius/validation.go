package nebius

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// maxUserDataBytes caps how much of a userdata file is read for validation.
const maxUserDataBytes = 64 * 1024

// validateUserDataFile checks that path is a regular file whose content
// starts with #cloud-config. Errors name the path and never include content.
func validateUserDataFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("nebius: userdata %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("nebius: userdata %s: not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("nebius: userdata %s: %w", path, err)
	}
	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(f, maxUserDataBytes))
	if err != nil {
		return fmt.Errorf("nebius: userdata %s: %w", path, err)
	}
	if !strings.HasPrefix(string(raw), "#cloud-config") {
		return fmt.Errorf("nebius: userdata %s must start with #cloud-config", path)
	}
	return nil
}

// validateStatic checks the machine configuration without any API call.
func (d *Driver) validateStatic() error {
	var problems []string
	add := func(cond bool, msg string) {
		if cond {
			problems = append(problems, msg)
		}
	}

	add(d.SAID == "", "sa-id is required")
	add(d.PublicKeyID == "", "public-key-id is required")
	add(d.PrivateKey == "" && d.PrivateKeyPath == "", "private-key or private-key-path is required")
	add(d.ProjectID == "", "project-id is required")
	add(d.SubnetID == "", "subnet-id is required")
	add(d.Platform == "", "platform is required")
	add(d.Preset == "", "preset is required")
	add(d.ImageFamily == "" && d.ImageID == "", "one of image-family or image-id is required")
	add(d.ImageFamily != "" && d.ImageID != "", "image-family and image-id are mutually exclusive")
	add(d.DiskSizeGiB < 1, "disk-size-gib must be greater than zero")
	if _, ok := parseDiskType(d.DiskType); !ok {
		problems = append(problems, "disk-type must be NETWORK_SSD, NETWORK_HDD, NETWORK_SSD_NON_REPLICATED or NETWORK_SSD_IO_M3")
	}
	switch d.PublicIP {
	case "static", "dynamic", "none":
	default:
		problems = append(problems, "public-ip must be static, dynamic or none")
	}
	add(d.PublicIP == "none" && d.UserDataFile == "", "public-ip=none requires userdata (Rancher custom install script)")
	add(d.SSHUser == "", "ssh-user is required")
	add(d.SSHPort < 1 || d.SSHPort > 65535, "ssh-port must be between 1 and 65535")
	add(d.CreateTimeout <= 0, "create-timeout must be greater than zero")
	add(d.DeleteTimeout <= 0, "delete-timeout must be greater than zero")
	for key, value := range d.Labels {
		if !validLabelKey(key) {
			problems = append(problems, "invalid label key "+key)
		}
		if !validLabelValue(value) {
			problems = append(problems, "invalid label value for key "+key)
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("nebius: invalid configuration: %s", strings.Join(problems, "; "))
}
