package storage

import (
	"context"
	"database/sql"
	"fmt"

	"ai-proxy/pkg/models"
)

// SaveWebSocketMessage stores a WebSocket message
func (db *DB) SaveWebSocketMessage(ctx context.Context, msg *models.WebSocketMessage) error {
	query := `
		INSERT INTO websocket_messages (
			request_id, uuid, direction, message_type, payload,
			payload_size, sequence_num, captured_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.ExecContext(ctx, query,
		msg.RequestID,
		msg.UUID,
		msg.Direction,
		msg.MessageType,
		msg.Payload,
		msg.PayloadSize,
		msg.SequenceNum,
		msg.CapturedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert websocket message: %w", err)
	}

	id, _ := result.LastInsertId()
	msg.ID = id

	return nil
}

// SaveWebSocketMessageForRequest stores a message linked to a request UUID
func (db *DB) SaveWebSocketMessageForRequest(ctx context.Context, requestUUID string, msg *models.WebSocketMessage) error {
	// Get the request ID
	var requestID int64
	if err := db.conn.QueryRowContext(ctx, "SELECT id FROM requests WHERE uuid = ?", requestUUID).Scan(&requestID); err != nil {
		return fmt.Errorf("failed to find request: %w", err)
	}

	msg.RequestID = requestID
	return db.SaveWebSocketMessage(ctx, msg)
}

// GetWebSocketMessages retrieves WebSocket messages for a request
func (db *DB) GetWebSocketMessages(ctx context.Context, requestUUID string, filter *WSFilter) ([]*models.WebSocketMessage, int64, error) {
	if filter == nil {
		filter = DefaultWSFilter()
	}

	// Build WHERE clause
	conditions := []string{"r.uuid = ?"}
	args := []interface{}{requestUUID}

	if filter.Direction != "" && filter.Direction != "both" {
		conditions = append(conditions, "ws.direction = ?")
		args = append(args, filter.Direction)
	}
	if filter.MessageType != "" && filter.MessageType != "all" {
		conditions = append(conditions, "ws.message_type = ?")
		args = append(args, filter.MessageType)
	}
	if filter.FromSequence > 0 {
		conditions = append(conditions, "ws.sequence_num >= ?")
		args = append(args, filter.FromSequence)
	}

	whereClause := "WHERE " + joinConditions(conditions)

	// Count total
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM websocket_messages ws
		JOIN requests r ON ws.request_id = r.id
		%s
	`, whereClause)

	var total int64
	if err := db.conn.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count messages: %w", err)
	}

	// Query messages
	query := fmt.Sprintf(`
		SELECT ws.id, ws.request_id, ws.uuid, ws.direction, ws.message_type,
			   ws.payload, ws.payload_size, ws.sequence_num, ws.captured_at
		FROM websocket_messages ws
		JOIN requests r ON ws.request_id = r.id
		%s
		ORDER BY ws.sequence_num ASC
		LIMIT ?
	`, whereClause)

	args = append(args, filter.Limit)

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []*models.WebSocketMessage
	for rows.Next() {
		var msg models.WebSocketMessage
		if err := rows.Scan(
			&msg.ID,
			&msg.RequestID,
			&msg.UUID,
			&msg.Direction,
			&msg.MessageType,
			&msg.Payload,
			&msg.PayloadSize,
			&msg.SequenceNum,
			&msg.CapturedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, &msg)
	}

	return messages, total, nil
}

// ListWebSocketConnections lists all WebSocket connections
func (db *DB) ListWebSocketConnections(ctx context.Context) ([]*models.WebSocketConnection, error) {
	query := `
		SELECT r.uuid, r.url, r.captured_at,
			   COUNT(ws.id) as msg_count,
			   MAX(ws.captured_at) as last_msg_at
		FROM requests r
		JOIN websocket_messages ws ON r.id = ws.request_id
		GROUP BY r.id
		ORDER BY r.captured_at DESC
	`

	rows, err := db.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query connections: %w", err)
	}
	defer rows.Close()

	var connections []*models.WebSocketConnection
	for rows.Next() {
		var conn models.WebSocketConnection
		var lastMsgAt sql.NullTime

		if err := rows.Scan(
			&conn.RequestID,
			&conn.URL,
			&conn.StartedAt,
			&conn.MessageCount,
			&lastMsgAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan connection: %w", err)
		}

		if lastMsgAt.Valid {
			conn.EndedAt = &lastMsgAt.Time
		}

		connections = append(connections, &conn)
	}

	return connections, nil
}

// GetNextSequenceNum returns the next sequence number for a WebSocket connection
func (db *DB) GetNextSequenceNum(ctx context.Context, requestID int64) (int, error) {
	var maxSeq sql.NullInt64
	err := db.conn.QueryRowContext(ctx,
		"SELECT MAX(sequence_num) FROM websocket_messages WHERE request_id = ?",
		requestID,
	).Scan(&maxSeq)
	if err != nil {
		return 1, nil
	}
	if !maxSeq.Valid {
		return 1, nil
	}
	return int(maxSeq.Int64) + 1, nil
}

// helper function
func joinConditions(conditions []string) string {
	result := ""
	for i, c := range conditions {
		if i > 0 {
			result += " AND "
		}
		result += c
	}
	return result
}
