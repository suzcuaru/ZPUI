package app

import (
	"encoding/json"
	"strings"
	"time"

	"zpui/internal/logdb"
)

// ============================================================
// BUG REPORT (расширенный баг-репорт)
// ============================================================
//
// Модалка ошибки («как в 1С»): пользователь видит код и текст ошибки и жмёт
// «Сообщить о проблеме». На сервер уходит ОБЫЧНОЕ обращение (feedback.send,
// тема bug — попадает в problem_reports и CRM), но в message лежит JSON
// со всеми деталями: что делал пользователь, код/текст ошибки, паспорт
// устройства, состояние движка, последние диагностики и хвост журнала
// ошибок. Логин, устройство и время приёма проставляет сервер — как у
// любого обращения.

// bugReportMsgLimit — лимит сервера на message (4000) с запасом на
// служебные поля конверта.
const bugReportMsgLimit = 3900

// bugReportErrTail — компактный хвост журнала: последние N ошибок.
func (a *App) bugReportErrTail(n int) []map[string]string {
	tail := make([]map[string]string, 0, n)
	rows, err := logdb.QueryRecentErrors(n)
	if err != nil {
		return tail
	}
	for i, r := range rows {
		if i >= n {
			break
		}
		msg := []rune(r.Message)
		if len(msg) > 200 {
			msg = msg[:200]
		}
		tail = append(tail, map[string]string{
			"ts": r.Timestamp, "level": r.Level, "src": r.Category, "msg": string(msg),
		})
	}
	return tail
}

func (a *App) BugReportSend(code, errText, userAction, comment string) map[string]interface{} {
	report := map[string]interface{}{
		"schema":      "zpui-bug-report/1",
		"time":        time.Now().Format("2006-01-02 15:04:05"),
		"code":        strings.TrimSpace(code),
		"error":       strings.TrimSpace(errText),
		"user_action": strings.TrimSpace(userAction),
		"comment":     strings.TrimSpace(comment),
		"engine": map[string]interface{}{
			"status":   string(a.zapret.GetStatus()),
			"strategy": a.zapret.GetCurrentStrategy(),
			"version":  a.zapret.GetVersion(),
		},
	}
	data, err := json.Marshal(report)
	if err != nil {
		return errResp("bug_report_build")
	}
	msg := string(data)
	// Диагностика и хвост журнала добавляются, пока влезает в лимит 4000:
	// сначала хвост ошибок (дешёвый и самый полезный), потом диагностика.
	if len([]rune(msg)) <= bugReportMsgLimit {
		full := report
		full["recent_errors"] = a.bugReportErrTail(10)
		if b, e := json.Marshal(full); e == nil && len([]rune(string(b))) <= 4000 {
			msg = string(b)
		}
	}
	if len([]rune(msg)) <= bugReportMsgLimit {
		full := report
		full["recent_errors"] = a.bugReportErrTail(5)
		full["diagnostics"] = a.lastDiagnosticsForReport()
		if b, e := json.Marshal(full); e == nil && len([]rune(string(b))) <= 4000 {
			msg = string(b)
		}
	}
	if r := []rune(msg); len(r) > 4000 {
		msg = string(r[:4000])
	}

	token := a.cfg.GetServerConfig().AccountToken
	if token == "" {
		return errResp("auth_required")
	}
	if _, e := a.syncCallMap("feedback.send", token, map[string]any{"topic": "bug", "message": msg}); e != nil {
		a.log.Warn("feedback", "bug report send failed: "+e.Code)
		return syncErrResp(e)
	}
	a.log.Info("feedback", "bug report sent (code: "+strings.TrimSpace(code)+")")
	return okResp()
}
