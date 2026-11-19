package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type ChecksumEntry struct {
	File   string
	SHA256 string
	Size   int64
}

func ComputeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func VerifyFileChecksum(path, expectedSHA256 string) (bool, error) {
	if expectedSHA256 == "" {
		return false, fmt.Errorf("empty checksum")
	}
	actual, err := ComputeSHA256(path)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(actual, strings.TrimSpace(expectedSHA256)), nil
}

func ParseChecksums(data []byte) map[string]ChecksumEntry {
	result := make(map[string]ChecksumEntry)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var hash, filename string
		var size int64

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		hash = parts[0]
		rest := strings.Join(parts[1:], " ")

		if idx := strings.Index(rest, "("); idx >= 0 {
			filename = strings.TrimSpace(rest[:idx])
			if endIdx := strings.Index(rest, " bytes)"); endIdx > idx {
				sizeStr := strings.TrimSpace(rest[idx+1 : endIdx])
				var sz int64
				fmt.Sscanf(sizeStr, "%d", &sz)
				size = sz
			}
		} else {
			filename = strings.TrimSpace(rest)
		}

		filename = strings.TrimPrefix(filename, "*")
		result[strings.ToLower(filename)] = ChecksumEntry{
			File:   filename,
			SHA256: strings.ToLower(hash),
			Size:   size,
		}
	}
	return result
}

// UpdateChecksumEntry переписывает строку файла в checksums.sha256 рядом с exe.
// Нужен после замены модуля: строка от инсталлятора перестаёт соответствовать
// новому файлу, и проверка целостности при каждом старте рапортует CHECKSUM MISMATCH.
func UpdateChecksumEntry(exeDir, name, sha string, size int64) error {
	p := filepath.Join(exeDir, "checksums.sha256")
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.EqualFold(ParseChecksums([]byte(trimmed))[strings.ToLower(name)].File, name) {
			continue
		}
		lines = append(lines, trimmed)
	}
	lines = append(lines, fmt.Sprintf("%s  %s (%d bytes)", sha, name, size))
	return os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

type progressWriter struct {
	total      int64
	written    int64
	progress   func(percent int, downloaded, total int64)
	lastReport int
	lastBytes  int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	if pw.progress == nil {
		return n, nil
	}
	if pw.total > 0 {
		pct := int(pw.written * 100 / pw.total)
		if pct > pw.lastReport+2 || pct >= 100 {
			pw.lastReport = pct
			pw.progress(pct, pw.written, pw.total)
		}
	} else if pw.written-pw.lastBytes >= 2<<20 {
		// размер неизвестен (напр., zip папки с Яндекс.Диска) — сообщаем объём
		pw.lastBytes = pw.written
		pw.progress(0, pw.written, 0)
	}
	return n, nil
}
