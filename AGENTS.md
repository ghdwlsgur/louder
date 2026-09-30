# AGENTS.md

## 1. Purpose

This repository implements the **SRE Multi-Cloud Cost Platform** on the `sre-core` Kubernetes cluster.

The long-term direction is:

1. Multi-cloud cost collection and alerting
2. Cloud resource inventory
3. Resource create/update/delete tracking
4. Governance and policy enforcement
5. Central notification and reporting

The **current implementation phase is Phase 1 only**:

- multi-cloud cost collection
- normalization into a common schema
- budget / cost increase analysis
- Microsoft Teams notification

Supported public clouds:

- AWS
- Azure
- GCP
- OCI
- IBM Cloud
- Naver Cloud Platform (NCP)
- NHN Cloud
- Alibaba Cloud

Do not implement resource inventory, cloud activity/audit tracking, automatic remediation, or security governance unless explicitly requested.

---

## 2. Language Rules

**Communicate with the user in Korean. Write source-code comments in English.**

All conversation, progress reports, explanations, and user-facing implementation summaries must be written in Korean unless the user explicitly requests another language.

New or modified source-code comments must be written in English across:

- Go
- TypeScript / JavaScript
- Rust
- SQL migrations
- YAML / configuration files
- shell scripts
- other implementation files

Do not perform bulk translation of existing Korean comments merely for consistency.

Only comments that are newly written or materially rewritten must follow the English-comment rule. A file containing both Korean and English comments is acceptable when preserving existing code.

This rule does **not** apply to:

- Markdown documentation (`*.md`)
- commit messages
- pull request titles or bodies
- user-visible product copy
- i18n resources
- external documentation that has its own language requirement

For comments, prefer explaining **why** rather than restating **what** the code does.

---

## 3. Required Architecture

The project uses the Kubernetes Operator Pattern.

The Operator is the **control plane**, not the billing engine.

```text
CRD
 ↓
Operator
 ↓
Collector Job / CronJob
 ↓
Provider Adapter
 ↓
Normalizer
 ↓
ClickHouse
 ↓
Analyzer
 ↓
Teams
```

Strict responsibility boundaries:

```text
Operator    = orchestration, reconciliation, lifecycle, status
Collector   = billing API/export execution
Adapter     = provider-specific implementation
Normalizer  = provider data -> internal common schema
Storage     = cost data persistence
Analyzer    = budget / anomaly / forecast logic
Notifier    = Teams notification delivery
```

Do not place full billing collection logic inside controller reconciliation loops.

Do not store billing records, daily costs, or resource-level usage records in Kubernetes CRDs or etcd.

Read before changing architecture:

- `docs/architecture.md`
- `docs/provider-contract.md`

---

## 4. Secrets

Production secrets must follow:

```text
Vault
  ↓
External Secrets Operator
  ↓
Kubernetes Secret
  ↓
Workload
```

Vault is the source of truth.

Never commit plaintext secrets to:

- Git
- Helm values
- CR specs
- ConfigMaps
- fixtures
- example manifests
- documentation

The Operator references Kubernetes Secrets only.

Do not add direct Vault SDK integration to the Operator unless explicitly required.

Read:

- `docs/secrets.md`
- `SECURITY.md`

---

## 5. Kubernetes Platform Rules

Before modifying deployment manifests, read:

- `docs/cluster-platform.md`

Important platform constraints include:

- target context is `innogrid-core-sre`
- every direct `kubectl` command targeting the shared platform must specify `--context innogrid-core-sre`
- disposable local Kubernetes tests must specify their explicit kind context (for example, `--context kind-louder-e2e`) and must not target a shared cluster
- Cinder-backed workloads must be pinned to `topology.kubernetes.io/zone=incheon`
- physical-host HA on on-prem workers may require `sre-core/host-ip`
- `ServiceMonitor`, `PodMonitor`, and `PrometheusRule` require `release: monitoring`
- ArgoCD CRD deployment should use `ServerSideApply=true`
- do not enable ArgoCD automated sync by default
- production images should be published to `harbor.sre.local`
- do not rely on Pod Security Admission to protect workloads

If platform state may have changed, re-check the cluster before deployment.

---

## 6. Testing

Every provider must pass the same provider contract.

Normal development and CI must not require live CSP access.

Required test layers:

- provider fixture tests
- provider contract tests
- normalizer golden tests
- analyzer tests
- integration tests
- kind-based Kubernetes E2E
- Vault -> ESO -> Secret E2E
- failure injection

Read:

- `docs/harness.md`
- `docs/provider-contract.md`

A new provider is not complete until its fixtures, contract tests, and normalization tests exist.

---

# Coding Behavior Principles

These rules exist to reduce common LLM coding mistakes.

Project-specific instructions still take precedence where they are more specific.

The default bias is toward **careful verification over fast guessing**.

## 1. Think Before Coding

Do not guess silently.

Before implementation:

- identify assumptions
- surface tradeoffs
- name uncertainty
- verify relevant existing behavior
- distinguish facts from assumptions
- identify simpler alternatives when they exist

If multiple interpretations are plausible, do not quietly choose one when the difference materially affects implementation.

If something important is unclear, explain what is unclear and ask the user before making a high-impact or irreversible choice.

For small, low-risk tasks, use judgment and avoid unnecessary ceremony.

Do not create uncertainty merely to ask questions.

## 2. Prefer Simplicity

Write the minimum code required to satisfy the request correctly.

Do not add:

- unrequested features
- speculative flexibility
- single-use abstractions with no clear benefit
- configuration knobs for scenarios that do not exist
- defensive complexity for impossible or irrelevant cases
- framework layers merely to make the design appear more extensible

If the same result can reasonably be implemented in 50 lines instead of 200, prefer the smaller implementation.

Ask:

> Would an experienced engineer consider this over-engineered?

If yes, simplify.

This does not mean avoiding necessary architectural boundaries such as the Provider contract or Operator/Collector separation defined by this repository.

## 3. Make Surgical Changes

Modify only the files and lines required for the requested work.

Do not:

- refactor unrelated working code
- reformat neighboring files for convenience
- rename unrelated symbols
- rewrite comments that are not part of the requested change
- delete unrelated dead code
- change style merely because you prefer another style

Follow the existing repository style even when it differs from personal preference.

If unrelated dead code or defects are discovered, mention them instead of silently cleaning them up.

Remove unused imports, variables, functions, or generated artifacts only when they were introduced by your own change.

Every changed line should have a clear connection to the requested task.

## 4. Work Toward Verifiable Outcomes

Translate requests into observable success criteria.

Examples:

```text
"Add validation"
→ add invalid-input tests and make them pass

"Fix the bug"
→ reproduce the bug with a test, then make the test pass

"Refactor"
→ preserve behavior and verify tests before and after

"Add a provider"
→ fixtures + contract tests + golden normalization + integration verification
```

For multi-step work, define a short implementation plan and a verification method for each meaningful stage.

Do not call work complete merely because code compiles.

Success criteria should be concrete enough that another engineer can independently verify the result.

## 5. Prefer Code Over Comments

The default is code that is readable without comments.

Prefer:

- clear naming
- small cohesive functions
- explicit types
- simple control flow

before adding explanatory comments.

Before adding a comment, ask whether the need for the comment is caused by:

- a poor name
- an oversized function
- hidden state
- unnecessary indirection

### Avoid comments that:

- restate the code
- repeat the function signature
- explain an obvious branch
- preserve commented-out old code
- describe change history such as "changed from X to Y"

These comments become stale easily.

### Write comments when they preserve a reason the code cannot express

Good comments explain **why**, including:

- why one approach was chosen over another
- rejected alternatives that matter later
- external system constraints
- provider API quirks
- Kubernetes/platform invariants
- accepted gaps
- behavior discovered through measurement or incidents
- non-obvious compatibility requirements

Keep comment density consistent with nearby code.

New files should not become heavily commented unless the domain genuinely requires it.

---

# Git / Pull Request Policy

## 1. No Force Push

Do not use:

```text
git push --force
git push --force-with-lease
```

If a branch must be brought up to date with `develop`, prefer merging `develop` into the working branch and pushing normally.

Example:

```bash
git checkout <feature-branch>
git fetch origin
git merge origin/develop
git push
```

Resolve conflicts explicitly and commit the merge.

Do not rewrite published branch history unless the user explicitly changes this policy.

## 2. Do Not Bypass Pull Requests

Do not push directly to:

```text
develop
main
```

unless the user explicitly authorizes a repository-specific exception.

All normal changes must go through a pull request.

Do not treat convenience, urgency, or a small diff as authorization to bypass this rule.

## 3. New Pull Requests Require User Authorization

Authorization to merge an existing PR does not automatically authorize:

- closing it
- recreating it
- changing the PR strategy
- creating replacement PRs
- splitting it into multiple PRs

Creating a new PR requires explicit user authorization unless the current task clearly and explicitly includes PR creation.

## 4. Do Not Bypass Branch Protection

Do not use administrative merge bypasses such as:

```text
gh pr merge --admin
```

unless the user explicitly requests that specific bypass.

Do not weaken branch protection, required checks, review rules, or repository policy on your own.

## 5. Be Careful with Stacked or Parallel PRs

When multiple PRs share the same base, merging one may create conflicts in the others.

If PRs depend on each other:

- identify the dependency
- define the intended merge order
- after an earlier PR is merged, merge updated `develop` into the remaining branch
- resolve conflicts
- push normally

Do not solve stacked-PR conflicts by force-pushing rewritten history.

## 6. Do Not Make Repository Actions Implicit

Code changes, commits, pushes, PR creation, PR merge, and deployment are separate actions.

Do not assume authorization for a later action merely because the user authorized an earlier one.

Examples:

```text
"Fix this"
does not automatically mean "push it"

"Push this branch"
does not automatically mean "open a PR"

"Merge this PR"
does not automatically mean "create a new PR if this one fails"
```

When repository mutation matters, follow the exact scope of the user's instruction.

---

# Agent Operating Rules

## Task planning and history

Every implementation task must have a concrete plan under `docs/task/pending/` before implementation starts. Write all plans and history files under `docs/task/` in English. Read `docs/task/README.md` for the file format and lifecycle.

Name plans `phase-{major}-lv{minor}-{short-description}.md`. The major phase groups a broad project stage; the level numbers sequence smaller tasks within that phase. Check all three status directories before choosing the next unused level. Do not reuse or overwrite a task file.

Move the plan from `pending/` to `activate/` when implementation begins. Keep its scope, decisions, completion checklist, and verification record current. Move it to `completed/` only after the planned work and its verification are finished; record commands and results, including checks not run. Keep task history in `docs/task` rather than deleting old plans.

For code implementation, use TDD in vertical slices: write one behavior test first, observe it fail (RED), add the minimum implementation, then verify it passes (GREEN) before refactoring or starting the next behavior. Do not batch all tests ahead of implementation. For documentation-only changes, record an appropriate non-test verification instead. The detailed workflow is in [skills/louder-tdd/SKILL.md](skills/louder-tdd/SKILL.md). Codex's user-level skill discovery link is a workstation-local setup and is not part of this repository.

1. Read the relevant referenced docs before modifying a subsystem.
2. Preserve architecture boundaries.
3. Prefer extending stable interfaces over adding provider-specific conditions to shared code.
4. Do not bypass Vault/ESO.
5. Do not store billing datasets in Kubernetes.
6. Add or update tests for every provider mapping or normalization change.
7. Do not silently enable ArgoCD automated sync.
8. Do not silently create Cinder-backed workloads schedulable in Seoul.
9. Do not treat a successful collector run as proof that provider billing data is fresh.
10. Prefer a small, structurally correct Phase 1 over prematurely implementing future governance features.
11. Keep changes operable, reversible, observable, and testable.
12. Preserve existing repository conventions unless this document explicitly overrides them.
13. Do not perform broad cleanup while implementing a focused request.
14. Explain meaningful tradeoffs before making high-impact architectural changes.
15. When changing Kubernetes manifests, validate them against `docs/cluster-platform.md`.
16. When changing provider behavior, validate it against `docs/provider-contract.md`.
17. When changing test strategy or CI, validate it against `docs/harness.md`.
18. When changing credentials or secret flow, validate it against `docs/secrets.md`.
19. Before changing authentication, authorization, RBAC, Secrets, provider credentials, TLS, CI credentials, or other security-sensitive behavior, read `SECURITY.md`.
