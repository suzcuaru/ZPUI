package blockcheck

type LayerResult struct {
	Ok       bool
	TimeMs   float64
	Error    string
	Detail   string
	IPs      []string
	CertCN   string
	Status   int
	StubPage bool
}

type CheckResult struct {
	URL     string
	Host    string
	HTTP    LayerResult

	Verdict    string
	Confidence string
	Notes      []string
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
	URL         string
	Host        string
	Direct      CheckResult
	WithBypass  *CheckResult
	Provider    ProviderInfo
	Blocked     bool
	BlockType   string
	BypassWorks bool
	InUserList  bool
	CheckedAt   string
}

const (
	VerdictOK       = "OK"
	VerdictTCPReset = "TCP_RESET"
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
	"access denied by rkn",
	"forbidden by law",
	"доступ к информационному ресурсу ограничен",
}
