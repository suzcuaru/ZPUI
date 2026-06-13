## Задача: Полный рефакторинг ZPUI

### Утверждённый план (практичный scope, без over-engineering):

Проект расположен в `c:\Users\Suzuc\Documents\VS_code\Проэкты\DPIF2\mod` Go бэкенд + React фронтенд (Vite). Desktop app для Windows (webview).

### ЭТАПЫ:

#### ЭТАП 1: SQLite база (Go бэкенд)

- Добавить `modernc.org/sqlite` (pure Go, без CGO)

- Создать `internal/database/db.go` — подключение SQLite, WAL mode, миграции

- Создать `internal/database/models.go` — Go структуры + SQL таблицы:

  - `session_devices` (mac, ip, hostname, first_seen, last_seen, total_dl, total_ul, is_online)
  - `device_connections` (device_mac, dst_host, dst_port, bytes_dl, bytes_ul, started_at, closed_at)
  - `action_logs` (timestamp, category, action, details JSON)
  - `traffic_snapshots` (timestamp, dl_speed, ul_speed, total_dl, total_ul, conn_count)

- Создать `internal/database/queries.go` — CRUD функции

- Интегрировать в main.go

#### ЭТАП 2: Device Tracking + API (Go бэкенд)

- Создать `internal/web/api_devices.go` — API endpoints:

  - GET `/api/devices` — все устройства из SQLite
  - GET `/api/devices/:mac` — детали + статистика
  - GET `/api/devices/:mac/connections` — соединения устройства
  - POST `/api/devices/:mac/ping` — ping

- Обновить `internal/web/server.go` — добавить маршруты

#### ЭТАП 3: Action Logger (Go бэкенд)

- Создать `internal/database/action_log.go` — логирование
- API middleware для автологирования запросов
- POST `/api/logs/frontend` — приём логов от фронтенда
- GET `/api/logs/actions` — получение логов с фильтрами

#### ЭТАП 4: Вкладка "Фильтры" (React)

- Создать `web/src/pages/FiltersPage.jsx` — объединяет StrategyPage + ListsPage

  - Верхняя секция: StrategyDropdown (кастомный dropdown выбора стратегии + автотест с SSE)
  - Нижняя секция: управление списками (ipset/hosts) — из ListsPage

- Создать `web/src/components/StrategyDropdown.jsx` — кастомный dropdown

- Удалить `web/src/pages/StrategyPage.jsx` (функционал перенесён)

#### ЭТАП 5: Вкладка "Устройства" + DeviceDrawer (React)

- Создать `web/src/pages/DevicesPage.jsx` — сетка устройств
- Создать `web/src/components/DeviceDrawer.jsx` — боковая панель (как LogDrawer, 380px, slide-in справа)
  - IP, MAC, hostname, Ping, скорость ↓/↑, трафик за сессию, соединения
- Обновить MonitorPage — карточки устройств кликабельные → DeviceDrawer

#### ЭТАП 6: Frontend Logger

- Создать `web/src/logger.js` — логирование действий + batch отправка на POST /api/logs/frontend

#### ЭТАП 7: CSS редизайн + window.go

- `internal/window/window.go` — мин 1000×1000, макс 2000×2000, начальный 1100×1050

- `web/src/App.css` — полный rewrite:

  - CSS Variables (цвета: bg-base #0e0f17, accent #6c5ce7, success/warning/danger)
  - Шрифты: Inter 13px body, JetBrains Mono 12px data
  - Sidebar 200px, StatusBar 40px, Cards border-radius 12px
  - Responsive breakpoints (1000-1200px compact, 1200+ full)

- `web/src/components/Sidebar.jsx` — обновить навигацию: Мониторинг, Устройства, Фильтры, Диагностика, Настройки, О программе

- `web/src/App.jsx` — обновить PAGES map (заменить strategy→filters, lists→devices), добавить DeviceDrawer

#### ЭТАП 8: Сборка

- `npm run build` (Vite)
- `go build` с новыми зависимостями

### Текущая структура вкладок (что менять):

- Было: Мониторинг, Стратегии, Списки, Диагностика, Общее, О программе
- Стало: Мониторинг, Устройства, Фильтры, Диагностика, Настройки, О программе

### Существующие API endpoints (все в internal/web/api.go и server.go):

25+ endpoints. Нужно прочитать эти файлы чтобы понять точные маршруты и добавить новые.

### Существующие React компоненты:

- App.jsx, api.js, db.js, main.jsx
- components/: Sidebar, StatusBar, Toast, LogDrawer, OfflineScreen
- pages/: MonitorPage, StrategyPage, ListsPage, DiagnosticsPage, GeneralPage, AboutPage

### Ключевые правила:

- Сохранить ВСЕ существующие функции (автотест SSE, ipset управление, etc)
- Drawer-паттерн для устройств (как LogDrawer)
- DeviceDrawer: 380px справа, slide-in 300ms, overlay
- Sidebar: 200px, текущий дизайн (нравится пользователю), только обновить навигацию
- Все тексты на русском языке
