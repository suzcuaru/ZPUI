package syncclient

// Клиент протокола ZPUI Sync: один эндпоинт POST /api/sync с конвертом
// {op,dev,token,payload,ticket} → {ok,data,next_after} / {ok:false,error:{code,...}}.
// Спека: Документы/Задачи/ZPUI-Sync/02-Протокол.md. Пакет не знает про сбор
// данных — только транспорт, джиттер и талонная логика (см. runner.go).

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

const Path = "/api/sync"

// Dev — паспорт устройства (тот же состав, что у v4/v5).
type Dev struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	AppVersion string `json:"app_version,omitempty"`
	OS         string `json:"os,omitempty"`
	MAC        string `json:"mac,omitempty"`
}

// Error — ошибка конверта. Коды: busy (выдан талон), auth_required, banned,
// unknown_op, bad_payload, too_large, internal. Data — необязательный блок
// данных при ошибке (например, totp_setup_required несёт новый секрет).
type Error struct {
	Code       string
	Msg        string
	Ticket     string
	RetryAfter int
	Data       json.RawMessage
}

func (e *Error) Error() string {
	if e.Msg != "" {
		return fmt.Sprintf("sync %s: %s", e.Code, e.Msg)
	}
	return "sync " + e.Code
}

// AsError разбирает ошибку в *Error; ok=false — обычный error (сеть, JSON).
func AsError(err error) (*Error, bool) {
	if e, ok := err.(*Error); ok {
		return e, true
	}
	return nil, false
}

// Resp — успешный ответ конверта.
type Resp struct {
	OK        bool            `json:"ok"`
	Data      json.RawMessage `json:"data"`
	NextAfter int             `json:"next_after"`
}

type respErrBody struct {
	Code       string          `json:"code"`
	Msg        string          `json:"msg,omitempty"`
	Ticket     string          `json:"ticket,omitempty"`
	RetryAfter int             `json:"retry_after,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

type respBody struct {
	OK        bool            `json:"ok"`
	Data      json.RawMessage `json:"data"`
	NextAfter int             `json:"next_after"`
	Error     *respErrBody    `json:"error"`
}

// Backend — транспорт для раннера (реализован Client'ом; в тестах — фейк).
type Backend interface {
	Call(op string, dev Dev, token, ticket string, payload any) (*Resp, error)
}

// Client — HTTP-клиент протокола. Один экземпляр на приложение.
type Client struct {
	Base string
	HTTP *http.Client
}

func New(base string, timeout time.Duration) *Client {
	return &Client{Base: strings.TrimRight(base, "/"), HTTP: &http.Client{Timeout: timeout}}
}

// Call — POST /api/sync. Ошибка конверта возвращается как *Error вместе с resp.
func (c *Client) Call(op string, dev Dev, token, ticket string, payload any) (*Resp, error) {
	env := map[string]any{"op": op}
	if dev.ID != "" {
		env["dev"] = dev
	}
	if token != "" {
		env["token"] = token
	}
	if payload != nil {
		env["payload"] = payload
	}
	if ticket != "" {
		env["ticket"] = ticket
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.Base+Path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	body = gunzipIfMarked(body)
	var out respBody
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("bad json response (http %d)", resp.StatusCode)
	}
	r := &Resp{OK: out.OK, Data: out.Data, NextAfter: out.NextAfter}
	if !out.OK {
		e := &Error{Code: "unknown"}
		if out.Error != nil {
			e.Code = out.Error.Code
			e.Msg = out.Error.Msg
			e.Ticket = out.Error.Ticket
			e.RetryAfter = out.Error.RetryAfter
			e.Data = out.Error.Data
		}
		return r, e
	}
	return r, nil
}

// gunzipIfMarked — прокси на пути могут отдавать gzip-тело без заголовка
// Content-Encoding: транспорт Go такие ответы не распаковывает. JSON не
// начинается с байтов 1f 8b, так что магия однозначна.
func gunzipIfMarked(raw []byte) []byte {
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		return raw
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return raw
	}
	out, err := io.ReadAll(io.LimitReader(zr, 32<<20))
	if err != nil {
		return raw
	}
	return out
}

// Jitter возвращает base с равномерным сдвигом ±frac (0.2 = ±20%).
// randFn — источник случайности; nil — math/rand.
func Jitter(base time.Duration, frac float64, randFn func() float64) time.Duration {
	if base <= 0 {
		return base
	}
	if frac <= 0 {
		return base
	}
	var r float64
	if randFn != nil {
		r = randFn()
	} else {
		r = rand.Float64()
	}
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	shift := float64(base) * frac * (2*r - 1)
	d := time.Duration(float64(base) + shift)
	if d < time.Second {
		d = time.Second
	}
	return d
}
