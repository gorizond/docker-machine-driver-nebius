package nebius

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

const (
	fakePEM = "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END PRIVATE KEY-----"
	fakeJWT = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.c2lnbmF0dXJl"
)

func TestLogOperationFields(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	defer SetLogger(nil)

	LogOperation(context.Background(), "create", "computeinstance-abc", time.Now().Add(-time.Second), nil)
	out := buf.String()
	for _, want := range []string{"operation=create", "instance_id=computeinstance-abc", "duration=", "result=ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
}

func TestLogOperationRedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	defer SetLogger(nil)

	err := errors.New("auth failed: " + fakePEM + " " + fakeJWT + " token=super-secret-value")
	LogOperation(context.Background(), "create", "computeinstance-abc", time.Now(), err)
	out := buf.String()

	if !strings.Contains(out, "result=error") {
		t.Errorf("log output missing error result: %s", out)
	}
	for _, secret := range []string{"MIIEvQIBADAN", fakeJWT, "super-secret-value", "BEGIN PRIVATE KEY"} {
		if strings.Contains(out, secret) {
			t.Errorf("log leaked secret %q: %s", secret, out)
		}
	}
}

func TestRedact(t *testing.T) {
	in := fakePEM + " " + fakeJWT + " Bearer abc.def.ghi token=abc123 private_key: topsecret"
	out := Redact(in)
	for _, secret := range []string{"MIIEvQIBADAN", fakeJWT, "abc.def.ghi", "abc123", "topsecret"} {
		if strings.Contains(out, secret) {
			t.Errorf("Redact leaked %q: %s", secret, out)
		}
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("Redact produced no marker: %s", out)
	}
}
