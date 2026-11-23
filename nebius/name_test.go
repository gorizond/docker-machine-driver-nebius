package nebius

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"node-1", "node-1"},
		{"Node 1", "node-1"},
		{"  UPPER_case.Value  ", "upper-case-value"},
		{"", "machine"},
		{"a", "a-0"},
		{"--already-valid--", "already-valid"},
		{"node___one", "node-one"},
	}

	for _, tt := range tests {
		got := NormalizeName(tt.in)
		if got != tt.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if !IsValidName(got) {
			t.Errorf("NormalizeName(%q) = %q is not a valid name", tt.in, got)
		}
	}
}

func TestNormalizeNameLongAndIdempotent(t *testing.T) {
	long := strings.Repeat("Very_Long_Machine_Name-", 10)
	got := NormalizeName(long)

	if len(got) > maxNameLength {
		t.Errorf("name length %d exceeds %d", len(got), maxNameLength)
	}
	if !IsValidName(got) {
		t.Errorf("normalized long name %q is invalid", got)
	}
	if again := NormalizeName(got); again != got {
		t.Errorf("not idempotent: %q -> %q", got, again)
	}
}

func TestNormalizeNameCollision(t *testing.T) {
	prefix := strings.Repeat("a", 54)
	first := NormalizeName(prefix + "-one")
	second := NormalizeName(prefix + "-two")
	if first == second {
		t.Fatalf("long names collided: %q", first)
	}
	if !IsValidName(first) || !IsValidName(second) {
		t.Fatalf("collision suffixes produced invalid names: %q %q", first, second)
	}
}

func TestIdempotencyKeyStable(t *testing.T) {
	first := IdempotencyKey("project-a", "node-1")
	if second := IdempotencyKey("project-a", "node-1"); first != second {
		t.Fatalf("key is not stable: %q != %q", first, second)
	}
	if IdempotencyKey("project-b", "node-1") == first {
		t.Error("key does not depend on project")
	}
	if IdempotencyKey("project-a", "node-2") == first {
		t.Error("key does not depend on machine name")
	}

	parsed, err := uuid.Parse(first)
	if err != nil {
		t.Fatalf("key is not a UUID: %v", err)
	}
	if parsed.Version() != 5 {
		t.Errorf("key version = %d, want 5", parsed.Version())
	}
}
