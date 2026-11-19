package blockcheck

import (
	"strings"
	"testing"
)

func has(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func TestSubdomainCandidatesKnownService(t *testing.T) {
	got := SubdomainCandidates("discord.com")
	want := []string{
		"api.discord.com", "cdn.discord.com", "cdn.discordapp.com",
		"discord.media", "discordapp.net", "gateway.discord.com", "gateway.discord.gg",
	}
	for _, w := range want {
		if !has(got, w) {
			t.Errorf("discord.com: ожидался кандидат %q, нет в %v", w, got)
		}
	}
	if has(got, "discord.com") {
		t.Errorf("сам хост не должен быть в кандидатах: %v", got)
	}
}

func TestSubdomainCandidatesPrefixes(t *testing.T) {
	got := SubdomainCandidates("example.org")
	if len(got) == 0 {
		t.Fatal("для обычного домена ожидались префиксные кандидаты")
	}
	for _, p := range []string{"api.example.org", "cdn.example.org", "media.example.org"} {
		if !has(got, p) {
			t.Errorf("ожидался префиксный кандидат %q, нет в %v", p, got)
		}
	}
	if len(got) != len(subdomainPrefixes) {
		t.Errorf("число кандидатов = %d, ожидалось %d (по числу префиксов)", len(got), len(subdomainPrefixes))
	}
}

func TestSubdomainCandidatesWwwHost(t *testing.T) {
	got := SubdomainCandidates("www.example.org")
	if len(got) == 0 {
		t.Fatal("для www-хоста ожидались кандидаты по базе example.org")
	}
	for _, h := range got {
		if h == "www.example.org" {
			t.Errorf("исходный хост www.example.org не должен попадать в кандидаты: %v", got)
		}
		if strings.HasPrefix(h, "www.www.") {
			t.Errorf("недопустимый кандидат %q", h)
		}
	}
	if !has(got, "api.example.org") {
		t.Errorf("кандидаты должны строиться от базы example.org, а не www.example.org: %v", got)
	}
}

func TestSubdomainCandidatesInvalid(t *testing.T) {
	if SubdomainCandidates("") != nil {
		t.Error("пустой хост должен давать nil")
	}
	if SubdomainCandidates("localhost") != nil {
		t.Error("хост без точки должен давать nil")
	}
	if got := SubdomainCandidates("https://example.org/"); got == nil || !has(got, "api.example.org") {
		t.Errorf("URL вместо хоста должен нормализоваться к базе: %v", got)
	}
}

func TestSubdomainCandidatesSortedDedup(t *testing.T) {
	got := SubdomainCandidates("7tv.app")
	if len(got) == 0 {
		t.Fatal("кандидаты не ожидались пустыми")
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("кандидаты не отсортированы/дубликаты: %v", got)
		}
	}
}
