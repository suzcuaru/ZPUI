// Package bgstore — хранилище фоновых картинок кастомных тем.
// Файлы лежат в <dataDir>/bg, фронтенд работает с ними через HTTP-пути
// /api/bg/{list,upload,delete} и /bg/<файл> (в дев-режиме то же эмулирует
// vite-мидлварь, в проде — Handler из этого пакета, подключённый к
// Wails assetserver).
package bgstore

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var extRe = regexp.MustCompile(`(?i)\.(jpe?g|png|webp|gif)$`)

// SafeName возвращает чистое имя файла (basename) с допустимым расширением
// или "" — пути наверх и посторонние расширения отсекаются.
func SafeName(raw string) string {
	base := filepath.Base(strings.TrimSpace(raw))
	if base == "." || base == ".." || base == "/" || base == `\` {
		return ""
	}
	if !extRe.MatchString(base) {
		return ""
	}
	return base
}

func Path(name string) string { return "/bg/" + url.PathEscape(name) }

func dir(dataDir string) string { return filepath.Join(dataDir, "bg") }

func list(dataDir string) []map[string]string {
	entries, err := os.ReadDir(dir(dataDir))
	if err != nil {
		return []map[string]string{}
	}
	out := make([]map[string]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !extRe.MatchString(e.Name()) {
			continue
		}
		out = append(out, map[string]string{"name": e.Name(), "path": Path(e.Name())})
	}
	return out
}

func contentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	}
	return "application/octet-stream"
}

// Handler обслуживает GET/POST /api/bg/* и GET /bg/<файл>.
func Handler(dataDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON := func(code int, obj any) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(obj)
		}
		p := r.URL.Path

		switch {
		case r.Method == http.MethodGet && (p == "/api/bg" || p == "/api/bg/" || p == "/api/bg/list"):
			if err := os.MkdirAll(dir(dataDir), 0o755); err != nil {
				writeJSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(http.StatusOK, map[string]any{"files": list(dataDir)})

		case r.Method == http.MethodPost && p == "/api/bg/upload":
			// Сырое тело + имя файла в X-Filename (URL-encoded), как ThemeEditor/ThemeZip.
			raw := r.Header.Get("X-Filename")
			if dec, err := url.QueryUnescape(raw); err == nil {
				raw = dec
			}
			name := SafeName(raw)
			if name == "" {
				writeJSON(http.StatusBadRequest, map[string]string{"error": "bad_file"})
				return
			}
			data, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
			if err != nil || len(data) == 0 {
				writeJSON(http.StatusBadRequest, map[string]string{"error": "bad_file"})
				return
			}
			if err := os.MkdirAll(dir(dataDir), 0o755); err != nil {
				writeJSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if err := os.WriteFile(filepath.Join(dir(dataDir), name), data, 0o644); err != nil {
				writeJSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(http.StatusOK, map[string]any{"ok": true, "name": name, "path": Path(name)})

		case r.Method == http.MethodPost && p == "/api/bg/delete":
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(http.StatusBadRequest, map[string]string{"error": "bad_name"})
				return
			}
			name := SafeName(body.Name)
			if name == "" {
				writeJSON(http.StatusBadRequest, map[string]string{"error": "bad_name"})
				return
			}
			os.Remove(filepath.Join(dir(dataDir), name))
			writeJSON(http.StatusOK, map[string]any{"ok": true})

		case r.Method == http.MethodGet && strings.HasPrefix(p, "/bg/"):
			name := SafeName(strings.TrimPrefix(p, "/bg/"))
			if name == "" {
				http.NotFound(w, r)
				return
			}
			f, err := os.Open(filepath.Join(dir(dataDir), name))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()
			w.Header().Set("Content-Type", contentType(name))
			w.Header().Set("Cache-Control", "no-store")
			io.Copy(w, f)

		default:
			writeJSON(http.StatusNotFound, map[string]string{"error": "not_found"})
		}
	})
}
