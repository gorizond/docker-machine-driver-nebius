package nebius

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWithExhaustedRetrySucceedsAfterQuota(t *testing.T) {
	old := sleepCtx
	sleepCtx = func(context.Context, time.Duration) error { return nil }
	defer func() { sleepCtx = old }()

	calls := 0
	got, err := withExhaustedRetry(context.Background(), func(context.Context) (int, error) {
		calls++
		if calls < 3 {
			return 0, status.Error(codes.ResourceExhausted, "quota")
		}
		return 42, nil
	})
	if err != nil || got != 42 || calls != 3 {
		t.Fatalf("got=%d calls=%d err=%v", got, calls, err)
	}
}

func TestWithExhaustedRetryStopsOnOtherErrors(t *testing.T) {
	calls := 0
	_, err := withExhaustedRetry(context.Background(), func(context.Context) (int, error) {
		calls++
		return 0, status.Error(codes.InvalidArgument, "bad")
	})
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
