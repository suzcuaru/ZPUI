package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"zpui/internal/logger"
	"zpui/internal/security"
)

var version = "1.0.0"

func downloadURL() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return "https://github.com/suzcuaru/ZPUI/releases/latest/download/zpui-win64.zip"
	case "386":
		return "https://github.com/suzcuaru/ZPUI/releases/latest/download/zpui-win32.zip"
	default:
		return "https://github.com/suzcuaru/ZPUI/releases/latest/download/zpui-win64.zip"
	}
}

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

	log("Self-updater started (v" + version + ")")

	zpuiExe := filepath.Join(exeDir, "zpui.exe")
	if _, err := os.Stat(zpuiExe); err != nil {
		logErr("zpui.exe not found in " + exeDir)
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

	log("Downloading update (" + downloadURL() + ")...")
	zipPath := filepath.Join(exeDir, "zpui-update.zip")
	if err := downloadFile(downloadURL(), zipPath); err != nil {
		logErr("Download failed: " + err.Error())
		log("Restoring backup...")
		copyFile(filepath.Join(backupDir, "zpui.exe.bak"), zpuiExe)
		os.Exit(1)
	}

		log("Scanning update for malware...")
		scanResult, scanErr := security.ScanZip(zipPath, []string{})
		if scanErr != nil {
			logErr("Security scan failed: " + scanErr.Error())
			log("Restoring backup...")
			copyFile(filepath.Join(backupDir, "zpui.exe.bak"), zpuiExe)
			os.Remove(zipPath)
			os.Exit(1)
		}
		if !scanResult.Clean {
			logErr(security.FormatScanResult(scanResult))
			log("Restoring backup...")
			copyFile(filepath.Join(backupDir, "zpui.exe.bak"), zpuiExe)
			os.Remove(zipPath)
			os.Exit(1)
		}
		log(security.FormatScanResult(scanResult))

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
