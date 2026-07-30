package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
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

type progressWriter struct {
	total      int64
	written    int64
	progress   func(percent int, downloaded, total int64)
	lastReport int
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	if pw.progress != nil && pw.total > 0 {
		pct := int(pw.written * 100 / pw.total)
		if pct > pw.lastReport+2 || pct >= 100 {
			pw.lastReport = pct
			pw.progress(pct, pw.written, pw.total)
		}
	}
	return n, nil
}
