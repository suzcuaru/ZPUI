package app

// Общие помощники авторизации/кабинета. Сам протокол — ZPUI Sync
// (app_auth_sync.go): конверт POST /api/sync вместо XML v4.

import (
	"strings"

	"zpui/internal/executil"
)

// OpenTelegramLink открывает ссылку сразу в Telegram-приложении (tg://),
// а не в браузере: ссылки прокси/приглашения передаются как есть; http-ссылки
// t.me/<имя> — только для каналов/групп проекта (белый список).
func (a *App) OpenTelegramLink(link string) map[string]interface{} {
	s := strings.TrimSpace(link)
	switch {
	case strings.HasPrefix(s, "tg://") && len(s) > len("tg://"):
		// Родная tg://-ссылка (например tg://proxy?server=…): обработчик
		// протокола — само приложение Telegram.
	case strings.HasPrefix(s, "https://t.me/") || strings.HasPrefix(s, "http://t.me/"):
		name := s[strings.Index(s, "t.me/")+len("t.me/"):]
		if i := strings.IndexAny(name, "/?#"); i >= 0 {
			name = name[:i]
		}
		switch name {
		case "zpui_app", "zpui_chat":
		default:
			return errResp("unknown telegram domain")
		}
		s = "tg://resolve?domain=" + name
	default:
		return errResp("bad telegram link")
	}
	if err := executil.HiddenCmd("rundll32", "url.dll,FileProtocolHandler", s).Start(); err != nil {
		return errResp("open: " + err.Error())
	}
	return okResp()
}
