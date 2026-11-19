package app

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
)

func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.json")
	cfg := config.Load(cfgPath, "zapret")
	logDir, err := os.MkdirTemp("", "zpui-test-logdb")
	if err != nil {
		t.Fatalf("logdb dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(logDir) })
	logMgr, err := logger.New(dir, logDir)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	t.Cleanup(func() { logMgr.Close() })
	return &App{cfg: cfg, log: logMgr}, cfgPath
}

// syncMockServer — перехватывает POST /api/sync; handler решает, что ответить.
// Полученный конверт возвращается через захваченную переменную.
func syncMockServer(t *testing.T, captured *string, handler func(body map[string]any) map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if captured != nil {
			*captured = string(b)
		}
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(handler(body))
	}))
}

func syncOK(data map[string]any) map[string]any {
	return map[string]any{"ok": true, "data": data, "next_after": 60}
}

func syncErr(code string, data map[string]any) map[string]any {
	e := map[string]any{"code": code}
	if data != nil {
		e["data"] = data
	}
	return map[string]any{"ok": false, "error": e}
}

// AccountMe (account.get через /api/sync): в аккаунте id подписки, блок
// subscription разворачивается в контракт UI и пишется в локальный кеш.
func TestAccountMeSync(t *testing.T) {
	a, _ := newTestApp(t)
	if err := a.cfg.UpdateServerConfig(func(s *config.ServerConfig) {
		s.AccountToken = "tok-1"
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	until := time.Now().UTC().Add(5 * 24 * time.Hour).Format(time.RFC3339)
	var gotReq string
	srv := syncMockServer(t, &gotReq, func(body map[string]any) map[string]any {
		if body["op"] != "account.get" {
			return syncErr("unknown_op", nil)
		}
		return syncOK(map[string]any{
			"account": map[string]any{
				"nick": "ivan", "email": "ivan@mail.ru", "subscription_id": "1",
				"created_at": "2026-10-01T00:00:00Z",
			},
			"subscription": map[string]any{
				"id": 1, "name": "Plus", "price": 150, "until": until,
				"smart_selection": true, "devices_limit": 5,
			},
		})
	})
	defer srv.Close()
	prev := ServerURL
	ServerURL = srv.URL
	defer func() { ServerURL = prev }()

	res := a.AccountMe()
	if isErrorResp(res) {
		t.Fatalf("AccountMe: %v", res["error"])
	}
	if res["login"] != "ivan" || res["email"] != "ivan@mail.ru" || res["subscription_id"] != "1" {
		t.Fatalf("profile: %v", res)
	}
	sub, ok := res["subscription"].(map[string]any)
	if !ok || sub["active"] != true || sub["name"] != "Plus" || sub["price"] != 150 || sub["devices_limit"] != 5 {
		t.Fatalf("subscription: %v", res["subscription"])
	}
	if sub["days_left"] == nil || sub["ends_at"] != until {
		t.Fatalf("subscription term: %v", sub)
	}
	// кеш профиля и подписки записан на диск
	sc := a.cfg.GetServerConfig()
	if sc.Profile == nil || sc.Profile.Nick != "ivan" || sc.Profile.Subscription == nil ||
		sc.Profile.Subscription.Name != "Plus" || sc.Profile.Subscription.ID != 1 {
		t.Fatalf("profile cache: %+v", sc.Profile)
	}
	// конверт должен нести токен и op account.get
	if !strings.Contains(gotReq, `"op":"account.get"`) || !strings.Contains(gotReq, `"token":"tok-1"`) {
		t.Fatalf("request envelope wrong: %s", gotReq)
	}
}

// AccountMe при недоступном сервере отвечает из локального кеша —
// приложение продолжает работать на подписке офлайн.
func TestAccountMeOfflineCache(t *testing.T) {
	a, _ := newTestApp(t)
	if err := a.cfg.UpdateServerConfig(func(s *config.ServerConfig) {
		s.AccountToken = "tok-1"
		s.Profile = &config.CachedProfile{
			Nick:  "ivan",
			Email: "ivan@mail.ru",
			Subscription: &config.CachedSubscription{
				ID: 1, Name: "Plus", Price: 150,
				Until: time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339),
			},
		}
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	prev := ServerURL
	ServerURL = "http://127.0.0.1:1" //connection refused
	defer func() { ServerURL = prev }()

	res := a.AccountMe()
	if isErrorResp(res) {
		t.Fatalf("offline AccountMe: %v", res["error"])
	}
	if res["cached"] != true || res["nick"] != "ivan" {
		t.Fatalf("offline profile: %v", res)
	}
	sub, ok := res["subscription"].(map[string]any)
	if !ok || sub["active"] != true || sub["name"] != "Plus" || sub["days_left"] == nil {
		t.Fatalf("offline subscription: %v", res["subscription"])
	}
}

// V4RegisterFinish: сессия из ответа сохраняется в конфиг (токен + email).
func TestV4RegisterFinishSavesSession(t *testing.T) {
	a, _ := newTestApp(t)

	var gotReq string
	srv := syncMockServer(t, &gotReq, func(body map[string]any) map[string]any {
		if body["op"] != "auth.register.finish" {
			return syncErr("unknown_op", nil)
		}
		return syncOK(map[string]any{
			"session": map[string]any{"token": "sess-9", "expires_at": "2026-11-04T00:00:00Z"},
			"account": map[string]any{"nick": "newuser", "email": "new@mail.ru"},
		})
	})
	defer srv.Close()
	prev := ServerURL
	ServerURL = srv.URL
	defer func() { ServerURL = prev }()

	res := a.V4RegisterFinish("reg-tok", "123456")
	if isErrorResp(res) {
		t.Fatalf("V4RegisterFinish: %v", res["error"])
	}
	if res["token"] != "sess-9" {
		t.Fatalf("token = %v", res["token"])
	}
	sc := a.cfg.GetServerConfig()
	if sc.AccountToken != "sess-9" || sc.AccountEmail != "new@mail.ru" {
		t.Fatalf("session not saved: %q/%q", sc.AccountToken, sc.AccountEmail)
	}
	if !strings.Contains(gotReq, `"op":"auth.register.finish"`) ||
		!strings.Contains(gotReq, `"reg_token":"reg-tok"`) ||
		!strings.Contains(gotReq, `"code":"123456"`) {
		t.Fatalf("request envelope wrong: %s", gotReq)
	}
}

// V4Login с totp_setup_required: код ошибки проксируется, секрет извлекается
// из data-блока ошибки конверта.
func TestV4LoginTotpSetupRequired(t *testing.T) {
	a, _ := newTestApp(t)
	srv := syncMockServer(t, nil, func(body map[string]any) map[string]any {
		if body["op"] == "captcha.new" {
			return syncOK(map[string]any{"challenge": solvableChallenge(t, "ab")})
		}
		if body["op"] == "auth.login" {
			return syncErr("totp_setup_required", map[string]any{
				"totp": map[string]any{"secret": "NEWSECRET", "uri": "otpauth://totp/ZPUI:x"},
			})
		}
		return syncErr("unknown_op", nil)
	})
	defer srv.Close()
	prev := ServerURL
	ServerURL = srv.URL
	defer func() { ServerURL = prev }()

	res := a.V4Login("ivan", "pw123456", "000000")
	if res["error"] != "totp_setup_required" {
		t.Fatalf("error = %v", res["error"])
	}
	if res["totp_secret"] != "NEWSECRET" {
		t.Fatalf("totp_secret = %v", res["totp_secret"])
	}
}

// solvableChallenge — ALTCHA-челлендж с известным решением (для моков).
func solvableChallenge(t *testing.T, salt string) string {
	t.Helper()
	ch := sha256hex(salt + strconv.Itoa(7))
	raw, _ := json.Marshal(altchaChallenge{
		Algorithm: "SHA-256", Salt: salt, Challenge: ch, Signature: "mock", Maxnumber: 100,
	})
	return base64.StdEncoding.EncodeToString(raw)
}

// V4LoginVerify (email-2FA): login_token идёт в token конверта, сессия
// из ответа сохраняется локально.
func TestV4LoginVerifySavesSession(t *testing.T) {
	a, _ := newTestApp(t)

	var gotReq string
	srv := syncMockServer(t, &gotReq, func(body map[string]any) map[string]any {
		if body["op"] != "auth.login.verify" {
			return syncErr("unknown_op", nil)
		}
		return syncOK(map[string]any{
			"session": map[string]any{"token": "sess-2fa", "expires_at": "2026-11-04T00:00:00Z"},
			"account": map[string]any{"nick": "ivan", "email": "ivan@mail.ru"},
		})
	})
	defer srv.Close()
	prev := ServerURL
	ServerURL = srv.URL
	defer func() { ServerURL = prev }()

	res := a.V4LoginVerify("ivan@mail.ru", "543210", "login-tok-1")
	if isErrorResp(res) {
		t.Fatalf("V4LoginVerify: %v", res["error"])
	}
	if res["token"] != "sess-2fa" {
		t.Fatalf("token = %v", res["token"])
	}
	sc := a.cfg.GetServerConfig()
	if sc.AccountToken != "sess-2fa" || sc.AccountEmail != "ivan@mail.ru" {
		t.Fatalf("session not saved: %q/%q", sc.AccountToken, sc.AccountEmail)
	}
	if !strings.Contains(gotReq, `"token":"login-tok-1"`) ||
		!strings.Contains(gotReq, `"code":"543210"`) ||
		!strings.Contains(gotReq, `"email":"ivan@mail.ru"`) {
		t.Fatalf("request envelope wrong: %s", gotReq)
	}
}

// V4Login с two_fa: первый шаг отдаёт login_token для второго шага.
func TestV4LoginTwoFA(t *testing.T) {
	a, _ := newTestApp(t)
	srv := syncMockServer(t, nil, func(body map[string]any) map[string]any {
		if body["op"] == "captcha.new" {
			return syncOK(map[string]any{"challenge": solvableChallenge(t, "cd")})
		}
		if body["op"] == "auth.login" {
			return syncOK(map[string]any{"two_fa": true, "token": "login-tok-2"})
		}
		return syncErr("unknown_op", nil)
	})
	defer srv.Close()
	prev := ServerURL
	ServerURL = srv.URL
	defer func() { ServerURL = prev }()

	res := a.V4Login("ivan", "pw123456", "")
	if res["two_fa"] != true || res["login_token"] != "login-tok-2" {
		t.Fatalf("two_fa response: %v", res)
	}
}

// V4Logout: сессия гасится на сервере, локальный токен очищается.
func TestV4LogoutClearsToken(t *testing.T) {
	a, _ := newTestApp(t)
	if err := a.cfg.UpdateServerConfig(func(s *config.ServerConfig) {
		s.AccountToken = "tok-x"
		s.AccountEmail = "x@mail.ru"
	}); err != nil {
		t.Fatal(err)
	}
	var gotReq string
	srv := syncMockServer(t, &gotReq, func(body map[string]any) map[string]any {
		return syncOK(nil)
	})
	defer srv.Close()
	prev := ServerURL
	ServerURL = srv.URL
	defer func() { ServerURL = prev }()

	if res := a.V4Logout(); isErrorResp(res) {
		t.Fatalf("V4Logout: %v", res["error"])
	}
	sc := a.cfg.GetServerConfig()
	if sc.AccountToken != "" || sc.AccountEmail != "" {
		t.Errorf("logout must clear token/email, got %q/%q", sc.AccountToken, sc.AccountEmail)
	}
	if !strings.Contains(gotReq, `"op":"account.logout"`) {
		t.Fatalf("logout envelope: %s", gotReq)
	}
}
