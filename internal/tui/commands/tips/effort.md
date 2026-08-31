tip: set reasoning effort — how hard the model thinks per turn
personalized: effort trades latency and cost for depth on {model}
---
# /effort

Sets the reasoning effort for models that support it: how much thinking the
model invests before answering.

## Usage
- `/effort` — show the current level.
- `/effort <low|medium|high|max>` — switch.

## Notes
- Models without effort control ignore the setting gracefully.
- Higher effort costs more tokens and time; routine edits rarely need it.

## Related
- `/model` — the model itself.
- `/provider` — provider-level knobs.
