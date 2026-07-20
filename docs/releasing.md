# Releasing

This document describes how to create a new release of the Cluster API Provider for cloudscale.ch.

## Version Numbering

Releases follow [Semantic Versioning](https://semver.org/). The current release series is defined in `metadata.yaml`:

- **1.0.x** — initial stable release series with `v1beta2` API contract

When introducing a new API version or breaking changes, add a new release series to `metadata.yaml`.

## How to Cut a Release

1. **Ensure `main` is in a releasable state**
    - CI checks are passing
    - E2E tests are green (check recent nightly/weekly runs)
    - All intended changes are merged

2. **Determine the version number**
    - Follow the current release series in `metadata.yaml`
    - Use patch bumps for bug fixes, minor bumps for new features

3. **Create and push a version tag**
   ```bash
   git checkout main
   git pull origin main
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

4. **The release workflow runs automatically**

   Pushing a tag matching `v*.*.*` triggers `.github/workflows/release.yml`, which:
    - Gates the release on `make govulncheck` (fails on known, reachable vulnerabilities)
    - Builds and pushes a Docker image (linux/amd64) to
      `quay.io/cloudscalech/cluster-api-cloudscale-controller:<tag>`
    - Signs the image with keyless [cosign](https://docs.sigstore.dev/) and attaches
      SLSA build-provenance and an SPDX SBOM as signed attestations
    - Generates release manifests via `make release-manifests`, a `checksums.txt`
      signed with `cosign sign-blob`, and SLSA provenance for the manifest files
    - Creates a GitHub release with auto-generated release notes and the following artifacts:
        - `infrastructure-components.yaml` — all CRDs, RBAC, and controller deployment
        - `metadata.yaml` — clusterctl provider metadata
        - `cluster-template*.yaml` / `cluster-class*.yaml` — workload cluster templates
        - `checksums.txt`, `checksums.txt.bundle` — manifest checksums + Sigstore bundle (signature + certificate)
        - `sbom.spdx.json` — SBOM of the controller image

## Post-Release Verification

After the workflow completes:

1. **Check the GitHub Actions run** succeeded without errors
2. **Verify the container image** exists
   on [quay.io](https://quay.io/repository/cloudscalech/cluster-api-cloudscale-controller)
3. **Verify the GitHub release** has all artifacts attached (manifests, checksums with bundle,
   SBOM, release attestation)
4. **Test installation** on a fresh management cluster.

See [Testing Releases](testing-releases.md) for detailed testing instructions.

Optionally, the release artifacts can be verified using the section below. This is optional because during release
the verification is already done.

## Verifying a Release

All release artifacts are signed with keyless [Sigstore](https://www.sigstore.dev/)
signatures using GitHub Actions OIDC. The signing
identity is this repository's release workflow at the release tag, i.e.
`https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/.github/workflows/release.yml@refs/tags/<tag>`
issued by `https://token.actions.githubusercontent.com`.

Requires [`cosign`](https://docs.sigstore.dev/cosign/system_config/installation/)
and the [GitHub CLI](https://cli.github.com/) (`gh`).

```bash
export TAG=v1.0.0
export IMG=quay.io/cloudscalech/cluster-api-cloudscale-controller:$TAG
export ID_REGEXP='^https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/\.github/workflows/release\.yml@refs/tags/'
export ISSUER=https://token.actions.githubusercontent.com
```

### Container image signature

`cosign sign` pushes the signature into the registry alongside the image itself, not to a separate file. We can verify
the signature using `cosign verify`.

```bash
cosign verify "$IMG" \
    --certificate-identity-regexp "$ID_REGEXP" \
    --certificate-oidc-issuer "$ISSUER"
```

### Build provenance & SBOM attestations

```bash
# Build provenance (predicate type slsa.dev/provenance/v1, this happens to be
# gh's default if --predicate-type is omitted, but named explicitly here anyway)
gh attestation verify oci://$IMG --owner cloudscale-ch \
  --predicate-type https://slsa.dev/provenance/v1

# The provenance claim's actual content: which commit/workflow run produced
# this image (buildDefinition, runDetails.builder.id):
gh attestation verify oci://$IMG --owner cloudscale-ch \
  --predicate-type https://slsa.dev/provenance/v1 \
  --format json \
  --jq '.[].verificationResult.statement.predicate'

# SBOM (predicate type spdx.dev/Document/v2.3, must be given explicitly,
# otherwise gh defaults to provenance and the SBOM is never checked)
gh attestation verify oci://$IMG --owner cloudscale-ch \
  --predicate-type https://spdx.dev/Document/v2.3

# A verified copy of the SBOM's actual content, extracted from the checked
# attestation rather than trusted from the release asset on its own:
gh attestation verify oci://$IMG --owner cloudscale-ch \
  --predicate-type https://spdx.dev/Document/v2.3 \
  --format json \
  --jq '.[].verificationResult.statement.predicate'

# Everything attached at once:
cosign tree "$IMG"
```

`sbom.spdx.json` is also attached to the GitHub release directly, but that copy isn't
independently verifiable as a standalone file: its attestation's subject is the image,
not the file. The SBOM extraction command above prints the verified content instead.

### Release manifests

Unlike the image, these are plain files with no registry to push into, so
`cosign sign-blob` produces `checksums.txt.bundle` as a standalone file instead. Both
checks below matter: `shasum` proves your files match `checksums.txt`; the signature
proves `checksums.txt` itself is genuine. Either alone isn't enough.

```bash
cd "$(mktemp -d)"
gh release download "$TAG" \
  -R cloudscale-ch/cluster-api-provider-cloudscale \
  -p 'checksums.txt' \
  -p 'checksums.txt.bundle' \
  -p '*.yaml'

# Files match their checksums. `shasum` ships by default on macOS and Linux;
# GNU coreutils users can use `sha256sum -c` instead (BSD sha256sum's -c compares
# against a single digest string, not a checksum file).
shasum -a 256 -c checksums.txt

# checksums.txt itself is signed (predicate type sigstore.dev/cosign/sign/v1). The
# bundle is a self-describing Sigstore bundle: signature + certificate +
# transparency-log proof.
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp "$ID_REGEXP" \
  --certificate-oidc-issuer "$ISSUER" \
  checksums.txt

# Each manifest also carries its own build provenance (predicate type
# slsa.dev/provenance/v1, gh's default, named explicitly here anyway):
for f in *.yaml; do
  gh attestation verify "$f" --owner cloudscale-ch \
    --predicate-type https://slsa.dev/provenance/v1
done
```
