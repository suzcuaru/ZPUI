package app

import (
	"os"
	"path/filepath"
	"testing"

	"zpui/internal/database"
)

// Одна БД на весь пакет: database.Init внутри защищён sync.Once, поэтому
// повторные вызовы в тестах безопасны, а Close делаем только здесь.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "zpui-app-test-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	if err := database.Init(filepath.Join(dir, "test.db")); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = database.Close()
	os.Exit(code)
}
