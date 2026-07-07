package blockcheck

type LayerResult struct {
	Ok       bool    `json:"ok"`
	TimeMs   float64 `json:"time_ms"`
	Error    string  `json:"error,omitempty"`
	Status   int     `json:"status,omitempty"`
	StubPage bool    `json:"stub_page,omitempty"`
}

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

type FullReport struct {
	URL       string       `json:"URL"`
	Host      string       `json:"Host"`
	Direct    CheckResult  `json:"Direct"`
	Provider  ProviderInfo `json:"Provider"`
	Blocked   bool         `json:"Blocked"`
	BlockType string       `json:"BlockType"`
	InUserList bool        `json:"InUserList"`
	CheckedAt string       `json:"CheckedAt"`
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

var stubMarkers = []string{
	"доступ ограничен",
	"заблокировано",
	"роскомнадзор",
	"roskomnadzor",
	"сайт заблокирован",
	"реестр доменных имен",
	"доступ к информационному ресурсу ограничен",
}
