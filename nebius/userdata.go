package nebius

import (
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// MergeUserData merges the driver SSH user and key into a #cloud-config
// supplied by Rancher's custom-install-script path. The rest of the document
// (runcmd, write_files, other users) is preserved.
func MergeUserData(raw string, cfg CloudInit) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RenderCloudInit(cfg)
	}
	if !strings.HasPrefix(raw, "#cloud-config") {
		return "", errors.New("nebius: userdata must be a cloud-config")
	}

	user := strings.TrimSpace(cfg.User)
	if user == "" {
		return "", errors.New("nebius: cloud-init requires an SSH user")
	}
	keys := normalizeKeys(cfg.SSHAuthorizedKeys)
	if len(keys) == 0 {
		return "", errors.New("nebius: cloud-init requires at least one SSH public key")
	}

	var doc map[string]any
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return "", fmt.Errorf("nebius: parse userdata: %w", err)
	}
	if doc == nil {
		doc = map[string]any{}
	}

	list, err := userList(doc["users"])
	if err != nil {
		return "", err
	}

	merged := false
	for _, entry := range list {
		m, ok := entry.(map[string]any)
		if !ok {
			return "", errors.New("nebius: userdata users entries must be mappings")
		}
		name, _ := m["name"].(string)
		if strings.TrimSpace(name) != user {
			continue
		}
		existing, err := keyList(m["ssh_authorized_keys"])
		if err != nil {
			return "", err
		}
		m["ssh_authorized_keys"] = normalizeKeys(append(existing, keys...))
		if _, ok := m["sudo"]; !ok {
			m["sudo"] = "ALL=(ALL) NOPASSWD:ALL"
		}
		if _, ok := m["shell"]; !ok {
			m["shell"] = "/bin/bash"
		}
		merged = true
		break
	}
	if !merged {
		list = append(list, map[string]any{
			"name":                user,
			"sudo":                "ALL=(ALL) NOPASSWD:ALL",
			"shell":               "/bin/bash",
			"ssh_authorized_keys": keys,
		})
	}
	doc["users"] = list

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("nebius: render userdata: %w", err)
	}
	rendered := "#cloud-config\n" + string(out)
	if len(rendered) > MaxCloudInitBytes {
		return "", fmt.Errorf("nebius: cloud-init is %d bytes, exceeds the %d byte API limit", len(rendered), MaxCloudInitBytes)
	}
	return rendered, nil
}

func userList(value any) ([]any, error) {
	switch v := value.(type) {
	case nil:
		return []any{}, nil
	case []any:
		return v, nil
	default:
		return nil, errors.New("nebius: userdata users must be a list")
	}
}

func keyList(value any) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []any:
		keys := make([]string, 0, len(v))
		for _, item := range v {
			key, ok := item.(string)
			if !ok {
				return nil, errors.New("nebius: userdata ssh_authorized_keys entries must be strings")
			}
			keys = append(keys, key)
		}
		return keys, nil
	case []string:
		return v, nil
	default:
		return nil, errors.New("nebius: userdata ssh_authorized_keys must be a list")
	}
}

// normalizeKeys trims keys, drops blanks and removes duplicates.
func normalizeKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}
