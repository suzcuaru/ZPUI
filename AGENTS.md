# AGENTS.md

Guidance for AI agents working on ZPUI.

## Что это

ZPUI v0.0.0 — модульная оболочка (shell). Ядро = UI + трей + система модулей. Весь функционал — внешние модули (отдельные `.exe` в `modules/`).

## Layout

- `main.go` — точка входа: single-instance, config, logger, modules manager, tray, Wails.
- `internal/app/` — ядро Wails: `App` struct, lifecycle (`Startup`/`Shutdown`/`BeforeClose`), контроллер окна/трея, API-биндинги (`func (a *App)`).
- `internal/modules/` — система модулей: `Discover()` (скан `modules/*/module.json`), `Manager` (запуск/стоп процессов).
- `internal/config/`, `internal/logger/`, `internal/singleinstance/`, `internal/executil/`, `internal/tray/` — инфраструктура.
- `web/` — React + Vite фронтенд. `web/src/api.js` — shim маршрутов → Wails-биндинги.

## Команды проверки

```bash
# Бэкенд
go vet ./...
go build ./...

# Фронтенд (из web/)
npm install
npm run build

# Полная сборка
wails build -Platform windows/amd64 -s -trimpath
```

> `go build ./...` требует собранный фронтенд (`web/dist/`), т.к. бэкенд встраивает его через `//go:embed all:web/dist`.
> Сборка идёт под `windows/amd64` (`-Platform windows/amd64`).

## Конвенции

- Все эндпоинты — методы `func (a *App)` в `internal/app/`; маршрут добавляется в `web/src/api.js`.
- Ответы бэкенда: `okResp()` / `errResp()` (в `internal/app/api.go`).
- Строки — через i18n (`web/src/locales/{ru,en}.json` + хук `useT`).
- Без комментариев, кроме неочевидной логики.

## Добавление модуля (в рантайме, без сборки ядра)

Положить папку с `module.json` + `.exe` в `modules/`. См. `MODULES.md`.
