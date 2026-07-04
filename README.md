<div align="center">

# ZPUI

**Контроллер обхода DPI (Zapret) для Windows**

Управление обходом блокировок, прокси, мониторингом и DNS для Xbox — в одном приложении с автообновлением.

[Релизы](https://github.com/suzcuaru/ZPUI/releases/latest) · [Сообщить о баге](https://github.com/suzcuaru/ZPUI/issues)

</div>

---

## Возможности

| Модуль | Описание |
|--------|---------|
| **Zapret** | Запуск/остановка и настройка обхода DPI, выбор стратегии, авто-подбор рабочих параметров |
| **Прокси** | Встроенный SOCKS5-прокси для перенаправления трафика |
| **Монитор** | Наблюдение за статусом процессов и доступностью ресурсов в реальном времени |
| **Xbox DNS** | Подбор и применение оптимального DNS для Xbox |
| **Blockcheck** | Проверка доступности заблокированных ресурсов |
| **Диагностика** | Сбор системной информации и логов в один архив |
| **Автообновление** | Приложение обновляет само себя и спутники через GitHub Releases |
| **Трей** | Сворачивание в системный трей, автозапуск, уведомления Windows |

## Установка

### Установщик (рекомендуется)

Скачайте `ZPUI-Setup-x.y.z.exe` из [последнего релиза](https://github.com/suzcuaru/ZPUI/releases/latest) и запустите.

- Устанавливается **без прав администратора** в `%LOCALAPPDATA%\Programs\ZPUI`
- Создаёт ярлыки в меню «Пуск» и на рабочем столе
- Регистрируется в «Установке и удалении программ»
- Обновляется из приложения (Настройки → Обновления компонентов)

### Портативная версия

Скачайте `zpui.zip` из [релиза](https://github.com/suzcuaru/ZPUI/releases/latest), распакуйте и запустите `zpui.exe`.

> Приложению требуется запуск от имени администратора (для управления драйвером WinDivert и маршрутизацией).

## Сборка из исходников

### Требования

- [Go](https://go.dev/dl/) 1.21+
- [Node.js](https://nodejs.org/) 20+
- [Wails CLI](https://wails.io/) v2.12+

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
```

### Быстрая сборка (всё в `build/dist/`)

```bat
build.bat
```

Скрипт увеличивает версию в `version.txt`, собирает фронтенд, ядро Wails и 4 спутника, а также генерирует `versions.json`.

### Сборка вручную

```bash
# 1. Фронтенд (обязательно: бэкенд встраивает web/dist через go:embed)
cd web && npm install && npm run build && cd ..

# 2. Ядро (Wails)
wails build -platform windows/amd64 -s -skipbindings -o zpui.exe \
    -ldflags "-s -w -H windowsgui -X main.version=1.0.0" -trimpath

# 3. Спутники
go build -ldflags "-s -w -H windowsgui -X main.version=1.0.0" -trimpath ./cmd/wizard/
go build -ldflags "-s -w -H windowsgui -X main.version=1.0.0" -trimpath ./cmd/autoselect/
go build -ldflags "-s -w -H windowsgui -X main.version=1.0.0" -trimpath ./cmd/selfupdate/
go build -ldflags "-s -w -H windowsgui -X main.version=1.0.0" -trimpath ./cmd/zapretupdate/
```

## Архитектура

```
┌─────────────────────────────────────────────────────────┐
│                     Фронтенд (React)                     │
│   web/src/api.js  →  window.go.main.App.* (Wails)        │
└──────────────────────────┬──────────────────────────────┘
                           │ Wails bindings (func (a *App))
┌──────────────────────────┴──────────────────────────────┐
│              Ядро приложения (Go, package main)           │
│   app.go · app_api*.go · versions.go · main.go           │
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────┴──────────────────────────────┐
│                   internal/ (доменные пакеты)             │
│  zapret · proxy · monitor · xboxdns · blockcheck         │
│  updater · config · database · logger · notify · tray    │
│  sysinfo · executil · singleinstance · mods              │
└─────────────────────────────────────────────────────────┘
```

**Поток данных:** фронтенд вызывает `api('GET','/api/...')` → `web/src/api.js` маршрутизирует вызов в Wails-биндинг `func (a *App) ...` → метод работает с пакетами из `internal/` и возвращает `map[string]interface{}`.

### Стек

| Слой | Технологии |
|------|-----------|
| Бэкенд | Go, [Wails v2](https://wails.io/) |
| Фронтенд | React 18, Vite |
| База данных | SQLite (через modernc.org/sqlite) |
| Обход DPI | [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) (управляется как внешний процесс/служба) |

### Структура репозитория

```
app*.go, main.go, versions.go   — ядро Wails и биндинги к фронтенду
window_windows.go               — нативное окно Windows
internal/
  zapret/      — управление обходом DPI (запуск, стратегии, обновление, бэкап состояния)
  proxy/       — встроенный SOCKS5-прокси
  monitor/     — мониторинг трафика и доступности ресурсов
  xboxdns/     — настройка DNS для Xbox
  blockcheck/  — проверка доступности заблокированных ресурсов
  updater/     — автообновление ZPUI, спутников и модов + сравнение версий (semver)
  config/      — конфигурация (JSON, потокобезопасная)
  database/    — SQLite-хранилище (снимки, доступность)
  logger/      — логирование в файлы
  notify/      — Windows toast-уведомления
  tray/        — системный трей
  sysinfo/     — сбор системной информации
  executil/    — обёртки над os/exec (скрытые окна)
  singleinstance/ — защита от повторного запуска
  mods/        — реестр расширений
cmd/
  autoselect/    — авто-подбор рабочих параметров zapret
  wizard/        — мастер первоначальной установки zapret
  selfupdate/    — самообновление ZPUI
  zapretupdate/  — обновление zapret
web/            — React + Vite фронтенд (src/{pages,components,hooks,locales})
installer/      — NSIS-скрипт установщика
build/          — скрипты сборки и ресурсы (icon.ico)
mods/           — пользовательские расширения (прокси, sysinfo, xbox-dns)
```

## Разработка

### Проверка и тесты

```bash
# Backend
go vet ./...        # статический анализ
go test ./...       # unit-тесты (config, monitor)
go build ./...      # компиляция всех пакетов

# Frontend (из web/)
npm run build       # production-сборка (vite)
npm test            # vitest однократно
npm run dev         # dev-сервер на :3000
```

> `go build ./...` требует собранный фронтенд (`web/dist/`), так как бэкенд встраивает его через `//go:embed all:web/dist`.

### Добавление новой настройки/эндпоинта

1. Добавить поле в `internal/config/config.go` (`Config`) + значение по умолчанию в `defaultConfig()`.
2. Добавить метод `func (a *App) ...` в один из `app_api*.go` (биндинг Wails).
3. Зарегистрировать маршрут в `web/src/api.js`.
4. Добавить строки в `web/src/locales/{ru,en}.json` + использовать через хук `useT`.

Подробности — в [`AGENTS.md`](AGENTS.md).

## Уведомления и обновления

- На старте (через 10 с) приложение проверяет обновления ZPUI и zapret. Ошибки сети/GitHub логируются (не проглатываются молча).
- Уведомления **дедуплицируются**: тост о конкретной версии показывается один раз — повторно не появляется, пока не выйдет новая версия.
- Сравнение версий — семантическое (semver), устойчиво к `v`-префиксу, пробелам и суффиксам.
- Если локальная версия zapret не определена (`service.bat` отсутствует/неполон), ложное «доступно обновление» не показывается.

## Релизы

Сборка релиза запускается автоматически при пуше тега `v*` (например `v1.0.47`) через [GitHub Actions](.github/workflows/release.yml). Также доступен ручной запуск: вкладка **Actions → Release → Run workflow**.

Релиз включает:

- NSIS-установщик `ZPUI-Setup-x.y.z.exe`
- Портативную версию `zpui.zip`
- Манифест `versions.json` (используется механизмом автообновления)
- Исполняемые файлы спутников

## Лицензия

См. [LICENSE](LICENSE).
