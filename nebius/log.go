package nebius

import (
	"context"
	"log/slog"
	"regexp"
	"sync"
	"time"
)

var (
	loggerMu sync.RWMutex
	logger   = slog.Default()

	pemSecretRe   = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	jwtRe         = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}`)
	bearerRe      = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
	secretValueRe = regexp.MustCompile(`(?i)\b(token|access[_-]?token|private[_-]?key|api[_-]?key|secret)\s*[=:]\s*"?[^"\s,;&}]+`)
)

// SetLogger sets the package logger; nil resets it to slog.Default().
func SetLogger(l *slog.Logger) {
	if l == nil {
		l = slog.Default()
	}
	loggerMu.Lock()
	logger = l
	loggerMu.Unlock()
}

// Redact removes private keys, JWTs, bearer tokens and key=value secrets from
// free-form text before it reaches a log or an error message.
func Redact(s string) string {
	s = pemSecretRe.ReplaceAllString(s, "[REDACTED]")
	s = jwtRe.ReplaceAllString(s, "[REDACTED]")
	s = bearerRe.ReplaceAllString(s, "Bearer [REDACTED]")
	s = secretValueRe.ReplaceAllString(s, "$1=[REDACTED]")
	return s
}

// LogOperation records one driver operation with its instance and timings.
// Only safe fields are logged; the error text is redacted.
func LogOperation(ctx context.Context, operation, instanceID string, start time.Time, err error) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	attrs := []slog.Attr{slog.String("operation", operation)}
	if instanceID != "" {
		attrs = append(attrs, slog.String("instance_id", instanceID))
	}
	attrs = append(attrs, slog.Duration("duration", time.Since(start)), slog.String("result", result))
	if err != nil {
		attrs = append(attrs, slog.String("error", Redact(err.Error())))
	}

	loggerMu.RLock()
	l := logger
	loggerMu.RUnlock()
	l.LogAttrs(ctx, slog.LevelInfo, "nebius operation", attrs...)
}
