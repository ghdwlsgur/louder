# Task Plans and History

Every implementation task starts with a plan that defines its scope and completion criteria. Keep implementation and verification results in the same document.

## Status directories

```text
docs/task/
├── pending/    Plan is ready; implementation has not started
├── activate/   Implementation or verification is in progress
└── completed/  Planned work and verification are finished
```

Move each plan through `pending → activate → completed`. Do not move a plan to `completed` while implementation or verification remains. Move the same file between directories so its history stays together.

## Phases and filenames

Use this filename format:

```text
phase-{major}-lv{minor}-{short-kebab-case-description}.md
```

Examples: `phase-1-lv1-harness-scope-alignment.md`, `phase-1-lv2-operator-scaffold.md`, `phase-2-lv1-cost-analysis.md`.

The major phase represents a broad project stage. The level number sequences smaller tasks within that phase. Before creating a plan, check all three status directories and use the next unused level for that phase. Do not reuse or overwrite a task filename. Store plan files directly in their status directory.

## Plan contents and language

Write all files under `docs/task/` in English. Each plan should record:

- Goal, scope, and explicit non-goals
- Dependencies and prior documents
- Work items with relevant file paths
- Independently verifiable completion criteria
- Design decisions, risks, and open questions
- Exact verification commands and results, including checks not run

## TDD for code changes

Implement one observable behavior at a time:

1. Identify behavior visible through a public interface.
2. Write one test for that behavior and run it to confirm a genuine failure (RED).
3. Add the minimum implementation needed to pass that test.
4. Confirm the test passes (GREEN), then refactor only while green and rerun verification.
5. Continue with the next behavior. Do not write a batch of tests before implementing them.

For documentation-only work, record suitable checks such as link, path, or format validation instead of behavior tests.

Keep canonical skill sources under `skills/`. Each skill should contain `SKILL.md` and only the supporting resources it needs. Codex discovers repository skills from `.agents/skills/` and user skills from `$HOME/.agents/skills/`. This workstation exposes `skills/louder-tdd/` through a user-level symlink so the repository does not need a `.agents/` directory. That link is local setup and is not shared with other clones; other developers can add a repository discovery link if they need the skill automatically in their Codex sessions.
