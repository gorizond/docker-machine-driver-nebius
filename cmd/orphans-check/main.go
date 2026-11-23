// Command orphans-check lists Nebius instances owned by this driver and exits
// non-zero when any remain. It is read-only.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gorizond/docker-machine-driver-nebius/nebius"
)

const timeout = 2 * time.Minute

func main() {
	ids, err := listOrphans()
	if err != nil {
		fmt.Fprintln(os.Stderr, "orphans-check:", nebius.Redact(err.Error()))
		os.Exit(2)
	}
	for _, id := range ids {
		fmt.Println(id)
	}
	if len(ids) > 0 {
		os.Exit(1)
	}
}

func listOrphans() ([]string, error) {
	cfg := nebius.ClientConfig{
		ServiceAccountID: os.Getenv("NEBIUS_SA_ID"),
		PublicKeyID:      os.Getenv("NEBIUS_PUBLIC_KEY_ID"),
		PrivateKeyPath:   os.Getenv("NEBIUS_PRIVATE_KEY_PATH"),
		ProjectID:        os.Getenv("NEBIUS_PROJECT_ID"),
		APIEndpoint:      os.Getenv("NEBIUS_API_ENDPOINT"),
	}
	if cfg.ServiceAccountID == "" || cfg.PublicKeyID == "" ||
		cfg.PrivateKeyPath == "" || cfg.ProjectID == "" {
		return nil, errors.New("NEBIUS_SA_ID, NEBIUS_PUBLIC_KEY_ID, NEBIUS_PRIVATE_KEY_PATH and NEBIUS_PROJECT_ID are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := nebius.NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	instances, err := client.ListInstances(ctx, cfg.ProjectID)
	if err != nil {
		return nil, nebius.MapError(err)
	}
	orphans := nebius.FilterManagedInstances(instances, strings.TrimSpace(os.Getenv("NEBIUS_CLUSTER_ID")))

	ids := make([]string, 0, len(orphans))
	for _, instance := range orphans {
		ids = append(ids, instance.GetMetadata().GetId())
	}
	return ids, nil
}
