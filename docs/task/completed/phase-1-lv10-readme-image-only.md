# Phase 1 LV10: README Image-Only Landing Page

## Goal

Keep only the project introduction image at the root of `README.md`.

## Scope

- Preserve the existing `assets/introduce.png` image as the sole rendered README content.
- Remove all other README headings, paragraphs, lists, and code blocks.

## Non-goals

- Modify or regenerate the image asset.
- Change other project documentation or source code.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/task/README.md`

## Completion criteria

- `README.md` contains one Markdown image and no other content.
- The Markdown image references the existing `assets/introduce.png` file.
- Task files remain in English and follow the task lifecycle.

## Verification plan

- Confirm README has exactly one non-empty line and it is the expected image reference.
- Confirm `assets/introduce.png` exists.
- Run `git diff --check`.

## Verification results

- `awk 'NF { count++; if ($0 != "![Louder project introduction](assets/introduce.png)") exit 1 } END { exit count != 1 }' README.md` — passed; the image reference is the only non-empty line.
- `test -f assets/introduce.png` — passed.
- `git diff --check` — passed.
