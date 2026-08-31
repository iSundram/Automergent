tip: toggle detailed change review for edit proposals
---
# /review-mode

Toggles detailed change review: while on, every edit proposal renders with
review grammar — the proposed change plus `a` to accept and apply, `r` to
reject — instead of a plain diff card.

## Notes
- A pure display/interaction toggle; agent behavior is unchanged.
- The status line and `/review` output use the same review styling.

## Related
- `/review` — one-off model review of the whole change set.
- `/mode` — approval autonomy (separate concern).
