package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"ai-proxy/pkg/models"
)

// SaveResponse stores an HTTP response
func (db *DB) SaveResponse(ctx context.Context, resp *models.HTTPResponse) error {
	headersJSON, err := json.Marshal(resp.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	query := `
		INSERT INTO responses (
			request_id, uuid, status_code, status_text, headers, body,
			body_size, content_type, latency_ms, captured_at
		) VALUES (
			(SELECT id FROM requests WHERE uuid = ?),
			?, ?, ?, ?, ?, ?, ?, ?, ?
		)
	`

	// Get request UUID from request_id if needed
	var requestUUID string
	if resp.RequestID > 0 {
		db.conn.QueryRowContext(ctx, "SELECT uuid FROM requests WHERE id = ?", resp.RequestID).Scan(&requestUUID)
	}

	result, err := db.conn.ExecContext(ctx, query,
		requestUUID,
		resp.UUID,
		resp.StatusCode,
		resp.StatusText,
		string(headersJSON),
		resp.Body,
		resp.BodySize,
		resp.ContentType,
		resp.LatencyMs,
		resp.CapturedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert response: %w", err)
	}

	id, _ := result.LastInsertId()
	resp.ID = id

	return nil
}

// SaveResponseForRequest stores a response linked to a request UUID
func (db *DB) SaveResponseForRequest(ctx context.Context, requestUUID string, resp *models.HTTPResponse) error {
	headersJSON, err := json.Marshal(resp.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	query := `
		INSERT INTO responses (
			request_id, uuid, status_code, status_text, headers, body,
			body_size, content_type, latency_ms, captured_at
		) VALUES (
			(SELECT id FROM requests WHERE uuid = ?),
			?, ?, ?, ?, ?, ?, ?, ?, ?
		)
	`

	result, err := db.conn.ExecContext(ctx, query,
		requestUUID,
		resp.UUID,
		resp.StatusCode,
		resp.StatusText,
		string(headersJSON),
		resp.Body,
		resp.BodySize,
		resp.ContentType,
		resp.LatencyMs,
		resp.CapturedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert response: %w", err)
	}

	id, _ := result.LastInsertId()
	resp.ID = id

	return nil
}

// GetResponse retrieves a response by request UUID
func (db *DB) GetResponse(ctx context.Context, requestUUID string) (*models.HTTPResponse, error) {
	query := `
		SELECT resp.id, resp.request_id, resp.uuid, resp.status_code, resp.status_text,
			   resp.headers, resp.body, resp.body_size, resp.content_type,
			   resp.latency_ms, resp.captured_at
		FROM responses resp
		JOIN requests r ON resp.request_id = r.id
		WHERE r.uuid = ?
	`

	var resp models.HTTPResponse
	var headersJSON sql.NullString
	var statusText, contentType sql.NullString

	err := db.conn.QueryRowContext(ctx, query, requestUUID).Scan(
		&resp.ID,
		&resp.RequestID,
		&resp.UUID,
		&resp.StatusCode,
		&statusText,
		&headersJSON,
		&resp.Body,
		&resp.BodySize,
		&contentType,
		&resp.LatencyMs,
		&resp.CapturedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get response: %w", err)
	}

	resp.StatusText = statusText.String
	resp.ContentType = contentType.String

	if headersJSON.Valid {
		json.Unmarshal([]byte(headersJSON.String), &resp.Headers)
	}

	return &resp, nil
}

// GetCapture retrieves a complete request/response pair
func (db *DB) GetCapture(ctx context.Context, uuid string) (*models.Capture, error) {
	req, err := db.GetRequest(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, nil
	}

	resp, err := db.GetResponse(ctx, uuid)
	if err != nil {
		return nil, err
	}

	return &models.Capture{
		Request:  req,
		Response: resp,
	}, nil
}
