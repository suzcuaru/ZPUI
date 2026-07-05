# Структура проекта ZPUI

Полное описание файловой структуры приложения ZPUI — контроллера обхода DPI через Zapret.
Документ описывает назначение каждой папки и ключевого файла, архитектуру и поток данных.

---

## 1. Обзор архитектуры

ZPUI — это **Wails v2** приложение: десктопный GUI на связке **Go (бэкенд)** + **React/Vite (фронтенд)**,
упакованные в один исполняемый файл `zpui.exe`.

```
┌──────────────────────────────────────────────────────────┐
│  zpui.exe (Wails)                                        │
│                                                          │
│  ┌────────────────────┐   IPC (Wails Events)   ┌────────┐│
│  │  React Frontend    │ ◄──────────────────► │  Go    ││
│  │  (web/, embed)     │   window.go.app.App.* │ Backend││
│  │  api.js shim       │ ──────────────────►   │(internal)│
│  └────────────────────┘                        └────────┘│
│                         запускает (отдельные процессы)    │
│                         selfupdate.exe / zapretupdate.exe │
└──────────────────────────────────────────────────────────┘
```

- **Фронтенд** (`web/`) встраивается в бинарник через `//go:embed all:web/dist`.
- **Связь**: фронтенд вызывает `api('GET', '/api/...')` → `web/src/api.js` маршрутизирует
  вызов на Wails-биндинг `window.go.app.App.<Method>()` → метод `func (a *App)` в `internal/app/`.
- **Сателлиты** (`cmd/`) — отдельные `.exe` для задач, требующих работы вне главного процесса
  (обновление, восстановление). wizard/autoselect теперь работают **внутрипроцессно** через пакеты `internal/wizard`, `internal/autoselect`.

---

## 2. Дерево проекта

```
ZPUI/
├── main.go                     # точка входа (package main) — создаёт и биндит app.App
├── go.mod / go.sum             # Go-модуль zpui
├── wails.json                  # конфиг Wails (имя, версия, frontend:dir)
├── build.bat                   # скрипт полного релизного билда
├── version.txt                 # текущая версия (читается/пишется build.bat)
├── config.example.json         # пример конфигурации
├── .gitignore
├── LICENSE
├── README.md
├── AGENTS.md                   # гайд для AI-агентов
├── STRUCTURE.md                # этот файл
│
├── internal/                   # бэкенд-пакеты Go
│   ├── app/                    # ★ ядро Wails-приложения (package app)
│   ├── wizard/                 # мастер первичной настройки
│   ├── autoselect/             # движок автоподбора стратегии
│   ├── zapret/                 # управление Zapret (DPI-обход)
│   ├── proxy/                  # SOCKS5-прокси
│   ├── monitor/                # мониторинг трафика
│   ├── config/                 # конфигурация (потокобезопасная)
│   ├── database/               # SQLite (zpui.db)
│   ├── updater/                # система обновлений
│   ├── xboxdns/                # настройка DNS для Xbox
│   ├── blockcheck/             # проверка блокировок
│   ├── sysinfo/                # системная информация
│   ├── tray/                   # системный трей
│   ├── notify/                 # Windows-уведомления (toast)
│   ├── logger/                 # ротируемый логгер
│   ├── executil/               # скрытый запуск процессов (Windows)
│   ├── singleinstance/         # защита от повторного запуска
│   └── mods/                   # реестр JS-модов
│
├── cmd/                        # сателлиты (отдельные .exe)
│   ├── selfupdate/             # самообновление ZPUI
│   ├── zapretupdate/           # обновление Zapret
│   ├── wizard/                 # CLI-обёртка над internal/wizard
│   └── autoselect/             # CLI-обёртка над internal/autoselect
│
├── web/                        # фронтенд (React + Vite)
│   ├── src/                    # исходники
│   ├── public/                 # статика
│   ├── wailsjs/                # Wails runtime (JS)
│   ├── package.json            # зависимости npm
│   ├── vite.config.js          # конфиг Vite
│   └── vitest.setup.js         # конфиг тестов
│
├── mods/                       # JS-моды (плагины)
│   ├── proxy/
│   ├── sysinfo/
│   └── xbox-dns/
│
├── build/                      # ресурсы и вывод сборки Wails
│   ├── appicon.png
│   └── windows/                # иконка, манифест, info.json
│
├── installer/                  # NSIS-инсталлятор
│   └── ZPUI.nsi
│
├── assets/                     # иконки SVG
│   └── icon.svg
│
└── .github/workflows/          # CI/CD
    ├── ci.yml                  # проверка (vet/test/build)
    └── release.yml             # сборка релиза
```

---

## 3. Корневые файлы

| Файл | Назначение |
|------|-----------|
| `main.go` | Точка входа (`package main`). Проверяет права админа, инициализирует логгер/БД/менеджеры, создаёт `app.NewApp(...)`, регистрирует трей и запускает Wails. |
| `go.mod` / `go.sum` | Описание Go-модуля `zpui` и зависимости. |
| `wails.json` | Конфигурация Wails: `outputfilename`, `frontend:dir`, `productVersion`. |
| `build.bat` | Релизная сборка: бамп версии → frontend → Wails-core → 4 сателлита → `build/dist/`. |
| `version.txt` | Текущая версия `X.Y.Z` (автоинкремент `build.bat`). |
| `config.example.json` | Пример конфигурации (настройки, список хостов и т.д.). |
| `AGENTS.md` | Гайдлайн для AI-агентов и контрибьюторов. |

---

## 4. `internal/` — бэкенд-пакеты

### `internal/app/` — ядро Wails-приложения (`package app`)
Структура `App` и все методы `func (a *App)` — это Wails-биндинги, доступные фронтенду
как `window.go.app.App.*`.

| Файл | Содержимое |
|------|-----------|
| `app.go` | Структура `App`, `NewApp()`, lifecycle `Startup`/`Shutdown`/`BeforeClose`, фоновые воркеры, проверка обновлений на старте. |
| `app_api.go` | Базовые эндпоинты (`GetStatus`, ресурсы, сеть). |
| `app_api_config.go` | Чтение/сохранение конфига. |
| `app_api_logs.go` | API логов (категории, архивы, срезы ошибок). |
| `app_api_proxy.go` | Управление прокси. |
| `app_api_system.go` | Система: автозапуск, бэкапы, игнорируемые версии, компоненты, ресурсы. |
| `app_api_types.go` | Хелперы ответов `errResp()` / `okResp()`, типы. |
| `app_api_xboxdns.go` | API Xbox DNS. |
| `app_api_zapret.go` | Управление Zapret (start/stop/strategies), `RunWizard()` (внутрипроцессно с эмитом `wizard:progress`). |
| `app_devices.go` | Трекер устройств, `RunAutoSelectStream()`, `RunUpdateStream()`. |
| `app_diag.go` | Диагностика соединения. |
| `app_i18n.go` | Бэкенд-локализация (`tr()`). |
| `versions.go` | `GetVersions`, `CheckZPUIUpdate`, `CheckComponentUpdates`, `UpdateComponent`. |
| `window_windows.go` | Управление окном: сворачивание в трей при закрытии. |

### Доменные пакеты

| Пакет | Назначение |
|-------|-----------|
| `internal/zapret/` | Управление Zapret: установка/удаление службы Windows, стратегии, автотест (`RunAutoTest`, `AutoSelectAndApply`), версия, проверка/обновление. Главный домен. |
| `internal/wizard/` | Мастер первичной настройки: проверка git → клон Zapret → определение ISP → автоподбор стратегии. Внутрипроцессный, вызывается из `App.RunWizard`. |
| `internal/autoselect/` | Движок автоподбора стратегии (обёртка над `zapret.AutoSelectAndApply`): тестирует → сортирует по скору → применяет с проверкой. Используется wizard + CLI. |
| `internal/proxy/` | SOCKS5-прокси-сервер. |
| `internal/monitor/` | Мониторинг трафика (скорость, снимки). |
| `internal/xboxdns/` | Настройка DNS для Xbox. |
| `internal/blockcheck/` | Проверка блокировок ресурсов (DPI). |
| `internal/updater/` | Система обновлений: запрос релизов GitHub (ETag-кеш `cache.go`), проверка компонентов, замена сателлитов, обновление модов, менеджер бэкапов, сравнение версий. |
| `internal/mods/` | Реестр JS-модов: сканирование `mods/`, манифесты `mod.json`, health-check, проверка обновлений модов. |

### Инфраструктурные пакеты

| Пакет | Назначение |
|-------|-----------|
| `internal/config/` | Загрузка/сохранение `config.json`, потокобезопасная (`sync.RWMutex`). |
| `internal/database/` | SQLite (`zpui.db`): модели, запросы (устройства, трафик). |
| `internal/logger/` | Ротируемый логгер по категориям → `logs/`. |
| `internal/notify/` | Windows toast-уведомления. |
| `internal/tray/` | Системный трей (fyne/systray). Интерфейс `Controller` реализуется `app.App`. |
| `internal/executil/` | Запуск процессов без всплывающего окна (`HiddenCmd`, Windows). |
| `internal/singleinstance/` | Блокировка повторного запуска (mutex). |
| `internal/sysinfo/` | Сбор системной информации. |

---

## 5. `cmd/` — сателлиты

Отдельные `.exe`, собираемые `build.bat`. wizard/autoselect — тонкие CLI-обёртки,
selfupdate/zapretupdate — самостоятельные обновляторы.

| Сателлит | Назначение |
|----------|-----------|
| `cmd/selfupdate/` | Самообновление ZPUI: скачивает `zpui.zip` из GitHub release, бэкапит, останавливает процесс, распаковывает (пропуская сам себя), перезапускает. |
| `cmd/zapretupdate/` | Обновление Zapret: бэкап пользовательских списков, скачивание, восстановление состояния (стратегии/службы). |
| `cmd/wizard/` | CLI-обёртка над `internal/wizard` (для восстановления/ручного запуска). |
| `cmd/autoselect/` | CLI-обёртка над `internal/autoselect` (для восстановления/ручного запуска). |

> wizard и autoselect также вызываются **внутрипроцессно** из главного приложения
> (через `internal/wizard` и `internal/autoselect`). `.exe`-обёртки остаются как CLI-инструменты.

---

## 6. `web/` — фронтенд (React + Vite)

```
web/src/
├── main.jsx                    # точка входа React
├── App.jsx                     # корневой компонент: роутинг страниц, поллинг статуса, тема
├── api.js                      # ★ shim: маршрутизация api('GET','/api/...') → window.go.app.App.*
├── api.test.js                 # тесты маршрутизации
├── db.js                       # обёртка хранилища (frontend)
├── lang.js                     # утилиты языка
├── utils.js                    # вспомогательные функции (formatBytes и т.д.)
│
├── pages/                      # страницы приложения
│   ├── DashboardPage.jsx       # дашборд (статус, быстрые действия)
│   ├── ZapretPage.jsx          # управление Zapret (стратегии, кэш, диагностика)
│   ├── ProxyPage.jsx           # настройки прокси
│   ├── XboxDnsPage.jsx         # Xbox DNS
│   ├── MonitorPage.jsx         # мониторинг трафика
│   └── SettingsPage.jsx        # настройки (тема, язык, обновления, сателлиты, уведомления)
│
├── components/
│   ├── layout/                 # Sidebar, Header, Footer
│   ├── ui/                     # переиспользуемые: Row, Switch, Modal
│   ├── feedback/               # Toast, OfflineScreen
│   ├── navigation/             # LogDrawer
│   ├── StartupScreen.jsx       # экран первичной настройки
│   ├── HealthCheckModal.jsx    # модалка проверки целостности файлов
│   ├── AutoSelectModal.jsx     # модалка автоподбора стратегии
│   ├── DiagnosticsModal.jsx    # модалка диагностики
│   └── ResourceChecker.jsx     # проверка ресурсов
│
├── hooks/                      # кастомные хуки
│   ├── usePolling.js           # периодический опрос API
│   ├── useDebouncedSave.js     # отложенное сохранение конфига
│   └── useServiceToggle.js     # переключатель службы
│
├── i18n/index.jsx              # провайдер i18n (useT)
├── locales/{ru,en}.json        # переводы строк
└── styles/index.css            # глобальные стили (CSS-переменные, темы)
```

- `web/wailsjs/runtime/` — JS-рантайм Wails (инжектится в окно).
- `web/vite.config.js`, `web/vitest.setup.js` — конфигурация сборки и тестов.
- Тесты (`*.test.js`) запускаются через **Vitest**: `npm test`.

---

## 7. `mods/` — система модов (JS-плагины)

Плагины с манифестом `mod.json` + точкой входа `index.js`. Сканируются `internal/mods.Registry`.

```
mods/
├── proxy/        { index.js, mod.json }
├── sysinfo/      { index.js, mod.json }
└── xbox-dns/     { index.js, mod.json }
```

Манифест `mod.json` описывает: `id`, `name`, `version`, `placements` (sidebar/dashboard/settings),
опциональный `backend.exe`, `repository` (для автообновления мода).

---

## 8. Сборка и релиз

| Компонент | Где |
|-----------|-----|
| Конфиг Wails | `wails.json` |
| Ресурсы сборки | `build/appicon.png`, `build/windows/` (иконка, манифест, info.json) |
| Скрипт сборки | `build.bat` (версия → frontend → core → 4 сателлита → `build/dist/` + `versions.json`) |
| Инсталлятор | `installer/ZPUI.nsi` (NSIS) |
| CI | `.github/workflows/ci.yml` (vet/test/build) |
| Релиз | `.github/workflows/release.yml` |

**Вывод сборки:**
- `build/bin/zpui.exe` — основной бинарник (gitignored).
- `build/dist/` — дистрибутив: `zpui.exe` + 4 сателлита + `versions.json` + `mods/` (gitignored).

---

## 9. Поток данных: фронтенд ↔ бэкенд

```
React-компонент
   │  api('GET', '/api/status')
   ▼
web/src/api.js  ── маршрут из GET_ROUTES/POST_ROUTES ──►  window.go.app.App.GetStatus()
                                                              │  (Wails IPC)
                                                              ▼
                                                  internal/app/app_api.go
                                                  func (a *App) GetStatus()
                                                              │
                                                              ▼
                                              internal/zapret, internal/monitor, ...
                                                              │
                                                              ▼
                                              map[string]interface{} → JSON → React
```

- **События (стриминг)**: бэкенд `runtime.EventsEmit(ctx, "event", data)` → фронтенд
  `window.runtime.EventsOn("event", cb)` (через `createStream()` в `api.js`).
  Примеры: `autoselect:event`, `wizard:progress`, `update:available`, `update:progress`.

---

## 10. Соглашения

- Все эндпоинты — методы `func (a *App)` в `internal/app/`; маршрут добавляется в `web/src/api.js`.
- Ответы бэкенда: `map[string]interface{}` с `{"error": "..."}` / `{"status": "ok"}`,
  хелперы `errResp()` / `okResp()` — в `internal/app/app_api_types.go`.
- Все пользовательские строки — через i18n (`web/src/locales/{ru,en}.json` + `useT`).
  Бэкенд использует `tr()` из `internal/app/app_i18n.go`.
- Общие хуки фронтенда: `usePolling`, `useDebouncedSave`, `useServiceToggle` (`web/src/hooks/`).
- Комментариев в коде нет, кроме неочевидной логики.
- Проверка после изменений Go: `go vet ./...`, `go test ./...`, `go build ./...`.
- Проверка фронтенда (`web/`): `npm run build`, `npm test`.
