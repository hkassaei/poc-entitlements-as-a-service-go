package db

import (
	"encoding/json"
	"time"
)

// Subscriber represents a row in the subscribers table.
type Subscriber struct {
	ID           string    `json:"id"`
	IMSI         string    `json:"imsi"`
	MSISDN       *string   `json:"msisdn"`
	ICCID        *string   `json:"iccid"`
	EID          *string   `json:"eid"`
	KiEncrypted  []byte    `json:"-"`
	OpEncrypted  []byte    `json:"-"`
	KiDEKWrapped []byte    `json:"-"`
	SQN          int64     `json:"sqn"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Device represents a row in the devices table.
type Device struct {
	ID           string    `json:"id"`
	SubscriberID *string   `json:"subscriber_id"`
	IMEI         string    `json:"imei"`
	Vendor       *string   `json:"vendor"`
	Model        *string   `json:"model"`
	SWVersion    *string   `json:"sw_version"`
	DeviceType   *string   `json:"device_type"`
	EID          *string   `json:"eid"`
	CreatedAt    time.Time `json:"created_at"`
}

// Entitlement represents a row in the entitlements table.
type Entitlement struct {
	ID           string          `json:"id"`
	SubscriberID string          `json:"subscriber_id"`
	AppID        string          `json:"app_id"`
	Status       int             `json:"status"`
	ProvStatus   int             `json:"prov_status"`
	TcStatus     int             `json:"tc_status"`
	ConfigData   json.RawMessage `json:"config_data"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// Token represents a row in the tokens table.
type Token struct {
	ID               string    `json:"id"`
	SubscriberID     string    `json:"subscriber_id"`
	TokenValue       string    `json:"token_value"`
	TokenType        string    `json:"token_type"`
	IssuedAt         time.Time `json:"issued_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	Scope            *string   `json:"scope"`
	OperationTargets []string  `json:"operation_targets"`
	Consumed         bool      `json:"consumed"`
	CreatedByIP      *string   `json:"created_by_ip"`
}

// AuditLog represents a row in the audit_log table.
type AuditLog struct {
	ID             int64           `json:"id"`
	Timestamp      time.Time       `json:"timestamp"`
	SubscriberID   *string         `json:"subscriber_id"`
	AppID          *string         `json:"app_id"`
	Operation      *string         `json:"operation"`
	RequestSummary json.RawMessage `json:"request_summary"`
	ResponseCode   *int            `json:"response_code"`
	ClientIP       *string         `json:"client_ip"`
	UserAgent      *string         `json:"user_agent"`
}
