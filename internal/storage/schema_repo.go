package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ai-proxy/pkg/models"
)

// SaveSchema stores or updates an inferred schema
func (db *DB) SaveSchema(ctx context.Context, schema *models.InferredSchema) error {
	query := `
		INSERT INTO schemas (host, method, path_pattern, format, content, sample_count, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(host, method, path_pattern, format) DO UPDATE SET
			content = excluded.content,
			sample_count = excluded.sample_count,
			updated_at = excluded.updated_at
	`

	result, err := db.conn.ExecContext(ctx, query,
		schema.Host,
		schema.Method,
		schema.PathPattern,
		schema.Format,
		schema.Content,
		schema.SampleCount,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to save schema: %w", err)
	}

	id, _ := result.LastInsertId()
	schema.ID = id

	return nil
}

// GetSchema retrieves a specific schema
func (db *DB) GetSchema(ctx context.Context, host, method, pathPattern string, format models.SchemaFormat) (*models.InferredSchema, error) {
	query := `
		SELECT id, host, method, path_pattern, format, content, sample_count, updated_at
		FROM schemas
		WHERE host = ? AND method = ? AND path_pattern = ? AND format = ?
	`

	var schema models.InferredSchema
	err := db.conn.QueryRowContext(ctx, query, host, method, pathPattern, format).Scan(
		&schema.ID,
		&schema.Host,
		&schema.Method,
		&schema.PathPattern,
		&schema.Format,
		&schema.Content,
		&schema.SampleCount,
		&schema.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get schema: %w", err)
	}

	return &schema, nil
}

// ListSchemas lists all schemas, optionally filtered by host
func (db *DB) ListSchemas(ctx context.Context, host string) ([]*models.SchemaListItem, error) {
	var args []interface{}
	whereClause := ""
	if host != "" {
		whereClause = "WHERE host = ?"
		args = append(args, host)
	}

	query := fmt.Sprintf(`
		SELECT host, method, path_pattern, sample_count,
			   GROUP_CONCAT(format) as formats
		FROM schemas
		%s
		GROUP BY host, method, path_pattern
		ORDER BY host, path_pattern
	`, whereClause)

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query schemas: %w", err)
	}
	defer rows.Close()

	// Group by host
	hostMap := make(map[string]*models.SchemaListItem)

	for rows.Next() {
		var host, method, pathPattern, formats string
		var sampleCount int

		if err := rows.Scan(&host, &method, &pathPattern, &sampleCount, &formats); err != nil {
			return nil, fmt.Errorf("failed to scan schema: %w", err)
		}

		if _, ok := hostMap[host]; !ok {
			hostMap[host] = &models.SchemaListItem{
				Host:      host,
				Endpoints: []*models.EndpointSummary{},
			}
		}

		hostMap[host].Endpoints = append(hostMap[host].Endpoints, &models.EndpointSummary{
			Method:      method,
			PathPattern: pathPattern,
			SampleCount: sampleCount,
			Formats:     splitFormats(formats),
		})
	}

	var result []*models.SchemaListItem
	for _, item := range hostMap {
		result = append(result, item)
	}

	return result, nil
}

// SaveEndpointPattern stores or updates an endpoint pattern
func (db *DB) SaveEndpointPattern(ctx context.Context, pattern *models.EndpointPattern) error {
	requestSchemaJSON, _ := json.Marshal(pattern.RequestSchema)
	responseSchemaJSON, _ := json.Marshal(pattern.ResponseSchemas)
	queryParamsJSON, _ := json.Marshal(pattern.QueryParams)

	query := `
		INSERT INTO endpoint_patterns (
			host, method, path_pattern, path_regex, request_schema,
			response_schemas, query_params, auth_type, sample_count, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(host, method, path_pattern) DO UPDATE SET
			path_regex = excluded.path_regex,
			request_schema = excluded.request_schema,
			response_schemas = excluded.response_schemas,
			query_params = excluded.query_params,
			auth_type = excluded.auth_type,
			sample_count = excluded.sample_count,
			updated_at = excluded.updated_at
	`

	result, err := db.conn.ExecContext(ctx, query,
		pattern.Host,
		pattern.Method,
		pattern.PathPattern,
		pattern.PathRegex,
		string(requestSchemaJSON),
		string(responseSchemaJSON),
		string(queryParamsJSON),
		pattern.AuthType,
		pattern.SampleCount,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to save endpoint pattern: %w", err)
	}

	id, _ := result.LastInsertId()
	pattern.ID = id

	return nil
}

// GetEndpointPattern retrieves a specific endpoint pattern
func (db *DB) GetEndpointPattern(ctx context.Context, host, method, pathPattern string) (*models.EndpointPattern, error) {
	query := `
		SELECT id, host, method, path_pattern, path_regex, request_schema,
			   response_schemas, query_params, auth_type, sample_count,
			   created_at, updated_at
		FROM endpoint_patterns
		WHERE host = ? AND method = ? AND path_pattern = ?
	`

	var pattern models.EndpointPattern
	var requestSchema, responseSchemas, queryParams sql.NullString
	var authType sql.NullString

	err := db.conn.QueryRowContext(ctx, query, host, method, pathPattern).Scan(
		&pattern.ID,
		&pattern.Host,
		&pattern.Method,
		&pattern.PathPattern,
		&pattern.PathRegex,
		&requestSchema,
		&responseSchemas,
		&queryParams,
		&authType,
		&pattern.SampleCount,
		&pattern.CreatedAt,
		&pattern.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint pattern: %w", err)
	}

	if requestSchema.Valid {
		json.Unmarshal([]byte(requestSchema.String), &pattern.RequestSchema)
	}
	if responseSchemas.Valid {
		json.Unmarshal([]byte(responseSchemas.String), &pattern.ResponseSchemas)
	}
	if queryParams.Valid {
		json.Unmarshal([]byte(queryParams.String), &pattern.QueryParams)
	}
	pattern.AuthType = authType.String

	return &pattern, nil
}

// ListEndpointPatterns lists all endpoint patterns for a host
func (db *DB) ListEndpointPatterns(ctx context.Context, host string) ([]*models.EndpointPattern, error) {
	query := `
		SELECT id, host, method, path_pattern, path_regex, request_schema,
			   response_schemas, query_params, auth_type, sample_count,
			   created_at, updated_at
		FROM endpoint_patterns
		WHERE host = ?
		ORDER BY path_pattern
	`

	rows, err := db.conn.QueryContext(ctx, query, host)
	if err != nil {
		return nil, fmt.Errorf("failed to query endpoint patterns: %w", err)
	}
	defer rows.Close()

	var patterns []*models.EndpointPattern
	for rows.Next() {
		var pattern models.EndpointPattern
		var requestSchema, responseSchemas, queryParams sql.NullString
		var authType sql.NullString

		if err := rows.Scan(
			&pattern.ID,
			&pattern.Host,
			&pattern.Method,
			&pattern.PathPattern,
			&pattern.PathRegex,
			&requestSchema,
			&responseSchemas,
			&queryParams,
			&authType,
			&pattern.SampleCount,
			&pattern.CreatedAt,
			&pattern.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan endpoint pattern: %w", err)
		}

		if requestSchema.Valid {
			json.Unmarshal([]byte(requestSchema.String), &pattern.RequestSchema)
		}
		if responseSchemas.Valid {
			json.Unmarshal([]byte(responseSchemas.String), &pattern.ResponseSchemas)
		}
		if queryParams.Valid {
			json.Unmarshal([]byte(queryParams.String), &pattern.QueryParams)
		}
		pattern.AuthType = authType.String

		patterns = append(patterns, &pattern)
	}

	return patterns, nil
}

// SaveInferredSchema is a convenience method for saving inferred schemas
func (db *DB) SaveInferredSchema(ctx context.Context, host, method, pathPattern, format, content string, sampleCount int) error {
	schema := &models.InferredSchema{
		Host:        host,
		Method:      method,
		PathPattern: pathPattern,
		Format:      models.SchemaFormat(format),
		Content:     content,
		SampleCount: sampleCount,
	}
	return db.SaveSchema(ctx, schema)
}

// GetSchemasByFormat retrieves schemas for a host filtered by format
func (db *DB) GetSchemasByFormat(ctx context.Context, host, format string) ([]*models.InferredSchema, error) {
	query := `
		SELECT id, host, method, path_pattern, format, content, sample_count, updated_at
		FROM schemas
		WHERE host = ? AND format = ?
	`

	rows, err := db.conn.QueryContext(ctx, query, host, format)
	if err != nil {
		return nil, fmt.Errorf("failed to query schemas: %w", err)
	}
	defer rows.Close()

	var schemas []*models.InferredSchema
	for rows.Next() {
		var s models.InferredSchema
		if err := rows.Scan(&s.ID, &s.Host, &s.Method, &s.PathPattern, &s.Format,
			&s.Content, &s.SampleCount, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan schema: %w", err)
		}
		schemas = append(schemas, &s)
	}

	return schemas, nil
}

// splitFormats splits comma-separated format string
func splitFormats(formats string) []string {
	if formats == "" {
		return nil
	}
	result := []string{}
	for _, f := range splitString(formats, ',') {
		if f != "" {
			result = append(result, f)
		}
	}
	return result
}

// splitString splits a string by separator
func splitString(s string, sep rune) []string {
	var result []string
	var current []rune
	for _, c := range s {
		if c == sep {
			result = append(result, string(current))
			current = nil
		} else {
			current = append(current, c)
		}
	}
	if len(current) > 0 {
		result = append(result, string(current))
	}
	return result
}
