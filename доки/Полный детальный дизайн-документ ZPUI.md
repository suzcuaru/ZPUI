# 📋 ПОЛНЫЙ ПЛАН РЕФАКТОРИНГА ZPUI (v2 — исправленный)

---

## ТЕКУЩИЙ ФУНКЦИОНАЛ (что сохраняем)

### 6 страниц (активных в App.jsx):
| Страница | API calls | Функционал |
|----------|-----------|------------|
| **MonitorPage** | `/api/proxy/connections`, `/api/monitor/devices`, `/api/resource-status` + IndexedDB snapshots | Графики скорости, подключения, прокси статус, ресурсы (default+user), устройства, сетевая информация |
| **StrategyPage** | `/api/zapret/strategies`, `/api/zapret/set-strategy`, `/api/strategy/stream` (SSE), `/api/strategy/cancel` | Карточки стратегий, автотест с SSE стримом, результаты с donut chart, calcBest(), применение лучшей |
| **ListsPage** | `/api/ipset/status`, `/api/ipset/toggle`, `/api/ipset/update`, `/api/hosts/update`, `/api/hosts/list`, `/api/hosts/save` | ipset статус/режимы (none/any/loaded), обновление ipset/hosts, пользовательские хосты, автопроверка обновлений |
| **DiagnosticsPage** | — (локальные тесты) | Ping тест, traceroute, DNS тест, winws тест |
| **GeneralPage** | `/api/config` GET/POST, `/api/autostart/status`, `/api/autostart/toggle`, `/api/autoupdate/status`, `/api/autoupdate/toggle` | Настройки: web_port, zapret_path, автозапуск, автопроверка обновлений, конфигурация прокси, конфигурация логов |
| **AboutPage** | — | Версия, ссылки, системная информация |

### Компоненты:
| Компонент | Функционал |
|-----------|------------|
| **Sidebar** | Навигация (6 вкладок), toggle Zapret/Proxy, копирование адреса прокси, кнопка логов |
| **StatusBar** | Статус Zapret/Proxy, стратегия, доступность ресурсов |
| **LogDrawer** | Выдвижная панель логов справа (300ms анимация), SSE поток `/api/log/stream`, фильтры по типу |
| **Toast** | Уведомления с auto-dismiss (3с для info, 6с для error) |
| **OfflineScreen** | Экран при потере бэкенда, кнопка "Повторить" |

### API (25+ endpoints):
- Status: `/api/status`, `/api/up/info`, `/api/config`, `/api/resource-status`
- Zapret: `/api/zapret/start`, `/api/zapret/stop`, `/api/zapret/strategies`, `/api/zapret/set-strategy`
- Strategy: `/api/strategy/stream` (SSE), `/api/strategy/cancel`
- Proxy: `/api/proxy/start`, `/api/proxy/stop`, `/api/proxy/connections`
- Monitor: `/api/monitor/devices`
- Lists: `/api/ipset/*`, `/api/hosts/*`
- Config: `/api/autostart/*`, `/api/autoupdate/*`
- Other: `/api/external`

### IndexedDB (`db.js`):
- `logSnapshot(data)` — снапшоты трафика (dl/ul speed, bytes)
- `getSnapshots()` — получение последних 30 мин
- `cacheSet(key, val)`, `cacheGet(key)` — кэш ресурсов
- `cleanOld()` — очистка старых данных

---

## НОВАЯ СТРУКТУРА ВКЛАДОК

| # | Вкладка | Откуда что берётся |
|---|---------|-------------------|
| 1 | 📊 **Мониторинг** | MonitorPage (как есть, + кликабельные устройства) |
| 2 | 📱 **Устройства** | НОВАЯ — сетка устройств, трафик по устройствам |
| 3 | 🔧 **Фильтры** | = StrategyPage (dropdown) + ListsPage (полностью) |
| 4 | 🔍 **Диагностика** | DiagnosticsPage (как есть) |
| 5 | ⚙️ **Настройки** | GeneralPage (как есть) |
| 6 | ℹ️ **О программе** | AboutPage (как есть) |

### Что переезжает куда:
- **StrategyPage** → вкладка "Фильтры", верхняя секция (dropdown вместо карточек)
- **ListsPage** → вкладка "Фильтры", нижняя секция (без изменений)
- **MonitorPage** → убираем секцию "устройства" (теперь отдельно)
- **Новое** → DevicesPage + DeviceDrawer + SQLite + Logger

---

## ЭТАПЫ РЕАЛИЗАЦИИ

### ЭТАП 1: SQLite база данных (Go бэкенд)

**Новые файлы:**
- `internal/database/db.go` — подключение к SQLite, миграции
- `internal/database/models.go` — структуры Go + SQL таблицы
- `internal/database/queries.go` — CRUD функции

**Таблицы:**
```sql
-- Устройства сессии (очищается при запуске)
session_devices (id, mac, ip, hostname, first_seen, last_seen, total_dl, total_ul, is_online)

-- Соединения устройств  
device_connections (id, device_mac, dst_host, dst_port, bytes_dl, bytes_ul, started_at, closed_at)

-- Логи действий
action_logs (id, timestamp, category, action, details JSON)

-- Снапшоты трафика (вместо IndexedDB)
traffic_snapshots (id, timestamp, dl_speed, ul_speed, total_dl, total_ul, conn_count)
```

**Зависимость:** `modernc.org/sqlite` (pure Go, без CGO)

---

### ЭТАП 2: Device Tracking + API (Go бэкенд)

**Новые файлы:**
- `internal/devices/tracker.go` — агрегация трафика из connections по MAC
- `internal/web/api_devices.go` — новые endpoints

**Новые API endpoints:**
| Method | Path | Описание |
|--------|------|----------|
| GET | `/api/devices` | Все устройства сессии из SQLite |
| GET | `/api/devices/:mac` | Детали + статистика |
| GET | `/api/devices/:mac/connections` | Соединения устройства |
| POST | `/api/devices/:mac/ping` | Измерить ping |

**Изменить:**
- `internal/web/server.go` — добавить маршруты
- `internal/web/api.go` — middleware логирования

---

### ЭТАП 3: Action Logger (Go бэкенд)

**Новый файл:**
- `internal/database/action_log.go`

**Новые API:**
| Method | Path | Описание |
|--------|------|----------|
| GET | `/api/logs/actions` | Логи с фильтрами (?category=&limit=) |
| POST | `/api/logs/frontend` | Batch логов от фронтенда |

**Логируем:**
- Все API запросы (middleware: method, path, duration, status)
- Действия пользователя (с фронтенда: навигация, клики, toggle)
- Ошибки (backend + frontend)

---

### ЭТАП 4: Фронтенд — Вкладка "Фильтры" (React)

**Новый файл:**
- `web/src/pages/FiltersPage.jsx` — объединяет стратегии + списки

**Содержание:**
1. **Верх — Стратегия**: Кастомный dropdown (`StrategyDropdown.jsx`)
   - Текущая стратегия с ✅ пометкой
   - Список всех стратегий
   - Кнопка "Автотест" → модальное окно с SSE стримом (из StrategyPage)
   - Результаты тестов + donut chart + "Применить лучшую" (из StrategyPage)
2. **Низ — Списки фильтрации**: Полностью из ListsPage
   - IPSet: статус, режим (none/any/loaded), обновление
   - Hosts: обновление, список пользовательских
   - Автопроверка обновлений

**Новый компонент:**
- `web/src/components/StrategyDropdown.jsx` — кастомный dropdown

**Удалить:**
- `web/src/pages/StrategyPage.jsx` → всё перенесено в FiltersPage

---

### ЭТАП 5: Фронтенд — Вкладка "Устройства" + DeviceDrawer

**Новые файлы:**
- `web/src/pages/DevicesPage.jsx` — сетка устройств
- `web/src/components/DeviceDrawer.jsx` — боковая панель (как LogDrawer)

**DevicesPage:**
- CSS Grid карточек: `repeat(auto-fill, minmax(200px, 1fr))`
- Каждая карточка: hostname, IP, MAC, онлайн статус, ↓↑ скорость, total трафик
- Клик → открывает DeviceDrawer

**DeviceDrawer (380px, справа):**
- Заголовок: hostname + IP + MAC
- Ping: значение + кнопка обновления
- Скорость: ↓/↑ с sparkline графиками
- Трафик за сессию: total ↓/↑
- Соединения: список активных подключений устройства
- Стили: как LogDrawer (overlay, slide-in 300ms)

**Изменить:**
- `MonitorPage.jsx` — карточки устройств кликабельны → DeviceDrawer
- `App.jsx` — добавить deviceDrawerOpen state + DevicesPage в PAGES

---

### ЭТАП 6: Frontend Logger

**Новый файл:** `web/src/logger.js`

```js
// Категории: navigation, click, api, api_error, toggle, strategy, device, error
// Batch отправка каждые 5с на POST /api/logs/frontend
// Обёртка вокруг api() — автологирование запросов
// Хук useLogAction() — логирование кликов
```

---

### ЭТАП 7: CSS Редизайн

**Изменить:**
- `web/src/App.css` — полный rewrite с CSS variables, адаптивностью

**Дизайн-система:**
- Шрифт: Inter (13px body), JetBrains Mono (12px data)
- Цвета: тёмная тема (bg-base #0e0f17, accent #6c5ce7)
- Sidebar: 200px fixed
- StatusBar: 40px fixed
- Cards: border-radius 12px, bg-elevated
- Drawers: 380px, slide-in справа
- Responsive: min-width 800px (content area)

**Изменить:**
- `internal/window/window.go` — мин 1000×1000, макс 2000×2000

---

### ЭТАП 8: Сборка

- `npm run build` (Vite)
- `go build` (с SQLite зависимостью)

---

## ИТОГО ФАЙЛОВ:

### Go (новые): 5
- `internal/database/db.go`
- `internal/database/models.go`
- `internal/database/queries.go`
- `internal/database/action_log.go`
- `internal/web/api_devices.go`

### Go (изменить): 2
- `internal/web/server.go`
- `internal/window/window.go`

### React (новые): 4
- `web/src/pages/DevicesPage.jsx`
- `web/src/pages/FiltersPage.jsx`
- `web/src/components/DeviceDrawer.jsx`
- `web/src/components/StrategyDropdown.jsx`
- `web/src/logger.js`

### React (изменить): 4
- `web/src/App.jsx`
- `web/src/App.css`
- `web/src/components/Sidebar.jsx`
- `web/src/pages/MonitorPage.jsx`

### React (удалить): 1
- `web/src/pages/StrategyPage.jsx` → в FiltersPage

---

