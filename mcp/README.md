# kubedrive-mcp

The stdio bridge to [KubeDrive](https://kubedrive.com)'s MCP server: the one part of KubeDrive
whose source is open (Apache-2.0, this directory only; the app is closed source). It reads
JSON-RPC messages from stdin, posts each to the KubeDrive app on the same computer with the
client token, and writes the answers to stdout. It keeps no state and holds no secrets.

Without the token, or with KubeDrive not running, the bridge still answers `initialize`, `ping`
and `tools/list` from the embedded catalog of every tool the app can offer, and fails tool calls
with the reason. That is how hosts that build MCP servers from source (Glama) see a working
server; to use the clusters, run the bridge where KubeDrive runs.

The setup for each client (Claude Desktop, Claude Code, Codex, Cursor) is at
https://kubedrive.com/docs/ai-agents

## Build and run

```sh
cd mcp
go build -o kubedrive-mcp .
KUBEDRIVE_MCP_TOKEN=kd_… ./kubedrive-mcp
```

- `KUBEDRIVE_MCP_TOKEN`: the client token from KubeDrive → AI Agents (starts with `kd_`). Only
  from the environment: a flag would show it in the process list.
- `KUBEDRIVE_MCP_URL`: KubeDrive's MCP endpoint, only when its port was changed
  (default `http://127.0.0.1:47011/mcp`). Only `http` to the loopback is accepted.

The repository's `Dockerfile` builds this module for container hosts.

## Source of truth

This directory is rendered from the private KubeDrive repository by its release workflow
(`scripts/mirror-mcp.sh`) with every tagged release; the tool catalog (`mcpbridge/catalog.json`)
is generated there from the server's tool definitions. Pull requests here are welcome as issues
and patches; they are applied upstream and come back with the next release.
