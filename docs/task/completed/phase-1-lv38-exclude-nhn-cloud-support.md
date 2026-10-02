# Phase 1 Level 38: Exclude NHN Cloud from Supported Providers

## Goal

Update the project scope to exclude NHN Cloud as directed by the user, and close the pending NHN adapter plan without implying that an adapter was implemented.

## Scope

- Remove NHN Cloud from the active supported-provider list and current provider layout examples.
- Keep historical notes that explain prior NHN investigation and daily-anomaly coverage decisions.
- Move the NHN adapter plan to `completed` with the outcome that NHN support is excluded and no adapter is being implemented.
- Do not change provider code or claim support for a new provider.

## Non-goals

- Re-investigating NHN billing APIs.
- Implementing monthly or daily NHN collection.
- Removing historical task records that mention earlier plans.

## Work items

1. Remove NHN from `AGENTS.md`, `SECURITY.md`, `docs/architecture.md`, `docs/provider-contract.md`, `docs/harness.md`, and `docs/secrets.md` wherever those documents describe current supported providers or current package/credential layouts.
2. Preserve historical references in completed task records and clarify current NHN exclusion where needed.
3. Move the pending NHN adapter plan to `completed` and record the user's scope decision.

## Completion criteria

- No current supported-provider list includes NHN Cloud.
- Current provider package and credential examples do not imply an NHN implementation.
- The NHN plan is in `docs/task/completed/` and states that the adapter was not implemented.
- `rg` review, Markdown whitespace checks, and `git diff --check` pass.

## Verification

- `rg -n -i "supported providers|supported public clouds|8 CSP|eight CSP|NHN Cloud|nhn/" AGENTS.md SECURITY.md docs/architecture.md docs/provider-contract.md docs/harness.md docs/secrets.md`
- `git diff --check`

## Results

- Removed NHN from the active supported-cloud list, security credential provider list, architecture and test harness package layouts, provider contract list, and Vault example layout.
- Updated the harness and architecture to state that NHN is outside the supported-provider scope; NCP monthly support remains represented separately.
- Closed the former NHN adapter plan as a scope decision and preserved its API research in `docs/task/completed/phase-1-lv34-nhn-cloud-support-exclusion.md`.
- `rg` review confirmed the current supported-provider lists and package layouts contain no NHN entry. Remaining mentions are explicit exclusion statements or historical task records.
- `git diff --check` — passed.
