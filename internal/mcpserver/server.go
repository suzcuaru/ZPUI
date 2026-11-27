package mcpserver

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Tool — описание инструмента MCP (tools/list) с JSON Schema аргументов.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// Handler — исполнение tools/call: возвращает JSON-сериализуемый результат.
// Ошибка трактуется как ошибка инструмента (isError в результате вызова).
type Handler func(args map[string]interface{}) (interface{}, error)

// Options — зависимости сервера. Всё колбэками: пакет не импортирует
// internal/app (нет цикла), приложение само собирает реестр инструментов.
type Options struct {
	Version string
	Key     func() string // актуальный Bearer-ключ из config.json
	// ExtensionID — ID браузерного расширения ZPUI (выводится из ключа
	// подписи пакета). Запросы с ровно таким Origin допускаются без ключа:
	// браузер не даёт страницам подделать Origin, а другой расширение с тем же
	// ID выпустить нельзя. Пусто — только Bearer-ключ.
	ExtensionID string
	Tools       func() []Tool
	Dispatch    func(name string, args map[string]interface{}) (interface{}, error)
	Notify      func(level, msg string) // nil = молча
}

// Server — локальный streamable-HTTP MCP-сервер. Только 127.0.0.1,
// stateless JSON-RPC (SSE-ответ одним event'ом — клиентам, требующим
// text/event-stream).
type Server struct {
	opts     Options
	mu       sync.Mutex
	ln       net.Listener
	http     *http.Server
	port     int
	running  bool
	lastErr  string
	sessions *sessionRegistry
}

func New(o Options) *Server {
	return &Server{opts: o, sessions: newSessionRegistry()}
}

// Start слушает 127.0.0.1:port. Повторный вызов на работающем сервере
// сначала останавливает старый слушатель (перезапуск на новом порту).
func (s *Server) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = ""
	if s.running {
		s.stopLocked()
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.lastErr = err.Error()
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.handleMCP)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	s.ln, s.http, s.port, s.running = ln, srv, port, true
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.running, s.lastErr = false, err.Error()
			s.mu.Unlock()
			s.logf("warn", "mcp server stopped: "+err.Error())
		}
	}()
	s.logf("info", "mcp server listening on "+addr)
	return nil
}

// Stop останавливает сервер (идемпотентно).
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

func (s *Server) stopLocked() {
	if !s.running || s.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.http.Shutdown(ctx)
	s.ln = nil
	s.http = nil
	s.running = false
}

func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

func (s *Server) LastError() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// Sessions — снимок активных сессий агентов (для модалки).
func (s *Server) Sessions() []Session {
	return s.sessions.snapshot()
}

// ClearSessions — сброс сессий (регенерация ключа).
func (s *Server) ClearSessions() {
	s.sessions.clear()
}

func (s *Server) logf(level, msg string) {
	if s.opts.Notify != nil {
		s.opts.Notify(level, msg)
	}
}

// authorize — Bearer-ключ в константном времени; запрет чужих Origin
// (DNS-rebinding): браузерная страница с чужого origin не должна уметь
// дёргать локальный сервер. Исключение — расширения Chromium
// (chrome-extension://): fetch из MV3-воркера всегда шлёт такой Origin,
// доступ по-прежнему только по Bearer-ключу.
func (s *Server) authorize(r *http.Request) (int, bool) {
	if o := r.Header.Get("Origin"); o != "" && !isLocalOrigin(o) && !isExtensionOrigin(o) {
		return http.StatusForbidden, false
	}
	// Своё расширение: Origin chrome-extension://<ID> вместо ключа.
	if s.isOwnExtension(r) {
		return 0, true
	}
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) <= len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return http.StatusUnauthorized, false
	}
	key := ""
	if s.opts.Key != nil {
		key = s.opts.Key()
	}
	if key == "" {
		return http.StatusUnauthorized, false
	}
	given := auth[len(prefix):]
	if subtle.ConstantTimeCompare([]byte(given), []byte(key)) == 1 {
		return 0, true
	}
	return http.StatusUnauthorized, false
}

// extensionOrigin — origin своего расширения: chrome-extension://<ID>.
func (s *Server) extensionOrigin() string {
	if s.opts.ExtensionID == "" {
		return ""
	}
	return "chrome-extension://" + s.opts.ExtensionID
}

func (s *Server) isOwnExtension(r *http.Request) bool {
	want := s.extensionOrigin()
	return want != "" && strings.EqualFold(r.Header.Get("Origin"), want)
}

// corsExtension — заголовки для своего расширения: ответ читается фоном,
// preflight закрыт (в т.ч. Private Network Access к 127.0.0.1).
func (s *Server) corsExtension(w http.ResponseWriter) {
	origin := s.extensionOrigin()
	if origin == "" {
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
	h.Set("Access-Control-Allow-Private-Network", "true")
	h.Set("Vary", "Origin")
}

func isLocalOrigin(origin string) bool {
	o := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://"))
	host := o
	if i := strings.IndexByte(o, ':'); i >= 0 {
		host = o[:i]
	}
	if strings.HasSuffix(host, "]") && strings.LastIndexByte(host, '[') >= 0 {
		host = strings.Trim(host, "[]")
	}
	return host == "127.0.0.1" || host == "localhost" || host == "::1" || host == "[::1]"
}

// isExtensionOrigin — origin расширения Chromium: схема chrome-extension://<id>
// одинакова во всех форках (Chrome, Яндекс, Opera, Edge). Расширения —
// доверенные локальные клиенты: пропуск на уровне Origin, авторизация
// остаётся за Bearer-ключом.
func isExtensionOrigin(origin string) bool {
	return strings.HasPrefix(strings.ToLower(origin), "chrome-extension://")
}
