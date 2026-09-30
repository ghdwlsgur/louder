# Phase 1 · LV3 — Move Codex Skill Discovery Outside the Repository

> Status: completed (2026-09-30)
> Scope: Keep the canonical TDD skill in the repository while removing the repository-local `.agents` discovery directory.

## Goal

Use Codex's documented user-level skill discovery location for this workstation so the repository does not need an `.agents` directory.

## Work items

- [x] Keep `skills/louder-tdd/SKILL.md` as the canonical repository copy.
- [x] Create a user-level symlink at `$HOME/.agents/skills/louder-tdd` pointing to the repository skill directory.
- [x] Remove the repository-local `.agents/skills/louder-tdd/SKILL.md` symlink and empty parent directories.
- [x] Update `AGENTS.md` and `docs/task/README.md` to explain the repository source and user-local discovery setup.
- [x] Record that the user-level link is machine-specific and is not distributed to other clones.
- [x] Verify the global link resolves to the repository skill, no repository `.agents` directory remains, the skill validates, and task documents remain English.

## Acceptance criteria

- The repository contains the skill source under `skills/louder-tdd/` and no `.agents` directory.
- This workstation exposes the skill through `$HOME/.agents/skills/louder-tdd`.
- Documentation does not imply that a user-level symlink is automatically available to other developers or clones.

## Verification

- Inspect the symlink target with `readlink` and confirm `SKILL.md` is readable through it.
- Run the skill metadata validator.
- Search `docs/task` for Hangul characters and run `git diff --check`.
- Codex may need a restart to refresh skill discovery; do not claim runtime discovery was verified in the current session.

### Results

- `$HOME/.agents/skills/louder-tdd` resolves to the repository's `skills/louder-tdd` directory, and `SKILL.md` is readable through it.
- The repository no longer contains `.agents`.
- Skill metadata validation, task-document English check, and `git diff --check` passed.
- Runtime skill-list refresh was not checked; restart Codex if the skill does not appear.

## References

- [Codex skill discovery locations](https://developers.openai.com/codex/skills)
