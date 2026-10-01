# Phase 1 LV21: Teams Workflow Webhook Notifier

## Goal

Add a provider-neutral notification interface and a Microsoft Teams Workflows webhook implementation that sends a safe Adaptive Card payload.

## Scope

- Define a small `Notification` value and `Notifier` interface.
- Add a fake notifier for Analyzer tests and a Teams webhook notifier for delivery.
- Require a valid HTTPS webhook URL and prevent redirects from forwarding the URL secret.
- Send an Adaptive Card message with deterministic detail ordering.
- Sanitize request and non-success HTTP errors so webhook URLs and response bodies never appear in returned errors.
- Test HTTP success, non-success, redirect, and invalid URL behavior using an in-process test server only.
- Document the `TEAMS_WEBHOOK_URL` Secret key and current Workflows webhook expectation.

## Non-goals

- Calling a real Teams tenant or provisioning a Teams Workflow.
- Kubernetes deployment, secret mounting, notification-policy routing, or runtime scheduling.
- Notification delivery retries, rate limiting, or duplicate suppression.
- Adaptive Card actions, mentions, images, or rich domain-specific budget layouts.

## Dependencies and prior documents

- `AGENTS.md`
- `SECURITY.md`
- `docs/architecture.md`
- `docs/secrets.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv20-stored-budget-evaluation.md`
- `skills/louder-tdd/SKILL.md`
- Microsoft Teams Workflows webhook and Adaptive Card documentation.

## Work items

1. Add a RED behavior test proving `Send` posts a valid Adaptive Card envelope and accepts a successful response.
2. Add a RED behavior test proving HTTP failures return a safe error without exposing webhook URL or response content.
3. Add a RED behavior test proving redirects are not followed and non-HTTPS/malformed webhook URLs are rejected.
4. Add the minimal notification types, fake notifier, Teams notifier, and payload encoder needed for those behaviors.
5. Document the expected namespace-local Teams Secret key and delivery limits.

## Completion criteria

- `Notifier.Send` accepts a provider-neutral notification value.
- Teams requests use `POST` with `application/json` and an Adaptive Card envelope.
- A successful 2xx response is treated as delivered; other responses return a stable sanitized error.
- Only HTTPS URLs without userinfo or fragments are accepted.
- Redirects are not followed, preventing accidental forwarding of the secret URL.
- Tests never make an external request and verify no webhook URL or response body appears in errors.
- `make test`, `make vet`, `make fmt-check`, and `git diff --check` pass.

## Design decisions and risks

- Use the Teams Workflows webhook and Adaptive Card format for new setup because Microsoft documents legacy Microsoft 365 Connectors as nearing deprecation.
- Store the complete workflow callback URL under `TEAMS_WEBHOOK_URL` in the Kubernetes Secret referenced by `NotificationPolicy`.
- Accept any syntactically valid HTTPS host because workflow callback hostnames may vary by tenant and region. The URL is treated as a secret and must come from trusted Vault/ESO configuration.
- Do not include request URLs, status response bodies, or transport errors in returned errors; diagnostics use one stable delivery error.
- Workflows are associated with their owners; teams must operationally ensure a durable workflow owner as described by Microsoft.

## Verification plan

- Run each focused notifier test immediately after adding it and capture genuine RED/GREEN results.
- `GOCACHE=/tmp/louder-go-build go test ./internal/notifier ./internal/analyzer -count=1`
- `GOCACHE=/tmp/louder-go-build make test`
- `GOCACHE=/tmp/louder-go-build make vet`
- `GOCACHE=/tmp/louder-go-build make fmt-check`
- `git diff --check`

## Verification results

- RED: `GOCACHE=/tmp/louder-go-build go test ./internal/notifier -run TestTeamsWebhookSendsAdaptiveCard -count=1` initially failed to compile because the notifier API did not exist.
- RED: `GOCACHE=/tmp/louder-go-build go test ./internal/notifier -run 'TestTeamsWebhook' -count=1` failed when a URL fragment was accepted by the initial URL validation.
- GREEN: Focused notifier tests pass for Adaptive Card request shape, deterministic detail ordering, HTTP and transport error sanitization, redirect rejection, invalid URLs, and FakeNotifier behavior. Tests use an in-process RoundTripper and make no network calls.
- GREEN: `GOCACHE=/tmp/louder-go-build go test ./internal/notifier ./internal/analyzer -count=1` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make test` passed all packages.
- GREEN: `GOCACHE=/tmp/louder-go-build make vet` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make fmt-check` passed.
- GREEN: `git diff --check` passed.
