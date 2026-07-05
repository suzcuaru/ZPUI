package updater

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const releaseCacheTTL = 5 * time.Minute

var (
	cacheMu      sync.RWMutex
	cachedRel    *releaseInfo
	cachedBody   []byte
	cachedEtag   string
	cachedAt     time.Time
	cacheFileDir string
)

// SetCacheDir задаёт каталог для персистентного кеша ответов GitHub API.
// Должен вызываться на старте приложения.
func SetCacheDir(dir string) {
	cacheMu.Lock()
	cacheFileDir = dir
	cacheMu.Unlock()
}

type releaseCacheFile struct {
	ETag string          `json:"etag"`
	Body json.RawMessage `json:"body"`
}

func cacheFilePath() string {
	if cacheFileDir == "" {
		return ""
	}
	return filepath.Join(cacheFileDir, "gh_release_cache.json")
}

func loadPersistentCache() (etag string, body []byte) {
	p := cacheFilePath()
	if p == "" {
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var cf releaseCacheFile
	if json.Unmarshal(data, &cf) == nil {
		etag = cf.ETag
		body = cf.Body
	}
	return
}

func savePersistentCache(etag string, body []byte) {
	p := cacheFilePath()
	if p == "" || etag == "" || body == nil {
		return
	}
	data, err := json.Marshal(releaseCacheFile{ETag: etag, Body: body})
	if err != nil {
		return
	}
	os.WriteFile(p, data, 0644)
}

// storeCache сохраняет результат в памяти (TTL) и на диск (ETag).
func storeCache(rel *releaseInfo, body []byte, etag string) {
	cacheMu.Lock()
	cachedRel = rel
	cachedBody = body
	cachedEtag = etag
	cachedAt = time.Now()
	cacheMu.Unlock()
	savePersistentCache(etag, body)
}
