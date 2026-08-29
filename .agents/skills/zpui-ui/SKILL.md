---
name: zpui-ui
description: Интерфейс и дизайн ZPUI — дизайн-система на CSS-токенах, компоненты web/src/components, i18n, фиксированное окно 960×640, паттерны страниц, модалок и настроек. Используй при любой работе с UI — «добавь кнопку/карточку/страницу/тумблер», «поменяй стиль», «сделай красиво», «не работает вёрстка», «переведи» — даже если слово «дизайн» не произнесено.
---

# Интерфейс и дизайн ZPUI

## 0. Сначала дизайн-док

Перед нетривиальной UI-работой открой `Документы/Дизайн-и-типографика.md` — визуальный язык, токены, правила i18n.

## 1. Токены — только CSS-переменные

Единственный источник — `web/src/styles/index.css`: `--bg-page/--bg-section/--bg-card/--bg-hover/--bg-inset`, `--text-primary/--text-secondary/--text-tertiary`, `--accent(-hover/-soft/-text)`, `--success/--danger/--warning` (+ пары `-bg`), `--radius-sm/md/lg`, `--shadow-sm/md/lg`, `--ease-*`, `--dur-*`, `--header-h/--sidebar-w/--footer-h`, шрифты `--font` (Inter) и `--font-mono` (JetBrains Mono).

Хардкод-цветов и магических чисел нет. Примеры правил: опасная зона («Полная переустановка» и т.п.) — всегда с рамкой `var(--danger)`; панели/карточки настроек — без обводок.

## 2. Компоненты — переиспользуй, не дублируй

- Базовые кирпичики — `web/src/components/ui/`: `Modal`, `Row`, `Switch`, `ConfirmDialog`, `ReleaseModal`, `ZapretVersionsModal`.
- Фичевые модалки — уровнем выше в `web/src/components/` (`AutoSelectModal`, `DiagnosticsModal`, `UpdateProgressModal`, `PhoneQrModal`, ...), каркас — в `layout/`, `navigation/`, `feedback/`.
- Новый переиспользуемый элемент — в `ui/`; страницы — `web/src/pages/` (Dashboard, Zapret, Proxy, XboxDns, Monitor, Sync, Report, Settings, Docs).
- Логика — в хуки `web/src/hooks/` (`usePolling`, `useDebouncedSave`, `useServiceToggle`, `useUpdateCheck`, `useUpdateFlow`).

## 3. i18n

Все пользовательские строки — `useT()`/`t('...')` в компонентах, `tr()` вне React; ключи сразу в **оба** файла `web/src/locales/ru.json` и `en.json`. Захардкоженных русских строк в JSX быть не должно.

## 4. Неприкосновенное

- Окно ровно 960×640, maximize отключён (`internal/app/window_windows.go`) — проектируй внутри этих рамок, менять нельзя.
- Иконка Windows-уведомлений — только в шапке toast'а из `appicon.ico`, без иконок в теле.
- Telegram-ссылки открываются только через `tg://` (`OpenTelegramLink`, `/api/telegram/open`) — не браузер и не http.

## 5. Паттерны поведения

- Настройки: `Row` + `Switch` с сохранением через `useDebouncedSave`; новое поле — и в `GetConfig()`, и в `SetConfig()` (см. скилл `zpui-dev`).
- Долгие операции — модалка с прогрессом, обновляемая через `EventsOn`/поллинг; никаких «зависших» кнопок без индикации.
- Индикаторы в сайдбаре: состояние вычисляет бэкенд (`/api/status`), чтобы индикатор переживал закрытие/переоткрытие окна (образец — индикатор экономного режима).
- Тултипы: горизонтальная ориентация с клампингом у краёв (`--tt-ml`); у левого края экрана — `pos="right"`.
- Баннер-подсказки на дашборде (образец — баннер YouTube → «Включить Xbox DNS»): проблема + одна явная кнопка действия.

## 6. Проверка

`cd web && npm run build` (обязательно), `npm test` (если менял утилиты/хуки), визуальная проверка — `npm run dev` на :3000.
