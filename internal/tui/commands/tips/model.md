tip: switch or inspect the AI model; opens the model hub
personalized: currently on {model} — switching mid-session keeps the transcript
---
# /model

Opens the model hub overlay to inspect and switch the active model: context
window, capabilities and live availability from the provider.

## Usage
- `/model` — open the hub.
- `/model <name>` — switch directly.
- `/model refresh` — force a live re-fetch of the provider's model list.

## Notes
- Switching keeps the conversation; the next turn uses the new model.
- The context-usage HUD recalculates against the new model's window size.
- Fallback chains (see `/provider fallback`) still apply on failure.

## Related
- `/provider` — provider-level configuration and testing.
- `/effort` — reasoning effort for the active model.
