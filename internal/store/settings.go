package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/NetSepio/gateway/internal/config"
)

// LoadPlatformSettings reads platform_settings and parses into PlatformSettings.
func (s *Store) LoadPlatformSettings(ctx context.Context) (config.PlatformValues, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM platform_settings`)
	if err != nil {
		return config.PlatformValues{}, fmt.Errorf("load platform settings: %w", err)
	}
	defer rows.Close()

	raw := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return config.PlatformValues{}, err
		}
		raw[key] = value
	}
	if err := rows.Err(); err != nil {
		return config.PlatformValues{}, err
	}
	return config.ParsePlatformSettings(raw)
}

// ListPlatformSettings returns all rows for the admin settings API.
func (s *Store) ListPlatformSettings(ctx context.Context) ([]config.PlatformSettingMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value, COALESCE(description, '') FROM platform_settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("list platform settings: %w", err)
	}
	defer rows.Close()

	var out []config.PlatformSettingMeta
	for rows.Next() {
		var m config.PlatformSettingMeta
		if err := rows.Scan(&m.Key, &m.Value, &m.Description); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpdatePlatformSettings patches known keys; validates by re-parsing the full set.
func (s *Store) UpdatePlatformSettings(ctx context.Context, updates map[string]string) (config.PlatformValues, error) {
	if len(updates) == 0 {
		return s.LoadPlatformSettings(ctx)
	}

	known := make(map[string]bool, len(config.KnownPlatformKeys))
	for _, k := range config.KnownPlatformKeys {
		known[k] = true
	}
	for k := range updates {
		if !known[k] {
			return config.PlatformValues{}, fmt.Errorf("unknown setting %q", k)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return config.PlatformValues{}, err
	}
	defer func() { _ = tx.Rollback() }()

	for key, value := range updates {
		value = strings.TrimSpace(value)
		if value == "" {
			return config.PlatformValues{}, fmt.Errorf("setting %q cannot be empty", key)
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE platform_settings SET value = $1, updated_at = now() WHERE key = $2`,
			value, key)
		if err != nil {
			return config.PlatformValues{}, err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return config.PlatformValues{}, fmt.Errorf("setting %q not found", key)
		}
	}

	if err := tx.Commit(); err != nil {
		return config.PlatformValues{}, err
	}

	parsed, err := s.LoadPlatformSettings(ctx)
	if err != nil {
		return config.PlatformValues{}, err
	}
	s.SetTierThresholds(parsed.XPTierThresholds)
	return parsed, nil
}
