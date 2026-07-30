package database

import "time"

// SessionDevice — устройство в текущей сессии
type SessionDevice struct {
	ID        string    `json:"id"`
	MAC       string    `json:"mac"`
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	TotalDL   int64     `json:"total_dl"`
	TotalUL   int64     `json:"total_ul"`
	IsOnline  bool      `json:"is_online"`
}

// DeviceConnection — соединение устройства
type DeviceConnection struct {
	ID        string     `json:"id"`
	DeviceID  string     `json:"device_id"`
	DstHost   string     `json:"dst_host"`
	DstPort   int        `json:"dst_port"`
	BytesDL   int64      `json:"bytes_dl"`
	BytesUL   int64      `json:"bytes_ul"`
	StartedAt time.Time  `json:"started_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}

// ActionLog — лог действия
type ActionLog struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Category  string    `json:"category"`
	Action    string    `json:"action"`
	Details   string    `json:"details,omitempty"`
}

// AvailabilityRecord — запись доступности ресурсов
type AvailabilityRecord struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Type           string    `json:"type"`
	TotalResources int       `json:"total_resources"`
	OKResources    int       `json:"ok_resources"`
	Pct            float64   `json:"pct"`
}

// TrafficSnapshot — снапшот трафика
type TrafficSnapshot struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	DLSpeed   float64   `json:"dl_speed"`
	ULSpeed   float64   `json:"ul_speed"`
	TotalDL   int64     `json:"total_dl"`
	TotalUL   int64     `json:"total_ul"`
	ConnCount int       `json:"conn_count"`
}

// ComponentVersion — версия компонента в БД
type ComponentVersion struct {
	ID               string     `json:"id"`
	InstalledVersion string     `json:"installed_version"`
	RemoteVersion    string     `json:"remote_version,omitempty"`
	RemoteSource     string     `json:"remote_source,omitempty"`
	RemoteUpdatedAt  *time.Time `json:"remote_updated_at,omitempty"`
	LocalUpdatedAt   time.Time  `json:"local_updated_at"`
}

// OperatorInfo — информация об операторе/провайдере
type OperatorInfo struct {
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	ISP       string    `json:"isp"`
	ASN       string    `json:"asn"`
	City      string    `json:"city"`
	Org       string    `json:"org"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// OperatorStrategy — привязка стратегии к оператору
type OperatorStrategy struct {
	ID              int        `json:"id"`
	OperatorKey     string     `json:"operator_key"`
	Strategy        string     `json:"strategy"`
	DisplayName     string     `json:"display_name"`
	AvailabilityPct float64    `json:"availability_pct"`
	IsActive        bool       `json:"is_active"`
	UseCount        int        `json:"use_count"`
	LastSource      string     `json:"last_source"`
	AutoTestResults string     `json:"auto_test_results,omitempty"`
	TestedAt        *time.Time `json:"tested_at,omitempty"`
	AppliedAt       *time.Time `json:"applied_at,omitempty"`
}

// ResourceAvailability — per-resource доступность одного ресурса в момент проверки
type ResourceAvailability struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	OperatorKey string    `json:"operator_key"`
	Host        string    `json:"host"`
	Type        string    `json:"type"`
	Ok          bool      `json:"ok"`
	Verdict     string    `json:"verdict"`
	LatencyMs   int       `json:"latency_ms"`
}

// ResourceDaily — агрегированная за день доступность ресурса
type ResourceDaily struct {
	Date         string  `json:"date"`
	OperatorKey  string  `json:"operator_key"`
	Host         string  `json:"host"`
	ChecksTotal  int     `json:"checks_total"`
	ChecksOK     int     `json:"checks_ok"`
	Pct          float64 `json:"pct"`
}

// CurrentOperator — текущий оператор и состояние
type CurrentOperator struct {
	ID                int        `json:"id"`
	OperatorKey       string     `json:"operator_key"`
	OperatorName      string     `json:"operator_name"`
	DetectedAt        *time.Time `json:"detected_at,omitempty"`
	Strategy          string     `json:"strategy,omitempty"`
	ZapretJustUpdated bool       `json:"zapret_just_updated"`
}