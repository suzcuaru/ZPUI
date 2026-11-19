# Git Workflow для ZPUI

## Версионирование

- **ZPUI core**: версия в `versions.txt` (поле `zpui`), бампится вручную. При сборке инжектится ldflags: `-X main.version=$VER`.
- **report**: версия там же (поле `report`); `tgproxy` — константа в пакете (поле `tgproxy` в versions.txt, инжектится ldflags).
- **Теги**: `v<major>.<minor>.<patch>` (напр. `v1.7.0`).
- **Локальная сборка** (`build.bat`): читает `versions.txt` без автоинкремента, не зависит от git-тегов.

## Формат коммитов

В проекте принят стиль conventional commits. Сообщение на русском или английском — кратко и по делу.

```
feat:       новая фича
fix:        багфикс
chore:      служебное (билд, CI, AGENTS, докер)
refactor:   рефакторинг без изменения поведения
docs:       документация (README и т.п.)
```

Примеры:
```
feat: per-category debug mode, availability module, LogDrawer redesign
fix: selfupdate.exe survives parent kill via CREATE_BREAKAWAY_FROM_JOB
chore: bump version to 1.0.52
refactor: extract availability checker from GetResourceStatus
```

## Команды

### Посмотреть статус изменений
```
git status --short
```

### Просмотр лога
```
git log --oneline -5
git log --oneline -10
```

### Просмотр тегов
```
git tag --list
git tag --list 'v*'
```

### Сделать коммит
```
git add -A
git commit -m "feat: описание изменения"
```

### Создать тег и запустить сборку на GitHub
```
git tag v1.1.1
git push origin main --tags
```

### Удалить тег (если ошибся)
```
git tag -d v1.1.1                          # локально
git push origin --delete v1.1.1            # на GitHub
```

### Остановить сборку на GitHub (если запустилась случайно)
GitHub Actions → вкладка Actions → найти запущенный workflow → Cancel run.

## Процесс релиза

1. Вносишь изменения в код
2. Прогоняешь `build.bat` → артефакты в `dist/<версия>/` (ZPUI.exe, report.exe, установщик, zapret, versions.json, checksums.sha256)
3. `release.bat` копирует содержимое `dist/<версия>/` в корень, коммит `vX.Y.Z`, тег, push
4. **GitHub Actions** (`release.yml`) автоматически:
   - Проверяет наличие `ZPUI.exe` + `versions.json` в корне
   - Пакует portable zip (ZPUI.exe, versions.json, domains.txt, skip-resources.txt)
   - Создаёт GitHub Release с установщиком `ZPUI-Setup-X.Y.Z-win32.exe` + portable zip; описание берёт из заголовка `## vX.Y.Z` в CHANGELOG
5. На **Яндекс.Диск** заливаешь вручную (канал обновлений): установщик + `checksums.sha256` в корень папки; `components/report/` — при изменении модуля. Папки версий не создаются.
6. Обновления пользователей: приложение проверяет версии при запуске — zapret качает с Яндекс.Диска или GitHub, установщик ZPUI — только с Яндекс.Диска.

Выпускать релиз можно только через `release.bat` + CI. Локальная сборка через `build.bat` — для тестирования, не для публикации.

## CI рецепты для отладки

Если CI упал:

1. Зайти на https://github.com/suzcuaru/ZPUI/actions
2. Найти упавший workflow
3. Открыть его → посмотреть логи нужного шага
4. Пофиксить проблему → сделать новый коммит → запустить повторно (Re-run all jobs) или поставить новый тег
