package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunForwardsRequestsAndNotifications(t *testing.T) {
	var mu sync.Mutex
	var gotAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.Unmarshal(body, &m)
		if len(m.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if m.Method == "slow" {
			time.Sleep(150 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(m.ID) + `,"result":{"method":"` + m.Method + `"}}`))
	}))
	defer srv.Close()
	in := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"slow\"}\n\n{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\"}\n")
	var out bytes.Buffer
	if err := Run(context.Background(), in, &out, Options{URL: srv.URL, Token: "kd_t"}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output = %q", out.String())
	}
	// The quick answer comes first: calls do not wait for each other.
	if !strings.Contains(lines[0], `"id":2`) || !strings.Contains(lines[1], `"id":1`) {
		t.Errorf("order = %v", lines)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(gotAuth) != 3 {
		t.Errorf("requests = %d", len(gotAuth))
	}
	for _, a := range gotAuth {
		if a != "Bearer kd_t" {
			t.Errorf("auth = %q", a)
		}
	}
}

func TestRunReportsUnreachableAndRefused(t *testing.T) {
	var out bytes.Buffer
	var mu sync.Mutex
	var logged []string
	opts := Options{URL: "http://127.0.0.1:1/mcp", Token: "kd_t", HTTP: &http.Client{Timeout: time.Second}, Log: func(f string, _ ...any) {
		mu.Lock()
		defer mu.Unlock()
		logged = append(logged, f)
	}}
	if err := Run(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"list_clusters"}}`+"\n"+`{"jsonrpc":"2.0","method":"note"}`+"\n"), &out, opts); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		ID    int `json:"id"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil || resp.ID != 7 || resp.Error.Code != codeUnavailable || !strings.Contains(resp.Error.Message, "start KubeDrive") {
		t.Fatalf("unreachable: %q (%v)", out.String(), err)
	}
	mu.Lock()
	if len(logged) == 0 {
		t.Error("nothing logged for the notification")
	}
	mu.Unlock()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()
	out.Reset()
	_ = Run(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":"a","method":"ping"}`+"\n"), &out, Options{URL: srv.URL, Token: "kd_old"})
	if !strings.Contains(out.String(), `"id":"a"`) || !strings.Contains(out.String(), "refused the token") {
		t.Errorf("refused: %q", out.String())
	}
}

// Without the app the bridge still introduces itself and lists every tool,
// so a client shows the tools before KubeDrive runs and a host that only
// starts the server sees one; a tool call says what to start.
func TestRunAnswersIntrospectionWithoutTheApp(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"t"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"ping"}` + "\n" +
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_clusters"}}` + "\n")
	var out bytes.Buffer
	opts := Options{URL: "http://127.0.0.1:1/mcp", Token: "kd_t", Version: "1.2.3", HTTP: &http.Client{Timeout: time.Second}}
	if err := Run(context.Background(), in, &out, opts); err != nil {
		t.Fatal(err)
	}
	answers := map[int]map[string]json.RawMessage{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m struct {
			ID     int                        `json:"id"`
			Result map[string]json.RawMessage `json:"result"`
			Error  *struct {
				Code    int
				Message string
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		if m.Error != nil {
			if m.ID != 4 || m.Error.Code != codeUnavailable || !strings.Contains(m.Error.Message, "start KubeDrive") {
				t.Errorf("id %d: error %+v", m.ID, m.Error)
			}
			continue
		}
		answers[m.ID] = m.Result
	}
	if len(answers) != 3 {
		t.Fatalf("answers = %v\n%s", answers, out.String())
	}
	var info struct {
		Name, Version string
	}
	_ = json.Unmarshal(answers[1]["serverInfo"], &info)
	if string(answers[1]["protocolVersion"]) != `"2025-03-26"` || info.Name != "kubedrive" || info.Version != "1.2.3" || !strings.Contains(string(answers[1]["instructions"]), "start KubeDrive") {
		t.Errorf("initialize = %s", answers[1])
	}
	var tools []struct {
		Name        string
		InputSchema json.RawMessage
	}
	if err := json.Unmarshal(answers[2]["tools"], &tools); err != nil || len(tools) < 20 {
		t.Fatalf("tools/list = %d tools (%v)", len(tools), err)
	}
	names := map[string]bool{}
	for _, x := range tools {
		names[x.Name] = true
		if !json.Valid(x.InputSchema) {
			t.Errorf("%s: bad schema", x.Name)
		}
	}
	for _, want := range []string{"list_clusters", "get_logs", "diagnose", "timeline", "apply", "exec"} {
		if !names[want] {
			t.Errorf("tools/list lacks %s", want)
		}
	}
	if len(answers[3]) != 0 {
		t.Errorf("ping = %v", answers[3])
	}
}

// Without a token nothing is sent to the server: the answer is the same as
// without the app, with the token in the reason.
func TestRunWithoutTokenSendsNothing(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"top"}}` + "\n")
	if err := Run(context.Background(), in, &out, Options{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Errorf("server got %d requests without a token", hits)
	}
	if !strings.Contains(out.String(), `"tools":[`) || !strings.Contains(out.String(), "KUBEDRIVE_MCP_TOKEN") {
		t.Errorf("out = %q", out.String())
	}
}

// The token travels as a header, so the bridge only talks plain HTTP to this
// computer, whatever KUBEDRIVE_MCP_URL says.
func TestRunStaysOnThisComputer(t *testing.T) {
	for _, bad := range []string{"http://example.com/mcp", "https://127.0.0.1:47011/mcp", "http://10.0.0.5:47011/mcp", "http://[2001:db8::1]:47011/mcp", "http://127.0.0.1.nip.io/mcp", "http://[fe80::1]/mcp", "ftp://127.0.0.1/mcp", "::"} {
		var out bytes.Buffer
		if err := Run(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), &out, Options{URL: bad, Token: "kd_t"}); err == nil || out.Len() != 0 {
			t.Errorf("%s: err = %v, out = %q", bad, err, out.String())
		}
	}
	for _, ok := range []string{"http://127.0.0.1:47011/mcp", "http://localhost:47012/mcp", "http://[::1]:47011/mcp", "http://127.5.5.5:1/mcp"} {
		if err := localURL(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
}

func TestParse(t *testing.T) {
	for _, c := range []struct {
		msg    string
		id     string
		isRq   bool
		method string
	}{
		{`{"id":1,"method":"x"}`, "1", true, "x"},
		{`{"id":"abc","method":"x","params":{"a":1}}`, `"abc"`, true, "x"},
		{`{"method":"x"}`, "", false, "x"},
		{`{"id":null,"method":"x"}`, "null", false, "x"},
		{`[{"id":1}]`, "", true, ""},
		{`not json`, "", true, ""},
	} {
		m := parse([]byte(c.msg))
		if string(m.id) != c.id || m.isRequest != c.isRq || m.method != c.method {
			t.Errorf("parse(%s) = %+v; want %q, %v, %q", c.msg, m, c.id, c.isRq, c.method)
		}
	}
}
