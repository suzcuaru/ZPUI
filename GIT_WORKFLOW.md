# Git Workflow для ZPUI

## Версионирование

- **ZPUI core**: `var version = "1.0.0"` в `main.go:34`. В CI переопределяется ldflags: `-X main.version=$VER`.
- **Модули** (`cmd/wizard`, `cmd/autoselect`, `cmd/selfupdate`, `cmd/zapretupdate`): версия хардкодом в `var version = "..."` в каждом `main.go`. CI читает их из исходников регуляркой, ldflags НЕ переопределяет.
- **Теги**: `v<major>.<minor>.<patch>` (напр. `v1.1.1`).
- **Локальная сборка** (`build.bat`): автоинкремент патча из `version.txt`, не зависит от git-тегов.

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
2. Я (или ты) делаю коммит
3. Ставлю тег `vX.Y.Z` и пу́шу с тегами
4. **GitHub Actions** (`release.yml`) автоматически:
   - Собирает ZPUI core + 4 модуля для **64-bit** (`win64`)
   - Собирает ZPUI core + 4 модуля для **32-bit** (`win32`)
   - Собирает NSIS-установщик для обеих архитектур:
     - `ZPUI-Setup-X.Y.Z-win64.exe`
     - `ZPUI-Setup-X.Y.Z-win32.exe`
   - Собирает portable zip для обеих архитектур:
     - `zpui-win64.zip`
     - `zpui-win32.zip`
   - Создаёт GitHub Release с автогенерацией changelog
5. **В релизе только** установщики и zip-архивы. Отдельных exe-файлов для скачивания нет — все модули внутри установщика/zip.
6. **Selfupdate** автоматически определяет разрядность системы (64/32) и скачивает соответствующий zip.
7. На странице релиза можно скачать установщик под свою архитектуру.

Выпускать релиз можно только с CI. Локальная сборка через `build.bat` — для тестирования, не для публикации.

## CI рецепты для отладки

Если CI упал:

1. Зайти на https://github.com/suzcuaru/ZPUI/actions
2. Найти упавший workflow
3. Открыть его → посмотреть логи нужного шага
4. Пофиксить проблему → сделать новый коммит → запустить повторно (Re-run all jobs) или поставить новый тег
