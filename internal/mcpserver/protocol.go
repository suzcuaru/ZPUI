package mcpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// JSON-RPC 2.0 поверх streamable HTTP (спецификация MCP). Stateless:
// Mcp-Session-Id не выдаётся; на POST с Accept: text/event-stream ответ
// уходит одним SSE-event'ом — так требуют некоторые клиенты.

const mcpProtocolVersion = "2025-06-18"

const (
	errParse    = -32700
	errNoMethod = -32601
	errBadParams = -32602
	errInternal  = -32603
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ClientInfo      clientInfo `json:"clientInfo"`
}

type callParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
	case http.MethodGet, http.MethodDelete:
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	default:
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if status, ok := s.authorize(r); !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="zpui-mcp"`)
		http.Error(w, http.StatusText(status), status)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Method == "" {
		writeRPC(w, r, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: errParse, Message: "parse error"}})
		return
	}

	// Уведомления (без id) не отвечают — 202 Accepted с пустым телом.
	if req.ID == nil {
		s.logf("info", "mcp notification: "+req.Method)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := s.dispatchRPC(&req, r)
	writeRPC(w, r, resp)
}

func (s *Server) dispatchRPC(req *rpcRequest, r *http.Request) rpcResponse {
	base := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p initParams
		_ = json.Unmarshal(req.Params, &p)
		name := p.ClientInfo.Name
		if name == "" {
			name = "agent"
		}
		s.sessions.register(r.RemoteAddr, name)
		s.logf("info", fmt.Sprintf("mcp agent connected: %s (%s)", name, r.RemoteAddr))
		base.Result = map[string]interface{}{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo": map[string]interface{}{
				"name":    "zpui",
				"version": s.opts.Version,
				"title":   "ZPUI",
			},
			"instructions": "ZPUI — управление обходом блокировок (движок zapret), DNS, SOCKS-прокси и аналитикой доступности.",
		}
		return base
	case "ping":
		base.Result = map[string]interface{}{}
		return base
	case "tools/list":
		tools := []Tool{}
		if s.opts.Tools != nil {
			tools = s.opts.Tools()
		}
		if tools == nil {
			tools = []Tool{}
		}
		base.Result = map[string]interface{}{"tools": tools}
		return base
	case "tools/call":
		var p callParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			base.Error = &rpcError{Code: errBadParams, Message: "invalid params: name required"}
			return base
		}
		if s.opts.Dispatch == nil {
			base.Error = &rpcError{Code: errInternal, Message: "no tools"}
			return base
		}
		if p.Arguments == nil {
			p.Arguments = map[string]interface{}{}
		}
		s.sessions.touch(r.RemoteAddr)
		res, err := s.opts.Dispatch(p.Name, p.Arguments)
		if err != nil {
			base.Result = map[string]interface{}{
				"content": []map[string]interface{}{{"type": "text", "text": err.Error()}},
				"isError": true,
			}
			return base
		}
		text, ok := res.(string)
		if !ok {
			b, _ := json.Marshal(res)
			text = string(b)
		}
		base.Result = map[string]interface{}{
			"content": []map[string]interface{}{{"type": "text", "text": text}},
		}
		return base
	default:
		base.Error = &rpcError{Code: errNoMethod, Message: "method not found: " + req.Method}
		return base
	}
}

// writeRPC — application/json, либо один event text/event-stream, если
// клиент ожидает SSE (часть MCP-клиентов читает ответ только стримом).
func writeRPC(w http.ResponseWriter, r *http.Request, resp rpcResponse) {
	b, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "marshal error", http.StatusInternalServerError)
		return
	}
	for _, ct := range r.Header.Values("Accept") {
		if containsToken(ct, "text/event-stream") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func containsToken(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if i := strings.IndexByte(part, ';'); i >= 0 {
			part = strings.TrimSpace(part[:i])
		}
		if strings.EqualFold(part, token) {
			return true
		}
	}
	return false
}
