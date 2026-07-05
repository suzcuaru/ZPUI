package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/logger"
)

var version = "1.0.0"

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseInfo struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Assets  []releaseAsset `json:"assets"`
}

const apiURL = "https://api.github.com/repos/suzcuaru/ZPUI/releases/latest"

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	logFile := filepath.Join(exeDir, "logs", "selfupdate.log")
	os.MkdirAll(filepath.Dir(logFile), 0755)
	logMgr, _ := logger.New(filepath.Dir(logFile), 7)
	defer logMgr.Close()

	log := func(msg string) {
		fmt.Println(msg)
		logMgr.Info("selfupdate", msg)
	}
	logErr := func(msg string) {
		fmt.Fprintln(os.Stderr, msg)
		logMgr.Error("selfupdate", msg)
	}

	log("Self-updater started")

	zpuiExe := filepath.Join(exeDir, "zpui.exe")
	if _, err := os.Stat(zpuiExe); err != nil {
		logErr("zpui.exe not found in " + exeDir)
		os.Exit(1)
	}

	log("Checking latest release...")
	rel, err := getLatestRelease()
	if err != nil {
		logErr("Failed to check release: " + err.Error())
		os.Exit(1)
	}
	log(fmt.Sprintf("Latest: %s (%s)", rel.TagName, rel.Name))

	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == "zpui.zip" || a.Name == "zpui-windows.zip" {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" && len(rel.Assets) > 0 {
		downloadURL = rel.Assets[0].BrowserDownloadURL
	}
	if downloadURL == "" {
		logErr("No download asset found in release")
		os.Exit(1)
	}

	backupDir := filepath.Join(exeDir, ".backup")
	os.MkdirAll(backupDir, 0755)

	log("Backing up zpui.exe...")
	copyFile(zpuiExe, filepath.Join(backupDir, "zpui.exe.bak"))

	dbPath := filepath.Join(exeDir, "zpui.db")
	if _, err := os.Stat(dbPath); err == nil {
		copyFile(dbPath, filepath.Join(backupDir, "zpui.db.bak"))
		log("Backed up database")
	}

	log("Stopping zpui.exe...")
	exec.Command("taskkill", "/IM", "zpui.exe", "/F").Run()
	time.Sleep(2 * time.Second)

	log("Downloading update...")
	zipPath := filepath.Join(exeDir, "zpui-update.zip")
	if err := downloadFile(downloadURL, zipPath); err != nil {
		logErr("Download failed: " + err.Error())
		log("Restoring backup...")
		copyFile(filepath.Join(backupDir, "zpui.exe.bak"), zpuiExe)
		os.Exit(1)
	}

	log("Extracting...")
	if err := unzipTo(zipPath, exeDir); err != nil {
		logErr("Extract failed: " + err.Error())
		log("Restoring backup...")
		copyFile(filepath.Join(backupDir, "zpui.exe.bak"), zpuiExe)
		copyFile(filepath.Join(backupDir, "zpui.db.bak"), dbPath)
		os.Exit(1)
	}

	os.Remove(zipPath)
	log("Update complete!")

	log("Starting zpui.exe...")
	exec.Command(zpuiExe).Start()
}

func getLatestRelease() (*releaseInfo, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rel releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func unzipTo(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	selfName := ""
	if exe, err := os.Executable(); err == nil {
		selfName = strings.ToLower(filepath.Base(exe))
	}

	for _, f := range r.File {
		if selfName != "" && strings.ToLower(filepath.Base(f.Name)) == selfName {
			continue
		}
		path := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(path), 0755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(path)
		if err != nil {
			rc.Close()
			return err
		}
		io.Copy(out, rc)
		out.Close()
		rc.Close()
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
