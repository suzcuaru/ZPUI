<div align="center">

# ZPUI

**Контроллер обхода DPI (Zapret) для Windows**

Управление обходом блокировок, прокси, мониторингом и DNS для Xbox — в одном приложении с автообновлением.

</div>

---

## Возможности

- **Zapret** — запуск/остановка и настройка обхода DPI, выбор стратегии, авто-подбор рабочих параметров
- **Прокси** — встроенный прокси-сервер для перенаправления трафика
- **Монитор** — наблюдение за статусом процессов и ресурсами в реальном времени
- **Xbox DNS** — подбор и применение лучшего DNS для Xbox
- **Blockcheck** — проверка доступности заблокированных ресурсов
- **Диагностика** — сбор системной информации и логов
- **Моды** — расширяемая система модулей с собственным обновлением
- **Автообновление** — приложение обновляет само себя и спутники через GitHub Releases
- **Трей** — сворачивание в системный трей, автозапуск

## Установка

### Установщик (рекомендуется)

Скачайте `ZPUI-Setup-x.y.z.msi` из [последнего релиза](https://github.com/suzcuaru/ZPUI/releases/latest) и запустите.

- Устанавливается **без прав администратора** в `%LOCALAPPDATA%\Programs\ZPUI`
- Создаёт ярлыки в меню «Пуск» и на рабочем столе
- Обновляется из приложения (Параметры → Обновления)

### Портативная версия

Скачайте `zpui.zip` из [релиза](https://github.com/suzcuaru/ZPUI/releases/latest), распакуйте и запустите `zpui.exe`.

> Приложению требуется запуск от имени администратора (для управления WinDivert/маршрутизацией).

## Сборка из исходников

Требования: [Go](https://go.dev/dl/) 1.21+, [Node.js](https://nodejs.org/) 20+, [Wails CLI](https://wails.io/).

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Быстрая сборка (всё в `build/dist`):

```bat
build.bat
```

Сборка вручную:

```bash
# Фронтенд
cd web
npm install
npm run build
cd ..

# Основное приложение (Wails)
wails build -platform windows/amd64 -s -skipbindings -o zpui.exe \
    -ldflags "-s -w -H windowsgui -X main.version=1.0.0" -trimpath

# Спутники
go build -ldflags "-s -w -H windowsgui" -trimpath ./cmd/wizard/
go build -ldflags "-s -w -H windowsgui" -trimpath ./cmd/autoselect/
go build -ldflags "-s -w -H windowsgui" -trimpath ./cmd/selfupdate/
go build -ldflags "-s -w -H windowsgui" -trimpath ./cmd/zapretupdate/
```

## Архитектура

| Слой | Технологии |
|------|-----------|
| Бэкенд | Go, [Wails v2](https://wails.io/) |
| Фронтенд | React, Vite |
| База данных | SQLite |
| Обход DPI | [zapret](https://github.com/bol-van/zapret) (управляется как внешний процесс) |

### Структура

```
app*.go, main.go          — ядро Wails, биндинги к фронтенду
internal/zapret           — управление обходом DPI
internal/proxy            — прокси-сервер
internal/monitor          — мониторинг
internal/xboxdns          — DNS для Xbox
internal/updater          — автообновление и спутники
internal/database         — SQLite хранилище
internal/config           — конфигурация
cmd/{autoselect,wizard,selfupdate,zapretupdate} — спутники
web/                      — React + Vite фронтенд
```

## Обновления и релизы

Сборка релиза запускается автоматически при пуше тега `v*` (например `v1.0.47`) через GitHub Actions.
Релиз включает MSI-установщик, портативную версию и манифест `versions.json`, который используется
механизмом автообновления.

## Лицензия

См. [LICENSE](LICENSE).
