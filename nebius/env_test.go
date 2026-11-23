package nebius

import "testing"

func TestSetConfigFromFlagsCredentialEnv(t *testing.T) {
	t.Setenv("NEBIUS_SA_ID", "serviceaccount-env")
	t.Setenv("NEBIUS_PUBLIC_KEY_ID", "publickey-env")
	t.Setenv("NEBIUS_PRIVATE_KEY", "-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----")
	t.Setenv("NEBIUS_PROJECT_ID", "project-env")
	t.Setenv("NEBIUS_API_ENDPOINT", "api.nebius.cloud:443")

	d := NewDriver("node-1", "/tmp/store")
	err := d.SetConfigFromFlags(fakeOptions{
		"nebius-subnet-id":    "vpcsubnet-1",
		"nebius-image-family": "ubuntu24.04-driverless",
		"nebius-platform":     "cpu-d3",
		"nebius-preset":       "4vcpu-16gb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.SAID != "serviceaccount-env" || d.PublicKeyID != "publickey-env" ||
		d.ProjectID != "project-env" || d.PrivateKey == "" || d.APIEndpoint != "api.nebius.cloud:443" {
		t.Fatalf("credential env not applied: %+v", d)
	}
}

func TestSetConfigFromFlagsFlagBeatsEnv(t *testing.T) {
	t.Setenv("NEBIUS_SA_ID", "serviceaccount-env")
	d := NewDriver("node-1", "/tmp/store")
	if err := d.SetConfigFromFlags(fakeOptions{"nebius-sa-id": "serviceaccount-flag"}); err != nil {
		t.Fatal(err)
	}
	if d.SAID != "serviceaccount-flag" {
		t.Fatalf("SAID = %q, want flag value", d.SAID)
	}
}
