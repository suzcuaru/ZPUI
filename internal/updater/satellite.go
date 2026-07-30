package updater

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	githubReleasePage = "https://github.com/suzcuaru/ZPUI/releases/latest"
	downloadBase      = githubReleasePage + "/download/"
	versionsURL       = downloadBase + "versions.json"
	githubAPIURL      = "https://api.github.com/repos/suzcuaru/ZPUI/releases/latest"
	userAgent         = "ZPUI/updater"
)

type RemoteVersions struct {
	ZPUI       string `json:"zpui"`
	SelfUpdate string `json:"selfupdate"`
	Report     string `json:"report"`
	Security   string `json:"security"`
}

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

func GetReleaseInfo() (*ReleaseInfo, error) {
	info, err := fetchReleaseFromAtom("suzcuaru", "ZPUI")
	if err != nil {
		rel, err2 := fetchReleaseInfo()
		if err2 != nil {
			return nil, err
		}
		return &ReleaseInfo{
			TagName: rel.TagName, Name: rel.Name, Body: rel.Body,
			PublishedAt: rel.PublishedAt, HTMLURL: rel.HTMLURL,
		}, nil
	}
	return info, nil
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

type ComponentUpdateStatus struct {
	Name        string `json:"name"`
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	NeedsUpdate bool   `json:"needs_update"`
	DownloadURL string `json:"download_url"`
	File        string `json:"file"`
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

// fetchReleaseInfo получает информацию о последнем релизе с кешированием
// (in-memory TTL 5 минут + ETag/If-None-Match для экономии rate-limit GitHub).
func fetchReleaseInfo() (*releaseInfo, error) {
	cacheMu.RLock()
	if cachedRel != nil && time.Since(cachedAt) < releaseCacheTTL {
		r := *cachedRel
		cacheMu.RUnlock()
		return &r, nil
	}
	etag := cachedEtag
	body := cachedBody
	cacheMu.RUnlock()

	if etag == "" || body == nil {
		etag, body = loadPersistentCache()
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := newGitHubRequest(githubAPIURL)
	if err != nil {
		return nil, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := client.Do(req)
	if err != nil {
		if rel := releaseFromCachedBody(body); rel != nil {
			storeCache(rel, body, etag)
			return rel, nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		if rel := releaseFromCachedBody(body); rel != nil {
			storeCache(rel, body, etag)
			return rel, nil
		}
		return nil, fmt.Errorf("github 304 без кеша")
	case http.StatusOK:
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			if rel := releaseFromCachedBody(body); rel != nil {
				return rel, nil
			}
			return nil, fmt.Errorf("чтение ответа github: %w", err)
		}
		var rel releaseInfo
		if err := json.Unmarshal(raw, &rel); err != nil {
			return nil, fmt.Errorf("разбор ответа github: %w", err)
		}
		storeCache(&rel, raw, resp.Header.Get("ETag"))
		return &rel, nil
	default:
		if rel := releaseFromCachedBody(body); rel != nil {
			return rel, nil
		}
		return nil, fmt.Errorf("github api вернул статус %d", resp.StatusCode)
	}
}

// releaseFromCachedBody разбирает кешированное тело ответа GitHub API.
// Возвращает nil, если кеша нет или он невалиден.
func releaseFromCachedBody(body []byte) *releaseInfo {
	if len(body) == 0 {
		return nil
	}
	var rel releaseInfo
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil
	}
	if rel.TagName == "" {
		return nil
	}
	return &rel
}

func findAsset(assets []releaseAsset, name string) string {
	for _, a := range assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// FetchRemoteVersions получает версии с Яндекс.Диска.
func FetchRemoteVersions() (*RemoteVersions, error) {
	return FetchRemoteVersionsYandex(YandexPublicURL)
}

func FetchRemoteVersionsGitHub() (*RemoteVersions, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", versionsURL, nil)
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
		return nil, fmt.Errorf("versions.json: status %d", resp.StatusCode)
	}

	var rv RemoteVersions
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rv); err != nil {
		return nil, fmt.Errorf("parse versions.json: %w", err)
	}
	return &rv, nil
}

func CheckAllComponents(localVersions map[string]string, exeDir string) ([]ComponentUpdateStatus, error) {
	remote, err := FetchRemoteVersions()
	if err != nil {
		return nil, err
	}

	remoteMap := map[string]string{
		"zpui":       remote.ZPUI,
		"selfupdate": remote.SelfUpdate,
		"report":     remote.Report,
		"security":   remote.Security,
	}

	fileMap := map[string]string{
		"zpui":       "ZPUI.exe",
		"selfupdate": "selfupdate.exe",
		"report":     "report.exe",
		"security":   "security.exe",
	}

	order := []string{"zpui", "selfupdate", "report", "security"}
	var result []ComponentUpdateStatus

	for _, key := range order {
		current := normalizeVersion(localVersions[key])
		latest := normalizeVersion(remoteMap[key])
		needs := latest != "" && IsNewer(current, latest)

		result = append(result, ComponentUpdateStatus{
			Name:        key,
			Current:     localVersions[key],
			Latest:      latest,
			NeedsUpdate: needs,
			File:        fileMap[key],
		})
	}

	return result, nil
}

func ReplaceModule(exeDir, name string) error {
	fileMap := map[string]string{
		"selfupdate": "selfupdate.exe",
		"report":     "report.exe",
		"security":   "security.exe",
	}

	fileName, ok := fileMap[name]
	if !ok {
		return fmt.Errorf("unknown module: %s", name)
	}

	targetPath := filepath.Join(exeDir, fileName)
	bakPath := targetPath + ".bak"

	if err := ReplaceComponent(exeDir, name); err == nil {
		return nil
	}

	yaURL, yErr := yandexDownloadURL(YandexPublicURL, "", fileName)
	if yErr != nil || yaURL == "" {
		return fmt.Errorf("download %s failed: %v", fileName, yErr)
	}
	if err := downloadFromYandex(yaURL, targetPath+".tmp"); err != nil {
		return fmt.Errorf("yandex download failed: %w", err)
	}

	bm := NewBackupManager(exeDir)
	if _, err := os.Stat(targetPath); err == nil {
		bm.BackupComponent("module_"+name, "pre-update", "module", []string{targetPath})
		os.Rename(targetPath, bakPath)
	}

	if err := os.Rename(targetPath+".tmp", targetPath); err != nil {
		os.Rename(bakPath, targetPath)
		return fmt.Errorf("replace failed: %w", err)
	}

	os.Remove(bakPath)
	return nil
}

func extractFromZip(zipPath, fileName, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.EqualFold(filepath.Base(f.Name), fileName) {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.Create(destPath + ".tmp")
			if err != nil {
				return err
			}
			defer out.Close()

			_, err = io.Copy(out, rc)
			return err
		}
	}

	return fmt.Errorf("file %s not found in archive", fileName)
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("статус %d при загрузке %s", resp.StatusCode, filepath.Base(dest))
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(resp.Body, 100<<20))
	return err
}
