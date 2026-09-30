# Phase 1 LV15: Isolated kind Cluster Names

## Goal

Allow local kind E2E runs to use a caller-selected cluster name so a pre-existing `louder-e2e` cluster does not block verification or get replaced.

## Scope

- Add a `KIND_CLUSTER_NAME` environment override to the kind E2E entry point, retaining `louder-e2e` as the default.
- Pass the selected name to all E2E helper scripts that build a kind context.
- Keep the refusal to replace a cluster with the selected name.
- Document the override and correct the harness description of the now-implemented AWS adapter.

## Non-goals

- Automatically delete, reuse, or mutate a pre-existing cluster.
- Change kind cluster topology, operator manifests, or production deployment configuration.
- Change AWS collection behavior or add live AWS calls to kind.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/harness.md`
- `docs/cluster-platform.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv14-aws-cost-explorer-collector.md`
- `skills/louder-tdd/SKILL.md`

## Work items

1. Reproduce that a requested custom name is ignored and the existing default cluster blocks the run (RED).
2. Implement the environment override and ensure every helper uses the selected kind context (GREEN).
3. Run the smoke E2E under a unique disposable cluster name and confirm cleanup affects only that name.
4. Update `docs/harness.md` to describe the actual AWS live adapter and kind cluster-name override.

## Completion criteria

- `KIND_CLUSTER_NAME=<unique-name> bash scripts/kind/e2e.sh` targets `kind-<unique-name>` throughout the flow.
- An existing cluster with another name is left untouched.
- A cluster already using the selected name still causes a refusal before any mutation.
- The default cluster name remains `louder-e2e` when the variable is unset.
- The smoke E2E completes successfully using a unique cluster name.
- All Go tests, shell syntax checks, formatting checks, and `git diff --check` pass.

## Design decisions and risks

- Keep the current stable default for compatibility; the override is opt-in.
- The script's cleanup trap deletes only the cluster it created, using the selected name.
- The existing harness paragraph claiming that live provider adapters are not implemented became stale after Phase 1 LV14 and will be corrected in this task.

## Verification plan

- Run the existing kind E2E with a custom name before the change and record the expected RED caused by the fixed default name.
- `KIND_CLUSTER_NAME=louder-e2e-lv15 bash scripts/kind/e2e.sh`
- `bash -n scripts/kind/*.sh`
- `GOCACHE=/tmp/louder-go-build make test`
- `GOCACHE=/tmp/louder-go-build make fmt-check`
- `git diff --check`

## Verification results

- RED: `KIND_CLUSTER_NAME=louder-e2e-lv15 bash scripts/kind/e2e.sh` used the fixed default and created `louder-e2e`, proving the requested override was ignored. The disposable cluster was cleaned up by the E2E script.
- GREEN: `KIND_CLUSTER_NAME=louder-e2e-lv15 GOCACHE=/tmp/louder-go-build make kind-e2e` created context `kind-louder-e2e-lv15`, passed the CloudAccount and fixture Collector smoke flow, and cleaned up the selected cluster.
- `KIND_CLUSTER_NAME=louder-e2e-lv15-secrets GOCACHE=/tmp/louder-go-build make kind-e2e-secrets` first exposed stale Secret-key JSONPaths in the Vault recovery helper. After updating those paths, the same command passed the Vault -> ESO -> Secret flow and outage/recovery scenario, then cleaned up the selected cluster.
- `bash -n scripts/kind/*.sh` passed.
- `GOCACHE=/tmp/louder-go-build make test` passed all Go tests.
- `GOCACHE=/tmp/louder-go-build make fmt-check` passed.
- `git diff --check` passed.
- A final search found no hard-coded `kind-louder-e2e` contexts, old AWS credential key names, or stale statement that live adapters are unimplemented in the changed harness areas.
