package tips

// mcp tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "mcp",
		Tip:          "MCP server management — status, tools, enable, disable, add",
		Personalized: "",
		Body:         "# /mcp\n\nManages Model Context Protocol servers: connection status, tool inventory,\nresources, prompts and the event log.\n\n## Subcommands\n- `/mcp` — server status overview.\n- `/mcp tools <server>` — the tools a server exposes.\n- `/mcp resources` / `/mcp prompts` — resource and prompt inventories.\n- `/mcp enable|disable <server>` — toggle a server.\n- `/mcp reconnect|refresh <server>` — connection maintenance.\n- `/mcp add <name> <transport> <url-or-cmd>` — register a server.\n- `/mcp remove <name>` — remove one.\n- `/mcp events` — the connection event log.\n\n## Notes\n- Server tools register into the agent's tool registry automatically\n  (`mcp__server__tool` naming).\n- Config changes hot-reload without a restart.\n- The agent can also read MCP resources via its list/read tools.\n\n## Related\n- `/doctor` — includes MCP connectivity checks.",
	})
}
