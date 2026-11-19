package app

import "testing"

// Разбор строк журнала службы: дата-время, уровень, сообщение; мусор — INFO.
func TestParseServiceLogLine(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		wantTs    string
		wantLevel string
		wantMsg   string
	}{
		{
			name:      "info",
			line:      "2026-10-23 09:20:13.977 INFO service starting v1.0.0",
			wantTs:    "2026-10-23 09:20:13",
			wantLevel: "INFO",
			wantMsg:   "service starting v1.0.0",
		},
		{
			name:      "error",
			line:      "2026-10-23 09:21:00.001 ERROR restart-app: access denied",
			wantTs:    "2026-10-23 09:21:00",
			wantLevel: "ERROR",
			wantMsg:   "restart-app: access denied",
		},
		{
			name:      "debug",
			line:      "2026-10-23 09:22:00.500 DEBUG dns: skip",
			wantTs:    "2026-10-23 09:22:00",
			wantLevel: "DEBUG",
			wantMsg:   "dns: skip",
		},
		{
			name:      "garbage",
			line:      "что-то без формата",
			wantTs:    "",
			wantLevel: "INFO",
			wantMsg:   "что-то без формата",
		},
		{
			name:      "level without message",
			line:      "2026-10-23 09:23:00.123 INFO",
			wantTs:    "2026-10-23 09:23:00",
			wantLevel: "INFO",
			wantMsg:   "INFO",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := parseServiceLogLine(tc.line)
			if e.Timestamp != tc.wantTs {
				t.Fatalf("timestamp = %q, want %q", e.Timestamp, tc.wantTs)
			}
			if e.Level != tc.wantLevel {
				t.Fatalf("level = %q, want %q", e.Level, tc.wantLevel)
			}
			if e.Message != tc.wantMsg {
				t.Fatalf("message = %q, want %q", e.Message, tc.wantMsg)
			}
			if e.Table != "service" {
				t.Fatalf("table = %q, want service", e.Table)
			}
		})
	}
}
