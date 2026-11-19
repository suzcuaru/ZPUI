package mcpserver

import (
	"sort"
	"sync"
	"time"
)

// Session — подключение ИИ-агента: имя из initialize + адрес + активность.
const sessionIdleTTL = 5 * time.Minute

type Session struct {
	Name     string    `json:"name"`
	Addr     string    `json:"addr"`
	Since    time.Time `json:"since"`
	Last     time.Time `json:"last"`
	Requests int64     `json:"requests"`
}

type sessionRegistry struct {
	mu  sync.Mutex
	m   map[string]*Session
	now func() time.Time
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{m: make(map[string]*Session), now: time.Now}
}

// touch обновляет активность адреса (каждый авторизованный запрос).
func (r *sessionRegistry) touch(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.m[addr]; ok {
		s.Last = r.now()
		s.Requests++
	}
}

// register создаёт/обновляет сессию при initialize.
func (r *sessionRegistry) register(addr, client string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reapLocked()
	if s, ok := r.m[addr]; ok {
		s.Name = client
		s.Last = r.now()
		return
	}
	now := r.now()
	r.m[addr] = &Session{Name: client, Addr: addr, Since: now, Last: now, Requests: 1}
}

func (r *sessionRegistry) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m = make(map[string]*Session)
}

func (r *sessionRegistry) snapshot() []Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reapLocked()
	out := make([]Session, 0, len(r.m))
	for _, s := range r.m {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Since.Before(out[j].Since) })
	return out
}

func (r *sessionRegistry) reapLocked() {
	cutoff := r.now().Add(-sessionIdleTTL)
	for k, s := range r.m {
		if s.Last.Before(cutoff) {
			delete(r.m, k)
		}
	}
}
