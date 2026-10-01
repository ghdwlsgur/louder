# Phase 1 Level 24: NotificationPolicy Event Routing

## Goal

Define explicit event subscriptions on `NotificationPolicy` and provide deterministic policy selection for events based on event type and matching CloudAccount metadata.

## Scope

- Add supported event types to `NotificationPolicy.spec.events` and regenerate the CRD.
- Deep-copy the event list in generated API object copies.
- Add a pure Analyzer selector that returns only Teams policies subscribed to the event and matching at least one relevant account.
- Document event subscription and selector semantics.

## Non-goals

- Scheduling Analyzer runs or resolving Kubernetes Secrets.
- Sending notifications through multiple resolved `NotificationPolicy` destinations.
- Retry, delivery status, or deduplication.
- Adding event producers beyond the existing budget threshold event.

## Dependencies and prior documents

- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/secrets.md`
- `docs/harness.md`
- `docs/task/completed/phase-1-lv22-budget-notification-routing.md`
- `docs/task/completed/phase-1-lv23-kind-analyzer-notifier-integration.md`

## Design decisions

- Supported event names are `DailySummary`, `BudgetWarning`, `BudgetExceeded`, `BudgetThreshold`, `CostAnomaly`, and `CollectionFailed`; `BudgetThreshold` is the event emitted by the current Analyzer.
- `spec.events` is required. A policy must opt into every event it should receive.
- A policy matches only when its type is `teams`, the event is listed, and at least one relevant CloudAccount's metadata contains every selector key/value. An empty selector matches any supplied account; an empty account set never matches.
- Selection order is deterministic by policy name.
- This slice exposes pure selection for later runtime wiring; it does not resolve credentials or send to selected policies.

## Work items

1. Add an Analyzer test for matching event subscription and selector; confirm it fails before implementation.
2. Implement the minimal selector and confirm the test passes.
3. Add one test at a time for wrong event, unmatched selector, empty selector/account behavior, and deterministic ordering.
4. Add `events` to the API type, deep-copy behavior, generated schema, and API schema tests.
5. Update architecture and secret examples with event and selector semantics.
6. Run focused tests and repository checks.

## Completion criteria

- Event subscription and metadata selection tests pass.
- CRD schema requires `events` and limits entries to supported event names.
- Deep copies do not share the event backing array.
- Documentation examples use the supported current `BudgetThreshold` event.
- Verification results and any checks not run are recorded below.

## Risks and open questions

- Requiring `events` means existing NotificationPolicy manifests need the field before applying the regenerated CRD. This API is still v1alpha1 and there are no production routing consumers yet.
- Runtime Secret resolution and delivery to selected policies remain a separate task.

## Verification

- RED: `GOCACHE=/tmp/louder-go-cache go test ./internal/analyzer -run '^TestSelectNotificationPoliciesMatchesSubscribedEventAndAccountMetadata$'` failed to compile because the `Events` field and selector were absent.
- RED: `GOCACHE=/tmp/louder-go-cache go test ./api/v1alpha1 -run '^TestGeneratedNotificationPolicySchemaRequiresSupportedEvents$'` failed because the generated CRD had no `spec.events` schema.
- GREEN: focused Analyzer and API tests passed after implementation.
- `GOCACHE=/tmp/louder-go-cache make test` — passed for all packages.
- `GOCACHE=/tmp/louder-go-cache make vet` — passed.
- `make fmt-check` — passed.
- `GOCACHE=/tmp/louder-go-cache make manifests` — passed; generated schema includes required event subscriptions and supported values.
- `git diff --check` — passed.
- Kind E2E was not run; this change adds a pure selection function and CRD schema without changing runtime deployment wiring.

## Completion record

- `NotificationPolicy.spec.events` is required, non-empty, and limited to the supported event names.
- Analyzer selection requires a Teams destination, matching event, and at least one relevant account matching the selector; selected policies are ordered by name.
- API deep-copy behavior and documentation examples are updated.
- Secret resolution and delivery through selected policies remain future runtime work.
