tip: stage and commit the session's changes
---
# /commit

Hands the commit job to the agent: it reviews the session's diff, drafts a
conventional-commit message, stages the changes and commits — asking for
confirmation before the actual `git commit`.

## Usage
- `/commit` — stage everything this session touched and commit.
- `/commit <message hint>` — steer the message draft.

## Notes
- The drafted message is shown before anything is committed.
- Pushing is never implied — commit and push stay separate decisions.
- Secret scanning runs before staging; flagged content blocks the commit.

## Related
- `/review` — review the diff before committing.
- `/diff` — inspect the raw changes yourself.
