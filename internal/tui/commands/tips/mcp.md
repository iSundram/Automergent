tip: MCP server management — status, tools, enable, disable, add
---
# /mcp

Manages Model Context Protocol servers: connection status, tool inventory,
resources, prompts and the event log.

## Subcommands
- `/mcp` — server status overview.
- `/mcp tools <server>` — the tools a server exposes.
- `/mcp resources` / `/mcp prompts` — resource and prompt inventories.
- `/mcp enable|disable <server>` — toggle a server.
- `/mcp reconnect|refresh <server>` — connection maintenance.
- `/mcp add <name> <transport> <url-or-cmd>` — register a server.
- `/mcp remove <name>` — remove one.
- `/mcp events` — the connection event log.

## Notes
- Server tools register into the agent's tool registry automatically
  (`mcp__server__tool` naming).
- Config changes hot-reload without a restart.
- The agent can also read MCP resources via its list/read tools.

## Related
- `/doctor` — includes MCP connectivity checks.
