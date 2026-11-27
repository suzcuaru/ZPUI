package app

// Авторизация и кабинет через конверт ZPUI Sync (POST /api/sync) — замена
// XML-протокола v4 (см. Документы/Задачи/ZPUI-Sync/02-Протокол.md §2, 06 §1).
// Имена биндингов и состав данных для фронта сохранены 1:1 с v4 (контракты
// web не меняются); ops: auth.login, auth.register.start/finish,
// auth.totp.confirm, auth.recovery.start/finish, captcha.new, account.*,
// device.*, subscriptions.list, promo.activate, banners.get,
// strategies.top.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/syncclient"
)

// boostySubscribeURL — оплата подписки только снаружи (кабинета на сайте
// больше нет); привязка платежа — по email аккаунта.
const boostySubscribeURL = "https://boosty.to/zpui"

// ---------- конверт: общий вызов ----------

// syncClient — клиент протокола для интерактивных вызовов (не цикл).
func (a *App) syncClient() *syncclient.Client {
	return syncclient.New(ServerURL, 20*time.Second)
}

// syncDev — паспорт устройства (тот же состав, что был у v4Device).
func (a *App) syncDev() syncclient.Dev {
	mac := ""
	if _, m := routeInfo(); m != "" {
		mac = m
	}
	host, _ := os.Hostname()
	return syncclient.Dev{
		ID:         a.cfg.GetSyncInstanceID(),
		Name:       host,
		AppVersion: a.version,
		OS:         a.osVersion(),
		MAC:        mac,
	}
}

// syncCall — один интерактивный вызов op через /api/sync; !ok → ошибка-код.
func (a *App) syncCall(op string, token string, payload map[string]any) (json.RawMessage, error) {
	resp, err := a.syncClient().Call(op, a.syncDev(), token, "", payload)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// syncCallMap — syncCall с разбором data в карту; ошибка конверта → errResp-код.
// out nil, если ответ пустой.
func (a *App) syncCallMap(op string, token string, payload map[string]any) (map[string]any, *syncclient.Error) {
	return a.syncCallMapToken(op, token, payload)
}

// syncCallMapToken — то же с явным токеном конверта (для auth.login.verify,
// где token — временный токен второго шага, а не сессия аккаунта).
func (a *App) syncCallMapToken(op string, token string, payload map[string]any) (map[string]any, *syncclient.Error) {
	data, err := a.syncCall(op, token, payload)
	if err != nil {
		if e, ok := syncclient.AsError(err); ok {
			return nil, e
		}
		return nil, &syncclient.Error{Code: netErrCode(err)}
	}
	if len(data) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, &syncclient.Error{Code: "bad_response"}
	}
	return m, nil
}

// syncErrResp — ошибка конверта в ответ биндингу (код как есть, плюс
// необязательный data-блок ошибки — например totp при totp_setup_required).
func syncErrResp(e *syncclient.Error) map[string]interface{} {
	out := errResp(e.Code)
	if e.Data != nil {
		var extra map[string]any
		if json.Unmarshal(e.Data, &extra) == nil {
			for k, v := range extra {
				if _, exists := out[k]; !exists {
					out[k] = v
				}
			}
		}
	}
	return out
}

// ---------- типы аккаунта (1:1 с v4) ----------

type syncSession struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

type syncAccount struct {
	Nick           string `json:"nick"`
	NickDisplay    string `json:"nick_display,omitempty"`
	Email          string `json:"email"`
	SubscriptionID string `json:"subscription_id,omitempty"`
	CreatedAt      string `json:"created_at"`
}

type syncSubscription struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Price          int    `json:"price"`
	Until          string `json:"until,omitempty"`
	SmartSelection bool   `json:"smart_selection"`
	DevicesLimit   int    `json:"devices_limit"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	SpeedTotalMbps int    `json:"speed_total_mbps"`
	BoostyURL      string `json:"boosty_url,omitempty"`
}

func sessionMap(s *syncSession) map[string]interface{} {
	if s == nil {
		return nil
	}
	return map[string]interface{}{"token": s.Token, "expires_at": s.ExpiresAt}
}

// accountMap — профиль для фронта: в аккаунте только id подписки,
// рядом карта subscription (название, цена, состав, срок).
func accountMap(acc *syncAccount, sub *syncSubscription) map[string]interface{} {
	if acc == nil {
		return nil
	}
	m := map[string]interface{}{
		"login":        acc.Nick,
		"nick":         acc.Nick,
		"nick_display": acc.NickDisplay,
		"email":        acc.Email,
		"created_at":   acc.CreatedAt,
	}
	if acc.SubscriptionID != "" {
		m["subscription_id"] = acc.SubscriptionID
	}
	m["subscription"] = subscriptionMap(sub)
	return m
}

// subscriptionMap — карта подписки; активность по сроку считается
// локально, чтобы закешированная подписка сама «погасла» по истечении.
func subscriptionMap(sub *syncSubscription) map[string]interface{} {
	if sub == nil {
		return map[string]interface{}{"active": false}
	}
	sm := map[string]interface{}{
		"active":           true,
		"id":               sub.ID,
		"name":             sub.Name,
		"price":            sub.Price,
		"smart_selection":  sub.SmartSelection,
		"devices_limit":    sub.DevicesLimit,
		"traffic_limit_gb": sub.TrafficLimitGB,
		"speed_total_mbps": sub.SpeedTotalMbps,
		"boosty_url":       sub.BoostyURL,
	}
	if sub.Until != "" {
		sm["ends_at"] = sub.Until
		if t, err := time.Parse(time.RFC3339, sub.Until); err == nil {
			sm["days_left"] = int(time.Until(t).Hours() / 24)
			if t.Before(time.Now()) {
				sm["active"] = false
			}
		}
	}
	return sm
}

// cachedProfileOf — снимок профиля и подписки для локального конфига.
func cachedProfileOf(acc *syncAccount, sub *syncSubscription) *config.CachedProfile {
	cp := &config.CachedProfile{
		Nick:        acc.Nick,
		NickDisplay: acc.NickDisplay,
		Email:       acc.Email,
		CreatedAt:   acc.CreatedAt,
	}
	if sub != nil {
		cp.Subscription = &config.CachedSubscription{
			ID: sub.ID, Name: sub.Name, Price: sub.Price, Until: sub.Until,
			SmartSelection: sub.SmartSelection, DevicesLimit: sub.DevicesLimit,
			TrafficLimitGB: sub.TrafficLimitGB, SpeedTotalMbps: sub.SpeedTotalMbps,
		}
	}
	return cp
}

// parseAccountBlock — {account, subscription} из data ответа.
func parseAccountBlock(data map[string]any) (*syncAccount, *syncSubscription) {
	if data == nil {
		return nil, nil
	}
	var acc *syncAccount
	var sub *syncSubscription
	if raw, err := json.Marshal(data["account"]); err == nil && len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &acc)
	}
	if raw, err := json.Marshal(data["subscription"]); err == nil && len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &sub)
	}
	return acc, sub
}

// saveSession — токен и почта аккаунта в конфиг; профиль с подпиской —
// в локальный кеш (офлайн-работа).
func (a *App) saveSession(acc *syncAccount, sub *syncSubscription, token string) {
	email := ""
	if acc != nil {
		email = acc.Email
	}
	_ = a.cfg.UpdateServerConfig(func(s *config.ServerConfig) {
		s.AccountToken = token
		if email != "" {
			s.AccountEmail = email
		}
		if acc != nil {
			s.Profile = cachedProfileOf(acc, sub)
		}
	})
}

// cacheProfile — обновляет локальный кеш профиля и подписки из свежего
// ответа сервера (acc nil — не трогаем).
func (a *App) cacheProfile(acc *syncAccount, sub *syncSubscription) {
	if acc == nil {
		return
	}
	cp := cachedProfileOf(acc, sub)
	_ = a.cfg.UpdateServerConfig(func(s *config.ServerConfig) { s.Profile = cp })
}

// cachedProfileMap — профиль из локального кеша, когда сервер недоступен;
// nil — кеша нет.
func (a *App) cachedProfileMap() map[string]interface{} {
	p := a.cfg.GetServerConfig().Profile
	if p == nil {
		return nil
	}
	var sub *syncSubscription
	if p.Subscription != nil {
		sub = &syncSubscription{
			ID: p.Subscription.ID, Name: p.Subscription.Name, Price: p.Subscription.Price,
			Until: p.Subscription.Until, SmartSelection: p.Subscription.SmartSelection,
			DevicesLimit: p.Subscription.DevicesLimit, TrafficLimitGB: p.Subscription.TrafficLimitGB,
			SpeedTotalMbps: p.Subscription.SpeedTotalMbps,
		}
	}
	return map[string]interface{}{
		"login":        p.Nick,
		"nick":         p.Nick,
		"nick_display": p.NickDisplay,
		"email":        p.Email,
		"created_at":   p.CreatedAt,
		"subscription": subscriptionMap(sub),
		"cached":       true,
	}
}

// logAuthEvent — факт авторизационного события в журнал действий
// (категория auth): попадает в секцию data.events. Секреты не пишутся.
func (a *App) logAuthEvent(action, account string) {
	_ = database.InsertActionLog(&database.ActionLog{
		Timestamp: time.Now(), Category: "auth", Action: action, Details: account,
	})
}

// ---------- ALTCHA ----------

type altchaChallenge struct {
	Algorithm string `json:"algorithm"`
	Salt      string `json:"salt"`
	Challenge string `json:"challenge"`
	Signature string `json:"signature"`
	Maxnumber int    `json:"maxnumber"`
}

type altchaSolution struct {
	Salt      string `json:"salt"`
	Number    int    `json:"number"`
	Challenge string `json:"challenge"`
	Signature string `json:"signature"`
}

// solveCaptcha — captcha.new + брутфорс proof-of-work (до ~100k sha256,
// доли секунды). Возвращает решение (base64) для payload-поля captcha.
func (a *App) solveCaptcha() (string, error) {
	data, err := a.syncCall("captcha.new", "", nil)
	if err != nil {
		return "", err
	}
	var d struct {
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(data, &d); err != nil || d.Challenge == "" {
		return "", fmt.Errorf("captcha challenge unavailable")
	}
	raw, err := base64.StdEncoding.DecodeString(d.Challenge)
	if err != nil {
		return "", err
	}
	var ch altchaChallenge
	if err := json.Unmarshal(raw, &ch); err != nil {
		return "", err
	}
	max := ch.Maxnumber
	if max <= 0 || max > 1000000 {
		max = 100000
	}
	for n := 0; n <= max; n++ {
		if sha256hex(ch.Salt+strconv.Itoa(n)) == ch.Challenge {
			sol, _ := json.Marshal(altchaSolution{
				Salt: ch.Salt, Number: n, Challenge: ch.Challenge, Signature: ch.Signature,
			})
			return base64.StdEncoding.EncodeToString(sol), nil
		}
	}
	return "", fmt.Errorf("captcha pow unsolved")
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ---------- биндинги для фронтенда ----------

// V4DeviceAccounts — история аккаунтов на этом устройстве (30 дней),
// для быстрого входа: список {nick, email, last_login_at}.
func (a *App) V4DeviceAccounts() map[string]interface{} {
	data, e := a.syncCallMap("device.accounts", "", nil)
	if e != nil {
		return syncErrResp(e)
	}
	accounts := []map[string]interface{}{}
	if data != nil {
		if raw, err := json.Marshal(data["accounts"]); err == nil {
			_ = json.Unmarshal(raw, &accounts)
		}
	}
	return okRespWithData(map[string]interface{}{"accounts": accounts})
}

// V4RegisterStart — шаг 1 регистрации: ник, почта, пароль и секретное
// слово восстановления. Ответ несёт секрет TOTP (base32 + otpauth:// URI).
func (a *App) V4RegisterStart(nick, email, password, keyword string) map[string]interface{} {
	cap, err := a.solveCaptcha()
	if err != nil {
		return errResp("bad_captcha")
	}
	data, e := a.syncCallMap("auth.register.start", "", map[string]any{
		"nick":     strings.ToLower(strings.TrimSpace(nick)),
		"email":    strings.ToLower(strings.TrimSpace(email)),
		"password": password,
		"keyword":  strings.TrimSpace(keyword),
		"captcha":  cap,
	})
	if e != nil {
		return syncErrResp(e)
	}
	out := okRespWithData(map[string]interface{}{})
	if data != nil {
		if v, ok := data["reg_token"].(string); ok && v != "" {
			out["reg_token"] = v
		} else if reg, ok := data["reg"].(map[string]any); ok {
			// Фолбэк: старые сборки сервера кладут токен заявки в reg.token.
			if v, ok := reg["token"].(string); ok && v != "" {
				out["reg_token"] = v
			}
		}
		if t, ok := data["totp"].(map[string]any); ok {
			out["totp_secret"], _ = t["secret"].(string)
			out["totp_uri"], _ = t["uri"].(string)
		}
	}
	return out
}

// V4RegisterFinish — шаг 2: код из аутентификатора; создаёт аккаунт,
// выдаёт сессию (токен сохраняется локально).
func (a *App) V4RegisterFinish(regToken, code string) map[string]interface{} {
	data, e := a.syncCallMap("auth.register.finish", "", map[string]any{
		"reg_token": strings.TrimSpace(regToken),
		"code":      strings.TrimSpace(code),
	})
	if e != nil {
		return syncErrResp(e)
	}
	sess, acc, sub := parseSessionBlock(data)
	if sess == nil {
		return errResp("bad_response")
	}
	a.saveSession(acc, sub, sess.Token)
	a.logAuthEvent("register", acc.Email)
	out := okRespWithData(sessionMap(sess))
	if raw, err := json.Marshal(data["registration_promo"]); err == nil && len(raw) > 0 && string(raw) != "null" {
		var promo map[string]any
		if json.Unmarshal(raw, &promo) == nil && promo["code"] != nil {
			out["registration_promo"] = promo
		}
	}
	return out
}

// parseSessionBlock — {session, account, subscription} из data ответа.
func parseSessionBlock(data map[string]any) (*syncSession, *syncAccount, *syncSubscription) {
	if data == nil {
		return nil, nil, nil
	}
	var sess *syncSession
	if raw, err := json.Marshal(data["session"]); err == nil && len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &sess)
	}
	acc, sub := parseAccountBlock(data)
	return sess, acc, sub
}

// V4Login — вход: логин/почта + пароль + код из аутентификатора (всегда).
// После recovery сервер ответит totp_setup_required и вернёт новый секрет —
// фронт показывает QR и завершает вход через V4TotpConfirm.
// Email-2FA (основной механизм): ответ two_fa=true несёт login_token —
// второй шаг через V4LoginVerify (op auth.login.verify).
func (a *App) V4Login(login, password, code string) map[string]interface{} {
	cap, err := a.solveCaptcha()
	if err != nil {
		return errResp("bad_captcha")
	}
	// Контракт v4: тело входа — объекты account/totp/captcha (плоские поля
	// оставлены для старых сборок сервера). Плоский payload без account
	// роняет старый opLogin в панику → клиент получает «internal».
	L := strings.ToLower(strings.TrimSpace(login))
	C := strings.TrimSpace(code)
	data, e := a.syncCallMap("auth.login", "", map[string]any{
		"login":    L,
		"password": password,
		"code":     C,
		"account":  map[string]any{"login": L, "password": password},
		"totp":     map[string]any{"code": C},
		"captcha":  cap,
	})
	if e != nil {
		out := syncErrResp(e)
		if e.Code == "totp_setup_required" && len(e.Data) > 0 {
			var extra struct {
				Totp struct {
					Secret string `json:"secret"`
					URI    string `json:"uri"`
				} `json:"totp"`
			}
			if json.Unmarshal(e.Data, &extra) == nil {
				out["totp_secret"] = extra.Totp.Secret
				out["totp_uri"] = extra.Totp.URI
			}
		}
		if isTwoFA(e.Code, e.Data) {
			return twoFAResp(e.Data, out)
		}
		return out
	}
	if isTwoFA("", mustMarshal(data)) {
		return twoFAResp(mustMarshal(data), okRespWithData(map[string]interface{}{}))
	}
	sess, acc, sub := parseSessionBlock(data)
	if sess == nil {
		return errResp("bad_response")
	}
	a.saveSession(acc, sub, sess.Token)
	a.logAuthEvent("login", acc.Nick)
	return okRespWithData(sessionMap(sess))
}

// isTwoFA — ответ auth.login требует email-подтверждение (второй шаг).
func isTwoFA(code string, data json.RawMessage) bool {
	if code == "two_fa_required" || code == "two_fa" {
		return true
	}
	if len(data) == 0 {
		return false
	}
	var d struct {
		TwoFA bool `json:"two_fa"`
	}
	return json.Unmarshal(data, &d) == nil && d.TwoFA
}

// twoFAResp — контракт фронта для второго шага: {two_fa, login_token}.
// login_token — временный токен «сессии регистрации» из ответа сервера.
func twoFAResp(data json.RawMessage, out map[string]interface{}) map[string]interface{} {
	if len(data) > 0 {
		var d struct {
			Token string `json:"token"`
		}
		if json.Unmarshal(data, &d) == nil && d.Token != "" {
			out["login_token"] = d.Token
		}
	}
	if _, ok := out["two_fa"]; !ok {
		out["two_fa"] = true
	}
	return out
}

func mustMarshal(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}

// V4LoginVerify — второй шаг email-2FA: код из письма + login_token из
// ответа auth.login (идёт в поле token конверта). Успех = authPayload
// с токеном сессии, сохраняется локально как обычный вход.
func (a *App) V4LoginVerify(email, code, loginToken string) map[string]interface{} {
	data, e := a.syncCallMapToken("auth.login.verify", strings.TrimSpace(loginToken), map[string]any{
		"email": strings.ToLower(strings.TrimSpace(email)),
		"code":  strings.TrimSpace(code),
	})
	if e != nil {
		return syncErrResp(e)
	}
	sess, acc, sub := parseSessionBlock(data)
	if sess == nil || sess.Token == "" {
		return errResp("bad_response")
	}
	a.saveSession(acc, sub, sess.Token)
	a.logAuthEvent("login", acc.Nick)
	return okRespWithData(sessionMap(sess))
}

// V4TotpConfirm — завершение входа после recovery: логин+пароль+код от
// нового секрета активирует аутентификатор и выдаёт сессию.
func (a *App) V4TotpConfirm(login, password, code string) map[string]interface{} {
	data, e := a.syncCallMap("auth.totp.confirm", "", map[string]any{
		"login":    strings.ToLower(strings.TrimSpace(login)),
		"password": password,
		"code":     strings.TrimSpace(code),
	})
	if e != nil {
		return syncErrResp(e)
	}
	sess, acc, sub := parseSessionBlock(data)
	if sess == nil {
		return errResp("bad_response")
	}
	a.saveSession(acc, sub, sess.Token)
	a.logAuthEvent("login", acc.Nick)
	return okRespWithData(sessionMap(sess))
}

// V4RecoveryStart — утерян аутентификатор: логин + ключевое слово →
// одноразовый токен сброса (15 минут).
func (a *App) V4RecoveryStart(login, keyword string) map[string]interface{} {
	data, e := a.syncCallMap("auth.recovery.start", "", map[string]any{
		"login":   strings.ToLower(strings.TrimSpace(login)),
		"keyword": strings.TrimSpace(keyword),
	})
	if e != nil {
		return syncErrResp(e)
	}
	out := okRespWithData(map[string]interface{}{})
	if data != nil {
		if v, ok := data["reset_token"].(string); ok {
			out["reset_token"] = v
		}
	}
	return out
}

// V4RecoveryFinish — новый пароль по токену сброса. Прочие сессии отзываются,
// аутентификатор аннулирован — следующий вход попросит привязку заново.
func (a *App) V4RecoveryFinish(resetToken, newPassword string) map[string]interface{} {
	_, e := a.syncCallMap("auth.recovery.finish", "", map[string]any{
		"reset_token":  strings.TrimSpace(resetToken),
		"new_password": newPassword,
	})
	if e != nil {
		return syncErrResp(e)
	}
	a.logAuthEvent("recovery_done", "")
	return okResp()
}

// V4AccountGet — профиль: ник, почта, подписка (id + блок данных и срока).
// Сервер недоступен — отвечаем из локального кеша: приложение продолжает
// работать на подписке офлайн; при возврате связи кеш обновится сам.
func (a *App) V4AccountGet() map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	data, e := a.syncCallMap("account.get", tok, nil)
	if e != nil {
		if e.Code == "auth_required" {
			_ = a.cfg.UpdateServerConfig(func(s *config.ServerConfig) { s.AccountToken = "" })
			return errResp("not_authorized")
		}
		if m := a.cachedProfileMap(); m != nil {
			return okRespWithData(m)
		}
		return syncErrResp(e)
	}
	acc, sub := parseAccountBlock(data)
	if acc == nil {
		return errResp("bad_response")
	}
	a.cacheProfile(acc, sub)
	return okRespWithData(accountMap(acc, sub))
}

// V4NickChange — смена ника, требует код из аутентификатора.
func (a *App) V4NickChange(nick, code string) map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	data, e := a.syncCallMap("account.nick.change", tok, map[string]any{
		"nick": strings.ToLower(strings.TrimSpace(nick)),
		"code": strings.TrimSpace(code),
	})
	if e != nil {
		return syncErrResp(e)
	}
	a.logAuthEvent("nick_change", strings.ToLower(strings.TrimSpace(nick)))
	acc, sub := parseAccountBlock(data)
	a.cacheProfile(acc, sub)
	return okRespWithData(map[string]interface{}{"account": accountMap(acc, sub)})
}

// V4EmailChange — смена почты: код аутентификатора + новый адрес.
func (a *App) V4EmailChange(email, code string) map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	data, e := a.syncCallMap("account.email.change", tok, map[string]any{
		"email": strings.ToLower(strings.TrimSpace(email)),
		"code":  strings.TrimSpace(code),
	})
	if e != nil {
		return syncErrResp(e)
	}
	a.logAuthEvent("email_change", strings.ToLower(strings.TrimSpace(email)))
	acc, sub := parseAccountBlock(data)
	if acc != nil && acc.Email != "" {
		_ = a.cfg.UpdateServerConfig(func(s *config.ServerConfig) {
			s.AccountEmail = acc.Email
		})
	}
	a.cacheProfile(acc, sub)
	return okRespWithData(map[string]interface{}{"account": accountMap(acc, sub)})
}

// V4PasswordChange — смена пароля (текущий + новый + код). Прочие сессии
// отзываются, текущая остаётся.
func (a *App) V4PasswordChange(oldPassword, newPassword, code string) map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	_, e := a.syncCallMap("account.password.change", tok, map[string]any{
		"old_password": oldPassword,
		"new_password": newPassword,
		"code":         strings.TrimSpace(code),
	})
	if e != nil {
		return syncErrResp(e)
	}
	return okResp()
}

// V4KeywordRotate — новое секретное слово (можно выбрать своё; пустое —
// сервер сгенерирует). Показывается один раз.
func (a *App) V4KeywordRotate(code, newKeyword string) map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	payload := map[string]any{"code": strings.TrimSpace(code)}
	if kw := strings.TrimSpace(newKeyword); kw != "" {
		payload["keyword"] = kw
	}
	data, e := a.syncCallMap("account.recovery.keyword.rotate", tok, payload)
	if e != nil {
		return syncErrResp(e)
	}
	out := okRespWithData(map[string]interface{}{})
	if data != nil {
		if v, ok := data["keyword"].(string); ok {
			out["keyword"] = v
		}
	}
	return out
}

// V4DeleteAccount — удаление аккаунта (код из аутентификатора); локальный
// токен очищается.
func (a *App) V4DeleteAccount(code string) map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	_, e := a.syncCallMap("account.delete", tok, map[string]any{
		"code": strings.TrimSpace(code),
	})
	if e != nil {
		return syncErrResp(e)
	}
	a.logAuthEvent("delete", "")
	_ = a.cfg.UpdateServerConfig(func(s *config.ServerConfig) {
		s.AccountToken = ""
		s.AccountEmail = ""
	})
	return okResp()
}

// V4Logout — выход: гасим сессию на сервере и локально.
func (a *App) V4Logout() map[string]interface{} {
	if tok := a.cfg.GetServerConfig().AccountToken; tok != "" {
		_, _ = a.syncCallMap("account.logout", tok, nil)
		a.logAuthEvent("logout", "")
	}
	if err := a.cfg.UpdateServerConfig(func(s *config.ServerConfig) {
		s.AccountToken = ""
		s.AccountEmail = ""
	}); err != nil {
		return errResp(err.Error())
	}
	return okResp()
}

// V4TOTPQR — QR-код (PNG data-URL) из otpauth:// URI для привязки
// аутентификатора (Яндекс.Ключ и т.п.).
func (a *App) V4TOTPQR(uri string) map[string]interface{} {
	uri = strings.TrimSpace(uri)
	if !strings.HasPrefix(uri, "otpauth://") {
		return errResp("bad_request")
	}
	png, err := qrcode.Encode(uri, qrcode.Medium, 220)
	if err != nil {
		return errResp(err.Error())
	}
	return okRespWithData(map[string]interface{}{
		"png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	})
}

// V4BannerGet — активный баннер (png/jpg/gif), anon; пусто в базе —
// error="no_banner": слот в панели скрыт.
func (a *App) V4BannerGet() map[string]interface{} {
	data, e := a.syncCallMap("banners.get", "", nil)
	if e != nil {
		return syncErrResp(e)
	}
	b, _ := data["banner"].(map[string]any)
	if b == nil {
		return errResp("no_banner")
	}
	return okRespWithData(map[string]interface{}{
		"id":        b["id"],
		"image":     b["image"],
		"link":      b["link"],
		"title":     b["title"],
		"placement": b["placement"],
		"kind":      b["kind"],
		"html":      b["html"],
		"text":      b["text"],
		"show_once": b["show_once"],
		"w":         b["w"],
		"h":         b["h"],
	})
}

// V4DevicesList — устройства аккаунта с живыми сессиями (кабинет); текущее
// устройство помечено current, его сессию завершают через выход, не здесь.
func (a *App) V4DevicesList() map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	data, e := a.syncCallMap("devices.list", tok, nil)
	if e != nil {
		return syncErrResp(e)
	}
	devices := []map[string]interface{}{}
	if data != nil {
		if raw, err := json.Marshal(data["devices"]); err == nil {
			_ = json.Unmarshal(raw, &devices)
		}
	}
	return okRespWithData(map[string]interface{}{"devices": devices})
}

// V4SessionKill — завершить сессию аккаунта на другом устройстве.
func (a *App) V4SessionKill(sessionID int64) map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	if sessionID <= 0 {
		return errResp("bad_request")
	}
	_, e := a.syncCallMap("device.session.kill", tok, map[string]any{"session_id": sessionID})
	if e != nil {
		return syncErrResp(e)
	}
	return okResp()
}

// V4DeviceUnbind — отвязать устройство ПК-приложения от своего аккаунта:
// сессии устройства завершаются (оно само выйдет по auth_required), запись в
// истории входов удаляется — устройство исчезает из кабинета. Текущее
// устройство так не отвязывают (для этого V4Logout).
func (a *App) V4DeviceUnbind(deviceID string) map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return errResp("bad_request")
	}
	_, e := a.syncCallMap("device.unbind", tok, map[string]any{"device_id": deviceID})
	if e != nil {
		return syncErrResp(e)
	}
	return okResp()
}

// V4PromoActivate — активация промокода: возвращает тариф, дни и новый срок.
func (a *App) V4PromoActivate(code string) map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return errResp("bad_request")
	}
	data, e := a.syncCallMap("promo.activate", tok, map[string]any{"code": code})
	if e != nil {
		return syncErrResp(e)
	}
	if data == nil || data["code"] == nil {
		return errResp("bad_response")
	}
	return okRespWithData(map[string]interface{}{
		"code":      data["code"],
		"days":      data["days"],
		"until":     data["until"],
		"sub_name":  data["sub_name"],
		"permanent": data["permanent"],
	})
}

// V4SubscriptionsList — подборка подписок для кабинета: не скрытые сервером,
// каждая с составом и своей ссылкой Boosty; помечена текущая (current).
func (a *App) V4SubscriptionsList() map[string]interface{} {
	tok := a.cfg.GetServerConfig().AccountToken
	if tok == "" {
		return errResp("not_authorized")
	}
	data, e := a.syncCallMap("subscriptions.list", tok, nil)
	if e != nil {
		return syncErrResp(e)
	}
	subs := []map[string]interface{}{}
	if data != nil {
		if raw, err := json.Marshal(data["subs"]); err == nil {
			_ = json.Unmarshal(raw, &subs)
		}
	}
	return okRespWithData(map[string]interface{}{"subs": subs})
}

// ---------- стратегии (умный подбор) ----------

// syncStrategiesTop — ответ strategies.top: топ стратегий оператора для
// умного подбора. Score — средняя доступность по сэмплам; Source — чьи
// сэмплы легли в агрегат: device или provider. Plan — персональный план
// подбора этого устройства (порядок задан сервером).
type syncStrategiesTop struct {
	Operator string                `json:"operator"`
	Source   string                `json:"source"`
	Items    []syncStrategyTopItem `json:"items"`
	Games    []syncStrategyGame    `json:"games"`
	Plan     []syncStrategyPlan    `json:"plan"`
}

type syncStrategyTopItem struct {
	Name       string  `json:"name"`
	Score      float64 `json:"score"`
	Av         float64 `json:"av"`
	PingMs     float64 `json:"ping"`
	SpeedMbps  float64 `json:"speed"`
	Samples    int64   `json:"samples"`
	LastSample string  `json:"last_sample"`
}

type syncStrategyGame struct {
	Name    string  `json:"name"`
	Score   float64 `json:"score"`
	PingMs  float64 `json:"ping"`
	Samples int64   `json:"samples"`
}

type syncStrategyPlan struct {
	Name string `json:"name"`
}

// strategiesTop — strategies.top: топ-5 стратегий и игровые баллы текущего
// оператора (данные умного подбора). Анонимно, как раньше.
func (a *App) strategiesTop(ispKey string) (*syncStrategiesTop, error) {
	data, e := a.syncCallMap("strategies.top", "", map[string]any{"isp_key": ispKey})
	if e != nil {
		return nil, fmt.Errorf("%s", e.Code)
	}
	if data == nil {
		return nil, fmt.Errorf("no strategies block")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var out syncStrategiesTop
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
