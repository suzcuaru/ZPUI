package zsvc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseIpcCommand(t *testing.T) {
	tests := []struct {
		line string
		want ipcCommand
	}{
		{"ping", ipcCommand{Name: "ping", Arg: ""}},
		{"  ping  ", ipcCommand{Name: "ping", Arg: ""}},
		{`app-hello {"pid":5,"version":"2.0.5","hidden":true,"autostart":true}`, ipcCommand{Name: "app-hello", Arg: `{"pid":5,"version":"2.0.5","hidden":true,"autostart":true}`}},
		{"app-exiting", ipcCommand{Name: "app-exiting", Arg: ""}},
		{`app-updating {"ttl_sec":300}`, ipcCommand{Name: "app-updating", Arg: `{"ttl_sec":300}`}},
		{"PING", ipcCommand{Name: "ping", Arg: ""}},
		{"", ipcCommand{Name: "", Arg: ""}},
		{"   ", ipcCommand{Name: "", Arg: ""}},
	}
	for _, tt := range tests {
		got := parseIpcCommand(tt.line)
		if got.Name != tt.want.Name || got.Arg != tt.want.Arg {
			t.Errorf("parseIpcCommand(%q) = %+v, want %+v", tt.line, got, tt.want)
		}
	}
}

func TestParseHelloArg(t *testing.T) {
	pid, version, hidden, autostart, err := parseHelloArg(`{"pid":1234,"version":"2.0.5","hidden":true,"autostart":true}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != 1234 || version != "2.0.5" || !hidden || !autostart {
		t.Errorf("got pid=%d version=%q hidden=%v autostart=%v", pid, version, hidden, autostart)
	}
	if _, _, _, _, err := parseHelloArg(`{`); err == nil {
		t.Errorf("malformed JSON must fail")
	}
	if _, _, _, _, err := parseHelloArg(``); err == nil {
		t.Errorf("empty arg must fail")
	}
}

func TestParseUpdatingArg(t *testing.T) {
	d, err := parseUpdatingArg(`{"ttl_sec":300}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 300*time.Second {
		t.Errorf("want 300s, got %v", d)
	}
	if _, err := parseUpdatingArg(`{"ttl_sec":-1}`); err == nil {
		t.Errorf("negative ttl must fail")
	}
	if _, err := parseUpdatingArg(`x`); err == nil {
		t.Errorf("malformed arg must fail")
	}
}

func TestParseRestartArg(t *testing.T) {
	hidden, err := parseRestartArg(`{"hidden":true}`)
	if err != nil || !hidden {
		t.Errorf("got hidden=%v err=%v, want true/nil", hidden, err)
	}
	hidden, err = parseRestartArg("")
	if err != nil || hidden {
		t.Errorf("empty arg: hidden=%v err=%v, want false/nil (default)", hidden, err)
	}
	if _, err := parseRestartArg(`zzz`); err == nil {
		t.Errorf("malformed arg must fail")
	}
}

func TestResponses(t *testing.T) {
	check := func(t *testing.T, s string) map[string]interface{} {
		t.Helper()
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &m); err != nil {
			t.Fatalf("invalid JSON %q: %v", s, err)
		}
		if !strings.HasSuffix(s, "\n") {
			t.Errorf("response must end with newline: %q", s)
		}
		return m
	}
	m := check(t, respOK())
	if m["ok"] != true {
		t.Errorf("respOK: ok must be true")
	}
	m = check(t, respOKVersion("1.2.3"))
	if m["version"] != "1.2.3" {
		t.Errorf("respOKVersion: version = %v", m["version"])
	}
	m = check(t, respErr("unknown_command"))
	if m["ok"] != false || m["error"] != "unknown_command" {
		t.Errorf("respErr: got %v", m)
	}
}
