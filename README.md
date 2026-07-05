<div align="center">

<img src="assets/icon.svg" width="120" height="120" alt="ZPUI logo" />

# ZPUI

### Контроллер обхода DPI (Zapret) для Windows

Управление обходом блокировок, прокси, мониторингом и DNS для Xbox —
в одном приложении с автообновлением и красивым UI.

[![Release](https://img.shields.io/github/v/release/suzcuaru/ZPUI?style=for-the-badge&label=RELEASE&color=0078D4)](https://github.com/suzcuaru/ZPUI/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/suzcuaru/ZPUI/total?style=for-the-badge&label=DOWNLOADS&color=2EA44F)](https://github.com/suzcuaru/ZPUI/releases/latest)
[![Stars](https://img.shields.io/github/stars/suzcuaru/ZPUI?style=for-the-badge&label=STARS&color=FFD700)](https://github.com/suzcuaru/ZPUI/stargazers)
[![License](https://img.shields.io/github/license/suzcuaru/ZPUI?style=for-the-badge&color=8B89CC)](LICENSE)

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-v2.12-FF0000?style=flat-square)](https://wails.io/)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react&logoColor=white)](https://react.dev/)
[![Vite](https://img.shields.io/badge/Vite-5-646CFF?style=flat-square&logo=vite&logoColor=white)](https://vitejs.dev/)
[![Platform](https://img.shields.io/badge/Platform-Windows%2010%2B-0078D4?style=flat-square&logo=windows&logoColor=white)]()

**[Скачать](https://github.com/suzcuaru/ZPUI/releases/latest)** ·
**[Возможности](#-возможности)** ·
**[Установка](#-установка)** ·
**[Архитектура](#-архитектура)** ·
**[Сборка](#-сборка-из-исходников)** ·
**[Вклад](#-вклад)**

</div>

---

## О проекте

**ZPUI** — это десктопное приложение на базе [Wails](https://wails.io/) (Go + React), которое объединяет все инструменты для обхода блокировок под одним интерфейсом:

- Устанавливает и настраивает [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) (обход DPI)
- Автоматически подбирает рабочую стратегию для вашего провайдера
- Управляет встроенным SOCKS5-прокси, мониторингом трафика и DNS для Xbox
- Обновляет само себя и спутники через GitHub Releases
- Сворачивается в системный трей, запускается вместе с Windows

> **Требуется** запуск от имени администратора (управление драйвером WinDivert и маршрутизацией).

---

## Возможности

| Модуль | Что делает |
|:------:|-----------|
| **Zapret** | Запуск/остановка обхода DPI, выбор стратегии, автоподбор рабочих параметров за один клик |
| **Прокси** | Встроенный SOCKS5-прокси для перенаправления трафика через Zapret |
| **Монитор** | Отслеживание статуса процессов и доступности ресурсов в реальном времени |
| **Xbox DNS** | Подбор и применение оптимального DNS-сервера для Xbox |
| **Blockcheck** | Проверка доступности заблокированных ресурсов (Discord, YouTube и др.) |
| **Диагностика** | Сбор системной информации и логов в один архив для отчёта о баге |
| **Автообновление** | Обновление приложения, спутников и модов через GitHub Releases (ETag-кеш, без лимитов) |
| **Трей** | Сворачивание в системный трей, автозапуск с Windows, toast-уведомления |
| **Моды** | Расширяемая система JS-плагинов (свой прокси, sysinfo, Xbox DNS) |

### Ключевые преимущества

- **Без прав администратора для установки** — ставится в профиль пользователя, не требует UAC
- **Умный установщик** — определяет существующую установку и предлагает обновление по версии
- **Сохранение данных** — обновление не затрагивает настройки, логи и конфигурацию Zapret
- **Семантическое сравнение версий** (semver) — корректно определяет, нужно ли обновление
- **Дедупликация уведомлений** — тост о конкретной версии показывается один раз

---

## Установка

### Установщик (рекомендуется)

Скачайте `ZPUI-Setup-x.y.z.exe` из [последнего релиза](https://github.com/suzcuaru/ZPUI/releases/latest) и запустите.

| Шаг | Описание |
|:---:|---------|
| 1 | **Лицензия MIT** — примите условия использования |
| 2 | **Папка установки** — по умолчанию `%LOCALAPPDATA%\Programs\ZPUI` (без прав админа) |
| 3 | **Ярлыки** — меню «Пуск» и/или рабочий стол (на выбор) |
| 4 | **Готово** — ZPUI запускается автоматически |

> При повторном запуске установщик **автоматически определит** установленную версию:
> - **Версия старее** → тихое обновление с сохранением данных
> - **Версия та же** → предложение переустановки
> - **Версия новее** → предупреждение о понижении версии

### Портативная версия

Скачайте `zpui.zip` из [релиза](https://github.com/suzcuaru/ZPUI/releases/latest), распакуйте в любую папку и запустите `zpui.exe`.

---

## Архитектура

```
┌─────────────────────────────────────────────────────────────┐
│                      Фронтенд (React)                        │
│         web/src/api.js  →  window.go.app.App.*              │
└──────────────────────────┬──────────────────────────────────┘
                           │ Wails IPC (биндинги + события)
┌──────────────────────────┴──────────────────────────────────┐
│              Ядро приложения (Go, package app)               │
│                  internal/app/app.go                         │
│      app_api*.go · versions.go · app_devices.go              │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────┴──────────────────────────────────┐
│                  internal/ (доменные пакеты)                 │
│                                                              │
│  zapret   — обход DPI (стратегии, автотест, служба)         │
│  wizard   — мастер первичной настройки (внутрипроцессно)    │
│  autoselect — движок автоподбора стратегии                  │
│  proxy    — SOCKS5-прокси                                   │
│  monitor  — мониторинг трафика                              │
│  xboxdns  — DNS для Xbox                                    │
│  updater  — обновления (ETag-кеш, semver, спутники)        │
│  config · database · logger · notify · tray · mods          │
└─────────────────────────────────────────────────────────────┘
                           │
┌──────────────────────────┴──────────────────────────────────┐
│          cmd/ (сателлиты — отдельные .exe)                  │
│                                                              │
│  selfupdate    — самообновление ZPUI                         │
│  zapretupdate  — обновление Zapret                           │
│  wizard        — CLI-обёртка над internal/wizard             │
│  autoselect    — CLI-обёртка над internal/autoselect         │
└─────────────────────────────────────────────────────────────┘
```

**Поток данных:** фронтенд вызывает `api('GET', '/api/...')` → `web/src/api.js` маршрутизирует вызов в Wails-биндинг `func (a *App)` → метод работает с пакетами из `internal/` и возвращает `map[string]interface{}`.

### Стек

| Слой | Технологии |
|------|-----------|
| Бэкенд | Go 1.25, [Wails v2.12](https://wails.io/) |
| Фронтенд | React 18, Vite 5 |
| База данных | SQLite (modernc.org/sqlite — чистый Go, без CGO) |
| Обход DPI | [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) |
| Установщик | NSIS (per-user, автообновление по версии) |

> Полная структура репозитория описана в [**STRUCTURE.md**](STRUCTURE.md).

---

## Сборка из исходников

### Требования

| Инструмент | Версия | Установка |
|-----------|--------|-----------|
| [Go](https://go.dev/dl/) | 1.25+ | `winget install GoLang.Go` |
| [Node.js](https://nodejs.org/) | 20+ | `winget install OpenJS.NodeJS.LTS` |
| [Wails CLI](https://wails.io/) | v2.12+ | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0` |
| [NSIS](https://nsis.sourceforge.io/) | 3.x | `winget install NSIS.NSIS` (для установщика) |

### Быстрая сборка

```bat
build.bat
```

Скрипт выполнит:
1. Автоинкремент версии (`version.txt`)
2. Сборку фронтенда (`npm run build`)
3. Сборку ядра Wails (`zpui.exe`)
4. Сборку 4 сателлитов (`wizard`, `autoselect`, `selfupdate`, `zapretupdate`)
5. Сборку `versions.json` и копирование модов в `build/dist/`

### Сборка установщика

```bash
makensis /DVERSION=1.0.49 /DDIST=build\dist /DICON=build\windows\icon.ico \
         /DLICENSE=LICENSE /DOUTDIR=build installer\ZPUI.nsi
```

### Сборка вручную

```bash
# 1. Фронтенд (обязательно: бэкенд встраивает web/dist через go:embed)
cd web && npm install && npm run build && cd ..

# 2. Ядро (Wails)
wails build -platform windows/amd64 -s -skipbindings -o zpui.exe \
    -ldflags "-s -w -H windowsgui -X main.version=1.0.49" -trimpath

# 3. Сателлиты
go build -ldflags "-s -w -H windowsgui -X main.version=1.0.49" -trimpath ./cmd/wizard/
go build -ldflags "-s -w -H windowsgui -X main.version=1.0.49" -trimpath ./cmd/autoselect/
go build -ldflags "-s -w -H windowsgui -X main.version=1.0.49" -trimpath ./cmd/selfupdate/
go build -ldflags "-s -w -H windowsgui -X main.version=1.0.49" -trimpath ./cmd/zapretupdate/
```

---

## Разработка

### Проверка и тесты

```bash
# Backend
go vet ./...        # статический анализ — должен проходить
go test ./...       # unit-тесты (config, monitor, zapret)
go build ./...      # компиляция всех пакетов

# Frontend (из web/)
npm run build       # production-сборка (vite) — должна проходить
npm test            # vitest (однократно)
npm run dev         # dev-сервер на :3000
```

> `go build ./...` требует собранный фронтенд (`web/dist/`), т.к. бэкенд встраивает его через `//go:embed all:web/dist`.

### Добавление новой настройки / эндпоинта

1. Добавить поле в `internal/config/config.go` (`Config`) + значение по умолчанию в `defaultConfig()`
2. Добавить метод `func (a *App) ...` в один из `internal/app/app_api*.go` (биндинг Wails)
3. Зарегистрировать маршрут в `web/src/api.js`
4. Добавить строки в `web/src/locales/{ru,en}.json` + использовать через хук `useT`

Подробности — в [**AGENTS.md**](AGENTS.md) и [**STRUCTURE.md**](STRUCTURE.md).

---

## Релизы

Сборка релиза запускается автоматически при пуше тега `v*` (например `v1.0.49`) через [GitHub Actions](.github/workflows/release.yml).
Также доступен ручной запуск: вкладка **Actions → Release → Run workflow**.

Релиз включает:

| Артефакт | Описание |
|----------|---------|
| `ZPUI-Setup-x.y.z.exe` | NSIS-установщик (per-user, автообновление по версии) |
| `zpui.zip` | Портативная версия |
| `versions.json` | Манифест версий для механизма автообновления |
| `*.exe` (спутники) | `wizard`, `autoselect`, `selfupdate`, `zapretupdate` |

---

## Вклад

Пулл-реквесты приветствуются! Для масштабных изменений сначала откройте issue для обсуждения.

1. Fork → branch (`git checkout -b feature/amazing`)
2. Коммитьте, следуя conventional commits (`feat:`, `fix:`, `refactor:`, `docs:`)
3. Убедитесь, что `go vet ./...`, `go test ./...`, `npm run build`, `npm test` проходят
4. Pull request → describe what & why

---

## Статистика

<div align="center">

| | |
|:---:|:---:|
| ![GitHub Repo stars](https://img.shields.io/github/stars/suzcuaru/ZPUI?style=social) | ![GitHub forks](https://img.shields.io/github/forks/suzcuaru/ZPUI?style=social) |
| ![GitHub watchers](https://img.shields.io/github/watchers/suzcuaru/ZPUI?style=social) | ![GitHub contributors](https://img.shields.io/github/contributors/suzcuaru/ZPUI) |
| ![GitHub issues](https://img.shields.io/github/issues/suzcuaru/ZPUI) | ![GitHub closed issues](https://img.shields.io/github/issues-closed/suzcuaru/ZPUI) |
| ![Repo Size](https://img.shields.io/github/repo-size/suzcuaru/ZPUI) | ![Last Commit](https://img.shields.io/github/last-commit/suzcuaru/ZPUI) |
| ![GitHub Release Date](https://img.shields.io/github/release-date/suzcuaru/ZPUI) | ![GitHub commits since](https://img.shields.io/github/commits-since/suzcuaru/ZPUI/latest) |

</div>

---

## Лицензия

[MIT License](LICENSE) — Copyright (c) 2026 SuzucaRU

<div align="center">

**ZPUI** использует [zapret-discord-youtube](https://github.com/flowseal/zapret-discord-youtube) от [flowseal](https://github.com/flowseal) и [WinDivert](https://github.com/basil00/Reqable) для обхода DPI.

ZPUI не связан с разработчиками zapret, WinDivert или Discord/YouTube.

</div>
