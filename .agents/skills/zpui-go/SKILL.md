---
name: zpui-go
description: Конвенции и паттерны Go-бэкенда ZPUI (Wails v2) — биндинги App, ответы okResp/errResp, config.json, фоновые циклы и экономный режим, Win32, 32-битная сборка. Используй при написании или правке любого Go-кода — «добавь метод/эндпоинт», «переработай пакет», «не собирается» — даже если слово «Go» не произнесено.
---

# Go-бэкенд ZPUI

## Карта кода

- **Биндинги:** `internal/app/` — методы `func (a *App)` в файлах по доменам (`app_api_zapret.go`, `app_api_proxy.go`, `app_api_updates.go`, ...). Жизненный цикл — `app.go` (OnStartup/OnShutdown), окно — `window_windows.go`.
- **Доменная логика:** `internal/<pkg>/` (zapret, proxy, tgproxy, xboxdns, monitor, updater, sync, isp, config, database, logdb, reports, power, tray, notify, autostart, executil, logger).
- **Версии:** ldflags-инжекция `-X main.version` (ZPUI), `-X zpui/internal/tgproxy.Version`; source of truth — `versions.txt`.

## Новый эндпоинт — ровно два шага

1. Маршрут в карту `web/src/api.js` (`GET_ROUTES`/`POST_ROUTES`).
2. Метод `func (a *App)` в подходящем `app_api_*.go`.

Ответ — только хелперы из `internal/app/app_api_types.go`: `okResp()`, `okRespWithData(map[string]interface{})`, `errResp(msg)`. Фронт проверяет поле `error`. Формат ответов менять нельзя.

## Правила биндингов

- Параметры — простые типизированные (строки, числа, bool): `map[string]interface{}` из JS приходит **пустым**.
- Биндинг не должен блокироваться дольше ~5 сек — замрёт весь WebView. Долгая работа: горутина + прогресс через `runtime.EventsEmit`, либо запуск + отдельный `GetXxxResult` с поллингом ~2с (таймаут 120с).
- Ошибки — явно (`if err != nil`), без паник; наружу — текст в `errResp`.

## Конфиг и фоновые циклы

- `config.json` — единственный источник истины (чтение/запись через `internal/config`). Новое поле обязательно в **обоих**: `GetConfig()` и `SetConfig()` (для `notify_*` — `SetNotifyFlags()`), иначе значение молча «слетает».
- Любой новый фоновый цикл обязан уважать экономный режим (`internal/power`): интервал — `power.Stretch(base)`, выход из тика при `power.Active()` / `power.Paused()`. «Сырые» `time.Ticker` без этого — баг.
- Состояние автозапуска в config.json не хранится — источник истины система (`schtasks`).

## Стиль

- Логи — `slog` (не `fmt.Println`/`log`); структурное логирование — `internal/logger`.
- Внешние процессы — только `executil.HiddenCmd` (иначе мерцание консольных окон).
- Win32 — `golang.org/x/sys/windows`, registry; прямые вызовы API — паттерн `syscall.NewLazyDLL` (образец — `internal/power/power.go`).
- Без комментариев, где логика очевидна; если неочевидна — короткий поясняющий.
- Имена файлов: пакеты — короткие строчные; группы методов — с префиксом `app_<домен>.go`.

## Сборка — 32 бита!

Production-сборка идёт `-platform windows/386`:

- `int` = 32 бита; не тащи 64-битную специфику и переполнения на указателях.
- Зависимости — только pure-Go (без cgo): БД — `modernc.org/sqlite`. Новая зависимость, требующая cgo или amd64, сломает сборку.
- Проверяй локально: `go vet ./...`, `go test ./...`, `go build ./...`; полная сборка — `cmd //c build.bat` (frontend → Wails `-platform windows/386` → модули → `dist/<версия>/`).
