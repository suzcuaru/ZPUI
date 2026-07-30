package isp

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

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
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DialContext:       (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		DisableKeepAlives: true,
	},
}

func DetectOperator() (*Operator, error) {
	type provider struct {
		url   string
		parse func([]byte) *Operator
	}

	providers := []provider{
		{"https://ipinfo.io/json", parseIPInfo},
		{"https://ip-api.com/json/", parseIPAPI},
		{"https://myip.dev/json", parseMyIP},
	}

	for _, p := range providers {
		resp, err := client.Get(p.url)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()

		if op := p.parse(body); op != nil && op.ISP != "" {
			op.Name = cleanOperatorName(op.ISP)
			op.Key = NormalizeISP(op.Name)
			return op, nil
		}
	}

	return &Operator{Key: "unknown", Name: "Неизвестно"}, nil
}

func NormalizeISP(isp string) string {
	return strings.ToLower(strings.TrimSpace(isp))
}

func MakeOperatorKey(isp string) string {
	return strings.ToLower(strings.TrimSpace(isp))
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
		return isp
	}

	s = strings.Trim(s, "\"'«»()")

	prefixes := []string{"ООО", "ПАО", "АО", "ЗАО", "ОАО", "Ltd", "LLC", "JSC", "OAO", "Inc"}
	for {
		changed := false
		for _, p := range prefixes {
			if strings.HasPrefix(s, p+" ") {
				s = strings.TrimSpace(s[len(p):])
				s = strings.Trim(s, "\"'«»() ")
				changed = true
				break
			}
			if strings.HasSuffix(s, " "+p) {
				s = strings.TrimSpace(s[:len(s)-len(p)])
				s = strings.Trim(s, "\"'«»() ")
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

	s = strings.Trim(s, "\"'«»() ")
	if s == "" {
		return isp
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
