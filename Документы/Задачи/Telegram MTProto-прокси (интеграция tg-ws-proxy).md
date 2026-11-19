# Telegram MTProto-прокси (интеграция tg-ws-proxy) — план и чек-лист

> Источник функционала: https://github.com/Flowseal/tg-ws-proxy (MIT).
> Решение по движку: **Go-native, в процессе** (пакет `internal/tgproxy`, горутины внутри `ZPUI.exe`).

## Цель
Добавить локальный MTProto-прокси для Telegram Desktop, интегрированный во вкладку «Прокси». Telegram подключается к нему как MTProto-прокси (`127.0.0.1:1443` + secret), прокси мостит трафик через WebSocket (TLS) к Telegram DC (`kws{dc}.web.telegram.org`). Работает параллельно с SOCKS5 (другой порт/протокол). Логирование, карточка в центре обновлений, данные в отчёте.

## Архитектура
- Новый пакет `internal/tgproxy/` — MTProto-over-WebSocket мост в процессе ZPUI.
- Слушает TCP (горутина на соединение), реализует MTProto-obfuscation handshake, устанавливает WSS к DC, ре-шифровывает трафик.
- `gorilla/websocket` уже в `go.mod` (indirect) — повышу до direct. AES-CTR/SHA256 — stdlib.

## Протокол (порт с `proxy/tg_ws_proxy.py`)
1. TCP-слушатель host:port (дефолт `127.0.0.1:1443`), `TCP_NODELAY`.
2. Handshake 64 байта: skip-префикс → `prekey+iv` → `dec_key=SHA256(prekey+secret)`, AES-CTR → проверка `proto_tag` ∈ {abridged, intermediate, padded} → `dc_idx` (int16 LE signed; `dc=abs`, `is_media=dc_idx<0`).
3. relay_init: 64 случайных байта с ограничениями, AES-CTR raw key шифрует хвост.
4. CryptoCtx: клиент — AES-CTR `SHA256(prekey+secret)` (fast-forward 64); Telegram — AES-CTR raw key.
5. WSS: `kws{dc}.web.telegram.org` / `kws{dc}-1.web.telegram.org`, путь `/apiw1`, target IP `dc_redirects[dc]` (DC2/DC4→`149.154.167.220`). Отправить relay_init → мост с ре-шифровкой.
6. Ссылка: `tg://proxy?server=<host>&port=<port>&secret=dd<32hex>`.

## Фазирование
- **Фаза 1 (MVP):** ядро моста + интеграция + UI + логи + карточка версии + отчёт. Прямой TCP-fallback при отказе WS.
- **Фаза 2 (позже):** CF-proxy/CF-worker fallback, пул WSS, Fake-TLS (ee-secret), PROXY-protocol v1.

---

## Чек-лист (отмечать `[x]` по выполнении)

### Фаза 1

#### 1. Конфиг
- [x] 1.1 `TelegramProxyConfig` в `internal/config/config.go` (Enabled, Port=1443, Secret, AutoStart, BindHost) + поле в `Config`, дефолт, геттер/сеттер (с авто-генерацией secret)
- [x] 1.2 ~~Поля в `GetConfig`/`SetConfig`~~ — решил через выделенные эндпоинты `/api/tgproxy/config` (как `/api/proxy/config`)
- [x] 1.3 Тесты config (round-trip + автогенерация secret) — `TestTgProxyConfig`

#### 2. Пакет `internal/tgproxy/`
- [x] 2.1 `proto.go` — константы MTProto, `parseHandshake`, `generateRelayInit`, `buildCryptoCtx` (AES-CTR)
- [x] 2.2 `ws.go` — WSS-клиент к DC (gorilla/websocket), TCP→IP, TLS SNI→домен, домены `wsDomains`, `dcDefaultIPs`
- [x] 2.3 `proxy.go` — TCP-сервер + обработчик (handshake→DC→мост), статистика (active/total/bytes/ws_ok/ws_fail)
- [x] 2.4 `manager.go` — Manager (Start/Stop/Restart/Status/Stats/Link/RegenerateSecret), `Version`, логи в категорию `tgproxy`
- [x] 2.5 Unit-тесты handshake/crypto round-trip — `proto_test.go` (3 DC/proto варианта, bad secret/tag) ✅ green

#### 3. Интеграция в приложение
- [x] 3.1 Поле `tgproxy` в `App`, создание в `NewApp` (`app.go`)
- [x] 3.2 Автозапуск при `--hidden` (Startup) + `Stop()` в `Shutdown`
- [x] 3.3 `internal/app/app_api_tgproxy.go` (Start/Stop/Restart/Status/GetConfig/SetConfig/GetLink/RegenerateSecret/OpenLink) + статус в `GetStatus` (`tg_proxy`)
- [x] 3.4 Маршруты в `web/src/api.js` (`/api/tgproxy/*`)

#### 4. Вкладка «Прокси»
- [x] 4.1 Подсекция «Telegram MTProto» в `ProxyPage.jsx` (hero с тумблером + статистика; секции «Подключение» (ссылка/копировать/открыть в TG, secret+регенерация) и «Настройки» (порт, автозапуск, статистика ws))
- [x] 4.2 Локали ru/en (`tgproxy.*`)
- [x] 4.3 CSS (`.tgp-*`, токены дизайн-системы)

#### 5. Логирование
- [x] 5.1 Бакет `tgproxy` в logdb (`tgproxy_logs`, таблица создаётся миграцией автоматически) + маппинг категории в `tableCategory` + кейсы в `GetLogs`/`ClearLogBucket`
- [x] 5.2 Вкладка в `LogDrawer.jsx` (`TABLES`) + ключ `logs.tgproxy` (ru/en)

#### 6. Центр обновлений
- [x] 6.1 `tgproxy.Version` (константа в пакете), поле `tgproxy` в `GetVersions` (`versions.go`)
- [x] 6.2 Карточка «Telegram Proxy» в `SettingsPage.jsx` (версия + пометка «Обновляется с ZPUI»; ключи `settings.tgProxyCard`/`tgProxyUpdWithZpui`)

#### 7. Отчёт
- [x] 7.1 `RuntimeState` + `TgProxy{Running,Port,ActiveConn,TotalConn,BytesIn,BytesOut}`; `ConfigSnapshot` + `TgProxy{Enabled,Port}` (`reports/generator.go`)
- [x] 7.2 Строка «Telegram MTProto proxy» в Diagnostic Summary (статус/порт/соединения/трафик) + строка в Configuration
- [x] 7.3 `collectReportRuntime` feeds из `a.tgproxy.GetStats()` (`app_api_reports.go`); `cmd/report/main.go` тоже заполняет порт

#### 8. Проверка
- [x] 8.1 `go vet ./...` — ok
- [x] 8.2 `go test ./...` — ok (config/tgproxy/logdb/reports + все прочие)
- [x] 8.3 `npm run build` + `npm test` (31/31) — ok

### Фаза 2 ✅ (v1.1.0 модуля, ZPUI v1.6.0)
- [x] CF-proxy fallback: встроенный декодированный пул доменов (Caesar-декодер `decodeCFDomain`), ежечасный refresh с GitHub, `balancer` (активный домен на DC), fallback `kws{dc}.{base}` (`balancer.go`, `bridge.go::cfProxy`)
- [x] CF-worker fallback: пользовательские домены, path `/apiws?dst=<ip>&dc=<dc>` (`bridge.go::cfWorker`)
- [x] Прямой TCP-fallback к DC:443 (в т.ч. тестовые DC) (`bridge.go::tcp`, `dcTestIPs`)
- [x] Пул WSS-соединений на (dc, isMedia): max age 120с, sweep 5с, refill с backoff, SNI-фронтинг sprinthost.ru при таймаутах, warmup при старте (`pool.go`)
- [x] Fake TLS (ee-secret): `verifyClientHello` (HMAC-SHA256 по hello), `buildServerHello`, `fakeTLSConn` (TLS-записи, CCS skip), HTTP-301 маскировка не-TLS клиентов, `proxyToMaskingDomain` (`faketls.go`); ee-ссылка в UI
- [x] PROXY-protocol v1: чтение заголовка до handshake, label из PROXY line (`proxy.go::readClientInit`)
- [x] MsgSplitter: разбивка MTProto-потока на пакеты (abridged/intermediate/padded) → отдельные WS-фреймы (`bridge.go`)
- [x] Blacklist/cooldown: WS-blacklist DC при всех 302, cooldown DC 60с / IP 1ч, приоритет fallback при cooldown (`proxy.go`)
- [x] Тестовые DC (10001+) авто-детект и маршрутизация по `dcTestIPs`
- [x] Тесты: splitter (abridged/intermediate/чанки), fake TLS verify/build/tamper, decodeCFDomain, balancer, normalizeDomainPool
- [x] UI: Fake TLS домен, тумблер CF-fallback, CF Worker домены, ee-ссылка с тегом, расширенная статистика (fbStats)
- [x] Кнопка «Подключить в Telegram»: открывает tg://-ссылку с анимацией загрузки (~2с), активна только при работающем мосте (backend-проверка + disabled); toast-подтверждение
- [x] Фикс: Telegram отклонял ссылку — `cmd /c start` обрезал URL на `&` (Go не квотит `&`); переход на `rundll32 url.dll,FileProtocolHandler`
- [x] Фикс «bad handshake» на всех CF-доменах: добавлен субпротокол `Sec-WebSocket-Protocol: binary` (как в raw_websocket.py) + InsecureSkipVerify (как CERT_NONE в upstream)
- [x] Анти-шторм: cooldown CF per-DC 10м / TCP per-IP 15м (отдельно от 1ч cooldown прямого WS), сводное WARN вместо пер-доменных, домены на DEBUG
- [x] UX тумблеров: pending-спиннер + быстрый опрос (0.7с) до смены состояния — мгновенный отклик вкл/выкл
- [x] UX Telegram-панели: блок «Подключение» (кнопка → ссылка → secret с меткой), настройки сеткой (поля вместе, тумблеры вместе), статистика 4 строки, хинт вместо статистики при выключенном
- [x] Полоса вкладок логов: компактные табы + горизонтальная прокрутка (колесо мыши → горизонталь через нативный non-passive wheel listener; тачпад нативно), скрытый скроллбар
- [x] Вкладка «Прокси» переписана под компактную вёрстку без прокрутки (960×640): две панели (SOCKS5 / Telegram), компактные шапки, сетки 2×2, внутренний скролл только у списка устройств
- [x] Учёт подключений: сессии/трафик считаются в bridgeWS/bridgeTCP (все каналы), per-IP карта `clients` → `ClientsByIP()`
- [x] Привязка к SOCKS: `ProxyStart/Stop` управляют обоими, `TgProxyStart/Stop` делегируют общим; автозапуск общий (один флаг); в панели TG — пилюля состояния вместо тумблера; настройки TG без отдельного автозапуска
- [x] Устройства: «Сеть» и «Прокси» — одна карточка на устройство (SOCKS-соединения + TG-бейдж), union по IP (`GetMonitorDevices`, `GetProxyConnections` + `tg_by_client`); дашборд-карточка прокси помечает `· TG`
- [x] ЛВС: bind `0.0.0.0` принимает устройства сети, autorule firewall `ZPUI Telegram Proxy`, ссылка по IP ПК
- [x] Мини-веб на порту моста (`webui.go`): HTTP-детект по методу, страница с кнопкой tg:// (server=IP устройства), secret, статистика (авто-refresh `/status.json`), FakeTLS-не-TLS клиенты идут как раньше
- [x] Синхронизация always-on: тумблер убран, `startSync` без gate, API всегда `enabled:true`
- [x] Версии: ZPUI 1.6.1, модуль tgproxy 1.2.0, CHANGELOG обновлён
- [x] Документация и версии: CHANGELOG v1.6.1, versions.txt zpui=1.6.1, модуль v1.2.0, AGENTS.md, Документы/Архитектура.md, Документы/Функции.md

---

## Заметки по реализации
- Слушатель: по умолчанию `127.0.0.1` (локально); если `0.0.0.0` — ссылка использует LAN IP (для других устройств).
- Secret: 32 hex; генерировать `crypto/rand`, если пусто; персистить в конфиг.
- Логи: старт/стоп, соединения (DC, media), WS успех/отказ/302, fallback, периодическая статистика.
- Не вводить второй трей — всё внутри процесса ZPUI.
