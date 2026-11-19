# Спека: скилл zpui-mcp в поставке + кнопка «Расположение файла» в модалке MCP

- Дата: 2026-11-19
- Статус: реализуется
- Связанные: `2026-11-12-mcp-server.md` (MCP-сервер), AGENTS.md §14 (реестр скиллов)

## 1. Контекст

ZPUI поднимает локальный MCP-сервер (127.0.0.1:8930, Bearer-ключ) для ИИ-агентов.
Агенту (ZCode/Claude) нужен скилл-файл с инструкцией по инструментам — сейчас его нет:
агент работает вслепую. Решение: скилл `zpui-mcp` едет в поставке приложения, лежит
в папке установки (`<install>\skills\zpui-mcp\SKILL.md`), самовосстанавливается из
embedded-копии в exe и открывается из UI одной кнопкой.

## 2. Цели

1. SKILL.md в папке установленного приложения (появляется сразу после установки и
   живёт при обновлениях).
2. В модалке MCP: строка «Скилл для агентов» — описание назначения + кнопка
   «Расположение файла» (открывает папку с файлом, файл подсвечен).
3. Скилл установлен в реестрах агентов: `.agents/skills/zpui-mcp/` проекта и
   `~/.agents/skills/zpui-mcp/` глобально, строка в AGENTS.md §14.

## 3. Изменения

### Одиночный источник контента (дедлик)

Канонический файл — `.agents/skills/zpui-mcp/SKILL.md` (реестр скиллов проекта).
Embedded-копия для go:embed — `internal/mcpserver/skilldata/SKILL.md`.
Дрейф ловит юнит-тест `TestSkillFileInSync` (сравнивает оба файла; go test падает —
забыли синкнуть копию после правки).

### Go (zpui-go-dev)

- `internal/mcpserver/skill.go`:
  - `//go:embed skilldata/SKILL.md` + `SkillEmbedded() string`.
  - `MaterializeSkill(exeDir string) (path string, err error)` — пишет
    `<exeDir>\skills\zpui-mcp\SKILL.md`, если файла нет или содержимое отличается
    (обновление exe приносит новый текст). Каталог создаёт MkdirAll.
- `internal/app/app_mcp.go`:
  - `startMcp()` и `GetMcpInfo()` вызывают `MaterializeSkill(a.exeDir)` (ошибка — в лог,
    не ломает ответ; GetMcpInfo отдаёт путь даже при выключенном MCP).
  - `GetMcpInfo`: новые поля `skill_path` (полный путь к SKILL.md), `skill_dir`.
  - Новый биндинг `OpenMcpSkillFolder() map[string]interface{}`: materialize →
    `explorer.exe /select,"<path>"` (паттерн `app_api_logs.go:163`); ошибка — errResp.
- `internal/pkginst/pkginst.go`: в список удаления `RemoveInstallation` добавить `skills`
  (деинсталлятор убирает за собой).

### Сборка (zpui-go-dev)

- `build.bat`: копировать `.agents\skills\zpui-mcp\SKILL.md` → `%VERDIR%\x86\skills\zpui-mcp\SKILL.md`
  и `%VERDIR%\x64\...` (файл присутствует сразу после установки, до первого запуска exe;
  exe дальше поддерживает его сам). Секция рядом с README/CHANGELOG (строка ~391).

### Frontend (zpui-react-dev)

- `web/src/api.js`: маршруты `GET /api/mcp/info` уже есть; добавить
  `'/api/mcp/open-skill-folder': (app) => app.OpenMcpSkillFolder()`.
- `web/src/components/McpModal.jsx`: в секцию «Сервер» после строки URL — set-row
  «Скилл для агентов»: desc = назначение (инструкция для ИИ-агентов, лежит в папке
  программы), кнопка с иконкой FolderOpen → `api('POST','/api/mcp/open-skill-folder')`,
  тост при ошибке. Подпись кнопки — `mcp.skillOpenFolder`.
- `web/src/locales/{ru,en}.json`: ключи `mcp.skillTitle`, `mcp.skillDesc`,
  `mcp.skillOpenFolder`, `mcp.skillOpenFail`.

## 4. Не делаем

- Никаких новых MCP-инструментов (реестр 11 инструментов не меняется).
- Никакой загрузки скилла из сети — только embedded + локальный файл.
- Страницу настройки пути скилла — путь всегда `<install>\skills\zpui-mcp\`.

## 5. Приёмка

1. `go vet ./...`, `go test ./...` — зелёные; `TestSkillFileInSync` ловит расхождение копий.
2. `npm run build` — зелёный.
3. `build.bat` — в dist-деревьях `x86/x64` лежит `skills\zpui-mcp\SKILL.md`; Setup/Update
   пакеты содержат его.
4. В ZPUI.exe: маркеры embedded-текста (`skills\zpui-mcp` / уникальная строка скилла) и
   биндинга `OpenMcpSkillFolder` присутствуют в бинарнике.
5. Модалка MCP: строка скилла видна при выключенном сервере, кнопка открывает проводник
   с выделенным файлом.
6. Скилл лежит в `.agents/skills/zpui-mcp/`, `~/.agents/skills/zpui-mcp/`; AGENTS.md §14
   дополнен.
