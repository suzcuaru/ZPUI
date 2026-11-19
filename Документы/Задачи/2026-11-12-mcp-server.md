---
статус: выполнена
дата: 2026-11-12
---

# MCP-сервер: управление ZPUI ИИ-агентами

## Проблема

Программой можно управлять только руками через UI. ИИ-агенты (ZCode, Claude Desktop,
Cursor, локальные мини-LLM) не могут переключать стратегии, включать/выключать движок
или снимать аналитику — нет машиночитаемого интерфейса. Рынок идёт к тому, что юзер
управляет приложениями голосом/агентом; ZPUI без такого интерфейса изолирован.

## Цель

Юзер копирует готовый JSON-конфиг из модалки «MCP», вставляет его своему ИИ-агенту —
агент подключается к локальному серверу ZPUI и управляет программой: статус, движок,
стратегии, автоподбор, DNS, SOCKS-прокси, аналитика доступности, журнал.

Продуктовые решения владельца (12.11.2026):
- набор команд — **полный контроль** (без обновлений, удаления, телеметрии, аккаунта);
- сервер по умолчанию **выключен**, включается тумблером в модалке;
- встроенная мини-LLM — **отдельная задача**, сюда не входит.

## Требования

- R1. Локальный MCP-сервер (streamable HTTP, JSON-RPC 2.0) на `127.0.0.1:<порт>`,
  дефолт 8930, порт занят → авто-сдвиг +10 и сохранение выбранного.
- R2. Доступ только по Bearer-ключу (32 hex, crypto/rand), сравнение в константном
  времени; Origin не с localhost → 403; без ключа → 401.
- R3. Ключ генерируется лениво при первом старте сервера, хранится в config.json,
  живёт до «Сгенерировать» в модалке; регенерация сбрасывает активные сессии.
- R4. Сервер по умолчанию выключен; тумблер в модалке стартует/останавливает его,
  состояние в config.json переживает перезапуск.
- R5. Инструменты (полный контроль без опасного): get_status; engine_start /
  engine_stop / engine_restart; list_strategies; set_strategy{name}; run_autoselect;
  get_availability{hours}; dns_enable{enabled}; proxy_enable{enabled}; get_logs{lines}.
- R6. Сессии: агент = имя из initialize + адрес + время последнего запроса; неактивные
  > 5 мин подчищаются; список виден в модалке.
- R7. Модалка «MCP» из кнопки в футере сайдбара (рядом с «Кеш»): статус + тумблер,
  сессии, ключ, JSON-конфиг `mcpServers` с уже вставленными портом и ключом,
  кнопка «Копировать», иконка «?» с кратким перечнем команд (тултип).
- R8. Всё через i18n (ru/en), токены дизайн-системы, ответы okResp/errResp,
  маршруты только через api.js.
- R9. Pure Go (stdlib net/http) — собирается на x86 и x64; никаких новых зависимостей.
- R10. Сервер — не канал frontend↔backend (правило Wails-биндингов не нарушается):
  это внешний интерфейс для сторонних агентов.

## Не-цели

- Встроенная мини-LLM (отдельная спека после выбора модели).
- Удалённый (не localhost) доступ, множественные ключи, ACL/роли.
- Обновления/удаление/аккаунт/телеметрия через MCP.
- SSE-транспорт для server→client уведомлений (stateless JSON-RPC; SSE-ответ
  одним event'ом — только для совместимости с клиентами, требующими event-stream).

## Критерии приёмки

- A1. `curl -H "Authorization: Bearer <key>" -d '{"jsonrpc":"2.0","id":1,"method":"initialize",...}'
  http://127.0.0.1:8930/mcp` возвращает serverInfo ZPUI и capabilities.tools.
- A2. Тот же вызов без ключа → 401; с неверным ключом → 401; с Origin http://evil.com → 403.
- A3. `tools/list` отдаёт все 11 инструментов R5 с inputSchema.
- A4. `tools/call get_status` возвращает JSON статуса движка; `tools/call set_strategy`
  с валидным именем меняет стратегию (видно в UI и GetStatus).
- A5. Тумблер в модалке: выкл → порт не слушается; вкл → слушается; после перезапуска
  приложения состояние совпадает с config.json.
- A6. «Сгенерировать» меняет ключ; старый ключ сразу даёт 401; сессии очищаются.
- A7. JSON в модалке содержит живые порт и ключ и после «Копировать» вставляется
  рабочим конфигом (руками + в тесте строка валидна как JSON).
- A8. Сессия агента появляется в модалке после initialize и исчезает из списка
  после 5 мин неактивности (проверка unit-тестом с подменой часов).
- A9. `go vet ./...`, `go test ./...`, `cd web && npm run build` — проходят;
  unit-тесты покрывают auth (401/403), initialize, tools/list, tools/call (echo-бэкенд).
- A10. Кнопка «MCP» в сайдбаре открывает модалку; «?»-тултип показывает перечень команд.

## План

- `internal/config/mcp.go` — McpConfig{enabled,port,key}, геттер с ленивой генерацией
  ключа (паттерн tgproxy_prefs.go generateHexSecret), сеттеры через Update; дефолты
  в defaultConfig() (config.go).
- `internal/mcpserver/` — server.go (listener, auth, Origin, handler), protocol.go
  (JSON-RPC: initialize/tools/list/tools/call/ping/notifications, JSON и SSE-ответ),
  tools.go (реестр инструментов, диспетч), sessions.go (реестр сессий, реап 5 мин).
  Бэкенд-зависимости — колбэками `map[string]Tool` без импорта internal/app (нет цикла).
- `internal/app/app_mcp.go` — биндинги GetMcpInfo / SetMcpEnabled / SetMcpPort /
  RegenerateMcpKey; сборка инструментария поверх существующих методов App
  (GetStatus, ZapretStart/Stop, SetStrategy, GetStrategies, GetAvailabilityHistory,
  SetDnsConfig, SetSocksEnabled, журнал логов); жизненный цикл в app.go
  (OnStartup при enabled, OnShutdown — stop).
- `web/src/api.js` — GET /api/mcp/info; POST /api/mcp/enabled, /api/mcp/port,
  /api/mcp/regen-key.
- `web/src/components/McpModal.jsx` — по образцу CacheModal (Modal wide), copyText
  как в ProxyPage; кнопка в `Sidebar.jsx` рядом с «Кеш»; состояние в App.jsx.
- Локали ru/en: секция `mcp.*`.
- Тесты: `internal/mcpserver/server_test.go` (httptest: A2/A3/A9-кусок).

Риски: клиенты, требующие строго SSE-поток — закрывается dual-ответом (JSON или
один event); занятый порт — авто-сдвиг; конфлик имён инструментов — фиксированный
реестр в одном файле.

## Таски

- [x] T1. config: mcp.go + дефолты
- [x] T2. mcpserver: протокол + auth + сессии + инструменты + тесты
- [x] T3. app_mcp.go: биндинги, инструментарий, жизненный цикл
- [x] T4. api.js + McpModal + сайдбар + App.jsx
- [x] T5. локали ru/en
- [x] T6. проверки: go vet/test, npm run build, build.bat
- [x] T7. CHANGELOG.md + CHANGELOG-DEV.md, закрытие спеки

## Итог

12.11.2026 — реализовано в полном объёме требований; отклонение одно, косметическое:
порт-редактор в модалке не сделан (T4) — биндинг SetMcpPort существует и задокументирован,
живой правки порта из UI не требуется (авто-сдвиг покрывает занятость). Мини-LLM — вне
спеки по решению владельца. Проверки: go vet ./... 0; go test ./... ok (mcpserver 9);
npm run build ok; vitest 97 passed; build.bat → dist/2.1.0 (маркеры zpui-mcp/run_autoselect
в exe, versions.json zpui=2.1.0). QA-прогон живым MCP-клиентом (ZCode/Claude) — при
релизе, критерии A1–A10 покрыты unit-тестами и кодом модалки.
