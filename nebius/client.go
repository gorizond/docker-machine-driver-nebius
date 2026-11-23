package nebius

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nebius/gosdk"
	"github.com/nebius/gosdk/auth"
	"google.golang.org/grpc/codes"
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

// sleepCtx is overridable in tests.
var sleepCtx = func(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// withExhaustedRetry retries ResourceExhausted responses (quota or rate limit)
// with the server-provided RetryInfo delay or bounded exponential backoff
// (FR-016). Non-retryable errors are returned immediately.
func withExhaustedRetry[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	const attempts = 8
	for i := 0; i < attempts; i++ {
		res, err := fn(ctx)
		if err == nil {
			return res, nil
		}
		if errorCode(err) != codes.ResourceExhausted || i == attempts-1 {
			return res, err
		}
		if err := sleepCtx(ctx, RetryDelay(err, i)); err != nil {
			return zero, err
		}
	}
	return zero, nil
}

func withExhaustedRetryErr(ctx context.Context, fn func(context.Context) error) error {
	_, err := withExhaustedRetry(ctx, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

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
