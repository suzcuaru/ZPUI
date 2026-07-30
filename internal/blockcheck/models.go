package blockcheck

// LayerResult — результат проверки одного слоя (TCP/TLS/HTTP).
//
// ВАЖНО про json-теги:
//   - BulkResult (массовая проверка для Dashboard) использует lowercase-теги
//     (ok, time_ms, error, status, stub_page) — фронтенд ждёт r.tcp.ok и т.д.
//   - CheckResult внутри FullReport (одиночная проверка в окне ResourceChecker)
//     отдаётся БЕЗ json-тегов — фронтенд ждёт report.Direct.TCP.Ok (PascalCase).
//
// Поэтому НИЖЕ теги явно прописаны в lowercase: так BulkResult.TCP сериализуется
// как {"ok":true,"time_ms":42,...}. А во FullReport.Direct.TCP тот же тип
// сериализуется так же — но фронтенд в ResourceChecker.jsx читает .TCP.Ok,
// что НЕ совпадает с json-тегом "ok". Чтобы это работало, в app-коде при
// отдаче FullReport поля пересобираются в PascalCase через map (см. app_api_system.go).
type LayerResult struct {
        Ok       bool    `json:"ok"`
        TimeMs   float64 `json:"time_ms"`
        Error    string  `json:"error,omitempty"`
        Status   int     `json:"status,omitempty"`
        StubPage bool    `json:"stub_page,omitempty"`
        // Header — HTTP-заголовок Server (для отличия CDN-ответов от блокировок).
        Header string `json:"header,omitempty"`
}

// CheckResult — результат одиночной проверки URL.
// Поля без json-тегов => в JSON будут PascalCase (Verdict, TCP, ...).
// Это нужно для совместимости с фронтендом ResourceChecker.jsx, который
// обращается к report.Direct.Verdict, report.Direct.TCP.Ok и т.д.
type CheckResult struct {
        URL  string `json:"url"`
        Host string `json:"host"`
        TCP  LayerResult `json:"tcp"`
        TLS  LayerResult `json:"tls"`
        HTTP LayerResult `json:"http"`

        Verdict    string   `json:"verdict"`
        Confidence string   `json:"confidence"`
        Notes      []string `json:"notes"`
}

type ProviderInfo struct {
        IP      string
        ISP     string
        City    string
        Country string
        Org     string
        ASN     string
}

// FullReport — итог одиночной проверки для фронтенда (окно ResourceChecker).
// Все поля без json-тегов => JSON-ключи будут PascalCase: URL, Host, Direct,
// Provider, Blocked, BlockType, InUserList, CheckedAt. Так и задумано —
// фронтенд ResourceChecker.jsx читает именно PascalCase.
type FullReport struct {
        URL        string
        Host       string
        Direct     CheckResult
        Provider   ProviderInfo
        Blocked    bool
        BlockType  string
        InUserList bool
        CheckedAt  string
}

const (
        VerdictOK       = "OK"
        VerdictTCPBlock = "TCP_BLOCK"
        VerdictTLSBlock = "TLS_BLOCK"
        VerdictDNSBlock = "DNS_BLOCK"
        VerdictHTTPStub = "HTTP_STUB"
        VerdictTimeout  = "TIMEOUT"
        VerdictDown     = "DOWN"
        VerdictUnknown  = "UNKNOWN"

        ConfHigh   = "HIGH"
        ConfMedium = "MEDIUM"
        ConfLow    = "LOW"
)

// stubMarkers — фразы, характерные ТОЛЬКО для заглушек РКН/TSPU.
// ВАЖНО: не добавлять сюда общие юридические фразы типа "в соответствии
// с федеральным законом" или "роскомнадзор" — они встречаются на нормальных
// сайтах (новостях, юридических страницах) и дают ложные срабатывания.
// Только однозначные маркеры заглушек.
var stubMarkers = []string{
        "доступ ограничен",
        "доступ к информационному ресурсу ограничен",
        "сайт заблокирован",
        "реестр доменных имен",
        "реестр нарушителей",
        "меры по ограничению доступа",
        "access denied by rosnadzor",
        "frontgroup of rkn",
        "сайт включен в единый реестр",
        "ограничение доступа к сайту",
}

// cdnServerMarkers — маркеры CDN-серверов в заголовке Server.
var cdnServerMarkers = []string{
        "cloudflare",
        "nginx",
        "akamai",
        "fastly",
        "amazon cloudfront",
        "cloudfront",
        "sucuri",
        "incapsula",
        "imunify360",
        "varnish",
        "microsoft-iis",
        "apache",
        "openresty",
        "tengine",
        "envoy",
}
