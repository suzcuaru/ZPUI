package syncclient

// Лимит payload'а секции. Тело POST /api/sync режут дважды: nginx-гард на 6 МБ
// (deploy/core/guard/nginx.conf: client_max_body_size) и само ядро на 256 КБ
// (core/syncproto.go: syncMaxBody, MaxBytesReader) — фактически первым отвечает
// ядро. Лимит клиента — 192 КБ на payload: запас на конверт (dev/token/ticket)
// и JSON-экранирование. Превысившая лимит секция усекается (не молча) — иначе
// сервер отвечает 413/too_large, ack нет и секция уходит каждый тик бесконечно.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"unicode/utf8"
)

const maxSectionPayload = 192 << 10

// capSectionPayload — уложить payload в лимит транспорта. Шаг усечения:
// самое тяжёлое поле — список/строка режутся до половины с сохранением ХВОСТА
// (последние элементы — курсор дифф-секции и журнал ошибок держат свежее),
// map — рекурсивно в самое тяжёлое поле, неразделимое тяжёлое поле удаляется.
// Факт усечения — Warn с размерами.
func capSectionPayload(op string, payload map[string]any, log Logger) map[string]any {
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) <= maxSectionPayload {
		return payload
	}
	orig := len(raw)
	for i := 0; i < 48; i++ {
		key, _ := heaviestKey(payload)
		if key == "" {
			break
		}
		if nv, changed := shrinkValue(payload[key]); changed {
			payload[key] = nv
		}
		if raw, err := json.Marshal(payload); err == nil && len(raw) <= maxSectionPayload {
			if log != nil {
				log.Warn("sync", fmt.Sprintf("%s: payload %d -> %d B, усечено до лимита %d B",
					op, orig, len(raw), maxSectionPayload))
			}
			return payload
		}
	}
	// Практически недостижимо (каждый шаг делит самое тяжёлое поле надвое).
	if log != nil {
		log.Warn("sync", op+": payload "+strconv.Itoa(orig)+" B не уложился в лимит "+strconv.Itoa(maxSectionPayload)+" B после усечения")
	}
	return payload
}

// heaviestKey — поле с самым длинным JSON-представлением.
func heaviestKey(m map[string]any) (string, int) {
	key, max := "", 0
	for k, v := range m {
		raw, err := json.Marshal(v)
		if err != nil {
			continue
		}
		if len(raw) > max {
			key, max = k, len(raw)
		}
	}
	return key, max
}

// shrinkValue — один шаг усечения значения. Списки и строки делятся надвое
// с сохранением хвоста; map — усечение самого тяжёлого поля, а неразделимое
// тяжёлое (>1 КБ) поле удаляется целиком.
func shrinkValue(v any) (any, bool) {
	switch t := v.(type) {
	case []any:
		if len(t) < 2 {
			return v, false
		}
		return t[len(t)/2:], true
	case []map[string]any:
		if len(t) < 2 {
			return v, false
		}
		return t[len(t)/2:], true
	case []string:
		if len(t) < 2 {
			return v, false
		}
		return t[len(t)/2:], true
	case string:
		if len(t) < 2 {
			return v, false
		}
		return tailString(t), true
	case map[string]any:
		key, size := heaviestKey(t)
		if key == "" {
			return v, false
		}
		if nv, changed := shrinkValue(t[key]); changed {
			t[key] = nv
			return v, true
		}
		if size > 1024 {
			delete(t, key)
			return v, true
		}
		return v, false
	}
	return v, false
}

// tailString — задняя половина строки по границе руны.
func tailString(s string) string {
	n := len(s) / 2
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[n:]
}
