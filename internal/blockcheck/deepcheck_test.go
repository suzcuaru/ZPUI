package blockcheck

import (
	"testing"
)

func TestCollectHosts(t *testing.T) {
	body := []byte(`
		<!DOCTYPE html><html>
		<head>
			<link rel="stylesheet" href="https://CDN.EXAMPLE.com/style.css">
			<script src="https://api.discord.com/v9/track.js"></script>
			<link href="//fonts.gstatic.com/font.woff">
		</head>
			<img src="/static/logo.png">
			<img src="https://i.ytimg.com/vi/abc/hqdefault.jpg">
			<iframe src="https://www.youtube.com/embed/abc"></iframe>
			fetch("https://api2.example.com:443/data")
			url(//cdn2.test.ru/path)
			<a href="mailto:noreply@test.com">mail</a>
		`)

	hosts := CollectHosts(body, "youtube.com")

	want := map[string]bool{
		"cdn.example.com":    true,
		"api.discord.com":    true,
		"fonts.gstatic.com":  true,
		"i.ytimg.com":        true,
		"api2.example.com":   true,
		"cdn2.test.ru":       true,
	}

	got := map[string]bool{}
	for _, h := range hosts {
		got[h] = true
	}

	for h := range want {
		if !got[h] {
			t.Errorf("expected host %q in results, got %v", h, hosts)
		}
	}
	// the main host (youtube.com) and www.youtube.com must be skipped
	if got["youtube.com"] {
		t.Errorf("main host should be skipped")
	}
	if got["www.youtube.com"] {
		t.Errorf("www-prefixed main host should be skipped")
	}
	// relative path /static/logo.png yields no host
	if got["static"] || got["logo.png"] {
		t.Errorf("relative path should not produce a host: %v", hosts)
	}
}

func TestLooksLikeHost(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"cdn.example.com", true},
		{"api2.example.com", true},
		{"1.2.3", false},   // no letter
		{"localhost", false}, // no dot
		{"", false},
		{"example.com", true},
	}
	for _, c := range cases {
		if got := looksLikeHost(c.in); got != c.want {
			t.Errorf("looksLikeHost(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
