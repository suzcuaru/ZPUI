# ZPUI Live — план полной переработки (сервер + VPN + админка + клиент)

> Статус: утверждено владельцем 26.09.2026. Заменяет инвайт-спецификацию v2 в частях:
> регистрация (email+логин+пароль, инвайт опционален), VPN-контур, подписки с лимитами
> скорости/трафика, вход в админку по логину+паролю, рейтинги оператор×стратегия.
> Ренейминг: **ZPUI Server → ZPUI Live**.

## 0. Принятые решения

1. **VPN — WireGuard**: контейнер wg + шейпер tc; клиент — встроенный wireguard-go (wintun.dll). Per-user лимиты скорости/приоритеты/учёт трафика.
2. **Регистрация**: свободная email+логин+пароль; инвайт — опциональное поле (валидный → его тариф сразу), без инвайта — триал (`settings.trial_days`).
3. **Email** обязателен и уникален, без SMTP-подтверждения; восстановление — Telegram + админ.
4. **Туннель**: по умолчанию раздельный (AllowedIPs-список нужных ресурсов с сервера), в приложении переключатель «весь трафик».
5. Приоритет устройств задаёт пользователь в ЛК; при превышении совокупного лимита скорость распределяется по весам (приоритетные устройства получают больше).

## 1. Серверная архитектура

Compose: + сервис `vpn` (WireGuard UDP 51820 + сайдкар `vpnctl` в одном контейнере/NetNS).
vpnctl (Go, :9103, INTERNAL_TOKEN): управление peers (`wg`), шейпинг (`tc htb`: корень = общий лимит подписки, классы на устройство: ceil = лимит устройства, rate = вес по приоритету), счётчики трафика (`nft`) → раз в минуту в control. Guard публикует UDP 51820. Белый IP: guard уже 0.0.0.0; в versions.txt — белый IP/домен (даст владелец).

### Миграции БД (lightMigrations, идемпотентно)

- `accounts` + `email TEXT UNIQUE`, `plan_code TEXT DEFAULT ''`, `must_change_password BOOLEAN DEFAULT FALSE`
- `plans` + `speed_total_mbps INT DEFAULT 0` (0=безлимит), `speed_per_device_mbps INT DEFAULT 0`, `traffic_gb INT DEFAULT 0`
- новая `plan_prices(plan_code, period[month|quarter|year], price, discount_percent)`
- `devices` + `vpn_pubkey`, `vpn_ip`, `vpn_status`, `priority INT DEFAULT 0`, `traffic_used BIGINT DEFAULT 0`, `traffic_reset_at`
- новые `admin_users(id, login UNIQUE, password_hash, role, created_at)`, `admin_sessions(token, admin_id, expires_at, created_at)`
- новая `strategy_samples(id, device_id, isp_key, isp_name, strategy, availability, ping_ms, speed_mbps, created_at)` + индексы `(isp_key, strategy, created_at)`
- `settings`: + `trial_days` (7), `smart_select_window` (100), `vpn_public_host`, `vpn_port` (51820), `vpn_subnet` (10.7.0.0/24)

### Клиентский API (новое/расширение)

| Маршрут | Суть |
|---|---|
| `POST /api/v1/account/register` | {email, login, password, invite_code?, device_id, device_name} → {token, account, plan, must_change_password}. Коды: `bad_email`, `bad_login` (3–32 `[a-z0-9_]`, с буквы), `login_taken`, `email_taken`, `weak_password`, `invite_used`, `trial_unavailable`. `/account/create` остаётся (совместимость). |
| `POST /api/v1/account/password` | (auth) {current, new} — смена пароля |
| `POST /api/v1/device/priority` | (auth) {device_id, priority} |
| `POST /api/v1/vpn/pubkey` | (auth) {device_id, pubkey} — регистрация ключа устройства |
| `GET /api/v1/vpn/config` | (auth) peer только для привязанного устройства активной подписки: {enabled, endpoint, server_pubkey, address, dns, mtu, allowed_ips (split) } |
| `GET /api/v1/strategies/top` | ?device_id= → {operator, strategies:[{name, availability, ping_ms, speed_mbps, samples}]} топ-5, окно = `smart_select_window` |
| `GET /api/v1/plans` | каталог: код, название, лимиты (устройства/скорость общая и на устройство/трафик), цены за месяц/квартал/год, скидки, features |
| login/account ответы | + `plan{code,name,expires_at,devices_limit,speed_total_mbps,speed_per_device_mbps,traffic_gb,traffic_used}`, `must_change_password` |

Телеметрия `POST /api/v1/telemetry` расширяется: `isp{key,name,asn}`, `strategy_results[{strategy, availability, ping_ms, speed_mbps}]`, `net{ping_ms, speed_mbps}` → пишется в `strategy_samples`.

### Админ-API (новое)

| Маршрут | Суть |
|---|---|
| `POST /admin/api/auth/login` | {login, password} → {token, expires_at}; `GET auth/me`; `POST auth/logout`. adminGate: Bearer-сессия админа; переходно — legacy `X-Admin-Token`. Bootstrap: env `ADMIN_LOGIN/ADMIN_PASSWORD` (дефолт admin/admin, warning в лог), DEMO_MODE: admin/admin123 |
| Аккаунты | POST create (админ создаёт с подпиской), POST set-plan, POST set-expiry; reset-password → временный пароль + `must_change_password=true` + revoke всех сессий (форс-логаут) |
| `GET /admin/api/operators?window=N` | матрица оператор×стратегия: avg availability/ping/speed, samples |
| `GET /admin/api/vpn` | peers, статусы, трафик, скорости; `POST vpn/peer/{id}/enable|disable` |
| `GET /admin/api/infra/logs?service=&tail=` | просмотр логов сервисов |
| `PUT /admin/api/settings` | + trial_days, smart_select_window, vpn_* |

## 2. Админка (adminweb — редизайн, тёмная тема в токенах ZPUI)

Вход логин+пароль. Группы: **Обзор** (KPI, графики онлайн/телеметрии/событий, инфо о сервере) · **ZPUI Core** (Аккаунты — создание/бан/подписка/срок/сброс пароля; Устройства — панель устройства: аккаунт, VPN, трафик, отчёты/логи/телеметрия; Подписки/Планы — CRUD с ценами×3 и скидками; Инвайты) · **Инфраструктура** (сервисы+VPN, Логи, Бэкапы, БД, Прокси, Telegram) · **Данные** (Телеметрия, Отчёты, События, Операторы×Стратегии + окно N) · **Контент** (Обновления, Баннеры) · **Настройки**.

## 3. Клиентское приложение

1. AuthGate: без обязательного токена; email+логин+пароль, подсказки формата, инлайн-ошибки (коды сервера), «сервер недоступен — продолжить офлайн»; удалить дубль AuthCard в UserPage.
2. Офлайн-модалка для авторизованных (затемнение, карточка по центру, retry, поддержка).
3. Форс-смена пароля при `must_change_password`.
4. ЛК без прокрутки (960×640): устройства + приоритет, VPN-блок (статус/скорость/трафик/лимиты), подписка, смена пароля, тумблер телеметрии.
5. Отчёт: модалка выбора «В файл / На сервер» до генерации; при отправке файл не создаётся.
6. Фикс тумблера телеметрии: PATCH-семантика `/api/config` (не перезаписывать `server.*` устаревшим снапшотом) + мьютекс RMW ServerConfig в Go; отправка телеметрии сразу при старте.
7. DNS-вкладка: без панели адреса сервиса; таблица пользовательских DNS (адрес, тип обычный/DoT/DoH, вкл; пусто изначально; несколько записей); IPv6; текущий системный DNS; активное состояние = в системе прописаны DNS из таблицы (Xbox и/или пользовательские); внешнее изменение DNS → тумблер гаснет. Go: локальный резолвер (plain/DoT/DoH) на 127.0.0.1:53; фикс перетирания DNS в `Load()` (`config.go:374`).
8. Звук: предекод буферов при unlock (первый клик звучит); громкость пользователя выше темы (`theme.sounds` больше не перетирает).
9. Тосты: blur+затемнение при прозрачности.
10. Прогресс-бары: единый компонент (токены, контекстный цвет, анимация); окно авто-подбора в стиле приложения; winprogress — акцентные цвета.
11. Умный подбор (базовый): телеметрия + `strategy_results`; автоподбор «smart» = топ-5 с сервера, фолбэк — классический полный перебор. В ручном режиме: «Умный» / «Классический полный» / «Частичный».
12. Графики: тултип с единицами по метрике (Мбит/с, мс) — фикс `MonitorPage.jsx:107`.
13. Справка: вкладка «Подписки» из `/api/v1/plans` (цены месяц/квартал/год, скидки, условия).
14. Брендинг ZPUI Live (локали ru/en, TitleBar, NSIS, README) + чистое удаление (VPN-адаптер/wintun в NSIS).

## 4. Этапы

0. backup.bat; этот документ. → 1. Контракты выше. → 2. Сервер Go (control/api + vpnctl). → 3. Compose/guard/README (net-eng). → 4. Админка (на контрактах). → 5. Клиент internal/ + web/ (параллельно). → 6. Интеграция на оркестраторе (go vet/test, npm builds, build.bat). → 7. QA E2E (demo-стенд, DEMO_MODE). → 8. Доки (CHANGELOG v2.1.0, Архитектура, Бизнес-логика, Функции, README).

Деплой на прод (server.bat) и коммиты — только по явной команде владельца.
