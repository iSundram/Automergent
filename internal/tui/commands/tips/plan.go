package tips

// plan tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "plan",
		Tip:          "enter plan mode — read-only analysis, output is a plan",
		Personalized: "",
		Body:         "# /plan\n\nSends the agent into plan mode: read-only exploration, then a written\nimplementation plan (as an artifact) for your approval before anything is\nedited.\n\n## Usage\n- `/plan` — plan mode with no particular focus.\n- `/plan <focus>` — steer what the plan should cover.\n- `/plan copy` — copy the current plan to the clipboard.\n\n## Notes\n- The plan lands in `.automergent/artifacts/plan.md` and appears in\n  `/artifact` for approve/reject.\n- Approving a plan puts the agent straight to work implementing it;\n  rejecting requires a reason, which the agent uses to revise.\n- If the task does not warrant a plan, the agent continues normally\n  instead — plan mode is a tool, not a toll.\n\n## Related\n- `/artifact` — the review browser for plans and deliverables.\n- `/mode` — plan mode is also one of the approval modes.",
	})
}
