# Phase 1 · LV2 — Ariadne-Based Boilerplate and Task History

> Status: completed (2026-09-30)
> Scope: Establish a minimal Go/Operator foundation and task-history workflow in this documentation-first repository.

## Goal

Use Ariadne as a reference for Go module layout, `internal` package boundaries, and repeatable Make commands while following Louder's Kubernetes Operator architecture and Phase 1 scope. Record project work in `docs/task` and define the plan lifecycle in `AGENTS.md`.

## Dependencies

- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/secrets.md`
- `docs/cluster-platform.md`
- `docs/harness.md`
- `/Users/jinhyeokhong/dev/dragpass-control-plane/docs/exec-plans/README.md` and its execution-plan format

## Scope and non-goals

- Included: Go module, Operator entry point, Phase 1 CRD types, CloudAccount reconciler scaffold, shared Provider contract, Makefile, concise developer guide, task-history directories and workflow.
- Excluded: CSP API collection, detailed Collector scheduling, ClickHouse schema/connection, Analyzer/Teams implementation, Kubernetes deployment, and cluster changes.
- CRD fields and scaffolding follow existing architecture documents; do not add speculative settings.

## Work items

- [x] Review Ariadne's Go module, package layout, and Makefile conventions.
- [x] Define task plan/status rules in `AGENTS.md` and this guide; keep the current plan in `activate` during implementation.
- [x] Store the repository TDD skill at `skills/louder-tdd/SKILL.md` and link `.agents/skills/louder-tdd/SKILL.md` to it.
- [x] Use test-first Red → Green cycles for observable Operator/API behavior.
- [x] Repeat one behavior test, minimum implementation, and verification per code slice.
- [x] Add `go.mod`, `Makefile`, `.gitignore`, and developer guide.
- [x] Show the project introduction image at the top of `README.md`.
- [x] Add `CloudAccount`, `BudgetPolicy`, and `NotificationPolicy` types and scheme registration in `api/v1alpha1`.
- [x] Add a Manager entry point and minimal reconciler in `cmd/operator` and `internal/controller`; keep CSP collection out of reconciliation.
- [x] Add the provider-neutral contract in `internal/provider`.
- [x] Test generated CRD group/version and regenerate the manifests.
- [x] Test the BudgetPolicy amount and `forecast.enabled` schema against the architecture document.
- [x] Run and record build, manifest, test, vet, format, and skill validation checks.

## Completion criteria

- [x] The boilerplate builds.
- [x] API types generate CRD manifests.
- [x] Operator/reconciler does not call billing APIs.
- [x] No plaintext secrets or real credentials are included.
- [x] `AGENTS.md` and `docs/task/README.md` agree on task states and filenames.
- [x] Write task planning/history documents in English.
- [x] Record implementation, verification, unrun checks, and remaining gaps.

## Design decisions

- 2026-09-30: Reuse Ariadne's Go/module conventions only; do not copy its application features into the Operator.
- 2026-09-30: Pin controller-runtime to a Kubernetes 1.35-compatible release line.
- 2026-09-30: Initial providers are AWS, GCP, and Azure. Other CSPs remain follow-on work.
- 2026-09-30: Name plans `phase-{major}-lv{minor}-{short-kebab-case-description}.md`.
- 2026-09-30: Keep always-on rules in `AGENTS.md`; put the repository-specific TDD procedure in `skills/louder-tdd/SKILL.md` and expose it through a Codex discovery symlink.
- 2026-09-30: Write all `docs/task` plans and history records in English.
- 2026-09-30: This task-history workflow was introduced after initial scaffolding work began. The plan was created before continuing implementation; the earlier harness-document change is recorded separately as Phase 1 LV1.
- 2026-09-30: CRD inspection found the initial BudgetPolicy schema differed from the architecture example. A generated-schema test now guards integer amount values and an object-shaped `forecast.enabled` field.

## Risks and remaining gaps

- Update controller-runtime and controller-tools versions together with the target Kubernetes API version.
- The scaffold has no Operator Deployment or RBAC manifests. Do not deploy it until a namespace-scoped Role grants only required API access, including Secret `get` in the watched namespace.
- The cache defaults to the `cloud-cost` namespace. Actual deployment configuration must preserve this scope.
- Secret readiness checks only that the referenced Secret exists. Provider-specific key-shape validation belongs with the provider implementations.
- Collector, provider adapters, storage, Analyzer, and Notifier implementations are follow-on tasks.
- kind E2E and live CSP checks are not part of this boilerplate task.

## Verification record

### TDD cycles

- `TestReconcileReportsMissingCredentialSecret`: the first attempt stopped before test execution because the module checksum was unavailable; it did not count as RED. After dependencies were downloaded, `go test ./internal/controller -run TestReconcileReportsMissingCredentialSecret -count=1` failed because the condition was absent (RED). The minimal status update made it pass (GREEN).
- `TestReconcileReportsCredentialSecretReady`: failed because the condition was absent (RED). After setting the condition when the Secret exists, `go test ./internal/controller -run 'TestReconcileReports(MissingCredentialSecret|CredentialSecretReady)' -count=1` passed (GREEN).
- `TestGeneratedCRDsUseFinOpsAPIVersion`: failed because generated group/version were empty (RED). Moving the Kubebuilder group marker to `groupversion_info.go`, regenerating CRDs, and removing stale generated files made it pass (GREEN).
- `TestGeneratedBudgetPolicySchemaMatchesArchitecture`: failed because amount was generated as string and forecast as boolean (RED). Changing amount to `int64`, modeling forecast as `{enabled: bool}`, and regenerating CRDs made it pass (GREEN).

### Final checks

- `GOCACHE=/private/tmp/louder-go-build-cache make manifests` — passed; all three CRDs use group `finops.sre.local` and version `v1alpha1`.
- `GOCACHE=/private/tmp/louder-go-build-cache make test` — passed.
- `GOCACHE=/private/tmp/louder-go-build-cache make vet` — passed.
- `make fmt-check` — passed.
- `GOCACHE=/private/tmp/louder-go-build-cache make build` — passed.
- `python3 /Users/jinhyeokhong/.codex/skills/.system/skill-creator/scripts/quick_validate.py skills/louder-tdd` and symlink checks — passed.
- Reviewed all task documents for Korean characters — no matches; task documents are English.
- README image path `assets/introduce.png` — confirmed present and referenced before the title.
- kind E2E and live CSP checks — not run; deployment, RBAC, and provider implementations are out of scope.

## References

- Prior task: [Phase 1 LV1 — Harness Provider Scope and Notifier Coverage](../completed/phase-1-lv1-harness-scope-alignment.md).
- Architecture: `docs/architecture.md`, `docs/provider-contract.md`, `docs/secrets.md`, `docs/cluster-platform.md`, and `docs/harness.md`.
