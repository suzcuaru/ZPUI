# Спека: живой балл активной стратегии (обновление из тактов проверки ресурсов) — 2.1.3

- Дата: 2026-11-20
- Статус: реализуется
- Запрос владельца: «проверка ресурсов — обновлять данные стратегии, на которой
  проводится проверка ресурсов, чтобы показывать реальные баллы активной стратегии».

## Проблема

Баллы стратегий (`StrategyScores` → `ScoreFromStoredResults`) читаются из
`operator_strategies.AutoTestResults` — JSON, который пишут ТОЛЬКО прогоны
автоподбора. Такты проверки ресурсов (каждые ~10 мин, `doResourceCheckAndSave`)
меряют доступность стандартных и ключевых ресурсов активной стратегии, но в балл
не попадают: панель стратегий / `list_strategies` / автосмена видят замер
последнего прогона (недельной давности), а не текущее состояние.

## Решение

После каждого такта проверки ресурсов обновлять в `AutoTestResults` активной
стратегии ОДНУ запись-маркер `Status:"live"` со свежими ok/total (стандартные) и
KeyOk/KeyTotal (ключевые), унаследовав ping/speed/base от последней реальной
записи прогона. `ScoreFromStoredResults` уже берёт последнюю запись, совпадающую
со стратегией, и считает балл по ней — запись `live` автоматически становится
источником показываемого балла, без правок ридера.

## Изменения (zpui-go-dev)

### internal/zapret/score.go
- `UpdateLiveScore(strategy string, pct, keyPct float64) error`:
  - прочитать AutoTestResults текущей строки стратегии из БД (путь как у
    `GetOperatorStrategies` — найди сеттер, вероятно
    `database.SetOperatorStrategyAutoTestResults` или аналог);
  - конвертировать pct/keyPct в Ok/Total/KeyOk/KeyTotal: Total=34 (стандартный
    пул такта — реальный total такта), KeyTotal=17 (реальный total ключевых из
    такта); если ключевых в такте нет — KeyOk/KeyTotal из последней записи;
  - найти последнюю запись с `Status=="live"` (strategy match): обновить её
    Ok/Total/KeyOk/KeyTotal in place (без роста JSON);
  - иначе: взять последнюю реальную запись этой стратегии (`pick(true)`-логика
    из ScoreFromStoredResults: пропуская Total==0 не-filtered). Если нет —
    выход без ошибки (нечего наследовать). Скопировать её поля
    (Strategy/Yandex*/Base*/Status→"live") с новыми Ok/Total/KeyOk/KeyTotal и
    APPEND в конец JSON;
  - сохранить JSON. Троттлинг: писать только если значения изменились.
- Хелпер `LiveScoreStrategyName()` не нужен — имя активной стратегии узнаёт app.

### internal/app (хук такта)
- В `doResourceCheckAndSave` (рядом с вызовом `autoSwitchTick(ok, total)`):
  новый шаг — если движок running и НЕ идёт подбор/автотест
  (гарды как в autoSwitchTick: `selectionRunning`, `IsAutoTestRunning`),
  взять из такта доступность стандартного пула (ok/total — уже есть у
  autoSwitchTick) и ключевых (найди в такте ok/total ключевых ресурсов —
  такт пишет в history по группам standard/key; если в контексте функции
  ключевых нет — вычисли из отчёта такта), и вызвать
  `a.zapret.UpdateLiveScore(cfg.GetCurrentStrategy(), pct, keyPct)`.
  Ошибка — только в лог (не ломает такт).
- Текущее имя стратегии: `cfg.GetCurrentStrategy()` (+".bat" — сверить с
  форматом имён в operator_strategies: там имя файла стратегии, см.
  ScoreFromStoredResults d.Strategy).

## Приёмка

1. `go vet ./...`, `go test ./...` — зелёные.
2. Юнит-тест: UpdateLiveScore — append при отсутствии live-записи (наследует
   Yandex*/Base*), update-in-place при повторных вызовах (JSON не растёт),
   нет реальной записи — no-op без ошибки; ScoreFromStoredResults после
   UpdateLiveScore возвращает свежую availability при сохранённом speed/ping.
3. Вручную (владелец): панель стратегий — балл активной стратегии меняется
   вслед за тактами проверки ресурсов.

## Не делаем

- Не трогаем логику эр (wipe прогона стирает и live-запись — норм).
- Не пишем live-балл НЕактивным стратегиям.
- Фронтенд не меняется (ридер уже берёт последнюю запись).
