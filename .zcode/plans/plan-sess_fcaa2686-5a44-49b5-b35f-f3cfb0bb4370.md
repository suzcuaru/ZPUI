# План: новая система обновлений + фиксация аудита структуры

## Решения (подтверждены)
- Модуль `cmd/selfupdate` (Центр обновлений) удаляется полностью.
- ZPUI: качаем **только NSIS-установщик, только с Яндекс.Диска** (без GitHub-фолбэка) → «Установить» → приложение закрывается → запускается установщик (видимый, как есть — **в ZPUI.nsi правок нет**; легаси-строки про selfupdate.exe сохраняем для апгрейда со старых версий).
- **Zapret — два источника:** версия проверяется **и на Яндекс.Диске, и на GitHub** (релизы Flowseal/zapret-discord-youtube); скачиваем **оттуда, где версия новее**; если оттуда скачать не удалось — **автоматический фолбэк на Яндекс.Диск**.
- Zapret ставится самим приложением (zip → «Установить» → ин-апп `InstallFromZip`).
- Яндекс.Диск: в корне только установщики `ZPUI-Setup-<v>-win32.exe` без папок версий; zapret — как раньше (`Zapret <v>/`).
- Рефакторинг структуры — НЕ сейчас; полный аудит фиксируется в `Документы/Задачи/`.

## Этап 1. Zapret: проверка и скачивание с двух источников

**`internal/zapret/updater.go` + `internal/updater/satellite.go`:**
- `CheckForUpdates()` → мульти-источник: Яндекс (`YandexZapretVersion`, папки `Zapret <v>`) **и** GitHub (`zapretGithubAPIURL` — тег последнего релиза Flowseal + URL zip-asset'а). Ошибка/недоступность одного источника не ломает другой.
- Итог проверки: `latest_version` = более новая из двух (существующий `CompareVersions`), `source` = выигравший источник, `download_url` (GitHub asset или zip с Диска) + запасной `fallback_url` (всегда Яндекс).
- `DownloadUpdateInfo()` → качает с выбранного источника; **при ошибке скачивания — повторная попытка с `fallback_url` (Яндекс)**. При равенстве версий приоритет — Яндекс (быстрее/надёжнее в РФ).
- Проверка SHA-256/целостность zip — как сейчас.

## Этап 2. Backend: источник версий и скачивание ZPUI (internal/updater)

**Новый файл `internal/updater/setup.go`:**
- `CheckZPUISetupUpdate(current string) (latest, url string, err)` — листинг публичной папки Яндекса (клиент из `yandex_disk.go`), regex `^ZPUI-Setup-(\d+(\.[\d]+)*[a-z]?)-win32\.exe$`, максимум по `CompareVersions`. Установщик — **только Яндекс**.
- `DownloadZPUISetup(url, dest, progress)` — в `%TEMP%\ZPUI-Setup-<v>-win32.exe` с прогрессом; проверка SHA-256 по `checksums.sha256` из корня Диска (запись добавим в build.bat; расхождение = ошибка).
- Удалить: `ZPUIZipURLYandex`, `DownloadZPUIUpdate` (zip-схема, downloader.go), ветку selfupdate из `CheckAllComponents` и поле `Selfupdate` в `RemoteVersions`.

## Этап 3. Backend: internal/app

- **Удалить `update_checker.go` целиком** (`selfupdate.exe --check`, `startUpdatePanelPolling`, кэш `updCache`); в `app.go:318` убрать вызов.
- **`app_api_updates.go` — переписать:**
  - `DownloadZPUISetup()` — события `update:download {component:"ZPUI", percent, downloaded, total}` / `update:download-done {component, ok, path, version|error}`.
  - `DownloadZapretUpdate()` — `CheckForUpdates()` (2 источника) + `DownloadUpdateInfo` (с фолбэком на Яндекс) → `%TEMP%\zapret-update.zip`, те же события с `component:"zapret"`.
  - `ApplyZPUISetup()` — detached-помощник PowerShell `Wait-Process -Id <pid>; Start-Process '<setup>'` (существующий паттерн холодной замены) → `a.Quit()`; установщик наследует admin ZPUI — без UAC.
  - `ApplyZapretPackage()` — `saveBackupToDB()` + `SetZapretJustUpdated(true)` + `InstallFromZip` с колбэком → события `update:install {component:"zapret", step, percent}` / `update:install-done`.
  - `GetUpdateState()` — `{zpui:{available, latest, downloading, downloaded}, zapret:{...}}` для восстановления модалки; флаги ставятся стартовой проверкой (кэш, **без сети в цикле трея**).
- **`versions.go`:** `CheckZPUIUpdate`/`CheckVersionsPanel` → новый источник; zapret-панель — через двух-источниковый `CheckForUpdates`; удалить `PrepareZPUIUpdate`, `LaunchZPUIUpdater`, `OpenUpdateCenter`, ветки selfupdate; `CheckComponentUpdates` — только report.
- **`app.go`:** `checkUpdatesOnStartup` — ZPUI через новый чек, zapret — обновлённая проверка (флаги + `update:available` + toast с дедупом как сейчас); `HasUpdateAvailable` → чтение флага; новые `HasZapretUpdateAvailable()`, `StartTrayUpdate(component)` = `ShowWindow()` + `EventsEmit("update:open-modal", {component})`; в Shutdown убрать `taskkill /IM selfupdate.exe`.
- **`app_devices.go`:** удалить легаси `RunUpdateStream`.

## Этап 4. Трей (internal/tray/tray.go)

Вместо «Обновление доступно» — два пункта (скрыты по умолчанию, show/hide по флагам):
- «Обновление ZPUI — скачать» → `controller.StartTrayUpdate("ZPUI")`
- «Обновление запрета — скачать» → `controller.StartTrayUpdate("zapret")`

Controller: `HasUpdateAvailable/OpenUpdateCenter` → `HasZPUIUpdateAvailable/HasZapretUpdateAvailable/StartTrayUpdate`.

## Этап 5. Фронтенд (web/)

- **Новый `components/UpdateProgressModal.jsx`** (+ `hooks/useUpdateFlow.js`): состояния downloading → downloaded → installing → done/error по событиям. **Во время скачивания/установки модалку нельзя закрыть** (без крестика, Escape, клика по фону); после скачивания — кнопка «Установить» (закрытие разрешено). ZPUI: «Установить» = закрытие приложения и запуск установщика («Запускается установщик…»); zapret: ин-апп установка с прогрессом. В состоянии скачивания zapret виден источник (GitHub/Яндекс) и автоматический фолбэк.
- `App.jsx`: слушает `update:open-modal` (трей) и `update:available`; `UpdateAvailableModal` — кнопка «Обновить» вместо «Открыть Центр обновлений» → сразу открывает UpdateProgressModal и начинает скачивание.
- `SettingsPage.jsx`: секция обновлений переделана — карточки ZPUI/zapret (версии + «Скачать»/«Установить» через ту же модалку), report/tgproxy — только версии; «Центр обновлений» исчезает.
- `api.js`: маршруты `/api/updates/download` (→ DownloadZPUISetup), `/api/updates/apply` (→ ApplyZPUISetup), `/api/updates/download-zapret`, `/api/updates/apply-zapret`, `/api/updates/state`; удалить `/api/updates/launch-updater`, `/api/updates/prepare`, `/api/updates/downloaded`, `/api/update/stream`.
- Локали ru/en (новые строки, убрать «Центр»), `dev-mock.js` (без selfupdate), `DocsPage.jsx`, `useUpdateCheck.js` (проверка при старте + ретрай 15с остаётся).

## Этап 6. Сборка и дистрибуция

- **Удалить каталог `cmd/selfupdate/`** (Go, frontend/, wails.json).
- **`build.bat`:** убрать все шаги selfupdate; `versions.json` — без ключа selfupdate; **`checksums.sha256` дополнить записью `ZPUI-Setup-<v>-win32.exe`** (после makensis).
- **`versions.txt`:** убрать `selfupdate=`; бамп `zpui=1.7.0`.
- `installer/ZPUI.nsi` и `release.yml` — без изменений; Яндекс.Диск (вручную, как сейчас): корень = установщики + `checksums.sha256` + `components/report/`, `Zapret <v>/` как раньше; папки версий `<v>/` больше не создаются (root `versions.json` опционально — legacy для старых клиентов).
- GitHub-релизы Flowseal (zapret) продолжают публиковаться автором zapret — ничего готовить не нужно.

## Этап 7. Документация

- **`Документы/Бизнес-логика.md`** §8 — переписать: ZPUI (установщик, только Яндекс), zapret (два источника, новее выигрывает, фолбэк на Яндекс), кнопки трея, незакрываемая модалка, раскладка Диска; §11, §14.
- **`Документы/Архитектура.md`**: §3 (без cmd/selfupdate), §4.10 (новый флоу), §4.13 (трей), §5.6/§6.4 (события/потоки), §1.3.
- **`Документы/Функции.md`** §9, §12.2, §1, §5.5; **`Документы/Концепция.md`** §8, §9; **README.md**; **`Документы/GIT_WORKFLOW.md`**.
- **AGENTS.md**: §3, §5/§11 (модули → только report), §8/§9 (правила обновлений), §10 (ci.yml в репо отсутствует — актуализировать).
- **CHANGELOG.md** — новый блок `## v1.7.0` (с нуля).
- **`Документы/Задачи/`**: (а) новый документ «Рефакторинг структуры исходников.md» — полный аудит (таблицы файлов, 17 кандидатов на разбиение с целевыми файлами, дубли formatBytes/netsh/EventsOn, стратегия доменных пакетов) для следующего захода; (б) пометить устаревшими пункты 1.x–2.2 в задаче «Доработка Центра обновлений…».

## Порядок работ и проверки

1. `backup.bat` → правки этапов 1–6 → `go vet ./...`, `go test ./...`, `cd web && npm run build && npm test` → `build.bat`.
2. Коммиты — только по явной просьбе.
3. Живой E2E (реальный Яндекс + GitHub, перезапуск через установщик) — на машине пользователя после сборки; в сессии проверяется сборка и логика локально.

## Замечания
- Старые клиенты ≤1.6.x потеряют автообновление ZPUI (их zip-схема с папками версий уходит) — zapret у них продолжит обновляться. Осознанный разовый разрыв.
- Проверка обновлений — только при запуске (10с, тумблер `auto_update_check`, тосты с дедупом), без интервалов; флаги трея кэшируются.