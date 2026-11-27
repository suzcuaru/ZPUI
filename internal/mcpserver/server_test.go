package mcpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer() *Server {
	return New(Options{
		Version:     "2.1.0-test",
		ExtensionID: "abcdefghijklmnop",
		Key:         func() string { return "aabbcc" },
		Tools: func() []Tool {
			return []Tool{{Name: "echo", Description: "эхо", InputSchema: map[string]interface{}{"type": "object"}}}
		},
		Dispatch: func(name string, args map[string]interface{}) (interface{}, error) {
			switch name {
			case "boom":
				return nil, fmt.Errorf("boom happened")
			case "echo":
				return map[string]interface{}{"echo": args["v"]}, nil
			}
			return nil, fmt.Errorf("неизвестный инструмент: %s", name)
		},
	})
}

func post(s *Server, key, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8930/mcp", strings.NewReader(body))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	s.handleMCP(rr, req)
	return rr
}

func resultMap(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, rr.Body.String())
	}
	return m
}

func TestAuth(t *testing.T) {
	s := newTestServer()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`

	if rr := post(s, "", body, nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("no key: want 401, got %d", rr.Code)
	}
	if rr := post(s, "wrong", body, nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: want 401, got %d", rr.Code)
	}
	if rr := post(s, "aabbcc", body, map[string]string{"Origin": "http://evil.com"}); rr.Code != http.StatusForbidden {
		t.Fatalf("evil origin: want 403, got %d", rr.Code)
	}
	if rr := post(s, "aabbcc", body, map[string]string{"Origin": "http://127.0.0.1:5173"}); rr.Code != http.StatusOK {
		t.Fatalf("local origin: want 200, got %d", rr.Code)
	}
	// Origin расширения Chromium (Chrome/Яндекс/Opera/Edge) — допуск по ключу.
	if rr := post(s, "aabbcc", body, map[string]string{"Origin": "chrome-extension://abcdefghijklmnop"}); rr.Code != http.StatusOK {
		t.Fatalf("extension origin: want 200, got %d", rr.Code)
	}
	// Своё расширение (точный ID) — без ключа.
	rrOwn := post(s, "", body, map[string]string{"Origin": "chrome-extension://abcdefghijklmnop"})
	if rrOwn.Code != http.StatusOK {
		t.Fatalf("own extension without key: want 200, got %d", rrOwn.Code)
	}
	if got := rrOwn.Header().Get("Access-Control-Allow-Origin"); got != "chrome-extension://abcdefghijklmnop" {
		t.Fatalf("own extension: want CORS origin for extension, got %q", got)
	}
	// Чужое расширение без ключа не проходит: ID подделать нельзя.
	if rr := post(s, "", body, map[string]string{"Origin": "chrome-extension://zzzzzzzzzzzzzzzz"}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("other extension without key: want 401, got %d", rr.Code)
	}
	// Префлайт своего расширения закрыт.
	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", "chrome-extension://abcdefghijklmnop")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rr0 := httptest.NewRecorder()
	s.handleMCP(rr0, req)
	if rr0.Code != http.StatusNoContent || rr0.Header().Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatalf("extension preflight: want 204 + private network, got %d %q", rr0.Code, rr0.Header().Get("Access-Control-Allow-Private-Network"))
	}
	if rr := post(s, "aabbcc", body, nil); rr.Code != http.StatusOK {
		t.Fatalf("valid key: want 200, got %d", rr.Code)
	}
}

func TestGetNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8930/mcp", nil)
	req.Header.Set("Authorization", "Bearer aabbcc")
	rr := httptest.NewRecorder()
	s.handleMCP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: want 405, got %d", rr.Code)
	}
}

func TestInitialize(t *testing.T) {
	s := newTestServer()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"zcode","version":"1.0"}}}`
	rr := post(s, "aabbcc", body, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	m := resultMap(t, rr)
	res, _ := m["result"].(map[string]interface{})
	if res == nil {
		t.Fatalf("no result: %s", rr.Body.String())
	}
	if res["protocolVersion"] != mcpProtocolVersion {
		t.Fatalf("protocolVersion: %v", res["protocolVersion"])
	}
	info, _ := res["serverInfo"].(map[string]interface{})
	if info == nil || info["name"] != "zpui" {
		t.Fatalf("serverInfo: %v", res["serverInfo"])
	}
	sess := s.Sessions()
	if len(sess) != 1 || sess[0].Name != "zcode" {
		t.Fatalf("session not registered: %+v", sess)
	}
}

func TestNotificationAccepted(t *testing.T) {
	s := newTestServer()
	rr := post(s, "aabbcc", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	if rr.Code != http.StatusAccepted || rr.Body.Len() != 0 {
		t.Fatalf("notification: want 202 empty, got %d (%q)", rr.Code, rr.Body.String())
	}
}

func TestToolsList(t *testing.T) {
	s := newTestServer()
	rr := post(s, "aabbcc", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, nil)
	m := resultMap(t, rr)
	res, _ := m["result"].(map[string]interface{})
	tools, _ := res["tools"].([]interface{})
	if len(tools) != 1 {
		t.Fatalf("tools: %v", res["tools"])
	}
	tool, _ := tools[0].(map[string]interface{})
	if tool["name"] != "echo" || tool["inputSchema"] == nil {
		t.Fatalf("tool entry: %v", tool)
	}
}

func TestToolsCall(t *testing.T) {
	s := newTestServer()
	rr := post(s, "aabbcc", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"v":"привет"}}}`, nil)
	m := resultMap(t, rr)
	res, _ := m["result"].(map[string]interface{})
	content, _ := res["content"].([]interface{})
	if len(content) != 1 || res["isError"] == true {
		t.Fatalf("result: %s", rr.Body.String())
	}
	item, _ := content[0].(map[string]interface{})
	var echoed map[string]interface{}
	if err := json.Unmarshal([]byte(item["text"].(string)), &echoed); err != nil {
		t.Fatalf("content text is not JSON: %v", err)
	}
	if echoed["echo"] != "привет" {
		t.Fatalf("echo payload: %v", echoed)
	}

	rr = post(s, "aabbcc", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"boom"}}`, nil)
	m = resultMap(t, rr)
	res, _ = m["result"].(map[string]interface{})
	if res["isError"] != true || !strings.Contains(fmt.Sprint(res["content"]), "boom happened") {
		t.Fatalf("tool error: %s", rr.Body.String())
	}

	rr = post(s, "aabbcc", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope"}}`, nil)
	m = resultMap(t, rr)
	res, _ = m["result"].(map[string]interface{})
	if res["isError"] != true {
		t.Fatalf("unknown tool: %s", rr.Body.String())
	}
}

func TestUnknownMethod(t *testing.T) {
	s := newTestServer()
	rr := post(s, "aabbcc", `{"jsonrpc":"2.0","id":6,"method":"resources/list"}`, nil)
	m := resultMap(t, rr)
	e, _ := m["error"].(map[string]interface{})
	if e == nil || e["code"] != float64(errNoMethod) {
		t.Fatalf("error: %s", rr.Body.String())
	}
}

func TestParseError(t *testing.T) {
	s := newTestServer()
	rr := post(s, "aabbcc", `{"jsonrpc":"2.0","id":7,"metho`, nil)
	m := resultMap(t, rr)
	e, _ := m["error"].(map[string]interface{})
	if e == nil || e["code"] != float64(errParse) {
		t.Fatalf("parse error: %s", rr.Body.String())
	}
}

func TestSSEResponse(t *testing.T) {
	s := newTestServer()
	rr := post(s, "aabbcc", `{"jsonrpc":"2.0","id":8,"method":"ping"}`,
		map[string]string{"Accept": "application/json, text/event-stream"})
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type: %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "event: message") || !strings.Contains(rr.Body.String(), `"result":{}`) {
		t.Fatalf("sse body: %q", rr.Body.String())
	}
}

func TestSessionReap(t *testing.T) {
	r := newSessionRegistry()
	now := time.Now()
	r.now = func() time.Time { return now }
	r.register("1.2.3.4:1", "agent")
	if got := len(r.snapshot()); got != 1 {
		t.Fatalf("want 1 session, got %d", got)
	}
	r.touch("1.2.3.4:1")
	now = now.Add(6 * time.Minute)
	if got := len(r.snapshot()); got != 0 {
		t.Fatalf("session must be reaped, got %d", got)
	}
}
