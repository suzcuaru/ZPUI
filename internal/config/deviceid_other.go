//go:build !windows

package config

// Не-Windows сборки (тесты на другой ОС): реестра нет, ID живёт в config.json.
func loadDeviceIDRegistry() string      { return "" }
func persistDeviceIDRegistry(id string) {}
