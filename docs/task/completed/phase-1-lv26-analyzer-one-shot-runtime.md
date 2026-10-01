# Phase 1 Level 26: Analyzer One-Shot Runtime

## Goal

Add a runnable Analyzer command that evaluates one namespaced BudgetPolicy against ClickHouse and delivers reached thresholds through matching NotificationPolicies and Kubernetes Secrets.

## Scope

- Add an Analyzer runtime entry point that reads one BudgetPolicy, same-namespace CloudAccounts, and NotificationPolicies through a direct Kubernetes client.
- Resolve each selected policy's Secret on demand and create a Teams notifier from `TEAMS_WEBHOOK_URL` without logging credential data.
- Add `cmd/analyzer` flags for namespace and BudgetPolicy name; open ClickHouse from the existing environment configuration and run once.
- Include the Analyzer executable in local builds and the runtime image.
- Test Kubernetes-object loading, Secret resolution, safe missing-key errors, and end-to-end service invocation with fake readers and notifiers.
- Document the run-once invocation and required Secret environment.

## Non-goals

- CronJob scheduling or controller reconciliation.
- Persisted notification deduplication or retry policy.
- Changes to the NotificationPolicy API or the existing dispatch semantics.
- Live Kubernetes, ClickHouse, or Teams credentials in unit tests.

## Dependencies and prior documents

- `docs/architecture.md`
- `docs/secrets.md`
- `docs/harness.md`
- `SECURITY.md`
- `docs/task/completed/phase-1-lv24-notification-policy-event-routing.md`
- `docs/task/completed/phase-1-lv25-budget-policy-notification-dispatch.md`

## Design decisions

- The command runs exactly one BudgetPolicy and exits, making it suitable for a future Kubernetes Job or CronJob.
- All Kubernetes reads are namespace-scoped and use a direct client rather than a cache, so Secret values are never stored in the Operator cache.
- The Analyzer process reads the ClickHouse Secret through `envFrom`; Teams webhook credentials are fetched by referenced Secret only after policy selection.
- Missing or malformed Teams Secret data produces a stable error without returning the secret value.
- Scheduling and deduplication are deferred together so repeated execution is not enabled before suppression semantics are defined.

## Work items

1. Add one runtime test for reading a BudgetPolicy and dispatching with an injected notifier factory; observe RED.
2. Implement the minimum runtime service and confirm GREEN.
3. Add tests one behavior at a time for namespace scoping, absent resources, and Secret key validation.
4. Add the CLI, local build target, and image binary.
5. Update runtime and secret documentation.
6. Run focused tests and repository checks.

## Completion criteria

- The command requires namespace and BudgetPolicy name and performs one evaluation.
- Kubernetes reads are restricted to that namespace.
- The cost reader and Teams delivery use existing interfaces.
- Secret values are never included in returned errors or logs.
- The Analyzer binary is included in `make build` and the container image.
- Verification commands and any checks not run are recorded below.

## Risks and open questions

- This run-once command can resend a threshold if invoked repeatedly. It must not be scheduled until a persisted deduplication design is implemented.
- The Kubernetes identity running this command will need namespace-scoped read access to BudgetPolicies, CloudAccounts, NotificationPolicies, and referenced Secrets; deployment RBAC is part of the scheduling task.

## Verification

- RED: `GOCACHE=/tmp/louder-go-cache go test ./internal/analyzer -run '^TestRunBudgetPolicyLoadsNamespaceResourcesAndSends$'` failed because the one-shot runtime did not exist.
- GREEN: the focused runtime test passed after implementation.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/analyzer -run '^TestRunBudgetPolicy(DoesNotUseAccountsFromOtherNamespaces|SanitizesMissingWebhookSecretKey|ReportsMissingBudgetPolicy)$'` — passed.
- `GOCACHE=/tmp/louder-go-cache make test` — passed for all packages.
- `GOCACHE=/tmp/louder-go-cache make vet` — passed.
- `make fmt-check` and `git diff --check` — passed.
- `GOCACHE=/tmp/louder-go-cache make build` — passed; builds operator, collector, and analyzer binaries.
- `GOCACHE=/tmp/louder-go-cache make docker-build IMAGE=louder-analyzer-runtime:local` — passed for the ARM64 image.
- `docker run --rm --entrypoint /louder-analyzer louder-analyzer-runtime:local --help` — passed and displayed Analyzer flags.
- Kind E2E was not run; this slice adds a one-shot executable but no Job/CronJob manifest or ServiceAccount RBAC.

## Completion record

- `cmd/analyzer` evaluates exactly one namespaced BudgetPolicy and exits.
- The runtime reads same-namespace resources with a direct client and fetches webhook Secrets only after a matching threshold destination is selected.
- ClickHouse connection configuration is read from the existing environment contract; Teams delivery uses the existing HTTPS-only notifier.
- The image includes `/louder-analyzer`; the Makefile builds `bin/louder-analyzer`.
- Scheduling and persisted notification deduplication remain required before recurring execution.
