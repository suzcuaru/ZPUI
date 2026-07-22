package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// Sha256Sum computes the SHA-256 checksum of a file.
func Sha256Sum(path string) (string, error) {
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

// VerifySha256 checks if a file matches the expected SHA-256 hash.
func VerifySha256(path, expectedHex string) error {
	actual, err := Sha256Sum(path)
	if err != nil {
		return fmt.Errorf("sha256 check failed: %w", err)
	}
	if actual != expectedHex {
		return fmt.Errorf("sha256 mismatch: got %s, expected %s", actual, expectedHex)
	}
	return nil
}

// VerifyFileSize checks if a file is within expected size range.
func VerifyFileSize(path string, minSize, maxSize int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat failed: %w", err)
	}
	if info.Size() < minSize {
		return fmt.Errorf("file too small: %d bytes (min %d)", info.Size(), minSize)
	}
	if maxSize > 0 && info.Size() > maxSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxSize)
	}
	return nil
}