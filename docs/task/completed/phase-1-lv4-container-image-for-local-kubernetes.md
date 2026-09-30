# Phase 1 · LV4 — Container Image for Local Kubernetes

> Status: completed (2026-09-30)
> Scope: Add a reproducible non-root Operator container image for local Kubernetes runs and later kind E2E work.

## Goal

Make the Operator buildable as a small runtime image so a future kind harness can load and run it without relying on a developer-specific image setup.

## Work items

- [x] Add a multi-stage Dockerfile that builds the existing Go entry point and copies only the binary into a non-root runtime image.
- [x] Add a `.dockerignore` that excludes local build output and repository metadata from the build context.
- [x] Add a Make target for building a locally tagged image.
- [x] Document the local image build command and note that kind E2E still requires deployment/RBAC manifests and cluster setup.
- [x] Build the image locally and inspect the resulting image configuration.
- [x] Record verification and remaining kind E2E prerequisites.

## Acceptance criteria

- The image contains the compiled Operator binary and no source tree or Go toolchain.
- The runtime uses a non-root user and starts the existing Operator entry point.
- A local image can be built using a documented Make target.
- The task does not add or imply deployment readiness; namespace-scoped RBAC remains a prerequisite.

## Verification

- Run the Docker build target.
- Inspect the image entrypoint and configured user.
- Run the existing Go test suite if any source code changes are needed; this task is expected to change build configuration only.
- Run `git diff --check`.

### Results

- `make docker-build` — passed; built `louder-operator:local` for `linux/arm64`.
- `docker image inspect louder-operator:local` — confirmed user `65532:65532` and entrypoint `/louder-operator`.
- Go source was unchanged, so the Go test suite was not rerun for this Docker-only change.
- kind E2E is still unavailable until the repository has installation and namespace-scoped RBAC manifests plus kind lifecycle/test wiring.

## References

- `docs/cluster-platform.md` — internal image and non-root runtime expectations.
- `SECURITY.md` — container security expectations.
- `docs/harness.md` — planned kind E2E flow.
