package config

// DNS 2.0: два файла рядом с config.json.
//
//	dns-system.json — системный список провайдеров. Источник истины — сервер
//	(op dns.providers с версией): клиент сравнивает версию и заменяет файл
//	ЦЕЛИКОМ (удаление на сервере доезжает до клиента). При отсутствии или
//	битости создаётся из вкомпилированного сида (DnsProvidersSeed).
//	Установщик файл не трогает (pkginst.protectedUserRel).
//
//	dns-user.json — пользовательские записи (CustomDns). Источник истины —
//	файл: при первом старте мигрирует из config.json (XboxDns.Custom), дальше
//	записывается при каждом сохранении конфига (save()).
//
// Оба файла — те же «skip-resources*.txt», только для DNS: приложение
// создаёт дефолт, сервер управляет системным, пользователь свой не теряет.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const (
	dnsSystemFileName = "dns-system.json"
	dnsUserFileName   = "dns-user.json"
)

var (
	dnsSystemMu       sync.RWMutex
	dnsSystemCurrent  []DnsProvider // то, что сейчас в файле (пусто = сид)
	dnsSystemPathSet  string        // путь к dns-system.json (фиксируется при старте)
	dnsSystemRestored bool          // файл пересоздавался из сида при старте
)

func (c *Config) dnsSystemPath() string {
	return filepath.Join(filepath.Dir(c.configPath), dnsSystemFileName)
}

func (c *Config) dnsUserPath() string {
	return filepath.Join(filepath.Dir(c.configPath), dnsUserFileName)
}

// ensureDnsFiles — при старте: создать/починить dns-system.json, мигрировать
// пользовательские записи в dns-user.json и загрузить оба в память.
// Вызывается из Load() один раз, до старта горутин.
func (c *Config) ensureDnsFiles() {
	// --- системный список ---
	dnsSystemPathSet = c.dnsSystemPath()
	entries := []DnsProvider{}
	if data, err := os.ReadFile(dnsSystemPathSet); err == nil {
		_ = json.Unmarshal(data, &entries)
	}
	if len(entries) == 0 {
		// нет файла или он битый — пересоздаём из сида
		entries = make([]DnsProvider, len(DnsProvidersSeed))
		copy(entries, DnsProvidersSeed)
		if err := writeJSONFile(dnsSystemPathSet, entries); err == nil {
			dnsSystemRestored = true
		}
	}
	dnsSystemMu.Lock()
	dnsSystemCurrent = entries
	dnsSystemMu.Unlock()

	// --- пользовательские записи: миграция в файл ---
	usrPath := c.dnsUserPath()
	if _, err := os.Stat(usrPath); os.IsNotExist(err) {
		_ = writeJSONFile(usrPath, c.XboxDns.Custom) // возможно пустой массив
	}
	// файл — источник истины: затираем то, что было в config.json
	if data, err := os.ReadFile(usrPath); err == nil {
		var custom []CustomDns
		if json.Unmarshal(data, &custom) == nil && custom != nil {
			c.XboxDns.Custom = custom
		}
	}
}

// writeJSONFile — компактная запись JSON-массива с отступами.
func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Providers — актуальный системный список провайдеров (из dns-system.json;
// пока файл не создан — сид). Вызывается из UI/движка вместо чтения
// DnsProviders напрямую.
func Providers() []DnsProvider {
	dnsSystemMu.RLock()
	defer dnsSystemMu.RUnlock()
	if len(dnsSystemCurrent) == 0 {
		return DnsProvidersSeed
	}
	out := make([]DnsProvider, len(dnsSystemCurrent))
	copy(out, dnsSystemCurrent)
	return out
}

// ProviderByID — провайдер системного списка по id (nil = нет такого).
func ProviderByID(id string) *DnsProvider {
	for _, p := range Providers() {
		if p.ID == id {
			p := p
			return &p
		}
	}
	return nil
}

// SetSystemProviders — полная замена системного списка (вызывает синк с
// сервера) с записью файла. Пустой список игнорируется: пустой файл означает
// «битый синк», клиент остаётся на прежнем наборе.
func SetSystemProviders(entries []DnsProvider) error {
	if len(entries) == 0 {
		return nil
	}
	dnsSystemMu.Lock()
	defer dnsSystemMu.Unlock()
	if dnsSystemPathSet == "" {
		return nil // конфиг ещё не загружен — синк не должен был зоваться
	}
	if err := writeJSONFile(dnsSystemPathSet, entries); err != nil {
		return err
	}
	dnsSystemCurrent = entries
	return nil
}
