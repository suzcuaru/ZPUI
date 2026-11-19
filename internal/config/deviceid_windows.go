package config

import (
	"golang.org/x/sys/windows/registry"
)

// Идентификатор устройства живёт в реестре (HKCU\Software\ZPUI\DeviceID):
// прописывается один раз и больше не меняется — переживает переустановку
// приложения (uninstall эти данные не чистит) и смену IP/железа.
const (
	deviceIDKeyPath = `Software\ZPUI`
	deviceIDValue   = "DeviceID"
)

// loadDeviceIDRegistry — прочитать вечный ID; "" если ещё не записан.
func loadDeviceIDRegistry() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, deviceIDKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue(deviceIDValue)
	if err != nil {
		return ""
	}
	return v
}

// persistDeviceIDRegistry — записать ID один раз (существующий не трогаем).
func persistDeviceIDRegistry(id string) {
	if id == "" {
		return
	}
	if loadDeviceIDRegistry() != "" {
		return
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, deviceIDKeyPath, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.SetStringValue(deviceIDValue, id)
}
