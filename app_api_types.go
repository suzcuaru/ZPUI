package main

import (
	"os"
)

// logFileInfo описывает запись в листинге лог-файлов (errors/archive).
type logFileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Date string `json:"date"`
}

// listLogDir возвращает файлы из каталога dir в обратном порядке (новые сверху),
// пропуская подкаталоги. При ошибке чтения возвращает пустой срез.
func listLogDir(dir string) []logFileInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []logFileInfo{}
	}
	var files []logFileInfo
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.IsDir() {
			continue
		}
		info, _ := e.Info()
		files = append(files, logFileInfo{
			Name: e.Name(),
			Size: info.Size(),
			Date: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	if files == nil {
		files = []logFileInfo{}
	}
	return files
}

// errResp возвращает стандартный ответ с ошибкой.
func errResp(msg string) map[string]interface{} {
	return map[string]interface{}{"error": msg}
}

// okResp возвращает стандартный успешный ответ.
func okResp() map[string]interface{} {
	return map[string]interface{}{"status": "ok"}
}
