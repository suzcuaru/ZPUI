# ZPUI — Пакет исправлений и доработок

## Что входит

### P0 — Критические исправления багов (7)
| # | Файл | Описание |
|---|------|----------|
| 1 | `internal/app/app.go` | Мутация слайса через append — порча данных ресурсов |
| 2 | `internal/xboxdns/manager.go` | Частичное включение DNS + hardcoded "Ethernet" + нет IPv6 + locale-dependent parsing |
| 3 | `internal/singleinstance/singleinstance.go` | Unchecked taskkill — ошибка игнорируется |
| 4 | `web/src/api.js` | DELETE-маршрут передаёт пустую строку |
| 5 | `web/src/pages/DashboardPage.jsx` | checkingNow не сбрасывается при ошибке → кнопка заблокирована навсегда |
| 6 | `web/src/pages/SettingsPage.jsx` | Poll-loop без cleanup при unmount (утечка памяти) |
| 7 | `internal/tray/tray.go` | Race condition при рестарте из трея + произвольный Sleep(2s) |

### P1 — Важные улучшения (8)
| # | Файл | Описание |
|---|------|----------|
| 8 | `internal/app/app.go` | GetCachedResourcePercent учитывает User-ресурсы в трее |
| 9 | `internal/autoselect/autoselect.go` | Утечка zapret.Manager — добавлен defer Stop() |
| 10 | `internal/logger/logger.go` | Структурированное логирование ERROR/WARN в БД |
| 11 | `internal/logger/logger.go` | ReadRecent() — предупреждение о больших лог-файлах |
| 12 | `internal/monitor/traffic.go` | TODO: замена PowerShell на gopsutil (100x быстрее) |
| 13 | `web/src/api.js` | Error propagation вместо молчаливого null |
| 14 | `web/src/hooks/useDebouncedSave.js` | Race condition — version counter + cleanup on unmount |
| 15 | `internal/config/config.go` | Новая секция `diagnostic_reports` в конфиге |

### Новая система: Диагностические отчёты (8 файлов)
| # | Файл | Описание |
|---|------|----------|
| 16 | `internal/database/db.go` | Таблицы `error_logs` + `diagnostic_reports` |
| 17 | `internal/database/models.go` | Модели `ErrorLog` + `DiagnosticReport` |
| 18 | `internal/database/queries.go` | Запросы для ошибок, статистики, отчётов |
| 19 | `internal/reports/generator.go` | Генератор MD-отчётов |
| 20 | `internal/reports/scheduler.go` | Планировщик с настраиваемой частотой |
| 21 | `internal/reports/uploader.go` | Автозагрузка на Яндекс.Диск (WebDAV) |
| 22 | `internal/app/app_api_reports.go` | Wails API: генерация, история, скачивание |

## Применение

### Автоматическое (рекомендуется)
```bash
cd /path/to/ZPUI
bash ZPUI_FIXES/APPLY_PATCHES.sh
```

Скрипт:
1. Создаёт бэкап оригинальных файлов в `ZPUI_FIXES/backup/<timestamp>/`
2. Заменяет 15 файлов на исправленные версии
3. Добавляет 4 новых файла (система отчётов)
4. Запускает `go mod tidy`

### Ручное
Скопируйте файлы из `replacements/` поверх соответствующих файлов.
Скопируйте файлы из `new_files/` в соответствующие директории проекта.

## Конфигурация отчётов

Добавляется в `config.json`:
```json
{
  "diagnostic_reports": {
    "enabled": false,
    "frequency": "weekly",
    "period_days": 7,
    "yandex_disk_upload": {
      "enabled": false,
      "public_key": ""
    },
    "auto_save_md": true
  }
}
```

| Режим | period_days | Для |
|-------|-------------|------|
| `hourly` | 1 | debug/dev |
| `daily` | 1 | dev |
| `weekly` | 7 | default |
| `biweekly` | 14 | stable |
| `monthly` | 30 | release |

## Откат
```bash
cp -r ZPUI_FIXES/backup/<timestamp>/* /path/to/ZPUI/
```

## Опциональные зависимости

Для оптимизации мониторинга трафика:
```bash
go get github.com/shirou/gopsutil/v4/net
```
Без gopsutil — автоматически используется PowerShell fallback (работает, но медленнее).