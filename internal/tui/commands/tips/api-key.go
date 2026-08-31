package tips

// api-key tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "api-key",
		Tip:          "set the API key for the active provider",
		Personalized: "",
		Body:         "# /api-key\n\nSets the API key for the active provider. For other providers' keys use\n`/provider api-key`, which names its target explicitly.\n\n## Usage\n- `/api-key <key>` — set the key for the active provider.\n- `/api-key` — show where the current key resolves from (config, env or\n  secret store — never the key itself).\n\n## Notes\n- Keys are stored in the user config and never echoed back.\n- Environment variables override config per the documented resolution\n  order.\n\n## Related\n- `/provider` — the provider studio, including per-provider keys.\n- `/doctor` — validates key resolution end to end.",
	})
}
