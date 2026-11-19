package isp

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Operator — сведения о текущем операторе связи. Частичный результат
// (есть IP/ASN/Org, но нет имени ISP) помечается Key=="" и Name=="":
// монитор не пишет его в БД как Unknown, а ретраит.
type Operator struct {
	Key  string
	Name string
	ISP  string
	ASN  string
	City string
	Org  string
	IP   string
}

type ipInfoResponse struct {
	IP      string `json:"ip"`
	Org     string `json:"org"`
	City    string `json:"city"`
	Country string `json:"country"`
	AS      string `json:"as"`
}

type ipAPIResponse struct {
	Query   string `json:"query"`
	ISP     string `json:"isp"`
	Org     string `json:"org"`
	City    string `json:"city"`
	Country string `json:"country"`
	AS      string `json:"as"`
}

type ipapiCoResponse struct {
	IP          string `json:"ip"`
	Org         string `json:"org"`
	ASN         string `json:"asn"`
	City        string `json:"city"`
	CountryName string `json:"country_name"`
}

type ipWhoIsResponse struct {
	IP         string `json:"ip"`
	City       string `json:"city"`
	Country    string `json:"country"`
	Success    bool   `json:"success"`
	Connection struct {
		ASN int64  `json:"asn"`
		Org string `json:"org"`
		ISP string `json:"isp"`
	} `json:"connection"`
}

type myIPResponse struct {
	IP      string `json:"ip"`
	Org     string `json:"org"`
	City    string `json:"city"`
	Country string `json:"country"`
	ASN     string `json:"asn"`
}

var client = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext:     (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		// Keep-alive включён: без него каждый опрос ISP (каждые 60с, всегда)
		// делал новый TCP+TLS-handshake — лишний сетевой шум и расход батареи.
	},
}

// whoisFallback — точка подмены в тестах.
var whoisFallback = whoisRIPE

type provider struct {
	name  string
	url   string
	parse func([]byte) *Operator
}

// RU-источники: ip-api и ipwho.is стабильны из РФ, ipapi.co и ipinfo.io —
// дополнительные. RIPE whois — фолбэк при полном молчании всех источников.
func providers() []provider {
	return []provider{
		{"ip-api.com", "http://ip-api.com/json/", parseIPAPI},
		{"ipwho.is", "https://ipwho.is/", parseIPWhoIs},
		{"ipapi.co", "https://ipapi.co/json/", parseIPAPICo},
		{"ipinfo.io", "https://ipinfo.io/json", parseIPInfo},
	}
}

// DetectOperator — параллельный опрос источников с ДЕТЕРМИНИРОВАННЫМ выбором:
// имя оператора всегда берётся у первичного источника (ip-api.com), остальные
// — только фолбэк, если первичный не ответил. Раньше побеждал первый
// ответивший, и один оператор получал разные имена/ключи у разных источников
// («PJSC "Vimpelcom"» против «PJSC Vimpelcom») — в автоподборе на сервере
// плодились дубли операторов. Пустые поля добираются из остальных ответов.
// Если имя никто не дал, но есть частичные данные — RIPE whois-фолбэк,
// а после него частичный оператор (Key=""). Unknown — только при полном нуле.
func DetectOperator() (*Operator, error) {
	primary := primarySourceName()
	type opResult struct {
		op   *Operator
		from string
	}
	ch := make(chan opResult, 4)
	var wg sync.WaitGroup
	for _, p := range providers() {
		wg.Add(1)
		go func(p provider) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, p.url, nil)
			if err != nil {
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			if op := p.parse(body); op != nil {
				ch <- opResult{op, p.name}
			}
		}(p)
	}

	var got []*Operator
	var winner *Operator // имя берём у него: primary, иначе первый с именем
	allDone := make(chan struct{})
	go func() { wg.Wait(); close(allDone) }()
	deadline := time.After(8 * time.Second)
collect:
	for {
		select {
		case r := <-ch:
			got = append(got, r.op)
			if r.op.ISP == "" {
				continue
			}
			if winner == nil {
				winner = r.op
			}
			if r.from == primary {
				// первичный источник ответил — дальше ждать нечего
				break collect
			}
		case <-allDone:
			break collect
		case <-deadline:
			break collect
		}
	}

	// имя оператора — только у победителя (детерминизм), остальные поля
	// добираются из всех ответов как раньше
	op := mergeOperators(got)
	if winner != nil {
		op.ISP = winner.ISP
	}
	if op.ISP == "" && op.IP != "" {
		// Фолбэк: RIPE whois по известному IP (хоть из частичных данных).
		if w := whoisFallback(op.IP); w != nil && w.ISP != "" {
			if op.ASN == "" {
				op.ASN = w.ASN
			}
			if op.Org == "" {
				op.Org = w.Org
			}
			if op.City == "" {
				op.City = w.City
			}
			op.ISP = w.ISP
		}
	}
	if op.ISP == "" {
		// Частичные данные без имени оператора — не Unknown: монитор
		// ретраит, не затирая последнего успеха.
		if op.Org != "" || op.ASN != "" || op.IP != "" {
			return op, nil
		}
		return &Operator{Key: "unknown", Name: "Неизвестно"}, nil
	}
	op.Name = cleanOperatorName(op.ISP)
	op.Key = NormalizeISP(op.Name)
	// Известный ASN — каноническое имя/ключ (иначе одна сеть живёт под
	// разными ключами у разных источников).
	applyASNAlias(op)
	return op, nil
}

// mergeOperators — первый непустой символ поля по порядку прихода ответов.
func mergeOperators(ops []*Operator) *Operator {
	base := &Operator{}
	for _, o := range ops {
		if base.IP == "" {
			base.IP = o.IP
		}
		if base.ISP == "" {
			base.ISP = o.ISP
		}
		if base.ASN == "" {
			base.ASN = o.ASN
		}
		if base.City == "" {
			base.City = o.City
		}
		if base.Org == "" {
			base.Org = o.Org
		}
	}
	if base.ISP == "" && base.Org != "" {
		base.ISP = base.Org
	}
	return base
}

// whoisRIPE — запрос к whois.ripe.net:43, парсинг netname/org-name/descr.
func whoisRIPE(ip string) *Operator {
	conn, err := net.DialTimeout("tcp", "whois.ripe.net:43", 4*time.Second)
	if err != nil {
		return nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(ip + "\r\n")); err != nil {
		return nil
	}
	reader := bufio.NewReader(io.LimitReader(conn, 64<<10))
	var netname, orgName, orgRef, descr, country string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "netname":
			if netname == "" {
				netname = value
			}
		case "org-name":
			if orgName == "" {
				orgName = value
			}
		case "org":
			if orgRef == "" {
				orgRef = value
			}
		case "descr":
			if descr == "" {
				descr = value
			}
		case "country":
			if country == "" {
				country = value
			}
		}
	}
	isp := orgName
	if isp == "" {
		isp = netname
	}
	if isp == "" {
		isp = descr
	}
	if isp == "" {
		return nil
	}
	return &Operator{ISP: isp, Org: orgRef, City: country}
}

// primarySourceName — источник, чьё имя оператора считается каноничным
// (см. DetectOperator): детерминизм имён важнее скорости ответа.
func primarySourceName() string {
	if ps := providers(); len(ps) > 0 {
		return ps[0].name
	}
	return "ip-api.com"
}

func NormalizeISP(isp string) string {
	// кавычки выкидываем и в ключе: «pjsc "vimpelcom"» и «pjsc vimpelcom» —
	// один оператор, разные источники
	isp = strings.ReplaceAll(isp, `"`, "")
	isp = strings.ReplaceAll(isp, "'", "")
	return strings.ToLower(strings.TrimSpace(isp))
}

func MakeOperatorKey(isp string) string {
	// ключ тоже без кавычек: разные источники пишут имя оператора по-разному
	// («PJSC "Vimpelcom"» / «PJSC Vimpelcom»), оператор должен быть один
	return NormalizeISP(isp)
}

func parseIPInfo(data []byte) *Operator {
	var r ipInfoResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil
	}
	isp, asn := splitOrg(r.Org)
	if r.AS != "" {
		asn = extractASN(r.AS)
	}
	return &Operator{
		IP:   r.IP,
		ISP:  isp,
		ASN:  asn,
		City: r.City,
		Org:  r.Org,
	}
}

func parseIPAPI(data []byte) *Operator {
	var r ipAPIResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil
	}
	isp := r.ISP
	if isp == "" {
		isp, _ = splitOrg(r.Org)
	}
	asn := extractASN(r.AS)
	return &Operator{
		IP:   r.Query,
		ISP:  isp,
		ASN:  asn,
		City: r.City,
		Org:  r.Org,
	}
}

func parseIPAPICo(data []byte) *Operator {
	var r ipapiCoResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil
	}
	isp, asn := splitOrg(r.Org)
	if r.ASN != "" {
		asn = extractASN(r.ASN)
	}
	return &Operator{
		IP:   r.IP,
		ISP:  isp,
		ASN:  asn,
		City: r.City,
		Org:  r.Org,
	}
}

func parseIPWhoIs(data []byte) *Operator {
	var r ipWhoIsResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil
	}
	if !r.Success && r.IP == "" {
		return nil
	}
	isp := r.Connection.ISP
	if isp == "" {
		isp = r.Connection.Org
	}
	asn := ""
	if r.Connection.ASN > 0 {
		asn = fmt.Sprintf("AS%d", r.Connection.ASN)
	}
	return &Operator{
		IP:   r.IP,
		ISP:  isp,
		ASN:  asn,
		City: r.City,
		Org:  r.Connection.Org,
	}
}

func parseMyIP(data []byte) *Operator {
	var r myIPResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil
	}
	isp, asn := splitOrg(r.Org)
	if r.ASN != "" {
		asn = r.ASN
	}
	return &Operator{
		IP:   r.IP,
		ISP:  isp,
		ASN:  asn,
		City: r.City,
		Org:  r.Org,
	}
}

// splitOrg splits an org string like "AS43038 МТС ООО \"Мобайл\""
// into ISP name ("МТС ООО \"Мобайл\"") and ASN ("AS43038").
func splitOrg(org string) (isp string, asn string) {
	org = strings.TrimSpace(org)
	if org == "" {
		return "", ""
	}
	parts := strings.SplitN(org, " ", 2)
	if len(parts) == 1 {
		return org, ""
	}
	if strings.HasPrefix(parts[0], "AS") || strings.HasPrefix(parts[0], "as") {
		return parts[1], parts[0]
	}
	return org, ""
}

// cleanOperatorName strips corporate prefixes/suffixes and returns
// the core operator name. E.g.:
//
//	"МТС ООО \"Мобайл\"" → "МТС"
//	"ПАО «Билайн»" → "Билайн"
//	"ООО \"Ростелеком\" филиал" → "Ростелеком"
//	"ROSTELECOM OAO" → "ROSTELECOM"
func cleanOperatorName(isp string) string {
	s := strings.TrimSpace(isp)
	if s == "" {
		return ""
	}

	s = stripWrappingQuotes(s)

	prefixes := []string{"ООО", "ПАО", "АО", "ЗАО", "ОАО", "Ltd", "LLC", "JSC", "OAO", "Inc"}
	for {
		changed := false
		for _, p := range prefixes {
			if strings.HasPrefix(s, p+" ") {
				s = strings.TrimSpace(s[len(p):])
				s = stripWrappingQuotes(s)
				changed = true
				break
			}
			if strings.HasSuffix(s, " "+p) {
				s = strings.TrimSpace(s[:len(s)-len(p)])
				s = stripWrappingQuotes(s)
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}

	suffixes := []string{" филиал", " branch", " OOO", " PAO", " JSC", " OAO", " Ltd", " LLC", " Inc"}
	for _, suf := range suffixes {
		if strings.HasSuffix(strings.ToLower(s), suf) {
			s = strings.TrimSpace(s[:len(s)-len(suf)])
			break
		}
	}

	s = strings.TrimSpace(s)
	if s == "" {
		return isp
	}
	return s
}

// stripWrappingQuotes снимает только балансную пару одноимённых обёрточных
// кавычек (ASCII " и '), не трогая кавычки/«» внутри строки.
func stripWrappingQuotes(s string) string {
	for len(s) >= 2 {
		c := s[0]
		if (c == '"' || c == '\'') && s[len(s)-1] == c {
			s = s[1 : len(s)-1]
			continue
		}
		break
	}
	return s
}

func extractASN(as string) string {
	if as == "" {
		return ""
	}
	parts := strings.SplitN(as, " ", 2)
	return parts[0]
}
