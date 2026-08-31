package tips

// base-url tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "base-url",
		Tip:          "set a custom API base URL (proxies, gateways, local servers)",
		Personalized: "",
		Body:         "# /base-url\n\nOverrides the API endpoint for the active provider — useful for proxies,\nenterprise gateways and locally hosted servers.\n\n## Usage\n- `/base-url <url>` — set the endpoint.\n- `/base-url` — show the current value.\n- `/base-url reset` — clear it back to the provider default.\n\n## Notes\n- Applies to the active provider only; per-provider overrides live in the\n  provider studio (`/provider base-url`).\n- An invalid URL fails fast with a validation error, not a request error.\n\n## Related\n- `/provider` — provider-level settings.",
	})
}
