package nebius

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestIsHelpers(t *testing.T) {
	notFound := status.Error(codes.NotFound, "missing")
	if !IsNotFound(notFound) || IsUnauthenticated(notFound) || IsAlreadyExists(notFound) {
		t.Error("NotFound classification is wrong")
	}
	if !IsUnauthenticated(status.Error(codes.Unauthenticated, "denied")) {
		t.Error("Unauthenticated not detected")
	}
	if !IsAlreadyExists(status.Error(codes.AlreadyExists, "exists")) {
		t.Error("AlreadyExists not detected")
	}
	for _, code := range []codes.Code{codes.ResourceExhausted, codes.Unavailable, codes.DeadlineExceeded} {
		if !IsRetryable(status.Error(code, "retry")) {
			t.Errorf("%s should be retryable", code)
		}
	}
	if IsRetryable(notFound) {
		t.Error("NotFound must not be retryable")
	}
}

func TestMapErrorActionable(t *testing.T) {
	tests := []struct {
		code codes.Code
		want string
	}{
		{codes.Unauthenticated, "credentials rejected"},
		{codes.PermissionDenied, "permission denied"},
		{codes.NotFound, "not found"},
		{codes.AlreadyExists, "already exists"},
		{codes.InvalidArgument, "invalid argument"},
		{codes.ResourceExhausted, "rate limit"},
		{codes.Unavailable, "temporarily unavailable"},
		{codes.DeadlineExceeded, "timed out"},
	}
	for _, tt := range tests {
		err := MapError(status.Error(tt.code, "server message"))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("MapError(%s) = %v, want substring %q", tt.code, err, tt.want)
		}
	}
	if MapError(nil) != nil {
		t.Error("MapError(nil) must be nil")
	}
}

func TestMapErrorNeverLeaksSecrets(t *testing.T) {
	err := MapError(status.Error(codes.InvalidArgument, "bad field token=super-secret-value"))
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Errorf("MapError leaked token: %v", err)
	}

	plain := errors.New("broken key " + fakePEM + " " + fakeJWT)
	mapped := MapError(plain)
	for _, secret := range []string{"MIIEvQIBADAN", fakeJWT} {
		if strings.Contains(mapped.Error(), secret) {
			t.Errorf("MapError leaked %q: %v", secret, mapped)
		}
	}
}

func TestContextErrorsAreClassified(t *testing.T) {
	if !IsRetryable(context.DeadlineExceeded) {
		t.Error("context deadline must be retryable")
	}
	if err := MapError(context.DeadlineExceeded); !strings.Contains(err.Error(), "timed out") {
		t.Errorf("MapError(context deadline) = %v", err)
	}
}

func TestRetryDelay(t *testing.T) {
	if got := RetryDelay(status.Error(codes.Unavailable, "x"), 0); got != time.Second {
		t.Errorf("attempt 0 delay = %s, want 1s", got)
	}
	if got := RetryDelay(status.Error(codes.Unavailable, "x"), 1); got != 2*time.Second {
		t.Errorf("attempt 1 delay = %s, want 2s", got)
	}
	if got := RetryDelay(status.Error(codes.Unavailable, "x"), 20); got != 30*time.Second {
		t.Errorf("attempt 20 delay = %s, want 30s cap", got)
	}

	withInfo, err := status.New(codes.ResourceExhausted, "slow down").WithDetails(
		&errdetails.RetryInfo{RetryDelay: durationpb.New(7 * time.Second)},
	)
	if err != nil {
		t.Fatalf("WithDetails: %v", err)
	}
	if got := RetryDelay(withInfo.Err(), 0); got != 7*time.Second {
		t.Errorf("RetryInfo delay = %s, want 7s", got)
	}
}
