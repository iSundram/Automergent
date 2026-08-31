package tips

// provider-base-url tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "provider-base-url",
		Tip:          "set a custom base URL for any provider",
		Personalized: "",
		Body:         "# /provider base-url\n\nOverrides the API endpoint for a named provider without switching to it.\n\n## Usage\n- `/provider base-url <provider> <url>` — set the endpoint.\n- `/provider base-url <provider> reset` — back to the provider default.\n\n## Notes\n- Useful for gateways and local servers on secondary providers.\n- Validated on save; bad URLs fail immediately.\n\n## Related\n- `/base-url` — the active provider's endpoint.",
	})
}
