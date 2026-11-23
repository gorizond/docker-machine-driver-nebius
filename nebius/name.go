package nebius

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const maxNameLength = 63

var (
	invalidNameChars = regexp.MustCompile(`[^a-z0-9-]+`)
	repeatedDashes   = regexp.MustCompile(`-{2,}`)
	validNameRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$`)

	// idempotencyNamespace is fixed for the driver lifetime: changing it
	// changes every idempotency key and breaks retry deduplication.
	idempotencyNamespace = uuid.MustParse("b3c2e6a4-5b0f-4d7a-9c1e-2f8a6d4c0e11")
)

// NormalizeName converts a Rancher machine name into a Nebius resource name
// matching ^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$ (<= 63 chars).
//
// Long input is truncated and gets an 8-hex sha256 suffix of the full name so
// two machines sharing a long prefix do not collide. The function is
// idempotent: its output is already valid and is returned unchanged.
func NormalizeName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = invalidNameChars.ReplaceAllString(n, "-")
	n = repeatedDashes.ReplaceAllString(n, "-")
	n = strings.Trim(n, "-")
	if n == "" {
		n = "machine"
	}

	if len(n) > maxNameLength {
		sum := sha256.Sum256([]byte(n))
		suffix := hex.EncodeToString(sum[:4])
		head := strings.TrimRight(n[:maxNameLength-len(suffix)-1], "-")
		if head == "" {
			head = "machine"
		}
		n = head + "-" + suffix
	}
	// The contract regex requires a final alphanumeric character, so a
	// single-character name is padded deterministically.
	if len(n) < 2 {
		n += "-0"
	}
	return n
}

// IsValidName reports whether name matches the Nebius naming regex.
func IsValidName(name string) bool {
	return len(name) <= maxNameLength && validNameRe.MatchString(name)
}

// IdempotencyKey returns a stable UUIDv5 for Create/Delete retries:
// uuidv5(namespace, projectID+"/"+machineName).
func IdempotencyKey(projectID, machineName string) string {
	return uuid.NewSHA1(idempotencyNamespace, []byte(projectID+"/"+machineName)).String()
}
