# KubeDrive's MCP bridge in a container, for hosts that run MCP servers from
# a Dockerfile (Glama, Smithery). Built from source: mcp/ is the bridge's Go
# module (Apache-2.0), rendered from the private KubeDrive repository with
# every release. The bridge forwards to the KubeDrive desktop app on the same
# computer, so in a container it has no app to talk to: it answers
# initialize, ping and tools/list itself (the full tool list) and fails tool
# calls with the reason. To use the clusters, run the bridge on the computer
# where KubeDrive runs, from the release's MCP bundle (kubedrive-<version>.mcpb)
# or the app's own copy; the setup for each client is at
# https://kubedrive.com/docs/ai-agents
#
# Set KUBEDRIVE_MCP_TOKEN (the client token from KubeDrive → AI Agents) and,
# when the app's port was changed, KUBEDRIVE_MCP_URL.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY mcp/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-w -s -X main.version=$(cat VERSION)" -o /kubedrive-mcp .

FROM alpine:3.21
COPY --from=build /kubedrive-mcp /usr/local/bin/kubedrive-mcp
RUN adduser -D -u 10001 mcp
USER mcp
ENTRYPOINT ["/usr/local/bin/kubedrive-mcp"]
