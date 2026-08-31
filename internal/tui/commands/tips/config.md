tip: open the settings picker — every persistent option
---
# /config

Opens the settings picker: every persistent option (provider, model, theme,
keybindings, effort, security paths) with live values, plus paths to the
global and project config files.

## Usage
- `/config` — open the picker.
- Changes made through focused commands (`/theme`, `/effort`, ...) persist
  to the same store.

## Notes
- Global config lives in `~/.automergent/config.yaml`; the project config
  overrides it per workspace.
- `/doctor` validates the merged result.

## Related
- `/doctor` — environment health check.
- `/env` — the runtime environment view.
