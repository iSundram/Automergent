package tips

// skills tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "skills",
		Tip:          "browse skills — runnable commands and project playbooks",
		Personalized: "",
		Body:         "# /skills\n\nOpens the skills inventory: custom markdown commands (runnable skills) and\nthe project/user skill playbooks the agent can load, with descriptions and\nsources.\n\n## Usage\n- `/skills` — the inventory page.\n\n## Notes\n- Skills live in `.automergent/skills/` (project) and\n  `~/.automergent/skills/` (user); project skills override user skills by\n  name.\n- The agent discovers and loads skills itself via the discover_skills and\n  skill tools; this page is your view of the same inventory.\n\n## Related\n- `/commands` — the command listing (runnable skills appear there too).\n- `/memory` — durable knowledge rather than playbooks.",
	})
}
