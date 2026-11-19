package app

// Туннель (Happ/VLESS) вырезан 11.11.2026 — из кабинета, подписок и сервера.
// Из этого файла остался только фидбек: «Сообщить о проблеме».

import (
	"strings"
)

// V4FeedbackSend — «Сообщить о проблеме»: тема + сообщение на сервер
// (feedback.send); логин, устройство и время проставляет сервер.
func (a *App) V4FeedbackSend(topic, message string) map[string]interface{} {
	_, e := a.syncCallMap("feedback.send", a.cfg.GetServerConfig().AccountToken,
		map[string]any{"topic": strings.TrimSpace(topic), "message": strings.TrimSpace(message)})
	if e != nil {
		return syncErrResp(e)
	}
	return okResp()
}
