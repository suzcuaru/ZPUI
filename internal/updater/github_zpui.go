package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Релизы ZPUI на GitHub — источник обновлений приложения в режимах
// github/ghproxy (и третий канал в auto после сервера и Яндекса).
// checksums.sha256 прикладывается релизным workflow (release.yml).

const zpuiGithubLatestURL = "https://api.github.com/repos/suzcuaru/ZPUI/releases/latest"

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

func fetchGHRelease(apiURL string) (*ghRelease, error) {
	req, err := newGitHubRequest(apiURL)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api: статус %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var rel ghRelease
	if err := json.Unmarshal(raw, &rel); err != nil {
		return nil, fmt.Errorf("разбор ответа GitHub: %w", err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("у релиза нет тега")
	}
	return &rel, nil
}

// CheckZPUIGithubRelease — последний релиз ZPUI на GitHub: ассет
// ZPUI-Setup-<ver>.exe + соседний checksums.sha256 (если приложен).
func CheckZPUIGithubRelease(current string) (*ZPUISetupInfo, error) {
	rel, err := fetchGHRelease(zpuiGithubLatestURL)
	if err != nil {
		return nil, err
	}
	ver := strings.TrimPrefix(strings.TrimPrefix(rel.TagName, "v"), "V")
	var setup, sums *ghAsset
	for i := range rel.Assets {
		n := strings.ToLower(rel.Assets[i].Name)
		switch {
		case strings.HasPrefix(n, "zpui-setup-") && strings.HasSuffix(n, ".exe") && !strings.Contains(n, "win32"):
			if setup == nil {
				setup = &rel.Assets[i]
			}
		case n == "checksums.sha256":
			sums = &rel.Assets[i]
		}
	}
	if setup == nil {
		return nil, fmt.Errorf("в релизе %s нет ZPUI-Setup-*.exe", rel.TagName)
	}
	info := &ZPUISetupInfo{
		Version:      ver,
		URL:          setup.BrowserDownloadURL,
		Source:       "github",
		ChecksumsURL: "",
	}
	if sums != nil {
		info.ChecksumsURL = sums.BrowserDownloadURL
	}
	return info, nil
}

// FetchChecksumsHTTP — скачивает checksums.sha256 по прямому URL
// (для источника GitHub; для Яндекса есть FetchChecksumsYandex).
func FetchChecksumsHTTP(url string) (map[string]ChecksumEntry, error) {
	req, err := newGitHubRequest(url)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksums: статус %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	out := ParseChecksums(raw)
	if len(out) == 0 {
		return nil, fmt.Errorf("checksums.sha256 пуст или не распознан")
	}
	return out, nil
}
