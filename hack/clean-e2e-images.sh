#!/usr/bin/env bash
# Delete e2e-* image tags older than 7 days from capcs-staging on quay.io.
# Requires QUAY_E2E_TOKEN environment variable (OAuth token with repo:admin scope).
# Set DRY_RUN=true to list tags without deleting them.

set -euo pipefail

REPO="cloudscalech/capcs-staging"
API="https://quay.io/api/v1/repository/${REPO}/tag/"
MAX_AGE_DAYS="${MAX_AGE_DAYS:-7}"
DRY_RUN="${DRY_RUN:-false}"

if [[ -z "${QUAY_E2E_TOKEN:-}" ]]; then
  echo "Error: QUAY_E2E_TOKEN environment variable is required" >&2
  exit 1
fi

cutoff=$(date -u -v-${MAX_AGE_DAYS}d +%s 2>/dev/null || date -u -d "${MAX_AGE_DAYS} days ago" +%s)

if [[ "${DRY_RUN}" == "true" ]]; then
  echo "DRY RUN: will list tags without deleting them"
fi
echo "Listing e2e-* tags older than ${MAX_AGE_DAYS} days..."

page=1
deleted=0
while true; do
  response=$(curl -s -H "Authorization: Bearer ${QUAY_E2E_TOKEN}" \
    "${API}?filter_tag_name=like:e2e-%25&limit=100&page=${page}")

  tags=$(echo "${response}" | jq -r '.tags // [] | .[] | select(.end_ts == null) | "\(.name) \(.start_ts)"')

  if [[ -z "${tags}" ]]; then
    break
  fi

  while IFS=' ' read -r name start_ts; do
    if [[ "${start_ts}" -lt "${cutoff}" ]]; then
      created=$(date -u -r "${start_ts}" 2>/dev/null || date -u -d "@${start_ts}")
      if [[ "${DRY_RUN}" == "true" ]]; then
        echo "Would delete tag: ${name} (created ${created})"
      else
        echo "Deleting tag: ${name} (created ${created})"
        curl -s -X DELETE -H "Authorization: Bearer ${QUAY_E2E_TOKEN}" "${API}${name}" > /dev/null
      fi
      deleted=$((deleted + 1))
    fi
  done <<< "${tags}"

  has_more=$(echo "${response}" | jq -r '.has_additional')
  if [[ "${has_more}" != "true" ]]; then
    break
  fi
  page=$((page + 1))
done

if [[ "${DRY_RUN}" == "true" ]]; then
  echo "Would delete ${deleted} e2e tag(s)."
else
  echo "Deleted ${deleted} e2e tag(s)."
fi
