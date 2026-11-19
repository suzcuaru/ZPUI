# ZPUI Live — остаток работ (пауза 27.09.2026, v2.1.2 задеплоен)

> Состояние: v2.1.2 в проде (панель CRM, боты, DNS plain-only, VPN-режимы, база без триала).
> Этот документ — очередь следующего раунда. Всё согласовано с владельцем, контракты зафиксированы.

## 1. Платежи и Telegram-бот (главный остаток)

Бот (`server/tgbot`) **готов**: инлайн-подписки/платежи, webhook+long-polling, `TELEGRAM_API_BASE`
(api.telegram.org заблокирован на сервере и у владельца), SQLite-история/чеки/ретеншн, 21 тест.
Не хватает **серверной части** (control/api):

### Контракт internal-эндпоинтов (X-Internal-Token, ответы {status}/{error})

1. `GET /internal/settings` → карта настроек БЕЗ секретов. Бот читает: `subscribe_card_number`,
   `subscribe_payment_note` (шаблон, плейсхолдеры {login} {id} {sum} {plan} {period} {days}),
   `subscribe_currency` (₽), `subscribe_discount_enabled` (true), `subscribe_discount_max_percent` (50),
   `subscribe_bot_retention_days` (180). Спеки уже в settings.go.
2. `POST /internal/telegram/me {chat_id}` → bound:true/false + account {id, login, status, plan_code,
   plan_name, plan_period, plan_price, plan_duration_days, vpn_access, devices_limit, devices_count,
   expires_at}.
3. `POST /internal/telegram/bind-code {chat_id}` → {code, expires_at} (6 цифр, TTL 15 мин,
   telegram_tokens kind='bind_code') + публичный `POST /api/v1/account/telegram/bind-code {code}` (auth).
4. `POST /internal/recover/password {chat_id}` → {code, expires_at} (по chat_id; rate-limit).
5. `POST /internal/payments/request {chat_id, login, type: new|renew|switch|change_period, plan_code,
   period, days, price, discount, total, remaining_days}` → {id} | 409 pending_exists.
   total пересчитать сервером (скидка = остаток_дней × цена_периода/длительность, потолок
   subscribe_discount_max_percent). days = 30/90/365 (или из plan_prices.days — см. п.3).
6. `GET /internal/payments/request/{id}` → статус заявки (для кнопки «Обновить статус»).
7. `POST /internal/subscription/cancel {chat_id}` → мгновенное завершение подписки (expires_at=now);
   not_allowed для бессрочных. Деньги за неиспользованные дни не возвращаются (текст бота).
8. `GET /internal/payments/{id}/receipt` → binary чека.
9. Опц.: в `GET /api/v1/plans` → prices[] + `days`:30|90|365 (бот уберёт константы).
10. Пуш боту: `POST http://tgbot:9102/notify` {type: payment_approved|payment_rejected|
    subscription_cancelled, chat_id, request_id, login, plan_code, period, total, comment, has_receipt}.

### Админ-API (панель уже рисует UI дефенсивно — этап «Платежи/Баланс»)

- `GET /admin/api/payments/requests?status=&q=` → {data:[...]}.
- `POST /admin/api/payments/{id}/approve` (multipart receipt ≤10МБ | {receipt_text}) → approve +
  применить оплаченное (new/switch → set-plan; renew → max(now,expires)+период; change_period →
  пересчёт даты) + пуш боту + чек пользователю.
- `POST /admin/api/payments/{id}/reject {reason}` → rejected + пуш (reason уйдёт пользователю).
- `GET /admin/api/payments/{id}/receipt` → binary.
- `GET /admin/api/balance/summary` → {income_total, income_month, income_expected_pending,
  by_plan[], by_month[], top_payers[], payments[]}.
- Таблица payment_requests (миграция): id, user_id, login, type, plan_code, period, days, price,
  discount, total, remaining_days, status, reason, decided_by, decided_at, receipt_file, created_at.
- В /admin/api/accounts: + payment_status (последняя заявка), registered_at.

### Настройки (уже в settingSpecs, заполняются в панели → «Оформление подписки»)

subscribe_bot_url, subscribe_site_url, subscribe_boosty_url, subscribe_card_number,
subscribe_payment_note («ZPUI {login} {id}»), subscribe_retention_days (180).

## 2. DNS — верификация на живой машине (код готов, требуется проверка владельцем)

- «Сейчас в системе» (реестр-чтение) — на машине владельца ранее было пусто при прописанных
  адресах; переработка на GetAdaptersAddresses+реестр должна это закрыть — проверить после установки.
- Мастер-тумблер при пустой пользовательской таблице + xbox-пресет: pre-flight обязан проверять
  пресетные серверы и включаться (раньше отказ «no DNS servers configured»).
- Сайдбар/hero/Обзор — единый статус (web: dnsUiState; go: status.xbox_dns).

## 3. Телеметрия — e2e-проверка после установки 2.1.2

Фиксы в коде: один писатель, анти-клоббер пустотой, тесты. Если тумблер снова сбросится —
в журнале Диагностики будут ошибки с кодами (frontlog) → репорт в задачу.

## 4. wstunnel (обфускация VPN) — активация

1. DNS пула сайтов → IP сервера; сертификат Let's Encrypt (или self-signed) в том zpui-vpn-tls.
2. `.env`: `VPN_OBFS_MODE=wstunnel`, `VPN_TLS_NAMES=<домены через запятую>`,
   `VPN_WSTUNNEL_PORT=8443`; открыть TCP 8443 наружу (UDP 51820 можно закрыть).
3. Клиент: компонент `components/wstunnel/wstunnel.exe` (дистрибутив Яндекс.Диск + build.bat,
   как xray) — без него статус «obfs_component_missing».
4. Формула клиента (внутренняя): `wstunnel client --tls-sni-override <сайт> -L udp://127.0.0.1:<p>:<wg_host>:<wg_port>?timeout_sec=0 wss://<сайт>:8443/v1`.
5. Ограничение: против активного пробинга не спасает; MTU рекомендован 1280–1400.

## 5. Прочее

- Токен Telegram-бота — в `.env` (TELEGRAM_BOT_TOKEN); при утечке — revoke в @BotFather.
- Регистрация webhook (с незаблокированной машины): `curl "https://api.telegram.org/bot$TOKEN/setWebhook" -d "url=https://<guard>/telegram/webhook/$SECRET" -d "secret_token=$SECRET" -d 'allowed_updates=["message","callback_query"]' -d "max_connections=4"`.
- store_test.go восстанавливался агентом после случайной очистки — сверить с QA-отчётом.
- CHANGELOG.md актуален для 2.1.2; перед следующим релизом дополнить платежами/ботом.
