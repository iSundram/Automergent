package tips

// provider tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "provider",
		Tip:          "configure providers — switch, test, fallback chain, API keys",
		Personalized: "active provider serves {model}; /provider test checks connectivity",
		Body:         "# /provider\n\nOpens the provider studio: switch providers, set per-provider API keys and\nbase URLs, test connectivity live, and manage the fallback chain used when\na provider fails.\n\n## Subcommands\n- `/provider test <name>` — live connectivity check (async result).\n- `/provider fallback` — show the fallback chain.\n- `/provider fallback <a,b,c>` — replace it.\n- `/provider api-key <name> <key>` — set a key without leaving the prompt.\n\n## Notes\n- Keys are never echoed back; `/provider` shows where a key resolves from\n  (config, env or secret store), not the key itself.\n- The fallback chain is tried in order on request failure.\n\n## Related\n- `/model` — model selection within the active provider.\n- `/doctor` — full environment health check.",
	})
}
