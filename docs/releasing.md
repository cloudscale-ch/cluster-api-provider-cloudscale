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
3. **Verify the GitHub release** has all artifacts attached (manifests, checksums,
   signature, SBOM)
4. **Verify the signatures/attestations** — see [Verifying a Release](#verifying-a-release)
5. **Test installation** on a fresh management cluster.

See [Testing Releases](testing-releases.md) for detailed testing instructions.

## Verifying a Release

All release artifacts are signed with keyless [Sigstore](https://www.sigstore.dev/)
signatures using GitHub Actions OIDC. The signing
identity is this repository's release workflow at the release tag, i.e.
`https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/.github/workflows/release.yml@refs/tags/<tag>`
issued by `https://token.actions.githubusercontent.com`.

Requires [`cosign`](https://docs.sigstore.dev/cosign/system_config/installation/)
and the [GitHub CLI](https://cli.github.com/) (`gh`).

### Container image signature

```bash
IMG=quay.io/cloudscalech/cluster-api-cloudscale-controller:v1.0.0
cosign verify "$IMG" \
  --certificate-identity-regexp '^https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/\.github/workflows/release\.yml@refs/tags/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

### Build provenance & SBOM attestations

```bash
# SLSA provenance + SBOM attestation attached to the image:
gh attestation verify oci://$IMG --owner cloudscale-ch

# List everything attached to the image (signature + attestations):
cosign tree "$IMG"
```

The SBOM is also attached to the GitHub release as `sbom.spdx.json`.

### Release manifests

Download `checksums.txt`, `checksums.txt.bundle` and the
`*.yaml` manifests into the same directory, then (run from that directory):

```bash
# Verify the manifest digests. `shasum` ships by default on both macOS and Linux
# and reads the checksum file; GNU coreutils users can use `sha256sum -c` instead.
# (Do not use BSD `sha256sum -c` — there `-c` compares against a single digest string.)
shasum -a 256 -c checksums.txt

# The bundle is a self-describing Sigstore bundle carrying the signature,
# signing certificate, and transparency-log proof.
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp '^https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/\.github/workflows/release\.yml@refs/tags/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# Each manifest also carries a SLSA provenance attestation:
gh attestation verify infrastructure-components.yaml --owner cloudscale-ch
```
