package nebius

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const readTimeout = 30 * time.Second

// Remove deletes the machine instance and waits for the operation. It is
// idempotent: a missing instance (NotFound or no instance at all) is success.
func (d *Driver) Remove() (err error) {
	start := time.Now()
	logID := d.InstanceID
	defer func() { LogOperation(context.Background(), "remove", logID, start, err) }()

	ctx, cancel := context.WithTimeout(context.Background(), d.DeleteTimeout)
	defer cancel()

	api, err := d.apiClient(ctx)
	if err != nil {
		return err
	}

	id := d.InstanceID
	if id == "" {
		name := NormalizeName(d.MachineName)
		instances, findErr := api.FindInstancesByName(ctx, d.ProjectID, name)
		if findErr != nil {
			return fmt.Errorf("nebius: lookup instance %q: %s", name, MapError(findErr).Error())
		}
		switch len(instances) {
		case 0:
			return nil
		case 1:
			if instances[0].GetMetadata().GetLabels()[managedByLabel] != managedByValue {
				return fmt.Errorf("nebius: instance %q is not managed by %s; refusing to delete", name, managedByValue)
			}
			id = instances[0].GetMetadata().GetId()
		default:
			return fmt.Errorf("nebius: found %d instances named %q; refusing to delete", len(instances), name)
		}
		logID = id
	}

	if err = api.DeleteInstance(ctx, id, d.DeleteTimeout); err != nil {
		return fmt.Errorf("nebius: delete instance %s: %s", id, MapError(err).Error())
	}
	d.InstanceID = ""
	d.IPAddress = ""
	return nil
}

// GetIP returns the machine IPv4 address, refreshing it from the API when the
// cached value is empty. In userdata/private-only mode it returns the private
// address; a missing address is an explicit error, never an empty answer.
func (d *Driver) GetIP() (string, error) {
	if d.IPAddress != "" {
		return d.IPAddress, nil
	}
	if d.PublicIP == "none" && d.UserDataFile == "" {
		return "", fmt.Errorf("%w (public-ip=none)", errNoPublicIP)
	}
	if d.InstanceID == "" {
		return "", errors.New("nebius: machine has no instance yet")
	}

	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()

	api, err := d.apiClient(ctx)
	if err != nil {
		return "", err
	}
	instance, err := api.GetInstance(ctx, d.InstanceID)
	if err != nil {
		return "", fmt.Errorf("nebius: get instance %s: %s", d.InstanceID, MapError(err).Error())
	}
	if d.PublicIP == "none" {
		ip := PublicIPv4(instance)
		if ip == "" {
			ip = PrivateIPv4(instance)
		}
		if ip == "" {
			return "", errNoPrivateIP
		}
		d.IPAddress = ip
		return ip, nil
	}
	ip := PublicIPv4(instance)
	if ip == "" {
		return "", errNoPublicIP
	}
	d.IPAddress = ip
	return ip, nil
}
