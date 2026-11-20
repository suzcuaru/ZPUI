# Спека: фикс «обновление не скачивается» (мгновенное «готово») + чистка мусора обновлятором — 2.1.2

- Дата: 2026-11-20
- Статус: реализуется
- Симптом (машина владельца, клиент 2.1.0): кнопка «Скачать» в панели «Обновления»
  мгновенно показывает «Готово — установлена версия …», ничего не скачивая и не
  устанавливая. Диагноз по живым логам/БД/серверу: ответ сервера здоровый
  (`/api/v1/updates/zpui` → 200, version 2.1.1, sha256, 725 байт — совпадает с
  guard-логами кликов), `CompareVersions("2.1.0","2.1.1")` корректен, в
  `action_logs` НЕТ ни одной строки категории updater за всю историю —
  `zpuiSourcePlans` молча возвращает `no_update`, и фронт превращает это в
  фальшивое «Готово — установлена версия» (openUpdateModal: `no_update → done`).

## Причины (все закрываются)

1. **Разрыв между проверкой и загрузкой.** Стартовая проверка
   (`runUpdateCheck`) ходит на сервер напрямую и кэширует только строку версии
   (`updFlags.zpuiLatest`). Загрузка (`zpuiSourcePlans`) заново опрашивает
   цепочку источников; любой сбой живого опроса уводит в фолбэк Яндекс, где
   лежат только версии ≤ 1.6.1 → «нет новее» → молчаливый `no_update`, хотя
   баннер «доступно 2.1.1» показан. `no_update` трактуется как финальный даже
   когда кэш проверки говорит об обратном.
2. **Нулевая наблюдаемость**: ни один вердикт источника не логируется.
3. **Лживый UI-текст**: `no_update` рендерится как «Готово — установлена
   версия …».

## Изменения

### Go (zpui-go-dev)

- `internal/app/update_state.go`: в `updateFlags` добавить кэш серверного
  манифеста доступного обновления: `zpuiSrvVersion/zpuiSrvURL/zpuiSrvSHA` +
  `setZPUIServerManifest(version, url, sha string)` (пустые значения очищают) и
  геттер. Кэш пишется в `runUpdateCheck` (app_updates_loop.go, ветка `needed`):
  при `srv != nil` — сохранить `srv.Version/srv.URL/srv.SHA256`, иначе — очистить.
- `internal/app/app_api_updates.go`:
  - `DownloadZPUISetup`: лог в самом начале `a.log.Info("updater",
    "manual download requested (source=X)")`.
  - `zpuiSourcePlans`: логировать каждый вердикт источника (server: reachable,
    версия манифеста, результат сравнения; yandex/github: версия/ошибка) —
    `a.log.Info/Warn("updater", ...)`.
  - После цикла: если `noUpdate` или `len(plans)==0` — проверить кэш
    (`zpuiSrvVersion`): если он новее `a.version` — собрать план из кэша
    (source "server", `downloadServerSetup` с реконструированным
    `serverZPUIManifest{Version, URL, SHA256, File: "ZPUI-Setup-"+ver}`) и
    залогировать «live source check failed/inconclusive — using cached server
    manifest from startup check». `no_update` возвращается ТОЛЬКО если кэш не
    новее текущей версии; иначе — `errResp` с перечнем причин (честная ошибка
    вместо фальшивого «готово»).
- **Чистка мусора обновлятором** (запрос владельца): новая функция в
  `internal/pkginst` `CleanupUpdateLeftovers(exeDir string)`: удаляет
  `%TEMP%\ZPUI-Setup-*.exe`, `%TEMP%\ZPUI-Update-*.exe`, `%TEMP%\ZPUI-*.zip.part`,
  временные CHANGELOG-файлы `%TEMP%\ZPUI-CHANGELOG-*` (если такие есть) и
  `ZPUI-Update-*.exe` в корне каталога установки (если появился). Вызовы:
  (1) в конце успешной установки/обновления в pkginst (после extractTree,
  когда приложение ещё не стартовало), (2) в ZPUI при старте отложенно
  (safeGo + sleep 2 мин, один раз) — чтобы чистить и после ручных Setup.
  Имена файлов запрета в %TEMP% не трогать, если шаблон не совпадает точно
  (посмотреть реальный шаблон в internal/zapret — downloadUpdate).

### Frontend (zpui-react-dev)

- `web/src/hooks/useUpdateFlow.js`: для `d.status === 'no_update'` ставить
  отдельный признак (`doneNoUpdate: true`) вместо подстановки version.
- Модалка обновления (компонент, рендерящий `update.doneBody`): при
  `doneNoUpdate` показывать «У вас последняя версия» (новый ключ
  `update.noUpdate` ru/en) вместо «Готово — установлена версия …».

## Приёмка

1. `go vet ./...`, `go test ./...`, `npm run build`, `npm test` — зелёные.
2. Новый юнит-тест: кэш-манифест строит план и перебивает `no_update`
   живых источников; `no_update` возвращается только при нек-новее-кэша.
3. Логи: каждый клик «Скачать» оставляет в updater_logs след запроса и вердиктов.
4. Сборка build.bat, маркеры в exe: «manual download requested», ключ
   `update.noUpdate` в бандле.
