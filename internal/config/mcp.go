package config

// McpConfig — локальный MCP-сервер (Model Context Protocol): управление
// ZPUI ИИ-агентами (ZCode, Claude Desktop, локальные мини-LLM). Слушает
// только 127.0.0.1, доступ по Bearer-ключу.
type McpConfig struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"` // по умолчанию 8930
	Key     string `json:"key"`  // 32 hex; генерируется при первом обращении
}

const McpDefaultPort = 8930

// GetMcpConfig возвращает конфиг MCP, при необходимости генерируя ключ
// (32 hex) и сохраняя его.
func (c *Config) GetMcpConfig() McpConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Mcp.Key == "" {
		c.Mcp.Key = generateHexSecret(16)
		_ = c.save()
	}
	if c.Mcp.Port == 0 {
		c.Mcp.Port = McpDefaultPort
	}
	return c.Mcp
}

// GetMcpKey — только ключ, без блокировки на запись (горячий путь HTTP).
func (c *Config) GetMcpKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Mcp.Key
}

func (c *Config) SetMcpEnabled(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Mcp.Enabled = on
	return c.save()
}

func (c *Config) SetMcpPort(port int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if port < 1 || port > 65535 {
		port = McpDefaultPort
	}
	c.Mcp.Port = port
	return c.save()
}

// RegenerateMcpKey выдаёт новый ключ и сохраняет его; старые сессии
// агентов с прежним ключом перестают проходить авторизацию.
func (c *Config) RegenerateMcpKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Mcp.Key = generateHexSecret(16)
	_ = c.save()
	return c.Mcp.Key
}
