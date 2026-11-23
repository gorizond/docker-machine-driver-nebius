package nebius

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nebius/gosdk/operations"
	"github.com/nebius/gosdk/serviceerror"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errorCode resolves the gRPC code of an API or operation error.
func errorCode(err error) codes.Code {
	var opErr *operations.Error
	if errors.As(err, &opErr) && opErr.Code != codes.OK {
		return opErr.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return codes.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return codes.Canceled
	}
	return status.Code(err)
}

// errorMessage resolves the server message without dumping the whole error.
func errorMessage(err error) string {
	var opErr *operations.Error
	if errors.As(err, &opErr) {
		return opErr.Message
	}
	return status.Convert(err).Message()
}

// IsNotFound reports whether err is a gRPC NotFound (delete success, get None).
func IsNotFound(err error) bool { return errorCode(err) == codes.NotFound }

// IsUnauthenticated reports whether the API rejected credentials.
func IsUnauthenticated(err error) bool { return errorCode(err) == codes.Unauthenticated }

// IsAlreadyExists reports whether the resource already exists.
func IsAlreadyExists(err error) bool { return errorCode(err) == codes.AlreadyExists }

// IsRetryable reports whether the error class is worth retrying.
func IsRetryable(err error) bool {
	switch errorCode(err) {
	case codes.ResourceExhausted, codes.Unavailable, codes.Aborted, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}

// RetryDelay returns the server-provided RetryInfo delay when present,
// otherwise a bounded exponential backoff (1s, 2s, ... max 30s).
func RetryDelay(err error, attempt int) time.Duration {
	if st, ok := status.FromError(err); ok {
		for _, detail := range st.Details() {
			if info, ok := detail.(*errdetails.RetryInfo); ok {
				if delay := info.GetRetryDelay(); delay != nil && delay.AsDuration() > 0 {
					return delay.AsDuration()
				}
			}
		}
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 5 {
		attempt = 5
	}
	delay := time.Second << attempt
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	return delay
}

// MapError converts an API error into an actionable message. Private keys,
// JWTs and tokens never reach the returned text.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	var msg string
	switch errorCode(err) {
	case codes.Unauthenticated:
		msg = "nebius: credentials rejected; check sa-id, public-key-id and the private key"
	case codes.PermissionDenied:
		msg = "nebius: permission denied; check the service account roles for the project and subnet"
	case codes.NotFound:
		msg = "nebius: resource not found"
	case codes.AlreadyExists:
		msg = "nebius: resource already exists; reconcile the instance by name"
	case codes.InvalidArgument:
		msg = "nebius: invalid argument: " + Redact(errorMessage(err))
	case codes.ResourceExhausted:
		if delay := RetryDelay(err, 0); delay > 0 {
			msg = fmt.Sprintf("nebius: quota or rate limit exhausted; retry in %s", delay)
		} else {
			msg = "nebius: quota or rate limit exhausted"
		}
	case codes.Unavailable:
		msg = "nebius: service temporarily unavailable; retry later"
	case codes.DeadlineExceeded:
		msg = "nebius: operation timed out"
	default:
		return errors.New(Redact(err.Error()))
	}
	if id := requestID(err); id != "" {
		msg += " (request " + id + ")"
	}
	return errors.New(msg)
}

func requestID(err error) string {
	var se *serviceerror.Error
	if errors.As(err, &se) {
		return se.RequestID
	}
	return ""
}
