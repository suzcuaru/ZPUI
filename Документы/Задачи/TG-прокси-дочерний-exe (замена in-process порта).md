# TG-прокси как дочерний exe из родной кодовой базы tg-ws-proxy — план

> Дата: 31.10.2026. Статус: РЕАЛИЗОВАНО 31.10.2026 (см. чек-лист в конце).
> Предыдущие документы: «Telegram MTProto-прокси (интеграция tg-ws-proxy).md» (in-process Go-порт, реализован),
> синк 1.4.1 = апстрим v1.11.2 (30.10.2026).
> Репозиторий апстрима: https://github.com/Flowseal/tg-ws-proxy — клон-сиблинг `../tg-ws-proxy` (v1.11.3+1, окт 2026).

## 1. Зачем

In-process Go-порт (`internal/tgproxy`, 16 файлов, ~4.5 тыс. строк Go) — это ручной форк апстрима:
каждый релиз Flowseal = ручной синк 15 файлов (синк 1.11.2 занял отдельную сессию). Апстрим при этом
активно воюет с DPI: v1.11.3 (30.10.2026, через день после волны блокировок РКН) добавил в Fake TLS
зеркалирование key share клиента и профилирование маскировочного сайта — мы отстаём сразу после синка.

Решение: **перестать поддерживать форк**. Собирать дочерний exe из родного кода апстрима **без единой
правки их файлов**, ZPUI управляет им как внешним процессом (по образцу winws.exe). Обновление =
`git fetch` + `checkout <тег>` + пересборка — ноль конфликто-мержей.

## 2. Что показало дотошное изучение кода

- **GUI — строго опциональная надстройка.** `proxy/` (ядро, 15 файлов) не импортирует `ui/` вообще.
  Весь трей/настройки/автозапуск/обновления живут в `windows.py` + `ui/` + `utils/tray_common.py`.
- **Консольный режим — первоклассный сценарий**, задокументирован в `docs/EN/BuildFromSource.md`:
  entry `proxy/tg_ws_proxy.py` (в pyproject: `tg-ws-proxy = proxy.tg_ws_proxy:main`), все настройки
  через argparse: `--host --port --secret --dc-ip DC:IP×N --fake-tls-domain --cfproxy-domain×N
  --cfproxy-worker-domain×N --no-cfproxy --no-h2 --no-secure --proxy-protocol --force-test-dc
  --pool-size --buf-kb --log-file --log-max-mb --log-backups -v`. Так он и работает в Docker
  (в Dockerfile GUI-зависимости не ставятся вообще).
- **«Убрать интерфейс и трей» = не упаковывать их.** Вырезать код не нужно: PyInstaller-спека
  берёт только `proxy/` + `utils/logging_setup.py`; в exe не попадают customtkinter/pystray/Pillow/pyperclip/psutil.
- **Зависимости ядра:** `cryptography` (AES-CTR; есть ctypes-фолбэк на libcrypto в `proxy/_aes.py`),
  `certifi`, `httpx[http2]`, `h2`. Python ≥3.8; CI апстрима — 3.11, Docker — 3.12 (у нас 3.12.10 — ок).
- **Логи:** `--log-file` с честной ротацией (RotatingFileHandler, минимум 1 бэкап); каждая минута —
  строка `stats: total=.. active=.. up=.. down=..` (proxy/stats.py) — парсится для счётчиков UI.
  Домены в логах цензурируются (DomainCensorFilter).
- **Чистая остановка:** `stop_event` внутри asyncio; жёсткий kill снаружи тоже безопасен (состояния нет).
- **Лицензия MIT** — вклейка в проприетарный ZPUI легальна при сохранении копии LICENSE и атрибуции.
- **Активность апстрима:** 73 коммита за 3 мес; релизы eженедельно-двухнедельные; с 1.11.2 у них
  появились SignPath-подпись и опциональная подпись сборок, win7-сборки отдельным Python 3.8.
- **Тесты:** pytest-набор (`tests/`, ~17 файлов) — готовый регресс-гейт после подъёма версии.
- Их updater (GitHub releases) и автозапуск — в трей-слое, в наш exe не попадают: обновлением и
  автозапуском управляет ZPUI.

## 3. Целевая архитектура

```
ZPUI.exe (Go)
  └─ internal/tgproxy (переписанный менеджер-супервизор, поверхность та же)
       └─ дочерний процесс components/tgproxy/ZPUI-TgProxy.exe
            = PyInstaller-onefile, console-подсистема, вход proxy/tg_ws_proxy.py
            = код апстрима 1:1, ноль правок
```

- **Биндинги не меняются:** `app_api_tgproxy.go` остаётся как есть (`/api/tgproxy/status|config|link|
  start|stop|restart|regenerate-secret|open-link`); фронт (ProxyPage, MonitorPage, сайдбар) не трогаем.
  Меняется только начинка `Manager`: вместо горутин — spawn/kill процесса (по образцу `internal/zapret`).
- **Статусы:** running/stopped/error/restarting — по живости процесса + TCP-пробе порта
  (ready-гейт до 15 c после старта, как `svcAwaitReady` у запрета).
- **Статистика для UI:** парсер строки `stats:` из лог-файла (tail) → `active/total/bytes_in/bytes_out`
  в существующем формате ответа. Гранулярность 60 с (у in-process было мгновенно) — единственная
  видимая деградация; `ClientsByIP` фронт не использует (проверено: в `app_api_tgproxy.go` не вызывается).
- **Ссылки dd/ee:** строит ZPUI из config (как сейчас, `GetLinks`).
- **Watchdog:** падение процесса → рестарт, лимит 5 за 60 c → error (правила движка запрета).

### Маппинг конфига ZPUI → аргументы exe (1:1, все поля покрываются)

| config.json (TelegramProxyConfig) | аргумент ZPUI-TgProxy.exe |
|---|---|
| Port | `--port` |
| BindHost | `--host` |
| Secret | `--secret` (всегда передаём свой — иначе сгенерит случайный) |
| DCIP []string «2:ip» | `--dc-ip` ×N |
| FakeTLSDomain | `--fake-tls-domain` |
| ProxyProtocol | `--proxy-protocol` |
| FallbackCF (true) | `--no-cfproxy` при false |
| CFUserDomains | `--cfproxy-domain` ×N |
| CFWorkerDomains | `--cfproxy-worker-domain` ×N |
| NoSecure | `--no-secure` |
| — | `--log-file <data>/logs/tgproxy.log --log-max-mb 5 --log-backups 1` |
| verbose (новое, опц.) | `-v` |

Смена настроек → рестарт процесса (как сейчас Restart() «на лету»).

## 4. Сборка

- **Код апстрима живёт сиблингом воркспейса:** `../tg-ws-proxy` (уже склонирован). НЕ вендорим в репозиторий ZPUI.
- **Пин версии:** `versions.txt` → поле `tgproxy=` теперь означает версию апстрима (напр. `tgproxy=1.11.3`).
  `build.bat` жёстко проверяет `git -C ../tg-ws-proxy describe --tags` == пину (дрейф клона = ошибка сборки).
- **Спека PyInstaller — наша,** в репозитории ZPUI (`packaging/tgproxy/tgproxy.spec` + wrapper-скрипт
  из 2 строк `from proxy.tg_ws_proxy import main; main()` — наш файл, не их). `console=True`,
  `upx=False` (как у них), `runtime_tmpdir=None`, datas: certifi; hiddenimports: httpx, h2, cryptography-ciphers.
- **VERSIONINFO — свой:** ProductName «ZPUI», OriginalFilename «ZPUI-TgProxy.exe», версия = апстрим-тег.
  (Урок из `Документы/Антивирусы-и-ложные-срабатывания.md`: пустой VERSIONINFO = корм для AV.)
- **Шаг build.bat:** venv (или pip --user) → `pip install certifi "httpx[http2]==0.28.1" cryptography h2 pyinstaller`
  → pyinstaller → exe в `components/tgproxy/` → smoke-тест: запуск на случайном порту с `--log-file`,
  ждать «Listening on», TCP-проба, kill. Опционально `pytest` апстрима как гейт.
- **Архитектуры (сделано):** по exe на битность — x86-сборка (32-битный Python `py -3.12-32`,
  установлен per-user) идёт в дерево x86, x64 — в дерево x64; нет 32-битного Python → в x86
  кладётся x64-exe с WARN. VERSIONINFO вшивается всегда (FileVersion=апстрим, ProductName ZPUI,
  OriginalFilename, MIT-копирайт), no-UPX, иконка ZPUI.

## 5. Обновления компонента

- Цикл релиза как у `cmd/report`: новый тег апстрима → `git -C ../tg-ws-proxy fetch && checkout vX.Y.Z`
  → bump `tgproxy=` в versions.txt → build.bat → выкладка (сервер управления → Яндекс, по действующей цепочке).
- Обновление у пользователя — штатный канал обновлений ZPUI (обновлятор кладёт exe в components/tgproxy).
- Позже (опционально): автопроверка нового апстрим-тега через GitHub API в Центре обновлений
  («доступна новая версия моста»), т.к. EXE-обновлятор ZPUI уже умеет докатывать файлы компонентов.

## 6. Миграция с in-process порта

1. Релиз N: менеджер `internal/tgproxy` переключён на дочерний exe; протокольные файлы Go-порта
   (proxy.go, bridge.go, pool.go, ws.go, h2.go, faketls.go, workerpool.go, balancer.go, proto.go — ~4 тыс. строк)
   помечены устаревшими/удалены, менеджер-обёртка остаётся. UI без изменений.
2. Релиз N+1: удалить остатки Go-порта и его тесты; в отчётах/логах ничего не меняется (surface тот же).

## 7. Риски

| Риск | Митигация |
|---|---|
| AV/SmartScreen на unsigned PyInstaller-exe (главный) | свой VERSIONINFO+иконка, no-UPX, те же FP-формы вендоров; среднесрочно — общая код-подпись ZPUI. Запасной вариант: onedir вместо onefile (менее подозрителен, старт быстрее) |
| 32-битная сборка (для x86-ZPUI) | одиночный x86-exe (WOW64) — рекомендован; 32-битный Python 3.12 ставится при необходимости |
| Старт onefile ~1–2 c (распаковка в %TEMP%) | ready-гейт по TCP-пробе; для постоянного компонента некритично |
| Статистика с лагом 60 с | парс `stats:`-строки; при желании — парс INFO-строк подключений |
| Клон-сиблинг могут удалить/забыть | build.bat сам клонирует при отсутствии и валидирует пин тега |
| Апстрим сломает CLI-контракт | маловероятно (CLI = их Docker-контракт); pytest-гейт + smoke-тест ловят до релиза |

## 8. Чек-лист работ — ВЫПОЛНЕНО 31.10.2026

- [x] 8.1 Компонент: отдельная папка-проект `../ZPUI-TgProxy` (свой git): клон апстрима в
      `tg-ws-proxy/` + наш интеграционный модуль в `integration/` (entry.py, tgproxy.spec,
      upstream_version.py, make_version_info.py, build.bat, smoke_test.py, README). exe собран,
      smoke на 14431 — OK. Решено: НЕ packaging/ внутри ZPUI, а отдельный проект воркспейса.
- [x] 8.2 `build.bat`: версия tgproxy читается из `proxy/__init__.py` апстрима (versions.txt
      поле `tgproxy=` удалено); шаг сборки компонента в STEP 5 (fail = WARN, сборка продолжается);
      exe+version.txt+build_info.txt+LICENSE кладутся в `components/tgproxy/` обоих деревьев;
      checksum компонента. `pkginst`: KillApps гасит ZPUI-TgProxy.exe, payloadTree/extractTree
      распаковывают components/tgproxy (до этого — только wintun/service!).
- [x] 8.3 Go: `internal/tgproxy` = супервизор (manager.go: spawn/kill/watchdog 5 за 60с,
      ready-гейт TCP 20с, состояния stopped/starting/running/error) + args.go (маппинг конфига в
      CLI, тесты) + logwatch.go (инкрементальный хвост tgproxy.log: строка `stats:` → счётчики
      UI-формата, per-IP сессии по маркерам открытия/закрытия; файл открывается на тик — не
      ломает ротацию). Протокольные файлы Go-порта удалены (протокол живёт в апстриме);
      cmd/tgbench, cmd/tgbenchold удалены. Поверхность Manager/биндингов не изменилась.
- [x] 8.4 UI: блок «Дополнительные настройки» дополнен всеми возможностями компонента
      (H2-медиа, порт 80, свои CF-домены, пул, буфер, dc_ip, PROXY protocol, тестовые DC,
      verbose) с группами и i18n ru/en; старые функции без изменений (герой, ссылки, QR,
      secret, SOCKS). Журнал tgproxy.log в logdb НЕ заводился: лог компонента лежит в
      `<data>/logs/tgproxy.log` (ротация апстрима), просмотр — по мере надобности.
- [x] 8.5 Проверки: go vet/test зелёные, npm run build зелёный, полная сборка build.bat —
      см. финальный отчёт сессии. Живой тест на установленной машине — за юзером.
- [x] 8.6 CHANGELOG v2.0.7 (юзер-фейс + техническое), LICENSE-атрибуция едет в компоненте
      (LICENSE-tg-ws-proxy.txt в components/tgproxy/); backup-система: проект tgproxy добавлен
      в backup.ps1/backup-all.bat.
