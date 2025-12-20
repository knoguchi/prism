package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// GetConfig retrieves a config value by key
func (db *DB) GetConfig(ctx context.Context, key string) (string, error) {
	var value string
	err := db.conn.QueryRowContext(ctx, "SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetConfig sets a config value
func (db *DB) SetConfig(ctx context.Context, key, value string) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO config (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value, time.Now())
	return err
}

// GetEnabledHosts returns the list of enabled hosts for traffic display
func (db *DB) GetEnabledHosts(ctx context.Context) ([]string, error) {
	value, err := db.GetConfig(ctx, "enabled_hosts")
	if err != nil {
		return nil, err
	}
	if value == "" {
		return []string{}, nil
	}

	var hosts []string
	if err := json.Unmarshal([]byte(value), &hosts); err != nil {
		return nil, err
	}
	return hosts, nil
}

// SetEnabledHosts saves the list of enabled hosts
func (db *DB) SetEnabledHosts(ctx context.Context, hosts []string) error {
	data, err := json.Marshal(hosts)
	if err != nil {
		return err
	}
	return db.SetConfig(ctx, "enabled_hosts", string(data))
}

// AddEnabledHost adds a host to the enabled list
func (db *DB) AddEnabledHost(ctx context.Context, host string) error {
	hosts, err := db.GetEnabledHosts(ctx)
	if err != nil {
		return err
	}

	// Check if already exists
	for _, h := range hosts {
		if h == host {
			return nil
		}
	}

	hosts = append(hosts, host)
	return db.SetEnabledHosts(ctx, hosts)
}

// RemoveEnabledHost removes a host from the enabled list
func (db *DB) RemoveEnabledHost(ctx context.Context, host string) error {
	hosts, err := db.GetEnabledHosts(ctx)
	if err != nil {
		return err
	}

	// Filter out the host
	filtered := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if h != host {
			filtered = append(filtered, h)
		}
	}

	return db.SetEnabledHosts(ctx, filtered)
}
