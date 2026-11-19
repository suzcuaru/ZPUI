package isp

import "strings"

// knownASNNames — канонические имена операторов по ASN. Разные geo-IP
// источники зовут одну и ту же сеть по-разному («Дом.ру» / «ДОМ.РУ» /
// «ER-Telecom» — один AS59713): без канона оператор распадался на несколько
// ключей, теряя историю автотестов и серверные рейтинги умного подбора.
var knownASNNames = map[string]string{
	"AS59713": "Дом.ру",
}

// applyASNAlias приводит имя и ключ оператора к канону, если ASN сети известен.
func applyASNAlias(op *Operator) {
	if op == nil || op.ASN == "" {
		return
	}
	canon, ok := knownASNNames[strings.ToUpper(strings.TrimSpace(op.ASN))]
	if !ok || canon == "" {
		return
	}
	op.ISP = canon
	op.Name = canon
	op.Key = MakeOperatorKey(canon)
}
