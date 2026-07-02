package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type BackupEntry struct {
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	Component  string    `json:"component"`
	BackupPath string    `json:"backup_path"`
	CreatedAt  time.Time `json:"created_at"`
	Size       int64     `json:"size"`
}

type IgnoredVersion struct {
	Component string `json:"component"`
	Version   string `json:"version"`
	Reason    string `json:"reason"`
}

type BackupManager struct {
	backupDir string
	ignoreDir string
}

func NewBackupManager(appDir string) *BackupManager {
	return &BackupManager{
		backupDir: filepath.Join(appDir, "backups"),
		ignoreDir: filepath.Join(appDir, "config"),
	}
}

func (bm *BackupManager) BackupComponent(name, version, componentType string, paths []string) (*BackupEntry, error) {
	timestamp := time.Now().Format("20060102_150405")
	safeName := strings.NewReplacer(".", "_", "/", "_", "\\", "_").Replace(name)
	backupName := fmt.Sprintf("%s_%s_%s", safeName, version, timestamp)
	backupPath := filepath.Join(bm.backupDir, backupName)

	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	var totalSize int64
	for _, src := range paths {
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		dest := filepath.Join(backupPath, filepath.Base(src))
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", src, err)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", dest, err)
		}
		totalSize += int64(len(data))
	}

	entry := &BackupEntry{
		Name:       backupName,
		Version:    version,
		Component:  name,
		BackupPath: backupPath,
		CreatedAt:  time.Now(),
		Size:       totalSize,
	}

	manifest := filepath.Join(backupPath, ".backup.json")
	data, _ := json.Marshal(entry)
	os.WriteFile(manifest, data, 0644)

	return entry, nil
}

func (bm *BackupManager) RestoreBackup(backupName string) error {
	backupPath := filepath.Join(bm.backupDir, backupName)
	manifestPath := filepath.Join(backupPath, ".backup.json")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("backup manifest not found: %s", backupName)
	}

	var entry BackupEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return err
	}

	entries, err := os.ReadDir(backupPath)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		src := filepath.Join(backupPath, e.Name())
		dst := filepath.Join(filepath.Dir(bm.backupDir), e.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read backup file %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("restore %s: %w", dst, err)
		}
	}

	return nil
}

func (bm *BackupManager) ListBackups(component string) []BackupEntry {
	var result []BackupEntry

	entries, err := os.ReadDir(bm.backupDir)
	if err != nil {
		return result
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(bm.backupDir, e.Name(), ".backup.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var entry BackupEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}
		if component != "" && entry.Component != component {
			continue
		}
		result = append(result, entry)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result
}

func (bm *BackupManager) AddIgnoredVersion(component, version, reason string) error {
	ignoreFile := filepath.Join(bm.ignoreDir, "ignore_versions.json")

	var ignored []IgnoredVersion
	if data, err := os.ReadFile(ignoreFile); err == nil {
		json.Unmarshal(data, &ignored)
	}

	for _, iv := range ignored {
		if iv.Component == component && iv.Version == version {
			return nil
		}
	}

	ignored = append(ignored, IgnoredVersion{
		Component: component,
		Version:   version,
		Reason:    reason,
	})

	data, _ := json.MarshalIndent(ignored, "", "  ")
	return os.WriteFile(ignoreFile, data, 0644)
}

func (bm *BackupManager) IsVersionIgnored(component, version string) bool {
	ignoreFile := filepath.Join(bm.ignoreDir, "ignore_versions.json")
	data, err := os.ReadFile(ignoreFile)
	if err != nil {
		return false
	}

	var ignored []IgnoredVersion
	if err := json.Unmarshal(data, &ignored); err != nil {
		return false
	}

	for _, iv := range ignored {
		if iv.Component == component && iv.Version == version {
			return true
		}
	}
	return false
}

func (bm *BackupManager) RemoveIgnoredVersion(component, version string) error {
	ignoreFile := filepath.Join(bm.ignoreDir, "ignore_versions.json")
	data, err := os.ReadFile(ignoreFile)
	if err != nil {
		return nil
	}

	var ignored []IgnoredVersion
	if err := json.Unmarshal(data, &ignored); err != nil {
		return nil
	}

	var filtered []IgnoredVersion
	for _, iv := range ignored {
		if iv.Component != component || iv.Version != version {
			filtered = append(filtered, iv)
		}
	}

	out, _ := json.MarshalIndent(filtered, "", "  ")
	return os.WriteFile(ignoreFile, out, 0644)
}

func (bm *BackupManager) ListIgnoredVersions() []IgnoredVersion {
	ignoreFile := filepath.Join(bm.ignoreDir, "ignore_versions.json")
	data, err := os.ReadFile(ignoreFile)
	if err != nil {
		return nil
	}

	var ignored []IgnoredVersion
	if err := json.Unmarshal(data, &ignored); err != nil {
		return nil
	}

	return ignored
}
