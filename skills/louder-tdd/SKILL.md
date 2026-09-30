---
name: louder-tdd
description: Implement or change Louder code with the repository's test-first Red, minimal Green, and Refactor workflow. Use for code changes; documentation-only edits do not need this skill.
---

# Louder TDD

Use this workflow for code changes in this repository. `AGENTS.md` makes TDD the default for code implementation; this skill gives the concrete cycle. Keep the active task plan in `docs/task/activate/` current.

## Before the first cycle

- Read the relevant design, provider, security, and harness documents required by `AGENTS.md`.
- Identify one externally observable behavior to implement and the public interface through which a caller can observe it.
- Make sure the active task plan names the behavior, its acceptance criteria, and the command that will verify it.

## Red → Green → Refactor

Repeat one behavior at a time:

1. **Red:** Write one focused test through the public interface. Run it before changing production code. Confirm it fails because the behavior is missing. A compile error, missing dependency, test setup failure, or unrelated failure does not count as Red; fix the test setup and rerun.
2. **Green:** Add only the smallest production change that satisfies that test. Run the focused test and confirm it passes.
3. **Refactor:** Once Green, simplify duplication or clarify the boundary without changing behavior. Re-run the focused test after each refactor.
4. Continue with the next behavior. Do not write a batch of speculative tests before implementation.

Tests should describe behavior, not private helpers or the current implementation shape. For security-sensitive behavior, include an appropriate failure case as well as success and follow `SECURITY.md`.

## Verification and task history

- Run the focused test for every Red/Green cycle, then run relevant package or repository checks before calling the code complete.
- Record exact commands and outcomes in the active plan, including checks that could not run and why.
- Move the plan to `docs/task/completed/` only after planned implementation and verification are done.
- Do not treat a test file's presence or a build alone as proof that behavior is correct.
