package nebius

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nebius/gosdk"
	"github.com/nebius/gosdk/auth"
)

// ClientConfig holds everything needed to build an authenticated gosdk client.
type ClientConfig struct {
	ServiceAccountID string
	PublicKeyID      string
	PrivateKey       string
	PrivateKeyPath   string
	ProjectID        string
	APIEndpoint      string
}

// Client wraps the Nebius gosdk. The SDK exchanges the service account JWT for
// an IAM access token and refreshes it asynchronously; no manual refresh here.
type Client struct {
	sdk       *gosdk.SDK
	projectID string
}

// NewClient initializes the Nebius SDK. It performs no API call, so nothing is
// authenticated until the first request.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	if cfg.ProjectID == "" {
		return nil, errors.New("nebius: project-id is required")
	}
	if cfg.ServiceAccountID == "" || cfg.PublicKeyID == "" {
		return nil, errors.New("nebius: sa-id and public-key-id are required")
	}

	var reader auth.ServiceAccountReader
	switch {
	case cfg.PrivateKey != "":
		reader = auth.NewPrivateKeyParser([]byte(cfg.PrivateKey), cfg.PublicKeyID, cfg.ServiceAccountID)
	case cfg.PrivateKeyPath != "":
		reader = auth.NewPrivateKeyFileParser(nil, cfg.PrivateKeyPath, cfg.PublicKeyID, cfg.ServiceAccountID)
	default:
		return nil, errors.New("nebius: private-key or private-key-path is required")
	}

	opts := []gosdk.Option{
		gosdk.WithParentID(cfg.ProjectID),
		gosdk.WithCredentials(gosdk.ServiceAccountReader(reader)),
	}
	if cfg.APIEndpoint != "" {
		opts = append(opts, gosdk.WithDomain(cfg.APIEndpoint))
	}

	sdk, err := gosdk.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("nebius: initialize SDK: %s", Redact(err.Error()))
	}
	return &Client{sdk: sdk, projectID: cfg.ProjectID}, nil
}

// Close releases SDK connections and the background token refresher.
func (c *Client) Close() error {
	if c == nil || c.sdk == nil {
		return nil
	}
	return c.sdk.Close()
}

// ParentID returns the project ID the client was built for.
func (c *Client) ParentID() string { return c.projectID }

// withAuthRetry repeats fn once when the API rejected credentials (FR-003).
func withAuthRetry[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	res, err := fn(ctx)
	if err == nil || !IsUnauthenticated(err) {
		return res, err
	}
	select {
	case <-ctx.Done():
		return res, ctx.Err()
	case <-time.After(time.Second):
	}
	return fn(ctx)
}
