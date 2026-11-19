// Package themes — хранилище кастомных тем оформления (галерея «Свои темы»).
// Слоты хранятся одним JSON-файлом рядом с config.json; активный слот
// связан с config.theme = "custom".
package themes

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("theme_not_found")
	ErrActive   = errors.New("theme_active")
)

// Slot — один слот галереи: имя + нормализованная тема (объект в формате
// фронтендового движка тем).
type Slot struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Theme map[string]any `json:"theme"`
}

type storeData struct {
	Themes   []Slot `json:"themes"`
	ActiveID string `json:"activeId"`
}

// Store — потокобезопасное хранилище слотов.
type Store struct {
	mu   sync.Mutex
	path string
	data storeData
}

func Load(path string) *Store {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var d storeData
	if err := json.Unmarshal(b, &d); err != nil {
		return s
	}
	clean := make([]Slot, 0, len(d.Themes))
	for _, sl := range d.Themes {
		if sl.ID == "" {
			continue
		}
		clean = append(clean, sl)
	}
	s.data = storeData{Themes: clean, ActiveID: d.ActiveID}
	return s
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

func newID() string {
	return "theme-" + strconv.FormatInt(time.Now().UnixNano(), 36) +
		strconv.FormatInt(time.Now().UnixNano()%1296, 36)
}

// List возвращает копию слотов (свежие объекты — React сравнивает по ссылке).
func (s *Store) List() []Slot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Slot, len(s.data.Themes))
	copy(out, s.data.Themes)
	return out
}

// ActiveID — id активного слота ("" — ни один не активен).
func (s *Store) ActiveID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.ActiveID
}

// Active — активный слот или nil.
func (s *Store) Active() *Slot {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Themes {
		if s.data.Themes[i].ID == s.data.ActiveID {
			cp := s.data.Themes[i]
			return &cp
		}
	}
	return nil
}

// Upsert создаёт или обновляет слот по id (пустой id — создание).
// Возвращает id слота.
func (s *Store) Upsert(id, name string, theme map[string]any) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name = strings.TrimSpace(name); name == "" && theme == nil && id == "" {
		return ""
	}
	if id != "" {
		for i := range s.data.Themes {
			if s.data.Themes[i].ID == id {
				if name != "" {
					s.data.Themes[i].Name = name
				}
				if theme != nil {
					s.data.Themes[i].Theme = theme
				}
				s.saveLocked()
				return id
			}
		}
	}
	if id == "" {
		id = newID()
	}
	slot := Slot{ID: id, Name: strings.TrimSpace(name), Theme: theme}
	if slot.Theme == nil {
		slot.Theme = map[string]any{}
	}
	s.data.Themes = append(s.data.Themes, slot)
	s.saveLocked()
	return id
}

// Activate делает слот активным. ErrNotFound — слота нет.
func (s *Store) Activate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Themes {
		if s.data.Themes[i].ID == id {
			s.data.ActiveID = id
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

// Delete удаляет слот. Активный удалять нельзя (ErrActive), отсутствующий — ErrNotFound.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == s.data.ActiveID {
		return ErrActive
	}
	for i := range s.data.Themes {
		if s.data.Themes[i].ID == id {
			s.data.Themes = append(s.data.Themes[:i], s.data.Themes[i+1:]...)
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

// ClearActive снимает активный слот (вызывается при переключении на обычную тему).
func (s *Store) ClearActive() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.ActiveID == "" {
		return
	}
	s.data.ActiveID = ""
	s.saveLocked()
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}
