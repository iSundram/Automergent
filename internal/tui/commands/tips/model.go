package tips

// model tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "model",
		Tip:          "switch or inspect the AI model; opens the model hub",
		Personalized: "currently on {model} — switching mid-session keeps the transcript",
		Body:         "# /model\n\nOpens the model hub overlay to inspect and switch the active model: context\nwindow, capabilities and live availability from the provider.\n\n## Usage\n- `/model` — open the hub.\n- `/model <name>` — switch directly.\n- `/model refresh` — force a live re-fetch of the provider's model list.\n\n## Notes\n- Switching keeps the conversation; the next turn uses the new model.\n- The context-usage HUD recalculates against the new model's window size.\n- Fallback chains (see `/provider fallback`) still apply on failure.\n\n## Related\n- `/provider` — provider-level configuration and testing.\n- `/effort` — reasoning effort for the active model.",
	})
}
