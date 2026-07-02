# AGENTS.md

Guidance for AI agents (and humans) working on ZPUI.

## Project layout

- `app*.go`, `main.go`, `versions.go`, `window_windows.go` — Wails app core (package `main`). All `func (a *App)` methods are Wails bindings exposed to the frontend.
- `internal/` — backend packages by domain: `zapret`, `proxy`, `monitor`, `config`, `database`, `updater`, `xboxdns`, `blockcheck`, `sysinfo`, `tray`, `logger`, `executil`, `autostart`, `singleinstance`, `mods`.
- `cmd/{autoselect,selfupdate,wizard,zapretupdate}` — satellite executables.
- `web/` — React + Vite frontend. `web/src/api.js` is a shim that routes `api('GET','/api/...')` calls to Wails bindings (`window.go.main.App.*`).

## Build & verify commands

### Backend (Go)

```bash
go vet ./...        # static checks — must pass
go test ./...       # unit tests (config, monitor, zapret)
go build ./...      # compile all packages
```

Run these from the repo root after any Go change.

### Frontend (web/)

Run from `web/`:

```bash
npm install
npm run build       # production build (vite) — must pass
npm test            # run vitest once
npm run test:watch  # vitest in watch mode
npm run dev         # dev server on :3000
```

### Full release build

```bash
build.bat           # bumps version, builds frontend + Wails core + 4 satellites → build/dist/
```

Requires `go`, `node`, and the `wails` CLI on PATH.

## Conventions

- Frontend → backend calls go through `web/src/api.js` (route map). Add new endpoints there and as a `func (a *App)` method.
- Shared frontend logic lives in `web/src/hooks/` (`usePolling`, `useDebouncedSave`, `useServiceToggle`) and `web/src/components/ui/` (`Row`, `Switch`, `Modal`).
- All user-facing strings go through i18n (`web/src/locales/{ru,en}.json` + `useT`). Non-React modules use the `tr()` accessor from `i18n`.
- Backend responses use `map[string]interface{}` with `{"error": "..."}` / `{"status": "ok"}`; helpers `errResp()` / `okResp()` live in `app_api_types.go`.
- No comments unless explaining non-obvious logic.
