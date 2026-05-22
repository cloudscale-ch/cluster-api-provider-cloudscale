#!/usr/bin/env bash
# Delete e2e-* image tags older than 7 days from capcs-staging on quay.io.
# Requires regctl to be installed and authenticated (via docker login or regctl registry login).
# If QUAY_E2E_USERNAME and QUAY_E2E_PASSWORD are set, the script logs in automatically.
# Set DRY_RUN=true to list tags without deleting them.

set -euo pipefail

REPO="quay.io/cloudscalech/capcs-staging"
MAX_AGE_DAYS="${MAX_AGE_DAYS:-7}"
DRY_RUN="${DRY_RUN:-false}"

if ! command -v regctl &>/dev/null; then
  echo "Error: regctl is not installed" >&2
  exit 1
fi

# Login if credentials are provided
if [[ -n "${QUAY_E2E_USERNAME:-}" && -n "${QUAY_E2E_PASSWORD:-}" ]]; then
  echo "${QUAY_E2E_PASSWORD}" | regctl registry login quay.io --user "${QUAY_E2E_USERNAME}" --pass-stdin
fi

# Calculate cutoff timestamp
cutoff=$(date -u -v-"${MAX_AGE_DAYS}"d +%s 2>/dev/null || date -u -d "${MAX_AGE_DAYS} days ago" +%s)

if [[ "${DRY_RUN}" == "true" ]]; then
  echo "DRY RUN: will list tags without deleting them"
fi
echo "Listing e2e-* tags older than ${MAX_AGE_DAYS} days..."

# List all tags and filter for e2e-* prefix
tags=$(regctl tag ls "${REPO}" | grep '^e2e-' || true)

if [[ -z "${tags}" ]]; then
  echo "No e2e-* tags found."
  exit 0
fi

deleted=0
while IFS= read -r name; do
  # Get the image creation timestamp in RFC3339 format using jsonPretty to ensure quotes and T/Z separators
  created=$(regctl image config "${REPO}:${name}" --format '{{jsonPretty .Created}}' 2>/dev/null | sed 's/^"//;s/"$//' || true)

  if [[ -z "${created}" ]]; then
    echo "Warning: could not get creation time for tag ${name}, skipping"
    continue
  fi

  # Convert to a format that date can parse (handle both RFC3339 and 'YYYY-MM-DD HH:MM:SS...' formats)
  normalized=$(echo "${created}" | sed 's/ /T/; s/ UTC$//')
  created_ts=$(date -u -jf "%Y-%m-%dT%H:%M:%S" "${normalized%.*}" +%s 2>/dev/null \
    || date -u -d "${normalized}" +%s 2>/dev/null \
    || date -u -d "${created}" +%s 2>/dev/null \
    || true)

  if [[ -z "${created_ts}" ]]; then
    echo "Warning: could not parse creation time '${created}' for tag ${name}, skipping"
    continue
  fi

  if [[ "${created_ts}" -lt "${cutoff}" ]]; then
    created_human=$(date -u -r "${created_ts}" 2>/dev/null || date -u -d "@${created_ts}")
    if [[ "${DRY_RUN}" == "true" ]]; then
      echo "Would delete tag: ${name} (created ${created_human})"
    else
      echo "Deleting tag: ${name} (created ${created_human})"
      regctl tag delete "${REPO}:${name}"
    fi
    deleted=$((deleted + 1))
  fi
done <<< "${tags}"

if [[ "${DRY_RUN}" == "true" ]]; then
  echo "Would delete ${deleted} e2e tag(s)."
else
  echo "Deleted ${deleted} e2e tag(s)."
fi
