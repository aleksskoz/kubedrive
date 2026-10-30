// Package mcpbridge is the stdio side of KubeDrive's MCP server for clients
// that speak only stdio (Claude Desktop): it reads JSON-RPC messages from
// stdin, posts each to the local HTTP server with the client token, and
// writes the answers to stdout. It keeps no state: the server keeps none
// either. Built as the kubedrive-mcp binary beside the app's own.
//
// When KubeDrive is not running, or no token is set, the bridge still
// answers initialize, ping and tools/list itself, from catalog.json (every
// tool the server can offer, generated from internal/mcp): a client can show
// the tools before the app is started, and a host that only checks that the
// server starts (a container with no KubeDrive) sees a working server. A
// tool call then fails with an error that says what to start.
package mcpbridge

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultURL is the server's endpoint when KubeDrive uses its default port.
const DefaultURL = "http://127.0.0.1:47011/mcp"

// maxMessage bounds one stdio message; the server refuses larger requests.
const maxMessage = 16 << 20

// JSON-RPC error codes the bridge itself reports.
const (
	codeParse       = -32700
	codeUnavailable = -32001 // KubeDrive not running, or the token refused
)

// protocolVersions are the MCP revisions the bridge answers for when it
// answers initialize itself; the same as the server's, newest first.
var protocolVersions = []string{"2025-06-18", "2025-03-26"}

// catalog is tools/list as the server answers it with every tool on: kept
// current by TestCatalogIsCurrent in internal/mcp.
//
//go:embed catalog.json
var catalog []byte

// Options configure Run.
type Options struct {
	URL   string
	Token string
	// Version is reported as the server version when the bridge answers
	// initialize itself.
	Version string
	// HTTP is the client to use; nil for a default with a long timeout (a
	// change waits for the user).
	HTTP *http.Client
	// Log receives what went wrong, for stderr; nil discards it.
	Log func(format string, args ...any)
}

// Run forwards messages until in ends or ctx is cancelled. Each message is
// handled in its own goroutine, so a call that waits for the user does not
// hold up the others; answers are written whole, one per line.
func Run(ctx context.Context, in io.Reader, out io.Writer, opts Options) error {
	if opts.URL == "" {
		opts.URL = DefaultURL
	}
	if err := localURL(opts.URL); err != nil {
		return err
	}
	if opts.HTTP == nil {
		opts.HTTP = &http.Client{Timeout: 10 * time.Minute}
	}
	if opts.Log == nil {
		opts.Log = func(string, ...any) {}
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.Token == "" {
		opts.Log("no token: set KUBEDRIVE_MCP_TOKEN as the snippet in KubeDrive → AI Agents shows; tool calls fail until then")
	}
	b := &bridge{opts: opts, out: out}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), maxMessage)
	var wg sync.WaitGroup
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		msg := make([]byte, len(line))
		copy(msg, line)
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.forward(ctx, msg)
		}()
	}
	wg.Wait()
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read stdin: %w", err)
	}
	return nil
}

// localURL accepts only plain HTTP to this computer: the token is sent as a
// header, and the bridge exists to reach the app on the same machine.
func localURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("bad KubeDrive URL %q: %w", raw, err)
	}
	host := u.Hostname()
	if u.Scheme != "http" || host == "" {
		return fmt.Errorf("KubeDrive URL %q: only http://127.0.0.1:<port>/mcp or http://localhost:<port>/mcp are accepted", raw)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("KubeDrive URL %q is not this computer: only http://127.0.0.1:<port>/mcp or http://localhost:<port>/mcp are accepted", raw)
}

type bridge struct {
	opts Options
	mu   sync.Mutex // guards out: one message per line, never interleaved
	out  io.Writer
}

// forward posts one message and writes the answer. Without a token, or when
// the server cannot be reached, the bridge answers what it can itself and
// fails the rest with an error the client can show.
func (b *bridge) forward(ctx context.Context, msg []byte) {
	m := parse(msg)
	if b.opts.Token == "" {
		b.offline(m, "KubeDrive's client token is not set: copy the snippet from KubeDrive → AI Agents, which sets KUBEDRIVE_MCP_TOKEN")
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.opts.URL, bytes.NewReader(msg))
	if err != nil {
		b.fail(m, codeParse, "bad request: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.opts.Token)
	resp, err := b.opts.HTTP.Do(req)
	if err != nil {
		b.opts.Log("KubeDrive is not reachable at %s: %v", b.opts.URL, err)
		b.offline(m, "KubeDrive is not running, or its AI Agents server is off: start KubeDrive and turn on AI Agents")
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMessage))
	if err != nil {
		b.fail(m, codeUnavailable, "reading KubeDrive's answer failed: "+err.Error())
		return
	}
	switch {
	case resp.StatusCode == http.StatusAccepted:
		// A notification: nothing to answer.
	case resp.StatusCode == http.StatusUnauthorized:
		b.fail(m, codeUnavailable, "KubeDrive refused the token: copy the current Claude Desktop snippet from KubeDrive → AI Agents")
	case resp.StatusCode == http.StatusOK && json.Valid(body):
		b.write(body)
	default:
		b.fail(m, codeUnavailable, fmt.Sprintf("KubeDrive answered HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}
}

// offline answers a message without the server: initialize, ping and
// tools/list from what the bridge knows, anything else with reason.
func (b *bridge) offline(m message, reason string) {
	if !m.isRequest {
		b.opts.Log("%s", reason)
		return
	}
	switch m.method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(m.params, &p)
		b.result(m.id, map[string]any{
			"protocolVersion": negotiate(p.ProtocolVersion),
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "kubedrive", "title": "KubeDrive", "version": b.opts.Version},
			"instructions": "The Kubernetes clusters the user allowed in KubeDrive, with the user's own credentials. " +
				reason + ". Until then every tool call fails; the tool list is the full one.",
		})
	case "ping":
		b.result(m.id, map[string]any{})
	case "tools/list":
		b.result(m.id, map[string]any{"tools": json.RawMessage(catalog)})
	default:
		b.fail(m, codeUnavailable, reason)
	}
}

// negotiate picks the protocol version to answer with, as the server does.
func negotiate(asked string) string {
	for _, v := range protocolVersions {
		if v == asked {
			return v
		}
	}
	return protocolVersions[0]
}

func (b *bridge) result(id json.RawMessage, result any) {
	out, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	if err != nil {
		b.fail(message{id: id, isRequest: true}, codeUnavailable, "building the answer failed: "+err.Error())
		return
	}
	b.write(out)
}

// fail answers a request with a JSON-RPC error; a notification gets none.
func (b *bridge) fail(m message, code int, text string) {
	if !m.isRequest {
		b.opts.Log("%s", text)
		return
	}
	id := m.id
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	out, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": text},
	})
	b.write(out)
}

func (b *bridge) write(msg []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, _ = b.out.Write(append(bytes.TrimSpace(msg), '\n'))
}

// message is what the bridge needs to know about one stdio message.
type message struct {
	id        json.RawMessage
	isRequest bool // has an id; a batch counts as a request with a null id
	method    string
	params    json.RawMessage
}

func parse(msg []byte) message {
	if len(msg) > 0 && msg[0] == '[' {
		return message{isRequest: true}
	}
	var m struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(msg, &m); err != nil {
		return message{isRequest: true}
	}
	return message{id: m.ID, isRequest: len(m.ID) > 0 && string(m.ID) != "null", method: m.Method, params: m.Params}
}
