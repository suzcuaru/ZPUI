# ZPUI API Server — интеграция админ-CRM

Документация для нового фронтенда админ-панели (Next.js CRM) и любых будущих
клиентов API. Сервер: `../ZPUI-Server/control/api` (Go, stdlib `net/http` + PostgreSQL),
брендинг — **ZPUI API Server**.

> Смежные документы: [`ZPUI-Server/README.md`](../../ZPUI-Server/README.md) (полное описание
> сервера и деплоя), [`Документы/Задачи/Админ-CRM-Next.md`](./Задачи/Админ-CRM-Next.md)
> (план перехода на новое приложение).

---

## 1. Базовые сведения

| Параметр | Значение |
|---|---|
| Адрес | `ADDR` (по умолчанию `:8080`; в проде за guard на `:4430`) |
| Проверка живости | `GET /healthz` → `{"status":"ok","service":"ZPUI API Server","version":"...","demo":false}` |
| Админ-API | `/admin/api/*` |
| Клиентский API | `/api/v1/*` |
| Межсервисный API | `/internal/*` (токен `X-Internal-Token`) |
| Админ-статика | `/admin/` → папка `web/admin` (собранный фронтенд) |
| Формат ответов | см. §3 |

## 2. Аутентификация админ-панели

1. `POST /admin/api/auth/login` с телом `{"login":"...","password":"..."}`
   → `{"status":"ok","token":"...","expires_at":"...","login":"...","role":"..."}`.
   Токен живёт 24 часа (таблица `admin_sessions`), bcrypt на сервере.
2. Каждый запрос: заголовок `Authorization: Bearer <token>`.
3. `GET /admin/api/auth/me` — проверить токен и получить `{login, role, id}`.
4. `POST /admin/api/auth/logout` — выйти (удалить сессию).
5. `401` / `{"error":"unauthorized"}` на любом запросе → сбросить сессию и
   показать экран логина.
6. Сервер дополнительно ограничивает админку по IP: только loopback, частные
   сети и Tailscale `100.64.0.0/10`. Публичный IP получит `403` даже с токеном.
7. В `DEMO_MODE=1` логин/пароль: `admin` / `admin123`, также принимается
   статический токен `demo-token` (заголовок `X-Admin-Token`).

Хранение на фронте: `localStorage['zpui-admin-session']` =
`{token, expires_at, login, role}` (как в текущей панели).

## 3. Формат ответов

- Успех: `{"status":"ok", ...поля}`; списки: `{"status":"ok","data":[...]}`.
- Ошибка: `{"error":"<код>", "detail":"<пояснение, опционально>"}` с
  соответствующим HTTP-статусом.
- Коды ошибок: `bad_request`, `unauthorized`, `forbidden`, `not_found`,
  `rate_limited` (+`retry_after`), `internal`.
- `detail` — человекочитаемое пояснение валидации (например,
  `"devices_limit: 1..10"`); можно показывать в тостах.

## 4. Конфигурация приложения (заглушка будущего API)

Приложение читает статический конфиг `app.config.json` из своей выдачи
(для Next.js — `public/app.config.json`):

```json
{
  "apiBase": "/admin/api",
  "appName": "ZPUI API Server CRM",
  "localApi": {
    "enabled": false,
    "base": "http://127.0.0.1:9110",
    "note": "Будущий собственный API — см. раздел 7. Пока заглушен."
  }
}
```

Правило: **все** запросы идут через единый клиент `api(method, path, body)`,
который берёт базу из `apiBase`; прямые `fetch`/`window.*` из компонентов
запрещены. Когда в будущем включается `localApi.enabled`, тот же клиент
переключается на `localApi.base` без изменений в компонентах.

## 5. Карта эндпоинтов админ-CRM

Полная таблица — в `../ZPUI-Server/README.md`. Кратко, по доменам (все под
`/admin/api`, кроме `auth/login` — за IP-гейтом):

| Домен | Эндпоинты |
|---|---|
| Auth | `POST auth/login`, `GET auth/me`, `POST auth/logout` |
| Обзор | `GET overview`, `GET stats` (14 дней для графиков), `GET events?limit&kind&q` |
| Пользователи | `GET accounts?q=`, `GET accounts/{id}`, `POST accounts`, `DELETE accounts/{id}`, `accounts/export` (CSV); действия: `set-plan`, `renew`, `set-expiry`, `extend`, `limit`, `ban`, `note`, `reset-password`, `tester`, `revoke-sessions` |
| Приглашения | `GET/POST invites`, `invites/summary`, `invites/revoke`, `invites/bulk-delete`, `DELETE invites/{id}` |
| Устройства | `GET devices?q&account_id`, `POST devices/{id}/tester`, `DELETE devices/{id}`; сессии: `GET sessions`, `POST sessions/cleanup` |
| Тарифы | `GET/POST plans`, `PUT/DELETE plans/{id}` — контракт см. §6 |
| Финансы | `GET finance/summary`; промокоды: `GET/POST promos`, `POST promos/{code}/enable|disable` |
| VPN/ноды | `GET vpn`, `POST vpn/peer/{id}/enable|disable`; `GET/POST nodes`, `POST nodes/{id}/enable|disable` |
| TG Proxy | `GET/PUT tgproxy`, `POST tgproxy/restart`, `GET tgproxy/stats?hours`, `GET tgproxy/devices`, `POST tgproxy/devices/grant|revoke` |
| Telegram | `GET telegram/status`, `POST tg/broadcast` |
| Обновления | `GET/POST updates/{zpui|zapret|components}/list|publish|flags|delete`, `GET/PUT updates/{kind}/settings` |
| Баннеры | `GET/POST banners`, `PUT/DELETE banners/{id}`, `GET banners/dismiss-stats` |
| Телеметрия | `GET telemetry`, `GET reports`, `GET reports/{id}`, `GET reports/{id}/raw` |
| Инфра | `GET infra/status`, `GET infra/proxy`, `infra/backup/*` (info/run/status/verify/log), `GET infra/logs?service&tail` |
| БД-админ | `GET db/tables`, `GET db/table/{name}`, `POST db/query`, `GET db/activity`, `POST db/kill`, `POST db/maintenance` |
| Настройки | `GET/PUT settings` (реестр типизированных ключей) |
| Тестеры | `POST testers/token`, `GET testers/tokens` |
| Бэкапы файлов | `GET backup/list`, `GET backup/download`, `POST backup/upload|delete|restore` |

## 6. Контракт тарифов (частая ошибка — «не сохраняется тариф»)

Модель `Plan`: `id, code, name, price, period, duration_days, devices_limit,
speed_total_mbps, speed_per_device_mbps, traffic_gb, vpn_access, tg_proxy,
features (многострочная строка), active, sort, prices[]`.

Обязательные поля и ограничения (иначе `400 bad_request` + `detail`):

- `code` — непустой (приводится к нижнему регистру), `name` — до 32 символов;
- `period` — строго `month` | `quarter` | `year`;
- `duration_days` — **1..3650** (30 / 90 / 365);
- `devices_limit` — **1..10**;
- `price >= 0`, `speed_total_mbps 0..100000`, `speed_per_device_mbps 0..10000`,
  `traffic_gb 0..1000000`;
- `prices` — **массив** `[{period, price, discount_percent}]`
  (`discount_percent 0..90`); объект `{month:{price,discount_percent},...}`
  тоже принимается (совместимость со старой панелью).

Пример корректного сохранения:

```json
PUT /admin/api/plans/1
{
  "code": "plus",
  "name": "ZPUI Plus",
  "period": "month",
  "duration_days": 30,
  "devices_limit": 5,
  "price": 150,
  "speed_total_mbps": 0,
  "speed_per_device_mbps": 0,
  "traffic_gb": 0,
  "vpn_access": true,
  "tg_proxy": true,
  "features": "Туннель для ПК и телефона\nОблачный Telegram-прокси",
  "active": true,
  "sort": 1,
  "prices": [
    {"period": "month", "price": 150, "discount_percent": 0},
    {"period": "quarter", "price": 405, "discount_percent": 10},
    {"period": "year", "price": 1440, "discount_percent": 20}
  ]
}
```

`POST /admin/api/plans` — upsert по `code`; `PUT /admin/api/plans/{id}` —
обновление по id; `DELETE` — удаление.

## 7. Будущий собственный API (пока заглушка в конфиге)

План: интерфейс CRM должен уметь работать не только с ZPUI API Server, но и с
отдельным «локальным» бэкендом другой системы. Механизм переключения заложен в
`app.config.json` → `localApi` (сейчас `enabled: false`).

Контракт, которому должен соответствовать будущий локальный API (тот же
конверт ответов из §3):

1. `POST {base}/auth/login` → `{token, expires_at, login, role}`.
2. `GET {base}/overview` → счётчики дашборда.
3. `GET {base}/accounts`, `GET {base}/accounts/{id}` + действия над аккаунтом.
4. `GET/POST {base}/plans`, `PUT/DELETE {base}/plans/{id}` (контракт §6).
5. `GET {base}/events`, `GET {base}/stats`.

Требования: Bearer-аутентификация как в §2, CORS для источника админки либо
reverse-proxy на том же origin, совместимость полей с §5 (маппинг описывается
в адаптере клиента `api.js`). Пока `localApi.enabled=false`, клиент обязан
использовать `apiBase` и вести себя как сегодня.

## 8. Правила фронтенда (конвенции)

- Запросы — только через клиент `api(method, path, body)` с таймаутом ~20 c.
- Долгие операции (публикация обновлений, backup run/verify) — запуск + поллинг
  статуса раз в ~2 с, таймаут 120 с.
- Все строки — через i18n (`locales/ru.json` + `en.json`, ключи с точками).
- Токены дизайна — CSS-переменные; никаких хардкод-цветов.
- Сайдбар — фиксированный, контент скроллится отдельно: никаких выезжающих
  панелей, сдвигающих макет (причина «дёрганья» старой панели).
