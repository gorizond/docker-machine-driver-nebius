package nebius

import (
	"errors"
	"fmt"
	"strings"
)

// MaxCloudInitBytes is the Nebius API limit for cloudInitUserData.
const MaxCloudInitBytes = 32768

// CloudInit describes the minimal cloud-config rendered by the driver.
type CloudInit struct {
	User              string
	SSHAuthorizedKeys []string
}

// RenderCloudInit renders a minimal #cloud-config with an SSH user and the
// machine public key. It never enables package_upgrade: a provider reboot
// would otherwise break the provisioning session.
func RenderCloudInit(cfg CloudInit) (string, error) {
	user := strings.TrimSpace(cfg.User)
	if user == "" {
		return "", errors.New("nebius: cloud-init requires an SSH user")
	}
	keys := make([]string, 0, len(cfg.SSHAuthorizedKeys))
	for _, key := range cfg.SSHAuthorizedKeys {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return "", errors.New("nebius: cloud-init requires at least one SSH public key")
	}

	var b strings.Builder
	b.WriteString("#cloud-config\n")
	b.WriteString("users:\n")
	fmt.Fprintf(&b, "  - name: %s\n", yamlQuote(user))
	b.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
	b.WriteString("    shell: /bin/bash\n")
	b.WriteString("    ssh_authorized_keys:\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "      - %s\n", yamlQuote(key))
	}

	out := b.String()
	if len(out) > MaxCloudInitBytes {
		return "", fmt.Errorf("nebius: cloud-init is %d bytes, exceeds the %d byte API limit", len(out), MaxCloudInitBytes)
	}
	return out, nil
}

func yamlQuote(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + replacer.Replace(s) + `"`
}
