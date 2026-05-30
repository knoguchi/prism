package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"prism/pkg/models"
)

// SaveRequest stores an HTTP request
func (db *DB) SaveRequest(ctx context.Context, req *models.HTTPRequest) error {
	headersJSON, err := json.Marshal(req.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	var tagsJSON []byte
	if len(req.Tags) > 0 {
		tagsJSON, _ = json.Marshal(req.Tags)
	}

	query := `
		INSERT INTO requests (
			uuid, method, url, host, path, query_string, headers, body, body_size,
			content_type, protocol, is_https, remote_addr, captured_at, tags, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.ExecContext(ctx, query,
		req.UUID,
		req.Method,
		req.URL,
		req.Host,
		req.Path,
		req.QueryString,
		string(headersJSON),
		req.Body,
		req.BodySize,
		req.ContentType,
		req.Protocol,
		req.IsHTTPS,
		req.RemoteAddr,
		req.CapturedAt,
		string(tagsJSON),
		req.Notes,
	)
	if err != nil {
		return fmt.Errorf("failed to insert request: %w", err)
	}

	id, _ := result.LastInsertId()
	req.ID = id

	// Update FTS index
	_, err = db.conn.ExecContext(ctx, `
		INSERT INTO requests_fts(rowid, uuid, url, headers, body)
		VALUES (?, ?, ?, ?, ?)
	`, id, req.UUID, req.URL, string(headersJSON), string(req.Body))
	if err != nil {
		// Log but don't fail - FTS is non-critical
	}

	return nil
}

// GetRequest retrieves a request by UUID
func (db *DB) GetRequest(ctx context.Context, uuid string) (*models.HTTPRequest, error) {
	query := `
		SELECT id, uuid, method, url, host, path, query_string, headers, body, body_size,
			   content_type, protocol, is_https, remote_addr, captured_at, tags, notes
		FROM requests WHERE uuid = ?
	`

	var req models.HTTPRequest
	var headersJSON, tagsJSON sql.NullString
	var queryString, contentType, remoteAddr, notes sql.NullString

	err := db.conn.QueryRowContext(ctx, query, uuid).Scan(
		&req.ID,
		&req.UUID,
		&req.Method,
		&req.URL,
		&req.Host,
		&req.Path,
		&queryString,
		&headersJSON,
		&req.Body,
		&req.BodySize,
		&contentType,
		&req.Protocol,
		&req.IsHTTPS,
		&remoteAddr,
		&req.CapturedAt,
		&tagsJSON,
		&notes,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get request: %w", err)
	}

	req.QueryString = queryString.String
	req.ContentType = contentType.String
	req.RemoteAddr = remoteAddr.String
	req.Notes = notes.String

	if headersJSON.Valid {
		json.Unmarshal([]byte(headersJSON.String), &req.Headers)
	}
	if tagsJSON.Valid {
		json.Unmarshal([]byte(tagsJSON.String), &req.Tags)
	}

	return &req, nil
}

// ListRequests lists requests with filtering and pagination
func (db *DB) ListRequests(ctx context.Context, filter *RequestFilter) ([]*models.CaptureListItem, int64, error) {
	if filter == nil {
		filter = DefaultRequestFilter()
	}

	// Build common conditions (excluding host filter)
	var commonConditions []string
	var commonArgs []interface{}

	if filter.Method != "" {
		commonConditions = append(commonConditions, "r.method = ?")
		commonArgs = append(commonArgs, filter.Method)
	}
	if filter.Path != "" {
		if strings.Contains(filter.Path, "*") {
			commonConditions = append(commonConditions, "r.path LIKE ?")
			commonArgs = append(commonArgs, strings.ReplaceAll(filter.Path, "*", "%"))
		} else {
			commonConditions = append(commonConditions, "r.path = ?")
			commonArgs = append(commonArgs, filter.Path)
		}
	}
	if filter.ContentType != "" {
		commonConditions = append(commonConditions, "(r.content_type LIKE ? OR resp.content_type LIKE ?)")
		commonArgs = append(commonArgs, filter.ContentType+"%", filter.ContentType+"%")
	}
	if filter.Status != "" {
		statusCond, statusArgs := buildStatusCondition(filter.Status)
		if statusCond != "" {
			commonConditions = append(commonConditions, statusCond)
			commonArgs = append(commonArgs, statusArgs...)
		}
	}
	if filter.From != nil {
		commonConditions = append(commonConditions, "r.captured_at >= ?")
		commonArgs = append(commonArgs, *filter.From)
	}
	if filter.To != nil {
		commonConditions = append(commonConditions, "r.captured_at <= ?")
		commonArgs = append(commonArgs, *filter.To)
	}

	// Build ORDER BY
	orderBy := "captured_at DESC"
	if filter.Sort != "" {
		dir := "DESC"
		if strings.ToLower(filter.Order) == "asc" {
			dir = "ASC"
		}
		orderBy = fmt.Sprintf("%s %s", filter.Sort, dir)
	}

	var query string
	var args []interface{}
	var total int64 = 0

	// UNION query: selected hosts (accumulated) + non-selected hosts (ephemeral)
	if len(filter.Hosts) > 0 && filter.LimitPerPath > 0 && filter.EphemeralLimit > 0 {
		// Build host placeholders for selected hosts
		hostPlaceholders := make([]string, len(filter.Hosts))
		for i := range filter.Hosts {
			hostPlaceholders[i] = "?"
		}
		hostsIN := strings.Join(hostPlaceholders, ",")

		// Build WHERE for selected hosts
		selectedWhere := "r.host IN (" + hostsIN + ")"
		if len(commonConditions) > 0 {
			selectedWhere = selectedWhere + " AND " + strings.Join(commonConditions, " AND ")
		}

		// Build WHERE for non-selected hosts
		ephemeralWhere := "r.host NOT IN (" + hostsIN + ")"
		if len(commonConditions) > 0 {
			ephemeralWhere = ephemeralWhere + " AND " + strings.Join(commonConditions, " AND ")
		}

		query = fmt.Sprintf(`
			WITH
			-- Part 1: Accumulated traffic from selected hosts (max N per path)
			selected_ranked AS (
				SELECT r.uuid, r.method, r.url, r.host, r.path,
					   COALESCE(resp.status_code, 0) as status_code,
					   COALESCE(resp.content_type, r.content_type) as content_type,
					   r.body_size, COALESCE(resp.body_size, 0) as resp_body_size,
					   COALESCE(resp.latency_ms, 0) as latency_ms,
					   r.captured_at,
					   ROW_NUMBER() OVER (
						   PARTITION BY r.method, r.is_https, r.host, r.path, COALESCE(r.query_string, '')
						   ORDER BY r.captured_at DESC
					   ) as row_num
				FROM requests r
				LEFT JOIN responses resp ON r.id = resp.request_id
				WHERE %s
			),
			selected AS (
				SELECT uuid, method, url, host, path, status_code, content_type,
					   body_size, resp_body_size, latency_ms, captured_at
				FROM selected_ranked WHERE row_num <= ?
			),
			-- Part 2: Ephemeral traffic from non-selected hosts (limited)
			ephemeral AS (
				SELECT r.uuid, r.method, r.url, r.host, r.path,
					   COALESCE(resp.status_code, 0) as status_code,
					   COALESCE(resp.content_type, r.content_type) as content_type,
					   r.body_size, COALESCE(resp.body_size, 0) as resp_body_size,
					   COALESCE(resp.latency_ms, 0) as latency_ms,
					   r.captured_at
				FROM requests r
				LEFT JOIN responses resp ON r.id = resp.request_id
				WHERE %s
				ORDER BY r.captured_at DESC
				LIMIT ?
			)
			SELECT * FROM selected
			UNION ALL
			SELECT * FROM ephemeral
			ORDER BY %s
		`, selectedWhere, ephemeralWhere, orderBy)

		// Add args: selected hosts, common args, limit_per_path, selected hosts again, common args again, ephemeral_limit
		for _, h := range filter.Hosts {
			args = append(args, h)
		}
		args = append(args, commonArgs...)
		args = append(args, filter.LimitPerPath)
		for _, h := range filter.Hosts {
			args = append(args, h)
		}
		args = append(args, commonArgs...)
		args = append(args, filter.EphemeralLimit)

	} else if len(filter.Hosts) > 0 && filter.LimitPerPath > 0 {
		// Only selected hosts with limit per path (original behavior)
		hostPlaceholders := make([]string, len(filter.Hosts))
		for i := range filter.Hosts {
			hostPlaceholders[i] = "?"
		}
		selectedWhere := "r.host IN (" + strings.Join(hostPlaceholders, ",") + ")"
		if len(commonConditions) > 0 {
			selectedWhere = selectedWhere + " AND " + strings.Join(commonConditions, " AND ")
		}

		query = fmt.Sprintf(`
			WITH ranked AS (
				SELECT r.uuid, r.method, r.url, r.host, r.path,
					   COALESCE(resp.status_code, 0) as status_code,
					   COALESCE(resp.content_type, r.content_type) as content_type,
					   r.body_size, COALESCE(resp.body_size, 0) as resp_body_size,
					   COALESCE(resp.latency_ms, 0) as latency_ms,
					   r.captured_at,
					   ROW_NUMBER() OVER (
						   PARTITION BY r.method, r.is_https, r.host, r.path, COALESCE(r.query_string, '')
						   ORDER BY r.captured_at DESC
					   ) as row_num
				FROM requests r
				LEFT JOIN responses resp ON r.id = resp.request_id
				WHERE %s
			)
			SELECT uuid, method, url, host, path, status_code, content_type,
				   body_size, resp_body_size, latency_ms, captured_at
			FROM ranked
			WHERE row_num <= ?
			ORDER BY %s
		`, selectedWhere, orderBy)

		for _, h := range filter.Hosts {
			args = append(args, h)
		}
		args = append(args, commonArgs...)
		args = append(args, filter.LimitPerPath)

	} else {
		// Simple query with limit (original behavior)
		whereClause := ""
		if filter.Host != "" {
			if strings.Contains(filter.Host, "*") {
				commonConditions = append([]string{"r.host LIKE ?"}, commonConditions...)
				args = append([]interface{}{strings.ReplaceAll(filter.Host, "*", "%")}, commonArgs...)
			} else {
				commonConditions = append([]string{"r.host = ?"}, commonConditions...)
				args = append([]interface{}{filter.Host}, commonArgs...)
			}
		} else {
			args = commonArgs
		}
		if len(commonConditions) > 0 {
			whereClause = "WHERE " + strings.Join(commonConditions, " AND ")
		}

		offset := (filter.Page - 1) * filter.Limit
		if offset < 0 {
			offset = 0
		}

		query = fmt.Sprintf(`
			SELECT r.uuid, r.method, r.url, r.host, r.path,
				   COALESCE(resp.status_code, 0), COALESCE(resp.content_type, r.content_type),
				   r.body_size, COALESCE(resp.body_size, 0), COALESCE(resp.latency_ms, 0),
				   r.captured_at
			FROM requests r
			LEFT JOIN responses resp ON r.id = resp.request_id
			%s
			ORDER BY r.%s
			LIMIT ? OFFSET ?
		`, whereClause, orderBy)
		args = append(args, filter.Limit, offset)
	}

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query requests: %w", err)
	}
	defer rows.Close()

	var items []*models.CaptureListItem
	for rows.Next() {
		var item models.CaptureListItem
		var contentType sql.NullString

		if err := rows.Scan(
			&item.ID,
			&item.Method,
			&item.URL,
			&item.Host,
			&item.Path,
			&item.StatusCode,
			&contentType,
			&item.RequestSize,
			&item.ResponseSize,
			&item.LatencyMs,
			&item.CapturedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan request: %w", err)
		}

		item.ContentType = contentType.String
		items = append(items, &item)
	}

	return items, total, nil
}

// ListEndpointUsage returns method/path usage summaries for a host.
func (db *DB) ListEndpointUsage(ctx context.Context, host, method, pathPrefix string, limit int) ([]*models.EndpointUsage, error) {
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if limit <= 0 {
		limit = 100
	}

	var conditions []string
	var args []interface{}

	conditions = append(conditions, "r.host = ?")
	args = append(args, host)

	if method != "" {
		conditions = append(conditions, "r.method = ?")
		args = append(args, method)
	}
	if pathPrefix != "" {
		conditions = append(conditions, "r.path LIKE ?")
		args = append(args, pathPrefix+"%")
	}

	where := strings.Join(conditions, " AND ")

	query := fmt.Sprintf(`
		SELECT r.method, r.path, COUNT(*) as sample_count, MAX(r.captured_at) as last_seen
		FROM requests r
		WHERE %s
		GROUP BY r.method, r.path
		ORDER BY sample_count DESC, last_seen DESC
		LIMIT ?
	`, where)

	args = append(args, limit)

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoint usage: %w", err)
	}
	defer rows.Close()

	var results []*models.EndpointUsage
	for rows.Next() {
		var item models.EndpointUsage
		var lastSeenRaw sql.NullString
		if err := rows.Scan(&item.Method, &item.Path, &item.SampleCount, &lastSeenRaw); err != nil {
			return nil, fmt.Errorf("failed to scan endpoint usage: %w", err)
		}
		if lastSeenRaw.Valid && lastSeenRaw.String != "" {
			parsed, err := time.Parse(time.RFC3339Nano, lastSeenRaw.String)
			if err != nil {
				parsed, err = time.Parse("2006-01-02 15:04:05", lastSeenRaw.String)
				if err != nil {
					parsed = time.Time{}
				}
			}
			item.LastSeen = parsed
		}
		results = append(results, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read endpoint usage rows: %w", err)
	}

	return results, nil
}

// DeleteRequest deletes a request by UUID
func (db *DB) DeleteRequest(ctx context.Context, uuid string) error {
	// Get the request ID for FTS cleanup
	var id int64
	if err := db.conn.QueryRowContext(ctx, "SELECT id FROM requests WHERE uuid = ?", uuid).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("failed to find request: %w", err)
	}

	// Delete from FTS
	db.conn.ExecContext(ctx, "DELETE FROM requests_fts WHERE rowid = ?", id)

	// Delete request (cascade will handle responses and WS messages)
	_, err := db.conn.ExecContext(ctx, "DELETE FROM requests WHERE uuid = ?", uuid)
	return err
}

// DeleteRequests bulk deletes requests matching filter
func (db *DB) DeleteRequests(ctx context.Context, filter *RequestFilter) (int64, error) {
	// For simplicity, get matching IDs first
	items, _, err := db.ListRequests(ctx, filter)
	if err != nil {
		return 0, err
	}

	if len(items) == 0 {
		return 0, nil
	}

	var deleted int64
	for _, item := range items {
		if err := db.DeleteRequest(ctx, item.ID); err != nil {
			return deleted, err
		}
		deleted++
	}

	return deleted, nil
}

// SearchRequests performs full-text search
func (db *DB) SearchRequests(ctx context.Context, query string, opts *SearchOptions) ([]*models.CaptureListItem, int64, error) {
	if opts == nil {
		opts = &SearchOptions{Limit: 50}
	}
	if opts.Limit == 0 {
		opts.Limit = 50
	}

	// Build FTS query
	ftsQuery := fmt.Sprintf(`
		SELECT r.uuid, r.method, r.url, r.host, r.path,
			   COALESCE(resp.status_code, 0), COALESCE(resp.content_type, r.content_type),
			   r.body_size, COALESCE(resp.body_size, 0), COALESCE(resp.latency_ms, 0),
			   r.captured_at
		FROM requests r
		LEFT JOIN responses resp ON r.id = resp.request_id
		WHERE r.id IN (
			SELECT rowid FROM requests_fts WHERE requests_fts MATCH ?
		)
		ORDER BY r.captured_at DESC
		LIMIT ?
	`)

	// Escape and format query for FTS5
	ftsSearchQuery := formatFTSQuery(query)

	rows, err := db.conn.QueryContext(ctx, ftsQuery, ftsSearchQuery, opts.Limit)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to search: %w", err)
	}
	defer rows.Close()

	var items []*models.CaptureListItem
	for rows.Next() {
		var item models.CaptureListItem
		var contentType sql.NullString

		if err := rows.Scan(
			&item.ID,
			&item.Method,
			&item.URL,
			&item.Host,
			&item.Path,
			&item.StatusCode,
			&contentType,
			&item.RequestSize,
			&item.ResponseSize,
			&item.LatencyMs,
			&item.CapturedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan result: %w", err)
		}

		item.ContentType = contentType.String
		items = append(items, &item)
	}

	return items, int64(len(items)), nil
}

// buildStatusCondition builds SQL condition for status filter
func buildStatusCondition(status string) (string, []interface{}) {
	status = strings.ToLower(status)

	// Handle patterns like "2xx", "4xx", "5xx"
	if strings.HasSuffix(status, "xx") {
		prefix := status[:1]
		return "resp.status_code >= ? AND resp.status_code < ?",
			[]interface{}{prefix + "00", prefix + "99"}
	}

	// Handle range like "500-599"
	if strings.Contains(status, "-") {
		parts := strings.Split(status, "-")
		if len(parts) == 2 {
			from, _ := strconv.Atoi(parts[0])
			to, _ := strconv.Atoi(parts[1])
			return "resp.status_code >= ? AND resp.status_code <= ?",
				[]interface{}{from, to}
		}
	}

	// Exact match
	code, err := strconv.Atoi(status)
	if err == nil {
		return "resp.status_code = ?", []interface{}{code}
	}

	return "", nil
}

// formatFTSQuery formats a search query for FTS5
func formatFTSQuery(query string) string {
	// Simple escaping - wrap in quotes if contains special chars
	if strings.ContainsAny(query, `"'(){}[]^*+-`) {
		return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	}
	return query
}

// PruneRequests cleans up old requests based on retention policy.
// For enabled hosts: keeps up to maxPerPathEnabled per (host, method, path) combination.
// For non-enabled hosts: keeps only the most recent ephemeralLimit records total.
//
// FIXME TODO: Not yet wired up. Call via proxy.Server.StartPruner() from main.
func (db *DB) PruneRequests(ctx context.Context, enabledHosts []string, maxPerPathEnabled, ephemeralLimit int) (int64, error) {
	if maxPerPathEnabled <= 0 {
		maxPerPathEnabled = 100 // Default: keep 100 per path for enabled hosts
	}
	if ephemeralLimit <= 0 {
		ephemeralLimit = 1000 // Default: keep 1000 most recent for non-enabled hosts
	}

	var totalDeleted int64

	// Part 1: Prune enabled hosts - keep max N per (host, method, path)
	if len(enabledHosts) > 0 {
		// Build placeholders for enabled hosts
		placeholders := make([]string, len(enabledHosts))
		args := make([]interface{}, len(enabledHosts))
		for i, h := range enabledHosts {
			placeholders[i] = "?"
			args[i] = h
		}
		hostList := strings.Join(placeholders, ", ")

		// Find requests to delete: those beyond the max per path
		query := fmt.Sprintf(`
			WITH ranked AS (
				SELECT id,
					ROW_NUMBER() OVER (
						PARTITION BY host, method, path
						ORDER BY captured_at DESC
					) as row_num
				FROM requests
				WHERE host IN (%s)
			)
			SELECT id FROM ranked WHERE row_num > ?
		`, hostList)

		args = append(args, maxPerPathEnabled)
		rows, err := db.conn.QueryContext(ctx, query, args...)
		if err != nil {
			return 0, fmt.Errorf("failed to find requests to prune for enabled hosts: %w", err)
		}

		var idsToDelete []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return 0, err
			}
			idsToDelete = append(idsToDelete, id)
		}
		rows.Close()

		// Delete in batches
		for _, id := range idsToDelete {
			db.conn.ExecContext(ctx, "DELETE FROM requests_fts WHERE rowid = ?", id)
			if _, err := db.conn.ExecContext(ctx, "DELETE FROM requests WHERE id = ?", id); err == nil {
				totalDeleted++
			}
		}
	}

	// Part 2: Prune non-enabled hosts - keep only recent N
	var excludeClause string
	var args []interface{}

	if len(enabledHosts) > 0 {
		placeholders := make([]string, len(enabledHosts))
		for i, h := range enabledHosts {
			placeholders[i] = "?"
			args = append(args, h)
		}
		excludeClause = fmt.Sprintf("WHERE host NOT IN (%s)", strings.Join(placeholders, ", "))
	}

	// Find IDs to keep (most recent N for non-enabled hosts)
	keepQuery := fmt.Sprintf(`
		SELECT id FROM requests %s
		ORDER BY captured_at DESC
		LIMIT ?
	`, excludeClause)
	args = append(args, ephemeralLimit)

	rows, err := db.conn.QueryContext(ctx, keepQuery, args...)
	if err != nil {
		return totalDeleted, fmt.Errorf("failed to find requests to keep: %w", err)
	}

	keepIDs := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return totalDeleted, err
		}
		keepIDs[id] = true
	}
	rows.Close()

	// Delete all non-enabled host requests that aren't in keepIDs
	var deleteQuery string
	var deleteArgs []interface{}

	if len(enabledHosts) > 0 {
		placeholders := make([]string, len(enabledHosts))
		for i, h := range enabledHosts {
			placeholders[i] = "?"
			deleteArgs = append(deleteArgs, h)
		}
		deleteQuery = fmt.Sprintf(`
			SELECT id FROM requests
			WHERE host NOT IN (%s)
		`, strings.Join(placeholders, ", "))
	} else {
		deleteQuery = "SELECT id FROM requests"
	}

	rows, err = db.conn.QueryContext(ctx, deleteQuery, deleteArgs...)
	if err != nil {
		return totalDeleted, fmt.Errorf("failed to list non-enabled host requests: %w", err)
	}

	var idsToDelete []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return totalDeleted, err
		}
		if !keepIDs[id] {
			idsToDelete = append(idsToDelete, id)
		}
	}
	rows.Close()

	// Delete in batches
	for _, id := range idsToDelete {
		db.conn.ExecContext(ctx, "DELETE FROM requests_fts WHERE rowid = ?", id)
		if _, err := db.conn.ExecContext(ctx, "DELETE FROM requests WHERE id = ?", id); err == nil {
			totalDeleted++
		}
	}

	return totalDeleted, nil
}

// GetStats returns overall statistics
func (db *DB) GetStats(ctx context.Context, period string) (*models.Stats, error) {
	stats := &models.Stats{
		ByMethod: make(map[string]int64),
		ByStatus: make(map[string]int64),
	}

	// Build time filter
	var timeFilter string
	var filterArg interface{}
	switch period {
	case "hour":
		timeFilter = "WHERE r.captured_at >= datetime('now', '-1 hour')"
	case "day":
		timeFilter = "WHERE r.captured_at >= datetime('now', '-1 day')"
	case "week":
		timeFilter = "WHERE r.captured_at >= datetime('now', '-7 days')"
	case "month":
		timeFilter = "WHERE r.captured_at >= datetime('now', '-30 days')"
	default:
		timeFilter = ""
	}
	_ = filterArg

	// Total requests
	db.conn.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM requests r %s", timeFilter)).Scan(&stats.TotalRequests)

	// Total responses
	db.conn.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM responses resp
		JOIN requests r ON resp.request_id = r.id %s
	`, timeFilter)).Scan(&stats.TotalResponses)

	// Total WebSocket messages
	db.conn.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM websocket_messages ws
		JOIN requests r ON ws.request_id = r.id %s
	`, timeFilter)).Scan(&stats.TotalWebSocketMessages)

	// By method
	rows, _ := db.conn.QueryContext(ctx, fmt.Sprintf(`
		SELECT method, COUNT(*) FROM requests r %s GROUP BY method
	`, timeFilter))
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var method string
			var count int64
			rows.Scan(&method, &count)
			stats.ByMethod[method] = count
		}
	}

	// By status
	rows, _ = db.conn.QueryContext(ctx, fmt.Sprintf(`
		SELECT
			CASE
				WHEN resp.status_code >= 200 AND resp.status_code < 300 THEN '2xx'
				WHEN resp.status_code >= 300 AND resp.status_code < 400 THEN '3xx'
				WHEN resp.status_code >= 400 AND resp.status_code < 500 THEN '4xx'
				WHEN resp.status_code >= 500 THEN '5xx'
				ELSE 'other'
			END as status_group,
			COUNT(*)
		FROM responses resp
		JOIN requests r ON resp.request_id = r.id %s
		GROUP BY status_group
	`, timeFilter))
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int64
			rows.Scan(&status, &count)
			stats.ByStatus[status] = count
		}
	}

	// Top hosts
	rows, _ = db.conn.QueryContext(ctx, fmt.Sprintf(`
		SELECT host, COUNT(*) as cnt FROM requests r %s
		GROUP BY host ORDER BY cnt DESC LIMIT 10
	`, timeFilter))
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var host string
			var count int64
			rows.Scan(&host, &count)
			stats.TopHosts = append(stats.TopHosts, &models.HostCount{Host: host, Count: count})
		}
	}

	// Average latency
	db.conn.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COALESCE(AVG(resp.latency_ms), 0) FROM responses resp
		JOIN requests r ON resp.request_id = r.id %s
	`, timeFilter)).Scan(&stats.AverageLatencyMs)

	return stats, nil
}

// GetTimeline returns traffic timeline data
func (db *DB) GetTimeline(ctx context.Context, from, to time.Time, interval string) ([]*models.TimelinePoint, error) {
	var format string
	switch interval {
	case "minute":
		format = "%Y-%m-%d %H:%M:00"
	case "hour":
		format = "%Y-%m-%d %H:00:00"
	case "day":
		format = "%Y-%m-%d"
	default:
		format = "%Y-%m-%d %H:00:00"
	}

	query := fmt.Sprintf(`
		SELECT strftime('%s', captured_at) as ts, COUNT(*), AVG(COALESCE(
			(SELECT latency_ms FROM responses WHERE request_id = r.id), 0
		))
		FROM requests r
		WHERE captured_at >= ? AND captured_at <= ?
		GROUP BY ts
		ORDER BY ts
	`, format)

	rows, err := db.conn.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []*models.TimelinePoint
	for rows.Next() {
		var point models.TimelinePoint
		if err := rows.Scan(&point.Timestamp, &point.Count, &point.AvgLatency); err != nil {
			return nil, err
		}
		points = append(points, &point)
	}

	return points, nil
}

// HostInfo contains host information for the sidebar
type HostInfo struct {
	Host         string `json:"host"`
	CaptureCount int64  `json:"capture_count"`
	LastSeenAt   string `json:"last_seen_at"`
}

// GetDistinctHosts returns unique hosts with their capture counts (lightweight query for sidebar)
func (db *DB) GetDistinctHosts(ctx context.Context) ([]*HostInfo, error) {
	query := `
		SELECT host, COUNT(*) as cnt, MAX(captured_at) as last_seen
		FROM requests
		GROUP BY host
		ORDER BY last_seen DESC
	`

	rows, err := db.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query hosts: %w", err)
	}
	defer rows.Close()

	var hosts []*HostInfo
	for rows.Next() {
		var h HostInfo
		if err := rows.Scan(&h.Host, &h.CaptureCount, &h.LastSeenAt); err != nil {
			return nil, fmt.Errorf("failed to scan host: %w", err)
		}
		hosts = append(hosts, &h)
	}

	return hosts, nil
}
