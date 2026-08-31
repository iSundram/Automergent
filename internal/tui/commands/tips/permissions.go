package tips

// permissions tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "permissions",
		Tip:          "manage the always-allow tool permission list",
		Personalized: "",
		Body:         "# /permissions\n\nOpens the permissions manager: the scopes the agent may use without asking,\nrecorded from your \"always allow\" answers and persisted with the session so\nthey survive restarts and resumes.\n\n## Usage\n- `/permissions` — open the picker.\n- `/permissions add <scope>` / `/permissions remove <scope>` — edit directly.\n- `/permissions clear` — drop everything.\n\n## Notes\n- Scopes are per tool and per path pattern where applicable.\n- Mode switches (`/mode`) do not clear the list; they stack on top of it.\n- Removing a scope takes effect on the next tool call.\n\n## Related\n- `/mode` — the coarser approval-mode switch.\n- `/doctor` — includes a permissions sanity check.",
	})
}
