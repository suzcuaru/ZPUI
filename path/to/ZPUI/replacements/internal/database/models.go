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

// ErrorLog — structured error record in DB
type ErrorLog struct {
        ID          string    `json:"id"`
        Timestamp   time.Time `json:"timestamp"`
        Level       string    `json:"level"`
        Category    string    `json:"category"`
        Message     string    `json:"message"`
        ContextJSON string    `json:"context_json,omitempty"`
}

// DiagnosticReport — saved diagnostic report
type DiagnosticReport struct {
        ID          string     `json:"id"`
        GeneratedAt time.Time  `json:"generated_at"`
        PeriodStart *time.Time `json:"period_start,omitempty"`
        PeriodEnd   *time.Time `json:"period_end,omitempty"`
        Frequency   string     `json:"frequency"`
        Content     string     `json:"content"`
        Uploaded    bool       `json:"uploaded"`
        UploadedAt  *time.Time `json:"uploaded_at,omitempty"`
}
