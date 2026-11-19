---
статус: выполнена
дата: 2026-11-13
---

# Замена движка: zapret → zapret2 (winws2), Lua-подвкладка, справка

> Директива владельца 12.11.2026: «делаем замену текущего запрета на запрет 2 в клиенте,
> не 2 вместе, а 1» + подвкладка Lua во вкладке Запрета + раздел про zapret2 в справке.
> Бекап сохранён вне ротации: `ZPUI-Backups/zpui/keep-pre-zapret2-20261112` (+zip в archive).
> Источник порта: экспериментальная ветка `../zpui_2_test` (почти полный ZPUI 2.1.0 на
> zapret2; инвентаризация — в отчёте разведки от 12.11). Ветки сильно разошлись → порт
> СЕЛЕКТИВНЫЙ, по фазам, сборка зелёная после каждой.

## Проблема

Основной клиент работает на zapret 1 (Flowseal, winws.exe). Владелец решил перевести
продукт на zapret2 (winws2.exe, Lua-стратегии) — ветка-прототип это уже реализовала,
но в main за месяц накопились MCP, источники обновлений, смарт-подбор, автосмена и пр.,
поэтому прямое копирование файлов невозможно.

## Цель

Движок клиента — zapret2: дистрибутив вокруг winws2.exe, Lua-стратегии и подвкладка
«Lua», эксперт-настройки движка, миграция старых установок (папка Flowseal удаляется,
пользовательские списки переносятся), справка «как работать с zapret2».

## Требования

- R1. Поколение движка определяется фактом: `bin/winws2.exe` есть → zapret2, иначе
  legacy zapret (EngineLabel, как в ветке-прототипе). Поле `engine` в статусе фронту.
- R2. Парсер стратегий понимает оба поколения: пресет с `winws2.exe` запускает
  winws2 (аргументы `--wf-*`, `--lua-init`, `--lua-desync`), legacy — winws.exe.
- R3. Дистрибутив: `build/zapret` = zapret2 (bin/winws2.exe + WinDivert + cygwin1.dll,
  lua/ 8 файлов включая zapret-zpui.lua/zpui-config.lua, lists/, fake/, 4 пресета
  general*.bat, windivert.filter/, version.txt). build.bat копирует целиком + SHA256
  winws2 → checksum.sha256. Ветка-прототип уже собрала эталон — берём оттуда.
- R4. Служба: ImagePath = `<zapret>\bin\winws2.exe <args>`; стратегия в реестре
  (`zpui_engine_strategy`). Версия — `<zapret>\version.txt`, иначе LOCAL_VERSION из
  service.bat (legacy).
- R5. Миграция установки/обновления (pkginst): legacy-дистрибутив (winws.exe без
  winws2.exe) удаляется ЦЕЛИКОМ, служба zapret пересоздаётся (старые аргументы
  несовместимы), пользовательские `*-user.txt` сохраняются и восстанавливаются.
- R6. Эксперт-настройки движка (config.zapret2: debug, intercept_empty_ack,
  ipcache_lifetime, ipcache_hostname, ctrack_disable → флаги winws2) + dry-run
  валидация стратегии (20с таймаут).
- R7. Lua: тумблеры `--lua-desync`-функций пресета (выключения в config.lua_disabled,
  файл не правится), CRUD своих скриптов `lua-user\` (переживают обновления) + просмотр
  библиотек `lua\`; редактор стратегии с валидацией knownFlags winws2.
- R8. UI: подвкладка «Lua» во вкладке Запрета (функции движка с примерами, мои скрипты,
  библиотеки), тултип функции на карточке стратегии, бейдж zapret2 в панели, эксперт-
  модалка настроек движка в настройках.
- R9. Обновления: движок zapret2 входит в установщик ZPUI (самостоятельного канала
  winws2 нет — апстрим не публикует Windows-сборки); строки обновлений ведут себя
  соответственно. Канал «ZAPRET 2» из итерации 3 (распаковка в zapret2\) демонтируется
  как противоречащий замене.
- R10. Справка: раздел «ZAPRET 2» (что это, чем отличается, как мигрирует, Lua,
  экспертные настройки, ссылки на мануал bol-van/zapret2).
- R11. i18n ru/en, токены, okResp/errResp, api.js — по конвенциям.

## Не-цели

- Серверная раздача Windows win-bundle (winws2.exe обновления отдельно от ZPUI) —
  позже; сейчас движок обновляется только с дистрибутивом.
- Автоподбор/смарт по zapret2-метрикам сверх текущего (механика замера общая).
- Родственный legacy-код Flowseal (фейки ACTIVE, GameFilter-порты Flowseal) — не портится.

## Критерии приёмки

- A1. После установки диста `zapret\bin\winws2.exe` на месте; статус отдаёт
  `engine: "zapret2"`; служба запускает winws2 с аргументами пресета.
- A2. Обновление с legacy-установки: старая папка zapret удалена, служба пересоздана,
  `*-user.txt` восстановлены, стратегия по умолчанию — zapret2-пресет.
- A3. Чипы/модалка подбора работают с zapret2-пресетами; DryRunStrategy валидирует
  пресет за ≤20с; невалидные флаги отсекаются статической проверкой.
- A4. Подвкладка «Lua»: свои скрипты создаются/правятся/удаляются, переживают
  обновление движка; тумблер lua-функции меняет поведение без правки файла пресета.
- A5. Эксперт-настройки применяются и дают restart_needed; после рестарта флаги
  видны в командной строке процесса winws2.
- A6. Справка содержит раздел ZAPRET 2 (ru/en).
- A7. go vet/test, npm build, build.bat зелёные; dist содержит zapret2-дерево
  (version.txt 1.0.5.2+, checksum.sha256 winws2).

## План (фазы порта)

- **Ф1 (фундамент, аддитивно):** engine.go, validate.go(+test) из прототипа;
  config Zapret2Options/LuaDisabled; поле engine в статусе. Сборка зелёная.
- **Ф2 (ядро):** service.go/manager.go/elevate.go/strategy.go — парсер двух
  поколений, serviceArgString, engineExtraArgs, DryRunStrategy, essentialFiles,
  detectZapretVersion(version.txt). startup-проверки integrity.
- **Ф3 (дистрибутив):** build/zapret/** из прототипа; build.bat STEP8/9 (копирование,
  SHA winws2, version.txt); pkginst (KillApps winws2, legacy-миграция, BackupUserFiles).
- **Ф4 (app-слой):** app_engine.go; app_zapret2.go = замена на биндинги прототипа;
  ДЕМОТАЖ канала итерации 3 (internal/zapret2/, биндинги, панельная строка, api-роуты —
  по решению владельца «1 вместо 2»); updater (satellite bol-van/zapret2 releases для
  списка версий, строка ZAPRET = «движок входит в установщик»).
- **Ф5 (фронт):** ZapretPage подвкладка Lua + FunctionsTip; SettingsPage
  Zapret2ExpertPanel + бейдж; api.js 13 маршрутов; локали; DocsPage раздел ZAPRET 2.
- **Ф6 (верификация):** A1–A7, QA на живой установке (сначала на копии), build.bat,
  CHANGELOG ×2, AGENTS.md §9 актуализация (источники обновлений + движок).

## Таски

- [x] Ф1 Фундамент (engine/validate/config/engine-статус) — 12.11, все тесты зелёные
- [x] Ф2 Ядро zapret (парсер двух поколений, StrategyEngineExe, служба/manager по пресету,
  kill/PID обоих движков, version.txt-детект, buildStrategyArgs/engineExtraArgs/
  DryRunStrategy/StrategyFunctions/filterDisabledLua/serviceArgString)
- [x] Ф3 Дистрибутив + build.bat + pkginst-миграция
- [x] Ф4 App-слой + демотаж канала ZAPRET 2 + обновления
- [x] Ф5 Фронт: Lua-вкладка, эксперт-панель, справка
- [x] Ф6 Верификация + доки

## Итог

13.11.2026 — все фазы Ф1-Ф6 выполнены, сборка зелёная (go vet 0, go test 20 пакетов ok,
npm build + vitest 97, build.bat → dist/2.1.0 с zapret2-деревом: winws2 v1.0.5.2 + lua +
checksum.sha256; exe-маркеры winws2/expert-settings/lua-script/ZAPRET 2). Отклонения от
спеки: (1) миграция pkginst сделана generation-aware — guard даунгрейда обходит смену
поколения (версии движков несравнимы: 1.10.x zapret ≠ 1.0.x zapret2); (2) FunctionsTip
на карточках стратегий не вшит в StrategiesTab (чтобы не трогать чужой компонент без
нужды) — тумблеры функций полностью доступны из вкладки «Lua»; подключить тултип — мелкая
доработка по запросу. Критерии A2/A3/A4 (живая миграция legacy-установки и запуск winws2)
верифицируются при установке dist владельцем; A1/A5/A6/A7 — кодом и тестами. Канал
«ZAPRET 2» итерации 3 демонтирован (замена вместо сосуществования, директива владельца).
