# Releasing

This document describes how to create a new release of the Cluster API Provider for cloudscale.ch.

## Version Numbering

Releases follow [Semantic Versioning](https://semver.org/). The current release series is defined in `metadata.yaml`:

- **0.1.x** — initial release series with `v1beta2` API contract

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
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```

4. **The release workflow runs automatically**

   Pushing a tag matching `v*.*.*` triggers `.github/workflows/release.yml`, which:
    - Builds and pushes a multi-arch Docker image (linux/amd64, linux/arm64) to
      `quay.io/cloudscalech/cluster-api-cloudscale-controller:<tag>`
    - Generates release manifests via `make release-manifests`
    - Creates a GitHub release with auto-generated release notes and the following artifacts:
        - `infrastructure-components.yaml` — all CRDs, RBAC, and controller deployment
        - `metadata.yaml` — clusterctl provider metadata
        - `cluster-template.yaml` — default workload cluster template

## Post-Release Verification

After the workflow completes:

1. **Check the GitHub Actions run** succeeded without errors
2. **Verify the container image** exists
   on [quay.io](https://quay.io/repository/cloudscalech/cluster-api-cloudscale-controller)
3. **Verify the GitHub release** has all 3 artifacts attached
4. **Test installation** on a fresh management cluster.

See [Testing Releases](testing-releases.md) for detailed testing instructions.
