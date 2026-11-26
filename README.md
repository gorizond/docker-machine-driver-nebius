# docker-machine-driver-nebius

Rancher node driver for Nebius AI Cloud (RKE2/K3s). Creates one Nebius instance per Rancher machine, delivers the SSH key via a minimal cloud-init, and lets Rancher install `rancher-system-agent` over SSH.

## Requirements

- Rancher 2.10+ (management cluster)
- Nebius project with a subnet, a security group allowing 22/6443/9345, and quota
- Service account **authorized key**: `saId` (`serviceaccount-*`), `publicKeyId` (`publickey-*`), private key in PKCS#8 PEM
- Image family (e.g. `ubuntu24.04-driverless`) or image id, platform and preset (e.g. `cpu-d3`, `4vcpu-16gb`)

## Install

1. Download the release: `docker-machine-driver-nebius_vX.Y.Z_linux_amd64.tar.gz` (binary at archive root).
2. Register the driver: `kubectl apply -f deploy/nodedriver.yaml` — set `spec.url` and `spec.checksum` (sha256).
3. Install the UI extension chart (gh-pages) — optional, but required for normal forms.
4. In Rancher: create a Nebius cloud credential, then a machine config, then add a node pool to an RKE2/K3s cluster.

## Flags

All flags are prefixed `--nebius-`. Required: `sa-id`, `public-key-id`, `private-key` or `private-key-path`, `project-id`, `subnet-id`, `platform`, `preset`, and exactly one of `image-family`/`image-id`.

Optional defaults: `disk-size-gib=50`, `disk-type=NETWORK_SSD`, `public-ip=static`, `ssh-user=ubuntu`, `ssh-port=22`, `create-timeout=20m`, `delete-timeout=5m`, `api-endpoint=api.nebius.cloud:443`, `security-groups`, `preemptible=false`, `labels`, `userdata`.

`--nebius-userdata` is a path to a `#cloud-config`. Rancher's custom-install-script path detects this flag, merges `--custom-install-script` into the file and passes the merged path; the driver then merges its SSH key and sends the result as `cloudInitUserData`. When the flag is set, the driver skips its own SSH wait (Rancher installs `rancher-system-agent` itself). Private-only mode (`public-ip=none`) requires `userdata`.

## Behavior

- **Create**: preflight (project, subnet, image, platform/preset, security groups) → adopt by deterministic name if it exists → otherwise create with a stable idempotency key and ownership labels → wait `RUNNING` → wait for SSH (skipped when `userdata` is set). Any failure after this call created the instance triggers a compensating delete.
- **Remove**: idempotent; `NotFound` counts as success; managed disks are deleted with the instance.
- **Power**: Start, Stop; Restart = stop → wait → start; Kill = stop (the public API has no hard-kill).
- **GetIP**: public IPv4; with `public-ip=none` and `userdata` it returns the private IPv4; other `public-ip=none` configurations fail explicitly.
- **Auth**: service account JWT → IAM token; the SDK refreshes the token automatically.

## Troubleshooting

| Symptom | Check |
|---|---|
| 401 | `saId` / `publicKeyId` / private key match; key is PKCS#8 |
| 403 | Service account has rights on the project/subnet |
| 429 | Rate limit: wait for the retry window, do not loop |
| No public IP | `public-ip=static` recommended; dynamic is released after a long stop |
| SSH timeout | Security group allows port 22; check `cloud-init status --long` on the node |
| Node not Ready | `rancher-system-agent` pod, `install.sh` reachability |

## Security

Private key and tokens stay in the Rancher credential and machine state; they are never written to logs or errors. The driver talks only to public Nebius API endpoints.

## Orphans

```bash
NEBIUS_SA_ID=... NEBIUS_PUBLIC_KEY_ID=... NEBIUS_PRIVATE_KEY_PATH=... NEBIUS_PROJECT_ID=... \
  scripts/verify-orphans.sh   # exit 0 = clean, 1 = orphans, 2 = config/API error
```

Spec and evidence: `0_WORK/specs/20261123-155910-nebius-node-driver/`.
