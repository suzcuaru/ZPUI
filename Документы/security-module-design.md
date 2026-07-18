# Модуль безопасности ZPUI — Дизайн-документ

## Обзор

Модуль безопасности (`internal/security`) обеспечивает проверку целостности файлов
и сканирование скачиваемых обновлений на наличие вредоносного ПО (стилеры, трояны, бэкдоры).
Все обновления (ZPUI, Zapret, модули) проходят через модуль безопасности **до** установки.

---

## Архитектура

```
┌─────────────────────────────────────────────────────┐
│                   Frontend (React)                   │
│  UpdateModal: changelog + кнопки Подтвердить/Отказ   │
│  SecurityError toast (не закрывается автоматически)  │
└──────────────────────┬──────────────────────────────┘
                       │ Wails Events
┌──────────────────────▼──────────────────────────────┐
│              internal/app (API layer)                │
│  CheckForUpdates → SecurityCheck → Modal → Apply     │
└──────────────────────┬──────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────┐
│            internal/security (новый пакет)            │
│                                                      │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────┐  │
│  │ Integrity   │  │ VirusScan    │  │ HashStore  │  │
│  │ Checker     │  │ (multi-engine│  │ (Yandex    │  │
│  │ (SHA-256,   │  │  via API)    │  │  Disk)     │  │
│  │  size, date)│  │              │  │            │  │
│  └─────────────┘  └──────────────┘  └────────────┘  │
└─────────────────────────────────────────────────────┘
```

---

## 1. Проверка целостности файлов (Integrity Checker)

### Принцип

Эталонные хеши хранятся на **Яндекс.Диске** в публичной папке. При каждом скачивании
обновления или при ручной проверке система:

1. Скачивает файл `manifest.json` с Яндекс.Диска
2. Вычисляет SHA-256, размер файла, дату модификации
3. Сравнивает с эталоном из манифеста
4. Если не совпадает — обновление отклоняется

### Структура `manifest.json` (на Яндекс.Диске)

```json
{
  "version": "1.0.0",
  "updated": "2025-07-13T12:00:00Z",
  "files": {
    "zpui-windows-amd64.zip": {
      "sha256": "a1b2c3d4e5f6...",
      "size": 15360000,
      "required": true
    },
    "zapret.zip": {
      "sha256": "f6e5d4c3b2a1...",
      "size": 8200000,
      "required": true
    },
    "winws.exe": {
      "sha256": "1a2b3c4d5e6f...",
      "size": 1024000,
      "required": true
    }
  }
}
```

### Хранение манифеста

- **URL**: `https://disk.yandex.ru/d/XXXXXXX/manifest.json` (публичная ссылка)
- Или через Yandex Disk API: `https://cloud-api.yandex.net/v1/disk/public/resources/download?public_key=XXX&path=/manifest.json`
- Кешируется локально на 5 минут (как existing `updater/cache.go`)

### Go интерфейс

```go
package security

type FileIntegrity struct {
    SHA256 string `json:"sha256"`
    Size   int64  `json:"size"`
}

type Manifest struct {
    Version string                   `json:"version"`
    Updated time.Time                `json:"updated"`
    Files   map[string]FileIntegrity `json:"files"`
}

type IntegrityChecker struct {
    manifestURL string
    cache       *Manifest
    cacheTime   time.Time
    log         *logger.Logger
}

// FetchManifest скачивает и кеширует манифест с Яндекс.Диска
func (c *IntegrityChecker) FetchManifest() (*Manifest, error)

// VerifyFile проверяет локальный файл по пути
func (c *IntegrityChecker) VerifyFile(path string, expectedName string) (*VerifyResult, error)

// VerifyDownloaded проверяет скачанный файл в temp-директории
func (c *IntegrityChecker) VerifyDownloaded(filePath string, expectedName string) (*VerifyResult, error)

type VerifyResult struct {
    OK       bool   `json:"ok"`
    SHA256   string `json:"sha256"`
    Size     int64  `json:"size"`
    Expected string `json:"expected"`
    Error    string `json:"error,omitempty"`
}
```

---

## 2. Сканер вирусов (VirusScan)

### Варианты реализации

#### Вариант A: VirusTotal API (рекомендуется)

- **Бесплатный API**: 4 запроса/минуту, 500 запросов/день
- Endpoint: `POST https://www.virustotal.com/api/v3/files`
- Загрузка файла (до 650МБ), затем получение отчёта
- Требуется API-ключ (хранится в config.json)

**Плюсы**: 70+ антивирусных движков, детектит стилеры/трояны
**Минусы**: Требует API-ключ, задержка ~30-60с на файл, лимиты запросов

#### Вариант B: Локальная проверка сигнатур (дополнение)

- Хранение базы известных хешей вредоносных файлов
- Проверка по базе `malware-hashes.json` (обновляется с Яндекс.Диска)
- Быстро, но покрывает только известные угрозы

#### Вариант C: YARA-правила (перспективное)

- Набор YARA-правил для детекции стилеров и модифицированных winws.exe
- Запускается локально, не требует API
- Требует поддержания актуальности правил

### Рекомендация: A + B

Использовать VirusTotal API для скачиваемых обновлений + локальную базу
хешей для быстрой pre-check проверки.

### Go интерфейс

```go
type VirusScanner struct {
    apiKey      string
    hashDB      map[string]bool  // known-bad hashes
    log         *logger.Logger
}

type ScanResult struct {
    Clean       bool              `json:"clean"`
    Scanned     bool              `json:"scanned"`
    Engines     map[string]string `json:"engines,omitempty"` // engine → verdict
    Detection   string            `json:"detection,omitempty"`
    Error       string            `json:"error,omitempty"`
}

// QuickCheckHash проверяет хеш по локальной базе (быстро)
func (s *VirusScanner) QuickCheckHash(sha256 string) bool

// ScanFile загружает файл в VirusTotal и ждёт результат
func (s *VirusScanner) ScanFile(filePath string) (*ScanResult, error)
```

---

## 3. Новый поток обновления

### Текущий поток (БЕЗ безопасности)

```
CheckForUpdates → ApplyUpdate → Download → Extract → Install
```

### Новый поток (С модулем безопасности)

```
CheckForUpdates
    │
    ▼
Download to temp (не в финальную директорию)
    │
    ▼
┌─────────────────────────────────┐
│     Security Module Check       │
│  1. Integrity (SHA-256, size)   │
│  2. VirusScan (VirusTotal API)  │
│  3. Local malware hash check    │
└──────────────┬──────────────────┘
               │
      ┌────────┴────────┐
      ▼                 ▼
   PASS               FAIL
      │                 │
      ▼                 ▼
 Show Update       Show Error
 Modal with:       Toast (не закрывается):
 - Changelog      - "Скачанный файл не прошёл
 - New features     проверку безопасности"
 - Version         - Причина (хеш не совпал /
                     вирус обнаружён)
      │                 │
   ┌──┴──┐              ▼
   ▼     ▼           Abort update
 Confirm Decline
   │
   ▼
 Apply update
 (extract → install)
```

### Фронтенд: UpdateModal

Модальное окно (только после успешной проверки безопасности):

```
┌──────────────────────────────────────────┐
│  Обновление Zapret  v0.9.9 → v1.0.0     │
│                                          │
│  ✅ Проверка безопасности пройдена        │
│     SHA-256: a1b2c3...                   │
│     VirusTotal: 0/70 обнаружений         │
│                                          │
│  ── Что нового ──                        │
│  • Улучшен обход DPI для РКН             │
│  • Добавлена поддержка QUIC              │
│  • Исправлены вылеты на Windows 11       │
│                                          │
│  Размер: 8.2 МБ                          │
│                                          │
│  [Отказаться]         [Подтвердить]      │
└──────────────────────────────────────────┘
```

### Реализация в коде

#### Backend: `internal/app/app_api_updates.go` (новый файл)

```go
type UpdateInfo struct {
    Component   string `json:"component"`    // "ZPUI" / "Zapret"
    CurrentVer  string `json:"current_ver"`
    NewVer      string `json:"new_ver"`
    Changelog   string `json:"changelog"`    // из GitHub release body
    DownloadURL string `json:"download_url"`
    Size        int64  `json:"size"`
}

// CheckUpdateWithSecurity — полная проверка обновления с security-чеком
func (a *App) CheckUpdateWithSecurity(component string) map[string]interface{} {
    // 1. Проверить наличие обновления
    // 2. Скачать во временную директорию
    // 3. Integrity check (SHA-256)
    // 4. Virus scan
    // 5. Вернуть результат + changelog
    // Если FAIL — вернуть error
    // Если PASS — вернуть update info для модалки
}

// ConfirmUpdate — применяется после подтверждения пользователем
func (a *App) ConfirmUpdate(component string, tempPath string) map[string]interface{} {
    // Извлечь и установить из уже проверенного temp-файла
}
```

#### Wails events

```go
// Прогресс скачивания и проверки
runtime.EventsEmit(ctx, "update:security-check", map[string]interface{}{
    "stage":    "downloading",  // downloading → integrity → scanning → done
    "percent":  45,
    "message":  "Скачивание обновления…",
})
```

#### Frontend

```jsx
// Новое модальное окно
function UpdateModal({ update, onConfirm, onDecline, securityPassed }) {
    return (
        <Modal>
            <h3>Обновление {update.component}</h3>
            {securityPassed && (
                <div className="security-badge">
                    <Shield /> Проверка безопасности пройдена
                </div>
            )}
            <div className="changelog">{update.changelog}</div>
            <button onClick={onDecline}>Отказаться</button>
            <button onClick={onConfirm}>Подтвердить обновление</button>
        </Modal>
    );
}
```

---

## 4. Конфигурация

### config.json — новые поля

```json
{
    "security": {
        "enabled": true,
        "virustotal_api_key": "",
        "integrity_manifest_url": "https://disk.yandex.ru/d/xxx",
        "strict_mode": true,
        "scan_timeout_sec": 120
    }
}
```

- `enabled`: вкл/выкл весь модуль
- `virustotal_api_key`: ключ API (пользователь вводит в настройках)
- `strict_mode`: если true — блокировать обновление при ошибке проверки; если false — показать предупреждение, но дать обновить
- `scan_timeout_sec`: таймаут сканирования

---

## 5. Интеграция с существующим кодом

### Точки интеграции

| Файл | Изменение |
|------|-----------|
| `internal/zapret/updater.go` | `PerformUpdate` → скачивать в temp → security check → только потом extract |
| `internal/updater/satellite.go` | `ReplaceModule` → security check перед заменой exe |
| `internal/app/versions.go` | `UpdateComponent` → обернуть в security flow |
| `internal/config/config.go` | Добавить `SecurityConfig` структуру |
| `web/src/api.js` | Новые маршруты: `/api/security/check-update`, `/api/security/confirm-update` |
| `web/src/pages/SettingsPage.jsx` | Секция «Безопасность»: вкл/выкл, API-ключ |
| `web/src/components/UpdateModal.jsx` | Новое модальное окно подтверждения |

### Очередность внедрения

1. **Фаза 1**: `internal/security/` — Integrity Checker (SHA-256 + Яндекс.Диск манифест)
2. **Фаза 2**: Обёртка обновлений — download → integrity check → modal → confirm → install
3. **Фаза 3**: VirusScanner — интеграция VirusTotal API
4. **Фаза 4**: UpdateModal UI — changelog, security badge, confirm/decline
5. **Фаза 5**: Настройки — секция безопасности в SettingsPage

---

## 6. Обработка ошибок

| Ситуация | Поведение |
|----------|----------|
| Файл не скачался | Toast: "Не удалось скачать обновление" + повтор через fallback URLs |
| Хеш не совпал | Toast (не закрывается): "Файл повреждён или подменён. Обновление отменено." |
| VirusTotal недоступен | Если strict_mode → отмена; иначе → предупреждение + возможность продолжить |
| VirusTotal обнаружил вирус | Toast: "Обнаружен вирус: [название]. Обновление заблокировано." |
| Манифест Яндекс.Диска недоступен | Если strict_mode → отмена; иначе → пропуск integrity check |
| Пользователь нажал «Отказаться» | Отмена, temp-файл удаляется, возврат к нормальной работе |

---

## 7. Дополнительные возможности

### Ручная проверка целостности
- Кнопка в настройках: «Проверить целостность файлов Zapret»
- Проверяет все essential files (`winws.exe`, `WinDivert.dll`, и т.д.) по манифесту
- Показывает результат: ✅ все файлы intact / ❌ какие файлы изменены

### Автоматическая проверка при запуске
- При запуске приложения — быстрая проверка целостности (только хеши, без VirusTotal)
- Если файлы изменены — toast с предупреждением

### Логирование
- Все проверки безопасности логируются в отдельный бакет `security`
- Результаты сохраняются в БД для аудита
