# Phase 1 LV22: Route Budget Threshold Notifications

## Goal

Connect stored monthly budget evaluation to the provider-neutral Notifier interface and deliver one notification for each reached threshold intent.

## Scope

- Add an Analyzer orchestration function that evaluates a BudgetPolicy using the storage CostReader and sends returned threshold intents through a Notifier.
- Map each intent to a stable `BudgetThreshold` notification with title, summary, severity, spend, budget, and currency details.
- Preserve deterministic threshold order and stop at the first notifier error.
- Avoid notifier calls when no threshold is reached.
- Test the behavior through public Analyzer, CostReader, and FakeNotifier interfaces.
- Document that scheduler, NotificationPolicy lookup, retry, and deduplication are still absent.

## Non-goals

- Teams-specific payload or HTTP changes.
- Kubernetes controller or CronJob deployment.
- NotificationPolicy lookup or event routing.
- Delivery retries, per-threshold deduplication, and persisted notification state.
- Additional budget calculations or currency conversion.

## Dependencies and prior documents

- `AGENTS.md`
- `SECURITY.md`
- `docs/architecture.md`
- `docs/secrets.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv20-stored-budget-evaluation.md`
- `docs/task/completed/phase-1-lv21-teams-workflow-notifier.md`
- `skills/louder-tdd/SKILL.md`

## Work items

1. Add a RED behavior test proving a reached budget threshold becomes a FakeNotifier notification with the expected provider-neutral payload.
2. Add a RED behavior test proving no reached threshold sends no notification.
3. Add a RED behavior test proving a notifier failure stops subsequent sends and is propagated.
4. Implement the minimal Analyzer-to-Notifier orchestration and payload mapping.
5. Document the callable routing path and remaining runtime responsibilities.

## Completion criteria

- The service reuses `EvaluateStoredBudget` and sends one notification per returned intent in ascending threshold order.
- Notification type, severity, title, summary, spend, budget, and currency are deterministic.
- No threshold intent produces no Notifier call.
- A notifier error is returned and no later intent is sent after that error.
- Tests use only fakes and never call a real Teams endpoint.
- `make test`, `make vet`, `make fmt-check`, and `git diff --check` pass.

## Design decisions and risks

- Use the provider-neutral event type `BudgetThreshold` and severity `Warning` for all configured thresholds; do not invent a separate severity policy at 100% in this slice.
- Send each reached threshold as an independent notification to preserve current evaluator semantics.
- A future scheduled runtime must add notification policy resolution and persisted deduplication before repeated evaluation is enabled.

## Verification plan

- Run each focused behavior test immediately after adding it and capture genuine RED/GREEN results.
- `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer ./internal/notifier -count=1`
- `GOCACHE=/tmp/louder-go-build make test`
- `GOCACHE=/tmp/louder-go-build make vet`
- `GOCACHE=/tmp/louder-go-build make fmt-check`
- `git diff --check`

## Verification results

- RED: `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer -run TestEvaluateAndNotifyBudgetSendsReachedThreshold -count=1` failed to compile because the Analyzer-to-Notifier service did not exist.
- GREEN: Focused routing tests pass for reached threshold payload, no notification below threshold, and stopping after the first delivery failure.
- GREEN: `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer ./internal/notifier -count=1` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make test` passed all packages.
- GREEN: `GOCACHE=/tmp/louder-go-build make vet` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make fmt-check` passed.
- GREEN: `git diff --check` passed.
