# KubeDrive's MCP bridge in a container, for hosts that run MCP servers from
# a Dockerfile (Glama, Smithery). The bridge forwards to the KubeDrive desktop
# app on the same computer, so in a container it has no app to talk to: it
# answers initialize, ping and tools/list itself (the full tool list) and
# fails tool calls with the reason. To use the clusters, run the bridge on the
# computer where KubeDrive runs, from the release's MCP bundle
# (kubedrive-<version>.mcpb) or the app's own copy; the setup for each client
# is at https://kubedrive.com/docs/ai-agents
#
# The bridge is a static Linux binary from the latest release. Set
# KUBEDRIVE_MCP_TOKEN (the client token from KubeDrive → AI Agents) and, when
# the app's port was changed, KUBEDRIVE_MCP_URL.
FROM alpine:3.21
ADD https://github.com/aleksskoz/kubedrive/releases/latest/download/kubedrive-mcp-linux-amd64 /usr/local/bin/kubedrive-mcp
RUN chmod 0755 /usr/local/bin/kubedrive-mcp && adduser -D -u 10001 mcp
USER mcp
ENTRYPOINT ["/usr/local/bin/kubedrive-mcp"]
