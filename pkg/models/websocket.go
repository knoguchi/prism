package models

import (
	"time"
)

// WSDirection represents the direction of a WebSocket message
type WSDirection string

const (
	WSDirectionClientToServer WSDirection = "c2s"
	WSDirectionServerToClient WSDirection = "s2c"
)

// WSMessageType represents the type of WebSocket message
type WSMessageType string

const (
	WSMessageTypeText   WSMessageType = "text"
	WSMessageTypeBinary WSMessageType = "binary"
	WSMessageTypePing   WSMessageType = "ping"
	WSMessageTypePong   WSMessageType = "pong"
	WSMessageTypeClose  WSMessageType = "close"
)

// WebSocketMessage represents a captured WebSocket message
type WebSocketMessage struct {
	ID          int64         `json:"id"`
	RequestID   int64         `json:"request_id"`
	UUID        string        `json:"uuid"`
	Direction   WSDirection   `json:"direction"`
	MessageType WSMessageType `json:"message_type"`
	Payload     []byte        `json:"payload,omitempty"`
	PayloadSize int64         `json:"payload_size"`
	SequenceNum int           `json:"sequence_num"`
	CapturedAt  time.Time     `json:"captured_at"`
}

// WebSocketConnection represents a WebSocket connection (upgrade request + messages)
type WebSocketConnection struct {
	RequestID    string              `json:"request_id"`
	URL          string              `json:"url"`
	MessageCount int                 `json:"message_count"`
	StartedAt    time.Time           `json:"started_at"`
	EndedAt      *time.Time          `json:"ended_at,omitempty"`
	CloseCode    int                 `json:"close_code,omitempty"`
	CloseReason  string              `json:"close_reason,omitempty"`
	Messages     []*WebSocketMessage `json:"messages,omitempty"`
}
