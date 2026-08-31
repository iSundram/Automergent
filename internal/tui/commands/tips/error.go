package tips

// error tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "error",
		Tip:          "the API error log — retries, failures, request ids (alias /errors)",
		Personalized: "",
		Body:         "# /error\n\nShows the recorded provider API failures for this session as a full page:\nretried attempts (warnings) and terminal failures (failures), newest first,\nwith error codes, retry timing, request ids for provider-side log\ncorrelation and remediation hints.\n\n## Usage\n- `/error` or `/errors` — the error page.\n- `/error <n>` — expand the nth entry in place.\n- `/error clear` — wipe the log.\n\n## Notes\n- Retried attempts are marked distinctly from final failures; a request\n  that eventually succeeded still shows its retries.\n- Credentials are sanitized out of recorded messages.\n- Session-scoped: `/new` starts a clean log.\n\n## Related\n- `/doctor` — connectivity check.\n- `/provider fallback` — the chain used on failure.",
	})
}
