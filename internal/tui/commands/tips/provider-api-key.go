package tips

// provider-api-key tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "provider-api-key",
		Tip:          "set the API key for any provider (not just active)",
		Personalized: "",
		Body:         "# /provider api-key\n\nSets the API key for a named provider without switching to it — configure\nnow, switch later.\n\n## Usage\n- `/provider api-key <provider> <key>` — set the key.\n- `/provider api-key <provider>` — show where its key resolves from.\n\n## Notes\n- Never echoes keys back; shows the resolution source only.\n- Stored in user config, so it survives restarts.\n\n## Related\n- `/api-key` — the active provider's key.\n- `/provider` — the full provider studio.",
	})
}
