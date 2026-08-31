tip: rewind to a checkpoint captured before an agent turn
---
# /rewind

Restores the conversation to the state captured before a chosen agent turn.
Checkpoints are captured automatically before every turn; the picker shows
each one's prompt, time and message count.

## Usage
- `/rewind` — open the checkpoint picker.
- `/rewind <n>` — jump to checkpoint n directly (1-based, oldest first).

## Notes
- Checkpoints after the chosen point are discarded.
- The session is persisted after rewinding.
- Checkpoints are session-scoped: switching sessions never mixes them.

## Related
- `/branch` — fork the session instead of truncating it.
- `/sessions` — switch to an entirely different session.
