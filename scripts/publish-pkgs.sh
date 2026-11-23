#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

UPSTREAM_PUBLISH="./node_modules/@rancher/shell/scripts/extension/publish"

bash "${UPSTREAM_PUBLISH}" "$@"

PKG="nebius-node-driver"
TEMPLATE_DIR="pkg/${PKG}/chart-templates"
TEMPLATE_SRC="${TEMPLATE_DIR}/nodedriver.yaml"
CHART_SRC="${TEMPLATE_DIR}/Chart.yaml"

if [[ ! -f "${TEMPLATE_SRC}" ]]; then
  echo "WARN: ${TEMPLATE_SRC} not found; skipping NodeDriver chart injection" >&2
  exit 0
fi

if [[ ! -d "charts/${PKG}" ]]; then
  # Nothing to patch (e.g. publish did not build anything)
  exit 0
fi

found_chart="false"
for chart_dir in "charts/${PKG}/"*; do
  [[ -d "${chart_dir}" ]] || continue
  found_chart="true"

  version="$(basename "${chart_dir}")"
  values_file="${chart_dir}/values.yaml"

  echo "Injecting NodeDriver into Helm chart: ${chart_dir}" >&2

  mkdir -p "${chart_dir}/templates"
  cp -f "${TEMPLATE_SRC}" "${chart_dir}/templates/nodedriver.yaml"

  if [[ -f "${chart_dir}/Chart.yaml" ]]; then
    yq -i '
      .annotations."catalog.cattle.io/rancher-version" = ">= 2.10.0" |
      .annotations."catalog.cattle.io/ui-extensions-version" = ">= 3.0.0 < 4.0.0"
    ' "${chart_dir}/Chart.yaml"
  elif [[ -f "${CHART_SRC}" ]]; then
    cp -f "${CHART_SRC}" "${chart_dir}/Chart.yaml"
  fi

  if [[ ! -f "${values_file}" ]]; then
    : > "${values_file}"
  fi

  yq -i '
    .nodeDriver.enabled = true |
    .nodeDriver.name = "nebius" |
    .nodeDriver.displayName = "Nebius AI Cloud" |
    .nodeDriver.description = "Nebius AI Cloud node driver" |
    .nodeDriver.active = true |
    .nodeDriver.addCloudCredential = true |
    .nodeDriver.builtin = false |
    .nodeDriver.checksum = "" |
    .nodeDriver.url = "" |
    .nodeDriver.versionOverride = "0.0.1" |
    .nodeDriver.publicCredentialFields = "projectId,apiEndpoint" |
    .nodeDriver.privateCredentialFields = "saId,publicKeyId,privateKey" |
    .nodeDriver.optionalCredentialFields = "projectId,apiEndpoint" |
    .nodeDriver.passwordFields = "privateKey" |
    .nodeDriver.defaults = "apiEndpoint:api.nebius.cloud:443" |
    .nodeDriver.fileToFieldAliases = "privateKey:privateKeyPath" |
    .nodeDriver.externalId = "" |
    .nodeDriver.whitelistDomains = ["api.nebius.cloud", "auth.eu.nebius.com"]
  ' "${values_file}"

  if [[ -d "assets" ]]; then
    mkdir -p "assets/${PKG}"
    rm -f "assets/${PKG}/${PKG}-${version}.tgz"
    helm package "${chart_dir}" -d "assets/${PKG}" > /dev/null
  fi
done

if [[ "${found_chart}" != "true" ]]; then
  exit 0
fi

if [[ -d "assets" ]]; then
  merge_args=()
  if [[ -f "assets/index.yaml" ]]; then
    merge_args+=(--merge "assets/index.yaml")
  elif [[ -f "index.yaml" ]]; then
    merge_args+=(--merge "index.yaml")
  fi

  helm repo index assets --url assets/ "${merge_args[@]}" > /dev/null
  cp -f assets/index.yaml index.yaml
fi

# The GH reusable workflow uploads ./tmp; rebuild it so it includes our patched chart + re-packed assets.
if [[ -d "tmp" ]]; then
  rm -rf tmp
  mkdir -p tmp/{assets,charts,extensions}

  if [[ -d "assets" ]]; then
    cp -R assets/* tmp/assets/
  fi

  if [[ -d "charts" ]]; then
    cp -R charts/* tmp/charts/
  fi

  if [[ -d "extensions" ]]; then
    cp -R extensions/* tmp/extensions/
  fi

  if [[ -f "index.yaml" ]]; then
    cp -f index.yaml tmp/index.yaml
  fi
fi
