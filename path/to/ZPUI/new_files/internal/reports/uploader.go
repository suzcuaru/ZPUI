package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Uploader uploads diagnostic reports to Yandex.Disk via WebDAV API.
type Uploader struct {
	publicKey string
	client    *http.Client
}

// NewUploader creates a new Yandex.Disk uploader.
func NewUploader(publicKey string) *Uploader {
	return &Uploader{
		publicKey: publicKey,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// SetPublicKey updates the public key.
func (u *Uploader) SetPublicKey(key string) { u.publicKey = key }

// UploadReport uploads an MD report to Yandex.Disk.
func (u *Uploader) UploadReport(content, filename string) error {
	if u.publicKey == "" {
		return fmt.Errorf("yandex disk public key not configured")
	}

	uploadURL, err := u.getUploadURL("ZPUI-Reports/" + filename)
	if err != nil {
		return fmt.Errorf("get upload url: %w", err)
	}

	req, err := http.NewRequest("PUT", uploadURL, bytes.NewReader([]byte(content)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/markdown; charset=utf-8")
	req.Header.Set("User-Agent", "ZPUI-Reporter/1.0")

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 202 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("upload status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// TestConnection checks if the Yandex.Disk public key works.
func (u *Uploader) TestConnection() error {
	if u.publicKey == "" { return fmt.Errorf("no public key") }
	q := url.Values{}
	q.Set("public_key", u.publicKey)
	reqURL := "https://cloud-api.yandex.net/v1/disk/public/resources?" + q.Encode()
	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("User-Agent", "ZPUI-Reporter/1.0")
	resp, err := u.client.Do(req)
	if err != nil { return err }
	resp.Body.Close()
	if resp.StatusCode != 200 { return fmt.Errorf("status %d", resp.StatusCode) }
	return nil
}

func (u *Uploader) getUploadURL(path string) (string, error) {
	q := url.Values{}
	q.Set("public_key", u.publicKey)
	q.Set("path", path)
	q.Set("overwrite", "true")
	reqURL := "https://cloud-api.yandex.net/v1/disk/public/resources/upload?" + q.Encode()

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil { return "", err }
	req.Header.Set("User-Agent", "ZPUI-Reporter/1.0")

	resp, err := u.client.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var result struct {
		Href    string `json:"href"`
		Method  string `json:"method"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Href == "" { return "", fmt.Errorf("empty href") }
	return result.Href, nil
}