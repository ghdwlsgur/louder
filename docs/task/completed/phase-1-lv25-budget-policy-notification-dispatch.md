# Phase 1 Level 25: Budget Policy Notification Dispatch

## Goal

Send reached budget threshold notifications through matching `NotificationPolicy` destinations, using an injected resolver to keep Kubernetes Secret access outside the Analyzer domain service.

## Scope

- Add a budget notification service that evaluates stored cost once, selects accounts covered by the `BudgetPolicy`, and selects matching NotificationPolicies for each event.
- Resolve a `notifier.Notifier` for each selected policy through an injected function and fan out every threshold notification.
- Return a clear error if thresholds are reached but no usable matching destination exists; stop on resolver or delivery errors.
- Add tests for account intersection, fanout, no threshold/no lookup, missing route, resolver errors, and delivery errors.
- Update architecture and harness documentation.

## Non-goals

- Kubernetes API client or Secret lookup implementation.
- Analyzer command, CronJob scheduling, or Operator reconciliation changes.
- Retry, delivery deduplication, or notification status persistence.
- Changes to the existing single-Notifier `EvaluateAndNotifyBudget` API.

## Dependencies and prior documents

- `docs/architecture.md`
- `docs/secrets.md`
- `SECURITY.md`
- `docs/harness.md`
- `docs/task/completed/phase-1-lv22-budget-notification-routing.md`
- `docs/task/completed/phase-1-lv24-notification-policy-event-routing.md`

## Design decisions

- The Analyzer receives a resolver callback keyed by `NotificationPolicy`; it never reads Secrets itself.
- Relevant accounts are the intersection of accounts matching the BudgetPolicy selector and accounts matching each NotificationPolicy selector.
- The service evaluates once, resolves each selected policy once per run, then sends each reached threshold to each selected destination in deterministic policy order.
- If reached thresholds have no matching destination, return `ErrNotifierRequired` to make missing routing configuration visible.
- Fail on the first resolver or delivery error; retry and deduplication are outside this slice.

## Work items

1. Add one public-service test for matching policy fanout; run and observe RED.
2. Implement the minimum service and resolver interface; confirm GREEN.
3. Add each failure and no-op test one at a time, running it before proceeding.
4. Update architecture and harness documentation.
5. Run focused and full repository verification.

## Completion criteria

- Only accounts selected by both the BudgetPolicy and NotificationPolicy can route a budget event.
- Each reached threshold reaches all matching destinations, with deterministic destination order.
- No notification lookup occurs when no threshold is reached.
- Missing destination, resolver failure, and delivery failure are observable and tested.
- Resolver errors do not expose Secret contents; no Kubernetes Secret read is added to Analyzer.
- Verification results and checks not run are recorded below.

## Risks and open questions

- Repeated executions can resend thresholds until deduplication is added.
- Runtime has no Analyzer command or schedule yet; this service is a tested domain seam, not a running workload.

## Verification

- RED: `GOCACHE=/tmp/louder-go-cache go test ./internal/analyzer -run '^TestEvaluateAndNotifyBudgetPoliciesSendsToMatchingDestinations$'` failed because the policy dispatch service did not exist.
- GREEN: the same focused test passed after implementing the service.
- Focused selector and dispatch tests passed: `GOCACHE=/tmp/louder-go-cache go test ./internal/analyzer -run '^(TestSelectNotificationPolicies|TestEvaluateAndNotifyBudgetPolicies)' -count=1`.
- `GOCACHE=/tmp/louder-go-cache make test` — passed for all packages.
- `GOCACHE=/tmp/louder-go-cache make vet` — passed.
- `make fmt-check` — passed.
- `git diff --check` — passed.
- Kind E2E was not run; this change adds Analyzer service behavior and does not add or alter a Kubernetes workload.

## Completion record

- Budget threshold intents are sent only through event-subscribed NotificationPolicies whose selectors match accounts already selected by the BudgetPolicy.
- All matching policies are resolved in deterministic name order before any sends; each intent is sent to every selected destination.
- Resolver errors are replaced with a stable error that excludes the underlying cause, protecting credential details.
- Kubernetes Secret resolution, scheduling, retries, and deduplication remain separate runtime work.
