package updater

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const userAgent = "ZPUI/updater"

const releaseCacheTTL = 5 * time.Minute

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseInfo struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	PublishedAt string         `json:"published_at"`
	HTMLURL     string         `json:"html_url"`
	Assets      []releaseAsset `json:"assets"`
}

type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
}

const zapretGithubAPIURL = "https://api.github.com/repos/Flowseal/zapret-discord-youtube/releases/latest"

func GetZapretReleaseInfo() (*ReleaseInfo, error) {
	info, err := fetchReleaseFromAtom("Flowseal", "zapret-discord-youtube")
	if err != nil {
		client := &http.Client{Timeout: 15 * time.Second}
		req, err2 := newGitHubRequest(zapretGithubAPIURL)
		if err2 != nil {
			return nil, err
		}
		resp, err3 := client.Do(req)
		if err3 != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("github вернул статус %d", resp.StatusCode)
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var rel releaseInfo
		if err := json.Unmarshal(raw, &rel); err != nil {
			return nil, fmt.Errorf("разбор ответа: %w", err)
		}
		return &ReleaseInfo{
			TagName: rel.TagName, Name: rel.Name, Body: rel.Body,
			PublishedAt: rel.PublishedAt, HTMLURL: rel.HTMLURL,
		}, nil
	}
	return info, nil
}

// GetZapretReleaseInfoByTag возвращает release notes конкретной версии
// zapret. Тела релизов Flowseal — фактический чейнджлог (файла CHANGELOG в
// репозитории нет). Источники: Atom-фид (без rate limit, ~10 последних
// релизов) → REST releases/tags/{tag}. Результат кэшируется на 5 минут.
func GetZapretReleaseInfoByTag(version string) (*ReleaseInfo, error) {
	tag := strings.TrimSpace(version)
	if tag == "" {
		return GetZapretReleaseInfo()
	}

	if rel := cachedTagRelease("zapret:" + tag); rel != nil {
		return rel, nil
	}
	if info, err := fetchReleaseFromAtomByTag("Flowseal", "zapret-discord-youtube", tag); err == nil {
		storeTagRelease("zapret:"+tag, info)
		return info, nil
	}

	raw, err := fetchGitHubTagRaw("Flowseal", "zapret-discord-youtube", tag)
	if err != nil {
		return nil, err
	}
	var rel releaseInfo
	if err := json.Unmarshal(raw, &rel); err != nil {
		return nil, fmt.Errorf("разбор ответа: %w", err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("релиз %s не найден", tag)
	}
	info := &ReleaseInfo{
		TagName:     strings.TrimPrefix(rel.TagName, "v"),
		Name:        rel.Name,
		Body:        rel.Body,
		PublishedAt: rel.PublishedAt,
		HTMLURL:     rel.HTMLURL,
	}
	storeTagRelease("zapret:"+tag, info)
	return info, nil
}

// fetchGitHubTagRaw — GET api.github.com/repos/{o}/{r}/releases/tags/{tag}
// (сначала тег как есть, затем с префиксом «v»); возвращает raw JSON.
func fetchGitHubTagRaw(owner, repo, tag string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	ids := []string{tag}
	if len(tag) > 0 && tag[0] >= '0' && tag[0] <= '9' {
		ids = append(ids, "v"+tag)
	}
	var lastErr error
	for _, id := range ids {
		req, err := newGitHubRequest(fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s",
			owner, repo, url.PathEscape(id)))
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && readErr == nil {
			return raw, nil
		}
		lastErr = fmt.Errorf("github вернул статус %d", resp.StatusCode)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("релиз не найден")
	}
	return nil, lastErr
}

// Кэш release notes по тегам (5 минут) — щадит rate limit GitHub при
// повторных открытиях модалки.
var (
	tagRelMu    sync.Mutex
	tagRelCache = map[string]tagCacheEntry{}
)

type tagCacheEntry struct {
	rel *ReleaseInfo
	at  time.Time
}

func cachedTagRelease(key string) *ReleaseInfo {
	tagRelMu.Lock()
	defer tagRelMu.Unlock()
	e, ok := tagRelCache[key]
	if !ok || time.Since(e.at) >= releaseCacheTTL {
		return nil
	}
	c := *e.rel
	return &c
}

func storeTagRelease(key string, rel *ReleaseInfo) {
	tagRelMu.Lock()
	defer tagRelMu.Unlock()
	tagRelCache[key] = tagCacheEntry{rel: rel, at: time.Now()}
}

// --- Atom feed parsing (no API rate limit) ---

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID      string `xml:"id"`
	Title   string `xml:"title"`
	Updated string `xml:"updated"`
	Content struct {
		Type string `xml:"type,attr"`
		Body string `xml:",chardata"`
	} `xml:"content"`
	Links []atomLink `xml:"link"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

// fetchReleaseFromAtom парсит Atom-фид релизов GitHub (без rate limit API).
func fetchReleaseFromAtom(owner, repo string) (*ReleaseInfo, error) {
	feed, err := fetchAtomFeed(owner, repo)
	if err != nil {
		return nil, err
	}
	if len(feed.Entries) == 0 {
		return nil, fmt.Errorf("нет релизов в фиде")
	}
	entry := feed.Entries[0]
	tag := extractAtomTag(entry)

	htmlURL := ""
	for _, l := range entry.Links {
		if l.Rel == "alternate" {
			htmlURL = l.Href
			break
		}
	}
	if htmlURL == "" {
		htmlURL = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, tag)
	}

	return &ReleaseInfo{
		TagName:     tag,
		Name:        strings.TrimSpace(entry.Title),
		Body:        cleanHTML(entry.Content.Body),
		PublishedAt: entry.Updated,
		HTMLURL:     htmlURL,
	}, nil
}

// fetchReleaseFromAtomByTag — то же, но выбирает запись фида по тегу версии.
// В фиде ~10 последних релизов; более старые не найдутся (вызывающий
// фолбэчится на REST).
func fetchReleaseFromAtomByTag(owner, repo, tag string) (*ReleaseInfo, error) {
	feed, err := fetchAtomFeed(owner, repo)
	if err != nil {
		return nil, err
	}
	for _, entry := range feed.Entries {
		entryTag := strings.TrimPrefix(extractAtomTag(entry), "v")
		if entryTag != tag && entryTag != strings.TrimPrefix(tag, "v") {
			continue
		}
		htmlURL := ""
		for _, l := range entry.Links {
			if l.Rel == "alternate" {
				htmlURL = l.Href
				break
			}
		}
		if htmlURL == "" {
			htmlURL = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, entryTag)
		}
		return &ReleaseInfo{
			TagName:     entryTag,
			Name:        strings.TrimSpace(entry.Title),
			Body:        cleanHTML(entry.Content.Body),
			PublishedAt: entry.Updated,
			HTMLURL:     htmlURL,
		}, nil
	}
	return nil, fmt.Errorf("релиз %s не найден в фиде", tag)
}

func fetchAtomFeed(owner, repo string) (*atomFeed, error) {
	feedURL := fmt.Sprintf("https://github.com/%s/%s/releases.atom", owner, repo)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github page: статус %d", resp.StatusCode)
	}

	var feed atomFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("atom parse: %w", err)
	}
	return &feed, nil
}

func extractAtomTag(entry atomEntry) string {
	for _, l := range entry.Links {
		if l.Rel == "alternate" {
			parts := strings.Split(l.Href, "/")
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}
	parts := strings.Split(entry.ID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return entry.Title
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)
var multiNewlineRe = regexp.MustCompile(`\n{3,}`)

// cleanHTML удаляет HTML-теги и декодирует сущности, возвращая читаемый текст.
func cleanHTML(s string) string {
	text := htmlTagRe.ReplaceAllString(s, "\n")
	text = html.UnescapeString(text)
	lines := strings.Split(text, "\n")
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	result := strings.Join(out, "\n")
	result = multiNewlineRe.ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}

func newGitHubRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}
